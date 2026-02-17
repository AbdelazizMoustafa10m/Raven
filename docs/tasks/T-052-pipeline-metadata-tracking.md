# T-052: Pipeline Metadata Tracking

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Small: 2-4hrs |
| Dependencies | T-050 |
| Blocked By | T-050 |
| Blocks | T-055 |

## Goal
Implement the pipeline metadata tracking subsystem that records per-phase and pipeline-level execution metadata: implementation status, review verdict, fix status, PR URL, branch name, timing, and error information. This metadata enables pipeline resume, progress reporting, and the TUI dashboard to display pipeline status.

## Background
Per PRD Section 5.9, "Pipeline metadata tracking: implementation status, review verdict, fix status, PR URL per phase." The metadata is persisted as part of the pipeline's `WorkflowState.Metadata` map, which is checkpointed via T-046. The metadata is structured as a `PipelineMetadata` type that is serialized/deserialized from the generic metadata map.

Pipeline metadata serves three consumers: (1) the pipeline orchestrator itself (for resume -- which phases are done?), (2) the TUI dashboard (for live status display), and (3) the `raven status` command (for offline status reporting).

## Technical Specifications
### Implementation Approach
Create `internal/pipeline/metadata.go` with a `PipelineMetadata` struct that tracks overall pipeline progress and per-phase results. Provide methods to serialize/deserialize from `map[string]interface{}` (the WorkflowState.Metadata type), update phase status, and query pipeline state. The metadata is updated by the pipeline orchestrator (T-050) after each phase stage completion.

### Key Components
- **PipelineMetadata**: Top-level struct with pipeline-wide and per-phase data
- **PhaseMetadata**: Per-phase tracking with stage-level status
- **Serialization helpers**: To/from map[string]interface{} for WorkflowState.Metadata
- **Status queries**: Helper methods to derive overall status, find next phase, check completion

