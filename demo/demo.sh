#!/usr/bin/env bash
# Kumquat Demo One-Click Deployment Script
#
# Creates a 3-cluster environment (1 Hub + 2 Sub) locally based on kind,
# and deploys all Kumquat components.
#
# Usage:
#   ./demo.sh up       - Create all resources
#   ./demo.sh down     - Clean up all resources
#   ./demo.sh status   - Show deployment status
#   ./demo.sh demo     - Deploy demo applications
#   ./demo.sh verify   - Run verification tests
#   ./demo.sh build    - Build images only

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# ======================== Configuration ========================

# Cluster names
HUB_CLUSTER="kumquat-hub"
SUB_CLUSTERS=("kumquat-sub-1" "kumquat-sub-2")
ALL_CLUSTERS=("${HUB_CLUSTER}" "${SUB_CLUSTERS[@]}")

# Shared Docker network
SHARED_NETWORK="kumquat-net"
SHARED_SUBNET="172.30.0.0/16"

# Pod CIDR (must not overlap)
HUB_POD_CIDR="10.244.0.0/16"
SUB1_POD_CIDR="10.245.0.0/16"
SUB2_POD_CIDR="10.246.0.0/16"

# Service CIDR (must not overlap)
HUB_SVC_CIDR="10.96.0.0/16"
SUB1_SVC_CIDR="10.97.0.0/16"
SUB2_SVC_CIDR="10.98.0.0/16"

# Image configuration
ENGINE_IMAGE="kumquat/engine:demo"
PORTAL_IMAGE="kumquat/portal:demo"
KIND_NODE_IMAGE="kindest/node:v1.29.2"

# Hub cluster port mappings
TUNNEL_NODEPORT=30443
PORTAL_NODEPORT=30080
VM_NODEPORT=30842

# Bootstrap Token (must match manager chart default)
BOOTSTRAP_TOKEN="07401b.f395accd246ae52d"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# ======================== Utility Functions ========================

log_info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_step()  { echo -e "${BLUE}[STEP]${NC} $1"; }
log_phase() { echo -e "${CYAN}[PHASE]${NC} $1"; }

retry() {
    local max_attempts=$1
    local interval=$2
    local cmd="${@:3}"
    local attempt=1

    while [[ $attempt -le $max_attempts ]]; do
        if eval "$cmd" 2>/dev/null; then
            return 0
        fi
        attempt=$((attempt + 1))
        if [[ $attempt -le $max_attempts ]]; then
            sleep "$interval"
        fi
    done
    return 1
}

wait_for_pods() {
    local context=$1
    local namespace=${2:-""}
    local timeout=${3:-300}

    log_info "Waiting for pods in ${context} ${namespace} to be Ready (timeout: ${timeout}s)..."

    if [[ -n "$namespace" ]]; then
        kubectl --context "$context" wait --for=condition=Ready pods \
            --namespace "$namespace" --all --timeout="${timeout}s" 2>/dev/null || true
    else
        kubectl --context "$context" wait --for=condition=Ready pods \
            --all-namespaces --field-selector=status.phase!=Succeeded --timeout="${timeout}s" 2>/dev/null || true
    fi
}

get_cluster_ip() {
    local cluster=$1
    docker inspect "${cluster}-control-plane" \
        --format "{{(index .NetworkSettings.Networks \"${SHARED_NETWORK}\").IPAddress}}" 2>/dev/null
}

# ======================== Prerequisites Check ========================

check_prerequisites() {
    log_phase "Checking prerequisites..."

    local missing=()

    if ! command -v docker &> /dev/null; then
        missing+=("docker")
    fi
    if ! command -v kind &> /dev/null; then
        missing+=("kind")
    fi
    if ! command -v kubectl &> /dev/null; then
        missing+=("kubectl")
    fi
    if ! command -v helm &> /dev/null; then
        missing+=("helm")
    fi
    if ! command -v go &> /dev/null; then
        missing+=("go")
    fi

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing[*]}"
        log_error "Please install them before running this script."
        exit 1
    fi

    # Check Docker is running
    if ! docker info &> /dev/null; then
        log_error "Docker is not running. Please start Docker first."
        exit 1
    fi

    log_info "All prerequisites met."
}

# ======================== Build Images ========================

