# Makefile for HuddleScope monorepo
# HS-001 baseline.

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
	$(GOSEC) -exclude-rules="internal/adapters/storage/sqlite/backup.go:G304,G703;internal/adapters/storage/sqlite/retentionpkg.go:G304;internal/cli/retention.go:G104" ./...
	@$(GO) clean -cache
	$(GOVULNCHECK) ./...

# Coverage check
check-coverage: ## Enforce >=80% coverage on internal packages
	@bash ./scripts/coverage-check.sh

# Build
# Build stamping (issue #161 AC-1/AC-3): overridable GOOS/GOARCH/GOARM/VERSION/
# COMMIT/BUILT_AT. Defaults keep `make build` producing exactly one binary at
# dist/huddlescope with a sane stamped version resolved from the repo.
# BUILT_AT defaults to SOURCE_DATE_EPOCH (if set) for reproducible builds,
# else the HEAD commit timestamp, else "dev" — a deterministic stamp is what
# makes same-commit + same-version builds byte-identical (AC-3).
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GOARM ?=
CGO_ENABLED ?= 0
VERSION ?= $(shell bash scripts/resolve-version.sh 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILT_AT ?= $(shell if [ -n "${SOURCE_DATE_EPOCH:-}" ]; then date -u -r "${SOURCE_DATE_EPOCH}" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d "@${SOURCE_DATE_EPOCH}" +%Y-%m-%dT%H:%M:%SZ; elif git log -1 --format=%ct >/dev/null 2>&1; then git log -1 --format=%ct | { read -r ts; date -u -r "$$ts" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d "@$$ts" +%Y-%m-%dT%H:%M:%SZ; } || echo dev; else echo dev; fi)

build: ## Build unified huddlescope binary to dist/ (stamped, static, -trimpath)
	@mkdir -p $(PREFIX)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) GOARM=$(GOARM) \
	$(GO) build -trimpath \
		-ldflags="-s -w \
			-X github.com/HuddleScope/huddlescope.github.io/internal/cli.cliVersion=$(VERSION) \
			-X github.com/HuddleScope/huddlescope.github.io/internal/cli.cliCommit=$(COMMIT) \
			-X github.com/HuddleScope/huddlescope.github.io/internal/cli.cliBuiltAt=$(BUILT_AT)" \
		-o $(PREFIX)/huddlescope ./cmd/huddlescope

# cross-build builds the release matrix (linux amd64/arm64/armv7l + darwin/arm64
# dev artifact) with the same stamping/flags as the release workflow. Convenience
# for local reproducible-build verification (AC-3/AC-7).
# Targets are enumerated as "os:arch[:goarm]" to avoid shell-splitting on "/".
RELEASE_TARGETS ?= linux:amd64 linux:arm64 linux:arm:7 darwin:arm64
cross-build: ## Build the release matrix (static, stamped) into dist/
	@mkdir -p $(PREFIX)
	@for target in $(RELEASE_TARGETS); do \
		os=""; arch=""; goarm=""; \
		IFS=':' read -r os arch goarm <<< "$$target"; \
		name="huddlescope_$(VERSION)_$${os}_$${arch}"; \
		if [ -n "$$goarm" ]; then name="$${name}v$${goarm}"; fi; \
		echo "Building $${os}/$${arch} -> $(PREFIX)/$${name}"; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$${os} GOARCH=$${arch} GOARM=$${goarm} \
		$(GO) build -trimpath \
			-ldflags="-s -w \
				-X github.com/HuddleScope/huddlescope.github.io/internal/cli.cliVersion=$(VERSION) \
				-X github.com/HuddleScope/huddlescope.github.io/internal/cli.cliCommit=$(COMMIT) \
				-X github.com/HuddleScope/huddlescope.github.io/internal/cli.cliBuiltAt=$(BUILT_AT)" \
			-o $(PREFIX)/$${name} ./cmd/huddlescope; \
	done

# docker-build builds the multi-stage runtime image locally (AC-5). Not run in
# CI (no Docker on check runs); the Dockerfile text-invariant unit test is the gate.
docker-build: ## Build the huddlescope runtime image locally
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILT_AT=$(BUILT_AT) -t huddlescope:$(VERSION) .

build-huddlescope: build ## Build unified huddlescope binary to dist/ (alias)

retention: build ## Run retention locally: export package to archive/retention, then apply
	./$(PREFIX)/huddlescope retention run

.PHONY: build build-huddlescope cross-build docker-build retention

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

site-seed: ## Copy committed sample fixtures into web/data + web/content/stories for local development
	@mkdir -p web/data web/content/stories
	cp web/fixtures/homepage.json web/fixtures/feeds.json web/data/
	cp web/fixtures/content/stories/*.md web/content/stories/
	@echo "Seeded web/data + web/content/stories with fixtures (untracked; rm to remove)"

site-watch: ## Run Hugo in watch mode for local development
	$(MAKE) site-check-hugo
	hugo server -s web

release-gate: ## Run the end-to-end release gate script
	@bash scripts/release-gate.sh

# Phony targets
.PHONY: help fmt lint test coverage check-coverage security build check clean site site-check-hugo site-test site-seed site-watch release-gate retention cross-build docker-build