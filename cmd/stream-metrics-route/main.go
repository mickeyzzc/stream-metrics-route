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
	pprofEnabled     = flag.Bool("pprof.enabled", true, "Enable pprof debug endpoints")
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

	receiver := receive.NewReceive(*maxRequestSize, *writeTimeout)

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

	if *pprofEnabled {
		route.GET("/debug/pprof/*any", gin.WrapH(http.DefaultServeMux))
	}

	srv := &http.Server{
		Addr:         ":" + *listenPort,
		Handler:      route,
		WriteTimeout: *writeTimeout,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			defaultTelemetry.Logger.Error("server error", err)
		}
	}()

	// Second SIGTERM forces immediate exit
	go func() {
		<-ch
		defaultTelemetry.Logger.Info("forced shutdown")
		os.Exit(1)
	}()

	// Wait for first shutdown signal
	sig := <-ch
	defaultTelemetry.Logger.Info("received signal, shutting down", map[string]interface{}{"signal": sig.String()})
	health = false

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), *writeTimeout+5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		defaultTelemetry.Logger.Error("server shutdown error", err)
	}

	defaultTelemetry.Logger.Info("server stopped")
}
