package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Workflow name constants
// ---------------------------------------------------------------------------

func TestBuiltinWorkflowNameConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "implement", WorkflowImplement)
	assert.Equal(t, "implement-review-pr", WorkflowImplementReview)
	assert.Equal(t, "pipeline", WorkflowPipeline)
	assert.Equal(t, "prd-decompose", WorkflowPRDDecompose)
}

// ---------------------------------------------------------------------------
// BuiltinDefinitions
// ---------------------------------------------------------------------------

func TestBuiltinDefinitions_ReturnsAllFour(t *testing.T) {
	t.Parallel()

	defs := BuiltinDefinitions()
	require.Len(t, defs, 4, "expected exactly four built-in workflow definitions")

	assert.Contains(t, defs, WorkflowImplement)
	assert.Contains(t, defs, WorkflowImplementReview)
	assert.Contains(t, defs, WorkflowPipeline)
	assert.Contains(t, defs, WorkflowPRDDecompose)
}

func TestBuiltinDefinitions_ReturnsCopy(t *testing.T) {
	t.Parallel()

	defs1 := BuiltinDefinitions()
	defs2 := BuiltinDefinitions()

	// Maps must be distinct objects (shallow copy), but values can be shared.
	assert.NotSame(t, &defs1, &defs2,
		"BuiltinDefinitions must return a new map on each call")
}

func TestBuiltinDefinitions_AllValid(t *testing.T) {
	t.Parallel()

	defs := BuiltinDefinitions()
	for name, def := range defs {
		name, def := name, def
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result := ValidateDefinition(def, nil)
			require.NotNil(t, result)
			assert.True(t, result.IsValid(),
				"built-in workflow %q must be structurally valid; errors: %v", name, result.Errors)
		})
	}
}

// ---------------------------------------------------------------------------
// GetDefinition
// ---------------------------------------------------------------------------

func TestGetDefinition_ReturnsKnownWorkflow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: WorkflowImplement, want: WorkflowImplement},
		{name: WorkflowImplementReview, want: WorkflowImplementReview},
		{name: WorkflowPipeline, want: WorkflowPipeline},
		{name: WorkflowPRDDecompose, want: WorkflowPRDDecompose},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			def := GetDefinition(tt.name)
			require.NotNil(t, def, "GetDefinition(%q) must not return nil", tt.name)
			assert.Equal(t, tt.want, def.Name)
		})
	}
}

func TestGetDefinition_ReturnsNilForUnknown(t *testing.T) {
	t.Parallel()

	def := GetDefinition("no-such-workflow")
	assert.Nil(t, def, "GetDefinition with unknown name must return nil")
}

func TestGetDefinition_ReturnsNilForEmptyString(t *testing.T) {
	t.Parallel()

	def := GetDefinition("")
	assert.Nil(t, def, "GetDefinition with empty string must return nil")
}

// ---------------------------------------------------------------------------
// Individual workflow definition shapes
// ---------------------------------------------------------------------------

func TestImplementWorkflow_Shape(t *testing.T) {
	t.Parallel()

	def := GetDefinition(WorkflowImplement)
	require.NotNil(t, def)

	assert.Equal(t, "run_implement", def.InitialStep)
	require.Len(t, def.Steps, 1)
	assert.Equal(t, "run_implement", def.Steps[0].Name)
	assert.Equal(t, StepDone, def.Steps[0].Transitions[EventSuccess])
	assert.Equal(t, StepFailed, def.Steps[0].Transitions[EventFailure])
}