### API/Interface Contracts
```go
// internal/pipeline/metadata.go

type PipelineMetadata struct {
    PipelineID    string          `json:"pipeline_id"`
    WorkflowName  string          `json:"workflow_name"`
    StartedAt     time.Time       `json:"started_at"`
    CompletedAt   *time.Time      `json:"completed_at,omitempty"`
    Status        string          `json:"status"` // running, completed, partial, failed
    Phases        []PhaseMetadata `json:"phases"`
    CurrentPhase  int             `json:"current_phase"` // index into Phases
    TotalPhases   int             `json:"total_phases"`
    Opts          PipelineOpts    `json:"opts"` // original options for resume
}

type PhaseMetadata struct {
    PhaseID       int           `json:"phase_id"`
    PhaseName     string        `json:"phase_name"`
    BranchName    string        `json:"branch_name"`
    Status        string        `json:"status"` // pending, implementing, reviewing, fixing, pr_creating, completed, failed, skipped
    ImplStatus    string        `json:"impl_status"` // pending, running, completed, failed, skipped
    ReviewVerdict string        `json:"review_verdict"` // pending, approved, changes_needed, blocking, skipped
    FixStatus     string        `json:"fix_status"` // pending, running, completed, failed, skipped
    PRURL         string        `json:"pr_url"`
    PRStatus      string        `json:"pr_status"` // pending, created, failed, skipped
    StartedAt     *time.Time    `json:"started_at,omitempty"`
    CompletedAt   *time.Time    `json:"completed_at,omitempty"`
    Duration      time.Duration `json:"duration"`
    ReviewCycles  int           `json:"review_cycles"` // number of review-fix iterations
    Error         string        `json:"error,omitempty"`
}

// NewPipelineMetadata creates initial metadata for a pipeline run.
func NewPipelineMetadata(pipelineID string, phases []task.Phase, opts PipelineOpts) *PipelineMetadata

// ToMetadataMap serializes PipelineMetadata into a map for WorkflowState.Metadata.
func (pm *PipelineMetadata) ToMetadataMap() map[string]interface{}

// PipelineMetadataFromMap deserializes PipelineMetadata from WorkflowState.Metadata.
func PipelineMetadataFromMap(m map[string]interface{}) (*PipelineMetadata, error)

// UpdatePhaseStatus updates the status of a specific phase.
func (pm *PipelineMetadata) UpdatePhaseStatus(phaseIndex int, status string)

// UpdatePhaseStage updates a specific stage within a phase.
func (pm *PipelineMetadata) UpdatePhaseStage(phaseIndex int, stage string, status string)

// SetPhaseResult records the final result for a phase.
func (pm *PipelineMetadata) SetPhaseResult(phaseIndex int, result PhaseResult)

// NextIncompletePhase returns the index of the first non-completed phase, or -1.
func (pm *PipelineMetadata) NextIncompletePhase() int

// IsComplete returns true if all phases are completed or skipped.
func (pm *PipelineMetadata) IsComplete() bool

// Summary returns a human-readable summary of pipeline progress.
func (pm *PipelineMetadata) Summary() string
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/task | - | Phase type for initialization |
| internal/pipeline (T-050) | - | PipelineOpts, PhaseResult types |
| encoding/json | stdlib | Serialization to/from map |
| time | stdlib | Timestamps and durations |

## Acceptance Criteria
- [ ] PipelineMetadata tracks pipeline-wide status (running, completed, partial, failed)
- [ ] PhaseMetadata tracks per-phase status and per-stage status (impl, review, fix, PR)
- [ ] ToMetadataMap/PipelineMetadataFromMap round-trip correctly
- [ ] UpdatePhaseStatus updates the correct phase
- [ ] UpdatePhaseStage updates specific stage (impl_status, review_verdict, fix_status, pr_status)
- [ ] SetPhaseResult records timing, error, and final status
- [ ] NextIncompletePhase returns correct index for resume
- [ ] IsComplete returns true only when all phases are completed or skipped
- [ ] Summary produces human-readable progress text
- [ ] Unit tests achieve 95% coverage

## Testing Requirements
### Unit Tests
- NewPipelineMetadata: creates correct number of PhaseMetadata entries
- ToMetadataMap and PipelineMetadataFromMap: round-trip preserves all fields
- UpdatePhaseStatus: correct phase updated, others unchanged
- UpdatePhaseStage with "impl": updates impl_status
- UpdatePhaseStage with "review": updates review_verdict
- UpdatePhaseStage with "fix": updates fix_status
- UpdatePhaseStage with "pr": updates pr_status
- SetPhaseResult: records all fields
- NextIncompletePhase with all pending: returns 0
- NextIncompletePhase with first completed: returns 1
- NextIncompletePhase with all completed: returns -1
- NextIncompletePhase with skipped phases: skips them
- IsComplete with all completed: returns true
- IsComplete with one pending: returns false
- IsComplete with mix of completed and skipped: returns true
- Summary includes phase count and status

### Integration Tests
- Pipeline metadata serialized to WorkflowState, checkpointed, loaded, and deserialized correctly

### Edge Cases to Handle
- Empty phases list (pipeline with no phases)
- Phase index out of bounds (return error, do not panic)
- Metadata map with missing keys (graceful deserialization with defaults)
- Very long PR URLs or error messages (truncation for display)

## Implementation Notes
### Recommended Approach
1. Define `PipelineMetadata` and `PhaseMetadata` structs with JSON tags
2. `ToMetadataMap`: marshal to JSON, then unmarshal into `map[string]interface{}` (simple but effective for the `Metadata` type in WorkflowState)
3. `PipelineMetadataFromMap`: marshal map to JSON, then unmarshal into `PipelineMetadata`
4. Update methods modify the struct directly (caller is responsible for re-serializing)
5. `Summary` uses a format like: "Pipeline: 2/5 phases complete | Current: Phase 3 (reviewing)"

### Potential Pitfalls
- `map[string]interface{}` loses type information for nested structs -- use JSON round-trip for conversion rather than direct type assertions
- `time.Duration` does not serialize cleanly to JSON -- consider storing as int64 nanoseconds or string
- `time.Time` pointers need nil handling during serialization

### Security Considerations
- PR URLs in metadata may be sensitive (private repos) -- metadata is stored in .raven/ (gitignored)
- No user input directly enters metadata -- all values are from internal state

## References
- [PRD Section 5.9 - Pipeline metadata tracking](docs/prd/PRD-Raven.md)
- [Go encoding/json documentation](https://pkg.go.dev/encoding/json)