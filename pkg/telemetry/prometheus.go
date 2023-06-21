package telemetry

import "github.com/prometheus/client_golang/prometheus"

var (
	defaultRegistry = prometheus.NewRegistry()
)

func init() {
	defaultRegistry.MustRegister(
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		prometheus.NewGoCollector(),
		prometheus.NewBuildInfoCollector(),
	)
}

func (o *Telemetry) Register(metrice prometheus.Collector) {
	o.Metrics.Register(metrice)
}
