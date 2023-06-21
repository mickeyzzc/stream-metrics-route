package telemetry

import (
	"fmt"
	"io"

	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/config"
)

var defaultTracer = new(tracer)

type tracer struct {
	opentracing.Tracer
	io.Closer
}

type JaegerConf struct {
	agentAddr    string
	samplerType  string
	samplerParam float64
	Cfg          *config.Configuration
}

/*
1 Constant 常量决策。
	类型：sampler.type=const
	值：
		sampler.param=1：全部采样
		sampler.param=0：全部丢弃

2 Probabilistic 概率决策。
	类型：sampler.type=probabilistic
	值：sampler.param=0.1表示 采样 1/10

3 Rate Limiting 频次策略。
	类型：sampler.type=ratelimiting
	值：sampler.param=2.0表示 每秒 采样2次

4 Remote 远端决策。
	类型：sampler.type=remote
*/

func (j *JaegerConf) initJaeger() {
	j.Cfg = &config.Configuration{
		Sampler: &config.SamplerConfig{
			Type:  j.samplerType,
			Param: j.samplerParam,
		},
		Reporter: &config.ReporterConfig{
			LogSpans:           false,
			LocalAgentHostPort: j.agentAddr,
		},
	}
}

func (j *JaegerConf) jaegerTracerNew(service string) *tracer {
	trace, closer, err := j.Cfg.New(service, config.Logger(jaeger.NullLogger))
	if err != nil {
		panic(fmt.Sprintf("ERROR: init Jaeger %v\n", err))
	}
	return &tracer{
		trace,
		closer,
	}
}

func jaegerConfNew(agentAddr, samplerType string) *JaegerConf {
	samplerParam := 0.0
	switch samplerType {
	case "const":
		samplerParam = 1.0
	case "probabilistic":
		samplerParam = 0.2
	case "ratelimiting":
		samplerParam = 2.0
	case "remote":
		samplerParam = 0.0
	default:
		samplerType = "ratelimiting"
		samplerParam = 2.0
	}

	return &JaegerConf{
		agentAddr:    agentAddr,
		samplerType:  samplerType,
		samplerParam: samplerParam,
	}
}

func TraceClose() {
	defaultTracer.Close()
}
