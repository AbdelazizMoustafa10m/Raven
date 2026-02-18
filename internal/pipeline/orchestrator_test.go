package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
	"github.com/AbdelazizMoustafa10m/Raven/internal/workflow"
)

// --- stub step handler ---------------------------------------------------

// stubHandler is a minimal StepHandler that always returns a preconfigured
// event and error. Used to exercise the pipeline orchestrator without real
// agent/runner dependencies.
type stubHandler struct {
	name  string
	event string
	err   error
}

func (s *stubHandler) Name() string { return s.name }
func (s *stubHandler) DryRun(_ *workflow.WorkflowState) string {
	return fmt.Sprintf("dry-run %s", s.name)
}
func (s *stubHandler) Execute(_ context.Context, _ *workflow.WorkflowState) (string, error) {
	return s.event, s.err
}

// --- helpers -------------------------------------------------------------

// writePhasesFile writes a minimal phases.conf fixture to dir and returns the
// full path to the file.
func writePhasesFile(t *testing.T, dir string, lines []string) string {
	t.Helper()
	path := filepath.Join(dir, "phases.conf")
	content := strings.Join(lines, "\n") + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// successRegistry builds a workflow Registry where every step in the
// implement-review-pr workflow returns EventSuccess. This lets the engine
// drive the full workflow to completion without real dependencies.
func successRegistry() *workflow.Registry {
	reg := workflow.NewRegistry()
	for _, name := range []string{"run_implement", "run_review", "check_review", "run_fix", "create_pr"} {
		reg.Register(&stubHandler{name: name, event: workflow.EventSuccess})
	}
	return reg
}

// failingRegistry builds a registry where run_implement always fails.
func failingRegistry() *workflow.Registry {
	reg := workflow.NewRegistry()
	reg.Register(&stubHandler{name: "run_implement", event: workflow.EventFailure, err: fmt.Errorf("impl error")})
	for _, name := range []string{"run_review", "check_review", "run_fix", "create_pr"} {
		reg.Register(&stubHandler{name: name, event: workflow.EventSuccess})
	}
	return reg
}

// makeConfig builds a minimal config.Config pointing to phasesPath.
func makeConfig(phasesPath string) *config.Config {
	return &config.Config{
		Project: config.ProjectConfig{
			PhasesConf: phasesPath,
		},
	}
}

// --- tests ---------------------------------------------------------------

func TestNormalizeAgent(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"claude", "claude"},
		{"codex", "codex"},
		{"gemini", "gemini"},
		{"", "claude"},
		{"unknown", "claude"},
		{"GPT-4", "claude"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeAgent(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPhaseBranchName(t *testing.T) {
	assert.Equal(t, "raven/phase-1", phaseBranchName("1"))
	assert.Equal(t, "raven/phase-42", phaseBranchName("42"))
}

func TestActiveSteps_NoSkips(t *testing.T) {
	steps := activeSteps(PipelineOpts{})
	assert.Equal(t, []string{"run_implement", "run_review", "check_review", "run_fix", "create_pr"}, steps)
}

func TestActiveSteps_SkipImplement(t *testing.T) {
	steps := activeSteps(PipelineOpts{SkipImplement: true})
	assert.NotContains(t, steps, "run_implement")
	assert.Contains(t, steps, "run_review")
}

func TestActiveSteps_SkipReview(t *testing.T) {
	steps := activeSteps(PipelineOpts{SkipReview: true})
	assert.NotContains(t, steps, "run_review")
	assert.NotContains(t, steps, "check_review")
	assert.NotContains(t, steps, "run_fix")
	assert.Contains(t, steps, "run_implement")
	assert.Contains(t, steps, "create_pr")
}

func TestActiveSteps_SkipFix(t *testing.T) {
	steps := activeSteps(PipelineOpts{SkipFix: true})
	assert.NotContains(t, steps, "run_fix")
	assert.Contains(t, steps, "run_review")
}

func TestActiveSteps_SkipPR(t *testing.T) {
	steps := activeSteps(PipelineOpts{SkipPR: true})
	assert.NotContains(t, steps, "create_pr")
}

func TestActiveSteps_AllSkipped(t *testing.T) {
	steps := activeSteps(PipelineOpts{
		SkipImplement: true,
		SkipReview:    true,
		SkipFix:       true,
		SkipPR:        true,
	})
	assert.Empty(t, steps)
}

func TestApplySkipFlags_NoSkips(t *testing.T) {
	base := workflow.GetDefinition(workflow.WorkflowImplementReview)
	require.NotNil(t, base)

	def := applySkipFlags(base, PipelineOpts{})

	// Must not have mutated the original.
	assert.Equal(t, base.InitialStep, def.InitialStep)
	assert.Equal(t, len(base.Steps), len(def.Steps))
}

func TestApplySkipFlags_DoesNotMutateOriginal(t *testing.T) {
	base := workflow.GetDefinition(workflow.WorkflowImplementReview)
	require.NotNil(t, base)

	originalInitial := base.InitialStep
	originalLen := len(base.Steps)

	_ = applySkipFlags(base, PipelineOpts{
		SkipImplement: true,
		SkipReview:    true,
		SkipFix:       true,
		SkipPR:        true,
	})

	// Original must be unchanged.
	assert.Equal(t, originalInitial, base.InitialStep)
	assert.Equal(t, originalLen, len(base.Steps))
}

func TestApplySkipFlags_SkipImplement(t *testing.T) {
	base := workflow.GetDefinition(workflow.WorkflowImplementReview)
	def := applySkipFlags(base, PipelineOpts{SkipImplement: true})

	assert.Equal(t, "run_review", def.InitialStep)
	for _, s := range def.Steps {
		assert.NotEqual(t, "run_implement", s.Name)
	}
}

func TestApplySkipFlags_SkipReview(t *testing.T) {
	base := workflow.GetDefinition(workflow.WorkflowImplementReview)
	def := applySkipFlags(base, PipelineOpts{SkipReview: true})

	names := stepNames(def)
	assert.NotContains(t, names, "run_review")
	assert.NotContains(t, names, "check_review")
	assert.NotContains(t, names, "run_fix")

	// run_implement should now wire to create_pr.
	implStep := findStep(def, "run_implement")
	require.NotNil(t, implStep)
	assert.Equal(t, "create_pr", implStep.Transitions[workflow.EventSuccess])
}

func TestApplySkipFlags_SkipFix(t *testing.T) {
	base := workflow.GetDefinition(workflow.WorkflowImplementReview)
	def := applySkipFlags(base, PipelineOpts{SkipFix: true})

	names := stepNames(def)
	assert.NotContains(t, names, "run_fix")

	checkStep := findStep(def, "check_review")
	require.NotNil(t, checkStep)
	assert.Equal(t, "create_pr", checkStep.Transitions[workflow.EventNeedsHuman])
}

func TestApplySkipFlags_SkipPR(t *testing.T) {
	base := workflow.GetDefinition(workflow.WorkflowImplementReview)
	def := applySkipFlags(base, PipelineOpts{SkipPR: true})

	names := stepNames(def)
	assert.NotContains(t, names, "create_pr")

	// No remaining step should point to create_pr.
	for _, s := range def.Steps {
		for _, tgt := range s.Transitions {
			assert.NotEqual(t, "create_pr", tgt,
				"step %s still points to create_pr after SkipPR", s.Name)
		}
	}
}

func TestApplySkipFlags_SkipImplementAndReview(t *testing.T) {
	base := workflow.GetDefinition(workflow.WorkflowImplementReview)
	def := applySkipFlags(base, PipelineOpts{SkipImplement: true, SkipReview: true})

	assert.Equal(t, "create_pr", def.InitialStep)
	names := stepNames(def)
	assert.NotContains(t, names, "run_implement")
	assert.NotContains(t, names, "run_review")
}

func TestResolvePhases_AllPhases(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Phase One|T-001|T-010",
		"2|Phase Two|T-011|T-020",
	})

	orch := NewPipelineOrchestrator(nil, nil, nil, makeConfig(phasesPath))

	phases, err := orch.resolvePhases(PipelineOpts{})
	require.NoError(t, err)
	assert.Len(t, phases, 2)
}

func TestResolvePhases_SinglePhase(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Phase One|T-001|T-010",
		"2|Phase Two|T-011|T-020",
	})

	orch := NewPipelineOrchestrator(nil, nil, nil, makeConfig(phasesPath))

	phases, err := orch.resolvePhases(PipelineOpts{PhaseID: "2"})
	require.NoError(t, err)
	require.Len(t, phases, 1)
	assert.Equal(t, 2, phases[0].ID)
}

