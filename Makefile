.PHONY: build test clean help engine armory

help: ## Show this help
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

engine: ## Build engine
	$(MAKE) -C engine build

test: ## Run engine tests
	$(MAKE) -C engine test

armory: ## Build all base images
	$(MAKE) -C armory all

clean: ## Clean build artifacts
	$(MAKE) -C engine clean
	$(MAKE) -C armory clean
