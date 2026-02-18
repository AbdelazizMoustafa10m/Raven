package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AbdelazizMoustafa10m/Raven/internal/task"
)

// PipelineStatusRunning indicates a pipeline run is actively in progress.
const PipelineStatusRunning = "running"

// PipelineMetadata tracks pipeline-wide and per-phase execution information.
// It is designed for JSON serialization and is persisted as part of the
// pipeline checkpoint in WorkflowState.Metadata.
type PipelineMetadata struct {
	// PipelineID is the unique identifier for this pipeline run.
	PipelineID string `json:"pipeline_id"`

	// WorkflowName is the name of the workflow used by this pipeline.
	WorkflowName string `json:"workflow_name"`

	// StartedAt records when the pipeline began execution.
	StartedAt time.Time `json:"started_at"`

	// CompletedAt records when the pipeline finished; nil if still running.
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Status is the overall pipeline status: running, completed, partial, failed.
	Status string `json:"status"`

	// Phases holds per-phase metadata indexed in execution order.
	Phases []PhaseMetadata `json:"phases"`

	// CurrentPhase is the index into Phases of the actively executing phase.
	CurrentPhase int `json:"current_phase"`

	// TotalPhases is the total number of phases in this pipeline run.
	TotalPhases int `json:"total_phases"`

	// Opts stores the original PipelineOpts so execution can be resumed.
	Opts PipelineOpts `json:"opts"`
}

// PhaseMetadata tracks the execution state of a single phase within a pipeline.
type PhaseMetadata struct {
	// PhaseID is the numeric identifier for this phase.
	PhaseID int `json:"phase_id"`

	// PhaseName is the human-readable name of this phase.
	PhaseName string `json:"phase_name"`

	// BranchName is the git branch created for this phase.
	BranchName string `json:"branch_name"`

	// Status is the overall phase lifecycle status:
	// pending, implementing, reviewing, fixing, pr_creating, completed, failed, skipped.
	Status string `json:"status"`

	// ImplStatus tracks the implementation step: pending, running, completed, failed, skipped.
	ImplStatus string `json:"impl_status"`

	// ReviewVerdict records the review outcome: pending, approved, changes_needed, blocking, skipped.
	ReviewVerdict string `json:"review_verdict"`

	// FixStatus tracks the fix step: pending, running, completed, failed, skipped.
	FixStatus string `json:"fix_status"`

	// PRURL is the URL of the pull request created for this phase, if any.
	PRURL string `json:"pr_url"`

	// PRStatus tracks the PR creation step: pending, created, failed, skipped.
	PRStatus string `json:"pr_status"`

	// StartedAt records when this phase began; nil if not yet started.
	StartedAt *time.Time `json:"started_at,omitempty"`

	// CompletedAt records when this phase finished; nil if still running.
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Duration stores elapsed time in nanoseconds. Use time.Duration(pm.Duration)
	// to convert back to time.Duration. Stored as int64 to avoid JSON
	// serialisation issues with time.Duration (which marshals as a number anyway
	// but this makes the intent explicit).
	Duration int64 `json:"duration_ns"`

	// ReviewCycles counts how many review-fix iterations occurred for this phase.
	ReviewCycles int `json:"review_cycles"`

	// Error holds an error message if the phase failed.
	Error string `json:"error,omitempty"`
}

// NewPipelineMetadata creates initial PipelineMetadata for a pipeline run,
// populating one pending PhaseMetadata entry for each supplied phase.
func NewPipelineMetadata(pipelineID string, phases []task.Phase, opts PipelineOpts) *PipelineMetadata {
	phasesMeta := make([]PhaseMetadata, len(phases))
	for i, ph := range phases {
		phasesMeta[i] = PhaseMetadata{
			PhaseID:       ph.ID,
			PhaseName:     ph.Name,
			BranchName:    phaseBranchName(fmt.Sprintf("%d", ph.ID)),
			Status:        PhaseStatusPending,
			ImplStatus:    PhaseStatusPending,
			ReviewVerdict: "pending",
			FixStatus:     PhaseStatusPending,
			PRStatus:      "pending",
		}
	}
	return &PipelineMetadata{
		PipelineID:   pipelineID,
		WorkflowName: "pipeline",
		StartedAt:    time.Now(),
		Status:       PipelineStatusRunning,
		Phases:       phasesMeta,
		CurrentPhase: 0,
		TotalPhases:  len(phases),
		Opts:         opts,
	}
}

