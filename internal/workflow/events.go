package workflow

import (
	"context"
	"time"
)

// Transition event constants define the set of outcomes a StepHandler can
// return from Execute. These are used as keys in StepDefinition.Transitions
// to determine the next step in the workflow graph. String values are used
// (not iota) so they round-trip cleanly through JSON checkpoint files.
const (
	// EventSuccess indicates the step completed successfully.
	EventSuccess = "success"

	// EventFailure indicates the step encountered a non-recoverable error.
	EventFailure = "failure"

	// EventBlocked indicates the step cannot proceed because a dependency
	// is not yet satisfied.
	EventBlocked = "blocked"

	// EventRateLimited indicates the step was interrupted by an API rate
	// limit and should be retried after a wait period.
	EventRateLimited = "rate_limited"

	// EventNeedsHuman indicates the step requires human intervention before
	// execution can continue.
	EventNeedsHuman = "needs_human"

	// EventPartial indicates the step completed partially; some work was
	// done but the step may need to be retried or continued.
	EventPartial = "partial"
)

// Terminal pseudo-step constants are used as transition targets to signal
// the end of workflow execution. They are prefixed with "__" to ensure they
// cannot collide with user-defined step names.
const (
	// StepDone is the terminal pseudo-step for successful workflow completion.
	StepDone = "__done__"

	// StepFailed is the terminal pseudo-step for failed workflow completion.
	StepFailed = "__failed__"
)

// WorkflowEventType constants identify the lifecycle phase of a WorkflowEvent.
// They populate the Type field of WorkflowEvent and are consumed by the TUI
// event log and structured log output.
const (
	// WEStepStarted is emitted when a step begins execution.
	WEStepStarted = "step_started"

	// WEStepCompleted is emitted when a step finishes with a transition event.
	WEStepCompleted = "step_completed"

	// WEStepFailed is emitted when a step returns an error.
	WEStepFailed = "step_failed"

	// WEWorkflowStarted is emitted when a workflow begins execution.
	WEWorkflowStarted = "workflow_started"

	// WEWorkflowCompleted is emitted when a workflow reaches StepDone.
	WEWorkflowCompleted = "workflow_completed"

	// WEWorkflowFailed is emitted when a workflow reaches StepFailed.
	WEWorkflowFailed = "workflow_failed"

	// WEWorkflowResumed is emitted when a workflow is resumed from a checkpoint.
	WEWorkflowResumed = "workflow_resumed"

	// WEStepSkipped is emitted when a step is bypassed (e.g. dry-run mode).
	WEStepSkipped = "step_skipped"

	// WECheckpoint is emitted after the workflow state is persisted to disk.
	WECheckpoint = "checkpoint"
)

// StepHandler is the interface every workflow step must implement. It forms
// the primary extension point for the workflow engine: each named step in a
// WorkflowDefinition is backed by a registered StepHandler.
type StepHandler interface {
	// Execute runs the step's logic. It receives the current workflow state
	// and must return one of the transition event constants (e.g. EventSuccess,
	// EventFailure) so the engine can resolve the next step via the transition
	// map. Execute must respect context cancellation.
	Execute(ctx context.Context, state *WorkflowState) (string, error)

	// DryRun returns a human-readable description of what Execute would do
	// without actually performing any side effects. Used for --dry-run mode.
	DryRun(state *WorkflowState) string

	// Name returns the unique identifier of this step handler, which must
	// match the step name in the WorkflowDefinition.
	Name() string
}

// WorkflowEvent is a structured message emitted by the workflow engine during
// execution. Events are sent over a channel for real-time consumption by the
// TUI event log and structured logging output.
type WorkflowEvent struct {
	// Type is one of the WE* constants describing the lifecycle milestone.
	Type string `json:"type"`

	// WorkflowID is the unique identifier of the running workflow instance.
	WorkflowID string `json:"workflow_id"`

	// Step is the name of the step that produced this event.
	Step string `json:"step"`

	// Event is the transition event returned by the step handler (e.g.
	// EventSuccess, EventFailure). Empty for lifecycle events that are not
	// the result of a StepHandler.Execute call.
	Event string `json:"event"`

	// Message is a human-readable description of the event.
	Message string `json:"message"`

	// Timestamp records when the event was emitted.
	Timestamp time.Time `json:"timestamp"`

	// Error holds the error message when Type is WEStepFailed or
	// WEWorkflowFailed. Omitted from JSON when empty.
	Error string `json:"error,omitempty"`
}

// WorkflowDefinition describes a workflow's state machine graph declaratively.
// It specifies the ordered list of steps, their transitions, and the initial
// entry point. Definitions can be loaded from TOML configuration files or
// constructed programmatically.
type WorkflowDefinition struct {
	// Name is the unique identifier of this workflow.
	Name string `json:"name" toml:"name"`

	// Description is a human-readable summary of the workflow's purpose.
	Description string `json:"description" toml:"description"`

	// Steps is the ordered list of step definitions that make up the workflow.
	Steps []StepDefinition `json:"steps" toml:"steps"`

	// InitialStep is the name of the first step to execute when the workflow
	// starts. It must match the Name field of one of the Steps entries.
	InitialStep string `json:"initial_step" toml:"initial_step"`
}

// StepDefinition describes a single step within a workflow and its outgoing
// transitions. The Transitions map resolves the next step name for each
// possible transition event returned by the corresponding StepHandler.
type StepDefinition struct {
	// Name is the unique identifier of this step within the workflow. It must
	// match the Name() return value of the registered StepHandler.
	Name string `json:"name" toml:"name"`

	// Transitions maps transition event names (keys) to the next step names
	// (values). For example: {"success": "build", "failure": "__failed__"}.
	// A step with an empty Transitions map is treated as a terminal step.
	Transitions map[string]string `json:"transitions" toml:"transitions"`

	// Parallel indicates whether this step can be executed concurrently with
	// other parallel-flagged steps in the same workflow phase.
	Parallel bool `json:"parallel,omitempty" toml:"parallel,omitempty"`
}
