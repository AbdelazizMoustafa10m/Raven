# Raven - AI Workflow Orchestration Command Center

## Project Overview

Raven is a Go-based AI workflow orchestration command center that manages the full lifecycle of AI-assisted software development -- from PRD decomposition to implementation, code review, fix application, and pull request creation. It orchestrates multiple AI agents (Claude, Codex, Gemini) through configurable workflow pipelines, providing both a headless CLI for automation and an interactive TUI dashboard for real-time monitoring and control.

Single binary, zero runtime dependencies, cross-platform (macOS/Linux/Windows).

**PRD:** `docs/prd/PRD-Raven.md`
**Tasks:** `docs/tasks/T-XXX-*.md`
**Progress:** `docs/tasks/PROGRESS.md`
**Index:** `docs/tasks/INDEX.md`

## Tech Stack

| Layer | Technology |
|-------|------------|
| Language | Go 1.24+ |
| CLI Framework | spf13/cobra v1.10+ |
| TUI Framework | charmbracelet/bubbletea v1.2+ |
| TUI Styling | charmbracelet/lipgloss v1.0+ |
| TUI Components | charmbracelet/bubbles (viewport, spinner, progress, table) |
| TUI Forms | charmbracelet/huh v0.6+ |
| Logging | charmbracelet/log |
| Config | BurntSushi/toml v1.5.0 |
| Concurrency | golang.org/x/sync/errgroup v0.19+ |
| Glob Matching | bmatcuk/doublestar v4.10+ |
| Testing | stretchr/testify v1.9+ |
| Hashing | cespare/xxhash v2 |

## Project Structure

```
cmd/raven/main.go              # Entry point
internal/
  cli/                          # Cobra commands (root, implement, review, fix, pr, pipeline, prd, status, resume, init, config, dashboard, version)
  config/                       # TOML config loading, defaults, resolution, validation, templates
  workflow/                     # State machine engine, state persistence, registry, events, builtins
  agent/                        # Agent interface, Claude adapter, Codex adapter, Gemini stub, rate-limit coordination, mock
  task/                         # Task state management, next-task selector, markdown parser, phases, progress
  loop/                         # Implementation loop runner, prompt templates, rate-limit recovery, events
  review/                       # Multi-agent parallel review, diff generation, JSON extraction, consolidation, report, fix engine
  prd/                          # PRD decomposition: shredder, parallel workers, merger, emitter
  pipeline/                     # Phase pipeline orchestrator, branch management, wizard, metadata
  git/                          # Git operations wrapper, stash/recovery
  tui/                          # Bubble Tea app, layout, sidebar, agent panel, event log, status bar, wizard, styles, keybindings
  buildinfo/                    # Version, commit, build date (ldflags)
templates/                      # Embedded project templates (go-cli)
testdata/                       # Test fixtures
docs/
  prd/                          # Product Requirements Document
  tasks/                        # Task specs, progress, index
```

## Go Conventions

### Code Style
- `go vet ./...` must pass with zero warnings
- `go test ./...` must pass
- All exported functions/types have doc comments
- Use `internal/` to enforce visibility boundaries
- Errors: `fmt.Errorf("context: %w", err)` for wrapping
- No global mutable state; pass dependencies via constructors
- No `init()` functions except for cobra command registration
- Prefer `io.Reader`/`io.Writer` interfaces for testability
- Use `context.Context` for cancellation propagation

### Naming
- Package names: lowercase, single word (`workflow`, `config`, `pipeline`)
- Interfaces: `-er` suffix or descriptive (`Agent`, `StepHandler`, `TaskSelector`)
- Test files: `*_test.go` in same package
- Test helpers: `testutil` package or `_test.go` helpers
- Constants: `CamelCase` not `SCREAMING_SNAKE`

### Testing
- Table-driven tests with `testify/assert` and `testify/require`
- Golden test pattern for output verification
- Use `t.Helper()` in test helpers
- Use `testdata/` for test fixtures
- Use `t.TempDir()` for temporary directories
- Subtests with `t.Run("descriptive name", ...)`

### Error Handling
- Return errors, never panic (except truly unrecoverable)
- Exit codes: 0 (success), 1 (error), 2 (partial success), 3 (user-cancelled)
- Use `charmbracelet/log` for structured logging, never `fmt.Printf` for diagnostics
- Wrap errors with context: `fmt.Errorf("loading config: %w", err)`
- All progress/status goes to stderr; structured output goes to stdout

### Concurrency
- Use `errgroup.Group` with `SetLimit()` for bounded parallel execution
- Each agent runs in a goroutine sending events via channels
- TUI's Elm architecture serializes state updates through `Update()` (no races)
- All long-running operations accept `context.Context` for cancellation

### Verification Commands

```bash
./run_pipeline_checks.sh # Full CI pipeline: mod tidy, fmt, vet, lint, test, build (or `make check`)
```

## Key Technical Decisions

1. **BurntSushi/toml v1.5.0** -- MetaData.Undecoded() for unknown-key detection, simpler API for read-only use case
2. **charmbracelet/log over slog** -- Pretty terminal output with component prefixes, consistent with TUI ecosystem
3. **Bubble Tea v1.2+** -- Stable release, Elm architecture maps perfectly to workflow state machine
4. **CGO_ENABLED=0** -- Pure Go for cross-compilation, no C dependencies
5. **os/exec for all external tools** -- Shell out to claude, codex, gemini, git, gh CLIs (same pattern as lazygit, gh, k9s)
6. **Lightweight state machine** -- No external framework (Temporal/Prefect are overkill for single-machine CLI)
7. **JSON checkpoints** -- Workflow state persisted to `.raven/state/` after every transition

## Core Abstractions

### Workflow Engine
A workflow is a state machine: steps + transitions + checkpoint/resume.
```go
type StepHandler interface {
    Execute(ctx context.Context, state *WorkflowState) (Event, error)
    DryRun(state *WorkflowState) string
    Name() string
}
```

### Agent Adapter
All AI agents implement a common interface:
```go
type Agent interface {
    Name() string
    Run(ctx context.Context, opts RunOpts) (*RunResult, error)
    CheckPrerequisites() error
    ParseRateLimit(output string) (*RateLimitInfo, bool)
    DryRunCommand(opts RunOpts) string
}
```

## Task Workflow

When implementing a task:
1. Read the task spec: `docs/tasks/T-XXX-*.md`
2. Check dependencies are complete in `docs/tasks/PROGRESS.md`
3. Implement following acceptance criteria
4. Write tests (table-driven, golden where appropriate)
5. Verify: `./run_pipeline_checks.sh`
6. Update `docs/tasks/PROGRESS.md` with completion entry
