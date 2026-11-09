# Makefile for HuddleScope monorepo
# HS-001 baseline; see docs/architecture.md for layout.

SHELL := /bin/bash
.ONESHELL:
.SHELLFLAGS := -e -o pipefail -c
.DELETE_ON_ERROR:
.SUFFIXES: .go .mod

GO ?= go
GOFMT ?= gofmt
GOLINT ?= golangci-lint run ./...
GOCOVER ?= $(GO) tool cover
GOSEC ?= $(GO) run github.com/securego/gosec/v2/cmd/gosec@v2.29.0
GOVULNCHECK ?= $(GO) run golang.org/x/vuln/cmd/govulncheck@v1.9.0

PREFIX ?= dist
BINARIES ?= $(PREFIX)/ingest $(PREFIX)/export
PACKAGE := $(shell $(GO) list ./... 2>/dev/null | grep -v '/vendor/' || true)

# Default target: help
help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Format
fmt: ## Format Go files with gofmt (and goimports if available)
	@$(GOFMT) -l -w $$(find . -type f -name '*.go' -not -path './vendor/*' -not -path './dist/*')

# Lint
lint: ## Run golangci-lint on all packages
	$(GOLINT)

# Test
test: ## Run vet and tests with race detector
	$(GO) vet ./...
	$(GO) test ./... -race

# Coverage
coverage: ## Run tests with coverage report
	$(GO) test ./... -coverprofile=coverage.out -covermode=atomic
	$(GOCOVER) -func=coverage.out

# Security
# Note: gosec and govulncheck require Go 1.27.2 compatibility. 
# Current system uses Go 1.27.3. Tool binaries compiled with Go 1.27.2 
# cannot read Go 1.27.3 cache format. 
# Workaround: run with cache disabled after cleaning.
security: ## Run gosec security scanner and govulncheck
	@$(GO) clean -cache
	$(GOSEC) ./...
	@$(GO) clean -cache
	$(GOVULNCHECK) ./...

# Coverage check
check-coverage: ## Enforce >=80% coverage on internal packages
	@bash /tmp/coverage-check.sh

# Build
build: ## Build binaries to dist/
	@mkdir -p $(PREFIX)
	$(GO) build -o $(PREFIX)/ ./cmd/...

# Check (CI pipeline)
check: ## Run format, lint, test, coverage check, and security checks
	@set -e; \
	$(MAKE) fmt; \
	$(MAKE) lint; \
	$(MAKE) test; \
	$(MAKE) coverage; \
	$(MAKE) check-coverage; \
	$(MAKE) security; \
	echo "All checks passed."

# Clean
clean: ## Remove build artifacts
	$(RM) -r $(PREFIX)
	$(RM) coverage.out

# Phony targets
.PHONY: help fmt lint test coverage check-coverage security build check clean