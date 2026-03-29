module github.com/platform-eng/golden-app

go 1.22

require (

	// chi is a lightweight HTTP router for Go.
	// It is compatible with net/http and adds URL parameters and middleware.
	// We use it instead of raw net/http for cleaner route definitions.
	github.com/go-chi/chi/v5 v5.0.12
	// pgx is the most popular PostgreSQL driver for Go.
	// It is faster than database/sql and supports PostgreSQL-specific features.
	// pgx/v5 is the current major version.
	github.com/jackc/pgx/v5 v5.5.4
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	golang.org/x/crypto v0.17.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/text v0.14.0 // indirect
)
