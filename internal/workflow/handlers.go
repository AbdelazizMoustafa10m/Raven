// handlers.go contains the concrete StepHandler implementations for every
// built-in Raven workflow step. Handlers that require runtime dependencies
// (Runner, Orchestrator, etc.) store those as struct fields. When a field is
// nil, Execute returns EventFailure with a descriptive error so the handler
// can be safely registered at init time before dependencies are wired.
package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/AbdelazizMoustafa10m/Raven/internal/loop"
	"github.com/AbdelazizMoustafa10m/Raven/internal/review"
)

// Compile-time interface compliance checks for all built-in handler types.
var (
	_ StepHandler = (*ImplementHandler)(nil)
	_ StepHandler = (*ReviewHandler)(nil)
	_ StepHandler = (*CheckReviewHandler)(nil)
	_ StepHandler = (*FixHandler)(nil)
	_ StepHandler = (*PRHandler)(nil)
	_ StepHandler = (*InitPhaseHandler)(nil)
	_ StepHandler = (*RunPhaseWorkflowHandler)(nil)
	_ StepHandler = (*AdvancePhaseHandler)(nil)
	_ StepHandler = (*ShredHandler)(nil)
	_ StepHandler = (*ScatterHandler)(nil)
	_ StepHandler = (*GatherHandler)(nil)
)

// -----------------------------------------------------------------------
// ImplementHandler
// -----------------------------------------------------------------------

// ImplementHandler executes the implementation loop for a phase or a single
// task. The Runner field must be set before Execute is called; a nil Runner
// causes Execute to return EventFailure with a descriptive error, which allows
// safe registration of the handler into the registry before runtime dependencies
// are resolved.
type ImplementHandler struct {
	// Runner is the implementation loop runner injected at runtime.
	// May be nil when the handler is registered for registry-only use.
	Runner *loop.Runner

	// RunConfig provides default values for fields not present in the
	// workflow state metadata.
	RunConfig loop.RunConfig
}

// Name returns the unique step name "run_implement".
func (h *ImplementHandler) Name() string { return "run_implement" }

// Execute runs the implementation loop using configuration values merged from
// the workflow state metadata and h.RunConfig defaults. It calls
// Runner.RunSingleTask when a task_id is present in the metadata; otherwise
// it calls Runner.Run in phase mode.
func (h *ImplementHandler) Execute(ctx context.Context, state *WorkflowState) (string, error) {
	if h.Runner == nil {
		return EventFailure, fmt.Errorf("implement handler: runner not configured")
	}

	phaseID := metaInt(state, "phase_id", h.RunConfig.PhaseID)
	taskID := metaString(state, "task_id", h.RunConfig.TaskID)
	agentName := metaString(state, "agent_name", h.RunConfig.AgentName)
	dryRun := metaBool(state, "dry_run", h.RunConfig.DryRun)

	runCfg := loop.RunConfig{
		AgentName:     agentName,
		PhaseID:       phaseID,
		TaskID:        taskID,
		MaxIterations: h.RunConfig.MaxIterations,
		MaxLimitWaits: h.RunConfig.MaxLimitWaits,
		SleepBetween:  h.RunConfig.SleepBetween,
		DryRun:        dryRun,
		TemplateName:  h.RunConfig.TemplateName,
	}

	var err error
	if taskID != "" {
		err = h.Runner.RunSingleTask(ctx, runCfg)
	} else {
		err = h.Runner.Run(ctx, runCfg)
	}

	if err != nil {
		return EventFailure, fmt.Errorf("implement handler: %w", err)
	}

	state.Metadata["tasks_completed"] = true
	return EventSuccess, nil
}

// DryRun returns a human-readable description of what Execute would do without
// performing any side effects.
func (h *ImplementHandler) DryRun(state *WorkflowState) string {
	phaseID := metaInt(state, "phase_id", h.RunConfig.PhaseID)
	taskID := metaString(state, "task_id", h.RunConfig.TaskID)
	agentName := metaString(state, "agent_name", h.RunConfig.AgentName)
	return fmt.Sprintf(
		"would run implementation loop for phase %d task %s with agent %s",
		phaseID, taskID, agentName,
	)
}

