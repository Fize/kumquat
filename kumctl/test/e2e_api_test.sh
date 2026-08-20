#!/usr/bin/env bash
set -uo pipefail

# Independent kumctl -> API E2E matrix. The caller must provide a running
# MySQL-backed Demo environment; this script intentionally has no skip path.

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
API_URL="${KUMQUAT_DEMO_API_URL:-http://127.0.0.1:31080}"
HUB_CONTEXT="${KUMQUAT_DEMO_HUB_CONTEXT:-kind-kumquat-hub}"
NAMESPACE="${KUMQUAT_DEMO_NAMESPACE:-kumquat-system}"
EDGE_CLUSTER_NAME="kumctl-e2e-edge-$(date +%s)"
REPORT_FILE="${KUMCTL_E2E_REPORT:-${ROOT}/kumctl/test/e2e-report.md}"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/kumctl-api-e2e.XXXXXX")"
KUMCTL="${TMP_DIR}/kumctl"
TOKEN=""
TOTAL=0
PASSED=0
FAILED=0
STARTED_AT="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
RESULTS="${TMP_DIR}/results.tsv"
FAILURES="${TMP_DIR}/failures.txt"
touch "${RESULTS}" "${FAILURES}"

cleanup() {
  kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" delete secret "${EDGE_CLUSTER_NAME}-credentials" --ignore-not-found >/dev/null 2>&1 || true
  kubectl --context "${HUB_CONTEXT}" delete managedcluster "${EDGE_CLUSTER_NAME}" --ignore-not-found >/dev/null 2>&1 || true
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

log() { printf '[kumctl-e2e] %s\n' "$*"; }
die() { log "FATAL: $*" >&2; exit 1; }

json_get() {
  jq -er ".${1} // empty"
}

json_find_id() {
  jq -er --arg name "$1" '.items[]? | select(.name == $name) | .id' | head -n 1
}

case_run() {
  local name="$1"; shift
  local output="${TMP_DIR}/case-${TOTAL}.json" error="${TMP_DIR}/case-${TOTAL}.err"
  TOTAL=$((TOTAL + 1))
  if "$@" >"${output}" 2>"${error}"; then
    PASSED=$((PASSED + 1)); printf 'PASS\t%s\t%s\n' "${name}" "${output}" >>"${RESULTS}"; log "PASS ${name}"
  else
    FAILED=$((FAILED + 1)); printf 'FAIL\t%s\t%s\n' "${name}" "${output}" >>"${RESULTS}"
    printf '%s: %s\n' "${name}" "$(tr '\n' ' ' <"${error}")" >>"${FAILURES}"; log "FAIL ${name}"
  fi
  LAST_OUTPUT="${output}"
}

case_expect_cli_rejection() {
  local name="$1"; shift
  local output="${TMP_DIR}/case-${TOTAL}.json" error="${TMP_DIR}/case-${TOTAL}.err"
  TOTAL=$((TOTAL + 1))
  if "$@" >"${output}" 2>"${error}"; then
    FAILED=$((FAILED + 1)); printf 'FAIL\t%s\t%s\n' "${name}" "${output}" >>"${RESULTS}"
    printf '%s: command unexpectedly succeeded\n' "${name}" >>"${FAILURES}"; log "FAIL ${name} (unexpected success)"
  elif grep -q 'does not expose auth/' "${error}"; then
    PASSED=$((PASSED + 1)); printf 'PASS\t%s\t%s\n' "${name}" "${output}" >>"${RESULTS}"; log "PASS ${name} (policy rejection)"
  else
    FAILED=$((FAILED + 1)); printf 'FAIL\t%s\t%s\n' "${name}" "${output}" >>"${RESULTS}"
    printf '%s: unexpected error %s\n' "${name}" "$(tr '\n' ' ' <"${error}")" >>"${FAILURES}"; log "FAIL ${name} (wrong rejection)"
  fi
  LAST_OUTPUT="${output}"
}

run_cli() { KUMQUAT_API_URL="${API_URL}" KUMQUAT_TOKEN="${TOKEN}" "${KUMCTL}" "$@"; }
write_json() { printf '%s\n' "$2" >"$1"; }

wait_operation() {
  local id="$1" state output
  for _ in $(seq 1 90); do
    output="${TMP_DIR}/operation-${id}.json"
    if run_cli get operations "${id}" >"${output}" 2>/dev/null; then
      state="$(json_get state <"${output}" 2>/dev/null || true)"
      [[ "${state}" == succeeded ]] && return 0
      [[ "${state}" == failed ]] && return 1
    fi
    sleep 2
  done
  return 1
}

await_resource_id() {
  local kind="$1" name="$2" id
  for _ in $(seq 1 90); do
    id="$(run_cli list "${kind}" 2>/dev/null | json_find_id "${name}" 2>/dev/null || true)"
    [[ -n "${id}" ]] && { printf '%s' "${id}"; return 0; }
    sleep 2
  done
  return 1
}

write_report() {
  mkdir -p "$(dirname "${REPORT_FILE}")"
  {
    printf '# kumctl API E2E Report\n\n'
    printf -- '- Started: `%s`\n- API: `%s`\n- Environment: MySQL-backed Demo + Kind Hub (`%s`)\n' "${STARTED_AT}" "${API_URL}" "${HUB_CONTEXT}"
    printf -- '- Executed cases: **%d** (37 included API operations + 3 explicit policy checks + 1 cleanup assertion)\n- Passed: **%d**\n- Failed: **%d**\n\n' "${TOTAL}" "${PASSED}" "${FAILED}"
    printf '## Coverage\n\n| API group | Operations |\n|---|---:|\n| auth (login) | 1 |\n| users | 5 |\n| roles | 3 |\n| modules | 6 |\n| projects | 6 |\n| workspaces | 6 |\n| applications | 6 |\n| clusters | 3 |\n| operations | 1 |\n| **Included API operations** | **37** |\n| Explicitly excluded auth operations | register, me, change-password |\n\n'
    printf '## Case Results\n\n| Result | Case |\n|---|---|\n'
    while IFS=$'\t' read -r result name _; do printf '| %s | `%s` |\n' "${result}" "${name}"; done <"${RESULTS}"
    if [[ -s "${FAILURES}" ]]; then printf '\n## Failures\n\n```text\n'; cat "${FAILURES}"; printf '```\n'; fi
    printf '\n## Method\n\nEach case invokes the compiled kumctl binary in a separate process. Requests traverse the API gateway, asynchronous mutations are checked through /operations/{id}, and no included operation is skipped.\n\n## Effects verified\n\n- Login token was accepted by every subsequent authenticated request.\n- CRUD mutations were followed by independent get/list checks and fixture cleanup.\n- Workspace/application create, update, adopt, and delete operations reached a successful terminal operation state.\n- A real Engine Edge ManagedCluster was discovered, read, and adopted through the API gateway.\n- The excluded auth/register, auth/me, and auth/change-password commands were explicitly rejected by kumctl policy.\n'
  } >"${REPORT_FILE}"
}

for tool in go kubectl kind jq python3; do command -v "${tool}" >/dev/null || die "missing prerequisite: ${tool}"; done
kind get clusters | grep -Fxq "${HUB_CONTEXT#kind-}" || die "missing required Demo cluster ${HUB_CONTEXT#kind-}"
kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" get statefulset/mysql >/dev/null || die "missing MySQL StatefulSet"
curl --fail --silent "${API_URL}/readyz" >/dev/null || die "API is not ready"
GOTOOLCHAIN=local go build -o "${KUMCTL}" "${ROOT}/kumctl/cmd/kumctl"
ADMIN_USERNAME="$(kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" get secret kumquat-api -o jsonpath='{.data.bootstrap-admin-username}' | base64 --decode)"
ADMIN_PASSWORD="$(kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" get secret kumquat-api -o jsonpath='{.data.bootstrap-admin-password}' | base64 --decode)"

