# Remote agent bootstrap

Remote registration is opt-in. Bootstrap material is externally managed and is
never passed in Helm values or Pod arguments.

First install the Hub CRDs and manager. The hostname or IP used in the Edge
agent's `manager.tunnel` URL must be present in the manager certificate before
cert-manager issues it. Use `certificate.externalDNSNames` for a hostname and
`certificate.externalIPAddresses` for an IP address:

```bash
kubectl --context kind-kumquat-hub apply -f engine/config/crd/bases/

helm upgrade --install engine-manager ./charts/manager \
  --kube-context kind-kumquat-hub \
  --namespace kumquat-system --create-namespace \
  --set 'certificate.externalDNSNames[0]=tunnel.example.test'

kubectl --context kind-kumquat-hub -n kumquat-system \
  wait --for=condition=Available deployment/engine-manager --timeout=5m
kubectl --context kind-kumquat-hub -n kumquat-system \
  wait --for=condition=Ready certificate/engine-manager --timeout=5m
```

Only after the CRD and TLS Secret are ready, generate the matching Hub and Edge
Secrets without printing the token. The script fails clearly if either prerequisite
is absent. It also pre-creates the Edge `ManagedCluster` and its per-cluster Hub
`ClusterRole`/`ClusterRoleBinding`:

```bash
HUB_CONTEXT=kind-kumquat-hub \
EDGE_CONTEXT=kind-edge \
CLUSTER_NAME=edge \
TOKEN_TTL_SECONDS=7200 \
./charts/manager/scripts/create-bootstrap-secrets.sh
```

Finally install the Edge agent. `HUB_TUNNEL_SERVER` must be exactly the DNS name
or IP configured in the manager Certificate SAN above:

```bash
helm upgrade --install engine-agent ./charts/agent --kube-context kind-edge \
  --namespace kumquat-system \
  --set clustername=edge \
  --set manager.master=https://HUB_API_SERVER:6443 \
  --set manager.tunnel=https://tunnel.example.test:5443 \
  --set manager.existingSecret=engine-agent-bootstrap
```

The Hub token has a stable cluster-derived ID and Secret name, a configurable
short TTL (two hours by default), and an immutable
`cluster.kumquat.io/bootstrap-cluster-name` claim. Its exact authentication group
is `system:bootstrappers:engine-agent:<cluster>`, whose role can access only that
`ManagedCluster`. Rerun the same command before expiry to rotate the token in
place. Kubernetes refreshes the mounted Secret files; REST requests use
`BearerTokenFile`, and each tunnel connection attempt rereads the token. The tool
also refreshes `hub-ca.crt` from the Hub kubeconfig and `tunnel-ca.crt` from the
manager TLS Secret, so neither connection disables TLS verification.

Hub-side Application reads for addon control are not part of base registration.
Set `ENABLE_HUB_ADDON_READ=true` when running the tool only when remote addon
reconciliation is intentionally enabled.

Independently, every heartbeat detects rotation of the Edge projected
ServiceAccount token and updates the Hub cluster credential Secret used by Engine
to project workloads into that Edge cluster.

The agent ServiceAccount has only the Kubernetes permissions Engine needs for
workspace and application projection (namespaces, quotas, limits, standard
workload controllers, Jobs/CronJobs, and PodDisruptionBudgets). Addon installation
is separately opt-in with `--set addons.enabled=true`; it is disabled by default
and adds explicit Helm installation permissions without using `cluster-admin`.
