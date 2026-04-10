.PHONY: alpine golang node clean help

REPO ?= kumquat/armory
BUILD_DIR ?= .build

# Default versions (override with: make alpine VERSION=3.21.3)
ALPINE_VERSION ?= 3.21.3
GO_VERSION ?= 1.24.4
NODE_VERSION ?= 22.15.0

# Colors
GREEN := \033[0;32m
RESET := \033[0m

help: ## Show this help
	@echo "Usage: make [target] [VARS]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  $(GREEN)%-10s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "Variables:"
	@echo "  REPO            Docker repo prefix (default: kumquat/armory)"
	@echo "  ALPINE_VERSION  Alpine version (default: $(ALPINE_VERSION))"
	@echo "  GO_VERSION      Go version (default: $(GO_VERSION))"
	@echo "  NODE_VERSION    Node.js version (default: $(NODE_VERSION))"
	@echo ""
	@echo "Examples:"
	@echo "  make alpine                          Build latest Alpine"
	@echo "  make alpine VERSION=3.20.5           Build specific version"
	@echo "  make golang GO_VERSION=1.23.6        Build Go 1.23"
	@echo "  make node NODE_VERSION=20.18.0       Build Node 20 LTS"
	@echo "  make all                             Build all images"
	@echo "  make push-all                        Build and push all"

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

golang: ## Build Go image (depends on alpine)
	@echo "$(GREEN)[ build ]$(RESET) $(REPO)/golang:$(GO_VERSION)"
	@mkdir -p $(BUILD_DIR)/golang-$(GO_VERSION)
	@sed -e 's|{{REPO}}|$(REPO)|g' \
	     -e 's|{{ALPINE_VERSION}}|$(ALPINE_VERSION)|g' \
	     -e 's|{{GO_VERSION}}|$(GO_VERSION)|g' \
	     docker/golang/Dockerfile.template \
	     > $(BUILD_DIR)/golang-$(GO_VERSION)/Dockerfile
	docker build -t $(REPO)/golang:$(GO_VERSION) \
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
	             -f $(BUILD_DIR)/node-$(NODE_VERSION)/Dockerfile \
	     $(BUILD_DIR)/node-$(NODE_VERSION)

all: alpine golang node ## Build all images

push-alpine: alpine ## Push Alpine image
	docker push $(REPO)/alpine:$(ALPINE_VERSION)
	docker push $(REPO)/alpine:latest

push-golang: golang ## Push Go image
	docker push $(REPO)/golang:$(GO_VERSION)

push-node: node ## Push Node.js image
	docker push $(REPO)/node:$(NODE_VERSION)

push-all: push-alpine push-golang push-node ## Push all images

clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)

lint: ## Lint all Dockerfiles
	@which hadolint >/dev/null 2>&1 && hadolint $(shell find $(BUILD_DIR) -name Dockerfile 2>/dev/null) || echo "$(GREEN)[ skip ]$(RESET) hadolint not installed"
