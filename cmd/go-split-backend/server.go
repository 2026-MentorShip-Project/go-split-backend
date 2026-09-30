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

	"go-split-backend/internal/auth"
	"go-split-backend/internal/controller"
	"go-split-backend/internal/database"
	"go-split-backend/internal/events"
	"go-split-backend/internal/httpx"
	"go-split-backend/internal/profiling"
	"go-split-backend/internal/ruleassist"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	swaggerfiles "github.com/swaggo/files"
	ginswagger "github.com/swaggo/gin-swagger"

	_ "go-split-backend/docs"
)

// @title go-split-backend service
// @version 1.0
// @basePath /
// @schemes http https
//
//go:generate swag init -d ../../ -g cmd/go-split-backend/server.go -o ../../docs

func main() {
	ctx := context.Background()
	stopProfiler := func() {}
	if stop, err := profiling.Start(ctx); err != nil {
		log.WithError(err).Warn("failed to start CPU profiler")
	} else {
		stopProfiler = stop
	}
	defer stopProfiler()

	db, err := database.Open(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to database, err: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(ctx, db); err != nil {
		log.Fatalf("Failed to run migrations, err: %v", err)
	}

	if _, err := events.SeedTemplates(ctx, db); err != nil {
		log.Fatalf("Failed to seed templates: %v", err)
	}
	r := gin.New()
	// Tag labels travel as path segments and may contain "/" (不喝酒/開車);
	// route on the escaped path so %2F stays inside the label.
	r.UseRawPath = true

	r.Use(httpx.CORS())
	r.Use(gin.RecoveryWithWriter(io.Discard, jsonRecoveryHandler))

	r.GET("/healthz", controller.HealthCheck(db))
	r.GET("/swagger/*any", ginswagger.WrapHandler(swaggerfiles.Handler))

	auth.New(db, auth.NewGoogleVerifier(os.Getenv("GOOGLE_CLIENT_ID"))).Register(r)
	eventsHandler := events.New(db)
	eventsHandler.Drafter = ruleDrafter(ctx)
	eventsHandler.Register(r)
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

// ruleDrafter picks the rule-drafting backend. RULE_DRAFT_FIXTURE=true serves
// canned plans for local development; otherwise Vertex, when configured.
func ruleDrafter(ctx context.Context) ruleassist.Generator {
	if os.Getenv("RULE_DRAFT_FIXTURE") == "true" {
		log.Warn("rule drafting answers from fixtures")
		return ruleassist.SampleFixture()
	}
	g, err := ruleassist.NewVertexGenerator(ctx, ruleassist.ConfigFromEnv())
	if errors.Is(err, ruleassist.ErrNotConfigured) {
		return nil
	}
	if err != nil {
		log.WithError(err).Error("rule drafting disabled")
		return nil
	}
	return g
}

func jsonRecoveryHandler(ctx *gin.Context, recovered any) {
	log.WithContext(ctx).WithField("stack", string(debug.Stack())).Error(fmt.Sprintf("%v", recovered))
	ctx.AbortWithStatus(http.StatusInternalServerError)
}