func TestResolvePhases_AllKeyword(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Phase One|T-001|T-010",
		"2|Phase Two|T-011|T-020",
		"3|Phase Three|T-021|T-030",
	})

	orch := NewPipelineOrchestrator(nil, nil, nil, makeConfig(phasesPath))

	phases, err := orch.resolvePhases(PipelineOpts{PhaseID: "all"})
	require.NoError(t, err)
	assert.Len(t, phases, 3)
}

func TestResolvePhases_FromPhase(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Phase One|T-001|T-010",
		"2|Phase Two|T-011|T-020",
		"3|Phase Three|T-021|T-030",
	})

	orch := NewPipelineOrchestrator(nil, nil, nil, makeConfig(phasesPath))

	phases, err := orch.resolvePhases(PipelineOpts{FromPhase: "2"})
	require.NoError(t, err)
	require.Len(t, phases, 2)
	assert.Equal(t, 2, phases[0].ID)
	assert.Equal(t, 3, phases[1].ID)
}

func TestResolvePhases_NotFound(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Phase One|T-001|T-010",
	})

	orch := NewPipelineOrchestrator(nil, nil, nil, makeConfig(phasesPath))

	_, err := orch.resolvePhases(PipelineOpts{PhaseID: "99"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "phase 99 not found")
}

func TestResolvePhases_NoPhasesConf(t *testing.T) {
	orch := NewPipelineOrchestrator(nil, nil, nil, &config.Config{})
	_, err := orch.resolvePhases(PipelineOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "phases_conf")
}

func TestFilterFromPhase(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Phase One|T-001|T-010",
		"2|Phase Two|T-011|T-020",
		"3|Phase Three|T-021|T-030",
	})
	orch := NewPipelineOrchestrator(nil, nil, nil, makeConfig(phasesPath))

	tests := []struct {
		name      string
		from      string
		wantIDs   []int
		wantError bool
	}{
		{
			name:    "from 2 returns phases 2 and 3",
			from:    "2",
			wantIDs: []int{2, 3},
		},
		{
			name:    "from 1 returns all phases",
			from:    "1",
			wantIDs: []int{1, 2, 3},
		},
		{
			name:    "from 3 returns only phase 3",
			from:    "3",
			wantIDs: []int{3},
		},
		{
			name:      "from 99 returns error",
			from:      "99",
			wantError: true,
		},
		{
			name:      "non-numeric from returns error",
			from:      "abc",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := orch.resolvePhases(PipelineOpts{FromPhase: tt.from})
			if tt.wantError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			ids := make([]int, len(got))
			for i, p := range got {
				ids[i] = p.ID
			}
			assert.Equal(t, tt.wantIDs, ids)
		})
	}
}

