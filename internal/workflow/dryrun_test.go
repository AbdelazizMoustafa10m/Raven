package workflow

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// minimalSingleStepDef returns a WorkflowDefinition with one step that
// transitions to StepDone on success and StepFailed on failure.
func minimalSingleStepDef() *WorkflowDefinition {
	return &WorkflowDefinition{
		Name:        "test-workflow",
		Description: "A minimal single-step workflow for tests.",
		InitialStep: "run_implement",
		Steps: []StepDefinition{
			{
				Name: "run_implement",
				Transitions: map[string]string{
					EventSuccess: StepDone,
					EventFailure: StepFailed,
				},
			},
		},
	}
}

// minimalState returns a WorkflowState suitable for passing to
// FormatWorkflowDryRun (the method ignores it, but callers still need one).
func minimalState() *WorkflowState {
	return NewWorkflowState("test-run-1", "test-workflow", "run_implement")
}

// ---------------------------------------------------------------------------
// Write
// ---------------------------------------------------------------------------

func TestDryRunFormatter_Write(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)
	f.Write("hello")
	assert.Equal(t, "hello", buf.String())
}

func TestDryRunFormatter_Write_EmptyString(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)
	f.Write("")
	assert.Equal(t, "", buf.String())
}

func TestDryRunFormatter_Write_MultipleWrites(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)
	f.Write("foo")
	f.Write("bar")
	assert.Equal(t, "foobar", buf.String())
}

// ---------------------------------------------------------------------------
// FormatWorkflowDryRun -- empty / nil definitions
// ---------------------------------------------------------------------------

func TestFormatWorkflowDryRun_EmptyDefinition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		def  *WorkflowDefinition
	}{
		{
			name: "nil definition",
			def:  nil,
		},
		{
			name: "definition with nil steps",
			def: &WorkflowDefinition{
				Name:        "no-steps",
				InitialStep: "run_implement",
				Steps:       nil,
			},
		},
		{
			name: "definition with empty steps slice",
			def: &WorkflowDefinition{
				Name:        "no-steps",
				InitialStep: "run_implement",
				Steps:       []StepDefinition{},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			f := NewDryRunFormatter(&buf, false)
			got := f.FormatWorkflowDryRun(tt.def, minimalState(), nil)
			assert.Equal(t, "No steps defined.\n", got)
		})
	}
}

// ---------------------------------------------------------------------------
// FormatWorkflowDryRun -- single step
// ---------------------------------------------------------------------------

func TestFormatWorkflowDryRun_SingleStep(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)
	def := minimalSingleStepDef()

	got := f.FormatWorkflowDryRun(def, minimalState(), map[string]string{})

	assert.Contains(t, got, "Workflow: test-workflow", "output must contain workflow header")
	assert.Contains(t, got, "1. run_implement", "output must list the step with its number")
	assert.Contains(t, got, "-> failure: FAILED", "failure transition must be labelled FAILED")
	assert.Contains(t, got, "-> success: DONE", "success transition must be labelled DONE")
}

func TestFormatWorkflowDryRun_SingleStep_UnderlineMatchesHeader(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)
	def := minimalSingleStepDef()

	got := f.FormatWorkflowDryRun(def, minimalState(), map[string]string{})

	// The header is "Workflow: test-workflow" (22 chars); verify underline exists.
	header := "Workflow: test-workflow"
	underline := strings.Repeat("=", len(header))
	assert.Contains(t, got, underline, "output must contain an underline matching the header length")
}

// ---------------------------------------------------------------------------
// FormatWorkflowDryRun -- step descriptions from stepOutputs
// ---------------------------------------------------------------------------

func TestFormatWorkflowDryRun_WithStepOutputs(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)
	def := minimalSingleStepDef()
	desc := "run the main implementation agent"
	stepOutputs := map[string]string{
		"run_implement": desc,
	}

	got := f.FormatWorkflowDryRun(def, minimalState(), stepOutputs)

	assert.Contains(t, got, desc,
		"the step description from stepOutputs must appear in the output")
}

func TestFormatWorkflowDryRun_FallbackDescription(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)
	def := minimalSingleStepDef()

	// No descriptions provided -- fallback is "step N".
	got := f.FormatWorkflowDryRun(def, minimalState(), map[string]string{})

	assert.Contains(t, got, "step 1",
		"when no stepOutputs entry exists the fallback 'step N' must appear")
}

