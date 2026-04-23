# Kumquat Demo Local Environment

One-click deployment of a complete Kumquat multi-cluster demo environment locally, based on kind with 3 interconnected clusters.

## Architecture Overview

```
┌───────────────────────────────────────────────────────────────────┐
│                      Docker Network: kumquat-net                  │
│                                                                    │
│  ┌───────────────────────┐  ┌───────────────────────┐            │
│  │   kumquat-hub         │  │   kumquat-sub-1        │            │
│  │                       │  │                        │            │
│  │  ┌─────────────────┐  │  │  ┌──────────────────┐ │            │
│  │  │ engine-manager  │◄═╪──╼──┤ engine-agent     │ │            │
│  │  │ (Scheduler,     │  │  │  │ (Edge Tunnel)    │ │            │
│  │  │  TunnelServer,  │  │  │  └──────────────────┘ │            │
│  │  │  APIServer)     │  │  │  ┌──────────────────┐ │            │
│  │  └─────────────────┘  │  │  │ kruise-rollout   │ │            │
│  │  ┌─────────────────┐  │  │  └──────────────────┘ │            │
│  │  │ engine-scheduler│  │  │  ┌──────────────────┐ │            │
│  │  └─────────────────┘  │  │  │ vmagent          │ │            │
│  │  ┌─────────────────┐  │  │  └──────────────────┘ │            │
│  │  │ portal          │  │  │  ┌──────────────────┐ │            │
│  │  │ (Gin + SQLite)  │  │  │  │ submariner       │ │            │
│  │  └─────────────────┘  │  │  └──────────────────┘ │            │
│  │  ┌─────────────────┐  │  └───────────────────────┘            │
│  │  │ kruise-rollout  │  │                                       │
│  │  └─────────────────┘  │  ┌───────────────────────┐            │
│  │  ┌─────────────────┐  │  │   kumquat-sub-2        │            │
│  │  │ VM Single       │  │  │                        │            │
│  │  └─────────────────┘  │  │  ┌──────────────────┐ │            │
│  │  ┌─────────────────┐  │  │  │ engine-agent     │ │            │
│  │  │ submariner      │  │  │  └──────────────────┘ │            │
│  │  │ broker          │  │  │  ┌──────────────────┐ │            │
│  │  └─────────────────┘  │  │  │ kruise-rollout   │ │            │
│  └───────────────────────┘  │  └──────────────────┘ │            │
│                              │  ┌──────────────────┐ │            │
│                              │  │ vmagent          │ │            │
│                              │  └──────────────────┘ │            │
│                              │  ┌──────────────────┐ │            │
│                              │  │ submariner       │ │            │
│                              │  └──────────────────┘ │            │
│                              └───────────────────────┘            │
│                                                                    │
│     ◄─── Submariner VXLAN Cross-Cluster Network ───►              │
└───────────────────────────────────────────────────────────────────┘
```

## Prerequisites

| Tool     | Version   | Description              |
|----------|-----------|--------------------------|
| Docker   | >= 20.0   | Container runtime        |
| kind     | >= 0.20   | Kubernetes in Docker     |
| kubectl  | >= 1.29   | K8s CLI tool             |
| helm     | >= 3.12   | Package manager          |
| Go       | >= 1.22   | Build engine/portal      |

**Hardware Requirements**: 8 CPU cores / 16 GB RAM / 30 GB disk

## Quick Start

```bash
# One-click deploy
./demo.sh up

# Check status
./demo.sh status

# Deploy demo applications
./demo.sh demo

# Run verification tests
./demo.sh verify

# One-click cleanup
./demo.sh down
```

## Component List

| Component           | Cluster       | Description                                |
|---------------------|---------------|--------------------------------------------|
| engine-manager      | Hub           | Scheduler + TunnelServer + APIServer       |
| engine-scheduler    | Hub           | Scheduler                                  |
| portal              | Hub           | User management API (Gin + SQLite)         |
| engine-agent        | Sub-1, Sub-2  | Edge agent                                 |
| kruise-rollout      | Hub, Sub-1, Sub-2 | Cross-cluster progressive rollout      |
| VictoriaMetrics Single | Hub       | Centralized monitoring storage             |
| vmagent             | Sub-1, Sub-2  | Metrics collection agent                   |
| Submariner Broker   | Hub           | Cross-cluster network hub                  |
| Submariner Operator | Sub-1, Sub-2  | Cross-cluster network agent                |

## Addon Configuration

The demo environment enables the following addons:

| Addon           | Hub          | Sub-1        | Sub-2        | Description                  |
|-----------------|--------------|--------------|--------------|------------------------------|
| mcs-lighthouse  | Broker       | Operator     | Operator     | Cross-cluster service discovery |
| kruise-rollout  | Enabled      | Enabled      | Enabled      | Cross-cluster rollout strategy |
| victoriametrics | VM Single    | vmagent      | vmagent      | Multi-cluster monitoring     |

## Demo Scenarios

### 1. Basic Application Deployment

Create an Application and verify automatic scheduling to Sub clusters:

```bash
kubectl --context kind-kumquat-hub apply -f manifests/demo-application.yaml
```

### 2. Canary Rollout

```bash
kubectl --context kind-kumquat-hub apply -f manifests/canary-demo.yaml
```

### 3. Blue-Green Rollout

```bash
kubectl --context kind-kumquat-hub apply -f manifests/bluegreen-demo.yaml
```

### 4. Cross-Cluster Service Discovery

```bash
# Export service on Sub-1
kubectl --context kind-kumquat-sub-1 apply -f manifests/demo-service-export.yaml

# Access from Sub-2
kubectl --context kind-kumquat-sub-2 run test --image=busybox --rm -it -- \
  wget -qO- nginx.default.svc.clusterset.local
```

### 5. Monitoring Data Query

```bash
# Port-forward VictoriaMetrics
kubectl --context kind-kumquat-hub port-forward -n victoriametrics \
  svc/victoria-metrics-victoria-metrics-single 8428:8428

# Query metrics
curl "http://localhost:8428/api/v1/query?query=up"
```

## Network Design

- All kind clusters join the shared Docker network `kumquat-net` (172.30.0.0/16)
- Non-overlapping Pod CIDR: Hub(10.244) / Sub-1(10.245) / Sub-2(10.246)
- Submariner VXLAN provides cross-cluster Pod-level connectivity and service discovery

## Troubleshooting

```bash
# View Manager logs
kubectl --context kind-kumquat-hub logs -n kumquat-system deploy/engine-manager

# View Agent logs
kubectl --context kind-kumquat-sub-1 logs -n kumquat-system deploy/engine-agent

# Check cluster registration status
kubectl --context kind-kumquat-hub get managedcluster

# Check Addon status
kubectl --context kind-kumquat-hub get managedcluster -o yaml | grep -A5 addonStatus
```