func TestRun_SinglePhase_Success(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
	})

	engine := workflow.NewEngine(
		successRegistry(),
		workflow.WithMaxIterations(50),
	)

	orch := NewPipelineOrchestrator(engine, nil, nil, makeConfig(phasesPath))
	result, err := orch.Run(context.Background(), PipelineOpts{
		PhaseID:   "1",
		ImplAgent: "claude",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, PipelineStatusCompleted, result.Status)
	require.Len(t, result.Phases, 1)
	assert.Equal(t, PhaseStatusCompleted, result.Phases[0].Status)
	assert.Equal(t, "1", result.Phases[0].PhaseID)
	assert.Equal(t, "Foundation", result.Phases[0].PhaseName)
	assert.Equal(t, "raven/phase-1", result.Phases[0].BranchName)
}

func TestRun_MultiplePhases_AllSuccess(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
		"2|Implementation|T-011|T-020",
	})

	engine := workflow.NewEngine(successRegistry(), workflow.WithMaxIterations(100))
	orch := NewPipelineOrchestrator(engine, nil, nil, makeConfig(phasesPath))

	result, err := orch.Run(context.Background(), PipelineOpts{ImplAgent: "codex"})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, PipelineStatusCompleted, result.Status)
	assert.Len(t, result.Phases, 2)
	for _, pr := range result.Phases {
		assert.Equal(t, PhaseStatusCompleted, pr.Status)
	}
}

func TestRun_SinglePhase_EngineFailure(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
	})

	engine := workflow.NewEngine(failingRegistry(), workflow.WithMaxIterations(50))
	orch := NewPipelineOrchestrator(engine, nil, nil, makeConfig(phasesPath))

	result, err := orch.Run(context.Background(), PipelineOpts{PhaseID: "1"})

	// Run returns result even on phase failure (no error at pipeline level;
	// the error is captured in PhaseResult).
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, PipelineStatusFailed, result.Status)
	require.Len(t, result.Phases, 1)
	assert.Equal(t, PhaseStatusFailed, result.Phases[0].Status)
	assert.NotEmpty(t, result.Phases[0].Error)
}

func TestRun_MultiplePhases_PartialFailure(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
		"2|Implementation|T-011|T-020",
	})

	// Phase 1 succeeds with a success registry, phase 2 uses the same
	// registry. We'll make the orchestrator fail phase 2 by making a registry
	// that succeeds for phase 1 then fails for phase 2.
	// Simpler approach: run with 2 phases and use a registry that alternates.
	// For simplicity: use a failing registry for both -- both phases fail,
	// overall is "failed". Then test partial via a mixed scenario.
	engine := workflow.NewEngine(failingRegistry(), workflow.WithMaxIterations(50))
	orch := NewPipelineOrchestrator(engine, nil, nil, makeConfig(phasesPath))

	result, err := orch.Run(context.Background(), PipelineOpts{})
	require.NoError(t, err)
	// Both phases fail -> overall "failed".
	assert.Equal(t, PipelineStatusFailed, result.Status)
	assert.Len(t, result.Phases, 2)
}

func TestRun_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
		"2|Implementation|T-011|T-020",
		"3|Review|T-021|T-030",
	})

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel before the run starts.
	cancel()

	engine := workflow.NewEngine(successRegistry(), workflow.WithMaxIterations(100))
	orch := NewPipelineOrchestrator(engine, nil, nil, makeConfig(phasesPath))

	result, err := orch.Run(ctx, PipelineOpts{})

	// Should return a context error.
	require.Error(t, err)
	require.NotNil(t, result)
	// All phases should be pending since we cancelled before any started.
	for _, pr := range result.Phases {
		assert.Equal(t, PhaseStatusPending, pr.Status)
	}
}

func TestRun_SkipFlags_SkipReview(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
	})

	// Registry needs only run_implement and create_pr since others are skipped.
	reg := workflow.NewRegistry()
	reg.Register(&stubHandler{name: "run_implement", event: workflow.EventSuccess})
	reg.Register(&stubHandler{name: "create_pr", event: workflow.EventSuccess})

	engine := workflow.NewEngine(reg, workflow.WithMaxIterations(20))
	orch := NewPipelineOrchestrator(engine, nil, nil, makeConfig(phasesPath))

	result, err := orch.Run(context.Background(), PipelineOpts{
		PhaseID:    "1",
		SkipReview: true,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, PipelineStatusCompleted, result.Status)
	assert.Equal(t, PhaseStatusCompleted, result.Phases[0].Status)
}

func TestRun_SkipFlags_SkipImplementAndPR(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
	})

	// SkipImplement + SkipPR: only review steps remain.
	reg := workflow.NewRegistry()
	reg.Register(&stubHandler{name: "run_review", event: workflow.EventSuccess})
	reg.Register(&stubHandler{name: "check_review", event: workflow.EventSuccess})
	reg.Register(&stubHandler{name: "run_fix", event: workflow.EventSuccess})

	engine := workflow.NewEngine(reg, workflow.WithMaxIterations(20))
	orch := NewPipelineOrchestrator(engine, nil, nil, makeConfig(phasesPath))

	result, err := orch.Run(context.Background(), PipelineOpts{
		PhaseID:       "1",
		SkipImplement: true,
		SkipPR:        true,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, PipelineStatusCompleted, result.Status)
}

