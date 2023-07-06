package remote

import (
	"context"
	"hash/fnv"
	"sort"
	"strconv"

	"github.com/prometheus/prometheus/prompb"
)

type RemoteCluster struct {
	uplen        int
	dimension    int
	filterLabels []string
	Writers      map[int]*RemoteWriterUrl
	Name         string
}

func NewRemoteCluster(name string, dimension int, filterLabels []string, Urls []string) *RemoteCluster {
	writers := make(map[int]*RemoteWriterUrl, len(Urls))
	for k, v := range Urls {
		writers[k] = NewRemoteWriterUrl(v)
	}
	return &RemoteCluster{
		Name:         name,
		uplen:        len(Urls),
		dimension:    dimension,
		filterLabels: filterLabels,
		Writers:      writers,
	}
}

// Store stores the given time series data using the remote cluster.
//
// ctx: the context for the request.
// req: the time series data to be stored.
// Returns the number of time series stored and any possible errors.
func (r *RemoteCluster) Store(ctx context.Context, req []prompb.TimeSeries) (int, error) {
	var sendSamplesChan = make(map[int][]prompb.TimeSeries, r.uplen)
	remoteWriteClusterTimeseries.WithLabelValues(r.Name).Add(float64(len(req)))
	for _, ts := range req {
		if len(ts.Labels) == 0 {
			remoteWriteClusterFalseTimeseries.WithLabelValues(r.Name).Inc()
			continue
		}
		if r.uplen > 1 {
			hash := sortLabelsHashKey(ts.Labels)
			dime := hashMod(r.dimension, hash)
			ts.Labels = append(ts.Labels, prompb.Label{
				Name:  "stream_task_id",
				Value: strconv.Itoa(dime),
			})
			sendSeries := prompb.TimeSeries{
				Labels:  ts.Labels,
				Samples: ts.GetSamples(),
			}
			var hashnode = func(r *RemoteCluster, hash uint32) uint32 {
				if len(r.filterLabels) > 0 {
					var tmpLabels = []prompb.Label{}
					for _, rlabel := range r.filterLabels {
						for _, label := range ts.Labels {
							if label.Name == rlabel {
								tmpLabels = append(tmpLabels, label)
							}
						}
					}
					return sortLabelsHashKey(tmpLabels)
				}
				return hash
			}(r, hash)
			tmpch := hashMod(r.uplen, hashnode)
			defaultTelemetry.Logger.Debug("hash Result", "uplen", r.uplen, "hashnode", hashnode, "chanlenum", tmpch)
			if _, ok := r.Writers[tmpch]; ok {
				sendSamplesChan[tmpch] = append(sendSamplesChan[tmpch], sendSeries)
			}
		} else {
			sendSeries := prompb.TimeSeries{
				Labels:  ts.Labels,
				Samples: ts.GetSamples(),
			}
			sendSamplesChan[0] = append(sendSamplesChan[0], sendSeries)
		}

	}
	for index, tsdata := range sendSamplesChan {
		defaultTelemetry.Logger.Debug("send samples", "index", index, "len", len(tsdata))
		go r.Writers[index].Store(ctx, tsdata)
	}
	return 0, nil
}

func sortLabelsHashKey(labels []prompb.Label) uint32 {
	newLabel := make([]string, 0, len(labels)*2)
	for _, lal := range labels {
		newLabel = append(newLabel, lal.Name)
		newLabel = append(newLabel, lal.Value)
	}

	sort.Strings(newLabel)
	h := fnv.New32a()
	for _, v := range newLabel {
		h.Write([]byte(v))
	}
	return h.Sum32()
}
