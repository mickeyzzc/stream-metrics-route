package setting

type KafkaConfig struct {
	Async                 bool                 `yaml:"async,omitempty"`
	KafkaBatchNumMessages int                  `yaml:"batch_num_messages,omitempty"`
	KafkaBatchBytes       int64                `yaml:"batch_bytes,omitempty"`
	KafkaBrokerList       string               `yaml:"broker_list"`
	KafkaTopic            string               `yaml:"topic"`
	Balancer              string               `yaml:"balancer,omitempty"`
	Match                 string               `yaml:"match,omitempty"`
	Basicauth             KafkaBasicAuthConfig `yaml:"basicauth,omitempty"`
	KafkaCompression      string               `yaml:"compression,omitempty"`
	SecurityProtocol      string               `yaml:"security_protocol,omitempty"`
	KafkaSslClient        KafkaSSLConfig       `yaml:"ssl_client,omitempty"`
	KafkaSasl             KafkaSaslConfig      `yaml:"sasl,omitempty"`
}

type KafkaBasicAuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type KafkaSSLConfig struct {
	SslClientCertFile string `yaml:"ssl_client_cert_file"`
	SslClientKeyFile  string `yaml:"ssl_client_key_file"`
	SslClientKeyPass  string `yaml:"ssl_client_key_pass"`
	SslCACertFile     string `yaml:"ssl_cacert_file"`
}

type KafkaSaslConfig struct {
	SaslMechanism string `yaml:"sasl_mechanism"`
	SaslUsername  string `yaml:"sasl_username"`
	SaslPassword  string `yaml:"sasl_password"`
}