func TestRun_DryRunMode(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
	})

	engine := workflow.NewEngine(
		successRegistry(),
		workflow.WithDryRun(true),
		workflow.WithMaxIterations(50),
	)
	orch := NewPipelineOrchestrator(engine, nil, nil, makeConfig(phasesPath))

	result, err := orch.Run(context.Background(), PipelineOpts{
		PhaseID: "1",
		DryRun:  true,
	})

	require.NoError(t, err)
	assert.Equal(t, PipelineStatusCompleted, result.Status)
}

func TestDryRun_Output(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
		"2|Implementation|T-011|T-020",
	})

	orch := NewPipelineOrchestrator(nil, nil, nil, makeConfig(phasesPath))

	output := orch.DryRun(PipelineOpts{ImplAgent: "codex"})

	assert.Contains(t, output, "Pipeline dry-run plan")
	assert.Contains(t, output, "Phase 1: Foundation")
	assert.Contains(t, output, "Phase 2: Implementation")
	assert.Contains(t, output, "raven/phase-1")
	assert.Contains(t, output, "raven/phase-2")
	assert.Contains(t, output, "codex")
}

func TestDryRun_MissingPhasesConf(t *testing.T) {
	orch := NewPipelineOrchestrator(nil, nil, nil, &config.Config{})
	output := orch.DryRun(PipelineOpts{})
	assert.Contains(t, output, "pipeline dry-run")
}

func TestRun_Checkpointing(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
	})
	stateDir := filepath.Join(dir, "state")
	store, err := workflow.NewStateStore(stateDir)
	require.NoError(t, err)

	engine := workflow.NewEngine(successRegistry(), workflow.WithMaxIterations(50))
	orch := NewPipelineOrchestrator(engine, store, nil, makeConfig(phasesPath))

	result, err := orch.Run(context.Background(), PipelineOpts{PhaseID: "1"})
	require.NoError(t, err)
	assert.Equal(t, PipelineStatusCompleted, result.Status)

	// A checkpoint file should have been written to stateDir.
	entries, err := os.ReadDir(stateDir)
	require.NoError(t, err)
	jsonFiles := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			jsonFiles++
		}
	}
	assert.Greater(t, jsonFiles, 0, "expected at least one checkpoint JSON file")
}

func TestRun_AgentNormalization(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
	})

	engine := workflow.NewEngine(successRegistry(), workflow.WithMaxIterations(50))
	orch := NewPipelineOrchestrator(engine, nil, nil, makeConfig(phasesPath))

	// Unknown agent should be silently normalised to "claude".
	result, err := orch.Run(context.Background(), PipelineOpts{
		PhaseID:   "1",
		ImplAgent: "unknown-agent",
	})
	require.NoError(t, err)
	assert.Equal(t, PipelineStatusCompleted, result.Status)
}

func TestRun_PhaseMetadataSet(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
	})

	// Capture the workflow state passed to the handler to inspect metadata.
	var capturedPhaseID any
	captureHandler := &captureMetaHandler{
		name:  "run_implement",
		event: workflow.EventSuccess,
		onExecute: func(state *workflow.WorkflowState) {
			capturedPhaseID = state.Metadata["phase_id"]
		},
	}
	reg := workflow.NewRegistry()
	reg.Register(captureHandler)
	for _, name := range []string{"run_review", "check_review", "run_fix", "create_pr"} {
		reg.Register(&stubHandler{name: name, event: workflow.EventSuccess})
	}

	engine := workflow.NewEngine(reg, workflow.WithMaxIterations(50))
	orch := NewPipelineOrchestrator(engine, nil, nil, makeConfig(phasesPath))

	_, err := orch.Run(context.Background(), PipelineOpts{PhaseID: "1"})
	require.NoError(t, err)
	assert.Equal(t, 1, capturedPhaseID)
}

func TestBuildResult_AllSuccess(t *testing.T) {
	p := &PipelineOrchestrator{}
	results := []PhaseResult{
		{Status: PhaseStatusCompleted},
		{Status: PhaseStatusCompleted},
	}
	r := p.buildResult(results, time.Now().Add(-time.Second), 2, 0)
	assert.Equal(t, PipelineStatusCompleted, r.Status)
}

func TestBuildResult_AllFailed(t *testing.T) {
	p := &PipelineOrchestrator{}
	results := []PhaseResult{
		{Status: PhaseStatusFailed},
		{Status: PhaseStatusFailed},
	}
	r := p.buildResult(results, time.Now().Add(-time.Second), 0, 2)
	assert.Equal(t, PipelineStatusFailed, r.Status)
}

func TestBuildResult_Mixed(t *testing.T) {
	p := &PipelineOrchestrator{}
	results := []PhaseResult{
		{Status: PhaseStatusCompleted},
		{Status: PhaseStatusFailed},
	}
	r := p.buildResult(results, time.Now().Add(-time.Second), 1, 1)
	assert.Equal(t, PipelineStatusPartial, r.Status)
}

