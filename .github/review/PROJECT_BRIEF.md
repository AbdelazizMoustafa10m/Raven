# Raven - Project Brief

## What is Raven?

Raven is a Go-based AI workflow orchestration command center. It manages the full lifecycle of AI-assisted software development: PRD decomposition, implementation loops, multi-agent code review, fix automation, and PR creation.

## Architecture

- **Language:** Go 1.24+, single binary, zero runtime dependencies
- **CLI:** spf13/cobra for command dispatch
- **TUI:** charmbracelet/bubbletea for interactive dashboard
- **Config:** TOML-based project configuration (`raven.toml`)
- **Concurrency:** goroutines + errgroup for parallel agent execution
- **External tools:** Shells out to claude, codex, gemini, git, gh CLIs

## Key Packages

| Package | Purpose |
|---------|---------|
| `internal/workflow` | Generic state machine engine with checkpoint/resume |
| `internal/agent` | Agent adapters (Claude, Codex, Gemini) with rate-limit parsing |
| `internal/task` | Task state management, dependency resolution, phase config |
| `internal/loop` | Implementation loop (task select -> prompt -> agent -> verify) |
| `internal/review` | Multi-agent parallel review with finding consolidation |
| `internal/prd` | PRD decomposition (shred -> scatter -> gather) |
| `internal/pipeline` | Phase pipeline orchestrator with branch management |
| `internal/tui` | Bubble Tea command center dashboard |
| `internal/config` | TOML config loading with resolution chain |
| `internal/git` | Git operations wrapper |

## Quality Standards

- All errors wrapped with context
- No global mutable state
- Interface-driven design for testability
- Table-driven tests with testify
- `go vet` and `go test -race` clean
