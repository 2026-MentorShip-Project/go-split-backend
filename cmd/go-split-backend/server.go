package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"go-split-backend/internal/controller"
	"go-split-backend/internal/database"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	swaggerfiles "github.com/swaggo/files"
	ginswagger "github.com/swaggo/gin-swagger"

	_ "go-split-backend/docs"
)

// @title go-split-backend service
// @version 1.0
// @basePath /
// @schemes https
//
//go:generate swag init -d ../../ -g cmd/go-split-backend/server.go -o ../../docs

func main() {
	db, err := database.Open(context.Background())
	if err != nil {
		log.Fatalf("Failed to connect to database, err: %v", err)
	}
	defer db.Close()

	// Example GIN service
	r := gin.New()

	r.Use(gin.RecoveryWithWriter(io.Discard, jsonRecoveryHandler))

	r.GET("/healthz", controller.HealthCheck)
	r.GET("/swagger/*any", ginswagger.WrapHandler(swaggerfiles.Handler))
	s := &http.Server{
		Addr:    ":8080",
		Handler: r,
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

	log.Info("API server up and running")

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
