package main

import (
	"testing"

	"github.com/grafana/pyroscope-go"
)

func TestProfilerConfigDisabledWithoutServerAddress(t *testing.T) {
	if _, ok := profilerConfig(func(string) string { return "" }); ok {
		t.Fatal("expected profiler to be disabled without a server address")
	}
}

func TestProfilerConfigUsesEnvironment(t *testing.T) {
	values := map[string]string{
		"PYROSCOPE_SERVER_ADDRESS":   "https://profiles.example",
		"PYROSCOPE_APPLICATION_NAME": "api",
		"PYROSCOPE_USERNAME":         "user",
		"PYROSCOPE_PASSWORD":         "pass",
	}
	cfg, ok := profilerConfig(func(key string) string { return values[key] })
	if !ok {
		t.Fatal("expected profiler to be enabled")
	}
	if cfg.ApplicationName != "api" || cfg.ServerAddress != "https://profiles.example" || cfg.BasicAuthUser != "user" || cfg.BasicAuthPassword != "pass" {
		t.Fatalf("unexpected profiler config: %+v", cfg)
	}
	if len(cfg.ProfileTypes) != 1 || cfg.ProfileTypes[0] != pyroscope.ProfileCPU {
		t.Fatalf("expected CPU-only profiling, got %v", cfg.ProfileTypes)
	}
}
