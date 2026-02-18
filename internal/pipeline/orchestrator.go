package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/log"

	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
	"github.com/AbdelazizMoustafa10m/Raven/internal/git"
	"github.com/AbdelazizMoustafa10m/Raven/internal/task"
	"github.com/AbdelazizMoustafa10m/Raven/internal/workflow"
)

// knownAgents is the set of recognised AI agent names. Agent names not in this
// set are replaced with the default "claude" agent to avoid silent failures.
var knownAgents = map[string]bool{
	"claude": true,
	"codex":  true,
	"gemini": true,
}

// defaultAgent is the fallback agent name used when an unrecognised agent is
// specified in PipelineOpts.
const defaultAgent = "claude"

// PipelineOpts configures a single pipeline run. Zero values apply sensible
// defaults (run all phases, use claude agent, no skip flags).
type PipelineOpts struct {
	// PhaseID is a single phase to run. Use "all" or "" to run every phase.
	// A numeric string (e.g. "1") restricts execution to that phase only.
	PhaseID string

	// FromPhase restricts execution to phases whose ID >= FromPhase (numeric
	// string). Ignored when PhaseID is a specific phase.
	FromPhase string

	// SkipImplement bypasses the run_implement step, jumping straight to review.
	SkipImplement bool

	// SkipReview bypasses run_review and check_review, jumping to create_pr.
	SkipReview bool

	// SkipFix bypasses run_fix; when review needs fixes the pipeline goes
	// directly to create_pr instead.
	SkipFix bool

	// SkipPR bypasses create_pr; the pipeline completes after review/fix.
	SkipPR bool

	// ImplAgent names the agent to use for the implementation step.
	ImplAgent string

	// ReviewAgent names the agent to use for review steps.
	ReviewAgent string

	// FixAgent names the agent to use for the fix step.
	FixAgent string

	// ReviewConcurrency is the maximum number of concurrent review agents.
	ReviewConcurrency int

	// MaxReviewCycles caps how many review-fix iterations may occur.
	MaxReviewCycles int

	// DryRun when true causes the orchestrator to describe what would happen
	// without making any changes.
	DryRun bool

	// Interactive enables interactive mode (user confirmations at phase
	// boundaries). Reserved for future use.
	Interactive bool
}

// Phase status constants describe the lifecycle status of a single phase within
// a pipeline run.
const (
	PhaseStatusPending      = "pending"
	PhaseStatusImplementing = "implementing"
	PhaseStatusReviewing    = "reviewing"
	PhaseStatusFixing       = "fixing"
	PhaseStatusPRCreated    = "pr_created"
	PhaseStatusCompleted    = "completed"
	PhaseStatusFailed       = "failed"
	PhaseStatusSkipped      = "skipped"
)

// Pipeline status constants describe the overall outcome of a pipeline run.
const (
	PipelineStatusCompleted = "completed"
	PipelineStatusPartial   = "partial"
	PipelineStatusFailed    = "failed"
)

// PhaseResult captures the outcome of a single phase execution within a
// pipeline run. It is persisted as part of the pipeline checkpoint.
type PhaseResult struct {
	PhaseID       string        `json:"phase_id"`
	PhaseName     string        `json:"phase_name"`
	Status        string        `json:"status"`
	ImplStatus    string        `json:"impl_status"`
	ReviewVerdict string        `json:"review_verdict"`
	FixStatus     string        `json:"fix_status"`
	PRURL         string        `json:"pr_url"`
	BranchName    string        `json:"branch_name"`
	Duration      time.Duration `json:"duration"`
	Error         string        `json:"error,omitempty"`
}

// PipelineResult is the final outcome of a complete pipeline run, containing
// per-phase results and aggregate status.
type PipelineResult struct {
	Phases        []PhaseResult `json:"phases"`
	TotalDuration time.Duration `json:"total_duration"`
	Status        string        `json:"status"`
}

// PipelineOrchestrator chains the implement -> review -> fix -> PR lifecycle
// across multiple phases using the workflow engine for per-phase execution.
type PipelineOrchestrator struct {
	engine    *workflow.Engine
	store     *workflow.StateStore
	gitClient *git.GitClient
	config    *config.Config
	logger    *log.Logger
	events    chan<- workflow.WorkflowEvent
}

