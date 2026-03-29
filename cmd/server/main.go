// =============================================================================
// Golden App — Airline Booking API
// Location: golden-app/cmd/server/main.go
//
// This is the entry point of the application. It:
//   1. Connects to PostgreSQL
//   2. Creates the schema and seeds initial data
//   3. Sets up the HTTP router and registers all routes
//   4. Starts the HTTP server
//   5. Handles graceful shutdown on SIGTERM/SIGINT
//
// Why cmd/server/main.go and not just main.go?
// The cmd/ convention is standard Go project layout. A project can have
// multiple binaries under cmd/ (e.g. cmd/server, cmd/migrate, cmd/cli).
// Each has its own main.go. Internal packages live in internal/.
// =============================================================================

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// chi is a lightweight HTTP router compatible with net/http.
	// It adds URL parameters ({id}), middleware, and route grouping
	// without replacing the standard library.
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	// Our internal packages
	"github.com/platform-eng/golden-app/internal/db"
	"github.com/platform-eng/golden-app/internal/handlers"
)

func main() {
	// -------------------------------------------------------------------------
	// Logger setup
	// -------------------------------------------------------------------------
	// log.SetFlags controls the prefix on each log line.
	// Ldate | Ltime | LUTC gives: "2026/03/29 12:00:00 UTC message"
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)
	log.Println("Starting Golden App — Airline Booking API")

	// -------------------------------------------------------------------------
	// Database connection
	// -------------------------------------------------------------------------
	// We give the database connection 30 seconds to succeed.
	// In Kubernetes, the database pod may still be starting when this pod
	// starts — the 30 second window gives PostgreSQL time to become ready.
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer dbCancel()

	log.Println("Connecting to PostgreSQL...")
	if err := db.Connect(dbCtx); err != nil {
		// log.Fatalf prints the message and calls os.Exit(1)
		// This causes Kubernetes to restart the pod (CrashLoopBackOff)
		// which is the correct behaviour — we cannot serve traffic without a DB
		log.Fatalf("Failed to connect to database: %v", err)
	}
	log.Println("Connected to PostgreSQL")

	// -------------------------------------------------------------------------
	// Schema creation and seed data
	// -------------------------------------------------------------------------
	log.Println("Creating schema...")
	if err := db.CreateSchema(context.Background()); err != nil {
		log.Fatalf("Failed to create schema: %v", err)
	}

	log.Println("Seeding data...")
	if err := db.SeedData(context.Background()); err != nil {
		log.Fatalf("Failed to seed data: %v", err)
	}
	log.Println("Database ready")

	// -------------------------------------------------------------------------
	// Router setup
	// -------------------------------------------------------------------------
	// chi.NewRouter() creates a new router.
	// Middleware wraps every request with additional behaviour.
	r := chi.NewRouter()

	// middleware.Logger logs each request: method, path, status, duration
	// Example: "GET /flights 200 1.2ms"
	r.Use(middleware.Logger)

	// middleware.Recoverer catches panics and returns 500 instead of crashing
	// Essential in production — a single handler panic should not kill the server
	r.Use(middleware.Recoverer)

	// middleware.RequestID adds a unique X-Request-Id header to each request
	// Useful for tracing a request through logs
	r.Use(middleware.RequestID)

	// middleware.RealIP reads the X-Forwarded-For header set by Traefik
	// so the logged IP is the client's real IP, not Traefik's IP
	r.Use(middleware.RealIP)

	// -------------------------------------------------------------------------
	// Routes
	// -------------------------------------------------------------------------
	// GET /health — liveness and readiness check
	// Kubernetes calls this to know if the pod is healthy.
	// Returns 200 if both the server and database are working.
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		status := map[string]string{
			"status": "ok",
			"db":     "ok",
		}

		// Check database connectivity
		if err := db.HealthCheck(r.Context()); err != nil {
			status["db"] = "error: " + err.Error()
			status["status"] = "degraded"
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","db":"ok"}`)
	})

	// Flight routes
	r.Get("/flights", handlers.ListFlights)
	r.Post("/flights", handlers.CreateFlight)
	r.Get("/flights/{id}", handlers.GetFlight)
	r.Get("/flights/{id}/seats", handlers.ListSeats)

	// Booking routes
	r.Post("/bookings", handlers.CreateBooking)
	r.Get("/bookings/{ref}", handlers.GetBooking)

	// -------------------------------------------------------------------------
	// HTTP server
	// -------------------------------------------------------------------------
	// Read port from environment — defaults to 8080.
	// Kubernetes injects PORT if needed, otherwise we use the default.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// http.Server with explicit timeouts.
	// Never use http.ListenAndServe directly — it has no timeouts
	// which leaves you vulnerable to slow-client attacks.
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,

		// ReadTimeout: max time to read the full request including body
		ReadTimeout: 10 * time.Second,

		// WriteTimeout: max time to write the response
		WriteTimeout: 30 * time.Second,

		// IdleTimeout: max time to wait for the next request on a keep-alive conn
		IdleTimeout: 120 * time.Second,
	}

	// -------------------------------------------------------------------------
	// Graceful shutdown
	// -------------------------------------------------------------------------
	// We start the server in a goroutine so we can listen for shutdown signals
	// on the main goroutine at the same time.
	//
	// A goroutine is a lightweight concurrent function. "go func()" starts
	// the function in a separate goroutine — it runs alongside main().
	go func() {
		log.Printf("Server listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// signal.Notify registers a channel to receive OS signals.
	// SIGTERM is sent by Kubernetes when it wants to stop the pod.
	// SIGINT is sent when you press Ctrl+C locally.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	// Block until we receive a signal
	sig := <-quit
	log.Printf("Received signal %s — shutting down gracefully", sig)

	// Give in-flight requests 30 seconds to complete
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// srv.Shutdown stops accepting new connections and waits for
	// existing requests to complete before returning
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	// Close the database pool
	db.Close()
	log.Println("Server stopped cleanly")
}