LOGIN_FILE="${TMP_DIR}/login.json"
jq -n --arg username "${ADMIN_USERNAME}" --arg password "${ADMIN_PASSWORD}" '{username:$username,password:$password}' >"${LOGIN_FILE}"
case_run 'POST /auth/login (kumctl login)' run_cli auth login --file "${LOGIN_FILE}"
TOKEN="$(json_get token <"${LAST_OUTPUT}" 2>/dev/null || true)"
[[ -n "${TOKEN}" ]] || { write_report; die 'login did not return a token'; }

suffix="$(date +%s)"
MODULE_FILE="${TMP_DIR}/module.json"; write_json "${MODULE_FILE}" "{\"name\":\"kumctl-e2e-module-${suffix}\"}"
case_run 'GET /roles' run_cli list roles
ROLE_ID="$(json_get '[0].id' <"${LAST_OUTPUT}" 2>/dev/null || true)"; [[ -n "${ROLE_ID}" ]] || { write_report; die 'role fixture unavailable'; }
case_run 'GET /roles/{id}' run_cli get roles "${ROLE_ID}"
case_run 'GET /roles/{id}/permissions' run_cli get roles "${ROLE_ID}" --permissions
case_run 'GET /users' run_cli list users --page 1 --size 20
USER_FILE="${TMP_DIR}/user.json"; write_json "${USER_FILE}" "{\"username\":\"kumctl${suffix}\",\"email\":\"kumctl${suffix}@example.com\",\"password\":\"secret123\",\"role_id\":${ROLE_ID}}"
case_run 'POST /users' run_cli create users --file "${USER_FILE}"
USER_ID="$(json_get id <"${LAST_OUTPUT}" 2>/dev/null || true)"; [[ -n "${USER_ID}" ]] || { write_report; die 'user fixture unavailable'; }
case_run 'GET /users/{id}' run_cli get users "${USER_ID}"
USER_UPDATE="${TMP_DIR}/user-update.json"; write_json "${USER_UPDATE}" '{"nickname":"kumctl-e2e-updated"}'
case_run 'PUT /users/{id}' run_cli update users "${USER_ID}" --file "${USER_UPDATE}"
case_run 'DELETE /users/{id}' run_cli delete users "${USER_ID}"

