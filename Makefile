.PHONY: help build run test clean deps install

# Configuration
BINARY_NAME ?= mediatx
BUILD_DIR ?= .
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "1.0.0")
BUILD_DATE = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_FLAGS = -ldflags="-s -w -X 'main.Version=$(VERSION)' -X 'main.buildDate=$(BUILD_DATE)'"

help:
	@echo ""
	@echo "╔╦╗┌─┐┌┬┐┬┌─┐╔╦╗═╗ ╦"
	@echo "║║║├┤  │││├─┤ ║ ╔╩╦╝"
	@echo "╩ ╩└─┘─┴┘┴┴ ┴ ╩ ╩ ╚═"
	@echo ""
	@echo "MediaTX - High Performance Media Server"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@echo "Available Commands:"
	@echo ""
	@echo "  make deps      - Install dependencies (go mod download + tidy)"
	@echo "  make build     - Build the binary (rtmp-server)"
	@echo "  make install   - Build and install binary to GOBIN"
	@echo "  make run       - Run the application (go run)"
	@echo "  make test      - Run tests"
	@echo "  make clean     - Clean build artifacts"
	@echo ""
	@echo "Configuration:"
	@echo "  BINARY_NAME=$(BINARY_NAME)"
	@echo "  BUILD_DIR=$(BUILD_DIR)"
	@echo "  VERSION=$(VERSION)"
	@echo ""

deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy
	@echo "✅ Dependencies installed"

build: deps
	@echo "Building $(BINARY_NAME)..."
	@go build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "✅ Binary built: $(BUILD_DIR)/$(BINARY_NAME)"
	@ls -lh $(BUILD_DIR)/$(BINARY_NAME)

install: deps
	@echo "Installing $(BINARY_NAME)..."
	@go install $(BUILD_FLAGS) .
	@echo "✅ Binary installed to $$(go env GOBIN)"

run:
	@echo ""
	@echo "╔╦╗┌─┐┌┬┐┬┌─┐╔╦╗═╗ ╦"
	@echo "║║║├┤  │││├─┤ ║ ╔╩╦╝"
	@echo "╩ ╩└─┘─┴┘┴┴ ┴ ╩ ╩ ╚═"
	@echo ""
	@echo "Starting MediaTX v$(VERSION)..."
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "• RTMP: rtmp://localhost:1935"
	@echo "• SRT:  srt://localhost:5000"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@go run .

test:
	@echo "Running tests..."
	@go test -v ./...

test-cover:
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report: coverage.html"

clean:
	@echo "Cleaning build artifacts..."
	@go clean
	@rm -f $(BUILD_DIR)/$(BINARY_NAME)
	@rm -f coverage.out coverage.html
	@echo "✅ Clean complete"

# Development helpers
dev: run

fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "✅ Code formatted"

vet:
	@echo "Running go vet..."
	@go vet ./...
	@echo "✅ go vet passed"

lint: fmt vet

# Build for different platforms
build-linux:
	@echo "Building for Linux AMD64..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	@echo "✅ Binary built: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64"

build-darwin:
	@echo "Building for macOS ARM64..."
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
		go build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .
	@echo "✅ Binary built: $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64"

build-windows:
	@echo "Building for Windows AMD64..."
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
		go build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .
	@echo "✅ Binary built: $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe"

build-all: build-linux build-darwin build-windows
	@echo ""
	@echo "✅ All binaries built successfully!"
	@ls -lh $(BUILD_DIR)/$(BINARY_NAME)-*
