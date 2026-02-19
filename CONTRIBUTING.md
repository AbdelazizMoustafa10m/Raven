# Contributing to Raven

Thank you for your interest in contributing to Raven. This document covers the development workflow, coding standards, and PR process.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Getting Started](#getting-started)
- [Project Layout](#project-layout)
- [Development Workflow](#development-workflow)
- [Testing](#testing)
- [Code Style](#code-style)
- [Commit Messages](#commit-messages)
- [Pull Request Process](#pull-request-process)
- [Code of Conduct](#code-of-conduct)

## Prerequisites

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.24 | Build and test |
| Git | 2.30 | Version control |
| Make | 3.81 | Build targets |
| golangci-lint | 1.64 | Static analysis (optional — `run_pipeline_checks.sh` skips lint gracefully if missing) |

Install Go from [go.dev/dl](https://go.dev/dl/). All other Go tooling is resolved via `go.mod`; no extra installation is required.

## Getting Started

```bash
# Clone the repository
git clone https://github.com/AbdelazizMoustafa10m/Raven.git
cd Raven

# Verify the build
make build

# Run all tests
make test

# Run static analysis
make vet
```

The `make build` target produces `bin/raven` with version information injected via ldflags. The binary is pure Go (`CGO_ENABLED=0`) and can be cross-compiled for any supported platform.

## Project Layout

```
cmd/raven/main.go      Entry point
internal/              All non-exported packages
  cli/                 One Cobra command file per subcommand
  config/              TOML config types, loading, resolution, validation
  workflow/            State machine engine, step registry, builtins
  agent/               Agent interface + Claude, Codex, Gemini adapters
  task/                Task spec parser, state manager, phases, selector
  loop/                Implementation loop runner, prompts, recovery
  review/              Multi-agent review, diff, consolidation, fix engine
  prd/                 PRD decomposition: shred, scatter, merge, emit
  pipeline/            Phase pipeline orchestrator, branch manager
  git/                 Git + gh CLI wrapper
  tui/                 Bubble Tea TUI application
  buildinfo/           Version stamp (set via ldflags at build time)
templates/             Embedded project template files
testdata/              Test fixtures shared across packages
tests/e2e/             End-to-end tests using mock agent scripts
docs/                  Documentation
scripts/               Build and packaging scripts
```

## Development Workflow

### Running All CI Checks Locally

Before pushing or opening a PR, run the full pipeline check script. It mirrors every job in `.github/workflows/ci.yml` so you can catch failures locally:

```bash
./run_pipeline_checks.sh              # Fail-fast mode (stops on first failure)
./run_pipeline_checks.sh --all        # Run all checks even if some fail
./run_pipeline_checks.sh --with-build # Include cross-platform build matrix
```

The script runs: module hygiene (`go mod tidy`), formatting (`gofmt`), static analysis (`go vet`), linting (`golangci-lint`), tests (`go test -race`), and build verification. It requires `go` and optionally `golangci-lint` (skips lint gracefully if missing). Install [gum](https://github.com/charmbracelet/gum) for a polished spinner UI.

You can also run the same checks via Make:

```bash
make check          # Equivalent to ./run_pipeline_checks.sh
```

### Individual Build Targets

```bash
make build          # Compile to bin/raven
make test           # Run all unit tests
make test-race      # Run tests with -race detector
make test-e2e       # Run end-to-end tests (builds binary, uses mock agents)
make vet            # Run go vet
make lint           # Run golangci-lint (requires golangci-lint installed)
make bench          # Run benchmarks with -benchtime=3s
make completions    # Generate shell completion files to completions/
make manpages       # Generate man pages to man/man1/
make clean          # Remove build artifacts
```

### Adding a New CLI Command

1. Create `internal/cli/<name>.go` with a `new<Name>Cmd() *cobra.Command` constructor.
2. Add an `init()` function that calls `rootCmd.AddCommand(new<Name>Cmd())`.
3. Write a corresponding `internal/cli/<name>_test.go`.
4. If the command requires new core logic, implement it in the appropriate `internal/<pkg>/` package.

### Adding a New Agent Adapter

1. Implement the `agent.Agent` interface in `internal/agent/<name>.go`.
2. Add a `var _ Agent = (*<Name>Agent)(nil)` compile-time interface check.
3. Register the new adapter in `internal/cli/implement.go`'s `buildAgentRegistry` function.
4. Add shell completion hints for the `--agent` flag in `implement.go` and `pipeline.go`.

## Testing

### Unit Tests

All unit tests live alongside the code they test as `*_test.go` files in the same package.

```bash
# Run all unit tests
go test ./...

# Run tests for a specific package
go test ./internal/agent/...

# Run a specific test
go test -run TestClaudeAgent_ParseRateLimit ./internal/agent/

# Run with race detector
go test -race ./...
```

Unit tests must use the table-driven pattern with `testify/assert` and `testify/require`:

```go
func TestMyFunc(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {name: "basic case", input: "hello", want: "HELLO"},
        {name: "empty input", input: "", wantErr: true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := MyFunc(tt.input)
            if tt.wantErr {
                require.Error(t, err)
                return
            }
            require.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### End-to-End Tests

E2E tests live in `tests/e2e/` and use mock agent scripts from `testdata/mock-agents/`. They build the `raven` binary and invoke it as a subprocess.

```bash
# Run e2e tests (slow; builds binary first)
make test-e2e

# Run with -short to skip slow tests
go test -short ./tests/e2e/
```

E2E tests are guarded with `if testing.Short() { t.Skip() }` so they are excluded from rapid unit-test cycles.

### Benchmarks

```bash
make bench

# Or directly:
go test -bench=. -benchtime=3s ./...
```

## Code Style

### General Rules

- `go vet ./...` must pass with zero warnings.
- `gofmt` formatting is enforced (run `gofmt -w .` before committing).
- All exported symbols must have doc comments.
- No global mutable state; pass dependencies via constructors.
- No `init()` functions except for Cobra command registration.
- Prefer `io.Reader`/`io.Writer` interfaces over `*os.File` for testability.
- All long-running functions accept `context.Context` as their first parameter.

### Error Handling

```go
// Wrap errors with context using %w
if err := doSomething(); err != nil {
    return fmt.Errorf("doing something for task %s: %w", taskID, err)
}

// Define sentinel errors for control flow
var ErrTaskBlocked = errors.New("task blocked by dependencies")
```

### Logging

Use `charmbracelet/log` with structured key-value pairs. Never use `fmt.Printf` for diagnostic output.

```go
log := logging.New("my-component")
log.Info("processing task", "id", taskID, "phase", phaseID)
log.Debug("agent output received", "bytes", len(output))
```

### Concurrency

- Use `errgroup.Group` with `SetLimit()` for bounded parallel execution.
- Protect shared state with `sync.Mutex` or `sync.RWMutex`.
- Never close a channel from the receiver side.
- All operations that can block accept a `context.Context` for cancellation.

### Naming

| Entity | Convention | Example |
|--------|------------|---------|
| Package | lowercase, single word | `workflow`, `agent` |
| Interface | `-er` suffix or descriptive noun | `Agent`, `StepHandler` |
| Constructor | `New<Type>` | `NewRegistry()` |
| Error variable | `Err<Condition>` | `ErrNotFound` |
| Constant | `CamelCase` | `EventSuccess`, `StepDone` |
| Test helper | lowercase, `t.Helper()` | `makeTestTask(t, ...)` |

## Commit Messages

Raven uses [Conventional Commits](https://www.conventionalcommits.org/).

```
<type>(<scope>): <short description>

[optional body]

[optional footer]
```

**Types:**

| Type | When to use |
|------|-------------|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation changes |
| `test` | Adding or fixing tests |
| `refactor` | Code change with no behaviour change |
| `perf` | Performance improvement |
| `ci` | CI/CD configuration |
| `chore` | Maintenance (deps, tooling) |

**Examples:**

```
feat(agent): add Gemini adapter with rate-limit detection
fix(loop): handle dirty working tree before agent invocation
docs(readme): add shell completion installation instructions
test(review): add table-driven tests for finding consolidation
```

## Pull Request Process

1. **Fork** the repository and create a feature branch from `main`:
   ```bash
   git checkout -b feat/my-feature
   ```

2. **Implement** your change following the standards above.

3. **Run the pipeline checks** to verify everything passes:
   ```bash
   ./run_pipeline_checks.sh
   ```
   This runs module hygiene, formatting, vet, lint, tests, and build — the same checks CI will run on your PR.

4. **Commit** using conventional commit messages.

5. **Open a PR** against `main` with a clear description of:
   - What the change does
   - Why it is needed
   - How it was tested
   - Any breaking changes

6. **Address review comments** by pushing additional commits (do not force-push during review).

7. PRs are merged with **squash merge** to keep the main branch history clean.

### PR Checklist

- [ ] `./run_pipeline_checks.sh` passes (or equivalently `make check`)
- [ ] All exported symbols have doc comments
- [ ] New features have tests
- [ ] PROGRESS.md updated if a task was completed

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](https://www.contributor-covenant.org/version/2/1/code_of_conduct/). By participating you agree to abide by its terms. Report unacceptable behaviour to the project maintainers.