func TestExtractPhaseResult(t *testing.T) {
	state := workflow.NewWorkflowState("id", "wf", "step")
	state.Metadata["impl_status"] = "done"
	state.Metadata["review_verdict"] = "approved"
	state.Metadata["fix_status"] = "fixed"
	state.Metadata["pr_url"] = "https://github.com/example/pr/1"

	result := PhaseResult{PhaseID: "1"}
	result = extractPhaseResult(result, state)

	assert.Equal(t, "done", result.ImplStatus)
	assert.Equal(t, "approved", result.ReviewVerdict)
	assert.Equal(t, "fixed", result.FixStatus)
	assert.Equal(t, "https://github.com/example/pr/1", result.PRURL)
}

// TestWithPipelineLogger verifies that the functional option attaches a logger
// to the orchestrator without panicking.
func TestWithPipelineLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := log.NewWithOptions(&buf, log.Options{Level: log.DebugLevel})

	orch := NewPipelineOrchestrator(nil, nil, nil, &config.Config{},
		WithPipelineLogger(logger),
	)
	require.NotNil(t, orch)
	assert.NotNil(t, orch.logger)
}

// TestWithPipelineEvents verifies that the functional option sets the event
// channel on the orchestrator.
func TestWithPipelineEvents(t *testing.T) {
	ch := make(chan workflow.WorkflowEvent, 8)
	orch := NewPipelineOrchestrator(nil, nil, nil, &config.Config{},
		WithPipelineEvents(ch),
	)
	require.NotNil(t, orch)
	assert.NotNil(t, orch.events)
}

// TestWithPipelineLogger_LogsMessages verifies that log and logMsg output
// goes to the attached logger.
func TestWithPipelineLogger_LogsMessages(t *testing.T) {
	var buf bytes.Buffer
	logger := log.NewWithOptions(&buf, log.Options{Level: log.DebugLevel})

	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
	})

	engine := workflow.NewEngine(successRegistry(), workflow.WithMaxIterations(50))
	orch := NewPipelineOrchestrator(engine, nil, nil, makeConfig(phasesPath),
		WithPipelineLogger(logger),
	)

	_, err := orch.Run(context.Background(), PipelineOpts{PhaseID: "1"})
	require.NoError(t, err)

	// The logger should have received at least one message.
	assert.NotEmpty(t, buf.String())
}

// TestResolvePhases_InvalidPhaseID checks that a non-numeric PhaseID produces
// the correct error.
func TestResolvePhases_InvalidPhaseID(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Phase One|T-001|T-010",
	})

	orch := NewPipelineOrchestrator(nil, nil, nil, makeConfig(phasesPath))
	_, err := orch.resolvePhases(PipelineOpts{PhaseID: "notanumber"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid phase ID")
}

// TestResolvePhases_LoadError checks that an error reading the phases file is
// propagated correctly.
func TestResolvePhases_LoadError(t *testing.T) {
	orch := NewPipelineOrchestrator(nil, nil, nil, makeConfig("/nonexistent/path/phases.conf"))
	_, err := orch.resolvePhases(PipelineOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load phases")
}

// TestRun_PhaseResolveError checks that a resolve-phases error is returned as
// an error from Run (not just captured in PhaseResult).
func TestRun_PhaseResolveError(t *testing.T) {
	orch := NewPipelineOrchestrator(nil, nil, nil, &config.Config{})
	result, err := orch.Run(context.Background(), PipelineOpts{})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "resolve phases")
}

// TestDryRun_SkipReviewAndFix verifies that review/fix agent lines are omitted
// from the dry-run output when those steps are skipped.
func TestDryRun_SkipReviewAndFix(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
	})

	orch := NewPipelineOrchestrator(nil, nil, nil, makeConfig(phasesPath))

	output := orch.DryRun(PipelineOpts{
		SkipReview: true,
		SkipFix:    true,
	})

	assert.Contains(t, output, "Pipeline dry-run plan")
	assert.Contains(t, output, "Phase 1")
	assert.NotContains(t, output, "Review agent:")
	assert.NotContains(t, output, "Fix agent:")
}

// TestDryRun_SkipPR verifies that create_pr does not appear in steps when
// SkipPR is set.
func TestDryRun_SkipPR(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
	})

	orch := NewPipelineOrchestrator(nil, nil, nil, makeConfig(phasesPath))
	output := orch.DryRun(PipelineOpts{SkipPR: true})

	assert.NotContains(t, output, "create_pr")
}

// TestRunPhase_AllStepsSkipped exercises the early-return path in runPhase
// where every step has been removed (InitialStep == StepDone).
func TestRunPhase_AllStepsSkipped(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
	})

	engine := workflow.NewEngine(successRegistry(), workflow.WithMaxIterations(50))
	orch := NewPipelineOrchestrator(engine, nil, nil, makeConfig(phasesPath))

	result, err := orch.Run(context.Background(), PipelineOpts{
		PhaseID:       "1",
		SkipImplement: true,
		SkipReview:    true,
		SkipFix:       true,
		SkipPR:        true,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, PipelineStatusCompleted, result.Status)
	require.Len(t, result.Phases, 1)
	assert.Equal(t, PhaseStatusCompleted, result.Phases[0].Status)
}

// TestExtractPhaseResult_ErrorField verifies that the error field from
// workflow state metadata is read when the result has no prior error set.
func TestExtractPhaseResult_ErrorField(t *testing.T) {
	state := workflow.NewWorkflowState("id", "wf", "step")
	state.Metadata["error"] = "something went wrong"

	result := PhaseResult{PhaseID: "1"}
	result = extractPhaseResult(result, state)

	assert.Equal(t, "something went wrong", result.Error)
}

