package profiling

import (
	"context"
	"testing"
	"time"
)

func TestCaptureCPUProfileProducesGzipProfile(t *testing.T) {
	profile, err := captureCPUProfile(context.Background(), 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(profile) < 2 || profile[0] != 0x1f || profile[1] != 0x8b {
		t.Fatalf("expected gzip profile, got %d bytes", len(profile))
	}
}