func TestImplementReviewWorkflow_Shape(t *testing.T) {
	t.Parallel()

	def := GetDefinition(WorkflowImplementReview)
	require.NotNil(t, def)

	assert.Equal(t, "run_implement", def.InitialStep)
	require.Len(t, def.Steps, 5)

	// Build a name→transitions index for clean assertions.
	byName := make(map[string]map[string]string, len(def.Steps))
	for _, s := range def.Steps {
		byName[s.Name] = s.Transitions
	}

	assert.Equal(t, "run_review", byName["run_implement"][EventSuccess])
	assert.Equal(t, StepFailed, byName["run_implement"][EventFailure])

	assert.Equal(t, "check_review", byName["run_review"][EventSuccess])
	assert.Equal(t, StepFailed, byName["run_review"][EventFailure])

	assert.Equal(t, "create_pr", byName["check_review"][EventSuccess])
	assert.Equal(t, "run_fix", byName["check_review"][EventNeedsHuman])
	assert.Equal(t, StepFailed, byName["check_review"][EventFailure])

	assert.Equal(t, "run_review", byName["run_fix"][EventSuccess])
	assert.Equal(t, StepFailed, byName["run_fix"][EventFailure])

	assert.Equal(t, StepDone, byName["create_pr"][EventSuccess])
	assert.Equal(t, StepFailed, byName["create_pr"][EventFailure])
}

func TestPipelineWorkflow_Shape(t *testing.T) {
	t.Parallel()

	def := GetDefinition(WorkflowPipeline)
	require.NotNil(t, def)

	assert.Equal(t, "init_phase", def.InitialStep)
	require.Len(t, def.Steps, 3)

	byName := make(map[string]map[string]string, len(def.Steps))
	for _, s := range def.Steps {
		byName[s.Name] = s.Transitions
	}

	assert.Equal(t, "run_phase_workflow", byName["init_phase"][EventSuccess])
	assert.Equal(t, StepFailed, byName["init_phase"][EventFailure])

	assert.Equal(t, "advance_phase", byName["run_phase_workflow"][EventSuccess])
	assert.Equal(t, StepFailed, byName["run_phase_workflow"][EventFailure])

	assert.Equal(t, StepDone, byName["advance_phase"][EventSuccess])
	assert.Equal(t, "init_phase", byName["advance_phase"][EventPartial])
	assert.Equal(t, StepFailed, byName["advance_phase"][EventFailure])
}

func TestPRDDecomposeWorkflow_Shape(t *testing.T) {
	t.Parallel()

	def := GetDefinition(WorkflowPRDDecompose)
	require.NotNil(t, def)

	assert.Equal(t, "shred", def.InitialStep)
	require.Len(t, def.Steps, 3)

	byName := make(map[string]map[string]string, len(def.Steps))
	for _, s := range def.Steps {
		byName[s.Name] = s.Transitions
	}

	assert.Equal(t, "scatter", byName["shred"][EventSuccess])
	assert.Equal(t, StepFailed, byName["shred"][EventFailure])

	assert.Equal(t, "gather", byName["scatter"][EventSuccess])
	assert.Equal(t, StepFailed, byName["scatter"][EventFailure])

	assert.Equal(t, StepDone, byName["gather"][EventSuccess])
	assert.Equal(t, StepFailed, byName["gather"][EventFailure])
}

// ---------------------------------------------------------------------------
// RegisterBuiltinHandlers
// ---------------------------------------------------------------------------

func TestRegisterBuiltinHandlers_RegistersAllSteps(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	RegisterBuiltinHandlers(reg, nil)

	wantSteps := []string{
		"run_implement",
		"run_review",
		"check_review",
		"run_fix",
		"create_pr",
		"init_phase",
		"run_phase_workflow",
		"advance_phase",
		"shred",
		"scatter",
		"gather",
	}

	for _, step := range wantSteps {
		assert.True(t, reg.Has(step), "expected handler for step %q to be registered", step)
	}
}

func TestRegisterBuiltinHandlers_AllWorkflowsValidWithRegistry(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	RegisterBuiltinHandlers(reg, nil)

	defs := BuiltinDefinitions()
	for name, def := range defs {
		name, def := name, def
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result := ValidateDefinition(def, reg)
			require.NotNil(t, result)
			assert.True(t, result.IsValid(),
				"workflow %q must be valid with all handlers registered; errors: %v", name, result.Errors)
		})
	}
}

func TestRegisterBuiltinHandlers_PanicsOnDoubleRegistration(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	RegisterBuiltinHandlers(reg, nil)

	assert.Panics(t, func() {
		RegisterBuiltinHandlers(reg, nil)
	}, "registering built-in handlers twice must panic")
}
