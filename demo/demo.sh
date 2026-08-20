#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
HUB_CLUSTER=kumquat-hub
SHARED_NETWORK=kumquat-net
OWNER_LABEL=io.kumquat.demo.owner
OWNER_VALUE=kumquat-kind-demo-v1
CLUSTER_MARKER=/etc/kumquat-demo-owner
ENGINE_IMAGE=kumquat/engine:demo
API_IMAGE=kumquat/api:demo
MYSQL_IMAGE=mysql:8.4
KIND_NODE_IMAGE=kindest/node:v1.29.2
HUB_CONTEXT="kind-${HUB_CLUSTER}"
NAMESPACE=kumquat-system
API_PORT=31080

log() { printf '[demo] %s\n' "$*"; }
die() { printf '[demo] ERROR: %s\n' "$*" >&2; exit 1; }
has_cluster() { kind get clusters 2>/dev/null | grep -Fxq "$1"; }
owns_cluster() {
  [[ "$(docker exec "$1-control-plane" cat "${CLUSTER_MARKER}" 2>/dev/null || true)" == "${OWNER_VALUE}" ]]
}
owns_network() {
  [[ "$(docker network inspect "${SHARED_NETWORK}" --format "{{ index .Labels \"${OWNER_LABEL}\" }}" 2>/dev/null || true)" == "${OWNER_VALUE}" ]]
}
secret_value() {
  kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" get secret kumquat-api \
    -o "jsonpath={.data.$1}" | base64 --decode
}

check_prerequisites() {
  local tool
  for tool in docker kind kubectl helm go curl openssl python3; do
    command -v "${tool}" >/dev/null || die "missing prerequisite: ${tool}"
  done
  docker info >/dev/null || die "Docker daemon is unavailable"
}

build_images() {
  log "building Engine and API images"
  docker build -t "${ENGINE_IMAGE}" "${PROJECT_ROOT}/engine"
  docker build -t "${API_IMAGE}" -f "${SCRIPT_DIR}/config/api-Dockerfile" "${PROJECT_ROOT}"
}

create_clusters() {
  if docker network inspect "${SHARED_NETWORK}" >/dev/null 2>&1; then
    owns_network || die "Docker network ${SHARED_NETWORK} exists but is not owned by this demo"
  else
    docker network create --driver bridge --subnet 172.30.0.0/16 \
      --label "${OWNER_LABEL}=${OWNER_VALUE}" "${SHARED_NETWORK}" >/dev/null
  fi
  local cluster config
  for cluster in "${HUB_CLUSTER}"; do
    config="${SCRIPT_DIR}/config/hub-kind.yaml"
    if has_cluster "${cluster}" && ! owns_cluster "${cluster}"; then
      die "Kind cluster ${cluster} exists but is not owned by this demo"
    fi
    if ! has_cluster "${cluster}"; then
      log "creating Kind cluster ${cluster}"
      kind create cluster --name "${cluster}" --config "${config}" --image "${KIND_NODE_IMAGE}"
      docker exec "${cluster}-control-plane" sh -c "printf '%s' '${OWNER_VALUE}' > '${CLUSTER_MARKER}'"
    fi
    if ! docker network inspect "${SHARED_NETWORK}" --format '{{range .Containers}}{{.Name}} {{end}}' | grep -Fq "${cluster}-control-plane"; then
      docker network connect "${SHARED_NETWORK}" "${cluster}-control-plane"
    fi
    # kind returns once the control plane is usable, before kube-proxy and the
    # local-path provisioner are necessarily healthy.  Serializing this gate
    # prevents several new clusters from exhausting Docker Desktop startup
    # resources at the same time.
    kubectl --context "kind-${cluster}" wait --for=condition=Ready node --all --timeout=2m
    kubectl --context "kind-${cluster}" -n kube-system rollout status daemonset/kube-proxy --timeout=2m
    kubectl --context "kind-${cluster}" -n local-path-storage rollout status deployment/local-path-provisioner --timeout=2m
  done
}

load_images() {
  local cluster
  for cluster in "${HUB_CLUSTER}"; do
    kind load docker-image "${ENGINE_IMAGE}" --name "${cluster}"
  done
  kind load docker-image "${API_IMAGE}" --name "${HUB_CLUSTER}"
}

install_cert_manager() {
  kubectl --context "${HUB_CONTEXT}" -n cert-manager get deploy cert-manager-webhook >/dev/null 2>&1 && return
  log "installing cert-manager"
  helm repo add jetstack https://charts.jetstack.io --force-update >/dev/null
  helm repo update >/dev/null
  helm --kube-context "${HUB_CONTEXT}" upgrade --install cert-manager jetstack/cert-manager \
    --namespace cert-manager --create-namespace --version v1.14.5 --set installCRDs=true \
    --wait --timeout 5m
}

ensure_secrets() {
  kubectl --context "${HUB_CONTEXT}" create namespace "${NAMESPACE}" --dry-run=client -o yaml | \
    kubectl --context "${HUB_CONTEXT}" apply -f - >/dev/null
  kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" get secret kumquat-api >/dev/null 2>&1 && return
  local jwt admin_password root_password mysql_password
  jwt="$(openssl rand -hex 32)"
  admin_password="$(openssl rand -hex 16)"
  root_password="$(openssl rand -hex 20)"
  mysql_password="$(openssl rand -hex 20)"
  kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" create secret generic kumquat-api \
    --from-literal=jwt-secret="${jwt}" \
    --from-literal=bootstrap-admin-username=admin \
    --from-literal=bootstrap-admin-email=admin@kumquat.local \
    --from-literal=bootstrap-admin-password="${admin_password}" \
    --from-literal=mysql-host=mysql.kumquat-system.svc.cluster.local:3306 \
    --from-literal=mysql-user=kumquat \
    --from-literal=mysql-password="${mysql_password}" \
    --from-literal=mysql-database=kumquat \
    --from-literal=mysql-root-password="${root_password}" >/dev/null
  log "generated credentials in Kubernetes Secret ${NAMESPACE}/kumquat-api"
}

deploy_mysql() {
  log "deploying MySQL 8"
  kubectl --context "${HUB_CONTEXT}" apply -f - <<'YAML'
apiVersion: v1
kind: Service
metadata: {name: mysql, namespace: kumquat-system}
spec:
  selector: {app: kumquat-mysql}
  ports: [{name: mysql, port: 3306, targetPort: 3306}]
---
apiVersion: apps/v1
kind: StatefulSet
metadata: {name: mysql, namespace: kumquat-system}
spec:
  serviceName: mysql
  replicas: 1
  selector: {matchLabels: {app: kumquat-mysql}}
  template:
    metadata: {labels: {app: kumquat-mysql}}
    spec:
      containers:
        - name: mysql
          image: mysql:8.4
          imagePullPolicy: IfNotPresent
          ports: [{containerPort: 3306, name: mysql}]
          env:
            - {name: MYSQL_DATABASE, valueFrom: {secretKeyRef: {name: kumquat-api, key: mysql-database}}}
            - {name: MYSQL_USER, valueFrom: {secretKeyRef: {name: kumquat-api, key: mysql-user}}}
            - {name: MYSQL_PASSWORD, valueFrom: {secretKeyRef: {name: kumquat-api, key: mysql-password}}}
            - {name: MYSQL_ROOT_PASSWORD, valueFrom: {secretKeyRef: {name: kumquat-api, key: mysql-root-password}}}
          readinessProbe:
            exec: {command: [sh, -c, 'mysqladmin ping -h 127.0.0.1 -uroot -p"$MYSQL_ROOT_PASSWORD" --silent']}
            initialDelaySeconds: 5
            periodSeconds: 3
          volumeMounts: [{name: data, mountPath: /var/lib/mysql}]
  volumeClaimTemplates:
    - metadata: {name: data}
      spec:
        accessModes: [ReadWriteOnce]
        resources: {requests: {storage: 2Gi}}
YAML
  kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" rollout status statefulset/mysql --timeout=5m
}

deploy_engine() {
  log "deploying Engine manager and scheduler"
  local crd
  for crd in \
    apps.kumquat.io_applications.yaml \
    storage.kumquat.io_managedclusters.yaml \
    workspace.kumquat.io_workspaces.yaml; do
    kubectl --context "${HUB_CONTEXT}" apply --server-side \
      -f "${PROJECT_ROOT}/engine/config/crd/bases/${crd}" >/dev/null
  done
  install_cert_manager
  helm --kube-context "${HUB_CONTEXT}" upgrade --install engine-manager "${PROJECT_ROOT}/charts/manager" \
    -n "${NAMESPACE}" --set image.repository=kumquat/engine --set image.tag=demo \
    --set image.pullPolicy=IfNotPresent --set service.type=NodePort --set service.nodePort=30443 \
    --timeout 5m
  kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" rollout status deployment/engine-manager --timeout=5m
  kubectl --context "${HUB_CONTEXT}" wait --for=condition=Available \
    apiservice/v1alpha1.cluster.kumquat.io --timeout=2m
  helm --kube-context "${HUB_CONTEXT}" upgrade --install engine-scheduler "${PROJECT_ROOT}/charts/scheduler" \
    -n "${NAMESPACE}" --set image.repository=kumquat/engine --set image.tag=demo \
    --set image.pullPolicy=IfNotPresent --wait --timeout 5m
  kubectl --context "${HUB_CONTEXT}" apply -f - <<YAML
apiVersion: storage.kumquat.io/v1alpha1
kind: ManagedCluster
metadata:
  name: ${HUB_CLUSTER}
  labels: {kumquat.io/demo-cluster: hub}
spec:
  connectionMode: Hub
  apiServer: in-cluster://
YAML
  helm --kube-context "${HUB_CONTEXT}" upgrade --install engine-hub-agent "${PROJECT_ROOT}/charts/agent" \
    -n "${NAMESPACE}" --set image.repository=kumquat/engine --set image.tag=demo \
    --set image.pullPolicy=IfNotPresent --set clustername="${HUB_CLUSTER}" \
    --set heartbeatInterval=5s --wait --timeout 5m
  for attempt in $(seq 1 30); do
    [[ -n "$(kubectl --context "${HUB_CONTEXT}" get managedcluster "${HUB_CLUSTER}" -o jsonpath='{.status.resourceSummary[0].allocatable.cpu}')" ]] && break
    sleep 1
  done
  [[ -n "$(kubectl --context "${HUB_CONTEXT}" get managedcluster "${HUB_CLUSTER}" -o jsonpath='{.status.resourceSummary[0].allocatable.cpu}')" ]] || \
    die "Hub agent did not report scheduler resourceSummary"
}

deploy_api() {
  log "deploying API with MySQL Secret references"
  kubectl --context "${HUB_CONTEXT}" apply -f "${PROJECT_ROOT}/api/deployments/rbac.yaml" >/dev/null
  kubectl --context "${HUB_CONTEXT}" apply -f "${PROJECT_ROOT}/api/deployments/configmap.yaml" >/dev/null
  kubectl --context "${HUB_CONTEXT}" apply -f "${PROJECT_ROOT}/api/deployments/deployment.yaml" >/dev/null
  kubectl --context "${HUB_CONTEXT}" apply -f "${PROJECT_ROOT}/api/deployments/service.yaml" >/dev/null
  kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" set image deployment/api \
    api="${API_IMAGE}" >/dev/null
  kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" patch service api --type merge \
    -p '{"spec":{"type":"NodePort","ports":[{"name":"http","port":8080,"targetPort":8080,"nodePort":30080},{"name":"metrics","port":9090,"targetPort":9090}]}}' >/dev/null
  kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" rollout status deployment/api --timeout=5m
  local attempt
  for attempt in $(seq 1 60); do
    curl --fail --silent "http://127.0.0.1:${API_PORT}/readyz" >/dev/null && return
    sleep 2
  done
  kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" logs deploy/api --tail=100 >&2 || true
  die "API /readyz did not become ready"
}

cmd_up() {
  check_prerequisites
  [[ "${KUMQUAT_DEMO_SKIP_BUILD:-0}" == "1" ]] || build_images
  create_clusters
  load_images
  ensure_secrets
  deploy_mysql
  deploy_engine
  deploy_api
  log "demo ready at http://127.0.0.1:${API_PORT}; credentials remain in Kubernetes Secret"
}

cmd_status() {
  local cluster
  for cluster in "${HUB_CLUSTER}"; do
    has_cluster "${cluster}" && printf 'READY %s\n' "${cluster}" || printf 'MISSING %s\n' "${cluster}"
  done
  if has_cluster "${HUB_CLUSTER}"; then
    kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" get pods
    curl --fail --silent "http://127.0.0.1:${API_PORT}/readyz" && printf '\n'
  fi
}

cmd_test() { "${SCRIPT_DIR}/test/e2e_test.sh"; }

cmd_down() {
  local cluster
  for cluster in kumquat-hub kumquat-sub-1 kumquat-sub-2; do
    if has_cluster "${cluster}"; then
      if owns_cluster "${cluster}"; then
        kind delete cluster --name "${cluster}"
      else
        log "preserving unowned Kind cluster ${cluster}"
      fi
    fi
  done
  if docker network inspect "${SHARED_NETWORK}" >/dev/null 2>&1; then
    if owns_network; then
      docker network rm "${SHARED_NETWORK}" >/dev/null || true
    else
      log "preserving unowned Docker network ${SHARED_NETWORK}"
    fi
  fi
  log "removed only demo-owned clusters and network; images were retained"
}

usage() { printf 'Usage: %s {up|test|e2e|status|down|build}\n' "$0"; }
case "${1:-}" in
  up) cmd_up ;;
  test|e2e) cmd_test ;;
  status) cmd_status ;;
  down) cmd_down ;;
  build) check_prerequisites; build_images ;;
  *) usage; exit 2 ;;
esac
