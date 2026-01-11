# runc-go Makefile

# Build configuration
BINARY_NAME := runc-go
GO := go
GOFLAGS := -ldflags="-s -w"
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Directories
BUILD_DIR := build
COVERAGE_DIR := coverage

# Default target
.PHONY: all
all: build

# Build the binary
.PHONY: build
build:
	$(GO) build $(GOFLAGS) -o $(BINARY_NAME) .

# Build with debug symbols
.PHONY: build-debug
build-debug:
	$(GO) build -o $(BINARY_NAME) .

# Run all tests
.PHONY: test
test:
	$(GO) test -v ./...

# Run tests with race detection
.PHONY: test-race
test-race:
	$(GO) test -race -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	@mkdir -p $(COVERAGE_DIR)
	$(GO) test -coverprofile=$(COVERAGE_DIR)/coverage.out ./...
	$(GO) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "Coverage report: $(COVERAGE_DIR)/coverage.html"

# Run tests with coverage summary
.PHONY: coverage
coverage:
	$(GO) test -cover ./...

# Run only unit tests (skip integration tests requiring root)
.PHONY: test-unit
test-unit:
	$(GO) test -v -short ./...

# Run integration tests (requires root)
.PHONY: test-integration
test-integration:
	sudo $(GO) test -v ./...

# Run linting
.PHONY: lint
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "Installing golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	}
	golangci-lint run ./...

# Format code
.PHONY: fmt
fmt:
	$(GO) fmt ./...

# Vet code
.PHONY: vet
vet:
	$(GO) vet ./...

# Run static analysis
.PHONY: check
check: fmt vet lint

# Clean build artifacts
.PHONY: clean
clean:
	rm -f $(BINARY_NAME)
	rm -rf $(BUILD_DIR)
	rm -rf $(COVERAGE_DIR)
	$(GO) clean

# Install the binary
.PHONY: install
install: build
	sudo cp $(BINARY_NAME) /usr/local/bin/

# Uninstall the binary
.PHONY: uninstall
uninstall:
	sudo rm -f /usr/local/bin/$(BINARY_NAME)

# Generate OCI spec
.PHONY: spec
spec: build
	./$(BINARY_NAME) spec

# Run quick validation
.PHONY: validate
validate: fmt vet test

# Show help
.PHONY: help
help:
	@echo "runc-go Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all             Build the binary (default)"
	@echo "  build           Build the binary with optimizations"
	@echo "  build-debug     Build with debug symbols"
	@echo "  test            Run all tests"
	@echo "  test-race       Run tests with race detection"
	@echo "  test-coverage   Run tests and generate coverage report"
	@echo "  coverage        Show coverage summary"
	@echo "  test-unit       Run unit tests only (no root required)"
	@echo "  test-integration Run integration tests (requires root)"
	@echo "  lint            Run golangci-lint"
	@echo "  fmt             Format code"
	@echo "  vet             Run go vet"
	@echo "  check           Run fmt, vet, and lint"
	@echo "  clean           Clean build artifacts"
	@echo "  install         Install binary to /usr/local/bin"
	@echo "  uninstall       Remove installed binary"
	@echo "  spec            Generate default OCI spec"
	@echo "  validate        Run fmt, vet, and test"
	@echo "  help            Show this help"
