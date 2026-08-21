# Kumquat 文档中心

[English](en/index.md)

欢迎阅读 Kumquat 多集群应用管理平台文档。

## 文档目录

| 文档 | 描述 | 适合读者 |
|------|------|----------|
| [架构设计](zh/architecture.md) | 用户入口、API、Engine 和成员集群的职责边界 | 开发者、架构师 |
| [调度器设计](zh/scheduler.md) | 多集群调度框架、插件机制 | 开发者 |
| [Addon 扩展设计](zh/addon.md) | 插件机制、Submariner 集成、自定义 Addon 开发 | 开发者、架构师 |
| [拓扑分布指南](zh/topology_spread.md) | 跨区域工作负载分布插件详解 | 开发者、运维 |
| [Edge 集群管理](zh/edge.md) | 隧道连接、Bootstrap Token、Agent 部署 | 运维、开发者 |
| [API 参考](zh/api.md) | CRD 规范、字段说明、使用示例 | 所有用户 |

## 快速导航

### 快速入门

如果你是第一次接触 Kumquat，建议按以下顺序阅读：

1. **[架构设计](zh/architecture.md)** - 了解 Kumquat 的整体设计理念和组件
2. **[API 参考](zh/api.md)** - 学习如何定义 Application 和 ManagedCluster
3. **[Edge 集群管理](zh/edge.md)** - 了解如何接入边缘集群（如需要）

### 深入理解

想要进一步理解 Kumquat 的关键机制：

1. **[调度器设计](zh/scheduler.md)** - 理解多集群调度的核心算法
2. **[Addon 扩展设计](zh/addon.md)** - 学习插件机制和 Submariner 集成
3. **[拓扑分布指南](zh/topology_spread.md)** - 学习跨区域高可用部署

### 常见场景

| 场景 | 推荐文档 |
|------|---------|
| 部署第一个跨集群应用 | [API 参考 - Application](zh/api.md#application) |
| 注册新的成员集群 | [API 参考 - ManagedCluster](zh/api.md#managedcluster) |
| 使用命令行操作资源 | [Kumctl](../kumctl/README.md) |
| 接入 NAT 后的边缘集群 | [Edge 集群管理](zh/edge.md) |
| 配置跨区域高可用 | [拓扑分布指南](zh/topology_spread.md) |
| 理解调度决策 | [调度器设计](zh/scheduler.md) |
| 集成 Submariner | [Addon 扩展设计](zh/addon.md#submariner-接入实现) |
| 开发自定义 Addon | [Addon 扩展设计](zh/addon.md#开发自定义-addon) |

## 项目链接

- [GitHub 仓库](https://github.com/fize/kumquat)
- [问题反馈](https://github.com/fize/kumquat/issues)

## 文档贡献

发现文档有误或想要补充内容？欢迎提交 PR！文档使用 Markdown 格式编写，位于 `docs/` 目录下。
