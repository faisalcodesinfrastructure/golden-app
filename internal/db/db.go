// =============================================================================
// Package db — PostgreSQL connection pool
// Location: golden-app/internal/db/db.go
//
// This file handles everything related to connecting to PostgreSQL:
//   - Reading the connection string from an environment variable
//   - Creating and configuring a connection pool
//   - Exposing a health check function
//
// The rest of the app imports this package to get a database connection.
// No other package knows how the connection is established — that detail
// lives here only. This is the separation of concerns principle.
// =============================================================================

package db

import (
	"context"
	"fmt"
	"os"
	"time"

	// pgxpool manages a pool of PostgreSQL connections.
	// A pool keeps multiple connections open so requests do not wait
	// for a connection to be established on every query.
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is the global connection pool used by all handlers.
// pgxpool.Pool is safe for concurrent use — multiple goroutines (requests)
// can use it simultaneously without any locking on our side.
var DB *pgxpool.Pool

// Connect reads the DATABASE_URL environment variable, creates a connection
// pool, and verifies the connection with a ping.
//
// DATABASE_URL format:
//   postgres://username:password@host:port/dbname?sslmode=disable
//
// This value comes from the Kubernetes Secret we created in Phase 1,
// injected as an environment variable by the Deployment manifest.
func Connect(ctx context.Context) error {
	// os.Getenv reads an environment variable.
	// In Kubernetes the Deployment injects this from the Secret.
	// For local development you set it manually before running.
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL environment variable is not set")
	}

	// pgxpool.ParseConfig parses the connection string into a config struct.
	// We use this instead of passing the URL directly so we can tune
	// pool settings before connecting.
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return fmt.Errorf("parsing database URL: %w", err)
	}

	// Pool configuration — tune connection behaviour
	// MaxConns: maximum number of connections in the pool
	// In production this is tuned based on PostgreSQL's max_connections setting
	config.MaxConns = 10

	// MinConns: keep this many connections open even when idle
	// Avoids latency spike on first request after idle period
	config.MinConns = 2

	// MaxConnLifetime: close and replace connections older than this
	// Prevents issues with stale connections
	config.MaxConnLifetime = 1 * time.Hour

	// MaxConnIdleTime: close idle connections after this duration
	config.MaxConnIdleTime = 30 * time.Minute

	// pgxpool.NewWithConfig creates the pool and opens initial connections
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("creating connection pool: %w", err)
	}

	// Ping verifies the connection is actually working.
	// This catches misconfigured DATABASE_URL at startup rather than
	// on the first real request.
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("pinging database: %w", err)
	}

	// Assign to the package-level variable so handlers can use it
	DB = pool
	return nil
}

// Close shuts down the connection pool gracefully.
// Called when the server receives a shutdown signal (SIGTERM, SIGINT).
// This ensures all in-flight queries complete before the process exits.
func Close() {
	if DB != nil {
		DB.Close()
	}
}

// HealthCheck runs a lightweight query to verify the database is reachable.
// Called by the GET /health endpoint to include database status in the response.
func HealthCheck(ctx context.Context) error {
	if DB == nil {
		return fmt.Errorf("database not connected")
	}
	// SELECT 1 is the lightest possible query — no table scan, no data
	// It just verifies the connection is alive and the server is responding
	_, err := DB.Exec(ctx, "SELECT 1")
	return err
}