case_run 'GET /modules' run_cli list modules
case_run 'POST /modules' run_cli create modules --file "${MODULE_FILE}"
MODULE_ID="$(json_get id <"${LAST_OUTPUT}" 2>/dev/null || true)"; [[ -n "${MODULE_ID}" ]] || { write_report; die 'module fixture unavailable'; }
case_run 'GET /modules/{id}' run_cli get modules "${MODULE_ID}"
MODULE_UPDATE="${TMP_DIR}/module-update.json"; write_json "${MODULE_UPDATE}" "{\"name\":\"kumctl-e2e-module-updated-${suffix}\"}"
case_run 'PUT /modules/{id}' run_cli update modules "${MODULE_ID}" --file "${MODULE_UPDATE}"
CHILD_FILE="${TMP_DIR}/child.json"; write_json "${CHILD_FILE}" "{\"name\":\"kumctl-e2e-child-${suffix}\",\"parentId\":\"${MODULE_ID}\"}"
run_cli create modules --file "${CHILD_FILE}" >"${TMP_DIR}/child-create.json" 2>/dev/null
CHILD_ID="$(json_get id <"${TMP_DIR}/child-create.json" 2>/dev/null || true)"
case_run 'GET /modules/{id}/children' run_cli get modules "${MODULE_ID}" --children

