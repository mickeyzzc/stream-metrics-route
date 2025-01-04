package router

import (
	"context"
	"stream-metrics-route/pkg/kafkaclient"
	"stream-metrics-route/pkg/remote"
	"stream-metrics-route/pkg/setting"
	"stream-metrics-route/pkg/telemetry"
	"strings"
	"sync"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/prompb"
)

var (
	DefaultRouters = &Routers{
		Routers: make(map[string]*Router, 0),
	}
)

func init() {
	defaultTelemetry = telemetry.NewTelemetry()
}

type Routers struct {
	Routers map[string]*Router
	lock    sync.RWMutex
}

func NewRouters() {
	DefaultRouters.Routers = make(map[string]*Router, 0)
}

func GetRouters() *Routers {
	return DefaultRouters
}

func BuildRouters(cfg *setting.Config) {
	DefaultRouters.lock.Lock()
	defer DefaultRouters.lock.Unlock()
	NewRouters()
	var err error
	for _, r := range cfg.RouterRule {
		var route RemoteStore
		switch r.UpStreams.UpStreamsType {
		case setting.Kafka:
			defaultTelemetry.Logger.Debug("kafka connect", "host", r.UpStreams.KafkaConfig.KafkaBrokerList, "topic", r.UpStreams.KafkaConfig.KafkaTopic)
			route, err = kafkaclient.NewKafka(
				r.RouterName,
				r.UpStreams.KafkaConfig,
			)
			if err != nil {
				defaultTelemetry.Logger.Error("kafka connect error", err)
				continue
			}
			routerInfo.WithLabelValues(r.RouterName, string(r.UpStreams.UpStreamsType), r.UpStreams.KafkaConfig.KafkaBrokerList, r.UpStreams.KafkaConfig.KafkaTopic).Set(1)
		case setting.RemoteWriter:
			defaultTelemetry.Logger.Debug("remote connect", "type", r.UpStreams.UpStreamsType, "urls", r.UpStreams.UpstreamUrls)
			route = remote.NewRemoteCluster(
				r.RouterName,
				r.HashLabels.Mode,
				r.HashLabels.Labels,
				r.UpStreams.UpstreamUrls,
			)
			routerInfo.WithLabelValues(r.RouterName, string(r.UpStreams.UpStreamsType), strings.Join(r.UpStreams.UpstreamUrls, ","), "").Set(1)
		default:
			defaultTelemetry.Logger.Debug("default remote connect", "type", r.UpStreams.UpStreamsType)
			route = remote.NewRemoteCluster(
				r.RouterName,
				r.HashLabels.Mode,
				r.HashLabels.Labels,
				r.UpStreams.UpstreamUrls,
			)
			routerInfo.WithLabelValues(r.RouterName, string(r.UpStreams.UpStreamsType), strings.Join(r.UpStreams.UpstreamUrls, ","), "").Set(1)
		}

		DefaultRouters.Routers[r.RouterName] = &Router{
			Name:                 r.RouterName,
			MetricRelabelConfigs: r.MetricRelabelConfigs,
			RemoteStore:          route,
		}
		defaultTelemetry.Logger.Debug("build router", "name", r.RouterName, "info", route)
	}
}

func (rs *Routers) Store(ctx context.Context, req []prompb.TimeSeries) (int, error) {
	defer ctx.Done()
	defaultTelemetry.Logger.Debug("store num ,", "len", len(rs.Routers))
	if len(rs.Routers) == 0 {
		return 500, nil
	}
	go routerTimeseries.WithLabelValues("all").Add(float64(len(req)))
	for _, r := range rs.Routers {
		defaultTelemetry.Logger.Debug("store ", "name", r.Name, "len", len(req))
		filterTs := r.filterLabels(req)
		if len(filterTs) == 0 {
			defaultTelemetry.Logger.Debug("filter timeseries null ", "name", r.Name)
			continue
		}
		go routerTimeseries.WithLabelValues(r.Name).Add(float64(len(filterTs)))
		defaultTelemetry.Logger.Debug("filter timeseries ", "name", r.Name, "timeseries", len(filterTs))

		if _, err := r.RemoteStore.Store(context.Background(), filterTs); err != nil {
			//TODO: log
			go routerFalseTimeseries.WithLabelValues(r.Name).Add(float64(len(filterTs)))
			defaultTelemetry.Logger.Error("remote store error", "err", err)
		}
	}
	return 0, nil
}

type Router struct {
	Name                 string
	MetricRelabelConfigs []*relabel.Config
	RemoteStore          RemoteStore
}

func (r *Router) filterLabels(ts []prompb.TimeSeries) []prompb.TimeSeries {
	fiterTS := make([]prompb.TimeSeries, 0)
	for _, t := range ts {
		lbs := formatLabelSet(t.Labels)
		lbls, keep := relabel.Process(lbs, r.MetricRelabelConfigs...)
		if !keep || lbls.IsEmpty() {
			continue
		}
		fiterTS = append(fiterTS, t)
	}
	return fiterTS
}

func formatLabelSet(lb []prompb.Label) labels.Labels {
	var m = make(map[string]string, 0)
	for _, v := range lb {
		m[v.Name] = v.Value
	}
	return labels.FromMap(m)
}
