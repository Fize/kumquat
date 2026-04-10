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
├── node/             基于 Alpine 的 Node.js 镜像
│   └── Dockerfile.template
├── python/           基于 Alpine 的 Python 镜像
│   └── Dockerfile.template
├── kubectl/          DevOps 工具：kubectl, helm, kustomize
│   └── Dockerfile.template
├── builder/          构建工具：gcc, make, cmake 等
│   └── Dockerfile.template
├── dind/             Docker-in-Docker，用于 CI/CD
│   ├── Dockerfile.template
│   └── entrypoint.sh
├── rust/             基于 Alpine 的 Rust 镜像
│   └── Dockerfile.template
└── java/             基于 Alpine 的 OpenJDK 镜像
    └── Dockerfile.template
```

## 快速开始

```bash
# 构建所有镜像（使用默认版本）
make all

# 构建单个镜像
make alpine
make golang
make node
make python
make kubectl
make builder
make dind
make rust
make java

# 构建指定版本
make alpine ALPINE_VERSION=3.20.5
make golang GO_VERSION=1.23.6
make kubectl KUBECTL_VERSION=1.30.0 HELM_VERSION=3.15.0

# 推送到镜像仓库
REPO=yourrepo make push-all

# 查看可用镜像
make list
```

## 镜像列表

### 基础镜像

| 镜像 | 基础 | 特性 |
|------|------|------|
| `alpine` | Alpine Linux | bash, curl, jq, tini (PID 1), USTC 镜像自动切换 |

### 语言运行时

| 镜像 | 基础 | 特性 |
|------|------|------|
| `golang` | armory/alpine | Go 工具链 |
| `node` | armory/alpine | Node.js + npm |
| `python` | armory/alpine | Python 3 + pip, requests, pyyaml, jinja2 |
| `rust` | armory/alpine | Rust + cargo, cargo-audit, cargo-watch |
| `java` | armory/alpine | OpenJDK + Maven + Gradle |

### DevOps 工具

| 镜像 | 基础 | 特性 |
|------|------|------|
| `kubectl` | armory/alpine | kubectl, helm, kustomize, yq, bash 补全 |
| `dind` | armory/alpine | Docker-in-Docker, buildx, compose |

### 构建工具

| 镜像 | 基础 | 特性 |
|------|------|------|
| `builder` | armory/alpine | gcc, g++, make, cmake, autoconf, openssl-dev 等 |

## 默认版本

| 镜像 | 版本 |
|------|------|
| Alpine | 3.21.3 |
| Go | 1.24.4 |
| Node.js | 22.15.0 |
| Python | 3.12.10 |
| kubectl | 1.32.0 |
| Helm | 3.17.0 |
| Kustomize | 5.6.0 |
| Docker (DinD) | 27.5.1 |
| Rust | 1.85.0 |
| OpenJDK | 21 |

## 设计选择

- **tini** 优于 s6-overlay：仅用于僵尸进程回收的轻量级 PID 1。
- **模板化**：每种镜像类型一个模板，`Makefile` 负责变量替换。
- **`# TAGS` 指令**：在 Dockerfile 顶部添加 `# TAGS v1 latest` 可打多个标签。
- **增量构建**：`bootstrap.sh` 仅构建 Dockerfile 发生变更的镜像。

## 许可证

Apache License 2.0