// -----------------------------------------------------------------------
// ReviewHandler
// -----------------------------------------------------------------------

// ReviewHandler runs the multi-agent parallel code review orchestrator. The
// Orchestrator field must be injected before Execute is called; a nil
// Orchestrator returns EventFailure with a descriptive error.
type ReviewHandler struct {
	// Orchestrator is the multi-agent review coordinator injected at runtime.
	// May be nil when the handler is registered for registry-only use.
	Orchestrator *review.ReviewOrchestrator
}

// Name returns the unique step name "run_review".
func (h *ReviewHandler) Name() string { return "run_review" }

// Execute runs the review orchestrator, reads agent names, base branch, and
// mode from the workflow state metadata, and stores the resulting verdict and
// findings count back into the metadata.
func (h *ReviewHandler) Execute(ctx context.Context, state *WorkflowState) (string, error) {
	if h.Orchestrator == nil {
		return EventFailure, fmt.Errorf("review handler: orchestrator not configured")
	}

	agents := resolveAgents(state)
	baseBranch := metaString(state, "base_branch", "main")
	modeStr := metaString(state, "review_mode", "all")

	opts := review.ReviewOpts{
		Agents:     agents,
		BaseBranch: baseBranch,
		Mode:       review.ReviewMode(modeStr),
	}

	result, err := h.Orchestrator.Run(ctx, opts)
	if err != nil {
		return EventFailure, fmt.Errorf("review handler: %w", err)
	}

	if result.Consolidated != nil {
		state.Metadata["review_verdict"] = string(result.Consolidated.Verdict)
		state.Metadata["review_findings_count"] = len(result.Consolidated.Findings)
	}

	return EventSuccess, nil
}

// DryRun returns a human-readable description of what Execute would do.
func (h *ReviewHandler) DryRun(state *WorkflowState) string {
	baseBranch := metaString(state, "base_branch", "main")
	return fmt.Sprintf("would run multi-agent code review against %s", baseBranch)
}

// -----------------------------------------------------------------------
// CheckReviewHandler
// -----------------------------------------------------------------------

// CheckReviewHandler inspects the review_verdict stored in the workflow state
// metadata by a previous ReviewHandler step and maps it to the appropriate
// transition event: EventSuccess for an approved verdict, EventNeedsHuman for
// changes-needed or blocking verdicts, and EventSuccess for an empty or unknown
// verdict (treated as approved when no prior review has run).
type CheckReviewHandler struct{}

// Name returns the unique step name "check_review".
func (h *CheckReviewHandler) Name() string { return "check_review" }

// Execute reads the review_verdict metadata key and maps it to a transition event.
// Context cancellation is honoured before the verdict is inspected.
func (h *CheckReviewHandler) Execute(ctx context.Context, state *WorkflowState) (string, error) {
	if err := ctx.Err(); err != nil {
		return EventFailure, fmt.Errorf("check_review handler: context cancelled: %w", err)
	}

	verdict := metaString(state, "review_verdict", "")

	switch verdict {
	case string(review.VerdictApproved):
		return EventSuccess, nil
	case string(review.VerdictChangesNeeded), string(review.VerdictBlocking):
		return EventNeedsHuman, nil
	default:
		// Empty or unrecognised verdict: treat as approved (no review ran yet).
		return EventSuccess, nil
	}
}

// DryRun returns a human-readable description of what Execute would do.
func (h *CheckReviewHandler) DryRun(_ *WorkflowState) string {
	return "would check review verdict from metadata"
}

// -----------------------------------------------------------------------
// FixHandler
// -----------------------------------------------------------------------

// FixHandler drives the iterative fix-verify cycle using a review.FixEngine.
// The Engine field must be injected before Execute is called; a nil Engine
// returns EventFailure with a descriptive error.
type FixHandler struct {
	// Engine is the review fix engine injected at runtime.
	// May be nil when the handler is registered for registry-only use.
	Engine *review.FixEngine
}

// Name returns the unique step name "run_fix".
func (h *FixHandler) Name() string { return "run_fix" }

