# T-001: Go Project Initialization and Module Setup

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 4-8hrs |
| Dependencies | None |
| Blocked By | None |
| Blocks | T-002, T-003, T-004, T-005, T-006, T-009, T-013, T-015 |

## Goal
Initialize the Go project with module, directory structure, entry point, and core dependency declarations. This is the absolute foundation -- every other task depends on the module existing, compiling, and having the canonical directory layout defined by the PRD.

## Background
Per PRD Section 6.2, Raven is structured as a standard Go project with `cmd/raven/main.go` as the entry point and all internal packages under `internal/`. The PRD specifies Go 1.24+ as the minimum version (Section 6.1) and `CGO_ENABLED=0` for pure Go cross-compilation (Section 6, Technical Decisions). The module path must be chosen carefully as it is baked into the binary via ldflags (used by T-003 and T-079).

The project must compile and pass `go vet` from this point forward. The entry point starts minimal -- it will be extended by T-006 (Cobra root command) and later tasks.

## Technical Specifications
### Implementation Approach
Run `go mod init` with the project module path, create the directory skeleton matching PRD Section 6.2, write a minimal `cmd/raven/main.go` that prints a placeholder message, and declare all required third-party dependencies in `go.mod`. The directory structure includes stub `.gitkeep` or `doc.go` files in each `internal/` subpackage so the layout is established.

### Key Components
- **go.mod**: Module declaration with Go 1.24+ directive and all direct dependencies
- **cmd/raven/main.go**: Minimal entry point (will be replaced by Cobra in T-006)
- **internal/ directory tree**: All subpackages from PRD Section 6.2 with `doc.go` package declarations
- **testdata/**: Test fixture directory
- **.gitignore**: Go-specific ignores plus `.raven/` state directory

### API/Interface Contracts
```go
// cmd/raven/main.go (minimal, replaced by T-006)
package main

import "fmt"

func main() {
    fmt.Println("raven - AI workflow orchestration command center")
}
```

```go
// internal/cli/doc.go (example for each subpackage)
// Package cli implements Cobra CLI commands for Raven.
package cli
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| Go | 1.24+ | Language runtime and toolchain |
| github.com/spf13/cobra | v1.10+ | CLI framework (declared now, used from T-006) |
| github.com/charmbracelet/bubbletea | v1.2+ | TUI framework |
| github.com/charmbracelet/lipgloss | v1.0+ | TUI styling |
| github.com/charmbracelet/bubbles | latest | TUI components |
| github.com/charmbracelet/huh | v0.6+ | TUI form builder |
| github.com/charmbracelet/log | latest | Structured logging |
| github.com/BurntSushi/toml | v1.5.0 | TOML config parsing |
| golang.org/x/sync | v0.19+ | errgroup for concurrency |
| github.com/bmatcuk/doublestar/v4 | v4.10+ | Glob matching |
| github.com/stretchr/testify | v1.9+ | Test assertions |
| github.com/cespare/xxhash/v2 | v2 | Fast content hashing |

## Acceptance Criteria
- [ ] `go.mod` exists with module path and Go 1.24+ directive
- [ ] `go.sum` is generated with all dependency checksums
- [ ] `cmd/raven/main.go` compiles and runs (`go run ./cmd/raven/`)
- [ ] `go build ./cmd/raven/` produces a binary
- [ ] `go vet ./...` passes with zero warnings
- [ ] All `internal/` subpackages exist with valid `doc.go` or placeholder files: `cli`, `config`, `workflow`, `agent`, `task`, `loop`, `review`, `prd`, `pipeline`, `git`, `tui`, `buildinfo`
- [ ] `testdata/` directory exists
- [ ] `.gitignore` includes: compiled binaries, `.raven/`, `dist/`, vendor/, IDE files
- [ ] `go mod tidy` produces no diff (modules are clean)
- [ ] All declared dependencies resolve successfully

## Testing Requirements
### Unit Tests
- `go build ./cmd/raven/` succeeds (compilation test)
- `go vet ./...` returns exit code 0
- `go mod tidy` produces no changes (idempotency check)

### Integration Tests
- Built binary runs and exits 0 (placeholder behavior)

### Edge Cases to Handle
- Module path must not conflict with existing Go modules on pkg.go.dev
- Go 1.24+ must be enforced in `go.mod` (use `go 1.24` directive, not `go 1.24.0` to allow patch versions)
- Ensure no `init()` functions are introduced (per CLAUDE.md conventions)

## Implementation Notes
### Recommended Approach
1. Create the project root directory structure:
   ```
   cmd/raven/
   internal/{cli,config,workflow,agent,task,loop,review,prd,pipeline,git,tui,buildinfo}/
   templates/go-cli/
   testdata/
   ```
2. Run `go mod init github.com/ravenco/raven` (or chosen module path)
3. Write `cmd/raven/main.go` with a minimal main function
4. Create `doc.go` in each `internal/` subpackage with package declaration and doc comment
5. Run `go get` for all direct dependencies listed in the PRD tech stack
6. Run `go mod tidy` to clean up
7. Create `.gitignore` with Go conventions plus Raven-specific entries
8. Verify: `go build ./cmd/raven/ && go vet ./...`

### Potential Pitfalls
- The module path chosen here is permanent -- it appears in ldflags, import paths, and GoReleaser config. Choose carefully and coordinate with T-002 and T-079.
- Do NOT add `replace` directives in `go.mod` -- all dependencies must resolve from their canonical module paths.
- The `go 1.24` directive in `go.mod` means users with Go < 1.24 will get a clear error message. This is intentional per PRD.
- Adding `doc.go` files ensures `go vet ./...` does not skip empty packages. Each file must have a valid `package` declaration matching the directory name.

### Security Considerations
- Verify all dependencies via `go mod verify` after initial setup
- Run `go mod audit` or equivalent to check for known vulnerabilities in dependencies
- The `.gitignore` must include `.raven/state/` to prevent accidental commit of workflow state containing potentially sensitive data

## References
- [Go Modules Reference](https://go.dev/ref/mod)
- [Go 1.24 Release Notes](https://go.dev/doc/go1.24)
- [PRD Section 6.1 - Recommended Stack](docs/prd/PRD-Raven.md)
- [PRD Section 6.2 - Project Structure](docs/prd/PRD-Raven.md)
- [Effective Go - Package Names](https://go.dev/doc/effective_go#package-names)