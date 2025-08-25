BINARY_NAME=smartroute
VERSION=1.0.0
BUILD_DIR=build
GO_FILES=$(shell find . -name "*.go" -not -path "./vendor/*")

# Build flags
LDFLAGS=-ldflags "-X main.version=$(VERSION) -s -w"

.PHONY: all build clean test install uninstall run daemon deps format lint help

# Default target
all: clean deps format test build

# Build the binary
build:
	@echo "🔨 Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) cmd/main.go
	@echo "✅ Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for all platforms
build-all:
	@echo "🔨 Building for all platforms..."
	@mkdir -p $(BUILD_DIR)
	# macOS
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 cmd/main.go
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 cmd/main.go
	# Linux
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 cmd/main.go
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 cmd/main.go
	# Windows
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe cmd/main.go
	@echo "✅ Cross-platform build complete"

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	go clean
	@echo "✅ Clean complete"

# Run tests
test:
	@echo "🧪 Running tests..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Tests complete (coverage report: coverage.html)"

# Install dependencies
deps:
	@echo "📦 Installing dependencies..."
	go mod download
	go mod tidy
	@echo "✅ Dependencies updated"

# Format code
format:
	@echo "💅 Formatting code..."
	go fmt ./...
	@echo "✅ Code formatted"

# Lint code
lint:
	@echo "🔍 Linting code..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "⚠️  golangci-lint not found, using go vet..."; \
		go vet ./...; \
	fi
	@echo "✅ Lint complete"

# Run the application once
run:
	@echo "🚀 Running $(BINARY_NAME)..."
	sudo go run cmd/main.go $(ARGS)

# Run as daemon
daemon:
	@echo "🌙 Starting daemon mode..."
	sudo go run cmd/main.go daemon $(ARGS)

# Install system service (requires build first)
install: build
	@echo "📥 Installing $(BINARY_NAME)..."
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	sudo chmod +x /usr/local/bin/$(BINARY_NAME)
	sudo /usr/local/bin/$(BINARY_NAME) install
	@echo "✅ Installation complete"

# Uninstall system service
uninstall:
	@echo "📤 Uninstalling $(BINARY_NAME)..."
	-sudo $(BINARY_NAME) uninstall 2>/dev/null || true
	-sudo rm -f /usr/local/bin/$(BINARY_NAME)
	-sudo rm -rf /etc/smartroute
	@echo "✅ Uninstall complete"

# Check service status
status:
	@echo "📊 Service status:"
	@$(BINARY_NAME) status 2>/dev/null || echo "Service not installed or not accessible"

# Test configuration
test-config:
	@echo "🔧 Testing configuration..."
	sudo go run cmd/main.go test

# Show version
version:
	@echo "📋 Version information:"
	@go run cmd/main.go version

# Development targets
dev-install: build
	@echo "🛠️  Installing for development..."
	cp $(BUILD_DIR)/$(BINARY_NAME) ./$(BINARY_NAME)
	@echo "✅ Development binary ready: ./$(BINARY_NAME)"

dev-test: dev-install
	@echo "🧪 Testing development build..."
	sudo ./$(BINARY_NAME) test

# Package for distribution
package: build-all
	@echo "📦 Creating distribution packages..."
	@mkdir -p $(BUILD_DIR)/dist
	# macOS
	tar -czf $(BUILD_DIR)/dist/$(BINARY_NAME)-$(VERSION)-darwin-amd64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-darwin-amd64 -C .. configs scripts
	tar -czf $(BUILD_DIR)/dist/$(BINARY_NAME)-$(VERSION)-darwin-arm64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-darwin-arm64 -C .. configs scripts
	# Linux
	tar -czf $(BUILD_DIR)/dist/$(BINARY_NAME)-$(VERSION)-linux-amd64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-linux-amd64 -C .. configs scripts
	tar -czf $(BUILD_DIR)/dist/$(BINARY_NAME)-$(VERSION)-linux-arm64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-linux-arm64 -C .. configs scripts
	# Windows
	zip -j $(BUILD_DIR)/dist/$(BINARY_NAME)-$(VERSION)-windows-amd64.zip $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe configs/* scripts/*
	@echo "✅ Distribution packages created in $(BUILD_DIR)/dist/"

# Help
help:
	@echo "Smart Route Manager - Makefile Help"
	@echo "===================================="
	@echo ""
	@echo "Build Commands:"
	@echo "  make build        Build the binary"
	@echo "  make build-all    Build for all platforms"
	@echo "  make clean        Clean build artifacts"
	@echo "  make package      Create distribution packages"
	@echo ""
	@echo "Development Commands:"
	@echo "  make test         Run tests with coverage"
	@echo "  make format       Format code"
	@echo "  make lint         Lint code"
	@echo "  make deps         Update dependencies"
	@echo ""
	@echo "Runtime Commands:"
	@echo "  make run          Run once (ARGS for arguments)"
	@echo "  make daemon       Run as daemon (ARGS for arguments)"
	@echo "  make test-config  Test configuration"
	@echo "  make version      Show version"
	@echo ""
	@echo "System Commands:"
	@echo "  make install      Install system service"
	@echo "  make uninstall    Uninstall system service"
	@echo "  make status       Check service status"
	@echo ""
	@echo "Development Commands:"
	@echo "  make dev-install  Build and install for development"
	@echo "  make dev-test     Test development build"
	@echo ""
	@echo "Examples:"
	@echo "  make run ARGS='--config configs/config.json'"
	@echo "  make daemon ARGS='--config configs/config.json --silent'"