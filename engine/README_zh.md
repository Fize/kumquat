# Kumquat Engine

[English](README.md)

[![Go Report Card](https://goreportcard.com/badge/github.com/fize/kumquat/engine)](https://goreportcard.com/report/github.com/fize/kumquat/engine)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

Kumquat Engine 是 [Kumquat](../README_zh.md) 多集群应用管理平台的核心系统，负责跨多个 Kubernetes 集群的应用分发、调度和管理。

## 架构

Kumquat Engine 采用 Hub-Spoke 架构，包含以下组件：

| 组件 | 描述 |
|------|------|
| **Manager** | 运行在 Hub 集群上的中央控制平面，管理 Application 和 Cluster CRD |
| **调度器** | 基于插件的 Filter/Score 架构多集群调度引擎 |
| **分发器** | 生成原生 K8s 资源并分发到目标集群 |
| **隧道服务器** | 基于 WebSocket 的反向隧道，用于 Edge 集群连接 |
| **Agent** | 运行在 Edge 集群上，维护隧道连接并执行工作负载 |

## 构建

```bash
# 构建二进制文件（manager 和 agent）
make build

# 运行单元测试
make test

# 构建 Docker 镜像
make docker-build
```

## 部署

```bash
# 安装 CRD 到集群
kubectl apply -f config/crd/bases/

# 使用 Helm 部署 Manager
helm install engine-manager ../charts/manager -n kumquat-system --create-namespace

# 部署 Agent
helm install engine-agent ../charts/agent -n kumquat-system \
  --set hub.url=<hub-url> \
  --set clusterName=<cluster-name>
```

## E2E 测试

```bash
# 使用 Kind 运行完整 E2E 测试套件
make e2e-kind

# 或分步执行
make e2e-kind-create  # 创建 Kind 集群
make e2e-kind-test    # 运行测试
make e2e-kind-delete  # 清理
```

## 文档

详见 [../docs/zh/](../docs/zh/) 获取架构设计、调度器、API 参考等详细文档。

## 许可证

Apache License 2.0
