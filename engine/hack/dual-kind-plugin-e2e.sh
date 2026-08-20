#!/usr/bin/env bash
set -euo pipefail

# Safe two-Kind (Hub + Edge) integration harness.  It never owns or deletes
# clusters that were not created by this run.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENGINE_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${ENGINE_ROOT}/.." && pwd)"
HUB_CLUSTER="${KUMQUAT_E2E_HUB_CLUSTER:-kumquat-e2e-hub}"
EDGE_CLUSTER="${KUMQUAT_E2E_EDGE_CLUSTER:-kumquat-e2e-edge}"
NETWORK="${KUMQUAT_E2E_NETWORK:-kumquat-e2e-net}"
OWNER_LABEL="io.kumquat.e2e.owner"
OWNER_VALUE="kumquat-dual-kind-plugin-e2e-v1"
MARKER="/etc/kumquat-dual-kind-plugin-e2e-owner"
IMAGE="${KUMQUAT_E2E_IMAGE:-kumquat/engine:demo}"
NODE_IMAGE="${KUMQUAT_E2E_NODE_IMAGE:-kindest/node:v1.29.2}"
NAMESPACE="kumquat-system"
TUNNEL_HOST="hub-tunnel"
HUB_CONTEXT="kind-${HUB_CLUSTER}"
EDGE_CONTEXT="kind-${EDGE_CLUSTER}"

