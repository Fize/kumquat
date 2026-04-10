# Kumquat

[中文](README_zh.md)

Kumquat is a cloud-native multi-cluster application management platform designed to simplify application distribution, scheduling, and management across multiple Kubernetes clusters.

## Features

- **Multi-Cluster Management**: Manage dozens of Kubernetes clusters from a single control plane
- **Unified Application Distribution**: Write once, deploy everywhere with standard K8s workloads
- **Intelligent Scheduling**: Advanced placement engine with Spread, BinPacking, and Affinity support
- **Dual Connection Mode**: Support both Hub (pull) and Edge (push) cluster connectivity
- **Policy-Based Overrides**: Customize configurations per cluster without duplicating YAMLs
- **Global Status Aggregation**: Real-time visibility into application health across all clusters
- **Extensible Addon System**: Plugin architecture for MCS, monitoring, and custom extensions
  - Built-in **Submariner Addon**: Cross-cluster service discovery and networking
  - Multiple network modes: IPsec tunnel, WireGuard, VXLAN, flat network
  - Automated ServiceExport/ServiceImport management

## Architecture

Kumquat adopts a Hub-Spoke architecture to manage multi-cluster environments efficiently.

![Architecture](docs/images/architecture.drawio.png)

### Components

| Component | Sub-project | Description |
|-----------|-------------|-------------|
| **Manager** | Engine | Central control plane running on Hub cluster |
| **Scheduler** | Engine | Multi-cluster placement engine with plugin-based Filter/Score architecture |
| **Agent** | Engine | Runs on Edge clusters, maintains tunnel connection |
| **Tunnel Server** | Engine | WebSocket-based reverse tunnel for Edge cluster connectivity |
| **Portal** | Portal | User management, authentication and authorization API |
| **Kumctl** | Kumctl | Command-line tool for cluster management |

### Connection Modes

| Mode | Direction | Use Case |
|------|-----------|----------|
| **Hub** | Manager → Cluster | Clusters accessible from Hub (same VPC, VPN) |
| **Edge** | Agent → Manager | Clusters behind NAT/firewall, no inbound access |

## Sub-projects

| Directory | Name | Description |
|-----------|------|-------------|
| [engine/](engine/) | Engine | Core system: multi-cluster application distribution, scheduling and management |
| [armory/](armory/) | Armory | Base Docker image builds (Alpine, Go, Node) |
| [portal/](portal/) | Portal | Application-layer API: user management, authentication (TBD) |
| [kumctl/](kumctl/) | Kumctl | Command-line tool (TBD) |

## Quick Start

### Prerequisites

- Go 1.22+
- Docker
- Kind (for local testing)
- kubectl

### Installation

```bash
# Clone the repository
git clone https://github.com/fize/kumquat.git
cd kumquat

# Build engine binaries
make -C engine build

# Build base images
make -C armory all
```

### Deploy Manager

```bash
# Install CRDs to your cluster
kubectl apply -f engine/config/crd/bases/

# Using Helm
helm install engine-manager charts/manager -n kumquat-system --create-namespace
```

### Register a Cluster (Hub Mode)

```yaml
apiVersion: storage.kumquat.io/v1alpha1
kind: ManagedCluster
metadata:
  name: production-east
  labels:
    env: production
    region: us-east
spec:
  connectionMode: Hub
  apiServer: https://prod-east.example.com:6443
  secretRef:
    name: prod-east-credentials
```

### Deploy an Application

```yaml
apiVersion: apps.kumquat.io/v1alpha1
kind: Application
metadata:
  name: nginx-app
  namespace: default
spec:
  replicas: 6
  workload:
    apiVersion: apps/v1
    kind: Deployment
    template:
      metadata:
        labels:
          app: nginx
      spec:
        containers:
        - name: nginx
          image: nginx:1.25
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
  clusterAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
      - matchExpressions:
        - key: env
          operator: In
          values: ["production"]
```

## Built-in Addons

### Submariner - Cross-Cluster Service Discovery

Kumquat includes a built-in **Submariner Addon** (mcs-lighthouse) for cross-cluster service discovery and networking.

```yaml
apiVersion: storage.kumquat.io/v1alpha1
kind: ManagedCluster
metadata:
  name: cluster-1
spec:
  connectionMode: Hub
  apiServer: https://cluster-1.example.com:6443
  addons:
    - name: mcs-lighthouse
      enabled: true
```

Network modes: IPsec Tunnel (default), Flat Network, VXLAN.

> **Important**: Flat network mode requires users to configure underlying network routing. See [Addon Design](docs/en/addon.md) for details.

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture](docs/en/architecture.md) | System architecture and design |
| [Scheduler Design](docs/en/scheduler.md) | Multi-cluster scheduling framework |
| [Topology Spread](docs/en/topology_spread.md) | Cross-zone/region workload distribution |
| [Edge Cluster](docs/en/edge.md) | Tunnel-based Edge cluster management |
| [API Reference](docs/en/api.md) | CRD specifications and examples |
| [Addon Design](docs/en/addon.md) | Plugin architecture and extensions |

## Testing

```bash
# Unit tests
make -C engine test

# E2E tests with Kind
make -C engine e2e-kind
```

## License

Apache License 2.0
