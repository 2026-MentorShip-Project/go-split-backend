package profiling

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"runtime/pprof"
	"strings"
	"time"

	"cloud.google.com/go/storage"
)

const (
	profileDuration = 60 * time.Second
	profileInterval = 10 * time.Minute
)

// Start enables periodic CPU profile uploads when PGO_GCS_BUCKET is set.
func Start(ctx context.Context) (func(), error) {
	bucket := os.Getenv("PGO_GCS_BUCKET")
	if bucket == "" {
		return func() {}, nil
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create profiling storage client: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(profileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				profile, err := captureCPUProfile(ctx, profileDuration)
				if err != nil {
					continue
				}
				if err := upload(ctx, client, bucket, profile); err != nil {
					log.Printf("CPU profile upload failed: %v", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return func() {
		cancel()
		<-done
		_ = client.Close()
	}, nil
}

func captureCPUProfile(ctx context.Context, duration time.Duration) ([]byte, error) {
	var profile bytes.Buffer
	if err := pprof.StartCPUProfile(&profile); err != nil {
		return nil, fmt.Errorf("start CPU profile: %w", err)
	}

	timer := time.NewTimer(duration)
	select {
	case <-timer.C:
	case <-ctx.Done():
		if !timer.Stop() {
			<-timer.C
		}
	}
	pprof.StopCPUProfile()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return profile.Bytes(), nil
}

func upload(ctx context.Context, client *storage.Client, bucket string, profile []byte) error {
	prefix := strings.Trim(os.Getenv("PGO_GCS_PREFIX"), "/")
	if prefix == "" {
		prefix = "pgo/cpu"
	}
	name := fmt.Sprintf("%s/%s-%d.pb.gz", prefix, os.Getenv("K_REVISION"), time.Now().UnixNano())
	w := client.Bucket(bucket).Object(name).NewWriter(ctx)
	w.ContentType = "application/octet-stream"
	w.Metadata = map[string]string{
		"service":     "go-split-backend",
		"environment": os.Getenv("APP_ENV"),
		"revision":    os.Getenv("K_REVISION"),
		"region":      os.Getenv("K_REGION"),
		"instance":    os.Getenv("HOSTNAME"),
	}
	if _, err := w.Write(profile); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}