func TestFormatWorkflowDryRun_EmptyDescriptionFallsBack(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)
	def := minimalSingleStepDef()

	// Empty string value -- also falls back.
	got := f.FormatWorkflowDryRun(def, minimalState(), map[string]string{
		"run_implement": "",
	})

	assert.Contains(t, got, "step 1",
		"empty description in stepOutputs must fall back to 'step N'")
}

// ---------------------------------------------------------------------------
// FormatWorkflowDryRun -- cycle detection
// ---------------------------------------------------------------------------

func TestFormatWorkflowDryRun_CycleDetection(t *testing.T) {
	t.Parallel()

	def := GetDefinition(WorkflowImplementReview)
	require.NotNil(t, def, "implement-review-pr builtin definition must exist")

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)
	got := f.FormatWorkflowDryRun(def, minimalState(), map[string]string{})

	// run_fix -> success -> run_review, and run_review has a lower step number.
	assert.Contains(t, got, "run_fix",
		"run_fix step must appear in the output")
	assert.Contains(t, got, "cycles back to step",
		"a cycle in the workflow graph must be annotated with 'cycles back to step'")

	// BFS visits run_review (step 2) before run_fix (step 4); the cycle note
	// must reference run_review.
	assert.Contains(t, got, "run_review",
		"the cycled-back target step name (run_review) must appear in the output")

	// Verify BFS order: run_review must appear before run_fix in the output.
	idxReview := strings.Index(got, "run_review")
	idxFix := strings.Index(got, "run_fix")
	assert.True(t, idxReview < idxFix,
		"run_review must appear before run_fix in BFS order (got idxReview=%d, idxFix=%d)",
		idxReview, idxFix)
}

func TestFormatWorkflowDryRun_CycleDetection_PipelineWorkflow(t *testing.T) {
	t.Parallel()

	// The pipeline workflow has advance_phase -> partial -> init_phase (cycle).
	def := GetDefinition(WorkflowPipeline)
	require.NotNil(t, def)

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)
	got := f.FormatWorkflowDryRun(def, minimalState(), map[string]string{})

	assert.Contains(t, got, "cycles back to step",
		"pipeline workflow cycle (advance_phase->init_phase) must be annotated")
	assert.Contains(t, got, "init_phase",
		"init_phase must appear as both a step and the cycle target")
}

// ---------------------------------------------------------------------------
// FormatWorkflowDryRun -- styled vs plain
// ---------------------------------------------------------------------------

func TestFormatWorkflowDryRun_StyledVsPlain(t *testing.T) {
	t.Parallel()

	def := minimalSingleStepDef()
	state := minimalState()
	stepOutputs := map[string]string{"run_implement": "implementation step"}

	var plainBuf, styledBuf bytes.Buffer
	plain := NewDryRunFormatter(&plainBuf, false)
	styled := NewDryRunFormatter(&styledBuf, true)

	plainOut := plain.FormatWorkflowDryRun(def, state, stepOutputs)
	styledOut := styled.FormatWorkflowDryRun(def, state, stepOutputs)

	// Plain output must NOT contain ANSI escape sequences.
	assert.False(t, strings.Contains(plainOut, "\x1b["),
		"plain (styled=false) output must not contain ANSI escape sequences")

	// Both outputs must contain the same step names.
	assert.Contains(t, plainOut, "run_implement")
	assert.Contains(t, styledOut, "run_implement")

	// Both outputs must contain the workflow name.
	assert.Contains(t, plainOut, "test-workflow")
	assert.Contains(t, styledOut, "test-workflow")
}

// ---------------------------------------------------------------------------
// FormatWorkflowDryRun -- determinism
// ---------------------------------------------------------------------------

func TestFormatWorkflowDryRun_Deterministic(t *testing.T) {
	t.Parallel()

	def := GetDefinition(WorkflowImplementReview)
	require.NotNil(t, def)

	state := minimalState()
	stepOutputs := map[string]string{
		"run_implement": "run the implementation agent",
		"run_review":    "run the review agent",
		"check_review":  "decide if fixes are needed",
		"run_fix":       "apply fixes",
		"create_pr":     "open a pull request",
	}

	var buf1, buf2 bytes.Buffer
	f1 := NewDryRunFormatter(&buf1, false)
	f2 := NewDryRunFormatter(&buf2, false)

	out1 := f1.FormatWorkflowDryRun(def, state, stepOutputs)
	out2 := f2.FormatWorkflowDryRun(def, state, stepOutputs)

	assert.Equal(t, out1, out2,
		"FormatWorkflowDryRun must produce identical output on repeated calls with the same inputs")
}

