.PHONY: alpine golang node python kubectl builder dind rust java clean help

REPO ?= kumquat/armory
BUILD_DIR ?= .build

# Default versions (override with: make alpine VERSION=3.21.3)
ALPINE_VERSION ?= 3.21.3
GO_VERSION ?= 1.24.4
NODE_VERSION ?= 22.15.0
PYTHON_VERSION ?= 3.12.10-r0
KUBECTL_VERSION ?= 1.32.0
HELM_VERSION ?= 3.17.0
KUSTOMIZE_VERSION ?= 5.6.0
DOCKER_VERSION ?= 27.5.1
BUILDX_VERSION ?= 0.21.0
RUST_VERSION ?= 1.85.0
JAVA_VERSION ?= 21

# Colors
GREEN := \033[0;32m
RESET := \033[0m

help: ## Show this help
	@echo "Usage: make [target] [VARS]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  $(GREEN)%-12s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "Variables:"
	@echo "  REPO              Docker repo prefix (default: $(REPO))"
	@echo "  ALPINE_VERSION    Alpine version (default: $(ALPINE_VERSION))"
	@echo "  GO_VERSION        Go version (default: $(GO_VERSION))"
	@echo "  NODE_VERSION      Node.js version (default: $(NODE_VERSION))"
	@echo "  PYTHON_VERSION    Python version (default: $(PYTHON_VERSION))"
	@echo "  KUBECTL_VERSION   kubectl version (default: $(KUBECTL_VERSION))"
	@echo "  HELM_VERSION      Helm version (default: $(HELM_VERSION))"
	@echo "  KUSTOMIZE_VERSION Kustomize version (default: $(KUSTOMIZE_VERSION))"
	@echo "  DOCKER_VERSION    Docker version for DinD (default: $(DOCKER_VERSION))"
	@echo "  RUST_VERSION      Rust version (default: $(RUST_VERSION))"
	@echo "  JAVA_VERSION      OpenJDK version (default: $(JAVA_VERSION))"
	@echo ""
	@echo "Examples:"
	@echo "  make alpine                          Build latest Alpine"
	@echo "  make alpine VERSION=3.20.5           Build specific version"
	@echo "  make golang GO_VERSION=1.23.6        Build Go 1.23"
	@echo "  make kubectl KUBECTL_VERSION=1.30.0  Build kubectl 1.30"
	@echo "  make all                             Build all images"
	@echo "  make push-all                        Build and push all"

# =============================================================================
# Alpine Base Image
# =============================================================================

alpine: ## Build Alpine base image
	@echo "$(GREEN)[ build ]$(RESET) $(REPO)/alpine:$(ALPINE_VERSION)"
	@mkdir -p $(BUILD_DIR)/alpine-$(ALPINE_VERSION)
	@sed -e 's|{{VERSION}}|$(ALPINE_VERSION)|g' \
	     docker/alpine/Dockerfile.template \
	     > $(BUILD_DIR)/alpine-$(ALPINE_VERSION)/Dockerfile
	@cp docker/alpine/init.sh $(BUILD_DIR)/alpine-$(ALPINE_VERSION)/init.sh
	docker build -t $(REPO)/alpine:$(ALPINE_VERSION) \
	             -t $(REPO)/alpine:latest \
	             -f $(BUILD_DIR)/alpine-$(ALPINE_VERSION)/Dockerfile \
	             $(BUILD_DIR)/alpine-$(ALPINE_VERSION)

# =============================================================================
# Language Runtimes
# =============================================================================

golang: ## Build Go image (depends on alpine)
	@echo "$(GREEN)[ build ]$(RESET) $(REPO)/golang:$(GO_VERSION)"
	@mkdir -p $(BUILD_DIR)/golang-$(GO_VERSION)
	@sed -e 's|{{REPO}}|$(REPO)|g' \
	     -e 's|{{ALPINE_VERSION}}|$(ALPINE_VERSION)|g' \
	     -e 's|{{GO_VERSION}}|$(GO_VERSION)|g' \
	     docker/golang/Dockerfile.template \
	     > $(BUILD_DIR)/golang-$(GO_VERSION)/Dockerfile
	docker build -t $(REPO)/golang:$(GO_VERSION) \
	             -t $(REPO)/golang:latest \
	             -f $(BUILD_DIR)/golang-$(GO_VERSION)/Dockerfile \
	             $(BUILD_DIR)/golang-$(GO_VERSION)

node: ## Build Node.js image (depends on alpine)
	@echo "$(GREEN)[ build ]$(RESET) $(REPO)/node:$(NODE_VERSION)"
	@mkdir -p $(BUILD_DIR)/node-$(NODE_VERSION)
	@sed -e 's|{{REPO}}|$(REPO)|g' \
	     -e 's|{{ALPINE_VERSION}}|$(ALPINE_VERSION)|g' \
	     -e 's|{{NODE_VERSION}}|$(NODE_VERSION)|g' \
	     docker/node/Dockerfile.template \
	     > $(BUILD_DIR)/node-$(NODE_VERSION)/Dockerfile
	docker build -t $(REPO)/node:$(NODE_VERSION) \
	             -t $(REPO)/node:latest \
	             -f $(BUILD_DIR)/node-$(NODE_VERSION)/Dockerfile \
	             $(BUILD_DIR)/node-$(NODE_VERSION)

python: ## Build Python image (depends on alpine)
	@echo "$(GREEN)[ build ]$(RESET) $(REPO)/python:$(PYTHON_VERSION)"
	@mkdir -p $(BUILD_DIR)/python-$(PYTHON_VERSION)
	@sed -e 's|{{REPO}}|$(REPO)|g' \
	     -e 's|{{ALPINE_VERSION}}|$(ALPINE_VERSION)|g' \
	     -e 's|{{VERSION}}|$(PYTHON_VERSION)|g' \
	     docker/python/Dockerfile.template \
	     > $(BUILD_DIR)/python-$(PYTHON_VERSION)/Dockerfile
	docker build -t $(REPO)/python:$(PYTHON_VERSION) \
	             -t $(REPO)/python:latest \
	             -f $(BUILD_DIR)/python-$(PYTHON_VERSION)/Dockerfile \
	             $(BUILD_DIR)/python-$(PYTHON_VERSION)

rust: ## Build Rust image (depends on alpine)
	@echo "$(GREEN)[ build ]$(RESET) $(REPO)/rust:$(RUST_VERSION)"
	@mkdir -p $(BUILD_DIR)/rust-$(RUST_VERSION)
	@sed -e 's|{{REPO}}|$(REPO)|g' \
	     -e 's|{{ALPINE_VERSION}}|$(ALPINE_VERSION)|g' \
	     -e 's|{{VERSION}}|$(RUST_VERSION)|g' \
	     docker/rust/Dockerfile.template \
	     > $(BUILD_DIR)/rust-$(RUST_VERSION)/Dockerfile
	docker build -t $(REPO)/rust:$(RUST_VERSION) \
	             -t $(REPO)/rust:latest \
	             -f $(BUILD_DIR)/rust-$(RUST_VERSION)/Dockerfile \
	             $(BUILD_DIR)/rust-$(RUST_VERSION)

java: ## Build Java image (depends on alpine)
	@echo "$(GREEN)[ build ]$(RESET) $(REPO)/java:$(JAVA_VERSION)"
	@mkdir -p $(BUILD_DIR)/java-$(JAVA_VERSION)
	@sed -e 's|{{REPO}}|$(REPO)|g' \
	     -e 's|{{ALPINE_VERSION}}|$(ALPINE_VERSION)|g' \
	     -e 's|{{VERSION}}|$(JAVA_VERSION)|g' \
	     docker/java/Dockerfile.template \
	     > $(BUILD_DIR)/java-$(JAVA_VERSION)/Dockerfile
	docker build -t $(REPO)/java:$(JAVA_VERSION) \
	             -t $(REPO)/java:latest \
	             -f $(BUILD_DIR)/java-$(JAVA_VERSION)/Dockerfile \
	             $(BUILD_DIR)/java-$(JAVA_VERSION)

# =============================================================================
# DevOps Tools
# =============================================================================

kubectl: ## Build kubectl image with helm and kustomize (depends on alpine)
	@echo "$(GREEN)[ build ]$(RESET) $(REPO)/kubectl:$(KUBECTL_VERSION)"
	@mkdir -p $(BUILD_DIR)/kubectl-$(KUBECTL_VERSION)
	@sed -e 's|{{REPO}}|$(REPO)|g' \
	     -e 's|{{ALPINE_VERSION}}|$(ALPINE_VERSION)|g' \
	     -e 's|{{KUBECTL_VERSION}}|$(KUBECTL_VERSION)|g' \
	     -e 's|{{HELM_VERSION}}|$(HELM_VERSION)|g' \
	     -e 's|{{KUSTOMIZE_VERSION}}|$(KUSTOMIZE_VERSION)|g' \
	     docker/kubectl/Dockerfile.template \
	     > $(BUILD_DIR)/kubectl-$(KUBECTL_VERSION)/Dockerfile
	docker build -t $(REPO)/kubectl:$(KUBECTL_VERSION) \
	             -t $(REPO)/kubectl:latest \
	             -f $(BUILD_DIR)/kubectl-$(KUBECTL_VERSION)/Dockerfile \
	             $(BUILD_DIR)/kubectl-$(KUBECTL_VERSION)

# =============================================================================
# Build Tools
# =============================================================================

builder: ## Build builder image with compilation tools (depends on alpine)
	@echo "$(GREEN)[ build ]$(RESET) $(REPO)/builder:latest"
	@mkdir -p $(BUILD_DIR)/builder
	@sed -e 's|{{REPO}}|$(REPO)|g' \
	     -e 's|{{ALPINE_VERSION}}|$(ALPINE_VERSION)|g' \
	     docker/builder/Dockerfile.template \
	     > $(BUILD_DIR)/builder/Dockerfile
	docker build -t $(REPO)/builder:latest \
	             -f $(BUILD_DIR)/builder/Dockerfile \
	             $(BUILD_DIR)/builder

# =============================================================================
# Docker-in-Docker
# =============================================================================

dind: ## Build Docker-in-Docker image for CI/CD (depends on alpine)
	@echo "$(GREEN)[ build ]$(RESET) $(REPO)/dind:$(DOCKER_VERSION)"
	@mkdir -p $(BUILD_DIR)/dind-$(DOCKER_VERSION)
	@sed -e 's|{{REPO}}|$(REPO)|g' \
	     -e 's|{{ALPINE_VERSION}}|$(ALPINE_VERSION)|g' \
	     -e 's|{{DOCKER_VERSION}}|$(DOCKER_VERSION)|g' \
	     -e 's|{{BUILDX_VERSION}}|$(BUILDX_VERSION)|g' \
	     docker/dind/Dockerfile.template \
	     > $(BUILD_DIR)/dind-$(DOCKER_VERSION)/Dockerfile
	@cp docker/dind/entrypoint.sh $(BUILD_DIR)/dind-$(DOCKER_VERSION)/entrypoint.sh
	docker build -t $(REPO)/dind:$(DOCKER_VERSION) \
	             -t $(REPO)/dind:latest \
	             -f $(BUILD_DIR)/dind-$(DOCKER_VERSION)/Dockerfile \
	             $(BUILD_DIR)/dind-$(DOCKER_VERSION)

# =============================================================================
# Build All
# =============================================================================

all: alpine golang node python kubectl builder dind rust java ## Build all images

# =============================================================================
# Push Targets
# =============================================================================

push-alpine: alpine ## Push Alpine image
	docker push $(REPO)/alpine:$(ALPINE_VERSION)
	docker push $(REPO)/alpine:latest

push-golang: golang ## Push Go image
	docker push $(REPO)/golang:$(GO_VERSION)
	docker push $(REPO)/golang:latest

push-node: node ## Push Node.js image
	docker push $(REPO)/node:$(NODE_VERSION)
	docker push $(REPO)/node:latest

push-python: python ## Push Python image
	docker push $(REPO)/python:$(PYTHON_VERSION)
	docker push $(REPO)/python:latest

push-kubectl: kubectl ## Push kubectl image
	docker push $(REPO)/kubectl:$(KUBECTL_VERSION)
	docker push $(REPO)/kubectl:latest

push-builder: builder ## Push builder image
	docker push $(REPO)/builder:latest

push-dind: dind ## Push DinD image
	docker push $(REPO)/dind:$(DOCKER_VERSION)
	docker push $(REPO)/dind:latest

push-rust: rust ## Push Rust image
	docker push $(REPO)/rust:$(RUST_VERSION)
	docker push $(REPO)/rust:latest

push-java: java ## Push Java image
	docker push $(REPO)/java:$(JAVA_VERSION)
	docker push $(REPO)/java:latest

push-all: push-alpine push-golang push-node push-python push-kubectl push-builder push-dind push-rust push-java ## Push all images

# =============================================================================
# Utilities
# =============================================================================

clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)

lint: ## Lint all Dockerfiles
	@which hadolint >/dev/null 2>&1 && hadolint $(shell find $(BUILD_DIR) -name Dockerfile 2>/dev/null) || echo "$(GREEN)[ skip ]$(RESET) hadolint not installed"

list: ## List all available images
	@echo "Available images:"
	@echo "  $(GREEN)alpine$(RESET)    - Alpine base image ($(ALPINE_VERSION))"
	@echo "  $(GREEN)golang$(RESET)    - Go runtime ($(GO_VERSION))"
	@echo "  $(GREEN)node$(RESET)      - Node.js runtime ($(NODE_VERSION))"
	@echo "  $(GREEN)python$(RESET)    - Python runtime ($(PYTHON_VERSION))"
	@echo "  $(GREEN)kubectl$(RESET)   - DevOps tools: kubectl ($(KUBECTL_VERSION)), helm ($(HELM_VERSION)), kustomize ($(KUSTOMIZE_VERSION))"
	@echo "  $(GREEN)builder$(RESET)   - Build tools: gcc, make, cmake, etc."
	@echo "  $(GREEN)dind$(RESET)      - Docker-in-Docker ($(DOCKER_VERSION))"
	@echo "  $(GREEN)rust$(RESET)      - Rust runtime ($(RUST_VERSION))"
	@echo "  $(GREEN)java$(RESET)      - OpenJDK ($(JAVA_VERSION))"
