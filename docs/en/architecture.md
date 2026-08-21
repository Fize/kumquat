# Kumquat Architecture

[中文文档](../zh/architecture.md)

## Overview

Kumquat is a cloud-native multi-cluster application management platform that adopts a **Hub-Spoke** architecture model to manage multiple Kubernetes clusters. This document explains the responsibility boundaries between the user entry points, Kumquat API, Engine control plane, and member clusters.

## Architecture Overview

![Architecture](../images/architecture.drawio.png)

Kumquat is organized into four layers:

| Layer | Responsibility |
|-------|----------------|
| User entry points | Users and automation manage applications, clusters, workspaces, and project resources through `kumctl` or HTTP APIs |
| Kumquat API | Handles authentication, authorization, project/module organization, idempotent operations, and user-facing resource APIs |
| Engine control plane | Runs in the Hub cluster and handles scheduling, distribution, Addon coordination, and status aggregation |
| Member clusters | Run the actual workloads; Hub clusters are accessed directly, while Edge clusters connect back through an Agent tunnel |

## Core Components

### 1. Kumquat API

Kumquat API is the unified entry point for users and automation.

**Key Responsibilities:**
- Manage users, roles, and permissions
- Organize projects, modules, and resource hierarchy
- Accept operations for applications, clusters, workspaces, and related resources
- Provide idempotency and operation status for write requests
- Project user intent into Engine resources that the control plane can execute

### 2. Engine Manager

Engine Manager runs in the Hub cluster and is the execution core of the multi-cluster control plane.

**Key Responsibilities:**
- Watch Application, ManagedCluster, Workspace, and related resources
- Coordinate application scheduling, workload distribution, and status aggregation
- Manage Addon lifecycle
- Provide the reverse tunnel endpoint used by Edge clusters

### 3. ApplicationReconciler

ApplicationReconciler is Kumquat Engine's core controller, responsible for managing the complete lifecycle of Application CRs.

**Key Responsibilities:**
- Watch Application CR create, update, and delete events
- Call Scheduler to select target clusters
- Call Dispatcher to distribute workloads to target clusters
- Coordinate StatusReconciler to aggregate cluster statuses

**Scheduling Flow:**

![Application Reconciler](../images/reconciler_flow.drawio.png)

### 4. Scheduler

The Scheduler adopts a plugin-based architecture similar to Kubernetes Scheduler Framework.

**Scheduling Phases:**

| Phase | Description | Built-in Plugins |
|-------|-------------|------------------|
| **Filter** | Filter out clusters that don't meet requirements | Health, Affinity, TaintToleration, Capacity, VolumeRestriction |
| **Score** | Score candidate clusters (0-100) | Affinity, Resource (LeastAllocated/MostAllocated), TopologySpread |
| **Select** | Final cluster selection based on strategy | SingleCluster, Spread |

**Built-in Plugins:**

| Plugin | Phase | Description |
|--------|-------|-------------|
| Health | Filter | Exclude clusters that are NotReady or disconnected |
| Affinity | Filter/Score | Filter by required affinity, score by preferred affinity |
| TaintToleration | Filter | Check if cluster taints are tolerated by application |
| Capacity | Filter | Check if cluster has sufficient resources |
| VolumeRestriction | Filter | Check if cluster supports required storage types |
| Resource | Score | Score by resource utilization (LeastAllocated/MostAllocated strategies) |
| TopologySpread | Score | Prefer topology domains with fewer replicas (optional, disabled by default) |

### 5. ClientManager

ClientManager manages Kubernetes client connections to member clusters and chooses the correct access path for each cluster.

**Key Features:**
- Supports both Hub and Edge connection modes
- Creates and caches clients on-demand
- Handles connection failures with graceful retry

**Hub Mode Connection:**

```
Manager ──────────────────────────────► Member Cluster API Server
         HTTPS (kubeconfig/token)
```

**Edge Mode Connection:**

```
Manager ◄───────────────────────────── Agent
         WebSocket (Tunnel)
              │
              │ Requests forwarded through Tunnel
              ▼
         Member Cluster API Server
```

### 6. TunnelServer

TunnelServer provides reverse tunnel connections for Edge clusters.

**Workflow:**

```
1. Agent connects to Manager's WebSocket endpoint on startup
   Agent ──────WebSocket──────► Manager:8443/connect

2. Manager verifies Agent identity (Bootstrap Token or SA Token)

3. After connection established, Agent maintains heartbeat
   Agent ────heartbeat (30s)────► Manager

4. When Manager needs to access Edge cluster, requests are forwarded through Tunnel
   Manager ────API Request────► Tunnel ────► Agent ────► Local API Server
```

### 7. Agent

Agent runs in Edge clusters and actively connects back to the Hub-side Manager.

**Key Responsibilities:**
- Establish and maintain the tunnel connection to Manager
- Report cluster heartbeat and basic status
- Receive control-plane requests forwarded through the tunnel
- Avoid exposing the Edge cluster Kubernetes API for inbound access

### 8. Addon Manager

Addon Manager installs and coordinates optional multi-cluster capabilities.

**Built-in Capabilities:**
- MCS/Submariner cross-cluster service discovery and networking
- Kruise Rollout progressive delivery coordination
- VictoriaMetrics multi-cluster monitoring integration
- Custom Addon extension points

### 9. StatusReconciler

StatusReconciler collects workload status from member clusters and aggregates updates to the Application CR.

**Aggregation Logic:**
- Collect workload status from each target cluster
- Calculate global replicas and ready replicas
- Set the application health phase
- Preserve per-cluster status and messages for troubleshooting

**Health Phases:**

| Phase | Description |
|-------|-------------|
| Healthy | Workloads in all clusters are running normally |
| Progressing | Workloads are being updated |
| Degraded | Workloads in one or more clusters are unhealthy |

## Data Flow

### Application Creation Flow

![Application Data Flow](../images/application_flow.drawio.png)

1. A user creates an application through `kumctl` or the Kumquat API.
2. The API handles authentication, authorization, idempotency, and resource conversion.
3. Engine observes the desired state and schedules the application.
4. Dispatcher applies native workloads to the selected member clusters.
5. StatusReconciler collects member-cluster results and updates application status.

### Status Sync Flow

![Status Sync Flow](../images/status_sync_flow.drawio.png)

## Connection Mode Comparison

| Capability | Hub Mode | Edge Mode |
|------------|----------|-----------|
| **Connection direction** | Manager -> cluster | Agent -> Manager |
| **Network requirement** | Member cluster API Server is reachable from the Hub | Agent can reach Manager |
| **Best fit** | Clusters connected through the same VPC or VPN | Clusters behind NAT or firewall |
| **Authentication** | kubeconfig or ServiceAccount Token | Bootstrap Token or ServiceAccount Token |
| **Agent required** | No | Yes |
| **Latency** | Low | Slightly higher because requests traverse the tunnel |

## High Availability

### Manager HA

- Deployed as Kubernetes Deployment, supports multiple replicas
- Uses Leader Election to ensure only one active instance
- Client caching and connection pool management

### Agent HA

- Supports automatic reconnection with exponential backoff
- Automatic connection rebuild on heartbeat timeout
- Local workloads unaffected by Tunnel disconnection

## Related Documentation

- [Scheduler Design](scheduler.md) - Detailed scheduler architecture and plugin mechanism
- [Topology Spread Guide](topology_spread.md) - Cross-region/zone workload distribution
- [Edge Cluster Management](edge.md) - Edge cluster operations and connection model
- [API Reference](api.md) - CRD specifications and examples
