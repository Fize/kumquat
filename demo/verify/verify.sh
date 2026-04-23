#!/usr/bin/env bash
# Kumquat Demo Verification Script
#
# Checks whether all components are running correctly.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEMO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

HUB_CLUSTER="kumquat-hub"
SUB_CLUSTERS=("kumquat-sub-1" "kumquat-sub-2")

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass=0
fail=0
warn=0

check() {
    local desc=$1
    local cmd=$2

    if eval "$cmd" 2>/dev/null; then
        echo -e "  ${GREEN}PASS${NC} ${desc}"
        ((pass++))
    else
        echo -e "  ${RED}FAIL${NC} ${desc}"
        ((fail++))
    fi
}

check_warn() {
    local desc=$1
    local cmd=$2

    if eval "$cmd" 2>/dev/null; then
        echo -e "  ${GREEN}PASS${NC} ${desc}"
        ((pass++))
    else
        echo -e "  ${YELLOW}WARN${NC} ${desc} (may take time)"
        ((warn++))
    fi
}

echo ""
echo "=============================================="
echo "  Kumquat Demo - Verification Tests"
echo "=============================================="
echo ""

# 1. Cluster Status
echo "--- Cluster Status ---"
for cluster in "${HUB_CLUSTER}" "${SUB_CLUSTERS[@]}"; do
    check "Cluster ${cluster} exists" \
        "kind get clusters 2>/dev/null | grep -q '^${cluster}$'"
done

echo ""

# 2. Hub Cluster Components
echo "--- Hub Cluster Components ---"
hub_ctx="kind-${HUB_CLUSTER}"

check "CRD applications.apps.kumquat.io established" \
    "kubectl --context ${hub_ctx} get crd applications.apps.kumquat.io -o jsonpath='{.status.conditions[?(@.type==\"Established\")].status}' 2>/dev/null | grep -q True"

check "CRD managedclusters.storage.kumquat.io established" \
    "kubectl --context ${hub_ctx} get crd managedclusters.storage.kumquat.io -o jsonpath='{.status.conditions[?(@.type==\"Established\")].status}' 2>/dev/null | grep -q True"

check "Namespace kumquat-system exists" \
    "kubectl --context ${hub_ctx} get namespace kumquat-system -o name 2>/dev/null | grep -q kumquat-system"

check "engine-manager Deployment available" \
    "kubectl --context ${hub_ctx} -n kumquat-system get deploy engine-manager -o jsonpath='{.status.availableReplicas}' 2>/dev/null | grep -v '^0$'"

check "engine-scheduler Deployment available" \
    "kubectl --context ${hub_ctx} -n kumquat-system get deploy engine-scheduler -o jsonpath='{.status.availableReplicas}' 2>/dev/null | grep -v '^0$'"

check "portal Deployment available" \
    "kubectl --context ${hub_ctx} -n portal get deploy portal -o jsonpath='{.status.availableReplicas}' 2>/dev/null | grep -v '^0$'"

echo ""

# 3. Sub Cluster Components
echo "--- Sub Cluster Components ---"
for sub in "${SUB_CLUSTERS[@]}"; do
    sub_ctx="kind-${sub}"
    check "engine-agent on ${sub} available" \
        "kubectl --context ${sub_ctx} -n kumquat-system get deploy engine-agent -o jsonpath='{.status.availableReplicas}' 2>/dev/null | grep -v '^0$'"
done

echo ""

# 4. ManagedCluster Registration
echo "--- Cluster Registration ---"
for sub in "${SUB_CLUSTERS[@]}"; do
    check_warn "ManagedCluster ${sub} registered" \
        "kubectl --context ${hub_ctx} get managedcluster ${sub} -o name 2>/dev/null | grep -q ${sub}"
done

check "ManagedCluster ${HUB_CLUSTER} registered" \
    "kubectl --context ${hub_ctx} get managedcluster ${HUB_CLUSTER} -o name 2>/dev/null | grep -q ${HUB_CLUSTER}"

echo ""

# 5. Addon Status
echo "--- Addon Status ---"
for sub in "${SUB_CLUSTERS[@]}"; do
    check_warn "mcs-lighthouse addon on ${sub}" \
        "kubectl --context ${hub_ctx} get managedcluster ${sub} -o jsonpath='{.spec.addons[?(@.name==\"mcs-lighthouse\")].enabled}' 2>/dev/null | grep -q true"
    check_warn "kruise-rollout addon on ${sub}" \
        "kubectl --context ${hub_ctx} get managedcluster ${sub} -o jsonpath='{.spec.addons[?(@.name==\"kruise-rollout\")].enabled}' 2>/dev/null | grep -q true"
    check_warn "victoriametrics addon on ${sub}" \
        "kubectl --context ${hub_ctx} get managedcluster ${sub} -o jsonpath='{.spec.addons[?(@.name==\"victoriametrics\")].enabled}' 2>/dev/null | grep -q true"
done

echo ""

# 6. Network Connectivity
echo "--- Network Connectivity ---"
hub_ip=$(docker inspect "${HUB_CLUSTER}-control-plane" \
    --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' 2>/dev/null | head -1)
sub1_ip=$(docker inspect "${SUB_CLUSTERS[0]}-control-plane" \
    --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' 2>/dev/null | head -1)

if [[ -n "$hub_ip" && -n "$sub1_ip" ]]; then
    check "Hub <-> Sub-1 network reachable" \
        "docker exec ${HUB_CLUSTER}-control-plane ping -c 1 -W 2 ${sub1_ip}"
else
    echo -e "  ${YELLOW}WARN${NC} Cannot determine cluster IPs for network test"
    ((warn++))
fi

echo ""

# 7. Portal Health Check
echo "--- Portal Health Check ---"
check "Portal API reachable" \
    "curl -s -o /dev/null -w '%{http_code}' http://localhost:30080/healthz 2>/dev/null | grep -q '200\|404'"

echo ""

# Summary
echo "=============================================="
echo -e "  ${GREEN}PASS${NC}: ${pass}  ${RED}FAIL${NC}: ${fail}  ${YELLOW}WARN${NC}: ${warn}"
echo "=============================================="

if [[ $fail -gt 0 ]]; then
    echo ""
    echo "Some checks failed. This may be because:"
    echo "  1. Addon components are still being installed (wait a few minutes)"
    echo "  2. Pod scheduling issues (check 'kubectl get events')"
    echo "  3. Image pull issues (check 'kubectl describe pod')"
    exit 1
fi
