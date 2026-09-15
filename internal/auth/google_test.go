package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

const testClientID = "client-123.apps.googleusercontent.com"

func tokenInfoServer(t *testing.T, status int, info tokenInfo) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id_token") == "" {
			t.Error("tokeninfo called without id_token")
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(info)
	}))
	t.Cleanup(s.Close)
	return s
}

func validTokenInfo() tokenInfo {
	return tokenInfo{
		Iss:           "https://accounts.google.com",
		Aud:           testClientID,
		Sub:           "1069",
		Email:         "host@example.com",
		EmailVerified: "true",
		Name:          "Host Person",
		Exp:           strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
	}
}

func TestGoogleVerifierAcceptsValidToken(t *testing.T) {
	s := tokenInfoServer(t, http.StatusOK, validTokenInfo())
	v := newGoogleVerifier(testClientID, s.URL)

	got, err := v.Verify(t.Context(), "token")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	want := GoogleIdentity{Sub: "1069", Email: "host@example.com", Name: "Host Person"}
	if got != want {
		t.Fatalf("Verify = %+v; want %+v", got, want)
	}
}

func TestGoogleVerifierRejectsBadClaims(t *testing.T) {
	cases := []struct {
		name   string
		status int
		mutate func(*tokenInfo)
	}{
		{"google rejects token", http.StatusBadRequest, func(i *tokenInfo) { *i = tokenInfo{Error: "Invalid Value"} }},
		{"audience mismatch", http.StatusOK, func(i *tokenInfo) { i.Aud = "other-client" }},
		{"unexpected issuer", http.StatusOK, func(i *tokenInfo) { i.Iss = "https://evil.example" }},
		{"expired", http.StatusOK, func(i *tokenInfo) { i.Exp = strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10) }},
		{"unparseable exp", http.StatusOK, func(i *tokenInfo) { i.Exp = "soon" }},
		{"email not verified", http.StatusOK, func(i *tokenInfo) { i.EmailVerified = "false" }},
		{"missing sub", http.StatusOK, func(i *tokenInfo) { i.Sub = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := validTokenInfo()
			tc.mutate(&info)
			v := newGoogleVerifier(testClientID, tokenInfoServer(t, tc.status, info).URL)

			_, err := v.Verify(t.Context(), "token")
			if !errors.Is(err, ErrInvalidGoogleToken) {
				t.Fatalf("Verify error = %v; want ErrInvalidGoogleToken", err)
			}
		})
	}
}

func TestGoogleVerifierReportsUpstreamFailure(t *testing.T) {
	v := newGoogleVerifier(testClientID, tokenInfoServer(t, http.StatusInternalServerError, tokenInfo{}).URL)

	_, err := v.Verify(t.Context(), "token")
	if err == nil || errors.Is(err, ErrInvalidGoogleToken) {
		t.Fatalf("Verify error = %v; want a non-token upstream error", err)
	}
}

func TestNewGoogleVerifierDisabledWithoutClientID(t *testing.T) {
	if NewGoogleVerifier("") != nil {
		t.Fatal("NewGoogleVerifier(\"\") must return nil so the endpoint is disabled")
	}
}

func TestGoogleVerifierPreservesNonJSONUpstreamStatus(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
	}))
	defer s.Close()
	v := newGoogleVerifier(testClientID, s.URL)
	_, err := v.Verify(t.Context(), "token")
	if err == nil || !strings.Contains(err.Error(), "503") || errors.Is(err, ErrInvalidGoogleToken) {
		t.Fatalf("Verify error = %v; want upstream status 503", err)
	}
}

func TestGoogleVerifierTransportErrorOmitsToken(t *testing.T) {
	v := newGoogleVerifier(testClientID, "https://example.invalid")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	const token = "secret-google-id-token"
	_, err := v.Verify(ctx, token)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Verify error = %v; want context cancellation", err)
	}
	if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "id_token=") {
		t.Fatal("transport error exposes ID token")
	}
}

type stubVerifier struct {
	err error
}

func (s stubVerifier) Verify(context.Context, string) (GoogleIdentity, error) {
	return GoogleIdentity{}, s.err
}

func postGoogle(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.Register(r)
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/google", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestPostGoogleRejectsBeforeTouchingDatabase(t *testing.T) {
	cases := []struct {
		name    string
		handler *Handler
		body    string
		status  int
	}{
		{"not configured", &Handler{}, `{"id_token":"t"}`, http.StatusServiceUnavailable},
		{"missing token", &Handler{Google: stubVerifier{}}, `{}`, http.StatusBadRequest},
		{"invalid token", &Handler{Google: stubVerifier{err: ErrInvalidGoogleToken}}, `{"id_token":"t"}`, http.StatusUnauthorized},
		{"google unreachable", &Handler{Google: stubVerifier{err: errors.New("dial tcp")}}, `{"id_token":"t"}`, http.StatusBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := postGoogle(t, tc.handler, tc.body)
			if w.Code != tc.status {
				t.Fatalf("POST /auth/google = %d %s; want %d", w.Code, w.Body, tc.status)
			}
		})
	}
}
