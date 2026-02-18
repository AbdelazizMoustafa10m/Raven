package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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

// RunSummary is a lightweight view of a persisted workflow run used for
// listing and status display without loading the full state.
type RunSummary struct {
	ID           string    `json:"id"`
	WorkflowName string    `json:"workflow_name"`
	CurrentStep  string    `json:"current_step"`
	Status       string    `json:"status"` // running, completed, failed, interrupted
	UpdatedAt    time.Time `json:"updated_at"`
	StepCount    int       `json:"step_count"`
}

// StatusFromState derives a human-readable status string from a WorkflowState.
// It returns:
//   - "completed"   if state.CurrentStep is StepDone
//   - "failed"      if state.CurrentStep is StepFailed or the last step event is EventFailure
//   - "running"     if the state has recorded steps and the current step is not terminal
//   - "interrupted" otherwise (no steps recorded and not at a terminal step)
func StatusFromState(state *WorkflowState) string {
	switch state.CurrentStep {
	case StepDone:
		return "completed"
	case StepFailed:
		return "failed"
	}
	last := state.LastStep()
	if last != nil && last.Event == EventFailure {
		return "failed"
	}
	if len(state.StepHistory) > 0 {
		return "running"
	}
	return "interrupted"
}

// StateStore persists and retrieves WorkflowState instances as JSON files
// inside a single directory. Each run is stored as <sanitizedID>.json.
// Writes are atomic: the file is written to a .tmp path and then renamed.
type StateStore struct {
	dir string
}

// NewStateStore creates a StateStore backed by the given directory.
// The directory is created (including all parents) if it does not exist.
func NewStateStore(dir string) (*StateStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("state: create store directory %q: %w", dir, err)
	}
	return &StateStore{dir: dir}, nil
}

// Save serialises state to disk using an atomic write (write-to-tmp, fsync,
// rename). The run ID is sanitised before use as a filename component.
func (s *StateStore) Save(state *WorkflowState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("state: marshal checkpoint for run %q: %w", state.ID, err)
	}

	safe := sanitizeID(state.ID)
	tmpPath := filepath.Join(s.dir, safe+".json.tmp")
	finalPath := filepath.Join(s.dir, safe+".json")

	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("state: write temp checkpoint: %w", err)
	}

	f, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("state: open temp for sync: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("state: sync temp checkpoint: %w", err)
	}
	f.Close()

	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("state: rename checkpoint: %w", err)
	}
	return nil
}

// Load reads and deserialises the WorkflowState for the given run ID.
// It returns a descriptive error if the run does not exist or is corrupt.
func (s *StateStore) Load(runID string) (*WorkflowState, error) {
	path := filepath.Join(s.dir, sanitizeID(runID)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("state: run %q not found", runID)
		}
		return nil, fmt.Errorf("state: read checkpoint: %w", err)
	}
	var state WorkflowState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("state: parse checkpoint for run %q: %w", runID, err)
	}
	return &state, nil
}

// List returns a RunSummary for every persisted run, sorted descending by
// UpdatedAt (most recent first). Corrupt or unreadable files are silently
// skipped. Returns an empty slice (never nil) when the store is empty.
func (s *StateStore) List() ([]RunSummary, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []RunSummary{}, nil
		}
		return nil, fmt.Errorf("state: read directory: %w", err)
	}

	var summaries []RunSummary
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			// skip unreadable files
			continue
		}
		var state WorkflowState
		if err := json.Unmarshal(data, &state); err != nil {
			// skip corrupt files
			continue
		}
		summaries = append(summaries, RunSummary{
			ID:           state.ID,
			WorkflowName: state.WorkflowName,
			CurrentStep:  state.CurrentStep,
			Status:       StatusFromState(&state),
			UpdatedAt:    state.UpdatedAt,
			StepCount:    len(state.StepHistory),
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})

	if summaries == nil {
		summaries = []RunSummary{}
	}
	return summaries, nil
}

// Delete removes the persisted state for the given run ID.
// It returns an error if the run does not exist.
func (s *StateStore) Delete(runID string) error {
	path := filepath.Join(s.dir, sanitizeID(runID)+".json")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("state: run %q not found", runID)
		}
		return fmt.Errorf("state: delete checkpoint: %w", err)
	}
	return nil
}

// LatestRun returns the most recently updated WorkflowState in the store,
// or nil if the store is empty.
func (s *StateStore) LatestRun() (*WorkflowState, error) {
	summaries, err := s.List()
	if err != nil {
		return nil, err
	}
	if len(summaries) == 0 {
		return nil, nil
	}
	return s.Load(summaries[0].ID)
}

// WithCheckpointing returns an EngineOption that auto-saves the WorkflowState
// to store after every step completes. Hook errors are logged but do not abort
// the workflow.
func WithCheckpointing(store *StateStore) EngineOption {
	return func(e *Engine) {
		e.postStepHook = func(state *WorkflowState) error {
			return store.Save(state)
		}
	}
}

// sanitizeID replaces any character outside [a-zA-Z0-9_-] with an underscore
// so that run IDs are safe to use as filesystem filenames.
func sanitizeID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}
