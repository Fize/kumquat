# Armory

Base Docker image builder for Kumquat, providing unified runtime base images for all subprojects.

## Directory Structure

```
docker/
├── alpine/           Alpine base image (bash, curl, jq, tini, automatic USTC mirror switching)
│   ├── Dockerfile.template
│   └── init.sh
├── golang/            Go image based on Alpine
│   └── Dockerfile.template
├── node/              Node.js image based on Alpine
│   └── Dockerfile.template
├── python/            Python image based on Alpine
│   └── Dockerfile.template
├── kubectl/          DevOps tools: kubectl, helm, kustomize
│   └── Dockerfile.template
├── builder/           Build tools: gcc, make, cmake, etc.
│   └── Dockerfile.template
├── dind/             Docker-in-Docker, for CI/CD
│   ├── Dockerfile.template
│   └── entrypoint.sh
├── rust/              Rust image based on Alpine
│   └── Dockerfile.template
└── java/              OpenJDK image based on Alpine
    └── Dockerfile.template
```

## Quick Start

```bash
# Build all images (using default versions)
make all

# Build a single image
make alpine
make golang
make node
make python
make kubectl
make builder
make dind
make rust
make java

# Build a specific version
make alpine ALPINE_VERSION=3.20.5
make golang GO_VERSION=1.23.6
make kubectl KUBECTL_VERSION=1.30.0 HELM_VERSION=3.15.0

# Push to image registry
REPO=yourrepo make push-all

# View available images
make list
```

## Image List

### Base Images

| Image | Base | Features |
|-------|------|----------|
| `alpine` | Alpine Linux | bash, curl, jq, tini (PID 1), automatic USTC mirror switching |

### Language Runtimes

| Image | Base | Features |
|-------|------|----------|
| `golang` | armory/alpine | Go toolchain |
| `node` | armory/alpine | Node.js + npm |
| `python` | armory/alpine | Python 3 + pip, requests, pyyaml, jinja2 |
| `rust` | armory/alpine | Rust + cargo, cargo-audit, cargo-watch |
| `java` | armory/alpine | OpenJDK + Maven + Gradle |

### DevOps Tools

| Image | Base | Features |
|-------|------|----------|
| `kubectl` | armory/alpine | kubectl, helm, kustomize, yq, bash completion |
| `dind` | armory/alpine | Docker-in-Docker, buildx, compose |

### Build Tools

| Image | Base | Features |
|-------|------|----------|
| `builder` | armory/alpine | gcc, g++, make, cmake, autoconf, openssl-dev, etc. |

## Default Versions

| Image | Version |
|-------|---------|
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

## Design Choices

- **tini** is preferred over s6-overlay: lightweight PID 1 for zombie process reaping only.
- **Templating**: one template per image type, `Makefile` handles variable substitution.
- **`# TAGS` directive**: Adding `# TAGS v1 latest` at the top of a Dockerfile allows multiple tags.
- **Incremental builds**: `bootstrap.sh` only builds images whose Dockerfile has changed.

## License

Apache License 2.0
