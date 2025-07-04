# Makefile for nix-operator

.PHONY: build test clean run-webserver run-controller fmt vet lint help

# 默认目标
all: build

# 构建目标
build: build-webserver build-controller

build-webserver:
	@echo "Building web server..."
	go build -o bin/webserver cmd/webserver/main.go

build-controller:
	@echo "Building controller..."
	go build -o bin/controller cmd/operator/main.go

# 测试目标
test:
	@echo "Running tests..."
	go test -v ./...

test-coverage:
	@echo "Running tests with coverage..."
	go test -v -cover ./...

test-coverage-html:
	@echo "Generating coverage report..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# 运行目标
run-webserver: build-webserver
	@echo "Starting web server..."
	./bin/webserver

run-controller: build-controller
	@echo "Starting controller..."
	./bin/controller

# 代码质量检查
fmt:
	@echo "Formatting code..."
	go fmt ./...

vet:
	@echo "Running go vet..."
	go vet ./...

lint:
	@echo "Running golint..."
	golint ./...

# 依赖管理
mod-tidy:
	@echo "Tidying go modules..."
	go mod tidy

mod-download:
	@echo "Downloading dependencies..."
	go mod download

# 清理目标
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.html

# 开发环境设置
dev-setup: mod-download
	@echo "Setting up development environment..."
	mkdir -p bin
	mkdir -p /tmp/nix-operator-config

# 创建示例配置
setup-examples:
	@echo "Creating example configurations..."
	mkdir -p examples
	echo '{
	"apiVersion": "v1",
	"kind": "HostsConfiguration",
	"metadata": {
		"name": "example-hosts"
	},
	"spec": {
		"hosts": [
			{
				"ip": "127.0.0.1",
				"hostnames": ["localhost", "local"]
			},
			{
				"ip": "192.168.1.100",
				"hostnames": ["server1"]
			}
		]
	}
}' > examples/hosts-config.json
	echo '{
	"apiVersion": "v1",
	"kind": "TimeConfiguration",
	"metadata": {
		"name": "example-time"
	},
	"spec": {
		"timezone": "Asia/Shanghai",
		"servers": ["ntp1.aliyun.com", "ntp2.aliyun.com"]
	}
}' > examples/time-config.json

# Docker 相关（可选）
docker-build:
	@echo "Building Docker image..."
	docker build -t nix-operator:latest .

# 安装目标
install: build
	@echo "Installing binaries..."
	sudo cp bin/webserver /usr/local/bin/
	sudo cp bin/controller /usr/local/bin/

# 卸载目标
uninstall:
	@echo "Uninstalling binaries..."
	sudo rm -f /usr/local/bin/webserver
	sudo rm -f /usr/local/bin/controller

# 帮助信息
help:
	@echo "Available targets:"
	@echo "  build           - Build all binaries"
	@echo "  build-webserver - Build web server binary"
	@echo "  build-controller- Build controller binary"
	@echo "  test            - Run all tests"
	@echo "  test-coverage   - Run tests with coverage"
	@echo "  test-coverage-html - Generate HTML coverage report"
	@echo "  run-webserver   - Build and run web server"
	@echo "  run-controller  - Build and run controller"
	@echo "  fmt             - Format code"
	@echo "  vet             - Run go vet"
	@echo "  lint            - Run golint"
	@echo "  mod-tidy        - Tidy go modules"
	@echo "  mod-download    - Download dependencies"
	@echo "  clean           - Clean build artifacts"
	@echo "  dev-setup       - Set up development environment"
	@echo "  setup-examples  - Create example configurations"
	@echo "  docker-build    - Build Docker image"
	@echo "  install         - Install binaries to system"
	@echo "  uninstall       - Remove binaries from system"
	@echo "  help            - Show this help message"

# 变量定义
GO_VERSION := $(shell go version | cut -d' ' -f3)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# 构建信息
info:
	@echo "Go Version: $(GO_VERSION)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"