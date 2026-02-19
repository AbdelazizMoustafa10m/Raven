# Raven - AI Workflow Orchestration Command Center
# GNU Makefile with build targets and ldflags for version injection.

# Project metadata
MODULE   := $(shell go list -m)
BINARY   := raven
BUILD_DIR := dist

# Version injection (extracted from git at build time)
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE     := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

# Build flags
LDFLAGS  := -s -w \
    -X $(MODULE)/internal/buildinfo.Version=$(VERSION) \
    -X $(MODULE)/internal/buildinfo.Commit=$(COMMIT) \
    -X $(MODULE)/internal/buildinfo.Date=$(DATE)

# Debug build ldflags (no -s -w, keeps symbols for dlv)
LDFLAGS_DEBUG := \
    -X $(MODULE)/internal/buildinfo.Version=$(VERSION) \
    -X $(MODULE)/internal/buildinfo.Commit=$(COMMIT) \
    -X $(MODULE)/internal/buildinfo.Date=$(DATE)

GOFLAGS  := CGO_ENABLED=0

.PHONY: all build test vet lint tidy clean install fmt bench run-version build-debug release-snapshot completions manpages

all: tidy vet test build

build:
	mkdir -p $(BUILD_DIR)
	$(GOFLAGS) go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/raven/

test:
	go test ./... -race -count=1

vet:
	go vet ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

fmt:
	gofmt -s -w .

clean:
	rm -rf $(BUILD_DIR)

install:
	$(GOFLAGS) go install -ldflags "$(LDFLAGS)" ./cmd/raven/

bench:
	go test ./... -bench=. -benchmem -count=1

# Development helper: build then run version subcommand
run-version: build
	./$(BUILD_DIR)/$(BINARY) version

# Release snapshot: build for all platforms without publishing (requires goreleaser)
release-snapshot:
	goreleaser build --snapshot --clean

# Generate shell completion scripts for all supported shells into completions/
completions:
	go run ./scripts/gen-completions completions

# Generate Unix man pages for all commands into man/man1/
manpages:
	go run ./scripts/gen-manpages man/man1

# Debug build (with symbols, for dlv debugger)
build-debug:
	mkdir -p $(BUILD_DIR)
	$(GOFLAGS) go build -gcflags="all=-N -l" -ldflags "$(LDFLAGS_DEBUG)" -o $(BUILD_DIR)/$(BINARY)-debug ./cmd/raven/