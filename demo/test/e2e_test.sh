#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
API_URL="${KUMQUAT_DEMO_API_URL:-http://127.0.0.1:31080}"
HUB_CONTEXT=kind-kumquat-hub
NAMESPACE=kumquat-system
TMP_DIR="$(mktemp -d)"
BODY_FILE="${TMP_DIR}/response.json"
AUTH_CONFIG="${TMP_DIR}/curl-auth.conf"
KUMCTL="${TMP_DIR}/kumctl"
TOKEN=""

cleanup() { rm -rf "${TMP_DIR}"; }
trap cleanup EXIT
log() { printf '[e2e] %s\n' "$*"; }
fail() { printf '[e2e] FAIL: %s\n' "$*" >&2; exit 1; }

json_get() {
  python3 -c 'import json,sys
value=json.load(sys.stdin)
for key in sys.argv[1].split("."):
    value=value[int(key)] if isinstance(value,list) else value[key]
print(value if not isinstance(value,(dict,list)) else json.dumps(value))' "$1"
}

secret_value() {
  kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" get secret kumquat-api \
    -o "jsonpath={.data.$1}" | base64 --decode
}

request() {
  local method=$1 path=$2 data=${3:-} expected=${4:-200}
  local args=(--silent --show-error --output "${BODY_FILE}" --write-out '%{http_code}' --request "${method}")
  [[ -n "${TOKEN}" ]] && args+=(--config "${AUTH_CONFIG}")
  if [[ -n "${data}" ]]; then
    args+=(--header 'Content-Type: application/json' --data-binary @-)
  fi
  local status
  if [[ -n "${data}" ]]; then
    status="$(printf '%s' "${data}" | curl "${args[@]}" "${API_URL}${path}")" || fail "${method} ${path} connection failed"
  else
    status="$(curl "${args[@]}" "${API_URL}${path}")" || fail "${method} ${path} connection failed"
  fi
  [[ "${status}" == "${expected}" ]] || fail "${method} ${path}: expected HTTP ${expected}, got ${status}: $(cat "${BODY_FILE}")"
}

wait_for_resource() {
  local kind=$1 name=$2 namespace=${3:-} attempt
  for attempt in $(seq 1 90); do
    if [[ -n "${namespace}" ]]; then
      kubectl --context "${HUB_CONTEXT}" -n "${namespace}" get "${kind}" "${name}" >/dev/null 2>&1 && return
    else
      kubectl --context "${HUB_CONTEXT}" get "${kind}" "${name}" >/dev/null 2>&1 && return
    fi
    sleep 2
  done
  fail "timed out waiting for ${kind}/${name} projection"
}

wait_operation() {
  local operation_id=$1 output=$2 attempt state
  for attempt in $(seq 1 90); do
    "${KUMCTL}" --server "${API_URL}" --token "${TOKEN}" get operations "${operation_id}" >"${output}"
    state="$(json_get state <"${output}")"
    [[ "${state}" == succeeded ]] && return
    [[ "${state}" == failed ]] && fail "operation ${operation_id} failed: $(cat "${output}")"
    sleep 2
  done
  fail "operation ${operation_id} timed out"
}

for tool in curl kubectl kind go python3; do command -v "${tool}" >/dev/null || fail "missing ${tool}"; done
for cluster in kumquat-hub; do
  kind get clusters | grep -Fxq "${cluster}" || fail "missing demo cluster ${cluster}"
done
curl --fail --silent "${API_URL}/readyz" >/dev/null || fail "API is not ready"
kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" wait --for=condition=Ready pod/mysql-0 --timeout=180s
kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" rollout status deployment/engine-manager --timeout=180s
kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" rollout status deployment/engine-scheduler --timeout=180s
kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" rollout status deployment/engine-hub-agent --timeout=180s
agent_args="$(kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" get deployment engine-hub-agent -o jsonpath='{.spec.template.spec.containers[0].args}')"
[[ "${agent_args}" != *bootstrap-token* ]] || fail "Hub agent exposes bootstrap token through Pod arguments"
[[ -z "$(kubectl --context "${HUB_CONTEXT}" get managedcluster kumquat-hub -o jsonpath='{.spec.secretRef.name}')" ]] || fail "local Hub cluster unexpectedly uses a credential Secret"
[[ "$(kubectl --context "${HUB_CONTEXT}" get managedcluster kumquat-hub -o jsonpath='{.spec.apiServer}')" == "in-cluster://" ]] || fail "local Hub cluster lacks the explicit in-cluster marker"

