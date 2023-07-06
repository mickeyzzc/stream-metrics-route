package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"stream-metrics-route/pkg/receive"
	"stream-metrics-route/pkg/router"
	"stream-metrics-route/pkg/setting"
	"stream-metrics-route/pkg/telemetry"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	defaultConfigName    = "config.yaml"
	defaultConfigPath, _ = os.Getwd()
	// 命令行参数
	confPath         = flag.String("config.path", defaultConfigPath, "Config Path.")
	confName         = flag.String("config.name", defaultConfigName, "default name 'config.yaml'")
	logLevel         = flag.String("log.level", "info", "debug or info")
	listenPort       = flag.String("listen.port", "8080", "listen port")
	configFile       = ""
	receiver         receive.Receive
	route            = gin.Default()
	defaultTelemetry = telemetry.NewTelemetry()
	health           = true
	defaultCfg       = &setting.Config{}
)

func init() {
	flag.Parse()
	configFile = *confPath + "/" + *confName
	var err error
	defaultCfg, err = setting.LoadFile(configFile)
	if err != nil { // 处理读取配置文件的错误
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}
	defaultTelemetry.LevelSet(context.Background(), *logLevel)

	route.GET("/metrics", gin.WrapH(
		promhttp.HandlerFor(defaultTelemetry.Metrics, promhttp.HandlerOpts{}),
	))
	router.BuildRouters(defaultCfg)
}

func main() {

	router_v1 := route.Group("api/v1")
	router_v1.POST("write", receiver.Handler())
	router_v1.POST("receive", receiver.Handler())
	// Set up channel to receive signals
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)

	// Set up router with health and readiness endpoints
	route.GET("/-/ready", receive.CheckReady)
	route.GET("/-/health", func(c *gin.Context) {
		data := gin.H{"code": 0, "msg": "no health", "data": nil}
		if health {
			if receive.CheckHealthy(c) {
				data = gin.H{"code": 2000, "msg": "ok", "data": nil}
				c.JSON(http.StatusOK, data)
				return
			}
		}
		defaultTelemetry.Logger.Error("not health now.", nil)
		c.JSON(http.StatusInternalServerError, data)
	})

	// Start router in a goroutine
	go route.Run(":" + *listenPort)

	// Listen for signals
	for {
		select {
		case <-ch:
			// Gracefully shutdown server on signal
			health = false
			receive.CheckWriteTask(200 * time.Millisecond)
			os.Exit(0)
		}
	}
}
