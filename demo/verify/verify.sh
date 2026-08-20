#!/usr/bin/env bash
set -euo pipefail

HUB_CONTEXT=kind-kumquat-hub
NAMESPACE=kumquat-system

for cluster in kumquat-hub; do
  kind get clusters | grep -Fxq "${cluster}"
done
kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" wait --for=condition=Ready pod/mysql-0 --timeout=120s
kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" rollout status deployment/engine-manager --timeout=120s
kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" rollout status deployment/engine-scheduler --timeout=120s
kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" rollout status deployment/engine-hub-agent --timeout=120s
kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" rollout status deployment/api --timeout=120s
curl --fail --silent http://127.0.0.1:31080/readyz
printf '\nAll required demo components are ready. Run ./demo.sh test for the strict business E2E.\n'
