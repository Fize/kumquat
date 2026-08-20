#!/usr/bin/env bash
set -euo pipefail

: "${HUB_CONTEXT:?set HUB_CONTEXT to the Hub kubectl context}"
: "${EDGE_CONTEXT:?set EDGE_CONTEXT to the Edge kubectl context}"
: "${CLUSTER_NAME:?set CLUSTER_NAME to the ManagedCluster name}"

[[ "${CLUSTER_NAME}" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]] || { printf 'invalid CLUSTER_NAME\n' >&2; exit 1; }
(( ${#CLUSTER_NAME} <= 63 )) || { printf 'CLUSTER_NAME exceeds 63 characters\n' >&2; exit 1; }

NAMESPACE="${NAMESPACE:-kumquat-system}"
EDGE_SECRET="${EDGE_SECRET:-engine-agent-bootstrap}"
TOKEN_TTL_SECONDS="${TOKEN_TTL_SECONDS:-7200}"
TUNNEL_TLS_SECRET="${TUNNEL_TLS_SECRET:-engine-manager-secret}"
ENABLE_HUB_ADDON_READ="${ENABLE_HUB_ADDON_READ:-false}"
group="system:bootstrappers:engine-agent:${CLUSTER_NAME}"
token_id="$(printf '%s' "${CLUSTER_NAME}" | openssl dgst -sha256 -r | cut -c1-6)"
hub_secret="bootstrap-token-${token_id}"
rbac_name="kumquat:bootstrap:${CLUSTER_NAME}"

[[ "${TOKEN_TTL_SECONDS}" =~ ^[1-9][0-9]*$ ]] || { printf 'TOKEN_TTL_SECONDS must be a positive integer\n' >&2; exit 1; }

if ! kubectl --context "${HUB_CONTEXT}" get crd managedclusters.storage.kumquat.io >/dev/null 2>&1; then
  printf 'Hub ManagedCluster CRD is missing; install the manager CRDs before running bootstrap\n' >&2
  exit 1
fi
if ! kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" get secret "${TUNNEL_TLS_SECRET}" >/dev/null 2>&1; then
  printf 'manager TLS Secret %s/%s is not ready; install manager and wait for its Certificate first\n' "${NAMESPACE}" "${TUNNEL_TLS_SECRET}" >&2
  exit 1
fi

existing_claim="$(kubectl --context "${HUB_CONTEXT}" -n kube-system get secret "${hub_secret}" -o jsonpath='{.metadata.annotations.cluster\.kumquat\.io/bootstrap-cluster-name}' 2>/dev/null || true)"
if [[ -n "${existing_claim}" && "${existing_claim}" != "${CLUSTER_NAME}" ]]; then
  printf 'refusing to rotate %s: immutable cluster claim differs\n' "${hub_secret}" >&2
  exit 1
fi

if expiration="$(date -u -v+"${TOKEN_TTL_SECONDS}"S '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null)"; then
  :
else
  expiration="$(date -u -d "+${TOKEN_TTL_SECONDS} seconds" '+%Y-%m-%dT%H:%M:%SZ')"
fi
token_secret="$(openssl rand -hex 8)"

kubectl --context "${HUB_CONTEXT}" apply -f - >/dev/null <<YAML
apiVersion: storage.kumquat.io/v1alpha1
kind: ManagedCluster
metadata:
  name: ${CLUSTER_NAME}
spec:
  connectionMode: Edge
---
apiVersion: v1
kind: Secret
metadata:
  name: ${hub_secret}
  namespace: kube-system
  annotations:
    cluster.kumquat.io/bootstrap-cluster-name: ${CLUSTER_NAME}
type: bootstrap.kubernetes.io/token
stringData:
  description: Short-lived registration for ${CLUSTER_NAME}
  token-id: ${token_id}
  token-secret: ${token_secret}
  expiration: ${expiration}
  usage-bootstrap-authentication: "true"
  auth-extra-groups: ${group}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ${rbac_name}
rules:
- apiGroups: ["storage.kumquat.io"]
  resources: ["managedclusters"]
  resourceNames: ["${CLUSTER_NAME}"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["storage.kumquat.io"]
  resources: ["managedclusters/status"]
  resourceNames: ["${CLUSTER_NAME}"]
  verbs: ["get", "update", "patch"]
- nonResourceURLs: ["/api", "/apis"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: ${rbac_name}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ${rbac_name}
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: Group
  name: ${group}
YAML

if [[ "${ENABLE_HUB_ADDON_READ}" == "true" ]]; then
  kubectl --context "${HUB_CONTEXT}" apply -f - >/dev/null <<YAML
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ${rbac_name}:addons
rules:
- apiGroups: ["apps.kumquat.io"]
  resources: ["applications"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: ${rbac_name}:addons
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ${rbac_name}:addons
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: Group
  name: ${group}
YAML
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT
chmod 700 "${tmp_dir}"
printf '%s' "${token_id}.${token_secret}" >"${tmp_dir}/token"
chmod 600 "${tmp_dir}/token"
kubectl config view --raw --minify --context "${HUB_CONTEXT}" -o jsonpath='{.clusters[0].cluster.certificate-authority-data}' | base64 --decode >"${tmp_dir}/hub-ca.crt"
kubectl --context "${HUB_CONTEXT}" -n "${NAMESPACE}" get secret "${TUNNEL_TLS_SECRET}" -o jsonpath='{.data.tls\.crt}' | base64 --decode >"${tmp_dir}/tunnel-ca.crt"
chmod 600 "${tmp_dir}/hub-ca.crt" "${tmp_dir}/tunnel-ca.crt"

kubectl --context "${EDGE_CONTEXT}" create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl --context "${EDGE_CONTEXT}" apply -f - >/dev/null
kubectl --context "${EDGE_CONTEXT}" -n "${NAMESPACE}" create secret generic "${EDGE_SECRET}" \
  --from-file=token="${tmp_dir}/token" \
  --from-file=hub-ca.crt="${tmp_dir}/hub-ca.crt" \
  --from-file=tunnel-ca.crt="${tmp_dir}/tunnel-ca.crt" \
  --dry-run=client -o yaml | kubectl --context "${EDGE_CONTEXT}" apply -f - >/dev/null

printf 'Rotated Hub Secret: %s\nEdge Secret: %s/%s\nExpires: %s\n' "${hub_secret}" "${NAMESPACE}" "${EDGE_SECRET}" "${expiration}"
