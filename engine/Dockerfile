# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY internal/ internal/

# Build manager
RUN CGO_ENABLED=0 GOOS=linux go build -a -o manager cmd/manager/manager.go

# Build agent
RUN CGO_ENABLED=0 GOOS=linux go build -a -o agent cmd/agent/agent.go

# Final image
FROM alpine:latest
WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/agent .

# Create home directory for non-root user so Helm libraries can write cache
RUN mkdir -p /home/nonroot/.cache/helm && chown -R 65532:65532 /home/nonroot
ENV HOME=/home/nonroot
ENV HELM_CACHE_HOME=/home/nonroot/.cache/helm
ENV XDG_CACHE_HOME=/home/nonroot/.cache

USER 65532:65532
# Default to manager, but can be overridden by command/args
ENTRYPOINT ["/manager"]