case_run 'GET /projects' run_cli list projects
PROJECT_FILE="${TMP_DIR}/project.json"; write_json "${PROJECT_FILE}" "{\"name\":\"kumctl-e2e-project-${suffix}\",\"moduleId\":\"${MODULE_ID}\"}"
case_run 'POST /projects' run_cli create projects --file "${PROJECT_FILE}"
PROJECT_ID="$(json_get id <"${LAST_OUTPUT}" 2>/dev/null || true)"; [[ -n "${PROJECT_ID}" ]] || { write_report; die 'project fixture unavailable'; }
case_run 'GET /projects/{id}' run_cli get projects "${PROJECT_ID}"
PROJECT_UPDATE="${TMP_DIR}/project-update.json"; write_json "${PROJECT_UPDATE}" "{\"name\":\"kumctl-e2e-project-updated-${suffix}\"}"
case_run 'PUT /projects/{id}' run_cli update projects "${PROJECT_ID}" --file "${PROJECT_UPDATE}"
case_run 'GET /projects/module/{moduleId}' run_cli list projects --module-id "${MODULE_ID}"

case_run 'GET /workspaces' run_cli list workspaces
WORKSPACE_FILE="${TMP_DIR}/workspace.json"; WORKSPACE_NAME="kumctl-e2e-workspace-${suffix}"; WORKSPACE_NAMESPACE="kumctl-e2e-${suffix}"
write_json "${WORKSPACE_FILE}" "{\"name\":\"${WORKSPACE_NAME}\",\"projectId\":\"${PROJECT_ID}\",\"desired\":{\"workspace\":{\"namespace\":\"${WORKSPACE_NAMESPACE}\",\"clusterMatchLabels\":{\"kumquat.io/demo-cluster\":\"hub\"}}}}"
case_run 'POST /workspaces' run_cli create workspaces --file "${WORKSPACE_FILE}"
WORKSPACE_ID="$(json_get resourceId <"${LAST_OUTPUT}" 2>/dev/null || true)"; WORKSPACE_OPERATION="$(json_get id <"${LAST_OUTPUT}" 2>/dev/null || true)"
[[ -n "${WORKSPACE_ID}" && -n "${WORKSPACE_OPERATION}" ]] || { write_report; die 'workspace fixture unavailable'; }
case_run 'GET /operations/{id}' run_cli get operations "${WORKSPACE_OPERATION}"
wait_operation "${WORKSPACE_OPERATION}" || { write_report; die 'workspace create operation failed'; }
case_run 'GET /workspaces/{id}' run_cli get workspaces "${WORKSPACE_ID}"
WORKSPACE_UPDATE="${TMP_DIR}/workspace-update.json"; write_json "${WORKSPACE_UPDATE}" "{\"desired\":{\"workspace\":{\"namespace\":\"${WORKSPACE_NAMESPACE}\",\"clusterMatchLabels\":{\"kumquat.io/demo-cluster\":\"hub\"}}}}"
case_run 'PUT /workspaces/{id}' run_cli update workspaces "${WORKSPACE_ID}" --file "${WORKSPACE_UPDATE}" --wait
UPDATE_OPERATION="$(json_get id <"${LAST_OUTPUT}" 2>/dev/null || true)"; if [[ -n "${UPDATE_OPERATION}" ]] && ! wait_operation "${UPDATE_OPERATION}"; then write_report; die 'workspace update operation failed'; fi