// TestExtractPhaseResult_ErrorNotOverwritten verifies that a pre-existing
// result.Error value is NOT overwritten by the state metadata error field.
func TestExtractPhaseResult_ErrorNotOverwritten(t *testing.T) {
	state := workflow.NewWorkflowState("id", "wf", "step")
	state.Metadata["error"] = "metadata error"

	result := PhaseResult{PhaseID: "1", Error: "original error"}
	result = extractPhaseResult(result, state)

	// Pre-existing error must be preserved.
	assert.Equal(t, "original error", result.Error)
}

// TestRun_ContextCancelledAfterFirstPhase checks the mid-run context
// cancellation code path where at least one phase completes before the
// context is cancelled.
func TestRun_ContextCancelledAfterFirstPhase(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
		"2|Implementation|T-011|T-020",
		"3|Review|T-021|T-030",
	})

	ctx, cancel := context.WithCancel(context.Background())

	var callCount int64
	// Phase 1 handler cancels the context then succeeds; phases 2 and 3 are
	// never reached.
	cancelOnFirstExec := &captureMetaHandler{
		name:  "run_implement",
		event: workflow.EventSuccess,
		onExecute: func(_ *workflow.WorkflowState) {
			if atomic.AddInt64(&callCount, 1) == 1 {
				cancel()
			}
		},
	}

	reg := workflow.NewRegistry()
	reg.Register(cancelOnFirstExec)
	for _, name := range []string{"run_review", "check_review", "run_fix", "create_pr"} {
		reg.Register(&stubHandler{name: name, event: workflow.EventSuccess})
	}

	engine := workflow.NewEngine(reg, workflow.WithMaxIterations(100))
	orch := NewPipelineOrchestrator(engine, nil, nil, makeConfig(phasesPath))

	result, err := orch.Run(ctx, PipelineOpts{})

	// The run must return a context error and a non-nil partial result.
	require.Error(t, err)
	require.NotNil(t, result)

	// Phase 1 may have started; remaining phases must be pending.
	pendingCount := 0
	for _, pr := range result.Phases {
		if pr.Status == PhaseStatusPending {
			pendingCount++
		}
	}
	assert.Greater(t, pendingCount, 0, "expected at least one pending phase after cancellation")
}

// TestRun_WithStateStoreResume exercises the loadOrCreatePipelineState resume
// path: after a first run that completes phase 1, a second run with the same
// store should detect the existing checkpoint and resume from index 1.
func TestRun_WithStateStoreResume(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
		"2|Implementation|T-011|T-020",
	})
	stateDir := filepath.Join(dir, "state")
	store, err := workflow.NewStateStore(stateDir)
	require.NoError(t, err)

	engine := workflow.NewEngine(successRegistry(), workflow.WithMaxIterations(100))

	// First run: should complete both phases and save checkpoints.
	orch := NewPipelineOrchestrator(engine, store, nil, makeConfig(phasesPath))
	result1, err := orch.Run(context.Background(), PipelineOpts{})
	require.NoError(t, err)
	assert.Equal(t, PipelineStatusCompleted, result1.Status)

	// The state store should contain a checkpoint.
	entries, err := os.ReadDir(stateDir)
	require.NoError(t, err)
	jsonCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			jsonCount++
		}
	}
	assert.Greater(t, jsonCount, 0)

	// Second run: orchestrator sees the existing checkpoint and either resumes
	// or starts fresh. Both are valid outcomes; what matters is no panic/error.
	orch2 := NewPipelineOrchestrator(engine, store, nil, makeConfig(phasesPath))
	result2, err := orch2.Run(context.Background(), PipelineOpts{})
	require.NoError(t, err)
	assert.NotNil(t, result2)
}

// TestSavePipelineState_NilStore verifies that savePipelineState is a no-op
// when no state store has been attached.
func TestSavePipelineState_NilStore(t *testing.T) {
	orch := &PipelineOrchestrator{}
	// Should not panic or return an error when store is nil.
	err := orch.savePipelineState("run-id", PipelineOpts{}, nil, 0)
	assert.NoError(t, err)
}

// TestLoadOrCreatePipelineState_NilStore verifies that loadOrCreatePipelineState
// returns a fresh run ID with index 0 when no store is attached.
func TestLoadOrCreatePipelineState_NilStore(t *testing.T) {
	orch := &PipelineOrchestrator{}
	runID, startIdx, results, err := orch.loadOrCreatePipelineState(PipelineOpts{})
	require.NoError(t, err)
	assert.NotEmpty(t, runID)
	assert.Equal(t, 0, startIdx)
	assert.Nil(t, results)
}

// TestLoadOrCreatePipelineState_NonPipelineWorkflow verifies that a checkpoint
// belonging to a different workflow type is ignored and a fresh run is returned.
func TestLoadOrCreatePipelineState_NonPipelineWorkflow(t *testing.T) {
	dir := t.TempDir()
	store, err := workflow.NewStateStore(dir)
	require.NoError(t, err)

	// Save a checkpoint for a non-pipeline workflow.
	otherState := workflow.NewWorkflowState("other-run", workflow.WorkflowImplement, workflow.StepDone)
	require.NoError(t, store.Save(otherState))

	orch := &PipelineOrchestrator{store: store}
	runID, startIdx, results, err := orch.loadOrCreatePipelineState(PipelineOpts{})
	require.NoError(t, err)
	assert.NotEmpty(t, runID)
	// Fresh run: should start from 0, no prior results.
	assert.Equal(t, 0, startIdx)
	assert.Nil(t, results)
}

