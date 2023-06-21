package setting_test

import (
	"stream-metrics-route/pkg/setting"
	"testing"
)

func TestConfigRead(t *testing.T) {
	testYaml := `
global:
  prefix: "global"
router_rules:
  - router_name: vmagent
    upstreams:
      upstream_type: kafka
      upstream_urls:
        - http://monitoring-vmagent-0.monitoring-vmagent.monitoring.svc:8429/api/v1/write
        - http://monitoring-vmagent-1.monitoring-vmagent.monitoring.svc:8429/api/v1/write
        - http://monitoring-vmagent-2.monitoring-vmagent.monitoring.svc:8429/api/v1/write

`
	cfg, err := setting.Load(testYaml)
	if err != nil {
		t.Error("parsing YAML file ", err)
		return
	}

	t.Log("\n", cfg.String())
}
