#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
assert_absent() {
  local needle=$1
  local content=$2
  if grep -Fq -- "${needle}" <<<"${content}"; then
    printf 'unexpected rendered content: %s\n' "${needle}" >&2
    exit 1
  fi
}

output="$(helm template engine-manager "${ROOT}/charts/manager")"
assert_absent 'name: kumquat:agent' "${output}"
assert_absent 'system:bootstrappers:engine-agent' "${output}"
assert_absent 'tunnel.external.example.test' "${output}"
certificate_output="$(helm template engine-manager "${ROOT}/charts/manager" \
  --set 'certificate.externalDNSNames[0]=tunnel.external.example.test' \
  --set 'certificate.externalIPAddresses[0]=192.0.2.10')"
grep -Fq -- '- "tunnel.external.example.test"' <<<"${certificate_output}"
grep -Fq 'ipAddresses:' <<<"${certificate_output}"
grep -Fq -- '- "192.0.2.10"' <<<"${certificate_output}"

agent="$(helm template engine-agent "${ROOT}/charts/agent" \
  --set clustername=edge \
  --set manager.master=https://hub.example.test:6443 \
  --set manager.existingSecret=engine-agent-bootstrap)"
grep -Fq 'name: KUMQUAT_AGENT_BOOTSTRAP_TOKEN_FILE' <<<"${agent}"
grep -Fq 'secretName: "engine-agent-bootstrap"' <<<"${agent}"
assert_absent '--bootstrap-token' "${agent}"
grep -Fq 'path: hub-ca.crt' <<<"${agent}"
grep -Fq 'path: tunnel-ca.crt' <<<"${agent}"
assert_absent 'secretKeyRef:' "${agent}"

default_agent="$(helm template engine-agent "${ROOT}/charts/agent")"
assert_absent 'rollouts.kruise.io' "${default_agent}"
addon_agent="$(helm template engine-agent "${ROOT}/charts/agent" --set addons.enabled=true)"
grep -Fq 'rollouts.kruise.io' <<<"${addon_agent}"
grep -Fq 'resources: ["rollouts"]' <<<"${addon_agent}"

grep -Fq 'system:bootstrappers:engine-agent:${CLUSTER_NAME}' "${ROOT}/charts/manager/scripts/create-bootstrap-secrets.sh"
grep -Fq 'resourceNames: ["${CLUSTER_NAME}"]' "${ROOT}/charts/manager/scripts/create-bootstrap-secrets.sh"
grep -Fq 'TOKEN_TTL_SECONDS="${TOKEN_TTL_SECONDS:-7200}"' "${ROOT}/charts/manager/scripts/create-bootstrap-secrets.sh"
grep -Fq -- '--from-file=hub-ca.crt=' "${ROOT}/charts/manager/scripts/create-bootstrap-secrets.sh"
grep -Fq -- '--from-file=tunnel-ca.crt=' "${ROOT}/charts/manager/scripts/create-bootstrap-secrets.sh"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT
mkdir -p "${tmp_dir}/bin"
cat >"${tmp_dir}/bin/kubectl" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
case " $* " in
  *" get crd managedclusters.storage.kumquat.io "*)
    [[ "${FAKE_FAIL_CRD:-false}" != "true" ]] || exit 1 ;;
  *" config view "*) printf 'aHViLWNh' ;;
  *" get secret bootstrap-token-"*) printf 'edge' ;;
  *" get secret engine-manager-secret "*)
    [[ "${FAKE_FAIL_TLS:-false}" != "true" ]] || exit 1
    printf 'dHVubmVsLWNh' ;;
  *" apply -f - "*) tee -a "${FAKE_KUBE_LOG}" >/dev/null ;;
  *" create namespace "*) printf '%s\n' 'apiVersion: v1' 'kind: Namespace' 'metadata: {name: kumquat-system}' ;;
  *" create secret generic "*) printf '%s\n' 'apiVersion: v1' 'kind: Secret' 'metadata: {name: engine-agent-bootstrap}' ;;
  *) printf 'unexpected kubectl invocation: %s\n' "$*" >&2; exit 1 ;;
esac
SH
chmod +x "${tmp_dir}/bin/kubectl"
export FAKE_KUBE_LOG="${tmp_dir}/applied.yaml"
if PATH="${tmp_dir}/bin:${PATH}" FAKE_FAIL_CRD=true HUB_CONTEXT=hub EDGE_CONTEXT=edge CLUSTER_NAME=edge \
  "${ROOT}/charts/manager/scripts/create-bootstrap-secrets.sh" >"${tmp_dir}/missing-crd.out" 2>&1; then
  printf 'bootstrap script accepted a Hub without the ManagedCluster CRD\n' >&2
  exit 1
fi
grep -Fq 'ManagedCluster CRD' "${tmp_dir}/missing-crd.out"
if PATH="${tmp_dir}/bin:${PATH}" FAKE_FAIL_TLS=true HUB_CONTEXT=hub EDGE_CONTEXT=edge CLUSTER_NAME=edge \
  "${ROOT}/charts/manager/scripts/create-bootstrap-secrets.sh" >"${tmp_dir}/missing-tls.out" 2>&1; then
  printf 'bootstrap script accepted a Hub without the manager TLS Secret\n' >&2
  exit 1
fi
grep -Fq 'manager TLS Secret' "${tmp_dir}/missing-tls.out"
for run in 1 2; do
  PATH="${tmp_dir}/bin:${PATH}" HUB_CONTEXT=hub EDGE_CONTEXT=edge CLUSTER_NAME=edge \
    "${ROOT}/charts/manager/scripts/create-bootstrap-secrets.sh" >"${tmp_dir}/run-${run}.out"
done
first_secret="$(sed -n 's/^Rotated Hub Secret: //p' "${tmp_dir}/run-1.out")"
second_secret="$(sed -n 's/^Rotated Hub Secret: //p' "${tmp_dir}/run-2.out")"
[[ -n "${first_secret}" && "${first_secret}" == "${second_secret}" ]]
[[ "$(grep '  token-secret:' "${FAKE_KUBE_LOG}" | sort -u | wc -l | tr -d ' ')" == "2" ]]