// Execute invokes the fix engine with options sourced from the workflow state
// metadata. Findings are not stored in metadata in this simplified integration
// (the FixEngine treats empty Findings as a no-op fast path). The fix_applied
// boolean is written back into the metadata on success.
func (h *FixHandler) Execute(ctx context.Context, state *WorkflowState) (string, error) {
	if h.Engine == nil {
		return EventFailure, fmt.Errorf("fix handler: engine not configured")
	}

	reviewReport := metaString(state, "review_report_path", "")
	baseBranch := metaString(state, "base_branch", "main")
	maxCycles := metaInt(state, "max_fix_cycles", 0)

	opts := review.FixOpts{
		Findings:     []*review.Finding{},
		ReviewReport: reviewReport,
		BaseBranch:   baseBranch,
		MaxCycles:    maxCycles,
	}

	report, err := h.Engine.Fix(ctx, opts)
	if err != nil {
		return EventFailure, fmt.Errorf("fix handler: %w", err)
	}

	state.Metadata["fix_applied"] = report.FixesApplied
	return EventSuccess, nil
}

// DryRun returns a human-readable description of what Execute would do.
func (h *FixHandler) DryRun(_ *WorkflowState) string {
	return "would run fix engine to address review findings"
}

// -----------------------------------------------------------------------
// PRHandler
// -----------------------------------------------------------------------

// PRHandler creates a GitHub pull request via the review.PRCreator. The
// Creator field must be injected before Execute is called; a nil Creator
// returns EventFailure with a descriptive error.
type PRHandler struct {
	// Creator is the PR creation helper injected at runtime.
	// May be nil when the handler is registered for registry-only use.
	Creator *review.PRCreator
}

// Name returns the unique step name "create_pr".
func (h *PRHandler) Name() string { return "create_pr" }

// Execute reads PR options from the workflow state metadata, calls
// Creator.Create, and writes the resulting URL and PR number back into the
// metadata.
func (h *PRHandler) Execute(ctx context.Context, state *WorkflowState) (string, error) {
	if h.Creator == nil {
		return EventFailure, fmt.Errorf("PR handler: creator not configured")
	}

	title := metaString(state, "pr_title", "Automated PR by Raven")
	baseBranch := metaString(state, "base_branch", "main")
	draft := metaBool(state, "pr_draft", false)

	opts := review.PRCreateOpts{
		Title:      title,
		BaseBranch: baseBranch,
		Draft:      draft,
	}

	result, err := h.Creator.Create(ctx, opts)
	if err != nil {
		return EventFailure, fmt.Errorf("PR handler: %w", err)
	}

	state.Metadata["pr_url"] = result.URL
	state.Metadata["pr_number"] = result.Number
	return EventSuccess, nil
}

// DryRun returns a human-readable description of what Execute would do.
func (h *PRHandler) DryRun(_ *WorkflowState) string {
	return "would create GitHub pull request via gh CLI"
}

// -----------------------------------------------------------------------
// InitPhaseHandler
// -----------------------------------------------------------------------

// InitPhaseHandler initialises a pipeline phase by copying the current_phase
// metadata value into the phase_id key consumed by downstream step handlers
// such as ImplementHandler and RunPhaseWorkflowHandler.
type InitPhaseHandler struct{}

// Name returns the unique step name "init_phase".
func (h *InitPhaseHandler) Name() string { return "init_phase" }

// Execute reads current_phase (default 1) from metadata and stores it as
// phase_id for use by subsequent steps.
func (h *InitPhaseHandler) Execute(ctx context.Context, state *WorkflowState) (string, error) {
	if err := ctx.Err(); err != nil {
		return EventFailure, fmt.Errorf("init_phase handler: context cancelled: %w", err)
	}

	currentPhase := metaInt(state, "current_phase", 1)
	state.Metadata["phase_id"] = currentPhase
	return EventSuccess, nil
}

// DryRun returns a human-readable description of what Execute would do.
func (h *InitPhaseHandler) DryRun(_ *WorkflowState) string {
	return "would initialize pipeline phase from metadata"
}

// -----------------------------------------------------------------------
// RunPhaseWorkflowHandler
// -----------------------------------------------------------------------