// PipelineOption is a functional option for configuring a PipelineOrchestrator.
type PipelineOption func(*PipelineOrchestrator)

// WithPipelineLogger attaches a charmbracelet/log Logger to the orchestrator.
func WithPipelineLogger(logger *log.Logger) PipelineOption {
	return func(p *PipelineOrchestrator) {
		p.logger = logger
	}
}

// WithPipelineEvents sets the channel on which the orchestrator forwards
// WorkflowEvents emitted by the per-phase workflow engine.
func WithPipelineEvents(ch chan<- workflow.WorkflowEvent) PipelineOption {
	return func(p *PipelineOrchestrator) {
		p.events = ch
	}
}

// NewPipelineOrchestrator constructs a PipelineOrchestrator with the given
// engine, state store, git client, and configuration. Functional options may
// be supplied to attach a logger or event channel.
func NewPipelineOrchestrator(
	engine *workflow.Engine,
	store *workflow.StateStore,
	gitClient *git.GitClient,
	cfg *config.Config,
	opts ...PipelineOption,
) *PipelineOrchestrator {
	p := &PipelineOrchestrator{
		engine:    engine,
		store:     store,
		gitClient: gitClient,
		config:    cfg,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Run executes the pipeline for the phases described by opts. It returns a
// PipelineResult containing per-phase outcomes and an aggregate status.
//
// If a previous pipeline run is found in the state store and has not completed,
// execution resumes from the last checkpointed phase index.
//
// Context cancellation is checked between phases; partial results are returned
// if the context is cancelled mid-run.
func (p *PipelineOrchestrator) Run(ctx context.Context, opts PipelineOpts) (*PipelineResult, error) {
	start := time.Now()

	phases, err := p.resolvePhases(opts)
	if err != nil {
		return nil, fmt.Errorf("pipeline orchestrator: resolve phases: %w", err)
	}

	// Normalise agent names -- fall back to default for unknown values.
	opts.ImplAgent = normalizeAgent(opts.ImplAgent)
	opts.ReviewAgent = normalizeAgent(opts.ReviewAgent)
	opts.FixAgent = normalizeAgent(opts.FixAgent)

	// Determine pipeline run ID and check for a resumable checkpoint.
	pipelineRunID, startIdx, existingResults, loadErr := p.loadOrCreatePipelineState(opts)
	if loadErr != nil {
		p.logMsg("warn: could not load existing pipeline state, starting fresh: " + loadErr.Error())
		pipelineRunID = fmt.Sprintf("pipeline-%d", time.Now().UnixNano())
		startIdx = 0
		existingResults = nil
	}

	results := make([]PhaseResult, len(phases))
	// Seed with any previously completed phase results.
	for i, r := range existingResults {
		if i < len(results) {
			results[i] = r
		}
	}

	successCount := 0
	failureCount := 0

	for i := startIdx; i < len(phases); i++ {
		ph := phases[i]

		// Honour context cancellation between phases.
		if ctxErr := ctx.Err(); ctxErr != nil {
			p.log("pipeline cancelled between phases",
				"phase", ph.ID,
				"context_error", ctxErr,
			)
			// Mark remaining phases as pending.
			for j := i; j < len(phases); j++ {
				results[j] = PhaseResult{
					PhaseID:   strconv.Itoa(phases[j].ID),
					PhaseName: phases[j].Name,
					Status:    PhaseStatusPending,
				}
			}
			return p.buildResult(results, start, successCount, failureCount), ctxErr
		}

		p.log("starting phase", "phase_id", ph.ID, "phase_name", ph.Name)

		pr, phaseErr := p.runPhase(ctx, ph, opts)
		results[i] = pr

		if phaseErr != nil {
			failureCount++
			p.log("phase failed", "phase_id", ph.ID, "error", phaseErr)
		} else {
			successCount++
			p.log("phase completed", "phase_id", ph.ID)
		}

		// Checkpoint pipeline state after each phase.
		if p.store != nil {
			if saveErr := p.savePipelineState(pipelineRunID, opts, results, i+1); saveErr != nil {
				p.log("warn: failed to save pipeline checkpoint", "error", saveErr)
			}
		}
	}

	return p.buildResult(results, start, successCount, failureCount), nil
}

// DryRun describes the planned pipeline actions without performing them. It
// returns a formatted string listing all phases, branch names, and the workflow
// steps that would be executed for each phase given the supplied opts.
func (p *PipelineOrchestrator) DryRun(opts PipelineOpts) string {
	phases, err := p.resolvePhases(opts)
	if err != nil {
		return fmt.Sprintf("pipeline dry-run: cannot resolve phases: %v", err)
	}

	opts.ImplAgent = normalizeAgent(opts.ImplAgent)
	opts.ReviewAgent = normalizeAgent(opts.ReviewAgent)
	opts.FixAgent = normalizeAgent(opts.FixAgent)

	var sb strings.Builder
	sb.WriteString("Pipeline dry-run plan\n")
	sb.WriteString(strings.Repeat("=", 40))
	sb.WriteString("\n\n")

	steps := activeSteps(opts)

	for _, ph := range phases {
		branchName := phaseBranchName(strconv.Itoa(ph.ID))
		fmt.Fprintf(&sb, "Phase %d: %s\n", ph.ID, ph.Name)
		fmt.Fprintf(&sb, "  Branch:       %s\n", branchName)
		fmt.Fprintf(&sb, "  Tasks:        %s - %s\n", ph.StartTask, ph.EndTask)
		fmt.Fprintf(&sb, "  Steps:        %s\n", strings.Join(steps, " -> "))
		fmt.Fprintf(&sb, "  Impl agent:   %s\n", opts.ImplAgent)
		if !opts.SkipReview {
			fmt.Fprintf(&sb, "  Review agent: %s\n", opts.ReviewAgent)
		}
		if !opts.SkipFix {
			fmt.Fprintf(&sb, "  Fix agent:    %s\n", opts.FixAgent)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// resolvePhases loads phases from configuration and applies PhaseID / FromPhase
// filters from opts. It returns an error if the specified phase does not exist.
func (p *PipelineOrchestrator) resolvePhases(opts PipelineOpts) ([]task.Phase, error) {
	phasesPath := p.config.Project.PhasesConf
	if phasesPath == "" {
		return nil, fmt.Errorf("pipeline orchestrator: project.phases_conf is not configured")
	}

	phases, err := task.LoadPhases(phasesPath)
	if err != nil {
		return nil, fmt.Errorf("pipeline orchestrator: load phases: %w", err)
	}

	// "all" or empty string: return every phase, optionally filtered by FromPhase.
	if opts.PhaseID == "" || strings.EqualFold(opts.PhaseID, "all") {
		if opts.FromPhase != "" {
			return filterFromPhase(phases, opts.FromPhase)
		}
		return phases, nil
	}

	// Specific numeric phase ID.
	id, err := strconv.Atoi(opts.PhaseID)
	if err != nil {
		return nil, fmt.Errorf("pipeline orchestrator: invalid phase ID %q: %w", opts.PhaseID, err)
	}
	ph := task.PhaseByID(phases, id)
	if ph == nil {
		return nil, fmt.Errorf("pipeline orchestrator: phase %d not found", id)
	}
	return []task.Phase{*ph}, nil
}

// filterFromPhase returns phases whose numeric ID is >= the fromPhase value.
func filterFromPhase(phases []task.Phase, fromPhase string) ([]task.Phase, error) {
	from, err := strconv.Atoi(fromPhase)
	if err != nil {
		return nil, fmt.Errorf("pipeline orchestrator: invalid from-phase %q: %w", fromPhase, err)
	}
	var out []task.Phase
	for _, ph := range phases {
		if ph.ID >= from {
			out = append(out, ph)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("pipeline orchestrator: no phases found with ID >= %d", from)
	}
	return out, nil
}

// runPhase executes the implement-review-pr workflow for a single phase. It
// creates a unique run ID, sets the git branch, builds a modified workflow
// definition based on skip flags, runs the engine, and extracts a PhaseResult
// from the final workflow state.
func (p *PipelineOrchestrator) runPhase(ctx context.Context, ph task.Phase, opts PipelineOpts) (PhaseResult, error) {
	phaseID := strconv.Itoa(ph.ID)
	branchName := phaseBranchName(phaseID)
	ts := time.Now().Format("20060102150405")
	runID := fmt.Sprintf("pipeline-%s-phase-%s", ts, phaseID)

	result := PhaseResult{
		PhaseID:    phaseID,
		PhaseName:  ph.Name,
		Status:     PhaseStatusPending,
		BranchName: branchName,
	}

	phaseStart := time.Now()

	// Manage the git branch (skip when gitClient is nil, e.g. in tests).
	if p.gitClient != nil {
		if err := p.ensureBranch(ctx, branchName); err != nil {
			result.Status = PhaseStatusFailed
			result.Error = err.Error()
			result.Duration = time.Since(phaseStart)
			return result, fmt.Errorf("pipeline orchestrator: phase %s: branch setup: %w", phaseID, err)
		}
	}

	// Get the base workflow definition and build a modified copy.
	// GetDefinition returns a pointer into the built-in map -- never mutate it.
	baseDef := workflow.GetDefinition(workflow.WorkflowImplementReview)
	if baseDef == nil {
		err := fmt.Errorf("pipeline orchestrator: built-in workflow %q not found", workflow.WorkflowImplementReview)
		result.Status = PhaseStatusFailed
		result.Error = err.Error()
		result.Duration = time.Since(phaseStart)
		return result, err
	}

	def := applySkipFlags(baseDef, opts)

	// If every step has been skipped and the initial step is a terminal
	// pseudo-step, skip the engine entirely and return a completed result.
	if def.InitialStep == workflow.StepDone || def.InitialStep == workflow.StepFailed || len(def.Steps) == 0 {
		result.Status = PhaseStatusCompleted
		result.Duration = time.Since(phaseStart)
		return result, nil
	}

	// Build the initial workflow state with per-phase metadata.
	state := workflow.NewWorkflowState(runID, def.Name, def.InitialStep)
	state.Metadata["phase_id"] = ph.ID
	state.Metadata["agent_name"] = opts.ImplAgent
	state.Metadata["review_agents"] = opts.ReviewAgent
	state.Metadata["fix_agent"] = opts.FixAgent
	state.Metadata["review_concurrency"] = opts.ReviewConcurrency
	state.Metadata["max_fix_cycles"] = opts.MaxReviewCycles
	state.Metadata["base_branch"] = "main"
	state.Metadata["branch_name"] = branchName
	state.Metadata["phase_name"] = ph.Name
	state.Metadata["start_task"] = ph.StartTask
	state.Metadata["end_task"] = ph.EndTask

	result.Status = PhaseStatusImplementing

	finalState, runErr := p.engine.Run(ctx, def, state)

	result.Duration = time.Since(phaseStart)

	// Extract outcome metadata from the final state.
	if finalState != nil {
		result = extractPhaseResult(result, finalState)
	}

	if runErr != nil {
		result.Status = PhaseStatusFailed
		if result.Error == "" {
			result.Error = runErr.Error()
		}
		return result, fmt.Errorf("pipeline orchestrator: phase %s: workflow run: %w", phaseID, runErr)
	}

	result.Status = PhaseStatusCompleted
	return result, nil
}

// ensureBranch creates the named branch if it does not already exist, or
// checks it out if it does.
func (p *PipelineOrchestrator) ensureBranch(ctx context.Context, branchName string) error {
	exists, err := p.gitClient.BranchExists(ctx, branchName)
	if err != nil {
		return fmt.Errorf("checking branch %q: %w", branchName, err)
	}
	if exists {
		return p.gitClient.Checkout(ctx, branchName)
	}
	return p.gitClient.CreateBranch(ctx, branchName, "")
}

// applySkipFlags returns a deep copy of def with steps removed and transitions
// rewired according to the skip flags in opts. The original definition is never
// modified.
func applySkipFlags(def *workflow.WorkflowDefinition, opts PipelineOpts) *workflow.WorkflowDefinition {
	// Deep-copy the step slice so we never mutate the original.
	steps := make([]workflow.StepDefinition, len(def.Steps))
	for i, s := range def.Steps {
		trans := make(map[string]string, len(s.Transitions))
		for k, v := range s.Transitions {
			trans[k] = v
		}
		steps[i] = workflow.StepDefinition{
			Name:        s.Name,
			Transitions: trans,
			Parallel:    s.Parallel,
		}
	}

	modified := &workflow.WorkflowDefinition{
		Name:        def.Name,
		Description: def.Description,
		InitialStep: def.InitialStep,
		Steps:       steps,
	}

	// removeStep removes the step with the given name and returns true when found.
	removeStep := func(name string) bool {
		for i, s := range modified.Steps {
			if s.Name == name {
				modified.Steps = append(modified.Steps[:i], modified.Steps[i+1:]...)
				return true
			}
		}
		return false
	}

	// rewireTransitions replaces every transition that targets oldTarget with
	// newTarget across all steps in the modified definition.
	rewireTransitions := func(oldTarget, newTarget string) {
		for i := range modified.Steps {
			for ev, tgt := range modified.Steps[i].Transitions {
				if tgt == oldTarget {
					modified.Steps[i].Transitions[ev] = newTarget
				}
			}
		}
	}

	// rewireStep rewrites the specific event transition for a named step.
	rewireStep := func(stepName, event, newTarget string) {
		for i := range modified.Steps {
			if modified.Steps[i].Name == stepName {
				modified.Steps[i].Transitions[event] = newTarget
				return
			}
		}
	}

	// SkipImplement: remove run_implement, promote run_review as initial step.
	if opts.SkipImplement {
		removeStep("run_implement")
		modified.InitialStep = "run_review"
	}

	// SkipReview: remove run_review, check_review, and run_fix (nothing to check).
	// Wire the preceding step directly to create_pr on success.
	if opts.SkipReview {
		removeStep("run_review")
		removeStep("check_review")
		removeStep("run_fix")
		if opts.SkipImplement {
			modified.InitialStep = "create_pr"
		} else {
			rewireStep("run_implement", workflow.EventSuccess, "create_pr")
		}
	}

	// SkipFix (only meaningful when review is not also skipped): remove run_fix
	// and wire check_review's needs_human transition directly to create_pr.
	if opts.SkipFix && !opts.SkipReview {
		removeStep("run_fix")
		rewireStep("check_review", workflow.EventNeedsHuman, "create_pr")
	}

	// SkipPR: remove create_pr; redirect any transition that targeted it to
	// StepDone so the workflow ends cleanly.
	if opts.SkipPR {
		removeStep("create_pr")
		rewireTransitions("create_pr", workflow.StepDone)
		if modified.InitialStep == "create_pr" {
			modified.InitialStep = workflow.StepDone
		}
	}

	return modified
}

// extractPhaseResult reads well-known metadata keys from state and populates
// additional PhaseResult fields. Pre-existing fields (e.g. Status, BranchName)
// are not overwritten.
func extractPhaseResult(result PhaseResult, state *workflow.WorkflowState) PhaseResult {
	if v, ok := state.Metadata["impl_status"]; ok {
		result.ImplStatus = fmt.Sprintf("%v", v)
	}
	if v, ok := state.Metadata["review_verdict"]; ok {
		result.ReviewVerdict = fmt.Sprintf("%v", v)
	}
	if v, ok := state.Metadata["fix_status"]; ok {
		result.FixStatus = fmt.Sprintf("%v", v)
	}
	if v, ok := state.Metadata["pr_url"]; ok {
		result.PRURL = fmt.Sprintf("%v", v)
	}
	if v, ok := state.Metadata["error"]; ok && result.Error == "" {
		result.Error = fmt.Sprintf("%v", v)
	}
	return result
}

// buildResult assembles the final PipelineResult from per-phase outcomes and
// elapsed time.
func (p *PipelineOrchestrator) buildResult(
	results []PhaseResult,
	start time.Time,
	successCount, failureCount int,
) *PipelineResult {
	status := PipelineStatusCompleted
	switch {
	case failureCount > 0 && successCount == 0:
		status = PipelineStatusFailed
	case failureCount > 0:
		status = PipelineStatusPartial
	}
	return &PipelineResult{
		Phases:        results,
		TotalDuration: time.Since(start),
		Status:        status,
	}
}

// savePipelineState persists the current pipeline run progress as a
// WorkflowState checkpoint so execution can be resumed after interruption.
func (p *PipelineOrchestrator) savePipelineState(
	runID string,
	opts PipelineOpts,
	results []PhaseResult,
	currentIdx int,
) error {
	if p.store == nil {
		return nil
	}

	// Serialise phase results through JSON so they survive the round-trip to
	// map[string]any (JSON numbers decode as float64).
	phasesJSON, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("marshal pipeline phases: %w", err)
	}
	var phasesAny []any
	if err := json.Unmarshal(phasesJSON, &phasesAny); err != nil {
		return fmt.Errorf("unmarshal pipeline phases: %w", err)
	}

	optsJSON, err := json.Marshal(opts)
	if err != nil {
		return fmt.Errorf("marshal pipeline opts: %w", err)
	}
	var optsAny map[string]any
	if err := json.Unmarshal(optsJSON, &optsAny); err != nil {
		return fmt.Errorf("unmarshal pipeline opts: %w", err)
	}

	state := workflow.NewWorkflowState(runID, workflow.WorkflowPipeline, "run_phase_workflow")
	state.Metadata["pipeline_phases"] = phasesAny
	state.Metadata["current_phase_index"] = currentIdx
	state.Metadata["pipeline_opts"] = optsAny

	return p.store.Save(state)
}

// loadOrCreatePipelineState looks for an existing incomplete pipeline run in
// the state store. If one is found it returns its run ID, the resume index, and
// any previously completed PhaseResults. Otherwise it returns a fresh run ID
// with index 0 and nil results.
func (p *PipelineOrchestrator) loadOrCreatePipelineState(_ PipelineOpts) (
	runID string,
	startIdx int,
	existingResults []PhaseResult,
	err error,
) {
	runID = fmt.Sprintf("pipeline-%d", time.Now().UnixNano())

	if p.store == nil {
		return runID, 0, nil, nil
	}

	latest, latestErr := p.store.LatestRun()
	if latestErr != nil || latest == nil {
		return runID, 0, nil, latestErr
	}

	// Only resume a pipeline-type workflow that has not yet completed or failed.
	if latest.WorkflowName != workflow.WorkflowPipeline {
		return runID, 0, nil, nil
	}
	status := workflow.StatusFromState(latest)
	if status == "completed" || status == "failed" {
		return runID, 0, nil, nil
	}

	// Extract the resume index.  JSON numbers unmarshal as float64.
	if raw, ok := latest.Metadata["current_phase_index"]; ok {
		if f, ok2 := raw.(float64); ok2 {
			startIdx = int(f)
		}
	}

	// Extract previously completed phase results.
	if raw, ok := latest.Metadata["pipeline_phases"]; ok {
		phasesJSON, jsonErr := json.Marshal(raw)
		if jsonErr == nil {
			_ = json.Unmarshal(phasesJSON, &existingResults)
		}
	}

	p.log("resuming pipeline", "run_id", latest.ID, "from_phase_index", startIdx)
	return latest.ID, startIdx, existingResults, nil
}

// phaseBranchName returns the git branch name for the given phase ID string.
func phaseBranchName(phaseID string) string {
	return fmt.Sprintf("raven/phase-%s", phaseID)
}

// normalizeAgent returns agentName if it is a known agent; otherwise it returns
// the defaultAgent constant.
func normalizeAgent(agentName string) string {
	if agentName == "" || !knownAgents[agentName] {
		return defaultAgent
	}
	return agentName
}

// activeSteps returns the ordered list of workflow step names that will be
// executed given the skip flags in opts. Used for dry-run output.
func activeSteps(opts PipelineOpts) []string {
	all := []string{"run_implement", "run_review", "check_review", "run_fix", "create_pr"}
	remove := map[string]bool{}

	if opts.SkipImplement {
		remove["run_implement"] = true
	}
	if opts.SkipReview {
		remove["run_review"] = true
		remove["check_review"] = true
		remove["run_fix"] = true
	}
	if opts.SkipFix && !opts.SkipReview {
		remove["run_fix"] = true
	}
	if opts.SkipPR {
		remove["create_pr"] = true
	}

	var out []string
	for _, s := range all {
		if !remove[s] {
			out = append(out, s)
		}
	}
	return out
}

// log writes a structured log message when a logger is attached.
func (p *PipelineOrchestrator) log(msg string, kvs ...any) {
	if p.logger == nil {
		return
	}
	p.logger.Info(msg, kvs...)
}

// logMsg writes a plain message when a logger is attached.
func (p *PipelineOrchestrator) logMsg(msg string) {
	if p.logger == nil {
		return
	}
	p.logger.Info(msg)
}