build_images() {
    log_phase "Building Docker images..."

    # Build engine image (manager + agent)
    log_step "Building engine image (${ENGINE_IMAGE})..."
    docker build --no-cache -t "${ENGINE_IMAGE}" \
        -f "${PROJECT_ROOT}/engine/Dockerfile" \
        "${PROJECT_ROOT}/engine/"

    # Also tag as engine-agent for clarity
    docker tag "${ENGINE_IMAGE}" "kumquat/engine-agent:demo" 2>/dev/null || true

    # Build portal image
    log_step "Building portal image (${PORTAL_IMAGE})..."
    if [[ -f "${PROJECT_ROOT}/portal/Dockerfile" ]]; then
        docker build --no-cache -t "${PORTAL_IMAGE}" \
            -f "${PROJECT_ROOT}/portal/Dockerfile" \
            "${PROJECT_ROOT}"
    else
        # Use demo Dockerfile if portal doesn't have its own
        docker build --no-cache -t "${PORTAL_IMAGE}" \
            -f "${SCRIPT_DIR}/config/portal-Dockerfile" \
            "${PROJECT_ROOT}"
    fi

    log_info "Images built successfully."
}

# ======================== Create Clusters ========================

create_shared_network() {
    log_step "Creating shared Docker network (${SHARED_NETWORK})..."
    if docker network inspect "${SHARED_NETWORK}" &> /dev/null; then
        log_warn "Network ${SHARED_NETWORK} already exists, skipping."
    else
        docker network create --driver bridge --subnet "${SHARED_SUBNET}" "${SHARED_NETWORK}"
        log_info "Network ${SHARED_NETWORK} created."
    fi
}

create_clusters() {
    log_phase "Creating kind clusters..."

    # Create Hub cluster
    log_step "Creating Hub cluster (${HUB_CLUSTER})..."
    if kind get clusters 2>/dev/null | grep -q "^${HUB_CLUSTER}$"; then
        log_warn "Cluster ${HUB_CLUSTER} already exists, skipping."
    else
        kind create cluster --name "${HUB_CLUSTER}" \
            --config "${SCRIPT_DIR}/config/hub-kind.yaml" \
            --image "${KIND_NODE_IMAGE}" \
            --retain
    fi

    # Create Sub clusters
    for i in "${!SUB_CLUSTERS[@]}"; do
        local sub="${SUB_CLUSTERS[$i]}"
        local config_file="${SCRIPT_DIR}/config/sub$((i+1))-kind.yaml"
        log_step "Creating Sub cluster (${sub})..."
        if kind get clusters 2>/dev/null | grep -q "^${sub}$"; then
            log_warn "Cluster ${sub} already exists, skipping."
        else
            kind create cluster --name "${sub}" \
                --config "${config_file}" \
                --image "${KIND_NODE_IMAGE}" \
                --retain
        fi
    done
}

connect_network() {
    log_phase "Connecting clusters to shared network and configuring routes..."

    # Connect all cluster nodes to shared network
    for cluster in "${ALL_CLUSTERS[@]}"; do
        local node="${cluster}-control-plane"
        log_step "Connecting ${node} to ${SHARED_NETWORK}..."
        if docker network inspect "${SHARED_NETWORK}" \
                --format '{{range .Containers}}{{.Name}} {{end}}' 2>/dev/null \
                | grep -q "${node}"; then
            log_warn "${node} already in ${SHARED_NETWORK}, skipping."
        else
            docker network connect "${SHARED_NETWORK}" "${node}"
        fi
    done

    # Configure routes between clusters
    local hub_ip
    hub_ip=$(get_cluster_ip "${HUB_CLUSTER}")

    local sub1_ip
    sub1_ip=$(get_cluster_ip "${SUB_CLUSTERS[0]}")

    local sub2_ip
    sub2_ip=$(get_cluster_ip "${SUB_CLUSTERS[1]}")

    log_step "Configuring Pod CIDR routes between clusters..."

    # Hub -> Sub-1, Sub-2
    docker exec "${HUB_CLUSTER}-control-plane" \
        ip route add "${SUB1_POD_CIDR}" via "${sub1_ip}" 2>/dev/null || true
    docker exec "${HUB_CLUSTER}-control-plane" \
        ip route add "${SUB2_POD_CIDR}" via "${sub2_ip}" 2>/dev/null || true

    # Sub-1 -> Hub, Sub-2
    docker exec "${SUB_CLUSTERS[0]}-control-plane" \
        ip route add "${HUB_POD_CIDR}" via "${hub_ip}" 2>/dev/null || true
    docker exec "${SUB_CLUSTERS[0]}-control-plane" \
        ip route add "${SUB2_POD_CIDR}" via "${sub2_ip}" 2>/dev/null || true

    # Sub-2 -> Hub, Sub-1
    docker exec "${SUB_CLUSTERS[1]}-control-plane" \
        ip route add "${HUB_POD_CIDR}" via "${hub_ip}" 2>/dev/null || true
    docker exec "${SUB_CLUSTERS[1]}-control-plane" \
        ip route add "${SUB1_POD_CIDR}" via "${sub1_ip}" 2>/dev/null || true

    log_info "Network connectivity configured."
}

