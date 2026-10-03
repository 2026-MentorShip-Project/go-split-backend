package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSessionToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		cookie string
		header string
		want   string
	}{
		{name: "given nothing then empty"},
		{name: "given a cookie then the cookie", cookie: "c1", want: "c1"},
		{name: "given a bearer header then the token", header: "Bearer t1", want: "t1"},
		{name: "given a lowercase scheme then the token", header: "bearer t1", want: "t1"},
		{name: "given both then the cookie wins", cookie: "c1", header: "Bearer t1", want: "c1"},
		{name: "given a basic header then empty", header: "Basic dXNlcjpwdw=="},
		{name: "given a bare scheme then empty", header: "Bearer"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: tc.cookie})
			}
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = req
			if got := sessionToken(c); got != tc.want {
				t.Fatalf("sessionToken = %q; want %q", got, tc.want)
			}
		})
	}
}