log() { printf '[dual-kind-e2e] %s\n' "$*"; }
die() { printf '[dual-kind-e2e] ERROR: %s\n' "$*" >&2; exit 1; }
has_cluster() { kind get clusters 2>/dev/null | grep -Fxq "$1"; }
owns_cluster() { [[ "$(docker exec "$1-control-plane" cat "$MARKER" 2>/dev/null || true)" == "$OWNER_VALUE" ]]; }
owns_network() { [[ "$(docker network inspect "$NETWORK" --format "{{ index .Labels \"$OWNER_LABEL\" }}" 2>/dev/null || true)" == "$OWNER_VALUE" ]]; }

require_tools() {
  local tool
  for tool in docker kind kubectl helm openssl curl; do
    command -v "$tool" >/dev/null || die "missing prerequisite: $tool"
  done
  docker info >/dev/null || die "Docker daemon is unavailable"
}

cluster_config() {
  local name="$1" file="$2"
  if [[ "$name" == "$HUB_CLUSTER" ]]; then
    cat >"$file" <<YAML
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: ${HUB_CLUSTER}
networking:
  podSubnet: "10.244.0.0/16"
  serviceSubnet: "10.96.0.0/16"
nodes:
- role: control-plane
  image: ${NODE_IMAGE}
  extraPortMappings:
  - containerPort: 30444
    hostPort: 31444
    protocol: TCP
YAML
  else
    cat >"$file" <<YAML
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: ${EDGE_CLUSTER}
networking:
  podSubnet: "10.245.0.0/16"
  serviceSubnet: "10.101.0.0/16"
nodes:
- role: control-plane
  image: ${NODE_IMAGE}
YAML
  fi
}

connect_cluster() {
  local cluster="$1"
  if ! docker network inspect "$NETWORK" --format '{{range .Containers}}{{.Name}} {{end}}' | grep -Fq "${cluster}-control-plane"; then
    docker network connect "$NETWORK" "${cluster}-control-plane"
  fi
  kubectl --context "kind-${cluster}" wait --for=condition=Ready node --all --timeout=3m
  kubectl --context "kind-${cluster}" -n kube-system rollout status daemonset/kube-proxy --timeout=3m
  kubectl --context "kind-${cluster}" -n local-path-storage rollout status deployment/local-path-provisioner --timeout=3m
}

create() {
  require_tools
  local tmp
  tmp="$(mktemp -d)"

  if docker network inspect "$NETWORK" >/dev/null 2>&1; then
    owns_network || die "Docker network $NETWORK exists but is not owned by this harness"
  else
    docker network create --driver bridge --subnet 172.31.0.0/16 \
      --label "$OWNER_LABEL=$OWNER_VALUE" "$NETWORK" >/dev/null
  fi

  local cluster config
  for cluster in "$HUB_CLUSTER" "$EDGE_CLUSTER"; do
    if has_cluster "$cluster"; then
      owns_cluster "$cluster" || die "Kind cluster $cluster exists but is not owned by this harness"
    else
      config="$tmp/${cluster}.yaml"
      cluster_config "$cluster" "$config"
      log "creating $cluster"
      kind create cluster --name "$cluster" --config "$config"
      docker exec "${cluster}-control-plane" sh -c "printf '%s' '$OWNER_VALUE' > '$MARKER'"
    fi
    connect_cluster "$cluster"
  done
  rm -rf "$tmp"
  log "two Kind clusters are ready: $HUB_CONTEXT and $EDGE_CONTEXT"
}

load_image() {
  if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
    log "building $IMAGE"
    docker build -t "$IMAGE" "$ENGINE_ROOT"
  fi
  kind load docker-image "$IMAGE" --name "$HUB_CLUSTER"
  kind load docker-image "$IMAGE" --name "$EDGE_CLUSTER"
}

install_cert_manager() {
  helm repo add jetstack https://charts.jetstack.io --force-update >/dev/null
  helm repo update >/dev/null
  helm --kube-context "$HUB_CONTEXT" upgrade --install cert-manager jetstack/cert-manager \
    --namespace cert-manager --create-namespace --version v1.14.5 --set installCRDs=true \
    --wait --timeout 8m
}

install_manager() {
  # The aggregated cluster API is served by manager's APIService; installing
  # the similarly named clusters.cluster.kumquat.io CRD would conflict with
  # that APIService.  Only storage/workload CRDs belong on the Hub.
  for crd in \
    "$ENGINE_ROOT/config/crd/bases/apps.kumquat.io_applications.yaml" \
    "$ENGINE_ROOT/config/crd/bases/storage.kumquat.io_managedclusters.yaml" \
    "$ENGINE_ROOT/config/crd/bases/workspace.kumquat.io_workspaces.yaml"; do
    kubectl --context "$HUB_CONTEXT" apply --server-side -f "$crd" >/dev/null
  done
  # A previous interrupted run may have left the conflicting CRD behind; its
  # API aggregation entry is auto-managed and cannot be adopted by Helm.
  kubectl --context "$HUB_CONTEXT" delete crd/clusters.cluster.kumquat.io --ignore-not-found >/dev/null 2>&1 || true
  kubectl --context "$HUB_CONTEXT" delete apiservice/v1alpha1.cluster.kumquat.io --ignore-not-found >/dev/null 2>&1 || true
  kubectl --context "$HUB_CONTEXT" create namespace "$NAMESPACE" --dry-run=client -o yaml | \
    kubectl --context "$HUB_CONTEXT" apply -f - >/dev/null
  install_cert_manager
  helm --kube-context "$HUB_CONTEXT" upgrade --install engine-manager "$REPO_ROOT/charts/manager" \
    --namespace "$NAMESPACE" --set image.repository="${IMAGE%%:*}" --set image.tag="${IMAGE##*:}" \
    --set image.pullPolicy=IfNotPresent --set service.type=NodePort --set service.nodePort=30444 \
    --set "certificate.externalDNSNames[0]=${TUNNEL_HOST}" --wait --timeout 8m
  kubectl --context "$HUB_CONTEXT" -n "$NAMESPACE" rollout status deployment/engine-manager --timeout=8m
  kubectl --context "$HUB_CONTEXT" -n "$NAMESPACE" wait --for=condition=Ready certificate/engine-manager --timeout=5m
  helm --kube-context "$HUB_CONTEXT" upgrade --install engine-scheduler "$REPO_ROOT/charts/scheduler" \
    --namespace "$NAMESPACE" --set image.repository="${IMAGE%%:*}" --set image.tag="${IMAGE##*:}" \
    --set image.pullPolicy=IfNotPresent --wait --timeout 8m
}

bootstrap_edge() {
  local hub_ip
  hub_ip="$(docker inspect -f "{{with index .NetworkSettings.Networks \"${NETWORK}\"}}{{.IPAddress}}{{end}}" "${HUB_CLUSTER}-control-plane")"
  [[ -n "$hub_ip" ]] || die "could not resolve Hub IP on $NETWORK"
  HUB_CONTEXT="$HUB_CONTEXT" EDGE_CONTEXT="$EDGE_CONTEXT" CLUSTER_NAME=edge \
    TOKEN_TTL_SECONDS=7200 NAMESPACE="$NAMESPACE" TUNNEL_TLS_SECRET=engine-manager-secret \
    "$REPO_ROOT/charts/manager/scripts/create-bootstrap-secrets.sh"
  helm --kube-context "$EDGE_CONTEXT" upgrade --install engine-agent "$REPO_ROOT/charts/agent" \
    --namespace "$NAMESPACE" --create-namespace \
    --set image.repository="${IMAGE%%:*}" --set image.tag="${IMAGE##*:}" \
    --set image.pullPolicy=IfNotPresent --set clustername=edge --set heartbeatInterval=5s \
    --set manager.master=https://kubernetes:6443 --set manager.tunnel="https://${TUNNEL_HOST}:30444" \
    --set manager.existingSecret=engine-agent-bootstrap --set addons.enabled=true \
    --timeout 8m
  kubectl --context "$EDGE_CONTEXT" -n "$NAMESPACE" patch deployment/engine-agent --type=json \
    -p="[{\"op\":\"add\",\"path\":\"/spec/template/spec/hostAliases\",\"value\":[{\"ip\":\"$hub_ip\",\"hostnames\":[\"kubernetes\",\"$TUNNEL_HOST\"]}]}]" >/dev/null
  kubectl --context "$EDGE_CONTEXT" -n "$NAMESPACE" rollout status deployment/engine-agent --timeout=8m
}

up() {
  create
  load_image
  install_manager
  bootstrap_edge
  log "Hub manager and Edge agent are running"
}

json_status() {
  local context="$1"
  kubectl --context "$context" get managedcluster edge -o json
}

wait_for_edge() {
  local deadline=$((SECONDS + ${KUMQUAT_E2E_EDGE_TIMEOUT:-300}))
  while (( SECONDS < deadline )); do
    local state
    state="$(json_status "$HUB_CONTEXT" 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("status",{}).get("state", ""))' 2>/dev/null || true)"
    if [[ "$state" == "Ready" ]]; then return 0; fi
    sleep 3
  done
  kubectl --context "$HUB_CONTEXT" get managedcluster edge -o yaml >&2 || true
  kubectl --context "$EDGE_CONTEXT" -n "$NAMESPACE" logs deploy/engine-agent --tail=120 >&2 || true
  return 1
}

set_all_addons() {
  kubectl --context "$HUB_CONTEXT" patch managedcluster edge --type=merge -p '{"spec":{"addons":[{"name":"mcs-lighthouse","enabled":true},{"name":"kruise-rollout","enabled":true},{"name":"victoriametrics","enabled":true}]}}' >/dev/null
}

addon_state() {
  local name="$1"
  kubectl --context "$HUB_CONTEXT" get managedcluster edge -o json | \
    python3 -c 'import json,sys; n=sys.argv[1]; d=json.load(sys.stdin); print(next((x.get("state","") for x in d.get("status",{}).get("addonStatus",[]) if x.get("name")==n), ""))' "$name"
}

wait_addons() {
  local deadline=$((SECONDS + ${KUMQUAT_E2E_ADDON_TIMEOUT:-900})) name state
  for name in mcs-lighthouse kruise-rollout victoriametrics; do
    while (( SECONDS < deadline )); do
      state="$(addon_state "$name" 2>/dev/null || true)"
      [[ "$state" == "Applied" ]] && break
      [[ "$state" == "Failed" ]] && break
      sleep 5
    done
    state="$(addon_state "$name" 2>/dev/null || true)"
    printf 'addon=%s state=%s\n' "$name" "$state"
    [[ "$state" == "Applied" ]] || return 1
  done
}

assert_plugin_resources() {
  log "checking plugin resources on Hub and Edge"
  kubectl --context "$HUB_CONTEXT" get namespace submariner-k8s-broker >/dev/null
  kubectl --context "$HUB_CONTEXT" get secret -n submariner-k8s-broker submariner-k8s-broker-client-token >/dev/null
  kubectl --context "$EDGE_CONTEXT" get namespace submariner-operator >/dev/null
  kubectl --context "$EDGE_CONTEXT" get crd rollouts.rollouts.kruise.io >/dev/null
  kubectl --context "$EDGE_CONTEXT" -n kruise-rollout rollout status deployment/kruise-rollout-controller-manager --timeout=5m >/dev/null
  kubectl --context "$EDGE_CONTEXT" -n kumquat-system rollout status deployment/submariner-submariner-operator --timeout=5m >/dev/null
  kubectl --context "$HUB_CONTEXT" -n victoriametrics rollout status statefulset/victoria-metrics-victoria-metrics-single-server --timeout=5m >/dev/null
  kubectl --context "$EDGE_CONTEXT" -n vm-agent rollout status deployment/vm-agent-victoria-metrics-agent --timeout=5m >/dev/null
  helm --kube-context "$HUB_CONTEXT" list -A --filter 'submariner-k8s-broker|victoria-metrics|kruise-rollout' --deployed
  helm --kube-context "$EDGE_CONTEXT" list -A --filter 'submariner|victoria|kruise-rollout' --deployed
}

test_run() {
  require_tools
  has_cluster "$HUB_CLUSTER" && owns_cluster "$HUB_CLUSTER" || die "owned Hub cluster is not present; run up first"
  has_cluster "$EDGE_CLUSTER" && owns_cluster "$EDGE_CLUSTER" || die "owned Edge cluster is not present; run up first"
  wait_for_edge
  set_all_addons
  wait_addons
  assert_plugin_resources
  log "dual-kind agent and all plugin checks passed"
}

status() {
  kind get clusters
  kubectl --context "$HUB_CONTEXT" get managedcluster edge -o wide 2>/dev/null || true
  kubectl --context "$EDGE_CONTEXT" -n "$NAMESPACE" get pods 2>/dev/null || true
}

down() {
  local cluster
  for cluster in "$EDGE_CLUSTER" "$HUB_CLUSTER"; do
    if has_cluster "$cluster"; then
      if owns_cluster "$cluster"; then
        kind delete cluster --name "$cluster"
      else
        log "preserving unowned cluster $cluster"
      fi
    fi
  done
  if docker network inspect "$NETWORK" >/dev/null 2>&1 && owns_network; then
    docker network rm "$NETWORK" >/dev/null || true
  fi
  log "removed only dual-kind harness resources"
}

case "${1:-}" in
  up) up ;;
  test|e2e) test_run ;;
  status) status ;;
  down) down ;;
  *) printf 'Usage: %s {up|test|status|down}\n' "$0"; exit 2 ;;
esac
