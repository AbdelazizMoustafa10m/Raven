package workflow

import (
	"github.com/AbdelazizMoustafa10m/Raven/internal/loop"
	"github.com/AbdelazizMoustafa10m/Raven/internal/review"
)

// Builtin workflow name constants identify the four workflow definitions
// shipped with Raven. Use these constants instead of raw string literals
// wherever a workflow name is required to avoid typos and enable grep-ability.
const (
	// WorkflowImplement runs the implementation loop for a single task or phase.
	WorkflowImplement = "implement"

	// WorkflowImplementReview runs implementation, multi-agent review, optional
	// fix, and pull request creation as a linear pipeline.
	WorkflowImplementReview = "implement-review-pr"

	// WorkflowPipeline orchestrates a multi-phase project pipeline, advancing
	// through phases until all work is complete.
	WorkflowPipeline = "pipeline"

	// WorkflowPRDDecompose decomposes a PRD document into actionable task files
	// via shred, scatter, and gather phases.
	WorkflowPRDDecompose = "prd-decompose"
)

// builtinDefs holds all four built-in workflow definitions, initialized once
// at package startup by buildBuiltinDefs.
var builtinDefs map[string]*WorkflowDefinition

func init() {
	builtinDefs = buildBuiltinDefs()
}

// buildBuiltinDefs constructs the four canonical workflow definitions and
// returns them as a name-keyed map. It is called exactly once from init().
func buildBuiltinDefs() map[string]*WorkflowDefinition {
	defs := make(map[string]*WorkflowDefinition, 4)

	// ------------------------------------------------------------------
	// implement: single-step implementation loop.
	// ------------------------------------------------------------------
	defs[WorkflowImplement] = &WorkflowDefinition{
		Name:        WorkflowImplement,
		Description: "Run the implementation loop for a single task or phase.",
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

	// ------------------------------------------------------------------
	// implement-review-pr: full implement → review → fix → PR pipeline.
	// ------------------------------------------------------------------
	defs[WorkflowImplementReview] = &WorkflowDefinition{
		Name:        WorkflowImplementReview,
		Description: "Run implementation, multi-agent review, optional fix, and pull request creation.",
		InitialStep: "run_implement",
		Steps: []StepDefinition{
			{
				Name: "run_implement",
				Transitions: map[string]string{
					EventSuccess: "run_review",
					EventFailure: StepFailed,
				},
			},
			{
				Name: "run_review",
				Transitions: map[string]string{
					EventSuccess: "check_review",
					EventFailure: StepFailed,
				},
			},
			{
				Name: "check_review",
				Transitions: map[string]string{
					EventSuccess:    "create_pr",
					EventNeedsHuman: "run_fix",
					EventFailure:    StepFailed,
				},
			},
			{
				Name: "run_fix",
				Transitions: map[string]string{
					EventSuccess: "run_review",
					EventFailure: StepFailed,
				},
			},
			{
				Name: "create_pr",
				Transitions: map[string]string{
					EventSuccess: StepDone,
					EventFailure: StepFailed,
				},
			},
		},
	}

	// ------------------------------------------------------------------
	// pipeline: multi-phase project pipeline.
	// ------------------------------------------------------------------
	defs[WorkflowPipeline] = &WorkflowDefinition{
		Name:        WorkflowPipeline,
		Description: "Orchestrate a multi-phase project pipeline, advancing phases until all work is complete.",
		InitialStep: "init_phase",
		Steps: []StepDefinition{
			{
				Name: "init_phase",
				Transitions: map[string]string{
					EventSuccess: "run_phase_workflow",
					EventFailure: StepFailed,
				},
			},
			{
				Name: "run_phase_workflow",
				Transitions: map[string]string{
					EventSuccess: "advance_phase",
					EventFailure: StepFailed,
				},
			},
			{
				Name: "advance_phase",
				Transitions: map[string]string{
					EventSuccess: StepDone,
					EventPartial: "init_phase",
					EventFailure: StepFailed,
				},
			},
		},
	}

	// ------------------------------------------------------------------
	// prd-decompose: shred → scatter → gather decomposition pipeline.
	// ------------------------------------------------------------------
	defs[WorkflowPRDDecompose] = &WorkflowDefinition{
		Name:        WorkflowPRDDecompose,
		Description: "Decompose a PRD document into actionable task files via shred, scatter, and gather phases.",
		InitialStep: "shred",
		Steps: []StepDefinition{
			{
				Name: "shred",
				Transitions: map[string]string{
					EventSuccess: "scatter",
					EventFailure: StepFailed,
				},
			},
			{
				Name: "scatter",
				Transitions: map[string]string{
					EventSuccess: "gather",
					EventFailure: StepFailed,
				},
			},
			{
				Name: "gather",
				Transitions: map[string]string{
					EventSuccess: StepDone,
					EventFailure: StepFailed,
				},
			},
		},
	}

	return defs
}

// BuiltinDefinitions returns all four built-in workflow definitions as a
// name-keyed map. The returned map is a shallow copy; callers must not modify
// the WorkflowDefinition values it contains.
func BuiltinDefinitions() map[string]*WorkflowDefinition {
	out := make(map[string]*WorkflowDefinition, len(builtinDefs))
	for k, v := range builtinDefs {
		out[k] = v
	}
	return out
}

// GetDefinition returns the built-in WorkflowDefinition for the given name, or
// nil if name does not correspond to a known built-in workflow.
func GetDefinition(name string) *WorkflowDefinition {
	return builtinDefs[name]
}

// HandlerDeps holds the runtime dependencies that built-in step handlers need
// to perform real work. Fields may be nil when the handlers are registered
// solely for validation or dry-run purposes; in that case Execute returns
// EventFailure with a descriptive error.
type HandlerDeps struct {
	// Runner is the implementation loop runner used by ImplementHandler and
	// RunPhaseWorkflowHandler.
	Runner *loop.Runner

	// RunConfig provides default values for the ImplementHandler when values
	// are not present in the workflow state metadata.
	RunConfig loop.RunConfig

	// ReviewOrchestrator is the multi-agent review coordinator used by
	// ReviewHandler.
	ReviewOrchestrator *review.ReviewOrchestrator

	// FixEngine is the review fix engine used by FixHandler.
	FixEngine *review.FixEngine

	// PRCreator is the GitHub PR creation helper used by PRHandler.
	PRCreator *review.PRCreator
}

// RegisterBuiltinHandlers registers all built-in step handlers into registry,
// injecting the runtime dependencies from deps. deps may be nil to register
// handlers without runtime dependencies (useful for validation or dry-run);
// in that case handlers that require dependencies return EventFailure when
// Execute is called.
//
// It must be called explicitly before constructing an Engine that will execute
// any of the built-in workflows. The function panics (via Registry.Register) if
// a handler name has already been registered in registry.
func RegisterBuiltinHandlers(registry *Registry, deps *HandlerDeps) {
	if deps == nil {
		deps = &HandlerDeps{}
	}

	registry.Register(&ImplementHandler{
		Runner:    deps.Runner,
		RunConfig: deps.RunConfig,
	})
	registry.Register(&ReviewHandler{
		Orchestrator: deps.ReviewOrchestrator,
	})
	registry.Register(&CheckReviewHandler{})
	registry.Register(&FixHandler{
		Engine: deps.FixEngine,
	})
	registry.Register(&PRHandler{
		Creator: deps.PRCreator,
	})
	registry.Register(&InitPhaseHandler{})
	registry.Register(&RunPhaseWorkflowHandler{
		Runner: deps.Runner,
	})
	registry.Register(&AdvancePhaseHandler{})
	registry.Register(&ShredHandler{})
	registry.Register(&ScatterHandler{})
	registry.Register(&GatherHandler{})
}