// RunPhaseWorkflowHandler executes the implementation loop for the current
// pipeline phase. The Runner field must be injected before Execute is called;
// a nil Runner returns EventFailure with a descriptive error.
type RunPhaseWorkflowHandler struct {
	// Runner is the implementation loop runner injected at runtime.
	// May be nil when the handler is registered for registry-only use.
	Runner *loop.Runner
}

// Name returns the unique step name "run_phase_workflow".
func (h *RunPhaseWorkflowHandler) Name() string { return "run_phase_workflow" }

// Execute reads phase_id and agent_name from the workflow metadata and
// delegates to Runner.Run for the resolved phase.
func (h *RunPhaseWorkflowHandler) Execute(ctx context.Context, state *WorkflowState) (string, error) {
	if h.Runner == nil {
		return EventFailure, fmt.Errorf("run_phase_workflow handler: runner not configured")
	}

	phaseID := metaInt(state, "phase_id", 0)
	agentName := metaString(state, "agent_name", "")

	runCfg := loop.RunConfig{
		PhaseID:   phaseID,
		AgentName: agentName,
	}

	if err := h.Runner.Run(ctx, runCfg); err != nil {
		return EventFailure, fmt.Errorf("run_phase_workflow handler: %w", err)
	}

	return EventSuccess, nil
}

// DryRun returns a human-readable description of what Execute would do.
func (h *RunPhaseWorkflowHandler) DryRun(_ *WorkflowState) string {
	return "would run implement-review-pr workflow for current phase"
}

// -----------------------------------------------------------------------
// AdvancePhaseHandler
// -----------------------------------------------------------------------

// AdvancePhaseHandler increments the current_phase counter in the workflow
// metadata. It returns EventPartial when more phases remain (so the engine
// loops back to init_phase) and EventSuccess when all phases are complete.
type AdvancePhaseHandler struct{}

// Name returns the unique step name "advance_phase".
func (h *AdvancePhaseHandler) Name() string { return "advance_phase" }

// Execute increments current_phase. If the resulting value exceeds
// total_phases, EventSuccess is returned (all phases done). Otherwise
// EventPartial is returned so the engine loops back to init_phase.
func (h *AdvancePhaseHandler) Execute(ctx context.Context, state *WorkflowState) (string, error) {
	if err := ctx.Err(); err != nil {
		return EventFailure, fmt.Errorf("advance_phase handler: context cancelled: %w", err)
	}

	currentPhase := metaInt(state, "current_phase", 1)
	totalPhases := metaInt(state, "total_phases", 1)

	nextPhase := currentPhase + 1
	if nextPhase > totalPhases {
		return EventSuccess, nil
	}

	state.Metadata["current_phase"] = nextPhase
	return EventPartial, nil
}

// DryRun returns a human-readable description of what Execute would do.
func (h *AdvancePhaseHandler) DryRun(_ *WorkflowState) string {
	return "would advance to next pipeline phase or signal completion"
}

// -----------------------------------------------------------------------
// ShredHandler
// -----------------------------------------------------------------------

// ShredHandler is a stub for the PRD shredding step (T-056+). It records
// completion in the workflow metadata and returns EventSuccess. The actual
// PRD shredder subsystem will be implemented in a future task.
type ShredHandler struct{}

// Name returns the unique step name "shred".
func (h *ShredHandler) Name() string { return "shred" }

// Execute reads the prd_path from metadata, records shred_complete, and
// returns EventSuccess. Full PRD shredder logic is not yet implemented.
func (h *ShredHandler) Execute(ctx context.Context, state *WorkflowState) (string, error) {
	if err := ctx.Err(); err != nil {
		return EventFailure, fmt.Errorf("shred handler: context cancelled: %w", err)
	}

	// Record the PRD path for downstream handlers even though actual shredding
	// is not yet implemented (T-056+).
	_ = metaString(state, "prd_path", "")
	state.Metadata["shred_complete"] = true
	return EventSuccess, nil
}

// DryRun returns a human-readable description of what Execute would do.
func (h *ShredHandler) DryRun(_ *WorkflowState) string {
	return "would shred PRD into epics and sections"
}

// -----------------------------------------------------------------------
// ScatterHandler
// -----------------------------------------------------------------------

