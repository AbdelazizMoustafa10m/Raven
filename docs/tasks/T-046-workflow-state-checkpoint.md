# T-046: Workflow State Checkpointing and Persistence

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-004, T-043, T-045 |
| Blocked By | T-045 |
| Blocks | T-047, T-050 |

## Goal
Implement workflow state persistence that checkpoints `WorkflowState` to disk as JSON after every state transition, and provides methods to list, load, and resume persisted workflow runs. This enables Raven's resumability guarantee: when a workflow fails or is interrupted, `raven resume` picks up exactly where it left off.

## Background
Per PRD Section 5.1, "Workflow state is checkpointed to disk after every state transition (`$PROJECT_ROOT/.raven/state/<workflow-run-id>.json`)." The checkpoint format is "JSON with workflow ID, current step, step history (timestamps, events, durations), metadata map." The PRD also specifies `raven resume [--run <id>]` to continue from the last checkpoint and `raven resume --list` to show all resumable workflow runs.

Checkpoints must be written atomically (write to temp file, then rename) to prevent corruption from crashes during write. The state directory (`.raven/state/`) must be created on first use.

## Technical Specifications
### Implementation Approach
Create `internal/workflow/state.go` with a `StateStore` struct that manages checkpoint persistence. The store takes a root directory path (typically `.raven/state/`) and provides `Save`, `Load`, `List`, and `Delete` methods. `Save` performs an atomic write (write to `.tmp` file, fsync, rename). `Load` reads and unmarshals a specific run's checkpoint. `List` scans the directory for all `.json` files and returns summaries. The workflow engine (T-045) calls `Save` after every state transition via a callback or middleware pattern.

### Key Components
- **StateStore**: Manages checkpoint file I/O in the `.raven/state/` directory
- **CheckpointMiddleware**: Wraps the engine's step execution to auto-save after each transition
- **RunSummary**: Lightweight struct for `resume --list` display (ID, workflow name, current step, updated time, status)
- **Atomic write**: Write to temp file, fsync, rename to prevent corruption

