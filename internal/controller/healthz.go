package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthCheck godoc
// @Summary     Health check
// @Description Pings the database pool with a 1s timeout. Returns 503 when
// @Description the pool is not reachable so Cloud Run stops routing traffic.
// @Produce     json
// @Success     200 "ok"
// @Failure     503 "database unavailable"
// @Router      /healthz [get]
func HealthCheck(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	}
}
