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
	confPath         = flag.String("config.path", defaultConfigPath, "Config Path.")
	confName         = flag.String("config.name", defaultConfigName, "default name 'config.yaml'")
	logLevel         = flag.String("log.level", "info", "debug or info")
	listenPort       = flag.String("listen.port", "8080", "listen port")
	maxRequestSize   = flag.Int64("max.request.size", 100*1024*1024, "max request size in bytes")
	writeTimeout     = flag.Duration("write.timeout", 30*time.Second, "write timeout")
	configFile       = ""
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
	if err != nil {
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}
	defaultTelemetry.LevelSet(context.Background(), *logLevel)

	route.GET("/metrics", gin.WrapH(
		promhttp.HandlerFor(defaultTelemetry.Metrics, promhttp.HandlerOpts{}),
	))
	router.BuildRouters(defaultCfg)
}

func main() {

	receiver = receive.NewReceive(*maxRequestSize, *writeTimeout)

	router_v1 := route.Group("api/v1")
	router_v1.POST("write", receiver.Handler())
	router_v1.POST("receive", receiver.Handler())

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)

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

	route.GET("/stats", func(c *gin.Context) {
		stats := router.GetRouters().GetRouterStats()
		c.JSON(http.StatusOK, gin.H{
			"code": 2000,
			"msg":  "ok",
			"data": stats,
		})
	})

	go route.Run(":" + *listenPort)

	for {
		select {
		case <-ch:
			health = false
			receive.CheckWriteTask(200 * time.Millisecond)
			os.Exit(0)
		}
	}
}