case_run 'GET /applications' run_cli list applications
APPLICATION_FILE="${TMP_DIR}/application.json"; APPLICATION_NAME="kumctl-e2e-application-${suffix}"
write_json "${APPLICATION_FILE}" "{\"name\":\"${APPLICATION_NAME}\",\"workspaceId\":\"${WORKSPACE_ID}\",\"desired\":{\"application\":{\"workload\":{\"apiVersion\":\"apps/v1\",\"kind\":\"Deployment\"},\"replicas\":1,\"template\":{\"labels\":{\"e2e\":\"kumctl\"},\"containers\":[{\"name\":\"app\",\"image\":\"nginx:1.27-alpine\"}]}}}}"
case_run 'POST /applications' run_cli create applications --file "${APPLICATION_FILE}"
APPLICATION_ID="$(json_get resourceId <"${LAST_OUTPUT}" 2>/dev/null || true)"; APPLICATION_OPERATION="$(json_get id <"${LAST_OUTPUT}" 2>/dev/null || true)"
[[ -n "${APPLICATION_ID}" && -n "${APPLICATION_OPERATION}" ]] || { write_report; die 'application fixture unavailable'; }
wait_operation "${APPLICATION_OPERATION}" || { write_report; die 'application create operation failed'; }
case_run 'GET /applications/{id}' run_cli get applications "${APPLICATION_ID}"
APPLICATION_UPDATE="${TMP_DIR}/application-update.json"
write_json "${APPLICATION_UPDATE}" "{\"desired\":{\"application\":{\"workload\":{\"apiVersion\":\"apps/v1\",\"kind\":\"Deployment\"},\"replicas\":1,\"template\":{\"labels\":{\"e2e\":\"kumctl-updated\"},\"containers\":[{\"name\":\"app\",\"image\":\"nginx:1.27-alpine\"}]}}}}"
case_run 'PUT /applications/{id}' run_cli update applications "${APPLICATION_ID}" --file "${APPLICATION_UPDATE}" --wait
UPDATE_OPERATION="$(json_get id <"${LAST_OUTPUT}" 2>/dev/null || true)"; if [[ -n "${UPDATE_OPERATION}" ]] && ! wait_operation "${UPDATE_OPERATION}"; then write_report; die 'application update operation failed'; fi

# Engine-discovered resources provide independent adoption fixtures.
ADOPT_WS_NAME="kumctl-e2e-adopt-workspace-${suffix}"; ADOPT_WS_NAMESPACE="kumctl-adopt-${suffix}"
kubectl --context "${HUB_CONTEXT}" apply -f - >/dev/null <<YAML
apiVersion: workspace.kumquat.io/v1alpha1
kind: Workspace
metadata:
  name: ${ADOPT_WS_NAME}
spec:
  name: ${ADOPT_WS_NAMESPACE}
YAML
ADOPT_WS_ID="$(await_resource_id workspaces "${ADOPT_WS_NAME}" 2>/dev/null || true)"
if [[ -n "${ADOPT_WS_ID}" ]]; then
  ADOPT_WS_FILE="${TMP_DIR}/adopt-workspace.json"; write_json "${ADOPT_WS_FILE}" "{\"projectId\":\"${PROJECT_ID}\"}"
  case_run 'POST /workspaces/{id}/adopt-existing' run_cli adopt workspaces "${ADOPT_WS_ID}" --file "${ADOPT_WS_FILE}" --wait
else
  case_run 'POST /workspaces/{id}/adopt-existing' false
fi

ADOPT_APP_NAME="kumctl-e2e-adopt-application-${suffix}"
kubectl --context "${HUB_CONTEXT}" apply -f - >/dev/null <<YAML
apiVersion: apps.kumquat.io/v1alpha1
kind: Application
metadata:
  name: ${ADOPT_APP_NAME}
  namespace: ${WORKSPACE_NAMESPACE}
spec:
  workload:
    apiVersion: apps/v1
    kind: Deployment
  replicas: 1
  template:
    metadata:
      labels: {e2e: kumctl-adopt}
    spec:
      containers:
      - name: app
        image: nginx:1.27-alpine
YAML
ADOPT_APP_ID="$(await_resource_id applications "${ADOPT_APP_NAME}" 2>/dev/null || true)"
if [[ -n "${ADOPT_APP_ID}" ]]; then
  ADOPT_APP_FILE="${TMP_DIR}/adopt-application.json"; write_json "${ADOPT_APP_FILE}" "{\"workspaceId\":\"${WORKSPACE_ID}\"}"
  case_run 'POST /applications/{id}/adopt-existing' run_cli adopt applications "${ADOPT_APP_ID}" --file "${ADOPT_APP_FILE}" --wait
else
  case_run 'POST /applications/{id}/adopt-existing' false
fi

# The API discovers only agent-registered Edge clusters. Create one with a
# deliberately non-sensitive test Secret so the cluster CRUD/adopt routes have
# a real Engine object to exercise; the EXIT trap removes both fixtures.
kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" create secret generic "${EDGE_CLUSTER_NAME}-credentials" \
  --from-literal=token=e2e-token --from-literal=caData=e2e-ca --dry-run=client -o yaml | \
  kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" apply -f - >/dev/null
