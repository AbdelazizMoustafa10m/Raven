# T-003: Build Info Package -- internal/buildinfo

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Small: 1-2hrs |
| Dependencies | T-001 |
| Blocked By | T-001 |
| Blocks | T-007, T-079 |

## Goal
Create the `internal/buildinfo` package that exposes version, commit hash, and build date as package-level variables populated by `-ldflags` at build time. This package is the single source of truth for Raven's build identity, consumed by the version command (T-007), the TUI title bar (T-066), and GoReleaser (T-079).

## Background
Per PRD Section 6.2, `internal/buildinfo/buildinfo.go` provides "Version, commit, build date (ldflags)." The Makefile (T-002) and GoReleaser (T-079) inject values into these variables at compile time using `-X` linker flags. If not injected (e.g., `go install` without ldflags), the variables default to "dev", "unknown", and "unknown" respectively to ensure the binary always has usable output.

Per PRD Section 5.11, `raven version [--json]` displays this information, so the package must also provide a structured representation suitable for JSON marshaling.

## Technical Specifications
### Implementation Approach
Create `internal/buildinfo/buildinfo.go` with three unexported `var` declarations (lowercase) that are populated by ldflags, plus exported accessor functions and a `Info` struct for structured access. The variable names must be exported (capitalized) because `-X` only works with exported package-level variables.

### Key Components
- **Version, Commit, Date**: Exported string variables with default values, set via `-ldflags -X`
- **Info struct**: Structured build information for JSON serialization
- **GetInfo()**: Returns a populated Info struct
- **String()**: Returns a human-readable one-line version string

### API/Interface Contracts
```go
// internal/buildinfo/buildinfo.go

// Package buildinfo provides version, commit, and build date information
// injected at compile time via -ldflags.
package buildinfo

import "fmt"

// Version is the semantic version tag (e.g., "2.0.0", "dev").
// Set via: -ldflags "-X <module>/internal/buildinfo.Version=<value>"
var Version = "dev"

// Commit is the short git commit hash (e.g., "a1b2c3d").
// Set via: -ldflags "-X <module>/internal/buildinfo.Commit=<value>"
var Commit = "unknown"

// Date is the UTC build timestamp in ISO 8601 format.
// Set via: -ldflags "-X <module>/internal/buildinfo.Date=<value>"
var Date = "unknown"

// Info holds structured build information.
type Info struct {
    Version string `json:"version"`
    Commit  string `json:"commit"`
    Date    string `json:"date"`
}

// GetInfo returns the current build information as a structured type.
func GetInfo() Info {
    return Info{
        Version: Version,
        Commit:  Commit,
        Date:    Date,
    }
}

// String returns a human-readable version string.
// Example: "raven v2.0.0 (commit: a1b2c3d, built: 2026-02-17T10:00:00Z)"
func (i Info) String() string {
    return fmt.Sprintf("raven v%s (commit: %s, built: %s)", i.Version, i.Commit, i.Date)
}
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| fmt | stdlib | String formatting for human-readable output |
| encoding/json | stdlib | JSON marshaling via struct tags (used by consumers) |

## Acceptance Criteria
- [ ] `internal/buildinfo/buildinfo.go` exists with package declaration
- [ ] `Version`, `Commit`, `Date` are exported package-level `var` declarations with string type
- [ ] Default values are "dev", "unknown", "unknown" respectively
- [ ] `Info` struct has JSON struct tags for all fields
- [ ] `GetInfo()` returns an `Info` struct populated from the package variables
- [ ] `Info.String()` returns the format: `raven v{version} (commit: {commit}, built: {date})`
- [ ] Building with ldflags overrides defaults: `go build -ldflags "-X .../buildinfo.Version=1.0.0" ./cmd/raven/`
- [ ] Building without ldflags uses defaults (no panic, no empty strings)
- [ ] `go vet ./internal/buildinfo/...` passes
- [ ] Unit tests achieve 100% coverage

## Testing Requirements
### Unit Tests
- `GetInfo()` returns Info with default values when no ldflags provided
- `Info.String()` with defaults returns `"raven vdev (commit: unknown, built: unknown)"`
- `Info.String()` with custom values returns expected format
- Info JSON marshaling produces `{"version":"dev","commit":"unknown","date":"unknown"}`
- Info JSON round-trip (marshal then unmarshal) preserves values

### Integration Tests
- Build binary with ldflags and verify the version output (tested as part of T-007)

### Edge Cases to Handle
- Empty string injected via ldflags (should not crash, will show empty)
- Very long version strings (e.g., `git describe` with dirty suffix)
- Non-ASCII characters in version/commit (should not occur but should not panic)

## Implementation Notes
### Recommended Approach
1. Create `internal/buildinfo/buildinfo.go`
2. Declare the three exported variables with defaults
3. Define the `Info` struct with JSON tags
4. Implement `GetInfo()` and `Info.String()`
5. Create `internal/buildinfo/buildinfo_test.go` with table-driven tests
6. Verify ldflags injection works: `go build -ldflags "-X $(go list -m)/internal/buildinfo.Version=test" ./cmd/raven/ && ./dist/raven`

### Potential Pitfalls
- The variable names MUST be exported (start with capital letter) for `-ldflags -X` to work. Unexported variables cannot be set via ldflags.
- The ldflags `-X` path must use the full module path (from `go.mod`), not a relative path. Example: `-X github.com/ravenco/raven/internal/buildinfo.Version=1.0.0`
- Do not use `const` -- ldflags can only set `var` declarations.
- Do not use `init()` functions -- per CLAUDE.md conventions, init() is only for cobra command registration.

### Security Considerations
- Build info is non-sensitive metadata; safe to expose in version output
- Do not add fields for build machine hostname, username, or directory path

## References
- [Go Linker Flags (-ldflags)](https://pkg.go.dev/cmd/link)
- [Go Build Flags](https://pkg.go.dev/cmd/go#hdr-Compile_packages_and_dependencies)
- [PRD Section 6.2 - buildinfo package](docs/prd/PRD-Raven.md)