package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func TestCORSAllowsListedOrigin(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", "http://localhost:3000,https://ui.example.com")
	r := newRouter()

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://ui.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://ui.example.com" {
		t.Fatalf("Allow-Origin = %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Allow-Credentials = %q", got)
	}
}

func TestCORSIgnoresUnlistedOrigin(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", "http://localhost:3000")
	r := newRouter()

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Allow-Origin unexpectedly set to %q", got)
	}
}

func TestCORSPreflightShortCircuits(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", "http://localhost:3000")
	r := newRouter()

	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight code = %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("Allow-Methods missing on preflight")
	}
}
