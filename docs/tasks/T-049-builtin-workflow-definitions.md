# T-049: Built-in Workflow Definitions and Step Handlers

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Large: 14-20hrs |
| Dependencies | T-043, T-044, T-045, T-048, T-027, T-029 |
| Blocked By | T-045, T-048 |
| Blocks | T-050, T-055 |

## Goal
Implement the four built-in workflow definitions and their corresponding step handlers that ship with Raven. These workflows compose existing subsystem capabilities (implementation loop, review pipeline, fix engine, PR creation, PRD decomposition) into declarative state machine graphs that the workflow engine can execute. This task transforms Raven's individual features into orchestrated workflows.

## Background
Per PRD Section 5.1, Raven ships with four built-in workflows:
1. **implement** -- single-task or phase-based implementation loop
2. **implement-review-pr** -- implement -> review -> fix cycles -> PR creation
3. **pipeline** -- multi-phase orchestrator (chains implement-review-pr per phase)
4. **prd-decompose** -- PRD -> epics -> parallel task decomposition -> merge

Each workflow is defined as a `WorkflowDefinition` with steps and transitions. Each step name maps to a `StepHandler` registered in the global registry. The step handlers are thin wrappers around existing subsystem logic: `internal/loop/runner.go` (T-027), review pipeline (T-031+), fix engine, PR creation, and PRD decomposition (T-056+).

Per PRD: "Custom workflows can be defined in raven.toml" (v2.0 internal support), but "expose the TOML definition DSL in v2.1 after validating the abstraction." For v2.0, the built-in definitions are hardcoded in Go and validated with the validation logic from T-048.

## Technical Specifications
### Implementation Approach
Create `internal/workflow/builtin.go` with functions that return the four built-in `WorkflowDefinition` structs. Create corresponding step handler implementations in `internal/workflow/handlers/` (one file per workflow family). Each step handler wraps calls to existing subsystem APIs. Register all handlers in the global registry via an `init()` function. The built-in definitions are also registered in a definitions map for lookup by name (used by the resume command).

### Key Components
- **BuiltinDefinitions**: Function returning all four built-in WorkflowDefinition structs
- **ImplementStepHandler**: Wraps the implementation loop runner (T-027)
- **ReviewStepHandler**: Wraps the review orchestrator (T-031+)
- **FixStepHandler**: Wraps the fix engine (T-031+)
- **PRStepHandler**: Wraps PR creation (T-031+)
- **BranchStepHandler**: Wraps git branch creation for pipeline phases
- **ShredStepHandler**: Wraps PRD shredder (T-057)
- **ScatterStepHandler**: Wraps scatter orchestrator (T-059)
- **GatherStepHandler**: Wraps merge/emit phases (T-060+)
- **DefinitionsRegistry**: Map of workflow name to definition, for resume lookup

### API/Interface Contracts
```go
// internal/workflow/builtin.go

// BuiltinDefinitions returns all built-in workflow definitions.
func BuiltinDefinitions() map[string]*WorkflowDefinition

// GetDefinition returns a built-in workflow definition by name.
// Returns nil if not found.
func GetDefinition(name string) *WorkflowDefinition

// RegisterBuiltinHandlers registers all built-in step handlers in the given registry.
func RegisterBuiltinHandlers(registry *Registry)

// Built-in workflow names
const (
    WorkflowImplement       = "implement"
    WorkflowImplementReview = "implement-review-pr"
    WorkflowPipeline        = "pipeline"
    WorkflowPRDDecompose    = "prd-decompose"
)

// --- implement workflow ---
// Steps: run_implement
// Transitions: run_implement -> success -> done, failure -> failed

// --- implement-review-pr workflow ---
// Steps: run_implement, run_review, check_review, run_fix, create_pr
// Transitions:
//   run_implement -> success -> run_review, failure -> failed
//   run_review -> success -> check_review, failure -> failed
//   check_review -> success(approved) -> create_pr, needs_human(changes) -> run_fix
//   run_fix -> success -> run_review (cycle back), failure -> failed
//   create_pr -> success -> done, failure -> failed

// --- pipeline workflow ---
// Steps: init_phase, run_phase_workflow, advance_phase
// This is a meta-workflow that iterates over phases.
// Handled specially: the init_phase handler sets up the current phase's parameters,
// run_phase_workflow delegates to implement-review-pr as a sub-workflow,
// advance_phase checks if more phases remain.

// --- prd-decompose workflow ---
// Steps: shred, scatter, gather
// Transitions: shred -> success -> scatter, scatter -> success -> gather, gather -> success -> done
```