ADMIN_USERNAME="$(secret_value bootstrap-admin-username)"
ADMIN_PASSWORD="$(secret_value bootstrap-admin-password)"
request POST /api/v1/auth/login "{\"username\":\"${ADMIN_USERNAME}\",\"password\":\"${ADMIN_PASSWORD}\"}" 200
TOKEN="$(json_get data.token <"${BODY_FILE}")"
[[ -n "${TOKEN}" ]] || fail "bootstrap admin login returned no token"
umask 077
printf 'header = "Authorization: Bearer %s"\n' "${TOKEN}" >"${AUTH_CONFIG}"
log "authenticated as bootstrap admin"

suffix="$(date +%s)"
request POST /api/v1/modules "{\"name\":\"e2e-module-${suffix}\"}" 200
MODULE_ID="$(json_get data.id <"${BODY_FILE}")"
request POST /api/v1/projects "{\"name\":\"e2e-project-${suffix}\",\"moduleId\":\"${MODULE_ID}\"}" 200
PROJECT_ID="$(json_get data.id <"${BODY_FILE}")"

go build -o "${KUMCTL}" "${PROJECT_ROOT}/kumctl/cmd/kumctl"
cat >"${TMP_DIR}/workspace.json" <<JSON
{"name":"e2e-workspace-${suffix}","projectId":"${PROJECT_ID}","desired":{"workspace":{"namespace":"e2e-${suffix}","clusterMatchLabels":{"kumquat.io/demo-cluster":"hub"}}}}
JSON
"${KUMCTL}" --server "${API_URL}" --token "${TOKEN}" --file "${TMP_DIR}/workspace.json" create workspaces >"${TMP_DIR}/workspace-accepted.json"
WORKSPACE_ID="$(json_get resourceId <"${TMP_DIR}/workspace-accepted.json")"
wait_operation "$(json_get id <"${TMP_DIR}/workspace-accepted.json")" "${TMP_DIR}/workspace-operation.json"
"${KUMCTL}" --server "${API_URL}" --token "${TOKEN}" get workspaces "${WORKSPACE_ID}" >"${TMP_DIR}/workspace-record.json"
[[ "$(json_get projectId <"${TMP_DIR}/workspace-record.json")" == "${PROJECT_ID}" ]] || fail "workspace project relation mismatch"
wait_for_resource workspace "e2e-workspace-${suffix}"
wait_for_resource namespace "e2e-${suffix}"

cat >"${TMP_DIR}/application.json" <<JSON
{"name":"e2e-application-${suffix}","workspaceId":"${WORKSPACE_ID}","desired":{"application":{"workload":{"apiVersion":"apps/v1","kind":"Deployment"},"replicas":1,"template":{"labels":{"demo":"strict-e2e"},"containers":[{"name":"app","image":"nginx:1.27-alpine"}]}}}}
JSON
"${KUMCTL}" --server "${API_URL}" --token "${TOKEN}" --file "${TMP_DIR}/application.json" create applications >"${TMP_DIR}/application-accepted.json"
APPLICATION_ID="$(json_get resourceId <"${TMP_DIR}/application-accepted.json")"
wait_operation "$(json_get id <"${TMP_DIR}/application-accepted.json")" "${TMP_DIR}/application-operation.json"
"${KUMCTL}" --server "${API_URL}" --token "${TOKEN}" get applications "${APPLICATION_ID}" >"${TMP_DIR}/application-record.json"
[[ "$(json_get parentId <"${TMP_DIR}/application-record.json")" == "${WORKSPACE_ID}" ]] || fail "application workspace relation mismatch"

wait_for_resource application "e2e-application-${suffix}" "e2e-${suffix}"
[[ "$(kubectl --context "${HUB_CONTEXT}" get workspace "e2e-workspace-${suffix}" -o jsonpath='{.metadata.labels.kumquat\.io/workspace-id}')" == "${WORKSPACE_ID}" ]] || fail "Engine Workspace identity label mismatch"
[[ "$(kubectl --context "${HUB_CONTEXT}" -n "e2e-${suffix}" get application "e2e-application-${suffix}" -o jsonpath='{.metadata.labels.kumquat\.io/application-id}')" == "${APPLICATION_ID}" ]] || fail "Engine Application identity label mismatch"
[[ "$(kubectl --context "${HUB_CONTEXT}" -n "e2e-${suffix}" get application "e2e-application-${suffix}" -o jsonpath='{.metadata.labels.kumquat\.io/workspace-id}')" == "${WORKSPACE_ID}" ]] || fail "Engine Application workspace label mismatch"

