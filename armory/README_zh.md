# Armory

[English](README.md)

Kumquat 基础 Docker 镜像构建，为各子项目提供统一的运行时基础镜像。

## 目录结构

```
docker/
├── alpine/           Alpine 基础镜像 (bash, curl, jq, tini, USTC 镜像自动切换)
│   ├── Dockerfile.template
│   └── init.sh
├── golang/           基于 Alpine 的 Go 镜像
│   └── Dockerfile.template
└── node/             基于 Alpine 的 Node.js 镜像
    └── Dockerfile.template
```

## 快速开始

```bash
# 构建所有镜像（默认版本：alpine 3.21.3, go 1.24.4, node 22.15.0）
make all

# 构建指定版本
make alpine VERSION=3.20.5
make golang GO_VERSION=1.23.6 ALPINE_VERSION=3.21.3
make node NODE_VERSION=20.18.0

# 推送到镜像仓库
REPO=yourrepo make push-all

# 或使用 bootstrap.sh 增量构建（基于 git diff）
./bootstrap.sh --push=true
```

## 镜像列表

| 镜像 | 基础 | 特性 |
|------|------|------|
| `alpine` | Alpine Linux | bash, curl, jq, tini (PID 1), USTC 镜像自动切换 |
| `golang` | armory/alpine | Go 工具链 |
| `node` | armory/alpine | Node.js + npm |

## 设计选择

- **tini** 优于 s6-overlay：仅用于僵尸进程回收的轻量级 PID 1。
- **模板化**：每种镜像类型一个模板，`Makefile` 负责变量替换。
- **`# TAGS` 指令**：在 Dockerfile 顶部添加 `# TAGS v1 latest` 可打多个标签。
- **增量构建**：`bootstrap.sh` 仅构建 Dockerfile 发生变更的镜像。

## 许可证

Apache License 2.0
