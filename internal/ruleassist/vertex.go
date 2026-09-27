package ruleassist

import (
	"context"
	"errors"
	"os"
	"strings"
)

// ErrNotConfigured means no model is set up. Callers answer 503 rather than
// failing the request, so an unconfigured deployment simply lacks the feature.
var ErrNotConfigured = errors.New("rule drafting is not configured")

// ErrNotImplemented marks the Vertex request path as still to be written. The
// configuration, gating and validation around it are complete.
var ErrNotImplemented = errors.New("vertex rule drafting is not wired yet")

// Config is what the deployment supplies. It carries no credentials: on Cloud
// Run the service account is the identity, and locally Application Default
// Credentials stand in.
type Config struct {
	Project  string
	Location string
	Model    string
}

// ConfigFromEnv reads VERTEX_PROJECT, VERTEX_LOCATION and VERTEX_MODEL, which
// Terraform sets on the Cloud Run service.
func ConfigFromEnv() Config {
	return Config{
		Project:  strings.TrimSpace(os.Getenv("VERTEX_PROJECT")),
		Location: strings.TrimSpace(os.Getenv("VERTEX_LOCATION")),
		Model:    strings.TrimSpace(os.Getenv("VERTEX_MODEL")),
	}
}

// Enabled reports whether every value a request needs is present. Leaving any
// of them empty keeps the feature off, which is the default.
func (c Config) Enabled() bool {
	return c.Project != "" && c.Location != "" && c.Model != ""
}

// VertexGenerator drafts through Vertex AI.
type VertexGenerator struct {
	Config Config
}

// NewVertexGenerator returns a generator, or ErrNotConfigured when the
// deployment has not chosen a model.
func NewVertexGenerator(c Config) (*VertexGenerator, error) {
	if !c.Enabled() {
		return nil, ErrNotConfigured
	}
	return &VertexGenerator{Config: c}, nil
}

// Draft implements Generator.
//
// The request and response handling is deliberately absent: the model id and
// its region are a deployment decision, and the surrounding contract —
// SystemPrompt, UserMessage, ResponseSchema and Check — is what the rest of the
// feature is built against. Swap FixtureGenerator for this once a model is
// chosen; nothing else changes.
func (g *VertexGenerator) Draft(_ context.Context, _ Input) (Plan, error) {
	return Plan{}, ErrNotImplemented
}