// ---------------------------------------------------------------------------
// FormatWorkflowDryRun -- multi-step linear workflow (no cycle)
// ---------------------------------------------------------------------------

func TestFormatWorkflowDryRun_MultiStepLinear(t *testing.T) {
	t.Parallel()

	def := &WorkflowDefinition{
		Name:        "linear-workflow",
		Description: "A -> B -> C -> DONE",
		InitialStep: "step_a",
		Steps: []StepDefinition{
			{
				Name: "step_a",
				Transitions: map[string]string{
					EventSuccess: "step_b",
					EventFailure: StepFailed,
				},
			},
			{
				Name: "step_b",
				Transitions: map[string]string{
					EventSuccess: "step_c",
					EventFailure: StepFailed,
				},
			},
			{
				Name: "step_c",
				Transitions: map[string]string{
					EventSuccess: StepDone,
					EventFailure: StepFailed,
				},
			},
		},
	}

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)
	got := f.FormatWorkflowDryRun(def, minimalState(), map[string]string{})

	// All three steps must appear.
	assert.Contains(t, got, "1. step_a")
	assert.Contains(t, got, "2. step_b")
	assert.Contains(t, got, "3. step_c")

	// No cycle annotation expected.
	assert.NotContains(t, got, "cycles back to step",
		"a linear workflow must not contain any cycle annotations")
}

// ---------------------------------------------------------------------------
// FormatPipelineDryRun -- empty pipeline
// ---------------------------------------------------------------------------

func TestFormatPipelineDryRun_Empty(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)
	got := f.FormatPipelineDryRun(PipelineDryRunInfo{
		TotalPhases: 0,
		Phases:      nil,
	})

	assert.Contains(t, got, "Total phases: 0",
		"empty pipeline output must include 'Total phases: 0'")
	assert.Contains(t, got, "Pipeline Dry Run",
		"output must include the top-level header")
}

// ---------------------------------------------------------------------------
// FormatPipelineDryRun -- single phase
// ---------------------------------------------------------------------------

func TestFormatPipelineDryRun_SinglePhase(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)

	got := f.FormatPipelineDryRun(PipelineDryRunInfo{
		TotalPhases: 1,
		Phases: []PhaseDryRunDetail{
			{
				PhaseID:     1,
				PhaseName:   "Foundation",
				BranchName:  "phase/1-foundation",
				BaseBranch:  "main",
				ImplAgent:   "claude",
				ReviewAgent: "codex",
				FixAgent:    "claude",
				Skipped:     nil,
				Steps:       nil,
			},
		},
	})

	assert.Contains(t, got, "Phase 1: Foundation", "phase heading must appear")
	assert.Contains(t, got, "Branch: phase/1-foundation (from main)", "branch info must appear")
	assert.Contains(t, got, "Implementation: claude", "implementation agent must appear")
	assert.Contains(t, got, "Review: codex", "review agent must appear when not skipped")
	assert.Contains(t, got, "Fix: claude", "fix agent must appear when not skipped")
	assert.Contains(t, got, "Total phases: 1", "footer must reflect correct total")
}

// ---------------------------------------------------------------------------
// FormatPipelineDryRun -- skipped stages
// ---------------------------------------------------------------------------

func TestFormatPipelineDryRun_SkippedStages(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)

	got := f.FormatPipelineDryRun(PipelineDryRunInfo{
		TotalPhases: 1,
		Phases: []PhaseDryRunDetail{
			{
				PhaseID:     1,
				PhaseName:   "Solo Impl",
				BranchName:  "phase/1-solo",
				BaseBranch:  "main",
				ImplAgent:   "claude",
				ReviewAgent: "codex",
				FixAgent:    "claude",
				Skipped:     []string{"review", "fix"},
			},
		},
	})

	assert.Contains(t, got, "[SKIPPED: review, fix]",
		"skipped stages must appear in a SKIPPED annotation in the order provided")
	assert.NotContains(t, got, "Review:",
		"Review agent line must be omitted when review is skipped")
	assert.NotContains(t, got, "Fix:",
		"Fix agent line must be omitted when fix is skipped")
	// Implementation is never skipped.
	assert.Contains(t, got, "Implementation: claude")
}

// ---------------------------------------------------------------------------
// FormatPipelineDryRun -- multiple phases with chaining
// ---------------------------------------------------------------------------

