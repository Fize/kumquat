.PHONY: build test run clean deps fmt lint

# 默认目标
.DEFAULT_GOAL := build

# 变量
BINARY_NAME := portal
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
	docker build -t kumquat-portal:latest .

# 帮助
help:
	@echo "Available targets:"
	@echo "  build        - Build the binary"
	@echo "  test         - Run tests"
	@echo "  run          - Run in development mode"
	@echo "  clean        - Clean build artifacts"
	@echo "  deps         - Install dependencies"
	@echo "  fmt          - Format code"
	@echo "  lint         - Run linter"
	@echo "  docker-build - Build Docker image"
	@echo "  help         - Show this help"
