.PHONY: build test test-mysql run clean deps fmt lint

# 默认目标
.DEFAULT_GOAL := build

# 变量
BINARY_NAME := api
BUILD_DIR := ./build
CMD_DIR := ./cmd/server
GO := go
GOFLAGS :=

# 构建
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# 测试
test:
	@echo "Running tests..."
	$(GO) test -v ./...

# Authoritative database-backed suite. It owns only the Compose project below,
# waits for MySQL health, rejects every skipped test, and always cleans up.
test-mysql:
	@set -eu; \
	for tool in docker go python3 mktemp; do command -v "$$tool" >/dev/null || { echo "missing prerequisite: $$tool" >&2; exit 1; }; done; \
	docker compose version >/dev/null; \
	run_dir="$$(mktemp -d "$${TMPDIR:-/tmp}/kumquat-api-mysql.XXXXXX")"; \
	project="kumquat-api-test-$$(basename "$$run_dir" | tr '[:upper:].' '[:lower:]_')"; \
	log_file="$$run_dir/go-test.json"; \
	cleanup() { docker compose -p "$$project" -f docker-compose.test.yaml down --volumes --remove-orphans >/dev/null 2>&1 || true; rm -f "$$log_file"; rmdir "$$run_dir" 2>/dev/null || true; }; \
	trap cleanup EXIT INT TERM; \
	docker compose -p "$$project" -f docker-compose.test.yaml up -d --wait --wait-timeout 180 mysql; \
	endpoint="$$(docker compose -p "$$project" -f docker-compose.test.yaml port mysql 3306)"; \
	port="$${endpoint##*:}"; \
	case "$$port" in ''|*[!0-9]*) echo "unable to parse MySQL host port from $$endpoint" >&2; exit 1 ;; esac; \
	export KUMQUAT_TEST_MYSQL_DSN="root:kumquat-test-root@tcp(127.0.0.1:$$port)/mysql?charset=utf8mb4&parseTime=True&loc=UTC"; \
	set +e; \
	$(GO) test -json -race -count=1 ./... >"$$log_file"; test_status=$$?; \
	python3 internal/testdb/assert_go_test_json.py "$$log_file"; skip_status=$$?; \
	set -e; \
	cat "$$log_file"; \
	if [ "$$test_status" -ne 0 ]; then exit "$$test_status"; fi; \
	exit "$$skip_status"

# 运行（开发模式）
run:
	@echo "Running in development mode..."
	$(GO) run $(CMD_DIR)/main.go

# 清理
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f *.db
	@echo "Clean complete"

# 安装依赖
deps:
	@echo "Installing dependencies..."
	$(GO) mod download
	$(GO) mod tidy

# 格式化代码
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

# 检查代码（如果安装了 golangci-lint）
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "Running linter..."; \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, skipping..."; \
	fi

# 数据库迁移（如果需要）
migrate:
	@echo "Running database migration..."
	$(GO) run $(CMD_DIR)/main.go migrate

# 构建 Docker 镜像（可选）
docker-build:
	@echo "Building Docker image..."
	docker build -t kumquat-api:latest .

# 帮助
help:
	@echo "Available targets:"
	@echo "  build        - Build the binary"
	@echo "  test         - Run tests"
	@echo "  test-mysql   - Run every API test against a real MySQL 8 container"
	@echo "  run          - Run in development mode"
	@echo "  clean        - Clean build artifacts"
	@echo "  deps         - Install dependencies"
	@echo "  fmt          - Format code"
	@echo "  lint         - Run linter"
	@echo "  docker-build - Build Docker image"
	@echo "  help         - Show this help"
