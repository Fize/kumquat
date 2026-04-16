# KubeVPN Integration TODO

## Overview
Integrate KubeVPN into Kumquat to provide developers with an out-of-the-box solution for connecting local environments to multi-cluster networks.

## Architecture

```
┌─────────────────────────────────────────────────┐
│ Layer 3: kumctl connect (User Interface)        │
│   One command to connect to cluster network      │
│   $ kumctl vpn connect production-east          │
│   $ kumctl vpn connect --workload nginx-app     │
├─────────────────────────────────────────────────┤
│ Layer 2: integrations/kubevpn/ (Deployment)     │
│   - Pre-configured Helm values                   │
│   - Deployment scripts and templates             │
│   - kumctl calls this layer for install/remove   │
├─────────────────────────────────────────────────┤
│ Layer 1: engine (Data Source)                   │
│   - ManagedCluster CR provides cluster info      │
│   - /proxy/{cluster} endpoint                    │
│   - Tunnel infrastructure                        │
└─────────────────────────────────────────────────┘
```

## Directory Structure

```
integrations/
└── kubevpn/
    ├── README.md              # Usage documentation
    ├── README_zh.md           # Chinese documentation
    ├── TODO.md                # This file
    ├── charts/
    │   └── values.yaml        # Pre-configured Helm values
    ├── config/
    │   └── kubevpn.yaml       # Kumquat-specific config template
    ├── scripts/
    │   ├── install.sh         # Install KubeVPN Server to cluster
    │   └── uninstall.sh       # Uninstall KubeVPN Server
    └── kumctl/
        └── connect.go         # kumctl vpn subcommand design
```

## Implementation Phases

### Phase 1: Foundation (Current)
**Status:** In Progress
**Dependencies:** None

- [ ] Create directory structure
- [ ] Write Helm values configuration (`charts/values.yaml`)
- [ ] Write install/uninstall scripts (`scripts/install.sh`, `scripts/uninstall.sh`)
- [ ] Write documentation (`README.md`, `README_zh.md`)
- [ ] Write kumctl integration design (`kumctl/connect.go`)

### Phase 2: kumctl Integration
**Status:** Pending
**Dependencies:** Phase 1 complete, kumctl project initialized

- [ ] Initialize kumctl Go module
- [ ] Implement `kumctl vpn install --cluster <name>`
- [ ] Implement `kumctl vpn uninstall --cluster <name>`
- [ ] Implement `kumctl vpn connect <cluster>`
- [ ] Implement `kumctl vpn disconnect`
- [ ] Implement `kumctl vpn status`
- [ ] Implement `kumctl vpn connect --workload <name>`

### Phase 3: Advanced Features (Optional)
**Status:** Pending
**Dependencies:** Phase 2 stable

- [ ] Multi-cluster VPN mesh support
- [ ] Integration with Kumquat Application for auto-service discovery
- [ ] Web UI integration in Portal
- [ ] Optional: Promote to lightweight Addon for declarative management

## User Workflow

```bash
# 1. Install KubeVPN Server to target cluster
$ kumctl vpn install --cluster production-east

# 2. Connect to cluster network
$ kumctl vpn connect production-east

# 3. Connect to specific workload network
$ kumctl vpn connect production-east --workload nginx-app --namespace default

# 4. Check connection status
$ kumctl vpn status

# 5. Disconnect
$ kumctl vpn disconnect

# 6. Uninstall KubeVPN Server
$ kumctl vpn uninstall --cluster production-east
```

## Technical Notes

### KubeVPN Server Deployment
- Uses official KubeVPN Helm chart: `kubevpn/kubevpn`
- Deploys to `kubevpn` namespace by default
- Configured to work with Kumquat's ManagedCluster credentials

### Local Client Requirements
- User needs `kubevpn` CLI installed locally
- kumctl will check and prompt installation if missing

### Integration with Engine
- Read cluster API server URL from `ManagedCluster.status.apiServerURL`
- Use `/proxy/{cluster}` endpoint for Edge mode clusters
- Leverage existing tunnel infrastructure

## References

- KubeVPN Official: https://kubevpn.dev
- KubeVPN Helm Chart: https://github.com/kubenetworks/kubevpn/tree/master/charts/kubevpn
- Kumquat Engine API: `/proxy/{cluster}` endpoint
- Kumquat CRD: `ManagedCluster` (storage.kumquat.io/v1alpha1)

## Last Updated
2026-04-16
