package auth

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCurrentSubjectWithoutMiddleware(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if got := CurrentSubject(c); got.IsHost() || got.IsGuest() {
		t.Fatalf("CurrentSubject on bare context = %+v; want zero Subject", got)
	}
}

func TestCurrentSubjectRoundTrip(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	want := Subject{HostID: 42}
	c.Set(subjectCtxKey, want)
	if got := CurrentSubject(c); got != want {
		t.Fatalf("CurrentSubject = %+v; want %+v", got, want)
	}
}
