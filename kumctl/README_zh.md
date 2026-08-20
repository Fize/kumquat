# Kumctl

[English](README.md)

Kumquat 命令行工具。`kumctl` 只访问 Kumquat API，不读取 Kubernetes
kubeconfig，也不依赖 Engine 或 client-go。

```bash
kumctl --server http://127.0.0.1:8080 --token "$KUMQUAT_TOKEN" list applications
kumctl --wait --file application.json create applications
kumctl --wait adopt clusters cluster_0123
```

所有输出均为 JSON。写操作自动发送 `Idempotency-Key`；可以用
`--idempotency-key` 固定该值，并用 `--wait` 等待 operation 完成。连接信息可
由 `KUMQUAT_API_URL`、`KUMQUAT_TOKEN` 或 `$KUMQUAT_CONFIG` 指向的 context
配置提供。
