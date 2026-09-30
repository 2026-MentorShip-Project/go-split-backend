package ruleassist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/genai"
)

// ErrNotConfigured means no model is set up. Callers answer 503 rather than
// failing the request, so an unconfigured deployment simply lacks the feature.
var ErrNotConfigured = errors.New("rule drafting is not configured")

// maxOutputTokens caps what one draft can cost; a plan for a sentence is a few
// hundred tokens.
const maxOutputTokens = 2048

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
	client *genai.Client
}

// NewVertexGenerator returns a generator, or ErrNotConfigured when the
// deployment has not chosen a model.
func NewVertexGenerator(ctx context.Context, c Config) (*VertexGenerator, error) {
	return newVertexGenerator(ctx, c, &genai.ClientConfig{})
}

func newVertexGenerator(ctx context.Context, c Config, cc *genai.ClientConfig) (*VertexGenerator, error) {
	if !c.Enabled() {
		return nil, ErrNotConfigured
	}
	cc.Backend = genai.BackendVertexAI
	cc.Project = c.Project
	cc.Location = c.Location
	client, err := genai.NewClient(ctx, cc)
	if err != nil {
		return nil, fmt.Errorf("vertex client: %w", err)
	}
	return &VertexGenerator{Config: c, client: client}, nil
}

// Draft implements Generator. Anything short of a complete JSON plan — a
// blocked prompt, a cut-off or refused answer, malformed output — is an error,
// never a partial plan.
func (g *VertexGenerator) Draft(ctx context.Context, in Input) (Plan, error) {
	msg, err := UserMessage(in)
	if err != nil {
		return Plan{}, err
	}
	resp, err := g.client.Models.GenerateContent(ctx, g.Config.Model, genai.Text(msg), &genai.GenerateContentConfig{
		SystemInstruction:  genai.NewContentFromText(SystemPrompt(), genai.RoleUser),
		ResponseMIMEType:   "application/json",
		ResponseJsonSchema: ResponseSchema(),
		Temperature:        genai.Ptr[float32](0),
		MaxOutputTokens:    maxOutputTokens,
	})
	if err != nil {
		return Plan{}, fmt.Errorf("generate: %w", err)
	}
	if fb := resp.PromptFeedback; fb != nil && fb.BlockReason != "" {
		return Plan{}, fmt.Errorf("prompt blocked: %s", fb.BlockReason)
	}
	if len(resp.Candidates) == 0 {
		return Plan{}, errors.New("no candidates")
	}
	if reason := resp.Candidates[0].FinishReason; reason != genai.FinishReasonStop {
		return Plan{}, fmt.Errorf("answer not finished: %s", reason)
	}
	var plan Plan
	if err := json.Unmarshal([]byte(resp.Text()), &plan); err != nil {
		return Plan{}, fmt.Errorf("decode plan: %w", err)
	}
	return plan, nil
}
