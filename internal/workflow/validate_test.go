package workflow

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// simpleWorkflow builds a minimal valid WorkflowDefinition with the given
// step names. The first step is the initial step; each step transitions to
// the next on EventSuccess, and the last step transitions to StepDone.
func simpleWorkflow(names ...string) *WorkflowDefinition {
	if len(names) == 0 {
		return &WorkflowDefinition{Name: "empty"}
	}
	steps := make([]StepDefinition, len(names))
	for i, name := range names {
		var transitions map[string]string
		if i < len(names)-1 {
			transitions = map[string]string{EventSuccess: names[i+1]}
		} else {
			transitions = map[string]string{EventSuccess: StepDone}
		}
		steps[i] = StepDefinition{
			Name:        name,
			Transitions: transitions,
		}
	}
	return &WorkflowDefinition{
		Name:        "test-workflow",
		InitialStep: names[0],
		Steps:       steps,
	}
}

// registryWith returns a fresh Registry with the given step names registered.
func registryWith(names ...string) *Registry {
	r := NewRegistry()
	for _, n := range names {
		r.Register(newStub(n))
	}
	return r
}

// hasError returns true if result.Errors contains at least one issue with
// the given code (and optionally matching step name when step != "").
func hasError(result *ValidationResult, code, step string) bool {
	for _, issue := range result.Errors {
		if issue.Code == code && (step == "" || issue.Step == step) {
			return true
		}
	}
	return false
}

