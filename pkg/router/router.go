package router

import (
	"context"
	"fmt"
	"stream-metrics-route/pkg/kafkaclient"
	"stream-metrics-route/pkg/remote"
	"stream-metrics-route/pkg/setting"
	"stream-metrics-route/pkg/telemetry"
	"strings"
	"sync"
	"time"

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

func Store(ctx context.Context, req []prompb.TimeSeries) []StoreResult {
	return DefaultRouters.Store(ctx, req)
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

type StoreResult struct {
	RouterName string
	Error     error
	Count     int
}

func (rs *Routers) Store(ctx context.Context, req []prompb.TimeSeries) []StoreResult {
	rs.lock.RLock()
	defer rs.lock.RUnlock()

	defaultTelemetry.Logger.Debug("store num ,", "len", len(rs.Routers))
	if len(rs.Routers) == 0 {
		return []StoreResult{{Error: fmt.Errorf("no routers configured")}}
	}

	routerTimeseries.WithLabelValues("all").Add(float64(len(req)))

	var wg sync.WaitGroup
	mu := sync.Mutex{}
	results := make([]StoreResult, 0)
	resultsMap := make(map[string]StoreResult)

	for _, r := range rs.Routers {
		defaultTelemetry.Logger.Debug("store ", "name", r.Name, "len", len(req))
		filterTs := r.filterLabels(req)
		if len(filterTs) == 0 {
			defaultTelemetry.Logger.Debug("filter timeseries null ", "name", r.Name)
			continue
		}
		routerTimeseries.WithLabelValues(r.Name).Add(float64(len(filterTs)))

		wg.Add(1)
		go func(router *Router, timeseries []prompb.TimeSeries) {
			defer wg.Done()

			start := time.Now()
			err := router.RemoteStore.Store(ctx, timeseries)
			duration := time.Since(start).Seconds()

			routerWriteDuration.WithLabelValues(router.Name).Observe(duration)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				routerErrors.WithLabelValues(router.Name, classifyError(err)).Inc()
				routerFalseTimeseries.WithLabelValues(router.Name).Add(float64(len(timeseries)))
				defaultTelemetry.Logger.Error("remote store error", "err", err, "router", router.Name)
				resultsMap[router.Name] = StoreResult{
					RouterName: router.Name,
					Error:      err,
					Count:      len(timeseries),
				}
			} else {
				resultsMap[router.Name] = StoreResult{
					RouterName: router.Name,
					Error:      nil,
					Count:      len(timeseries),
				}
			}
		}(r, filterTs)
	}

	wg.Wait()

	for _, r := range results {
		results = append(results, r)
	}
	for _, result := range resultsMap {
		results = append(results, result)
	}

	return results
}

func (rs *Routers) IsHealthy() bool {
	rs.lock.RLock()
	defer rs.lock.RUnlock()

	for _, r := range rs.Routers {
		if !r.RemoteStore.IsHealthy() {
			return false
		}
	}
	return true
}

func (rs *Routers) GetRouterStats() map[string]interface{} {
	rs.lock.RLock()
	defer rs.lock.RUnlock()

	stats := make(map[string]interface{})
	for name, r := range rs.Routers {
		stats[name] = r.RemoteStore.GetStats()
	}
	return stats
}

type Router struct {
	Name                 string
	MetricRelabelConfigs []*relabel.Config
	RemoteStore          RemoteStore
}

func (r *Router) filterLabels(ts []prompb.TimeSeries) []prompb.TimeSeries {
	filtered := make([]prompb.TimeSeries, 0, len(ts))
	for _, t := range ts {
		if len(t.Labels) == 0 {
			continue
		}
		lbs := formatLabelSet(t.Labels)
		lbls, keep := relabel.Process(lbs, r.MetricRelabelConfigs...)
		if !keep || lbls.IsEmpty() {
			continue
		}
		newLabels := make([]prompb.Label, 0, len(lbls))
		for _, l := range lbls {
			newLabels = append(newLabels, prompb.Label{
				Name:  l.Name,
				Value: l.Value,
			})
		}
		filtered = append(filtered, prompb.TimeSeries{
			Labels:  newLabels,
			Samples: t.Samples,
		})
	}
	return filtered
}

func formatLabelSet(lb []prompb.Label) labels.Labels {
	var m = make(map[string]string, 0)
	for _, v := range lb {
		m[v.Name] = v.Value
	}
	return labels.FromMap(m)
}

type MultiError struct {
	Errors []error
}

func (me *MultiError) Error() string {
	if len(me.Errors) == 0 {
		return ""
	}
	if len(me.Errors) == 1 {
		return me.Errors[0].Error()
	}
	return fmt.Sprintf("%d errors: first error: %v", len(me.Errors), me.Errors[0])
}

func (me *MultiError) Add(err error) {
	if err != nil {
		me.Errors = append(me.Errors, err)
	}
}

func (me *MultiError) HasError() bool {
	return len(me.Errors) > 0
}

func classifyError(err error) string {
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "timeout"):
		return "timeout"
	case strings.Contains(errStr, "connection"):
		return "connection"
	case strings.Contains(errStr, "circuit"):
		return "circuit_breaker"
	default:
		return "unknown"
	}
}
