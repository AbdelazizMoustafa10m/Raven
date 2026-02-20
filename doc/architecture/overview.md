# Architecture

Raven is organized as a set of focused internal packages under `internal/`. This document describes the package layout, core abstractions, and key design decisions.

## Package Layout

```
cmd/raven/main.go          Entry point (calls cli.Execute, exits with code)
internal/
  cli/                     Cobra command definitions (one file per command)
  config/                  TOML loading, defaults, four-layer resolution, validation
  workflow/                State machine engine, step registry, checkpointing
  agent/                   Agent interface, Claude/Codex/Gemini adapters, rate-limit coordinator
  task/                    Task spec parser, state manager, phase config, dependency resolver
  loop/                    Implementation loop runner, prompt generator, recovery handlers
  review/                  Diff generator, multi-agent orchestrator, consolidator, fix engine
  prd/                     PRD shredder, parallel scatter workers, merger, task file emitter
  pipeline/                Phase pipeline orchestrator, branch manager, interactive wizard
  git/                     Git operations wrapper (git and gh CLI)
  tui/                     Bubble Tea app, layout, panels, event bridge
  buildinfo/               Version, commit, date (injected via ldflags)
```

## Core Abstractions

### Workflow Engine

A workflow is a state machine: steps + transitions + checkpoint/resume. The engine persists state to JSON after every transition, enabling interrupted runs to be resumed.

```go
type StepHandler interface {
    Execute(ctx context.Context, state *WorkflowState) (Event, error)
    DryRun(state *WorkflowState) string
    Name() string
}
```

See [Workflow Engine](workflows.md) for the full reference, built-in workflows, and custom workflow guide.

### Agent Adapter

All AI agents implement a common interface for subprocess orchestration:

```go
type Agent interface {
    Name() string
    Run(ctx context.Context, opts RunOpts) (*RunResult, error)
    CheckPrerequisites() error
    ParseRateLimit(output string) (*RateLimitInfo, bool)
    DryRunCommand(opts RunOpts) string
}
```

See [Agent Adapters](agents.md) for adapter details, rate-limit coordination, and how to add a custom agent.

## Key Design Decisions

1. **BurntSushi/toml v1.5.0** -- `MetaData.Undecoded()` for unknown-key detection, simpler API for read-only use case.
2. **charmbracelet/log over slog** -- Pretty terminal output with component prefixes, consistent with TUI ecosystem.
3. **Bubble Tea v1.2+** -- Stable release, Elm architecture maps perfectly to workflow state machine.
4. **CGO_ENABLED=0** -- Pure Go for cross-compilation, no C dependencies.
5. **os/exec for all external tools** -- Shell out to `claude`, `codex`, `gemini`, `git`, `gh` CLIs (same pattern as lazygit, gh, k9s).
6. **Lightweight state machine** -- No external framework (Temporal/Prefect are overkill for single-machine CLI).
7. **JSON checkpoints** -- Workflow state persisted to `.raven/state/` after every transition.

## Concurrency Model

- `errgroup.Group` with `SetLimit()` for bounded parallel execution.
- Each agent runs in a goroutine sending events via channels.
- TUI's Elm architecture serializes state updates through `Update()` (no races).
- All long-running operations accept `context.Context` for cancellation.

## Related Documentation

- [Workflow Engine](workflows.md) -- State machine design, built-in workflows, custom workflows
- [Agent Adapters](agents.md) -- Agent interface, Claude/Codex/Gemini adapters, rate-limit coordination
- [Configuration Reference](../reference/configuration.md) -- Full `raven.toml` field reference
