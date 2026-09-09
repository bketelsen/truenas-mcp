.PHONY: all build install run clean fmt vet lint lint-version-check test test-cover tidy check verify-static verify ci completions manpages release-check snapshot bump help

# Build variables
BINARY     := truenas-mcp
MODULE     := truenas-mcp
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS    := -ldflags "-s -w -X $(MODULE)/version.Version=$(VERSION) -X $(MODULE)/version.Commit=$(COMMIT) -X $(MODULE)/version.Date=$(BUILD_TIME) -X $(MODULE)/version.BuiltBy=make"

# Go commands
GO    := go
GOFMT := gofmt

# Pinned golangci-lint release, read from mise.toml — the single source of
# every tool pin: `mise install` provisions it locally and in CI
# (jdx/mise-action), verified against mise.lock. Bump it there in a dedicated
# commit; never edit this line. Compared against `golangci-lint version
# --short`, which prints the bare "MAJOR.MINOR.PATCH".
GOLANGCI_LINT_VERSION := $(strip $(shell sed -n 's/^golangci-lint = "\(.*\)"/\1/p' mise.toml))
# The Go release this module is built with, from go.mod's go line — the only
# Go pin (mise reads the same line). golangci-lint must be built with a Go at
# least this new, or its embedded gofmt and typechecker disagree with the
# toolchain.
GO_VERSION := $(strip $(shell sed -n 's/^go \(.*\)/\1/p' go.mod))
GOFILES    := $(shell find . -type f -name '*.go' -not -path "./vendor/*")

all: fmt vet build

## build: Build the truenas-mcp binary into build/
build:
	$(GO) build $(LDFLAGS) -o build/$(BINARY) .

## install: Install truenas-mcp to GOPATH/bin
install:
	$(GO) install $(LDFLAGS) .

## run: Build and run `truenas-mcp serve`
run: build
	./build/$(BINARY) serve

## clean: Remove build artifacts
clean:
	rm -rf build/ dist/ completions/ manpages/ coverage.out coverage.html
	$(GO) clean

## fmt: Format Go source files
fmt:
	$(GOFMT) -s -w $(GOFILES)

## vet: Run go vet
vet:
	$(GO) vet ./...

## lint: Run linter (requires golangci-lint; fails if the mise.toml pin is missing or the installed release differs)
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		$(MAKE) --no-print-directory lint-version-check && \
		golangci-lint run; \
	else \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) is required for make lint (not installed)"; \
		echo "install with: mise install"; \
		exit 1; \
	fi

## lint-version-check: Fail unless the installed golangci-lint is the mise.toml pin and was built with a Go no older than go.mod's go line
lint-version-check:
	@test -n "$(GOLANGCI_LINT_VERSION)" || { echo "mise.toml pins no golangci-lint"; exit 1; }
	@installed="$$(golangci-lint version --short 2>/dev/null)" || { \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) is required (not installed; run: mise install)"; exit 1; }; \
	if [ "$$installed" != "$(GOLANGCI_LINT_VERSION)" ]; then \
		echo "expected golangci-lint $(GOLANGCI_LINT_VERSION), found $$installed (run: mise install)"; \
		exit 1; \
	fi; \
	built="$$(golangci-lint version 2>/dev/null | sed -n 's/.*built with go\([0-9.]*\).*/\1/p')"; \
	if [ -n "$$built" ] && [ "$$(printf '%s\n%s\n' "$(GO_VERSION)" "$$built" | sort -V | head -1)" != "$(GO_VERSION)" ]; then \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) was built with go$$built, older than go.mod's go$(GO_VERSION): bump golangci-lint first"; \
		exit 1; \
	fi

## test: Run tests with the race detector
test:
	$(GO) test -race -count=1 ./...

## test-cover: Run tests with coverage and write coverage.html
test-cover:
	$(GO) test -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

## tidy: Tidy and verify go modules
tidy:
	$(GO) mod tidy
	$(GO) mod verify

## check: Run fmt, lint, and test
check: fmt lint test

## verify-static: The non-mutating static checks shared by verify and ci (tidy diff, vet, gofmt -l, exact-pin lint)
verify-static:
	@echo "==> verify: go.mod is tidy"
	$(GO) mod tidy -diff
	@echo "==> verify: go vet"
	$(GO) vet ./...
	@echo "==> verify: gofmt"
	test -z "$$($(GOFMT) -s -l $$(git ls-files '*.go'))"
	@echo "==> lint (golangci-lint $(GOLANGCI_LINT_VERSION))"
	$(MAKE) lint-version-check
	golangci-lint run

## verify: Credential-free, non-mutating gate for read-only reviewers (verify-static plus tests)
verify: verify-static
	@echo "==> tests"
	$(GO) test -count=1 ./...

## ci: Run the credential-free CI gate (verify-static, then race tests and cross-build)
ci: verify-static
	@echo "==> race detector"
	$(GO) test -race -count=1 ./...
	@echo "==> cross-architecture build"
	GOOS=linux GOARCH=amd64 $(MAKE) build
	GOOS=linux GOARCH=arm64 $(MAKE) build
	GOOS=darwin GOARCH=arm64 $(MAKE) build
	@echo "==> CI gate passed"

## completions: Generate shell completions into completions/ (used by GoReleaser)
completions:
	./scripts/completions.sh

## manpages: Generate the gzipped man page into manpages/ (used by GoReleaser)
manpages:
	./scripts/manpages.sh

## release-check: Validate .goreleaser.yaml (requires goreleaser-pro on PATH)
release-check:
	goreleaser check

## snapshot: Build a local, unpublished release into dist/ (requires goreleaser-pro on PATH)
snapshot:
	goreleaser release --snapshot --clean

## bump: Gate, tag the next semantic version with svu, and push the tag (triggers the release workflow)
bump:
	@$(MAKE) build
	@$(MAKE) test
	@$(MAKE) fmt
	$(MAKE) lint
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Working directory is not clean. Please commit or stash changes before bumping version."; \
		exit 1; \
	fi
	@echo "Creating new tag..."
	@version=$$(svu next); \
		git tag -a $$version -m "Version $$version"; \
		echo "Tagged version $$version"; \
		echo "Pushing tag $$version to origin..."; \
		git push origin $$version

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'
