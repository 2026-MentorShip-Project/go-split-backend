package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"go-split-backend/internal/config"
	"go-split-backend/internal/controller"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// @title go-split-backend service
// @version 1.0
// @description go-split-backend service for recommendation ecosystem
// @contact.email ai-rec-sys@appier.com
// @basePath /
// @schemes https
//
//go:generate swag init -d ../../ -g cmd/go-split-backend/server.go -o ../../docs

func main() {
	// Parse config
	var cf = flag.String("c", "", "config file")
	flag.Parse()

	cfg := &config.Config{}
	if err := config.Load(*cf, cfg); err != nil {
		log.Fatalf("Failed to load config, err: %v", err)
	}
	// logkit.InitLogging(cfg.Logging, &logkit.BaseLogFormat{})
	log.Debug("Config loaded successfully: %v", cfg)
	// // Init tracer
	// shutdownFunc := initTracer(cfg.Tracing)
	// defer func() {
	// 	if err := shutdownFunc(context.Background()); err != nil {
	// 		log.Errorf("Fail to shutdown tracer provider, err: %v.", err)
	// 	}
	// }()

	// Example GIN service
	r := gin.New()

	if cfg.EnableGinLogger {
		r.Use(gin.Logger())
	}

	r.Use(gin.RecoveryWithWriter(io.Discard, jsonRecoveryHandler))
	// if cfg.Logging.Format == "json" {
	// } else {
	// 	r.Use(gin.Recovery())
	// }

	r.GET("/healthz", controller.HealthCheck)
	s := &http.Server{
		Addr: "0.0.0.0:8080",
		// Handler: tracekit.OtelHTTPHandler(r, "go-split-backend", cfg.Tracing),
	}

	defer func() {
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.Shutdown(timeoutCtx); err != nil {
			log.Fatalf("Failed to shutdown server, err: %v", err)
		}
	}()
	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to listen and serve http server, err: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Info("Shutting down server ...")
}

func jsonRecoveryHandler(ctx *gin.Context, recovered any) {
	log.WithContext(ctx).WithField("stack", string(debug.Stack())).Error(fmt.Sprintf("%v", recovered))
	ctx.AbortWithStatus(http.StatusInternalServerError)
}

// func initTracer(cfg tracekit.Config) func(context.Context) error {
// 	if !cfg.Enable {
// 		return func(context.Context) error { return nil }
// 	}

// 	shutdownFunc, err := tracekit.InitProvider(cfg)
// 	if err != nil {
// 		log.Errorf("Fail to initialize tracer provider, err: %v", err)
// 		return func(context.Context) error { return nil }
// 	}

// 	return shutdownFunc
// }
