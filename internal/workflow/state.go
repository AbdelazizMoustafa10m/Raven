package workflow

import "time"

// WorkflowState holds the current state of a workflow execution.
// It is persisted as JSON to .raven/state/<id>.json after every transition.
type WorkflowState struct {
	ID           string         `json:"id"`
	WorkflowName string         `json:"workflow_name"`
	CurrentStep  string         `json:"current_step"`
	StepHistory  []StepRecord   `json:"step_history"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// StepRecord captures the execution details of a single workflow step.
// Duration is serialized as nanoseconds (int64) in JSON, which is the
// default Go behavior for time.Duration.
type StepRecord struct {
	Step      string        `json:"step"`
	Event     string        `json:"event"`
	StartedAt time.Time     `json:"started_at"`
	Duration  time.Duration `json:"duration"`
	Error     string        `json:"error,omitempty"`
}

// NewWorkflowState creates a new WorkflowState with the given ID, workflow name,
// and initial step. It initializes StepHistory to an empty slice (not nil) and
// Metadata to an empty map so that JSON serialization produces [] and {} rather
// than null.
func NewWorkflowState(id, workflowName, initialStep string) *WorkflowState {
	now := time.Now()
	return &WorkflowState{
		ID:           id,
		WorkflowName: workflowName,
		CurrentStep:  initialStep,
		StepHistory:  []StepRecord{},
		Metadata:     map[string]any{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// AddStepRecord appends a completed step record and updates the UpdatedAt timestamp.
func (ws *WorkflowState) AddStepRecord(record StepRecord) {
	ws.StepHistory = append(ws.StepHistory, record)
	ws.UpdatedAt = time.Now()
}

// LastStep returns the most recent step record, or nil if no steps have been executed.
func (ws *WorkflowState) LastStep() *StepRecord {
	if len(ws.StepHistory) == 0 {
		return nil
	}
	return &ws.StepHistory[len(ws.StepHistory)-1]
}
