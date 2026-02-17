# T-005: Structured Logging with charmbracelet/log

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Small: 2-4hrs |
| Dependencies | T-001 |
| Blocked By | T-001 |
| Blocks | T-006, T-083 |

## Goal
Establish the project-wide logging infrastructure using `charmbracelet/log`, providing a centralized logger factory with component prefixes, level configuration, and stderr-only output. This ensures consistent, pretty terminal logging across all Raven subsystems while maintaining the PRD requirement that stdout is reserved for structured output.

## Background
Per PRD Section 6.5, Raven uses `charmbracelet/log` for pretty terminal output with component prefixes. Log levels are: `debug` (enabled by `--verbose`), `info` (default), `warn`, `error`, and `fatal` (`--quiet` shows only `error` and `fatal`). The PRD specifies `RAVEN_LOG_FORMAT=json` for JSON-structured logs in CI, and `RAVEN_DEBUG=1` for verbose diagnostics. All logs go to stderr; only structured output goes to stdout.

Per CLAUDE.md, `charmbracelet/log` is chosen over `slog` for its pretty terminal output and consistency with the TUI ecosystem. The library implements the `slog.Handler` interface, providing forward compatibility with Go's standard structured logging.

## Technical Specifications
### Implementation Approach
Create a `internal/logging` package (or place utilities in a top-level logging setup function called from main/CLI init) that provides a `NewLogger(component string)` factory function. Each logger is configured with a component prefix, writes to stderr, and respects the global log level. The global level is set once from CLI flags (`--verbose`, `--quiet`) or environment variables.

Since `charmbracelet/log` provides a default global logger and supports creating child loggers with `WithPrefix()`, the implementation wraps this API to provide Raven-specific conventions.

### Key Components
- **Logger Factory**: `NewLogger(component string) *log.Logger` creates loggers with component prefixes
- **SetLevel**: Sets global log level from CLI flag/env (called during CLI initialization)
- **SetFormat**: Switches between text (default) and JSON format for CI
- **Global configuration**: Level, format, and output destination (stderr)

### API/Interface Contracts
```go
// internal/logging/logging.go

// Package logging provides Raven's logging infrastructure built on charmbracelet/log.
package logging

import (
    "io"
    "os"

    "github.com/charmbracelet/log"
)

// Level constants matching charmbracelet/log levels.
const (
    LevelDebug = log.DebugLevel
    LevelInfo  = log.InfoLevel
    LevelWarn  = log.WarnLevel
    LevelError = log.ErrorLevel
    LevelFatal = log.FatalLevel
)

// Setup configures the global logging defaults. Call once during CLI initialization.
// - verbose: sets level to Debug
// - quiet: sets level to Error
// - jsonFormat: switches to JSON formatter
// All loggers write to stderr to keep stdout clean for structured output.
func Setup(verbose, quiet, jsonFormat bool) {
    level := log.InfoLevel
    if verbose {
        level = log.DebugLevel
    }
    if quiet {
        level = log.ErrorLevel
    }

    log.SetLevel(level)
    log.SetOutput(os.Stderr)

    if jsonFormat {
        log.SetFormatter(log.JSONFormatter)
    }
}

// New creates a logger with the given component prefix.
// Example: logging.New("config") produces logs like "[config] loading raven.toml"
func New(component string) *log.Logger {
    return log.WithPrefix(component)
}

// SetOutput overrides the output writer (useful for testing).
func SetOutput(w io.Writer) {
    log.SetOutput(w)
}
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| github.com/charmbracelet/log | latest (v0.4.x) | Structured, colorful terminal logging |
| os | stdlib | stderr output |

## Acceptance Criteria
- [ ] `internal/logging/logging.go` exists with `Setup()` and `New()` functions
- [ ] `Setup(false, false, false)` sets level to Info and output to stderr
- [ ] `Setup(true, false, false)` sets level to Debug
- [ ] `Setup(false, true, false)` sets level to Error
- [ ] `Setup(false, false, true)` enables JSON formatter
- [ ] `New("config")` returns a logger with "[config]" prefix
- [ ] All loggers write to stderr (verified by capturing output)
- [ ] No logging functions write to stdout
- [ ] Logger respects global level: Debug messages hidden at Info level
- [ ] `RAVEN_LOG_FORMAT=json` environment variable is documented (actual env var handling done in T-006/T-010)
- [ ] Unit tests achieve 90% coverage
- [ ] `go vet ./internal/logging/...` passes

## Testing Requirements
### Unit Tests
- Setup with verbose=true sets debug level; debug messages visible
- Setup with quiet=true sets error level; info messages hidden, error messages visible
- Setup with jsonFormat=true produces JSON-formatted output
- New("component") produces logger with correct prefix
- Logger output goes to configured writer (use bytes.Buffer for capture)
- Multiple loggers with different prefixes share the same level

### Integration Tests
- None (logging is unit-testable via output capture)

### Edge Cases to Handle
- Both verbose and quiet set: quiet should win (document this behavior)
- Empty component string: should produce logger without prefix (not crash)
- Concurrent logging from multiple goroutines: charmbracelet/log is thread-safe
- Very long log messages: should not truncate (library handles wrapping)

## Implementation Notes
### Recommended Approach
1. Create `internal/logging/logging.go`
2. Implement `Setup()` to configure the global logger defaults
3. Implement `New()` using `log.WithPrefix()`
4. Create `internal/logging/logging_test.go` with output capture tests
5. Document the convention: all packages call `logging.New("pkgname")` at package level
6. Verify: `go build ./... && go vet ./... && go test ./internal/logging/...`

### Potential Pitfalls
- `charmbracelet/log` uses a global default logger. `Setup()` must be called before any logging occurs (i.e., in the Cobra PersistentPreRun hook, implemented in T-006).
- The `log.SetLevel()` and `log.SetFormatter()` calls modify global state. In tests, use `SetOutput()` with a `bytes.Buffer` and reset after each test using `t.Cleanup()`.
- Do not use `log.Fatal()` for recoverable errors -- it calls `os.Exit(1)`. Use it only for truly fatal initialization failures.
- The JSON formatter output is newline-delimited JSON (NDJSON), suitable for log aggregation tools.

### Security Considerations
- Never log sensitive data (API keys, tokens) at any level
- The debug level (`--verbose`) may expose agent command lines that include model names and prompt paths -- acceptable but document this

## References
- [charmbracelet/log GitHub Repository](https://github.com/charmbracelet/log)
- [charmbracelet/log Go Package Documentation](https://pkg.go.dev/github.com/charmbracelet/log)
- [PRD Section 6.5 - Logging & Diagnostics](docs/prd/PRD-Raven.md)
- [CLAUDE.md - charmbracelet/log over slog](CLAUDE.md)