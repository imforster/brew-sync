# brew-sync Makefile

BINARY_NAME := brew-sync
GO := go
GOFLAGS :=

# Version info injected at build time
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -X brew-sync/cmd.Version=$(VERSION) -X brew-sync/cmd.Commit=$(COMMIT)

# Build output directory
BUILD_DIR := build

.PHONY: all build clean test test-verbose test-property test-race lint fmt vet tidy install uninstall help

## Default target
all: fmt vet test build

## Build the binary
build:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) .

## Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)
	$(GO) clean -testcache

## Run all tests
test:
	$(GO) test ./... $(GOFLAGS)

## Run tests with verbose output
test-verbose:
	$(GO) test -v ./... $(GOFLAGS)

## Run only property-based tests (rapid)
test-property:
	$(GO) test -v -run "Property" ./... $(GOFLAGS)

## Run tests with race detector
test-race:
	$(GO) test -race ./... $(GOFLAGS)

## Run tests with coverage and generate report
test-cover:
	$(GO) test -coverprofile=$(BUILD_DIR)/coverage.out ./...
	$(GO) tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html
	@echo "Coverage report: $(BUILD_DIR)/coverage.html"

## Format source code
fmt:
	$(GO) fmt ./...

## Run go vet
vet:
	$(GO) vet ./...

## Run golangci-lint (requires golangci-lint installed)
lint:
	golangci-lint run ./...

## Tidy module dependencies
tidy:
	$(GO) mod tidy

## Install the binary to $GOPATH/bin
install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME)

## Uninstall the binary from $GOPATH/bin
uninstall:
	rm -f $(GOPATH)/bin/$(BINARY_NAME)

## Show help
help:
	@echo "brew-sync Makefile targets:"
	@echo ""
	@echo "  make              Build after formatting, vetting, and testing"
	@echo "  make build        Build the binary to $(BUILD_DIR)/$(BINARY_NAME)"
	@echo "  make clean        Remove build artifacts and test cache"
	@echo "  make test         Run all tests"
	@echo "  make test-verbose Run tests with verbose output"
	@echo "  make test-property Run only property-based tests"
	@echo "  make test-race    Run tests with the race detector"
	@echo "  make test-cover   Run tests with coverage report"
	@echo "  make fmt          Format source code"
	@echo "  make vet          Run go vet"
	@echo "  make lint         Run golangci-lint"
	@echo "  make tidy         Tidy module dependencies"
	@echo "  make install      Install binary to GOPATH/bin"
	@echo "  make uninstall    Remove binary from GOPATH/bin"
