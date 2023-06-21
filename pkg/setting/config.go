package setting

import (
	"fmt"
	"os"

	"github.com/prometheus/prometheus/model/relabel"
	"gopkg.in/yaml.v2"
)

var (
	DefaultConfig = Config{
		GlobalConfig: GlobalConf{
			Prefix: "stream-metrics-route",
		},
		RouterRule: []RouterRuleConf{},
	}
)

type Config struct {
	GlobalConfig GlobalConf       `yaml:"global"`
	RouterRule   []RouterRuleConf `yaml:"router_rules"`
}

type GlobalConf struct {
	Prefix string `yaml:"prefix"`
}

type RouterRuleConf struct {
	RouterName string        `yaml:"router_name"`
	UpStreams  UpStreamsConf `yaml:"upstreams"`

	HashLabels           HashLabels        `yaml:"hash_labels,omitempty"`
	MetricRelabelConfigs []*relabel.Config `yaml:"metric_relabel_configs,omitempty"`
}

type UpStreamsConf struct {
	UpStreamsType RemoteType  `yaml:"upstream_type"`
	UpstreamUrls  []string    `yaml:"upstream_urls,omitempty"`
	KafkaConfig   KafkaConfig `yaml:"kafka_config,omitempty"`
}

type RemoteType string

const (
	Kafka        RemoteType = "kafka"
	RemoteWriter RemoteType = "remotewriter"
)

type HashLabels struct {
	Mode   int      `yaml:"mode"`
	Labels []string `yaml:"labels"`
}

func (c Config) String() string {
	b, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Sprintf("<error creating config string: %s>", err)
	}
	return string(b)
}

func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = Config{}
	type plain Config
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	return nil
}

func LoadFile(filename string) (*Config, error) {

	cfg, err := loadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("parsing YAML file %s: %w", filename, err)
	}
	DefaultConfig = *cfg
	return cfg, nil
}

func loadFile(filename string) (*Config, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	cfg, err := Load(string(content))
	if err != nil {
		return nil, fmt.Errorf("parsing YAML file %s: %w", filename, err)
	}

	return cfg, nil
}

func Load(s string) (*Config, error) {
	cfg := &Config{}
	//cfg = DefaultConfig
	err := yaml.UnmarshalStrict([]byte(s), cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