### API/Interface Contracts
```go
// internal/workflow/state.go

// StateStore manages workflow state persistence to disk.
type StateStore struct {
    dir string // e.g., ".raven/state"
}

// NewStateStore creates a store backed by the given directory.
// Creates the directory if it does not exist.
func NewStateStore(dir string) (*StateStore, error)

// Save atomically writes the workflow state to a checkpoint file.
// File path: <dir>/<state.ID>.json
func (s *StateStore) Save(state *WorkflowState) error

// Load reads and unmarshals a workflow state from its checkpoint file.
func (s *StateStore) Load(runID string) (*WorkflowState, error)

// List returns summaries of all persisted workflow runs, sorted by UpdatedAt descending.
func (s *StateStore) List() ([]RunSummary, error)

// Delete removes a checkpoint file for the given run ID.
func (s *StateStore) Delete(runID string) error

// LatestRun returns the most recently updated workflow run, or nil if none exist.
func (s *StateStore) LatestRun() (*WorkflowState, error)

// RunSummary is a lightweight view of a persisted run for listing.
type RunSummary struct {
    ID           string    `json:"id"`
    WorkflowName string    `json:"workflow_name"`
    CurrentStep  string    `json:"current_step"`
    Status       string    `json:"status"` // running, completed, failed, interrupted
    UpdatedAt    time.Time `json:"updated_at"`
    StepCount    int       `json:"step_count"`
}

// StatusFromState derives a display status from WorkflowState.
func StatusFromState(state *WorkflowState) string

// WithCheckpointing returns an EngineOption that auto-saves state after each step.
func WithCheckpointing(store *StateStore) EngineOption
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/workflow (T-004) | - | WorkflowState type |
| encoding/json | stdlib | JSON marshaling |
| os | stdlib | File I/O, directory creation |
| filepath | stdlib | Path construction |
| sort | stdlib | Sorting run summaries |
| io | stdlib | File sync (Fsync) |

## Acceptance Criteria
- [ ] StateStore creates `.raven/state/` directory on first use if it does not exist
- [ ] Save writes `WorkflowState` as indented JSON to `<dir>/<id>.json`
- [ ] Save is atomic: writes to temp file, fsyncs, then renames (no partial writes visible)
- [ ] Load reads and unmarshals a checkpoint by run ID
- [ ] Load returns clear error for nonexistent run ID
- [ ] List returns all runs sorted by UpdatedAt descending
- [ ] List handles empty directory (returns empty slice, no error)
- [ ] Delete removes the checkpoint file
- [ ] LatestRun returns the most recently updated run
- [ ] WithCheckpointing integrates with the engine to auto-save after each step
- [ ] StatusFromState correctly derives: "running" (has current step, no terminal), "completed" (current step is done), "failed" (current step is failed or last event is failure), "interrupted" (other)
- [ ] Unit tests achieve 90% coverage using t.TempDir()

## Testing Requirements
### Unit Tests
- Save and Load round-trip: state survives serialization
- Save creates directory if missing
- Save is atomic: concurrent reads during write never see partial data (simulate with goroutines)
- Load nonexistent ID returns error
- List with 0 runs returns empty slice
- List with 3 runs returns them sorted by UpdatedAt desc
- Delete removes file, subsequent Load returns error
- LatestRun returns correct run when multiple exist
- StatusFromState with terminal done step returns "completed"
- StatusFromState with terminal failed step returns "failed"
- StatusFromState with active step returns "running"
- WithCheckpointing: engine saves state after each step execution

### Integration Tests
- Full workflow run with checkpointing: verify checkpoint files created at each step
- Resume from checkpoint: load state, feed to engine, verify execution continues from correct step

### Edge Cases to Handle
- Run ID containing special characters (sanitize for filesystem safety)
- Checkpoint file with corrupt JSON (return parse error, do not crash)
- Disk full during Save (return write error)
- Permission denied on state directory (return clear error)
- Multiple concurrent saves for different workflow IDs (no conflicts)
- Very large metadata map in WorkflowState (test with 100+ keys)

## Implementation Notes
### Recommended Approach
1. Create `StateStore` with `dir` field, `NewStateStore` creates dir with `os.MkdirAll`
2. `Save`: marshal to JSON with `json.MarshalIndent`, write to `<id>.json.tmp`, call `f.Sync()`, then `os.Rename` to `<id>.json`
3. `Load`: read file with `os.ReadFile`, unmarshal with `json.Unmarshal`
4. `List`: read directory with `os.ReadDir`, filter for `.json` files, load each to build RunSummary (or parse just the needed fields for performance)
5. For `List` performance: consider reading only the first few hundred bytes of each file to extract summary fields, rather than full unmarshal
6. `WithCheckpointing`: return an `EngineOption` that sets a post-step callback on the engine; the engine calls this callback after recording each StepRecord
7. Sanitize run IDs for filesystem: replace characters not in `[a-zA-Z0-9_-]` with `_`

### Potential Pitfalls
- Atomic rename on Windows: `os.Rename` may fail if the target file is open by another process. For v2.0 this is acceptable; document the limitation
- Do not use `json.MarshalIndent` with tabs -- use 2-space indent for human readability
- The `List` method should not fail if one checkpoint file is corrupt -- skip it and log a warning
- Ensure `f.Sync()` is called before `os.Rename()` to prevent data loss on power failure (especially on Linux ext4 with delayed allocation)

### Security Considerations
- Checkpoint files may contain workflow metadata with sensitive data (agent prompts, file paths) -- document that `.raven/` should be in `.gitignore`
- Validate that the state directory path does not traverse outside the project root

## References
- [PRD Section 5.1 - Workflow state checkpointing](docs/prd/PRD-Raven.md)
- [Atomically writing files in Go](https://michael.stapelberg.ch/posts/2017-01-28-golang_atomically_writing/)
- [google/renameio - atomic file operations](https://pkg.go.dev/github.com/google/renameio)
- [natefinch/atomic - atomic file writing](https://github.com/natefinch/atomic)