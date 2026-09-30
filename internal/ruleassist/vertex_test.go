package ruleassist

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/auth"
	"google.golang.org/genai"
)

type staticToken struct{}

func (staticToken) Token(context.Context) (*auth.Token, error) {
	return &auth.Token{Value: "test", Expiry: time.Now().Add(time.Hour)}, nil
}

func fakeCredentials() *auth.Credentials {
	return auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: staticToken{}})
}

// fakeVertex answers every generateContent call with body and records the
// request it received.
func fakeVertex(t *testing.T, status int, body string) (*VertexGenerator, *map[string]any, *string) {
	t.Helper()
	var sent map[string]any
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &sent)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	g, err := newVertexGenerator(t.Context(), Config{Project: "p", Location: "global", Model: "gemini-test"},
		&genai.ClientConfig{Credentials: fakeCredentials(), HTTPOptions: genai.HTTPOptions{BaseURL: srv.URL}})
	if err != nil {
		t.Fatal(err)
	}
	return g, &sent, &path
}

func answer(text, finish string) string {
	raw, _ := json.Marshal(map[string]any{"candidates": []any{map[string]any{
		"content":      map[string]any{"role": "model", "parts": []any{map[string]any{"text": text}}},
		"finishReason": finish,
	}}})
	return string(raw)
}

func TestVertexDraftSendsPromptSchemaAndDecodesPlan(t *testing.T) {
	g, sent, path := fakeVertex(t, 200, answer(`{"new_item_tags":[],"new_cond_tags":["吃素"],"rules":[],"member_conds":[],"note":"ok"}`, "STOP"))

	plan, err := g.Draft(t.Context(), Input{Text: "吃素的不用分肉錢", ItemTags: []string{"肉品"}})

	if err != nil {
		t.Fatal(err)
	}
	if plan.Note != "ok" || len(plan.NewCondTags) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	if !strings.HasSuffix(*path, "/models/gemini-test:generateContent") {
		t.Errorf("path = %s", *path)
	}
	body, _ := json.Marshal(*sent)
	for _, want := range []string{`"responseJsonSchema"`, `"responseMimeType":"application/json"`, `"temperature":0`, `"maxOutputTokens":2048`, "Go-Split", "吃素的不用分肉錢"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("request lacks %s: %s", want, body)
		}
	}
}

func TestVertexDraftRefusesAnythingButACompletePlan(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"blocked prompt", 200, `{"promptFeedback":{"blockReason":"SAFETY"}}`, "prompt blocked"},
		{"no candidates", 200, `{"candidates":[]}`, "no candidates"},
		{"cut off", 200, answer(`{"rules":[`, "MAX_TOKENS"), "not finished"},
		{"refused for safety", 200, answer("", "SAFETY"), "not finished"},
		{"not json", 200, answer("sorry, I can't", "STOP"), "decode plan"},
		{"upstream error", 500, `{"error":{"code":500,"message":"boom","status":"INTERNAL"}}`, "generate"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, _, _ := fakeVertex(t, c.status, c.body)
			plan, err := g.Draft(t.Context(), Input{Text: "x"})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
			if plan.Rules != nil || plan.Note != "" {
				t.Fatalf("partial plan leaked: %+v", plan)
			}
		})
	}
}
