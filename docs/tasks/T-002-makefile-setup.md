# T-002: Makefile with Build Targets and ldflags

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Small: 2-4hrs |
| Dependencies | T-001 |
| Blocked By | T-001 |
| Blocks | T-003, T-079, T-085 |

## Goal
Create a Makefile that provides standardized build, test, lint, and clean targets with ldflags for version injection. This Makefile is the primary developer interface for building Raven and is consumed by CI (T-085) and GoReleaser (T-079).

## Background
Per PRD Section 6.2, the project root contains a `Makefile`. Per PRD Technical Decisions, Raven uses `CGO_ENABLED=0` for pure Go cross-compilation and injects version information via `-ldflags`. The Makefile must capture the git commit SHA, build date, and version tag at build time and pass them to the `internal/buildinfo` package (T-003) via `-X` ldflags. The CLAUDE.md verification commands (`go build ./cmd/raven/`, `go vet ./...`, `go test ./...`, `go mod tidy`) should all be available as Makefile targets.

## Technical Specifications
### Implementation Approach
Create a `Makefile` at the project root with GNU Make syntax. Define variables for the Go module path, binary name, build directory, version (from git tags or default), commit (from git rev-parse), and date (from `date -u`). The `build` target compiles with ldflags injecting these values into `internal/buildinfo`. Include targets for test, vet, lint, tidy, clean, install, and a convenience `all` target.

### Key Components
- **Makefile**: GNU Make file at project root
- **Version variables**: Extracted from git tags, commit hash, and build timestamp
- **ldflags string**: `-X` flags for `internal/buildinfo.Version`, `.Commit`, `.Date`
- **Build targets**: build, test, vet, lint, tidy, clean, install, all, fmt, bench

### API/Interface Contracts
```makefile
# Makefile

# Project metadata
MODULE   := $(shell go list -m)
BINARY   := raven
BUILD_DIR := dist

# Version injection
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE     := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

# Build flags
LDFLAGS  := -s -w \
    -X $(MODULE)/internal/buildinfo.Version=$(VERSION) \
    -X $(MODULE)/internal/buildinfo.Commit=$(COMMIT) \
    -X $(MODULE)/internal/buildinfo.Date=$(DATE)

GOFLAGS  := CGO_ENABLED=0

.PHONY: all build test vet lint tidy clean install fmt bench

all: tidy vet test build

build:
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

# Development helper: build + run version
run-version: build
	./$(BUILD_DIR)/$(BINARY) version

# Debug build (with symbols, for dlv debugger)
build-debug:
	$(GOFLAGS) go build -gcflags="all=-N -l" -o $(BUILD_DIR)/$(BINARY)-debug ./cmd/raven/
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| GNU Make | 3.81+ | Build automation (ships with macOS/Linux) |
| Go | 1.24+ | Build toolchain |
| git | 2.x | Version extraction (git describe, rev-parse) |

## Acceptance Criteria
- [ ] `Makefile` exists at project root
- [ ] `make build` produces a binary at `dist/raven`
- [ ] `make build` injects version, commit, and date via ldflags
- [ ] `make test` runs all tests with race detector
- [ ] `make vet` runs `go vet ./...`
- [ ] `make tidy` runs `go mod tidy`
- [ ] `make clean` removes the `dist/` directory
- [ ] `make all` runs tidy, vet, test, and build in sequence
- [ ] `make install` installs the binary to `$GOPATH/bin`
- [ ] `make build-debug` produces a debug binary with symbols
- [ ] `CGO_ENABLED=0` is set for all build and install targets
- [ ] The ldflags `-X` paths match the variable names in `internal/buildinfo` (coordinated with T-003)

## Testing Requirements
### Unit Tests
- No Go unit tests (Makefile is declarative)

### Integration Tests
- `make build` produces a binary that runs without error
- Binary produced by `make build` reports non-empty version when T-003 and T-007 are complete
- `make clean && make build` produces identical binary

### Edge Cases to Handle
- Building without git (no `.git` directory): version defaults to "dev", commit to "unknown"
- Building with dirty working tree: `git describe --dirty` appends "-dirty" to version
- Building on systems without `golangci-lint`: `make lint` should fail with a clear error, not silently skip
- GNU Make vs BSD Make: use POSIX-compatible syntax where possible (avoid GNU-specific features)

## Implementation Notes
### Recommended Approach
1. Create `Makefile` at project root
2. Define version extraction variables using `$(shell ...)` commands
3. Define the LDFLAGS string with `-X` flags pointing to `$(MODULE)/internal/buildinfo.*`
4. Create each target: build, test, vet, lint, tidy, fmt, clean, install, bench, all
5. Create the `dist/` output directory in the build target
6. Add `.PHONY` declarations for all targets
7. Add `build-debug` target without `-s -w` flags
8. Test: `make clean && make all`

### Potential Pitfalls
- The `$(MODULE)` variable must exactly match the module path in `go.mod`. If the module path changes, ldflags break silently (variables are not found, default to empty string).
- The `-s -w` ldflags strip debug symbols. The `build-debug` target omits these for `dlv` debugging.
- macOS ships with BSD Make but `make` is aliased to GNU Make when Xcode CLI tools are installed. Test both if possible.
- Ensure the `dist/` directory is created by the build target (`mkdir -p $(BUILD_DIR)` before `go build`).

### Security Considerations
- The `-s` ldflag strips symbol table, making reverse engineering slightly harder in release builds
- Do not embed sensitive values (API keys, tokens) via ldflags -- only build metadata

## References
- [Go Build Modes - ldflags](https://pkg.go.dev/cmd/go#hdr-Build_modes)
- [GNU Make Manual](https://www.gnu.org/software/make/manual/make.html)
- [PRD Section 6.2 - Project Structure](docs/prd/PRD-Raven.md)
- [PRD Technical Decisions - CGO_ENABLED=0](docs/prd/PRD-Raven.md)