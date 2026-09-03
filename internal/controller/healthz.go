package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthCheck godoc
// @Summary 	Health check
// @Description Usage for checking service liveness
// @Produce 	json
// @Success 	200 "ok"
// @Router 		/healthz [get]
func HealthCheck(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"message": "ok",
	})
}
