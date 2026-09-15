package httpx

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

const defaultAllowedOrigins = "http://localhost:3000"

// CORS returns a middleware that echoes the request's Origin when it is in
// ALLOWED_ORIGINS (comma-separated, default http://localhost:3000). Sends
// credentialed CORS so the session cookie can ride cross-origin.
// Set ALLOWED_ORIGINS=* to allow any origin for temporary local testing.
func CORS() gin.HandlerFunc {
	allowed := parseOrigins(os.Getenv("ALLOWED_ORIGINS"))
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && (allowed[origin] || allowed["*"]) {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Vary", "Origin")
			if c.Request.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				h.Set("Access-Control-Max-Age", "600")
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
		}
		c.Next()
	}
}

func parseOrigins(raw string) map[string]bool {
	if raw == "" {
		raw = defaultAllowedOrigins
	}
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out[v] = true
		}
	}
	return out
}
