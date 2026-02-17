# T-004: Central Data Types -- WorkflowState, RunOpts, RunResult, Task, Phase, StepRecord

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 4-8hrs |
| Dependencies | T-001 |
| Blocked By | T-001 |
| Blocks | T-031, T-043, T-045, T-046, T-066, T-067 |

## Goal
Define the central data types specified in PRD Section 6.4 that form the contract between all major subsystems: workflow engine, agent adapters, task management, and the TUI. These types are referenced by nearly every subsequent task and must be defined early with stable JSON serialization for checkpoint persistence.

## Background
Per PRD Section 6.4, Raven has six core type groups: `WorkflowState` and `StepRecord` (workflow engine), `RunOpts` and `RunResult` with `RateLimitInfo` (agent system), and `Task` and `Phase` (task management). These types appear in the PRD with exact field definitions and JSON struct tags, and they are referenced by T-031 (review types), T-043 (workflow events), T-045 (engine core), T-046 (checkpointing), T-066 (TUI scaffold), and T-067 (TUI message types).

The types must be placed in appropriate packages per the project structure: workflow types in `internal/workflow/`, agent types in `internal/agent/`, and task types in `internal/task/`. Each type must have proper JSON tags for serialization and doc comments for clarity.

## Technical Specifications
### Implementation Approach
Create type definition files in three packages: `internal/workflow/state.go` for workflow types, `internal/agent/types.go` for agent types, and `internal/task/types.go` for task types. Each file contains the types from PRD Section 6.4 with JSON struct tags exactly as specified. Include validation methods, constructor functions, and status constants where applicable.

### Key Components
- **WorkflowState**: Core state machine state with step history and metadata
- **StepRecord**: Record of a single step execution (timestamp, duration, event, error)
- **RunOpts**: Options for invoking an agent (prompt, model, effort, tools, working directory)
- **RunResult**: Result from an agent invocation (stdout, stderr, exit code, duration, rate-limit info)
- **RateLimitInfo**: Rate-limit detection result (is limited, reset duration, message)
- **Task**: Task specification (ID, title, status, phase, dependencies, spec file path)
- **Phase**: Phase definition (ID, name, task range)

### API/Interface Contracts
```go
// internal/workflow/state.go

// Package workflow implements the generic workflow state machine engine.
package workflow

import "time"

// WorkflowState holds the current state of a workflow execution.
// It is persisted as JSON to .raven/state/<id>.json after every transition.
type WorkflowState struct {
    ID           string                 `json:"id"`
    WorkflowName string                 `json:"workflow_name"`
    CurrentStep  string                 `json:"current_step"`
    StepHistory  []StepRecord           `json:"step_history"`
    Metadata     map[string]interface{} `json:"metadata"`
    CreatedAt    time.Time              `json:"created_at"`
    UpdatedAt    time.Time              `json:"updated_at"`
}

// StepRecord captures the execution details of a single workflow step.
type StepRecord struct {
    Step      string        `json:"step"`
    Event     string        `json:"event"`
    StartedAt time.Time     `json:"started_at"`
    Duration  time.Duration `json:"duration"`
    Error     string        `json:"error,omitempty"`
}

// NewWorkflowState creates a new WorkflowState with the given ID and workflow name.
func NewWorkflowState(id, workflowName, initialStep string) *WorkflowState

// AddStepRecord appends a completed step record and updates the timestamp.
func (ws *WorkflowState) AddStepRecord(record StepRecord)

// LastStep returns the most recent step record, or nil if no steps executed.
func (ws *WorkflowState) LastStep() *StepRecord
```

```go
// internal/agent/types.go

// Package agent defines the agent adapter interface and shared types.
package agent

import "time"

// RunOpts specifies options for a single agent invocation.
type RunOpts struct {
    Prompt       string   `json:"prompt,omitempty"`
    PromptFile   string   `json:"prompt_file,omitempty"`
    Model        string   `json:"model,omitempty"`
    Effort       string   `json:"effort,omitempty"`
    AllowedTools string   `json:"allowed_tools,omitempty"`
    OutputFormat string   `json:"output_format,omitempty"`
    WorkDir      string   `json:"work_dir,omitempty"`
    Env          []string `json:"env,omitempty"`
}

// RunResult captures the output of an agent invocation.
type RunResult struct {
    Stdout    string        `json:"stdout"`
    Stderr    string        `json:"stderr"`
    ExitCode  int           `json:"exit_code"`
    Duration  time.Duration `json:"duration"`
    RateLimit *RateLimitInfo `json:"rate_limit,omitempty"`
}

// RateLimitInfo describes a detected rate-limit condition.
type RateLimitInfo struct {
    IsLimited  bool          `json:"is_limited"`
    ResetAfter time.Duration `json:"reset_after"`
    Message    string        `json:"message"`
}

// Success returns true if the agent exited with code 0.
func (r *RunResult) Success() bool

// WasRateLimited returns true if the result indicates a rate-limit condition.
func (r *RunResult) WasRateLimited() bool
```

