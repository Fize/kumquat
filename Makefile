.PHONY: build test test-api-mysql test-kumctl-e2e demo-up demo-test demo-down clean help engine armory

help: ## Show this help
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

engine: ## Build engine
	$(MAKE) -C engine build

test: ## Run engine tests
	$(MAKE) -C engine test

test-api-mysql: ## Run all API tests against an isolated Docker MySQL 8
	$(MAKE) -C apiserver test-mysql

test-kumctl-e2e: ## Run one independent kumctl case for every included API operation
	bash kumctl/test/e2e_api_test.sh

demo-up: ## Build and deploy the local Kind demo with MySQL
	./demo/demo.sh up

demo-test: ## Run strict API-to-Engine Kind E2E
	./demo/demo.sh test

demo-down: ## Remove only demo-owned Kind clusters and network
	./demo/demo.sh down

armory: ## Build all base images
	$(MAKE) -C armory all

clean: ## Clean build artifacts
	$(MAKE) -C engine clean
	$(MAKE) -C armory clean