// ToMetadataMap serializes PipelineMetadata into a map[string]interface{} suitable
// for storage in WorkflowState.Metadata. It uses a JSON round-trip to ensure
// all values are JSON-compatible primitives.
func (pm *PipelineMetadata) ToMetadataMap() map[string]interface{} {
	b, err := json.Marshal(pm)
	if err != nil {
		// Return a minimal map on marshal failure; this should never happen with
		// well-formed PipelineMetadata.
		return map[string]interface{}{
			"pipeline_id": pm.PipelineID,
			"status":      pm.Status,
		}
	}
	var m map[string]interface{}
	// Unmarshal error into an empty map is safe to ignore here; a nil map would
	// only arise if the JSON is invalid, which cannot happen from a valid Marshal.
	_ = json.Unmarshal(b, &m)
	if m == nil {
		m = make(map[string]interface{})
	}
	return m
}

// PipelineMetadataFromMap deserializes PipelineMetadata from a
// map[string]interface{} (e.g. WorkflowState.Metadata). It uses a JSON
// round-trip for type safety.
func PipelineMetadataFromMap(m map[string]interface{}) (*PipelineMetadata, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("pipeline metadata: marshal map: %w", err)
	}
	var pm PipelineMetadata
	if err := json.Unmarshal(b, &pm); err != nil {
		return nil, fmt.Errorf("pipeline metadata: unmarshal: %w", err)
	}
	return &pm, nil
}

// UpdatePhaseStatus sets the Status field for the phase at phaseIndex.
// It returns silently if phaseIndex is out of bounds.
func (pm *PipelineMetadata) UpdatePhaseStatus(phaseIndex int, status string) {
	if phaseIndex < 0 || phaseIndex >= len(pm.Phases) {
		return
	}
	pm.Phases[phaseIndex].Status = status
}

// UpdatePhaseStage sets a stage-specific status field for the phase at phaseIndex.
// stage must be one of "impl", "review", "fix", or "pr".
// It returns silently if phaseIndex is out of bounds.
func (pm *PipelineMetadata) UpdatePhaseStage(phaseIndex int, stage string, status string) {
	if phaseIndex < 0 || phaseIndex >= len(pm.Phases) {
		return
	}
	switch stage {
	case "impl":
		pm.Phases[phaseIndex].ImplStatus = status
	case "review":
		pm.Phases[phaseIndex].ReviewVerdict = status
	case "fix":
		pm.Phases[phaseIndex].FixStatus = status
	case "pr":
		pm.Phases[phaseIndex].PRStatus = status
	}
}

// SetPhaseResult records the final result for the phase at phaseIndex.
// It updates Status, ImplStatus, ReviewVerdict, FixStatus, PRURL, PRStatus,
// Error, and Duration from the supplied PhaseResult. CompletedAt is set to
// the current time.
// It returns silently if phaseIndex is out of bounds.
func (pm *PipelineMetadata) SetPhaseResult(phaseIndex int, result PhaseResult) {
	if phaseIndex < 0 || phaseIndex >= len(pm.Phases) {
		return
	}
	now := time.Now()
	ph := &pm.Phases[phaseIndex]
	ph.Status = result.Status
	ph.ImplStatus = result.ImplStatus
	ph.ReviewVerdict = result.ReviewVerdict
	ph.FixStatus = result.FixStatus
	ph.PRURL = result.PRURL
	ph.Error = result.Error
	ph.Duration = int64(result.Duration)
	ph.CompletedAt = &now
	if result.PRURL != "" {
		ph.PRStatus = "created"
	}
}

// NextIncompletePhase returns the index of the first phase that is neither
// completed nor skipped. It returns -1 if all phases are done.
func (pm *PipelineMetadata) NextIncompletePhase() int {
	for i, ph := range pm.Phases {
		if ph.Status != PhaseStatusCompleted && ph.Status != PhaseStatusSkipped {
			return i
		}
	}
	return -1
}

// IsComplete returns true if every phase is in the completed or skipped state.
func (pm *PipelineMetadata) IsComplete() bool {
	return pm.NextIncompletePhase() == -1
}

// Summary returns a human-readable one-line summary of the pipeline's progress.
// Example: "Pipeline: 2/5 phases complete | Current: Phase 3 (reviewing)"
func (pm *PipelineMetadata) Summary() string {
	completed := 0
	for _, ph := range pm.Phases {
		if ph.Status == PhaseStatusCompleted || ph.Status == PhaseStatusSkipped {
			completed++
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Pipeline: %d/%d phases complete", completed, pm.TotalPhases)

	// Describe the currently active phase if one exists.
	if pm.CurrentPhase >= 0 && pm.CurrentPhase < len(pm.Phases) {
		cur := pm.Phases[pm.CurrentPhase]
		fmt.Fprintf(&sb, " | Current: Phase %d (%s)", cur.PhaseID, cur.Status)
	}

	return sb.String()
}
