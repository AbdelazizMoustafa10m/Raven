# Workflow Engine

Raven's workflow engine is a lightweight state machine that drives multi-step AI development pipelines. This document explains the engine's design, the four built-in workflows, checkpoint/resume support, and how to define custom workflows.

## Overview

A workflow is a directed graph of named steps. Each step is backed by a `StepHandler` implementation that performs work and returns a transition event (e.g. `success`, `failure`). The engine resolves the next step by looking up the event in the step's transition map.

Workflow state is persisted to a JSON checkpoint file after every transition. If the process is interrupted, the workflow can be resumed from the last completed step using `raven resume`.

## StepHandler Interface

Every step in a workflow must have a registered `StepHandler`:

```go
type StepHandler interface {
    // Execute runs the step's logic. Returns a transition event string
    // (e.g. "success", "failure") and an error.
    Execute(ctx context.Context, state *WorkflowState) (string, error)

    // DryRun returns a human-readable description of what Execute would
    // do without performing any side effects.
    DryRun(state *WorkflowState) string

    // Name returns the unique step identifier that must match the step
    // name in the WorkflowDefinition.
    Name() string
}
```

## Transition Events

| Event | Constant | Description |
|-------|----------|-------------|
| `success` | `EventSuccess` | Step completed successfully |
| `failure` | `EventFailure` | Non-recoverable error |
| `blocked` | `EventBlocked` | Dependency not yet satisfied |
| `rate_limited` | `EventRateLimited` | API rate limit encountered |
| `needs_human` | `EventNeedsHuman` | Human intervention required |
| `partial` | `EventPartial` | Partial completion; may continue |

## Terminal Pseudo-Steps

| Pseudo-step | Description |
|-------------|-------------|
| `__done__` | Workflow completed successfully |
| `__failed__` | Workflow terminated with failure |

## Workflow Lifecycle Events

The engine emits structured `WorkflowEvent` messages during execution. These are consumed by the TUI event log and structured log output.

| Event Type | When Emitted |
|------------|-------------|
| `workflow_started` | Workflow begins execution |
| `step_started` | A step begins |
| `step_completed` | A step finishes with a transition event |
| `step_failed` | A step returns an error |
| `step_skipped` | A step is bypassed (e.g. dry-run mode) |
| `workflow_completed` | Workflow reaches `__done__` |
| `workflow_failed` | Workflow reaches `__failed__` |
| `workflow_resumed` | Workflow resumed from a checkpoint |
| `checkpoint` | State persisted to disk after a transition |

## State Machine Diagram

The following shows the state transitions for the `implement-review-pr` built-in workflow:

```
[START]
  |
  v
run_implement
  | success         failure
  |--------> run_review -------> [__failed__]
             |
             | success
             v
         check_review
             | success           needs_human
             |-------> create_pr ---------> run_fix
             |                                |
             |                                | success
             |                                v
             |                           run_review (loop back)
             |
             v (from create_pr success)
          [__done__]
```

## Built-in Workflows

Raven ships with four built-in workflows. Their definitions are registered automatically when the binary starts.

### implement

Runs the AI implementation loop for a single phase or task.

```
[START] -> run_implement -> [__done__]
                         \-> [__failed__]
```

**Used by:** `raven implement`

**Steps:**

| Step | Handler | Description |
|------|---------|-------------|
| `run_implement` | `ImplementHandler` | Runs the loop runner for the configured phase or task |

### implement-review-pr

Full linear pipeline: implement, review, optionally fix, then create a PR.

**Used by:** `raven pipeline`

**Steps:**

| Step | Handler | Description |
|------|---------|-------------|
| `run_implement` | `ImplementHandler` | Implementation loop |
| `run_review` | `ReviewHandler` | Multi-agent parallel review |
| `check_review` | `CheckReviewHandler` | Evaluates review verdict; routes to fix or PR |
| `run_fix` | `FixHandler` | Apply review findings and re-verify |
| `create_pr` | `PRHandler` | Generate PR body and open PR via `gh` |

### pipeline

Multi-phase project pipeline that advances through phases until all work is complete.

```
[START] -> init_phase -> run_phase_workflow -> advance_phase
                                                    |
                                                    | partial (more phases remain)
                                                    v
                                               init_phase (loop)
                                                    |
                                                    | success (all phases done)
                                                    v
                                               [__done__]
```

**Used by:** `raven pipeline` (multi-phase mode)

### prd-decompose

Decomposes a PRD document into structured task files.

```
[START] -> shred -> scatter -> gather -> [__done__]
```

**Used by:** `raven prd`

| Step | Handler | Description |
|------|---------|-------------|
| `shred` | `ShredHandler` | Single-agent PRD-to-epics call |
| `scatter` | `ScatterHandler` | Parallel per-epic task generation |
| `gather` | `GatherHandler` | Merge, deduplicate, validate, and emit task files |

## Checkpointing and Resume

After every successful step transition, the engine serializes the `WorkflowState` to a JSON file in `.raven/state/<run-id>.json`. The state includes:

- Current step name
- All completed step records with durations and transition events
- Workflow metadata (phase, task, start time)

To list available checkpoints:

```bash
raven resume --list
```

To resume a specific run:

```bash
raven resume --run <run-id>
```

To remove old checkpoints:

```bash
raven resume --clean <run-id>
raven resume --clean-all
```

## Custom Workflow Guide

Define custom workflows in `raven.toml` under `[workflows.<name>]`. Custom workflows use the same built-in step handlers.

### Example: implement-only

```toml
[workflows.implement-only]
description = "Implementation without review or PR"
steps       = ["implement"]

[workflows.implement-only.transitions.implement]
success = "__done__"
failure = "__failed__"
```

### Example: review-then-pr

```toml
[workflows.review-then-pr]
description = "Review existing code and create a PR without re-implementing"
steps       = ["run_review", "check_review", "create_pr", "run_fix"]

[workflows.review-then-pr.transitions.run_review]
success = "check_review"
failure = "__failed__"

[workflows.review-then-pr.transitions.check_review]
success     = "create_pr"
needs_human = "run_fix"
failure     = "__failed__"

[workflows.review-then-pr.transitions.run_fix]
success = "run_review"
failure = "__failed__"

[workflows.review-then-pr.transitions.create_pr]
success = "__done__"
failure = "__failed__"
```

### Validation

Run `raven config validate` to check that all workflow definitions are structurally valid. The validator checks:

- All steps referenced in transitions are defined
- The `initial_step` exists in the steps list
- No unreachable steps (warning, not error)
- Cycle detection (cycles are warnings, not errors, since review-fix loops are intentional)

## Dry-Run Mode

Every step handler implements `DryRun(state)` which returns a text description of what `Execute` would do. Pass `--dry-run` to any command to print the dry-run plan without executing.

```bash
raven pipeline --phase 2 --impl-agent claude --dry-run
```

The output shows the planned step sequence, the agent command that would be invoked, and the expected transitions.

## Engine Options

The workflow engine is constructed with functional options:

| Option | Description |
|--------|-------------|
| `WithCheckpointing(store)` | Enables JSON checkpoint persistence after each step |
| `WithEvents(ch)` | Subscribes a `chan WorkflowEvent` to receive lifecycle events |
| `WithMaxSteps(n)` | Limits total step executions (guards against infinite loops) |