```go
// internal/task/types.go

// Package task provides task discovery, state management, and selection.
package task

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
    StatusNotStarted TaskStatus = "not_started"
    StatusInProgress TaskStatus = "in_progress"
    StatusCompleted  TaskStatus = "completed"
    StatusBlocked    TaskStatus = "blocked"
    StatusSkipped    TaskStatus = "skipped"
)

// Task represents a single implementation task parsed from a markdown spec file.
type Task struct {
    ID           string     `json:"id"`
    Title        string     `json:"title"`
    Status       TaskStatus `json:"status"`
    Phase        int        `json:"phase"`
    Dependencies []string   `json:"dependencies"`
    SpecFile     string     `json:"spec_file"`
}

// Phase represents a group of related tasks executed together.
type Phase struct {
    ID        int    `json:"id"`
    Name      string `json:"name"`
    StartTask string `json:"start_task"`
    EndTask   string `json:"end_task"`
}

// IsReady returns true if all dependencies are in the completed set.
func (t *Task) IsReady(completedTasks map[string]bool) bool

// ValidStatus returns true if the status is a known TaskStatus value.
func ValidStatus(s TaskStatus) bool
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| time | stdlib | Duration and timestamp types |
| encoding/json | stdlib | JSON marshaling/unmarshaling for persistence |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] `internal/workflow/state.go` contains `WorkflowState` and `StepRecord` exactly matching PRD Section 6.4 field names and types
- [ ] `internal/agent/types.go` contains `RunOpts`, `RunResult`, and `RateLimitInfo` matching PRD Section 6.4
- [ ] `internal/task/types.go` contains `Task` and `Phase` matching PRD Section 6.4
- [ ] All types have JSON struct tags for serialization
- [ ] `TaskStatus` is a typed string constant with all five statuses from PRD Section 5.3
- [ ] `WorkflowState` has constructor `NewWorkflowState()` and helper methods
- [ ] `RunResult` has `Success()` and `WasRateLimited()` helper methods
- [ ] `Task.IsReady()` correctly checks dependencies against a completed set
- [ ] JSON round-trip (marshal then unmarshal) preserves all fields for every type
- [ ] All types have doc comments
- [ ] Unit tests achieve 95% coverage across all three files
- [ ] `go vet ./...` passes

## Testing Requirements
### Unit Tests
- WorkflowState JSON round-trip preserves all fields including Metadata map
- WorkflowState.AddStepRecord appends to history and updates UpdatedAt
- WorkflowState.LastStep returns nil for empty history, correct record otherwise
- StepRecord with empty Error field omits "error" key in JSON (omitempty)
- RunOpts JSON round-trip with all fields populated
- RunOpts JSON round-trip with only Prompt set (omitempty for others)
- RunResult.Success() returns true for ExitCode 0, false otherwise
- RunResult.WasRateLimited() returns true when RateLimit.IsLimited is true
- RateLimitInfo with zero Duration serializes correctly
- Task.IsReady() with no dependencies returns true
- Task.IsReady() with all dependencies completed returns true
- Task.IsReady() with some dependencies not completed returns false
- TaskStatus constants serialize to their string values
- ValidStatus returns true for known statuses, false for unknown
- Phase JSON round-trip preserves all fields

### Integration Tests
- None required (pure data types)

### Edge Cases to Handle
- WorkflowState.Metadata with nested maps and slices (must round-trip via `map[string]interface{}`)
- StepRecord.Duration is `time.Duration` (int64 nanoseconds in JSON by default) -- document this
- RunOpts with both Prompt and PromptFile set (document which takes precedence -- decided by agent adapter)
- Task with empty Dependencies slice vs nil Dependencies slice (both should serialize as `[]`)
- Phase with ID 0 (valid, represents the "setup" phase)

## Implementation Notes
### Recommended Approach
1. Create `internal/workflow/state.go` with WorkflowState and StepRecord
2. Create `internal/agent/types.go` with RunOpts, RunResult, RateLimitInfo
3. Create `internal/task/types.go` with Task, Phase, TaskStatus
4. Add constructor/helper methods to each type
5. Create test files with table-driven tests for JSON round-trips and helper methods
6. Verify: `go build ./... && go vet ./... && go test ./...`

### Potential Pitfalls
- `time.Duration` serializes as nanoseconds (int64) in JSON by default. This is the Go standard behavior and should be documented. Consumers that need human-readable durations should format on display, not in serialization.
- The `Metadata map[string]interface{}` field in WorkflowState will deserialize JSON numbers as `float64` (Go's `encoding/json` behavior). If integer precision matters, consumers must handle this.
- Do not use `iota` for TaskStatus -- it must serialize to its string value in JSON (same rationale as T-031's Verdict type).
- Ensure `Dependencies []string` serializes as `[]` (empty array), not `null`, when empty. Initialize to `[]string{}` in constructors.

### Security Considerations
- RunOpts.Env may contain sensitive environment variables -- ensure these are not logged at info level
- WorkflowState.Metadata may contain project-specific data -- sanitize before displaying in TUI or logs

## References
- [PRD Section 6.4 - Central Data Types](docs/prd/PRD-Raven.md)
- [PRD Section 5.1 - Workflow State](docs/prd/PRD-Raven.md)
- [PRD Section 5.2 - Agent RunOpts/RunResult](docs/prd/PRD-Raven.md)
- [PRD Section 5.3 - Task Status Values](docs/prd/PRD-Raven.md)
- [Go encoding/json documentation](https://pkg.go.dev/encoding/json)