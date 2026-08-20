# Kumctl

Kumquat's API-only command-line entry point. It never reads Kubernetes
kubeconfig and has no dependency on Engine or client-go.

```bash
kumctl --server http://127.0.0.1:8080 --token "$KUMQUAT_TOKEN" list applications
kumctl --wait --file application.json create applications
kumctl --wait adopt clusters cluster_0123
```

Output is JSON. Mutations send an idempotency key and `--wait` polls the
operation resource. Connection settings come from flags, `KUMQUAT_API_URL`,
`KUMQUAT_TOKEN`, or the context selected in `$KUMQUAT_CONFIG`.