// hasWarning mirrors hasError but checks result.Warnings.
func hasWarning(result *ValidationResult, code, step string) bool {
	for _, issue := range result.Warnings {
		if issue.Code == code && (step == "" || issue.Step == step) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// ValidationResult helpers
// ---------------------------------------------------------------------------

func TestValidationResult_IsValid_NoErrors(t *testing.T) {
	t.Parallel()

	r := &ValidationResult{}
	assert.True(t, r.IsValid())
}

func TestValidationResult_IsValid_WithWarningOnly(t *testing.T) {
	t.Parallel()

	r := &ValidationResult{
		Warnings: []ValidationIssue{{Code: IssueUnreachableStep, Step: "x", Message: "msg"}},
	}
	assert.True(t, r.IsValid(), "warnings alone must not invalidate the result")
}

func TestValidationResult_IsValid_WithErrors(t *testing.T) {
	t.Parallel()

	r := &ValidationResult{
		Errors: []ValidationIssue{{Code: IssueNoSteps, Message: "no steps"}},
	}
	assert.False(t, r.IsValid())
}

func TestValidationResult_String_ContainsIssues(t *testing.T) {
	t.Parallel()

	r := &ValidationResult{
		Errors: []ValidationIssue{
			{Code: IssueNoSteps, Message: "workflow definition has no steps"},
		},
		Warnings: []ValidationIssue{
			{Code: IssueUnreachableStep, Step: "orphan", Message: "unreachable"},
		},
	}

	s := r.String()
	assert.Contains(t, s, "Errors (1):")
	assert.Contains(t, s, IssueNoSteps)
	assert.Contains(t, s, "Warnings (1):")
	assert.Contains(t, s, IssueUnreachableStep)
	assert.Contains(t, s, `"orphan"`)
}

func TestValidationResult_String_ZeroCounts(t *testing.T) {
	t.Parallel()

	r := &ValidationResult{}
	s := r.String()
	assert.Contains(t, s, "Errors (0):")
	assert.Contains(t, s, "Warnings (0):")
}

func TestValidationResult_String_NoStepField(t *testing.T) {
	t.Parallel()

	// Issues without a Step field must not render empty quotes.
	r := &ValidationResult{
		Errors: []ValidationIssue{
			{Code: IssueNoSteps, Step: "", Message: "no steps"},
		},
	}
	s := r.String()
	assert.NotContains(t, s, `step ""`)
	assert.Contains(t, s, "no steps")
}

// ---------------------------------------------------------------------------
// ValidateDefinition – nil / empty
// ---------------------------------------------------------------------------

func TestValidateDefinition_NilDef(t *testing.T) {
	t.Parallel()

	result := ValidateDefinition(nil, nil)
	require.NotNil(t, result)
	assert.True(t, hasError(result, IssueNoSteps, ""))
}

func TestValidateDefinition_EmptySteps(t *testing.T) {
	t.Parallel()

	def := &WorkflowDefinition{Name: "empty"}
	result := ValidateDefinition(def, nil)
	assert.True(t, hasError(result, IssueNoSteps, ""))
}

// ---------------------------------------------------------------------------
// ValidateDefinition – empty step name
// ---------------------------------------------------------------------------

func TestValidateDefinition_EmptyStepName(t *testing.T) {
	t.Parallel()

	def := &WorkflowDefinition{
		Name:        "w",
		InitialStep: "a",
		Steps: []StepDefinition{
			{Name: "a", Transitions: map[string]string{EventSuccess: StepDone}},
			{Name: "", Transitions: map[string]string{EventSuccess: StepDone}},
		},
	}
	result := ValidateDefinition(def, nil)
	assert.True(t, hasError(result, IssueEmptyStepName, ""))
}

// ---------------------------------------------------------------------------
// ValidateDefinition – duplicate step names
// ---------------------------------------------------------------------------

func TestValidateDefinition_DuplicateStepName(t *testing.T) {
	t.Parallel()

	def := &WorkflowDefinition{
		Name:        "w",
		InitialStep: "a",
		Steps: []StepDefinition{
			{Name: "a", Transitions: map[string]string{EventSuccess: StepDone}},
			{Name: "a", Transitions: map[string]string{EventSuccess: StepDone}},
		},
	}
	result := ValidateDefinition(def, nil)
	assert.True(t, hasError(result, IssueDuplicateStep, "a"))
}

// ---------------------------------------------------------------------------
// ValidateDefinition – missing initial step
// ---------------------------------------------------------------------------

func TestValidateDefinition_MissingInitialStep_Empty(t *testing.T) {
	t.Parallel()

	def := &WorkflowDefinition{
		Name:        "w",
		InitialStep: "",
		Steps: []StepDefinition{
			{Name: "a", Transitions: map[string]string{EventSuccess: StepDone}},
		},
	}
	result := ValidateDefinition(def, nil)
	assert.True(t, hasError(result, IssueMissingInitial, ""))
}

func TestValidateDefinition_MissingInitialStep_NotInList(t *testing.T) {
	t.Parallel()

	def := &WorkflowDefinition{
		Name:        "w",
		InitialStep: "missing",
		Steps: []StepDefinition{
			{Name: "a", Transitions: map[string]string{EventSuccess: StepDone}},
		},
	}
	result := ValidateDefinition(def, nil)
	assert.True(t, hasError(result, IssueMissingInitial, "missing"))
}

// ---------------------------------------------------------------------------
// ValidateDefinition – invalid transition targets
// ---------------------------------------------------------------------------

func TestValidateDefinition_InvalidTransitionTarget(t *testing.T) {
	t.Parallel()

	def := &WorkflowDefinition{
		Name:        "w",
		InitialStep: "a",
		Steps: []StepDefinition{
			{Name: "a", Transitions: map[string]string{EventSuccess: "no-such-step"}},
		},
	}
	result := ValidateDefinition(def, nil)
	assert.True(t, hasError(result, IssueInvalidTarget, "a"))
}

func TestValidateDefinition_TerminalTargetsAlwaysValid(t *testing.T) {
	t.Parallel()

	def := &WorkflowDefinition{
		Name:        "w",
		InitialStep: "a",
		Steps: []StepDefinition{
			{Name: "a", Transitions: map[string]string{
				EventSuccess: StepDone,
				EventFailure: StepFailed,
			}},
		},
	}
	result := ValidateDefinition(def, nil)
	assert.False(t, hasError(result, IssueInvalidTarget, "a"),
		"StepDone and StepFailed must always be valid transition targets")
	assert.True(t, result.IsValid())
}

// ---------------------------------------------------------------------------
// ValidateDefinition – handler checks
// ---------------------------------------------------------------------------

func TestValidateDefinition_MissingHandler(t *testing.T) {
	t.Parallel()

	def := simpleWorkflow("a", "b")
	reg := registryWith("a") // "b" is missing
	result := ValidateDefinition(def, reg)
	assert.True(t, hasError(result, IssueMissingHandler, "b"))
	assert.False(t, hasError(result, IssueMissingHandler, "a"))
}

func TestValidateDefinition_NilRegistry_SkipsHandlerChecks(t *testing.T) {
	t.Parallel()

	def := simpleWorkflow("a", "b")
	result := ValidateDefinition(def, nil)
	assert.False(t, hasError(result, IssueMissingHandler, ""))
}

func TestValidateDefinition_AllHandlersPresent_NoHandlerError(t *testing.T) {
	t.Parallel()

	def := simpleWorkflow("a", "b")
	reg := registryWith("a", "b")
	result := ValidateDefinition(def, reg)
	assert.False(t, hasError(result, IssueMissingHandler, ""))
}

// ---------------------------------------------------------------------------
// ValidateDefinition – reachability
// ---------------------------------------------------------------------------

func TestValidateDefinition_UnreachableStep(t *testing.T) {
	t.Parallel()

	def := &WorkflowDefinition{
		Name:        "w",
		InitialStep: "a",
		Steps: []StepDefinition{
			{Name: "a", Transitions: map[string]string{EventSuccess: StepDone}},
			{Name: "b", Transitions: map[string]string{EventSuccess: StepDone}}, // no path leads here
		},
	}
	result := ValidateDefinition(def, nil)
	assert.True(t, hasWarning(result, IssueUnreachableStep, "b"))
	assert.False(t, hasWarning(result, IssueUnreachableStep, "a"))
}

func TestValidateDefinition_AllStepsReachable_NoWarning(t *testing.T) {
	t.Parallel()

	def := simpleWorkflow("a", "b", "c")
	result := ValidateDefinition(def, nil)
	assert.False(t, hasWarning(result, IssueUnreachableStep, ""))
}

func TestValidateDefinition_DiamondGraph_NotUnreachable(t *testing.T) {
	t.Parallel()

	// a → b, a → c, b → d, c → d, d → done
	def := &WorkflowDefinition{
		Name:        "diamond",
		InitialStep: "a",
		Steps: []StepDefinition{
			{Name: "a", Transitions: map[string]string{"left": "b", "right": "c"}},
			{Name: "b", Transitions: map[string]string{EventSuccess: "d"}},
			{Name: "c", Transitions: map[string]string{EventSuccess: "d"}},
			{Name: "d", Transitions: map[string]string{EventSuccess: StepDone}},
		},
	}
	result := ValidateDefinition(def, nil)
	assert.False(t, hasWarning(result, IssueUnreachableStep, ""),
		"diamond-shaped (convergent) graphs must not produce unreachable warnings")
}

// ---------------------------------------------------------------------------
// ValidateDefinition – cycle detection
// ---------------------------------------------------------------------------

func TestValidateDefinition_CycleDetected_IsWarning(t *testing.T) {
	t.Parallel()

	// a → b → a (direct cycle)
	def := &WorkflowDefinition{
		Name:        "cyclic",
		InitialStep: "a",
		Steps: []StepDefinition{
			{Name: "a", Transitions: map[string]string{EventSuccess: "b", EventFailure: StepFailed}},
			{Name: "b", Transitions: map[string]string{EventSuccess: "a", EventFailure: StepFailed}},
		},
	}
	result := ValidateDefinition(def, nil)
	assert.True(t, hasWarning(result, IssueCycleDetected, ""),
		"a directed cycle must produce a CYCLE_DETECTED warning")
	// Cycles are warnings, not errors.
	assert.False(t, hasError(result, IssueCycleDetected, ""))
}

func TestValidateDefinition_NoCycle_LinearChain(t *testing.T) {
	t.Parallel()

	def := simpleWorkflow("a", "b", "c")
	result := ValidateDefinition(def, nil)
	assert.False(t, hasWarning(result, IssueCycleDetected, ""))
}

func TestValidateDefinition_DiamondGraph_NotACycle(t *testing.T) {
	t.Parallel()

	// Diamond-shaped convergence must NOT be detected as a cycle.
	def := &WorkflowDefinition{
		Name:        "diamond",
		InitialStep: "a",
		Steps: []StepDefinition{
			{Name: "a", Transitions: map[string]string{"left": "b", "right": "c"}},
			{Name: "b", Transitions: map[string]string{EventSuccess: "d"}},
			{Name: "c", Transitions: map[string]string{EventSuccess: "d"}},
			{Name: "d", Transitions: map[string]string{EventSuccess: StepDone}},
		},
	}
	result := ValidateDefinition(def, nil)
	assert.False(t, hasWarning(result, IssueCycleDetected, ""),
		"diamond convergence must not be flagged as a cycle")
}

func TestValidateDefinition_CycleMessage_ContainsSteps(t *testing.T) {
	t.Parallel()

	def := &WorkflowDefinition{
		Name:        "review-loop",
		InitialStep: "review",
		Steps: []StepDefinition{
			{Name: "review", Transitions: map[string]string{"needs_fix": "fix", EventSuccess: StepDone}},
			{Name: "fix", Transitions: map[string]string{EventSuccess: "review", EventFailure: StepFailed}},
		},
	}
	result := ValidateDefinition(def, nil)
	require.True(t, hasWarning(result, IssueCycleDetected, ""))

	// The warning message must mention the involved steps.
	found := false
	for _, issue := range result.Warnings {
		if issue.Code == IssueCycleDetected {
			found = strings.Contains(issue.Message, "review") &&
				strings.Contains(issue.Message, "fix")
			if found {
				break
			}
		}
	}
	assert.True(t, found, "cycle warning message must contain the names of the involved steps")
}

// ---------------------------------------------------------------------------
// ValidateDefinition – no transitions
// ---------------------------------------------------------------------------

func TestValidateDefinition_NoTransitions_Warning(t *testing.T) {
	t.Parallel()

	def := &WorkflowDefinition{
		Name:        "w",
		InitialStep: "a",
		Steps: []StepDefinition{
			{Name: "a", Transitions: map[string]string{EventSuccess: "b"}},
			{Name: "b", Transitions: nil}, // no transitions
		},
	}
	result := ValidateDefinition(def, nil)
	assert.True(t, hasWarning(result, IssueNoTransitions, "b"))
	assert.False(t, hasWarning(result, IssueNoTransitions, "a"))
}

func TestValidateDefinition_AllHaveTransitions_NoWarning(t *testing.T) {
	t.Parallel()

	def := simpleWorkflow("a", "b")
	result := ValidateDefinition(def, nil)
	assert.False(t, hasWarning(result, IssueNoTransitions, ""))
}

// ---------------------------------------------------------------------------
// ValidateDefinition – table-driven comprehensive
// ---------------------------------------------------------------------------

func TestValidateDefinition_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		def          *WorkflowDefinition
		registry     *Registry
		wantValid    bool
		wantErrCodes []string
		wantWarnCodes []string
	}{
		{
			name:         "nil definition",
			def:          nil,
			wantValid:    false,
			wantErrCodes: []string{IssueNoSteps},
		},
		{
			name:         "empty steps slice",
			def:          &WorkflowDefinition{Name: "empty"},
			wantValid:    false,
			wantErrCodes: []string{IssueNoSteps},
		},
		{
			name:      "minimal valid single-step workflow",
			def:       simpleWorkflow("only"),
			wantValid: true,
		},
		{
			name:         "missing initial step",
			def: &WorkflowDefinition{
				Name:        "w",
				InitialStep: "ghost",
				Steps: []StepDefinition{
					{Name: "a", Transitions: map[string]string{EventSuccess: StepDone}},
				},
			},
			wantValid:    false,
			wantErrCodes: []string{IssueMissingInitial},
		},
		{
			name:         "invalid transition target",
			def: &WorkflowDefinition{
				Name:        "w",
				InitialStep: "a",
				Steps: []StepDefinition{
					{Name: "a", Transitions: map[string]string{EventSuccess: "nonexistent"}},
				},
			},
			wantValid:    false,
			wantErrCodes: []string{IssueInvalidTarget},
		},
		{
			name: "step with both done and failed transitions",
			def: &WorkflowDefinition{
				Name:        "w",
				InitialStep: "a",
				Steps: []StepDefinition{
					{Name: "a", Transitions: map[string]string{
						EventSuccess: StepDone,
						EventFailure: StepFailed,
					}},
				},
			},
			wantValid: true,
		},
		{
			name:          "unreachable step produces warning",
			def: &WorkflowDefinition{
				Name:        "w",
				InitialStep: "a",
				Steps: []StepDefinition{
					{Name: "a", Transitions: map[string]string{EventSuccess: StepDone}},
					{Name: "orphan", Transitions: map[string]string{EventSuccess: StepDone}},
				},
			},
			wantValid:     true,
			wantWarnCodes: []string{IssueUnreachableStep},
		},
		{
			name: "three-step linear chain",
			def:  simpleWorkflow("build", "test", "deploy"),
			wantValid: true,
		},
		{
			name: "handler missing with registry",
			def:  simpleWorkflow("a", "b"),
			registry: registryWith("a"),
			wantValid:    false,
			wantErrCodes: []string{IssueMissingHandler},
		},
		{
			name: "all handlers present",
			def:  simpleWorkflow("a", "b"),
			registry: registryWith("a", "b"),
			wantValid: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := ValidateDefinition(tt.def, tt.registry)
			require.NotNil(t, result)

			assert.Equal(t, tt.wantValid, result.IsValid(),
				"IsValid mismatch; errors: %v, warnings: %v", result.Errors, result.Warnings)

			for _, code := range tt.wantErrCodes {
				assert.True(t, hasError(result, code, ""),
					"expected error code %q not found; errors: %v", code, result.Errors)
			}
			for _, code := range tt.wantWarnCodes {
				assert.True(t, hasWarning(result, code, ""),
					"expected warning code %q not found; warnings: %v", code, result.Warnings)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ValidateDefinitions (batch)
// ---------------------------------------------------------------------------

func TestValidateDefinitions_ReturnsResultPerWorkflow(t *testing.T) {
	t.Parallel()

	defs := map[string]*WorkflowDefinition{
		"good":  simpleWorkflow("a", "b"),
		"empty": {Name: "empty"},
	}
	results := ValidateDefinitions(defs, nil)
	require.Len(t, results, 2)

	assert.True(t, results["good"].IsValid())
	assert.False(t, results["empty"].IsValid())
	assert.True(t, hasError(results["empty"], IssueNoSteps, ""))
}

func TestValidateDefinitions_EmptyMap(t *testing.T) {
	t.Parallel()

	results := ValidateDefinitions(map[string]*WorkflowDefinition{}, nil)
	assert.Empty(t, results)
}

func TestValidateDefinitions_NilMap(t *testing.T) {
	t.Parallel()

	results := ValidateDefinitions(nil, nil)
	assert.Empty(t, results)
}

func TestValidateDefinitions_WithRegistry(t *testing.T) {
	t.Parallel()

	defs := map[string]*WorkflowDefinition{
		"w": simpleWorkflow("x"),
	}
	reg := registryWith("x")
	results := ValidateDefinitions(defs, reg)
	require.Contains(t, results, "w")
	assert.True(t, results["w"].IsValid())
}

// ---------------------------------------------------------------------------
// Review-fix cycle: intentional loop is valid (warnings only)
// ---------------------------------------------------------------------------

func TestValidateDefinition_ReviewFixLoop_ValidWithCycleWarning(t *testing.T) {
	t.Parallel()

	def := &WorkflowDefinition{
		Name:        "review-fix-loop",
		InitialStep: "implement",
		Steps: []StepDefinition{
			{Name: "implement", Transitions: map[string]string{EventSuccess: "review", EventFailure: StepFailed}},
			{Name: "review", Transitions: map[string]string{"approved": StepDone, "needs_fix": "fix"}},
			{Name: "fix", Transitions: map[string]string{EventSuccess: "review", EventFailure: StepFailed}},
		},
	}
	result := ValidateDefinition(def, nil)

	// The workflow is structurally valid (no errors).
	assert.True(t, result.IsValid(),
		"a review-fix loop must be valid; errors: %v", result.Errors)

	// But a cycle warning should be present.
	assert.True(t, hasWarning(result, IssueCycleDetected, ""),
		"intentional review-fix loop must produce a CYCLE_DETECTED warning")
}
