package database

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context) (*pgxpool.Pool, error) {
	port, err := strconv.ParseUint(env("DB_PORT", "5432"), 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid DB_PORT: %w", err)
	}

	config, err := pgxpool.ParseConfig("")
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}
	config.ConnConfig.Port = uint16(port)
	for name, target := range map[string]*string{
		"DB_HOST":     &config.ConnConfig.Host,
		"DB_USER":     &config.ConnConfig.User,
		"DB_PASSWORD": &config.ConnConfig.Password,
		"DB_NAME":     &config.ConnConfig.Database,
	} {
		value := os.Getenv(name)
		if value == "" {
			return nil, fmt.Errorf("%s is required", name)
		}
		*target = value
	}
	if raw := os.Getenv("DB_MAX_CONNS"); raw != "" {
		maxConns, err := parseMaxConns(raw)
		if err != nil {
			return nil, err
		}
		config.MaxConns = maxConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// parseMaxConns reads DB_MAX_CONNS. Unset keeps pgx's default of
// max(4, runtime.NumCPU()), which counts host CPUs rather than Cloud Run's
// vCPU limit, so production sets it explicitly to bound total connections.
func parseMaxConns(raw string) (int32, error) {
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid DB_MAX_CONNS %q: want a positive integer", raw)
	}
	return int32(n), nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
