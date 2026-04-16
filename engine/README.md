# Kumquat Engine

[![Go Report Card](https://goreportcard.com/badge/github.com/fize/kumquat/engine)](https://goreportcard.com/report/github.com/fize/kumquat/engine)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

Kumquat Engine is the core system of the [Kumquat](../README.md) multi-cluster application management platform, responsible for application distribution, scheduling and management across multiple Kubernetes clusters.

## Architecture

Kumquat Engine adopts a Hub-Spoke architecture and includes the following components:

| Component | Description |
|-----------|-------------|
| **Manager** | Central control plane running on the Hub cluster, manages Application and Cluster CRDs |
| **Scheduler** | Multi-cluster scheduling engine based on plugin Filter/Score architecture |
| **Distributor** | Generates native K8s resources and distributes them to target clusters |
| **Tunnel Server** | Reverse tunnel based on WebSocket, used for Edge cluster connections |
| **Agent** | Runs on the Edge cluster, maintains tunnel connection and executes workloads |

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
# Install CRDs to cluster
kubectl apply -f config/crd/bases/

# Deploy Manager with Helm
helm install engine-manager ../charts/manager -n kumquat-system --create-namespace

# Deploy Agent
helm install engine-agent ../charts/agent -n kumquat-system \
  --set hub.url=<hub-url> \
  --set clusterName=<cluster-name>
```

## E2E Testing

```bash
# Run full E2E test suite with Kind
make e2e-kind

# Or step-by-step
make e2e-kind-create  # Create Kind cluster
make e2e-kind-test    # Run tests
make e2e-kind-delete  # Cleanup
```

## Documentation

See [../docs/](../docs/) for detailed documentation on architecture design, scheduler, API reference, etc.

## License

Apache License 2.0
