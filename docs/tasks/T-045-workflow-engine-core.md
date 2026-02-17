# T-045: Workflow Engine Core -- State Machine Runner

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Large: 14-20hrs |
| Dependencies | T-004, T-043, T-044 |
| Blocked By | T-043, T-044 |
| Blocks | T-046, T-047, T-048, T-049, T-050 |

## Goal
Implement the core state machine workflow runner that executes multi-step workflows by resolving step handlers from the registry, executing them, processing transition events to determine the next step, and emitting structured events for TUI consumption. This is the heart of Raven's generic workflow engine -- the orchestrator that makes every workflow (implement, review, pipeline, prd-decompose) possible.

## Background
Per PRD Section 5.1, the workflow engine is "a state-machine-based workflow runner that executes configurable multi-step workflows with checkpoint/resume, conditional branching, and parallel step execution." The engine is a Go library in `internal/workflow/` consumed by both CLI and TUI. It accepts a `WorkflowDefinition` (the graph), a `Registry` (the handlers), and an optional initial `WorkflowState` (for resume). It walks the state machine step by step, calling `StepHandler.Execute()` at each step, using the returned event to determine the next step via the definition's transition map.

Key behaviors:
- Supports `--dry-run` mode (calls `StepHandler.DryRun()` instead of `Execute()`)
- Supports `--step <name>` to run a single step in isolation
- Emits `WorkflowEvent` messages to a channel for TUI/logging
- Uses `context.Context` for cancellation propagation
- Handles step errors (step returns error) differently from transition failures (no transition defined for event)

Checkpoint/resume is handled by T-046 (separate concern). This task focuses on the execution loop itself.

## Technical Specifications
### Implementation Approach
Create `internal/workflow/engine.go` with an `Engine` struct that takes a `Registry`, an event channel, and options. The `Run` method accepts a `WorkflowDefinition` and optional `WorkflowState` (nil for fresh start). The core loop: resolve current step from registry, execute handler, record step result in state, look up transition for returned event, advance to next step or terminate.

### Key Components
- **Engine**: The main workflow executor
- **EngineOption**: Functional options for engine configuration (dry-run, single-step, event channel)
- **Run loop**: The core state machine loop -- resolve handler, execute, transition, repeat
- **Event emission**: Sends WorkflowEvent to channel at each lifecycle point
- **Error handling**: Distinguishes step execution errors from missing transitions

