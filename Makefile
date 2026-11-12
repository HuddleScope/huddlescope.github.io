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
GOSEC ?= ~/go/bin/gosec
GOVULNCHECK ?= ~/go/bin/govulncheck

PREFIX ?= dist
BINARIES ?= $(PREFIX)/ingest $(PREFIX)/export $(PREFIX)/heartbeat
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
# Note: gosec and govulncheck binaries live in ~/go/bin (installed via
# `go install` locally; CI installs the same in `.github/workflows/ci.yml`).
# Override with GOSEC / GOVULNCHECK env vars if installed elsewhere.
security: ## Run gosec security scanner and govulncheck (fails CI on findings)
	@$(GO) clean -cache
	$(GOSEC) ./...
	@$(GO) clean -cache
	$(GOVULNCHECK) ./...

# Coverage check
check-coverage: ## Enforce >=80% coverage on internal packages
	@bash ./scripts/coverage-check.sh

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

# Site (Hugo)
site: ## Build Hugo site from web/ directory
	$(MAKE) site-check-hugo
	hugo -s web

site-check-hugo:
	@command -v hugo >/dev/null 2>&1 || { echo "Error: hugo not found in PATH"; exit 1; }
	@echo "Hugo version: $$(hugo version | head -1)"

site-test: ## Run site build verification (success/failure/idempotency)
	@bash scripts/site-test.sh

site-watch: ## Run Hugo in watch mode for local development
	$(MAKE) site-check-hugo
	hugo server -s web

# Phony targets
.PHONY: help fmt lint test coverage check-coverage security build check clean site site-check-hugo site-test site-watch