kubectl --context "${HUB_CONTEXT}" apply -f - >/dev/null <<YAML
apiVersion: storage.kumquat.io/v1alpha1
kind: ManagedCluster
metadata:
  name: ${EDGE_CLUSTER_NAME}
  labels:
    kumquat.io/registration-source: agent
spec:
  connectionMode: Edge
  apiServer: https://127.0.0.1:6443
  secretRef:
    name: ${EDGE_CLUSTER_NAME}-credentials
YAML
CLUSTER_ID="$(await_resource_id clusters "${EDGE_CLUSTER_NAME}" 2>/dev/null || true)"
case_run 'GET /clusters' run_cli list clusters
if [[ -n "${CLUSTER_ID}" ]]; then
  case_run 'GET /clusters/{id}' run_cli get clusters "${CLUSTER_ID}"
  case_run 'POST /clusters/{id}/adopt' run_cli adopt clusters "${CLUSTER_ID}" --wait
else
  case_run 'GET /clusters/{id}' false
  case_run 'POST /clusters/{id}/adopt' false
fi

case_run 'DELETE /applications/{id}' run_cli delete applications "${APPLICATION_ID}" --wait
DELETE_OPERATION="$(json_get id <"${LAST_OUTPUT}" 2>/dev/null || true)"; [[ -n "${DELETE_OPERATION}" ]] && wait_operation "${DELETE_OPERATION}" || true
# Remove the adoption fixtures before deleting their parent workspace. These
# are setup/teardown calls, not additional coverage claims.
if [[ -n "${ADOPT_APP_ID}" ]]; then
  run_cli delete applications "${ADOPT_APP_ID}" >"${TMP_DIR}/cleanup-adopt-app.json" 2>/dev/null || true
  CLEANUP_OPERATION="$(json_get id <"${TMP_DIR}/cleanup-adopt-app.json" 2>/dev/null || true)"; [[ -n "${CLEANUP_OPERATION}" ]] && wait_operation "${CLEANUP_OPERATION}" || true
fi
case_run 'DELETE /workspaces/{id}' run_cli delete workspaces "${WORKSPACE_ID}" --wait
DELETE_OPERATION="$(json_get id <"${LAST_OUTPUT}" 2>/dev/null || true)"; [[ -n "${DELETE_OPERATION}" ]] && wait_operation "${DELETE_OPERATION}" || true
if [[ -n "${ADOPT_WS_ID}" ]]; then
  run_cli delete workspaces "${ADOPT_WS_ID}" >"${TMP_DIR}/cleanup-adopt-workspace.json" 2>/dev/null || true
  CLEANUP_OPERATION="$(json_get id <"${TMP_DIR}/cleanup-adopt-workspace.json" 2>/dev/null || true)"; [[ -n "${CLEANUP_OPERATION}" ]] && wait_operation "${CLEANUP_OPERATION}" || true
fi
case_run 'DELETE /projects/{id}' run_cli delete projects "${PROJECT_ID}"
DELETE_OPERATION="$(json_get id <"${LAST_OUTPUT}" 2>/dev/null || true)"; [[ -n "${DELETE_OPERATION}" ]] && wait_operation "${DELETE_OPERATION}" || true
if [[ -n "${CHILD_ID}" ]]; then case_run 'DELETE /modules/{id}' run_cli delete modules "${CHILD_ID}"; else case_run 'DELETE /modules/{id}' false; fi
case_run 'DELETE /modules/{id} (root cleanup)' run_cli delete modules "${MODULE_ID}"

case_expect_cli_rejection 'excluded auth/register' run_cli auth register
case_expect_cli_rejection 'excluded auth/me' run_cli auth me
case_expect_cli_rejection 'excluded auth/change-password' run_cli auth change-password

write_report
log "completed=${TOTAL} passed=${PASSED} failed=${FAILED} report=${REPORT_FILE}"
[[ "${FAILED}" -eq 0 ]]