# ======================== Ensure Cluster Health ========================

ensure_cluster_health() {
    log_phase "Ensuring cluster health (sysctl, kube-proxy)..."

    for cluster in "${ALL_CLUSTERS[@]}"; do
        local node="${cluster}-control-plane"
        local ctx="kind-${cluster}"

        # Tune sysctl parameters to prevent "too many open files" in kube-proxy
        log_step "Tuning sysctl on ${node}..."
        docker exec "${node}" sh -c "
            sysctl -w fs.inotify.max_user_watches=524288 2>/dev/null
            sysctl -w fs.inotify.max_user_instances=512 2>/dev/null
            sysctl -w fs.file-max=2097152 2>/dev/null
            sysctl -w net.netfilter.nf_conntrack_max=262144 2>/dev/null
        " 2>/dev/null || true

        # Restart kube-proxy if it's not running properly
        log_step "Checking kube-proxy on ${cluster}..."
        local kp_status
        kp_status=$(kubectl --context "${ctx}" -n kube-system get pods -l k8s-app=kube-proxy \
            -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo "Unknown")

        if [[ "${kp_status}" != "Running" ]]; then
            log_warn "kube-proxy not Running (status: ${kp_status}), restarting..."
            kubectl --context "${ctx}" -n kube-system delete pods -l k8s-app=kube-proxy --force --grace-period=0 2>/dev/null || true
            # Wait for kube-proxy to come back
            local kp_retries=0
            while [[ $kp_retries -lt 30 ]]; do
                kp_status=$(kubectl --context "${ctx}" -n kube-system get pods -l k8s-app=kube-proxy \
                    -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo "Unknown")
                if [[ "${kp_status}" == "Running" ]]; then
                    break
                fi
                kp_retries=$((kp_retries + 1))
                sleep 2
            done
            if [[ "${kp_status}" == "Running" ]]; then
                log_info "kube-proxy on ${cluster} is now Running"
            else
                log_error "kube-proxy on ${cluster} failed to start (status: ${kp_status})"
            fi
        else
            log_info "kube-proxy on ${cluster} is Running"
        fi
    done
}

# ======================== Load Images ========================

load_images() {
    log_phase "Loading images into kind clusters..."

    for cluster in "${ALL_CLUSTERS[@]}"; do
        log_step "Loading images into ${cluster}..."
        kind load docker-image "${ENGINE_IMAGE}" --name "${cluster}"
        kind load docker-image "${PORTAL_IMAGE}" --name "${cluster}"
    done

    log_info "Images loaded."
}

# ======================== Deploy Hub ========================

install_cert_manager() {
    log_phase "Installing cert-manager..."

    local ctx="kind-${HUB_CLUSTER}"

    # Check if cert-manager is already installed
    if kubectl --context "$ctx" get deployment -n cert-manager cert-manager-webhook &>/dev/null; then
        log_info "cert-manager already installed, skipping."
        return 0
    fi

    log_step "Installing cert-manager via Helm..."
    helm repo add jetstack https://charts.jetstack.io --force-update 2>/dev/null || true
    helm repo update

    helm --kube-context "$ctx" upgrade --install cert-manager jetstack/cert-manager \
        --namespace cert-manager \
        --create-namespace \
        --version v1.14.5 \
        --set installCRDs=true \
        --wait --timeout 300s || {
        log_error "cert-manager installation failed"
        exit 1
    }

    log_info "cert-manager installed successfully."
}