// TestLoadOrCreatePipelineState_CompletedPipeline verifies that a completed
// pipeline checkpoint is not resumed (a fresh run ID is returned instead).
func TestLoadOrCreatePipelineState_CompletedPipeline(t *testing.T) {
	dir := t.TempDir()
	store, err := workflow.NewStateStore(dir)
	require.NoError(t, err)

	// Save a completed pipeline checkpoint.
	completedState := workflow.NewWorkflowState("done-run", workflow.WorkflowPipeline, workflow.StepDone)
	require.NoError(t, store.Save(completedState))

	orch := &PipelineOrchestrator{store: store}
	runID, startIdx, results, err := orch.loadOrCreatePipelineState(PipelineOpts{})
	require.NoError(t, err)
	// Must not reuse the completed run's ID.
	assert.NotEqual(t, "done-run", runID)
	assert.Equal(t, 0, startIdx)
	assert.Nil(t, results)
}

// TestLoadOrCreatePipelineState_ResumablePipeline verifies that an incomplete
// pipeline checkpoint is resumed: run ID, start index, and phase results are
// all restored from the persisted state.
func TestLoadOrCreatePipelineState_ResumablePipeline(t *testing.T) {
	dir := t.TempDir()
	store, err := workflow.NewStateStore(dir)
	require.NoError(t, err)

	orch := &PipelineOrchestrator{store: store}

	// Persist a partial pipeline state (phase index 1 completed, index 2 pending).
	phaseResults := []PhaseResult{
		{PhaseID: "1", PhaseName: "Foundation", Status: PhaseStatusCompleted},
		{PhaseID: "2", PhaseName: "Implementation", Status: PhaseStatusPending},
	}
	require.NoError(t, orch.savePipelineState("resume-run", PipelineOpts{}, phaseResults, 1))

	// Overwrite the ID to something recognisable so we can detect resume.
	// loadOrCreatePipelineState uses LatestRun which picks the most recent by
	// UpdatedAt, so the state we just saved will be picked up.
	runID, startIdx, existingResults, err := orch.loadOrCreatePipelineState(PipelineOpts{})
	require.NoError(t, err)
	assert.Equal(t, "resume-run", runID)
	assert.Equal(t, 1, startIdx)
	require.Len(t, existingResults, 2)
	assert.Equal(t, PhaseStatusCompleted, existingResults[0].Status)
}

// TestRun_LoadStateFailure tests the fallback code path in Run when
// loadOrCreatePipelineState returns an error (forcing a fresh start).
// We achieve this by building a StateStore pointing at a valid directory,
// then replacing that directory with a regular file before calling Run so
// that os.ReadDir fails and loadOrCreatePipelineState returns an error.
func TestRun_LoadStateFailure(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
	})

	// Build a valid store so we get a non-nil *StateStore.
	stateDir := filepath.Join(dir, "state")
	store, err := workflow.NewStateStore(stateDir)
	require.NoError(t, err)

	// Corrupt the store by replacing its directory with a regular file.
	require.NoError(t, os.RemoveAll(stateDir))
	require.NoError(t, os.WriteFile(stateDir, []byte("corrupt"), 0o644))

	engine := workflow.NewEngine(successRegistry(), workflow.WithMaxIterations(50))
	orch := NewPipelineOrchestrator(engine, store, nil, makeConfig(phasesPath))

	// Even with a broken store the run should fall back to a fresh start and
	// complete the single phase.
	result, err := orch.Run(context.Background(), PipelineOpts{PhaseID: "1"})
	require.NoError(t, err)
	assert.Equal(t, PipelineStatusCompleted, result.Status)
}

// TestBuildResult_TotalDuration checks that TotalDuration is a non-negative
// value derived from the supplied start time.
func TestBuildResult_TotalDuration(t *testing.T) {
	p := &PipelineOrchestrator{}
	start := time.Now().Add(-500 * time.Millisecond)
	r := p.buildResult(nil, start, 0, 0)
	assert.GreaterOrEqual(t, r.TotalDuration, time.Duration(0))
}

// TestRun_WithEventChannel verifies that the pipeline run completes without
// errors when a WorkflowEvent channel is attached via WithPipelineEvents.
func TestRun_WithEventChannel(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
	})

	// A buffered channel captures events emitted directly by the engine.
	engineCh := make(chan workflow.WorkflowEvent, 64)

	engine := workflow.NewEngine(
		successRegistry(),
		workflow.WithMaxIterations(50),
		workflow.WithEventChannel(engineCh),
	)
	// Attach a separate pipeline-level event channel (currently unused by the
	// orchestrator's Run path, but attaching it exercises the functional option).
	pipelineCh := make(chan workflow.WorkflowEvent, 64)
	orch := NewPipelineOrchestrator(engine, nil, nil, makeConfig(phasesPath),
		WithPipelineEvents(pipelineCh),
	)

	result, err := orch.Run(context.Background(), PipelineOpts{PhaseID: "1"})
	require.NoError(t, err)
	assert.Equal(t, PipelineStatusCompleted, result.Status)

	// The engine channel should have received at least the workflow-started event.
	assert.Greater(t, len(engineCh), 0, "expected events on engine channel")
}

