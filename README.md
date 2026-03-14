# golden-app

Sample application for the Platform Engineering Golden Path workshop.

This repo is what a developer receives from the Backstage Scaffolder template.
It contains a Go airline booking API, PostgreSQL database, Helm chart,
and GitHub Actions CI pipeline.

---

## Structure (built across phases)

```
golden-app/
├── cmd/server/main.go      Phase 2 — Go HTTP server
├── internal/db/            Phase 2 — PostgreSQL queries
├── internal/handlers/      Phase 2 — HTTP handlers
├── Dockerfile              Phase 2 — multi-stage container build
├── go.mod / go.sum         Phase 2 — Go module
├── helm/golden-app/        Phase 2 — Helm chart
│   ├── Chart.yaml
│   ├── values.yaml         image.tag updated by CI on every push
│   └── templates/
└── .github/workflows/
    └── ci.yaml             Phase 4 — GitHub Actions CI
```

---

## API (Phase 2)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | `{"status":"ok","db":"ok"}` |
| GET | `/flights` | List all flights |
| POST | `/flights` | Create a flight |
| GET | `/flights/{id}` | Get one flight |
| GET | `/flights/{id}/seats` | List available seats |
| POST | `/bookings` | Book a seat |
| GET | `/bookings/{id}` | Get a booking |

---

## Phase instructions

Phase 2 instructions are added to this README in Phase 2.
Phase 4 CI instructions are in `.github/README.md` (added in Phase 4).

---

## What is next

Complete Phase 0 and Phase 1 first.
Phase 2 instructions: return here after Phase 1 is complete.