deploy_hub() {
    log_phase "Deploying Hub cluster components..."

    local ctx="kind-${HUB_CLUSTER}"

    # Install cert-manager first (required by manager webhook certs)
    install_cert_manager

    # Apply CRDs
    log_step "Installing CRDs..."
    kubectl --context "$ctx" apply --server-side \
        -f "${PROJECT_ROOT}/engine/config/crd/bases/"

    # Wait for CRDs
    kubectl --context "$ctx" wait --for=condition=Established \
        crd/applications.apps.kumquat.io --timeout=60s || true
    kubectl --context "$ctx" wait --for=condition=Established \
        crd/managedclusters.storage.kumquat.io --timeout=60s || true

    # Create namespace
    kubectl --context "$ctx" create namespace kumquat-system \
        --dry-run=client -o yaml | kubectl --context "$ctx" apply -f -

    # Get Hub control plane IP
    local hub_ip
    hub_ip=$(get_cluster_ip "${HUB_CLUSTER}")

    # Install Manager via Helm
    log_step "Installing engine-manager..."
    helm --kube-context "$ctx" upgrade --install engine-manager \
        "${PROJECT_ROOT}/charts/manager/" \
        --namespace kumquat-system \
        --set image.repository=kumquat/engine \
        --set image.tag=demo \
        --set image.pullPolicy=IfNotPresent \
        --set tunnelPort=5443 \
        --set service.type=NodePort \
        --set service.nodePort=30443 \
        --set insecureSkipTLSVerify=true \
        --wait --timeout 300s || {
        log_warn "Helm install for manager may have issues, continuing..."
    }

    # Install Scheduler via Helm
    log_step "Installing engine-scheduler..."
    helm --kube-context "$ctx" upgrade --install engine-scheduler \
        "${PROJECT_ROOT}/charts/scheduler/" \
        --namespace kumquat-system \
        --set image.repository=kumquat/engine \
        --set image.tag=demo \
        --set image.pullPolicy=IfNotPresent \
        --wait --timeout 300s || {
        log_warn "Helm install for scheduler may have issues, continuing..."
    }

    # Deploy Portal
    log_step "Installing portal..."
    deploy_portal "$ctx"

    # Wait for Hub pods
    log_step "Waiting for Hub pods to be Ready..."
    wait_for_pods "$ctx" "kumquat-system" 300

    log_info "Hub cluster deployed."
}

