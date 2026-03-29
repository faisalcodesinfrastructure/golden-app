# golden-app — Airline Booking API

Platform Engineering Golden Path — Phase 2

---

## What this is

A Go airline booking API backed by PostgreSQL. This is the application
a developer receives when they use the Backstage Scaffolder template.

## API endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness check — `{"status":"ok","db":"ok"}` |
| GET | `/flights` | List all flights |
| POST | `/flights` | Create a flight |
| GET | `/flights/{id}` | Get one flight |
| GET | `/flights/{id}/seats` | List seats (add `?available=true` to filter) |
| POST | `/bookings` | Book a seat |
| GET | `/bookings/{ref}` | Get a booking by reference |

## Local development

```bash
# Prerequisites: PostgreSQL running locally
export DATABASE_URL="postgres://app:dev-password-change-in-production@localhost:5432/airlinedb?sslmode=disable"

# Run
go run ./cmd/server

# Test
curl http://localhost:8080/health
curl http://localhost:8080/flights
```

## Deployment

Push to main → GitHub Actions builds image → updates values.yaml → Flux deploys.

Access via Traefik: `http://golden-app.127.0.0.1.nip.io:8080/flights`