wait_for_resource deployment "e2e-application-${suffix}" "e2e-${suffix}"
selector_id="$(kubectl --context "${HUB_CONTEXT}" -n "e2e-${suffix}" get deployment "e2e-application-${suffix}" -o jsonpath='{.spec.selector.matchLabels.kumquat\.io/application-id}')"
template_id="$(kubectl --context "${HUB_CONTEXT}" -n "e2e-${suffix}" get deployment "e2e-application-${suffix}" -o jsonpath='{.spec.template.metadata.labels.kumquat\.io/application-id}')"
[[ "${selector_id}" == "${APPLICATION_ID}" && "${template_id}" == "${APPLICATION_ID}" ]] || fail "immutable workload identity labels mismatch"
for mutable in kumquat.io/module-id kumquat.io/project-id; do
  selector_and_template="$(kubectl --context "${HUB_CONTEXT}" -n "e2e-${suffix}" get deployment "e2e-application-${suffix}" -o jsonpath='{.spec.selector.matchLabels}{.spec.template.metadata.labels}')"
  [[ "${selector_and_template}" != *"${mutable}"* ]] || fail "mutable ownership label leaked into workload selector/template"
done

table_count="$(kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" exec mysql-0 -- sh -c 'MYSQL_PWD="$MYSQL_PASSWORD" mysql -u"$MYSQL_USER" -D"$MYSQL_DATABASE" -Nse "SELECT COUNT(*) FROM resource_records"')"
[[ "${table_count}" -ge 2 ]] || fail "MySQL does not contain API business resources"
api_sql_type="$(kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" get deploy api -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="KUMQUAT_API_SQL_TYPE")].value}')"
[[ "${api_sql_type}" == mysql ]] || fail "API deployment is not configured for MySQL"

heartbeat_before="$(kubectl --context "${HUB_CONTEXT}" get managedcluster kumquat-hub -o jsonpath='{.status.lastKeepAliveTime}')"
kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" rollout restart deployment/engine-manager deployment/engine-hub-agent >/dev/null
kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" rollout status deployment/engine-manager --timeout=180s
kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" rollout status deployment/engine-hub-agent --timeout=180s
heartbeat_after="${heartbeat_before}"
for _ in $(seq 1 30); do
  heartbeat_after="$(kubectl --context "${HUB_CONTEXT}" get managedcluster kumquat-hub -o jsonpath='{.status.lastKeepAliveTime}')"
  [[ -n "${heartbeat_after}" && "${heartbeat_after}" != "${heartbeat_before}" ]] && break
  sleep 2
done
[[ -n "${heartbeat_after}" && "${heartbeat_after}" != "${heartbeat_before}" ]] || fail "Hub heartbeat did not resume after manager/agent restart"
wait_for_resource deployment "e2e-application-${suffix}" "e2e-${suffix}"

kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" rollout restart deployment/api >/dev/null
kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" rollout status deployment/api --timeout=180s
api_ready=false
for _ in $(seq 1 30); do
  if curl --fail --silent "${API_URL}/readyz" >/dev/null; then
    api_ready=true
    break
  fi
  sleep 2
done
[[ "${api_ready}" == true ]] || fail "API not ready after restart"
request POST /api/v1/auth/login "{\"username\":\"${ADMIN_USERNAME}\",\"password\":\"${ADMIN_PASSWORD}\"}" 200
TOKEN="$(json_get data.token <"${BODY_FILE}")"
printf 'header = "Authorization: Bearer %s"\n' "${TOKEN}" >"${AUTH_CONFIG}"
for target in "modules/${MODULE_ID}" "projects/${PROJECT_ID}" "workspaces/${WORKSPACE_ID}" "applications/${APPLICATION_ID}"; do
  request GET "/api/v1/${target}" "" 200
done

log "PASS: authenticated business flow, Engine projection, immutable labels, MySQL persistence, token-free Engine restart, and API restart"
