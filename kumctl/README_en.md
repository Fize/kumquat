# kumctl

[中文](README.md)

`kumctl` is the API-only terminal and agent entry point. It talks to the
Kumquat API gateway and never reads a Kubernetes kubeconfig or imports Engine
client libraries.

Global flags may appear before or after the command (both forms are accepted):

```bash
export KUMQUAT_API_URL=http://127.0.0.1:8080
kumctl --token "$KUMQUAT_TOKEN" list applications
```

## Authentication

Login is the only authentication operation exposed by the CLI. Registration,
`auth/me`, and `auth/change-password` deliberately remain API-only.

```bash
kumctl --server "$KUMQUAT_API_URL" login --username alice --password secret
kumctl --server "$KUMQUAT_API_URL" auth login --file login.json
```

## Commands

The resource names accept singular or plural forms. Every API CRUD route is
available for `users`, `modules`, `projects`, `workspaces`, and
`applications`:

```bash
kumctl --file module.json create module
kumctl get module mod_123
kumctl --file module-update.json update modules mod_123
kumctl delete project prj_123
```

Read-only API resources are intentionally read-only in kumctl as well:
`roles` support `list`/`get`, `operations` support `get`, and `clusters`
support `list`/`get` plus `adopt`.

API-specific read and adoption actions are also available:

```bash
kumctl --children get module mod_root
kumctl --permissions get role 1
kumctl --module-id mod_root --page 1 --size 50 list projects
kumctl adopt cluster cluster_123
kumctl --file adopt.json adopt workspace ws_123
kumctl --file adopt.json adopt application app_123
```

Mutations automatically receive an idempotency key. Use `--idempotency-key`
to supply a stable key, and `--wait` to poll an asynchronous operation until
it succeeds or fails. All output is JSON. Server and token settings come from
flags, `KUMQUAT_API_URL`, `KUMQUAT_TOKEN`, or the context selected in
`$KUMQUAT_CONFIG`.
