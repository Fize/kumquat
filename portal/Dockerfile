# Portal Dockerfile
FROM golang:1.24-bullseye AS builder

# Install build dependencies for SQLite (requires CGO)
RUN apt-get update && apt-get install -y gcc libsqlite3-dev && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

# Copy engine module first (required for replace directive)
COPY engine/go.mod engine/go.sum ./engine/
COPY engine/pkg/ ./engine/pkg/
COPY engine/cmd/ ./engine/cmd/
COPY engine/internal/ ./engine/internal/
COPY engine/config/ ./engine/config/

# Copy portal files and fix the replace path
COPY portal/go.mod portal/go.sum ./

# Fix replace directive path from ../engine to ./engine
RUN sed -i 's|replace github.com/fize/kumquat/engine => ../engine|replace github.com/fize/kumquat/engine => ./engine|' go.mod

RUN GOTOOLCHAIN=auto go mod download

COPY portal/cmd/ ./cmd/
COPY portal/pkg/ ./pkg/
COPY portal/config/ ./config/
COPY portal/docs/ ./docs/

# Build with CGO enabled for SQLite support (use host arch for CGO compatibility)
RUN GOTOOLCHAIN=auto CGO_ENABLED=1 GOOS=linux go build -a -o portal cmd/server/main.go

FROM debian:bullseye-slim
# Install SQLite runtime libraries
RUN apt-get update && apt-get install -y ca-certificates libsqlite3-0 && rm -rf /var/lib/apt/lists/*
# Create non-root user and group for running the application
RUN addgroup --system --gid 65532 nonroot && adduser --system --uid 65532 --gid 65532 nonroot
WORKDIR /app
COPY --from=builder /workspace/portal .
COPY --from=builder /workspace/config/ ./config/
# Ensure /data volume is writable by the non-root user
RUN mkdir -p /data && chown 65532:65532 /data
# Run as non-root user
USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/app/portal"]