func TestFormatPipelineDryRun_MultiplePhases(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)

	got := f.FormatPipelineDryRun(PipelineDryRunInfo{
		TotalPhases: 2,
		Phases: []PhaseDryRunDetail{
			{
				PhaseID:     1,
				PhaseName:   "Foundation",
				BranchName:  "phase/1-foundation",
				BaseBranch:  "main",
				ImplAgent:   "claude",
				ReviewAgent: "codex",
				FixAgent:    "claude",
			},
			{
				PhaseID:     2,
				PhaseName:   "Core Features",
				BranchName:  "phase/2-core",
				BaseBranch:  "phase/1-foundation",
				ImplAgent:   "claude",
				ReviewAgent: "gemini",
				FixAgent:    "claude",
			},
		},
	})

	assert.Contains(t, got, "Phase 1: Foundation")
	assert.Contains(t, got, "Phase 2: Core Features")
	// Second phase branches from the first phase's branch.
	assert.Contains(t, got, "Branch: phase/2-core (from phase/1-foundation)",
		"second phase must show it branches from the first phase")
	assert.Contains(t, got, "Total phases: 2")
}

// ---------------------------------------------------------------------------
// FormatPipelineDryRun -- steps within a phase
// ---------------------------------------------------------------------------

func TestFormatPipelineDryRun_WithSteps(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)

	got := f.FormatPipelineDryRun(PipelineDryRunInfo{
		TotalPhases: 1,
		Phases: []PhaseDryRunDetail{
			{
				PhaseID:     1,
				PhaseName:   "Foundation",
				BranchName:  "phase/1-foundation",
				BaseBranch:  "main",
				ImplAgent:   "claude",
				ReviewAgent: "codex",
				FixAgent:    "claude",
				Steps: []StepDryRunDetail{
					{
						StepName:    "run_implement",
						Description: "run the implementation agent",
						Transitions: map[string]string{
							EventSuccess: "run_review",
							EventFailure: StepFailed,
						},
					},
					{
						StepName:    "run_review",
						Description: "run the review agent",
						Transitions: map[string]string{
							EventSuccess: StepDone,
							EventFailure: StepFailed,
						},
					},
				},
			},
		},
	})

	assert.Contains(t, got, "Steps:", "Steps section must appear when steps are provided")
	assert.Contains(t, got, "run_implement", "first step name must appear")
	assert.Contains(t, got, "run_review", "second step name must appear")
	assert.Contains(t, got, "run the implementation agent", "step description must appear")
	assert.Contains(t, got, "run the review agent", "step description must appear")
}

func TestFormatPipelineDryRun_StepsTransitions(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)

	got := f.FormatPipelineDryRun(PipelineDryRunInfo{
		TotalPhases: 1,
		Phases: []PhaseDryRunDetail{
			{
				PhaseID:    1,
				PhaseName:  "Foundation",
				BranchName: "phase/1-foundation",
				BaseBranch: "main",
				ImplAgent:  "claude",
				Steps: []StepDryRunDetail{
					{
						StepName:    "step_a",
						Description: "first step",
						Transitions: map[string]string{
							EventSuccess: StepDone,
							EventFailure: StepFailed,
						},
					},
				},
			},
		},
	})

	assert.Contains(t, got, "-> success: DONE")
	assert.Contains(t, got, "-> failure: FAILED")
}

func TestFormatPipelineDryRun_StepsFallbackDescription(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)

	got := f.FormatPipelineDryRun(PipelineDryRunInfo{
		TotalPhases: 1,
		Phases: []PhaseDryRunDetail{
			{
				PhaseID:    1,
				PhaseName:  "Foundation",
				BranchName: "phase/1-foundation",
				BaseBranch: "main",
				ImplAgent:  "claude",
				Steps: []StepDryRunDetail{
					{StepName: "run_implement", Description: ""},
				},
			},
		},
	})

	assert.Contains(t, got, "step 1",
		"empty description in step must fall back to 'step N'")
}

// ---------------------------------------------------------------------------
// FormatPipelineDryRun -- all stages skipped (except implementation)
// ---------------------------------------------------------------------------

func TestFormatPipelineDryRun_AllSkipped(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)

	got := f.FormatPipelineDryRun(PipelineDryRunInfo{
		TotalPhases: 1,
		Phases: []PhaseDryRunDetail{
			{
				PhaseID:     1,
				PhaseName:   "Impl Only",
				BranchName:  "phase/1-impl",
				BaseBranch:  "main",
				ImplAgent:   "claude",
				ReviewAgent: "codex",
				FixAgent:    "claude",
				Skipped:     []string{"review", "fix", "pr"},
			},
		},
	})

	// The SKIPPED annotation must list all three in the order provided.
	assert.Contains(t, got, "[SKIPPED: review, fix, pr]",
		"all skipped stages must appear in the annotation")

	// Implementation line is always emitted.
	assert.Contains(t, got, "Implementation: claude")

	// Review and Fix lines must be absent.
	assert.NotContains(t, got, "Review:")
	assert.NotContains(t, got, "Fix:")
}

