# Kumquat Engine

[中文](README_zh.md)

[![Go Report Card](https://goreportcard.com/badge/github.com/fize/kumquat/engine)](https://goreportcard.com/report/github.com/fize/kumquat/engine)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

Kumquat Engine is the core system of the [Kumquat](../README.md) multi-cluster application management platform, responsible for application distribution, scheduling, and management across multiple Kubernetes clusters.

## Architecture

Kumquat Engine adopts a Hub-Spoke architecture with the following components:

| Component | Description |
|-----------|-------------|
| **Manager** | Central control plane running on Hub cluster. Manages Application and Cluster CRDs. |
| **Scheduler** | Multi-cluster placement engine with plugin-based Filter/Score architecture. |
| **Dispatcher** | Generates and distributes native K8s resources to target clusters. |
| **Tunnel Server** | WebSocket-based reverse tunnel for Edge cluster connectivity. |
| **Agent** | Runs on Edge clusters, maintains tunnel connection and executes workloads. |

## Build

```bash
# Build binaries (manager and agent)
make build

# Run unit tests
make test

# Build Docker images
make docker-build
```

## Deploy

```bash
# Install CRDs to your cluster
kubectl apply -f config/crd/bases/

# Deploy Manager using Helm
helm install engine-manager ../charts/manager -n kumquat-system --create-namespace

# Deploy Agent
helm install engine-agent ../charts/agent -n kumquat-system \
  --set hub.url=<hub-url> \
  --set clusterName=<cluster-name>
```

## E2E Tests

```bash
# Full E2E suite with Kind
make e2e-kind

# Or step by step
make e2e-kind-create  # Create Kind cluster
make e2e-kind-test    # Run tests
make e2e-kind-delete  # Cleanup
```

## Documentation

See [../docs/en/](../docs/en/) for detailed documentation including architecture, scheduler design, API reference, and addon design.

## License

Apache License 2.0