```go
// internal/workflow/handlers.go (step handler implementations)

// ImplementHandler wraps the implementation loop runner.
type ImplementHandler struct {
    loopRunner *loop.Runner       // from T-027
    agent      agent.Agent
    config     *config.Config
}

func (h *ImplementHandler) Name() string { return "run_implement" }
func (h *ImplementHandler) Execute(ctx context.Context, state *WorkflowState) (string, error)
func (h *ImplementHandler) DryRun(state *WorkflowState) string

// ReviewHandler wraps the review orchestrator.
type ReviewHandler struct { /* review dependencies */ }

// FixHandler wraps the fix engine.
type FixHandler struct { /* fix dependencies */ }

// PRHandler wraps PR creation.
type PRHandler struct { /* git/PR dependencies */ }

// CheckReviewHandler examines review results and returns the appropriate event.
type CheckReviewHandler struct{}

// BranchHandler creates/switches git branches for pipeline phases.
type BranchHandler struct { /* git client */ }

// ShredHandler wraps the PRD shredder.
type ShredHandler struct { /* shredder dependencies */ }

// ScatterHandler wraps the scatter orchestrator.
type ScatterHandler struct { /* scatter dependencies */ }

// GatherHandler wraps the merge/emit phases.
type GatherHandler struct { /* gather dependencies */ }
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/workflow (T-043,T-044,T-045) | - | Engine, Registry, types |
| internal/workflow (T-048) | - | Validation for definitions |
| internal/loop (T-027) | - | Implementation loop runner |
| internal/review (T-031+) | - | Review, fix, PR subsystems |
| internal/prd (T-056+) | - | PRD decomposition subsystems |
| internal/agent | - | Agent interface |
| internal/config (T-009) | - | Configuration |
| internal/git (T-015) | - | Git operations |

## Acceptance Criteria
- [ ] Four built-in WorkflowDefinitions are defined: implement, implement-review-pr, pipeline, prd-decompose
- [ ] All definitions pass validation (ValidateDefinition from T-048)
- [ ] All step handlers implement the StepHandler interface
- [ ] ImplementHandler delegates to the implementation loop runner
- [ ] ReviewHandler delegates to the review orchestrator
- [ ] FixHandler delegates to the fix engine
- [ ] PRHandler delegates to PR creation
- [ ] CheckReviewHandler examines review results and returns "success" (approved) or "needs_human" (changes needed)
- [ ] implement-review-pr workflow correctly cycles between review and fix when changes are needed
- [ ] Pipeline workflow iterates over phases using state metadata
- [ ] prd-decompose workflow chains shred -> scatter -> gather
- [ ] RegisterBuiltinHandlers registers all handlers in the registry
- [ ] GetDefinition returns correct definition by name
- [ ] Step handlers use WorkflowState.Metadata to pass data between steps
- [ ] DryRun methods return human-readable descriptions of what each step would do
- [ ] Unit tests achieve 80% coverage (handlers wrap subsystems, deep testing is in subsystem tests)

## Testing Requirements
### Unit Tests
- BuiltinDefinitions returns 4 definitions
- Each definition passes ValidateDefinition with a fully registered registry
- GetDefinition returns correct definition for each name
- GetDefinition returns nil for unknown name
- ImplementHandler.Name() returns "run_implement"
- ImplementHandler.DryRun returns description including task/phase info from state
- CheckReviewHandler returns "success" when metadata indicates approved
- CheckReviewHandler returns "needs_human" when metadata indicates changes needed
- All handlers' Execute methods pass context cancellation correctly
- Step handlers read and write to WorkflowState.Metadata correctly

### Integration Tests
- implement workflow with mock agent: runs loop, transitions to done
- implement-review-pr workflow with mock agent: runs implement, review, check (approved), PR, done
- implement-review-pr workflow with mock agent: runs implement, review, check (changes), fix, review, check (approved), PR, done
- prd-decompose workflow with mock agent: shred, scatter, gather, done

### Edge Cases to Handle
- Review handler returns failure (agent error): workflow transitions to failed
- Fix handler runs maximum cycles: transitions to failed after max retries
- Pipeline with single phase: runs once, transitions to done
- Pipeline with empty phase (no tasks): skips, advances to next phase
- Step handler panic: engine recovers, records error

## Implementation Notes
### Recommended Approach
1. Start with workflow definitions: define the four graphs as Go structs in `builtin.go`
2. Implement step handlers one family at a time: implement handlers first, then review/fix/PR, then pipeline, then PRD
3. Each handler stores its dependencies as struct fields (injected at construction time)
4. Handlers communicate between steps via `WorkflowState.Metadata`:
   - implement step writes: `metadata["tasks_completed"]`, `metadata["last_task_id"]`
   - review step writes: `metadata["review_verdict"]`, `metadata["review_report_path"]`
   - check step reads: `metadata["review_verdict"]` to determine transition
   - fix step reads: `metadata["review_report_path"]`
   - PR step reads: accumulated metadata for PR body generation
5. RegisterBuiltinHandlers creates handler instances with nil dependencies (they are set later by the CLI command when the config and agents are available). Use a `HandlerFactory` pattern or lazy initialization.
6. Alternatively, register handler constructors rather than instances, and build them when the workflow is started

### Potential Pitfalls
- Handler dependency injection: handlers need access to agents, config, git client, etc. These are not available at registry init time. Use either (a) a factory pattern where handlers are constructed per-run, or (b) a `Configure()` method on handlers called before execution
- The pipeline workflow is a meta-workflow -- it needs to iterate over phases. Implement this as a loop within the step handler, not as workflow-level looping (the engine processes one step at a time)
- Review-fix cycle: the transition from check -> fix -> review -> check creates an intentional cycle. Ensure the max iterations guard in the engine (T-045) is set appropriately (or use a cycle counter in metadata)
- Do not hardcode agent names in handlers -- use the agent from config/CLI flags passed via WorkflowState.Metadata

### Security Considerations
- Step handlers invoke external processes (agents, git, gh) -- ensure context cancellation is propagated to all subprocesses
- PR handler invokes `gh pr create` -- ensure no user-controlled strings are injected into shell commands without sanitization

## References
- [PRD Section 5.1 - Built-in workflows](docs/prd/PRD-Raven.md)
- [PRD Section 5.4 - Implementation loop](docs/prd/PRD-Raven.md)
- [PRD Section 5.5 - Review pipeline](docs/prd/PRD-Raven.md)
- [PRD Section 5.9 - Pipeline orchestrator](docs/prd/PRD-Raven.md)
- [PRD Section 5.8 - PRD decomposition](docs/prd/PRD-Raven.md)