### API/Interface Contracts
```go
// internal/workflow/engine.go

// Engine executes workflow definitions using registered step handlers.
type Engine struct {
    registry   *Registry
    events     chan<- WorkflowEvent
    dryRun     bool
    singleStep string // if non-empty, run only this step
    logger     *log.Logger
}

// EngineOption configures the Engine.
type EngineOption func(*Engine)

func WithDryRun(dryRun bool) EngineOption
func WithSingleStep(stepName string) EngineOption
func WithEventChannel(ch chan<- WorkflowEvent) EngineOption
func WithLogger(logger *log.Logger) EngineOption

// NewEngine creates a workflow engine with the given registry and options.
func NewEngine(registry *Registry, opts ...EngineOption) *Engine

// Run executes a workflow definition from start (or from a resumed state).
// If state is nil, a new WorkflowState is created.
// Returns the final WorkflowState and any error.
func (e *Engine) Run(ctx context.Context, def *WorkflowDefinition, state *WorkflowState) (*WorkflowState, error)

// RunStep executes a single named step in isolation.
// Useful for debugging and testing individual steps.
func (e *Engine) RunStep(ctx context.Context, def *WorkflowDefinition, stepName string, state *WorkflowState) (*WorkflowState, error)

// Validate checks a workflow definition for errors:
// - All steps have registered handlers
// - All transition targets reference valid steps or terminal pseudo-steps
// - Initial step exists
// - No unreachable steps (warning, not error)
func (e *Engine) Validate(def *WorkflowDefinition) []error
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/workflow (T-004) | - | WorkflowState, StepRecord |
| internal/workflow (T-043) | - | Event types, StepHandler, WorkflowDefinition |
| internal/workflow (T-044) | - | Registry for handler lookup |
| context | stdlib | Cancellation propagation |
| time | stdlib | Duration tracking |
| fmt | stdlib | Error wrapping |
| charmbracelet/log | latest | Structured logging |

## Acceptance Criteria
- [ ] Engine executes a multi-step workflow from initial step to terminal step
- [ ] Each step's handler is resolved from the registry by name
- [ ] StepHandler.Execute() is called with current WorkflowState and context
- [ ] Returned event is used to look up next step from transitions map
- [ ] Terminal pseudo-steps (StepDone, StepFailed) end the workflow
- [ ] Missing handler for a step returns a clear error
- [ ] Missing transition for an event returns a clear error with step name and event
- [ ] StepRecord is appended to WorkflowState.StepHistory after each step
- [ ] WorkflowState.CurrentStep is updated after each transition
- [ ] WorkflowState.UpdatedAt is updated after each step
- [ ] DryRun mode calls DryRun() on each handler and does not execute
- [ ] SingleStep mode runs only the named step and returns
- [ ] WorkflowEvents are emitted: workflow_started, step_started, step_completed/step_failed, workflow_completed/workflow_failed
- [ ] Context cancellation stops the engine between steps
- [ ] Validate catches: missing handlers, invalid transition targets, missing initial step
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- Linear 3-step workflow (A -> B -> C -> done): all steps execute in order
- Branching workflow (A -> success -> B, A -> failure -> C): correct path followed
- Terminal step reached: engine returns final state with no error
- Step handler returns error: engine records error and stops (or transitions to failure step)
- Missing handler for step: returns descriptive error
- Missing transition for event: returns descriptive error with step and event names
- DryRun mode: all DryRun() methods called, Execute() never called
- SingleStep mode: only named step executed, engine returns after that step
- Context cancelled between steps: engine returns context error
- Context cancelled during step: handler receives cancelled context
- Resume from existing state: engine starts at state.CurrentStep
- StepHistory is accumulated across all steps
- Validate with valid definition: returns empty errors
- Validate with missing handler: returns error identifying the step
- Validate with invalid transition target: returns error identifying the transition

### Integration Tests
- Multi-step workflow with mock handlers that manipulate state metadata
- Workflow with conditional branching based on handler return values

### Edge Cases to Handle
- Workflow with single step that transitions to done
- Workflow where step returns unexpected event (not in transitions)
- Workflow that would loop forever (max iterations guard -- configurable, default 1000)
- State metadata modified by step handler is visible to subsequent steps
- Empty WorkflowDefinition (no steps)
- Step that returns empty string event

## Implementation Notes
### Recommended Approach
1. Create `Engine` struct with fields for registry, events channel, options
2. `Run()` initializes state if nil, emits workflow_started event
3. Core loop: check context, resolve handler, emit step_started, execute, record StepRecord, emit step_completed/failed, look up transition, advance CurrentStep
4. Terminal check: if next step is StepDone or StepFailed, exit loop
5. Max iterations guard: prevent infinite loops (configurable, default 1000)
6. `RunStep()` delegates to `Run()` with SingleStep option
7. `Validate()` walks the definition graph checking all references
8. DryRun mode replaces Execute() with DryRun() in the loop

### Potential Pitfalls
- Do not send events to a nil channel -- check before sending, or use a no-op channel
- The engine must handle the case where a step handler panics -- use recover() to convert to error
- StepRecord.Duration must be measured accurately (time.Since, not time.Now diff)
- The max iterations guard should be documented so users understand the limit
- Do not close the events channel from the engine -- the caller owns channel lifecycle

### Security Considerations
- Step handlers may execute external processes -- the engine itself does not need to validate this, but it should propagate context cancellation faithfully
- Ensure workflow IDs are generated safely (UUID or timestamp-based)

## References
- [PRD Section 5.1 - Generic Workflow Engine](docs/prd/PRD-Raven.md)
- [Go state machine patterns](https://medium.com/@johnsiilver/go-state-machine-patterns-3b667f345b5e)
- [Go context.Context documentation](https://pkg.go.dev/context)