deploy_portal() {
    local ctx=$1

    # Create portal namespace
    kubectl --context "$ctx" create namespace portal \
        --dry-run=client -o yaml | kubectl --context "$ctx" apply -f -

    # Create portal ServiceAccount and RBAC
    kubectl --context "$ctx" apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: portal
  namespace: portal
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: portal-manager
rules:
  - apiGroups: ["cluster.kumquat.io"]
    resources: ["clusters"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["cluster.kumquat.io"]
    resources: ["clusters/status"]
    verbs: ["get", "update", "patch"]
  - apiGroups: ["apps.kumquat.io"]
    resources: ["applications"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["apps.kumquat.io"]
    resources: ["applications/status"]
    verbs: ["get", "update", "patch"]
  - apiGroups: ["workspace.kumquat.io"]
    resources: ["workspaces"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["workspace.kumquat.io"]
    resources: ["workspaces/status"]
    verbs: ["get", "update", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: portal-manager
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: portal-manager
subjects:
  - kind: ServiceAccount
    name: portal
    namespace: portal
EOF

    # Create portal ConfigMap
    kubectl --context "$ctx" create configmap portal-config \
        --from-file=config.yaml="${SCRIPT_DIR}/config/portal-config.yaml" \
        --namespace portal \
        --dry-run=client -o yaml | kubectl --context "$ctx" apply -f -

    # Create portal PVC for SQLite data
    kubectl --context "$ctx" apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: portal-data
  namespace: portal
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
EOF

    # Deploy portal
    kubectl --context "$ctx" apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: portal
  namespace: portal
spec:
  replicas: 1
  selector:
    matchLabels:
      app: portal
  template:
    metadata:
      labels:
        app: portal
    spec:
      serviceAccountName: portal
      containers:
        - name: portal
          image: "${PORTAL_IMAGE}"
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: config
              mountPath: /app/config
            - name: data
              mountPath: /data
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 15
          readinessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 512Mi
      volumes:
        - name: config
          configMap:
            name: portal-config
        - name: data
          persistentVolumeClaim:
            claimName: portal-data
---
apiVersion: v1
kind: Service
metadata:
  name: portal
  namespace: portal
spec:
  type: NodePort
  selector:
    app: portal
  ports:
    - port: 8080
      targetPort: 8080
      nodePort: ${PORTAL_NODEPORT}
EOF
}

# ======================== Deploy Sub ========================

deploy_sub_clusters() {
    log_phase "Deploying Sub cluster agents..."

    local hub_ip
    hub_ip=$(get_cluster_ip "${HUB_CLUSTER}")

    for i in "${!SUB_CLUSTERS[@]}"; do
        local sub="${SUB_CLUSTERS[$i]}"
        local ctx="kind-${sub}"

        log_step "Installing engine-agent on ${sub}..."

        # Create namespace
        kubectl --context "$ctx" create namespace kumquat-system \
            --dry-run=client -o yaml | kubectl --context "$ctx" apply -f -

        # Install Agent via Helm
        helm --kube-context "$ctx" upgrade --install engine-agent \
            "${PROJECT_ROOT}/charts/agent/" \
            --namespace kumquat-system \
            --set image.repository=kumquat/engine \
            --set image.tag=demo \
            --set image.pullPolicy=IfNotPresent \
            --set clustername="${sub}" \
            --set manager.token="${BOOTSTRAP_TOKEN}" \
            --set manager.master="https://${hub_ip}:6443" \
            --set manager.tunnel="https://${hub_ip}:30443" \
            --wait --timeout 300s || {
            log_warn "Helm install for agent on ${sub} may have issues, continuing..."
        }

        # Wait for pods
        wait_for_pods "$ctx" "kumquat-system" 300
    done

    log_info "Sub clusters deployed."
}

# ======================== Register Clusters ========================

register_clusters() {
    log_phase "Registering ManagedCluster CRs on Hub..."

    local ctx="kind-${HUB_CLUSTER}"

    for sub in "${SUB_CLUSTERS[@]}"; do
        log_step "Registering ${sub}..."

        kubectl --context "$ctx" apply -f - <<EOF
apiVersion: storage.kumquat.io/v1alpha1
kind: ManagedCluster
metadata:
  name: ${sub}
spec:
  connectionMode: Edge
  addons:
    - name: mcs-lighthouse
      enabled: true
      config:
        submarinerChartVersion: "0.23.0-m0"
    - name: kruise-rollout
      enabled: true
      config:
        chartVersion: "0.6.2"
    - name: victoriametrics
      enabled: true
EOF
    done

    # Also enable addons on Hub (for demo when Hub runs workloads too)
    log_step "Enabling addons on Hub cluster..."
    kubectl --context "$ctx" apply -f - <<EOF
apiVersion: storage.kumquat.io/v1alpha1
kind: ManagedCluster
metadata:
  name: ${HUB_CLUSTER}
spec:
  connectionMode: Hub
  addons:
    - name: mcs-lighthouse
      enabled: true
      config:
        submarinerChartVersion: "0.23.0-m0"
    - name: kruise-rollout
      enabled: true
      config:
        chartVersion: "0.6.2"
    - name: victoriametrics
      enabled: true
EOF

    log_info "Clusters registered with addons."
}

# ======================== Main Commands ========================

cmd_up() {
    echo ""
    echo "=============================================="
    echo "  Kumquat Demo - One-Click Deploy"
    echo "=============================================="
    echo ""

    check_prerequisites

    # Phase 1: Build
    build_images

    # Phase 2: Create shared network
    create_shared_network

    # Phase 3: Create clusters
    create_clusters

    # Phase 4: Connect network
    connect_network

    # Phase 4.5: Ensure cluster health
    ensure_cluster_health

    # Phase 5: Load images
    load_images

    # Phase 6: Deploy Hub
    deploy_hub

    # Phase 7: Deploy Sub clusters
    deploy_sub_clusters

    # Phase 8: Register clusters
    register_clusters

    echo ""
    echo "=============================================="
    echo "  Kumquat Demo Deployed Successfully!"
    echo "=============================================="
    echo ""
    print_access_info
}

cmd_down() {
    echo ""
    echo "=============================================="
    echo "  Kumquat Demo - Cleanup"
    echo "=============================================="
    echo ""

    for cluster in "${ALL_CLUSTERS[@]}"; do
        if kind get clusters 2>/dev/null | grep -q "^${cluster}$"; then
            log_info "Deleting cluster ${cluster}..."
            kind delete cluster --name "${cluster}"
        else
            log_warn "Cluster ${cluster} does not exist."
        fi
    done

    # Remove shared network
    if docker network inspect "${SHARED_NETWORK}" &> /dev/null; then
        log_info "Removing network ${SHARED_NETWORK}..."
        docker network rm "${SHARED_NETWORK}" || true
    fi

    # Remove demo images (optional)
    log_info "Demo images retained (use 'docker rmi' to remove manually if needed)."

    echo ""
    log_info "Cleanup complete."
}

cmd_status() {
    echo ""
    echo "=============================================="
    echo "  Kumquat Demo - Status Overview"
    echo "=============================================="
    echo ""

    for cluster in "${ALL_CLUSTERS[@]}"; do
        local ctx="kind-${cluster}"
        if kind get clusters 2>/dev/null | grep -q "^${cluster}$"; then
            echo -e "${GREEN}OK${NC} ${cluster}"
            echo "  Nodes:"
            kubectl --context "$ctx" get nodes -o wide 2>/dev/null || echo "    (unable to get nodes)"
            echo ""
            echo "  Pods (kumquat-system):"
            kubectl --context "$ctx" get pods -n kumquat-system 2>/dev/null || echo "    (no pods)"
            echo ""
            if [[ "$cluster" == "${HUB_CLUSTER}" ]]; then
                echo "  Pods (portal):"
                kubectl --context "$ctx" get pods -n portal 2>/dev/null || echo "    (no pods)"
                echo ""
            fi
        else
            echo -e "${RED}X${NC} ${cluster} (not created)"
        fi
        echo ""
    done

    # Show ManagedCluster status on Hub
    local hub_ctx="kind-${HUB_CLUSTER}"
    if kind get clusters 2>/dev/null | grep -q "^${HUB_CLUSTER}$"; then
        echo "ManagedClusters:"
        kubectl --context "$hub_ctx" get managedcluster 2>/dev/null || echo "  (none)"
        echo ""

        echo "CRDs:"
        kubectl --context "$hub_ctx" get crd 2>/dev/null | grep kumquat || echo "  (no Kumquat CRDs)"
    fi
}

cmd_demo() {
    echo ""
    echo "=============================================="
    echo "  Kumquat Demo - Deploy Demo App"
    echo "=============================================="
    echo ""

    local ctx="kind-${HUB_CLUSTER}"

    if ! kind get clusters 2>/dev/null | grep -q "^${HUB_CLUSTER}$"; then
        log_error "Hub cluster not found. Please run './demo.sh up' first."
        exit 1
    fi

    # Deploy demo application
    log_step "Deploying demo application (nginx, 6 replicas across 2 sub clusters)..."
    kubectl --context "$ctx" apply -f "${SCRIPT_DIR}/manifests/demo-application.yaml"

    echo ""
    log_info "Demo application deployed. Check status:"
    echo "  kubectl --context kind-${HUB_CLUSTER} get application"
    echo "  kubectl --context kind-${HUB_CLUSTER} get managedcluster"
    echo ""
    log_info "For more demo scenarios, see manifests/ directory."
}

cmd_verify() {
    echo ""
    echo "=============================================="
    echo "  Kumquat Demo - Verification Tests"
    echo "=============================================="
    echo ""

    bash "${SCRIPT_DIR}/verify/verify.sh"
}

cmd_build() {
    check_prerequisites
    build_images
    log_info "Images built. Use './demo.sh up' to deploy."
}

print_access_info() {
    local hub_ctx="kind-${HUB_CLUSTER}"
    local hub_ip
    hub_ip=$(get_cluster_ip "${HUB_CLUSTER}")

    echo "Access Information:"
    echo ""
    echo "  Hub Cluster:"
    echo "    kubectl config use-context kind-${HUB_CLUSTER}"
    echo "    Portal API:  http://localhost:${PORTAL_NODEPORT}"
    echo "    Tunnel:      wss://localhost:${TUNNEL_NODEPORT}/connect"
    echo ""
    echo "  Sub Clusters:"
    for sub in "${SUB_CLUSTERS[@]}"; do
        echo "    kubectl config use-context kind-${sub}"
    done
    echo ""
    echo "  Demo Commands:"
    echo "    ./demo.sh demo     # Deploy demo applications"
    echo "    ./demo.sh verify   # Run verification tests"
    echo "    ./demo.sh status   # Show cluster status"
    echo ""
}

# ======================== Entry Point ========================

usage() {
    echo "Kumquat Demo One-Click Deployment Script"
    echo ""
    echo "Usage: $0 <command>"
    echo ""
    echo "Commands:"
    echo "  up       Create all resources (build images + create clusters + deploy components)"
    echo "  down     Clean up all resources"
    echo "  status   Show deployment status"
    echo "  demo     Deploy demo applications"
    echo "  verify   Run verification tests"
    echo "  build    Build images only"
    echo ""
}

case "${1:-}" in
    up)     cmd_up ;;
    down)   cmd_down ;;
    status) cmd_status ;;
    demo)   cmd_demo ;;
    verify) cmd_verify ;;
    build)  cmd_build ;;
    *)      usage; exit 1 ;;
esac