// ---------------------------------------------------------------------------
// FormatPipelineDryRun -- determinism
// ---------------------------------------------------------------------------

func TestFormatPipelineDryRun_Deterministic(t *testing.T) {
	t.Parallel()

	info := PipelineDryRunInfo{
		TotalPhases: 2,
		Phases: []PhaseDryRunDetail{
			{
				PhaseID:     1,
				PhaseName:   "Foundation",
				BranchName:  "phase/1-foundation",
				BaseBranch:  "main",
				ImplAgent:   "claude",
				ReviewAgent: "codex",
				FixAgent:    "claude",
				Steps: []StepDryRunDetail{
					{
						StepName:    "run_implement",
						Description: "implementation step",
						Transitions: map[string]string{
							EventSuccess: StepDone,
							EventFailure: StepFailed,
						},
					},
				},
			},
			{
				PhaseID:     2,
				PhaseName:   "Core Features",
				BranchName:  "phase/2-core",
				BaseBranch:  "phase/1-foundation",
				ImplAgent:   "claude",
				ReviewAgent: "gemini",
				FixAgent:    "claude",
			},
		},
	}

	var buf1, buf2 bytes.Buffer
	f1 := NewDryRunFormatter(&buf1, false)
	f2 := NewDryRunFormatter(&buf2, false)

	out1 := f1.FormatPipelineDryRun(info)
	out2 := f2.FormatPipelineDryRun(info)

	assert.Equal(t, out1, out2,
		"FormatPipelineDryRun must produce identical output on repeated calls with the same inputs")
}

// ---------------------------------------------------------------------------
// FormatPipelineDryRun -- styled vs plain
// ---------------------------------------------------------------------------

func TestFormatPipelineDryRun_PlainHasNoANSI(t *testing.T) {
	t.Parallel()

	info := PipelineDryRunInfo{
		TotalPhases: 1,
		Phases: []PhaseDryRunDetail{
			{
				PhaseID:     1,
				PhaseName:   "Foundation",
				BranchName:  "phase/1-foundation",
				BaseBranch:  "main",
				ImplAgent:   "claude",
				ReviewAgent: "codex",
				FixAgent:    "claude",
				Skipped:     []string{"fix"},
			},
		},
	}

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)
	got := f.FormatPipelineDryRun(info)

	assert.False(t, strings.Contains(got, "\x1b["),
		"plain (styled=false) pipeline output must not contain ANSI escape sequences")
}

// ---------------------------------------------------------------------------
// FormatPipelineDryRun -- cycle detection in step transitions
// ---------------------------------------------------------------------------

func TestFormatPipelineDryRun_StepCycleAnnotation(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)

	// Construct a phase whose steps form a cycle: step_a -> step_b -> step_a.
	got := f.FormatPipelineDryRun(PipelineDryRunInfo{
		TotalPhases: 1,
		Phases: []PhaseDryRunDetail{
			{
				PhaseID:    1,
				PhaseName:  "Cyclic",
				BranchName: "phase/1-cyclic",
				BaseBranch: "main",
				ImplAgent:  "claude",
				Steps: []StepDryRunDetail{
					{
						StepName:    "step_a",
						Description: "first step",
						Transitions: map[string]string{
							EventSuccess: "step_b",
						},
					},
					{
						StepName:    "step_b",
						Description: "second step",
						Transitions: map[string]string{
							EventSuccess: "step_a", // cycle back to step 1
						},
					},
				},
			},
		},
	})

	assert.Contains(t, got, "cycles back to step 1",
		"a cycle in pipeline step transitions must be annotated with 'cycles back to step N'")
}

// ---------------------------------------------------------------------------
// NewDryRunFormatter -- constructor
// ---------------------------------------------------------------------------

func TestNewDryRunFormatter_NotNil(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)
	require.NotNil(t, f, "NewDryRunFormatter must not return nil")
}

func TestNewDryRunFormatter_WritesToProvidedWriter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	f := NewDryRunFormatter(&buf, false)
	f.Write("sentinel")
	assert.Equal(t, "sentinel", buf.String(),
		"Write must forward to the io.Writer provided to NewDryRunFormatter")
}
