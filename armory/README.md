# Armory

[中文](README_zh.md)

Base Docker image builds for Kumquat, providing unified runtime base images for all sub-projects.

## Structure

```
docker/
├── alpine/           Alpine base image (bash, curl, jq, tini, USTC mirror)
│   ├── Dockerfile.template
│   └── init.sh
├── golang/           Go on Alpine
│   └── Dockerfile.template
└── node/             Node.js on Alpine
    └── Dockerfile.template
```

## Quick Start

```bash
# Build all (default versions: alpine 3.21.3, go 1.24.4, node 22.15.0)
make all

# Build individual with custom version
make alpine VERSION=3.20.5
make golang GO_VERSION=1.23.6 ALPINE_VERSION=3.21.3
make node NODE_VERSION=20.18.0

# Push to registry
REPO=yourrepo make push-all

# Or use bootstrap.sh for incremental build (based on git diff)
./bootstrap.sh --push=true
```

## Images

| Image | Base | Features |
|-------|------|----------|
| `alpine` | Alpine Linux | bash, curl, jq, tini (PID 1), USTC mirror auto-switch |
| `golang` | armory/alpine | Go toolchain |
| `node` | armory/alpine | Node.js + npm |

## Design Choices

- **tini** over s6-overlay: Lightweight PID 1 for zombie process reaping only.
- **Template-based**: Single template per image type; `Makefile` handles variable substitution.
- **`# TAGS` directive**: Add `# TAGS v1 latest` at top of Dockerfile for multiple tags.
- **Incremental build**: `bootstrap.sh` only builds images whose Dockerfiles changed.

## License

Apache License 2.0
