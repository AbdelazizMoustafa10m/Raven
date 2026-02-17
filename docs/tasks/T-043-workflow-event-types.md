# T-043: Workflow Event Types and Constants

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Small: 2-4hrs |
| Dependencies | T-004 |
| Blocked By | T-004 |
| Blocks | T-044, T-045, T-046, T-048, T-049 |

## Goal
Define the core type system for the workflow engine: transition events, workflow event messages for TUI consumption, step handler interface, and workflow definition types. These types form the contract between the workflow engine, step handlers, CLI commands, and the TUI layer.

## Background
Per PRD Section 5.1, the workflow engine uses a state-machine-based runner with typed transition events (`success`, `failure`, `blocked`, `rate_limited`, `needs_human`, `partial`). Workflow execution emits structured `WorkflowEvent` messages to a channel consumed by the TUI. The `StepHandler` interface is the extension point for all workflow steps. Workflow definitions describe the state machine graph (steps, transitions, metadata). These types are defined in `internal/workflow/events.go` and related files, and are consumed by every other workflow-related task.

T-004 provides the central data types (`WorkflowState`, `StepRecord`) that this task builds upon.

## Technical Specifications
### Implementation Approach
Define all workflow-related types in `internal/workflow/events.go`. Use Go string constants (not iota) for transition events so they serialize cleanly to JSON in checkpoint files. Define the `StepHandler` interface with `Execute`, `DryRun`, and `Name` methods per the PRD. Define `WorkflowEvent` as the structured message type sent over channels for TUI consumption. Define `WorkflowDefinition` and `StepDefinition` types to describe workflow graphs declaratively.

### Key Components
- **Event constants**: `EventSuccess`, `EventFailure`, `EventBlocked`, `EventRateLimited`, `EventNeedsHuman`, `EventPartial`
- **StepHandler interface**: The contract every workflow step must implement
- **WorkflowEvent**: Structured event for TUI consumption with type, step, message, timestamp
- **WorkflowDefinition**: Declarative workflow graph (steps, transitions, description, parallel flags)
- **StepDefinition**: Individual step within a workflow definition (name, transitions map, parallel flag)
- **TransitionMap**: Maps event names to next step names

### API/Interface Contracts
```go
// internal/workflow/events.go

// Transition events -- these are the outcomes a StepHandler can return.
const (
    EventSuccess    = "success"
    EventFailure    = "failure"
    EventBlocked    = "blocked"
    EventRateLimited = "rate_limited"
    EventNeedsHuman = "needs_human"
    EventPartial    = "partial"
)

// Terminal pseudo-steps used as transition targets.
const (
    StepDone   = "__done__"
    StepFailed = "__failed__"
)

// StepHandler is the interface every workflow step must implement.
type StepHandler interface {
    Execute(ctx context.Context, state *WorkflowState) (string, error) // returns event name
    DryRun(state *WorkflowState) string                                // returns description
    Name() string
}

// WorkflowEvent is emitted by the engine for TUI/logging consumption.
type WorkflowEvent struct {
    Type        string    `json:"type"`         // step_started, step_completed, step_failed, workflow_started, workflow_completed, etc.
    WorkflowID  string    `json:"workflow_id"`
    Step        string    `json:"step"`
    Event       string    `json:"event"`        // transition event (success, failure, etc.)
    Message     string    `json:"message"`
    Timestamp   time.Time `json:"timestamp"`
    Error       string    `json:"error,omitempty"`
}

// WorkflowDefinition describes a workflow's state machine graph.
type WorkflowDefinition struct {
    Name        string                    `json:"name" toml:"name"`
    Description string                    `json:"description" toml:"description"`
    Steps       []StepDefinition          `json:"steps" toml:"steps"`
    InitialStep string                    `json:"initial_step" toml:"initial_step"`
}

// StepDefinition describes a single step and its transitions.
type StepDefinition struct {
    Name        string            `json:"name" toml:"name"`
    Transitions map[string]string `json:"transitions" toml:"transitions"` // event -> next step
    Parallel    bool              `json:"parallel,omitempty" toml:"parallel,omitempty"`
}

// WorkflowEventType constants for the Type field of WorkflowEvent.
const (
    WEStepStarted       = "step_started"
    WEStepCompleted     = "step_completed"
    WEStepFailed        = "step_failed"
    WEWorkflowStarted   = "workflow_started"
    WEWorkflowCompleted = "workflow_completed"
    WEWorkflowFailed    = "workflow_failed"
    WEWorkflowResumed   = "workflow_resumed"
    WEStepSkipped       = "step_skipped"
    WECheckpoint        = "checkpoint"
)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| time | stdlib | Timestamps on events |
| context | stdlib | Context in StepHandler.Execute |
| internal/workflow (T-004) | - | WorkflowState, StepRecord types |

## Acceptance Criteria
- [ ] All six transition event constants are defined and documented
- [ ] StepHandler interface matches the PRD specification with Execute returning (string, error)
- [ ] WorkflowEvent struct is JSON-serializable and includes all fields needed by TUI
- [ ] WorkflowDefinition and StepDefinition types support TOML deserialization via struct tags
- [ ] Terminal pseudo-steps (StepDone, StepFailed) are defined for transition targets
- [ ] WorkflowEventType constants cover all lifecycle milestones
- [ ] All types have godoc comments explaining their purpose
- [ ] Unit tests verify JSON round-trip serialization for all types
- [ ] Unit tests achieve 95% coverage

## Testing Requirements
### Unit Tests
- All event constants have expected string values
- WorkflowEvent marshals to JSON and back without data loss
- WorkflowDefinition marshals to/from JSON correctly
- StepDefinition transitions map serializes correctly
- Terminal pseudo-step constants are distinct from user step names
- WorkflowEventType constants are non-empty and unique

### Integration Tests
- None (pure type definitions; integration tested via T-045 and T-046)

### Edge Cases to Handle
- Empty transitions map in StepDefinition (terminal step)
- WorkflowEvent with empty optional fields (Error)
- Ensure event constants do not collide with step names (prefix convention)

## Implementation Notes
### Recommended Approach
1. Create `internal/workflow/events.go` with all constants and types
2. Use string constants (not iota) for JSON/TOML compatibility
3. Add comprehensive godoc comments on every exported type and constant
4. Write table-driven tests verifying serialization round-trips
5. Keep this file focused on types only -- no logic

### Potential Pitfalls
- Do not use `Event` as the return type for StepHandler.Execute -- use `string` to keep the interface simple and avoid circular dependencies
- Ensure TOML struct tags use snake_case to match raven.toml conventions
- The `Transitions` field is a `map[string]string` where keys are event names and values are step names -- document this clearly

### Security Considerations
- None specific to type definitions

## References
- [PRD Section 5.1 - Generic Workflow Engine](docs/prd/PRD-Raven.md)
- [PRD Section 6.4 - Central Data Types](docs/prd/PRD-Raven.md)
- [Go encoding/json documentation](https://pkg.go.dev/encoding/json)
- [BurntSushi/toml struct tags](https://pkg.go.dev/github.com/BurntSushi/toml)