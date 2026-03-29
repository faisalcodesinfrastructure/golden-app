# =============================================================================
# Dockerfile — Multi-stage build
# Location: golden-app/Dockerfile
#
# Stage 1 (builder): compiles the Go binary
# Stage 2 (runtime): copies only the binary into a minimal image
#
# Why multi-stage?
# The Go compiler and source code are only needed at build time.
# The final image only needs the compiled binary.
# This produces a much smaller image — typically 10-20MB vs 300-400MB
# for a single-stage build with the full Go toolchain.
# =============================================================================

# ─── Stage 1: Builder ────────────────────────────────────────────────────────
# golang:1.22-alpine is the Go toolchain on Alpine Linux.
# Alpine is a minimal Linux distribution — smaller than Ubuntu/Debian.
# We name this stage "builder" so we can reference it in stage 2.
FROM golang:1.22-alpine AS builder

# Install git — required by go mod download for some dependencies
RUN apk add --no-cache git

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum first — before copying source code.
# Docker caches each layer. If go.mod and go.sum have not changed,
# Docker reuses the cached layer for go mod download — much faster builds.
COPY go.mod go.sum ./

# Download all dependencies declared in go.mod.
# This runs as a separate layer so it is cached independently.
RUN go mod download

# Copy the rest of the source code
# This layer changes on every code change, but dependency download above
# is still cached as long as go.mod and go.sum are unchanged.
COPY . .

# Build the binary
# CGO_ENABLED=0 disables cgo — produces a statically linked binary
#   that does not need any C libraries. Essential for scratch/distroless images.
# GOOS=linux GOARCH=arm64 targets Apple Silicon Macs and AWS Graviton
#   Change to amd64 for Intel/AMD processors
# -ldflags="-w -s" strips debug symbols — reduces binary size by ~30%
#   -w: disable DWARF generation
#   -s: disable symbol table
# -o /app/server: output binary name and location
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
    go build \
    -ldflags="-w -s" \
    -o /app/server \
    ./cmd/server

# ─── Stage 2: Runtime ────────────────────────────────────────────────────────
# gcr.io/distroless/static-debian12 is a minimal base image with:
#   - No shell (no bash, sh)
#   - No package manager
#   - No extra utilities
#   - Only the CA certificates and timezone data needed for network calls
# This minimises the attack surface — there is almost nothing to exploit.
FROM gcr.io/distroless/static-debian12:nonroot

# Copy only the compiled binary from the builder stage.
# Everything else — Go toolchain, source code, go.mod — stays in stage 1
# and is not included in the final image.
COPY --from=builder /app/server /server

# nonroot: run as a non-root user (UID 65532)
# This aligns with our Kubernetes securityContext settings
USER nonroot:nonroot

# Document that the container listens on port 8080.
# This does not publish the port — it is documentation for humans and tools.
EXPOSE 8080

# The command to run when the container starts.
# ENTRYPOINT is preferred over CMD for executables — it cannot be
# accidentally overridden by docker run arguments.
ENTRYPOINT ["/server"]