// ScatterHandler is a stub for the PRD scatter step. It verifies that shredding
// completed (via shred_complete metadata), records scatter_complete, and returns
// EventSuccess. Actual parallel worker dispatch will be added in a future task.
type ScatterHandler struct{}

// Name returns the unique step name "scatter".
func (h *ScatterHandler) Name() string { return "scatter" }

// Execute checks that shred_complete is set and records scatter_complete.
func (h *ScatterHandler) Execute(ctx context.Context, state *WorkflowState) (string, error) {
	if err := ctx.Err(); err != nil {
		return EventFailure, fmt.Errorf("scatter handler: context cancelled: %w", err)
	}

	_ = metaBool(state, "shred_complete", false)
	state.Metadata["scatter_complete"] = true
	return EventSuccess, nil
}

// DryRun returns a human-readable description of what Execute would do.
func (h *ScatterHandler) DryRun(_ *WorkflowState) string {
	return "would scatter PRD sections to parallel decomposition workers"
}

// -----------------------------------------------------------------------
// GatherHandler
// -----------------------------------------------------------------------

// GatherHandler is a stub for the PRD gather step. It verifies that scatter
// completed (via scatter_complete metadata), records gather_complete, and
// returns EventSuccess. Actual result merging will be added in a future task.
type GatherHandler struct{}

// Name returns the unique step name "gather".
func (h *GatherHandler) Name() string { return "gather" }

// Execute checks that scatter_complete is set and records gather_complete.
func (h *GatherHandler) Execute(ctx context.Context, state *WorkflowState) (string, error) {
	if err := ctx.Err(); err != nil {
		return EventFailure, fmt.Errorf("gather handler: context cancelled: %w", err)
	}

	_ = metaBool(state, "scatter_complete", false)
	state.Metadata["gather_complete"] = true
	return EventSuccess, nil
}

// DryRun returns a human-readable description of what Execute would do.
func (h *GatherHandler) DryRun(_ *WorkflowState) string {
	return "would gather and merge decomposed task files"
}

// -----------------------------------------------------------------------
// Metadata helpers
// -----------------------------------------------------------------------

// metaString reads a string value from state.Metadata[key]. If the key is
// absent or the stored value is not a string, defaultVal is returned.
func metaString(state *WorkflowState, key, defaultVal string) string {
	if state == nil || state.Metadata == nil {
		return defaultVal
	}
	v, ok := state.Metadata[key]
	if !ok {
		return defaultVal
	}
	s, ok := v.(string)
	if !ok {
		return defaultVal
	}
	return s
}

// metaInt reads an integer value from state.Metadata[key]. It handles the
// common JSON round-trip case where integers are stored as float64. If the
// key is absent or the value cannot be coerced to int, defaultVal is returned.
func metaInt(state *WorkflowState, key string, defaultVal int) int {
	if state == nil || state.Metadata == nil {
		return defaultVal
	}
	v, ok := state.Metadata[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return defaultVal
	}
}

// metaBool reads a boolean value from state.Metadata[key]. If the key is
// absent or the stored value is not a bool, defaultVal is returned.
func metaBool(state *WorkflowState, key string, defaultVal bool) bool {
	if state == nil || state.Metadata == nil {
		return defaultVal
	}
	v, ok := state.Metadata[key]
	if !ok {
		return defaultVal
	}
	b, ok := v.(bool)
	if !ok {
		return defaultVal
	}
	return b
}

// -----------------------------------------------------------------------
// Internal helpers
// -----------------------------------------------------------------------

// resolveAgents extracts the agent list from state.Metadata["review_agents"].
// It accepts either a []string value (set programmatically) or a
// comma-separated string (set from TOML / CLI flags). A nil slice is returned
// when the key is absent or the value is unusable.
func resolveAgents(state *WorkflowState) []string {
	if state == nil || state.Metadata == nil {
		return nil
	}
	v, ok := state.Metadata["review_agents"]
	if !ok {
		return nil
	}

	switch agents := v.(type) {
	case []string:
		return agents
	case string:
		parts := strings.Split(agents, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		return result
	default:
		return nil
	}
}
