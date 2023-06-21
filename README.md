# Stream Metrics Route （指标流路由）
Stream Metrics Route is a Golang application that receives monitoring metrics from Prometheus remote write and distributes them to various backend destinations based on route configuration. Metrics can be forwarded to supported backend types including:
- Prometheus remote write
- Kafka
By using Prometheus relabeling, metrics received from Prometheus can be dynamically routed to appropriate backend endpoints. This allows for a flexible metrics flow and processing pipeline.
## Features
- Supports Prometheus remote write as input
- Supports Prometheus relabeling for dynamic routing of metrics
- Forwards metrics to Prometheus remote write endpoints
- Streams metrics into Kafka topics
- Golang application with configurable YAML routing files
## Getting Started
1. Create YAML routing configuration files that specify match criteria and backend destinations for metrics
2. Start the Stream Metrics Route application
3. Configure jobs in Prometheus to send metrics to Stream Metrics Route's remote write endpoint
4. Metrics received by Stream Metrics Route will be automatically routed to backends based on match criteria
5. View received metrics in the configured backend systems for monitoring and alerting
An example YAML route configuration may be:

```yaml
router_rules:
- router_name: kafka-node-exporter
  upstreams:
    upstream_type: kafka
    kafka_config:
      broker_list: "kafka1:9092,kafka2:9092"
      topic: base_metrics
  metric_relabel_configs:
  - source_labels: [__name__]
    separator: ;
    regex: node_(cpu|memory|disk|network|filesystem)_.*
    replacement: $1
    action: keep
```

This will send all metrics with job label "node-exporter"  to a Prometheus server, and all metrics matching the name prefix "node_(cpu|memory|disk|network|filesystem)_" to a "base_metrics" Kafka topic.
Stream Metrics Route provides a flexible routing solution for managing the flow of metrics and distribution to various systems. 