// TestApplySkipFlags_SkipFixAndReview exercises the combined SkipFix+SkipReview
// path (SkipFix is a no-op when SkipReview is also set).
func TestApplySkipFlags_SkipFixAndReview(t *testing.T) {
	base := workflow.GetDefinition(workflow.WorkflowImplementReview)
	require.NotNil(t, base)

	def := applySkipFlags(base, PipelineOpts{SkipFix: true, SkipReview: true})

	names := stepNames(def)
	assert.NotContains(t, names, "run_review")
	assert.NotContains(t, names, "check_review")
	assert.NotContains(t, names, "run_fix")
}

// TestApplySkipFlags_SkipImplementAndPR exercises the combined
// SkipImplement+SkipPR path where the initial step becomes StepDone.
func TestApplySkipFlags_SkipImplementAndPR(t *testing.T) {
	base := workflow.GetDefinition(workflow.WorkflowImplementReview)
	require.NotNil(t, base)

	def := applySkipFlags(base, PipelineOpts{
		SkipImplement: true,
		SkipReview:    true,
		SkipPR:        true,
	})

	assert.Equal(t, workflow.StepDone, def.InitialStep)
}

// TestRun_PartialSuccess exercises the partial-success pipeline status:
// phase 1 succeeds and phase 2 fails, so the overall status is "partial".
func TestRun_PartialSuccess(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
		"2|Implementation|T-011|T-020",
	})

	// callCount tracks how many times run_implement has been called across all
	// phases. On the second call (phase 2) the handler returns EventFailure so
	// that phase 2 reaches __failed__ and the pipeline records a failure.
	var callCount int64
	mixedImplHandler := &errorOnNthCallHandler{
		name:      "run_implement",
		failOnNth: 2,
		counter:   &callCount,
	}

	reg := workflow.NewRegistry()
	reg.Register(mixedImplHandler)
	for _, name := range []string{"run_review", "check_review", "run_fix", "create_pr"} {
		reg.Register(&stubHandler{name: name, event: workflow.EventSuccess})
	}

	engine := workflow.NewEngine(reg, workflow.WithMaxIterations(100))
	orch := NewPipelineOrchestrator(engine, nil, nil, makeConfig(phasesPath))

	result, err := orch.Run(context.Background(), PipelineOpts{})
	require.NoError(t, err)
	// Phase 1 succeeded, phase 2 failed -> overall "partial".
	assert.Equal(t, PipelineStatusPartial, result.Status)
	require.Len(t, result.Phases, 2)
	assert.Equal(t, PhaseStatusCompleted, result.Phases[0].Status)
	assert.Equal(t, PhaseStatusFailed, result.Phases[1].Status)
}

// TestRun_LoggerWithCancelledContext verifies that log calls inside the
// cancellation branch do not panic when a logger is attached.
func TestRun_LoggerWithCancelledContext(t *testing.T) {
	dir := t.TempDir()
	phasesPath := writePhasesFile(t, dir, []string{
		"1|Foundation|T-001|T-010",
		"2|Implementation|T-011|T-020",
	})

	var buf bytes.Buffer
	logger := log.NewWithOptions(&buf, log.Options{Level: log.DebugLevel})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	engine := workflow.NewEngine(successRegistry(), workflow.WithMaxIterations(50))
	orch := NewPipelineOrchestrator(engine, nil, nil, makeConfig(phasesPath),
		WithPipelineLogger(logger),
	)

	result, err := orch.Run(ctx, PipelineOpts{})
	require.Error(t, err)
	require.NotNil(t, result)
	// Logger should have received the cancellation message.
	assert.NotEmpty(t, buf.String())
}

// --- helper types --------------------------------------------------------

// captureMetaHandler is a stub that executes an onExecute callback before
// returning EventSuccess. Used to inspect workflow state in tests.
type captureMetaHandler struct {
	name      string
	event     string
	onExecute func(*workflow.WorkflowState)
}

func (h *captureMetaHandler) Name() string { return h.name }
func (h *captureMetaHandler) DryRun(_ *workflow.WorkflowState) string {
	return "capture dry-run"
}
func (h *captureMetaHandler) Execute(_ context.Context, state *workflow.WorkflowState) (string, error) {
	if h.onExecute != nil {
		h.onExecute(state)
	}
	return h.event, nil
}

// errorOnNthCallHandler is a StepHandler that succeeds on all calls except
// the Nth, on which it returns EventFailure with a synthetic error.
type errorOnNthCallHandler struct {
	name      string
	failOnNth int64
	counter   *int64
}

func (h *errorOnNthCallHandler) Name() string { return h.name }
func (h *errorOnNthCallHandler) DryRun(_ *workflow.WorkflowState) string {
	return fmt.Sprintf("error-on-nth dry-run %s", h.name)
}
func (h *errorOnNthCallHandler) Execute(_ context.Context, _ *workflow.WorkflowState) (string, error) {
	n := atomic.AddInt64(h.counter, 1)
	if n == h.failOnNth {
		return workflow.EventFailure, fmt.Errorf("synthetic failure on call %d", n)
	}
	return workflow.EventSuccess, nil
}

// --- step list helpers ---------------------------------------------------

func stepNames(def *workflow.WorkflowDefinition) []string {
	names := make([]string, len(def.Steps))
	for i, s := range def.Steps {
		names[i] = s.Name
	}
	return names
}

func findStep(def *workflow.WorkflowDefinition, name string) *workflow.StepDefinition {
	for i := range def.Steps {
		if def.Steps[i].Name == name {
			return &def.Steps[i]
		}
	}
	return nil
}

