# T-048: Workflow Definition Validation

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-043, T-044, T-009 |
| Blocked By | T-043 |
| Blocks | T-049, T-050 |

## Goal
Implement comprehensive validation for workflow definitions, both built-in and user-defined in `raven.toml`. Validation catches structural errors at load time (before execution), including unknown steps, unreachable states, cycles in the transition graph, missing handlers, and invalid transition targets. This provides a fast-fail experience with clear error messages instead of runtime failures during long-running workflows.

## Background
Per PRD Section 5.1, "Workflow definitions in TOML are validated at load time (unknown steps, unreachable states, cycles)." The PRD also notes that custom workflow definitions can be specified in `raven.toml`:

```toml
[workflows.my-custom]
description = "Documentation generation"
steps = ["scan_modules", "generate_docs", "review_docs", "commit"]

[workflows.my-custom.transitions]
scan_modules = { on_success = "generate_docs" }
generate_docs = { on_success = "review_docs", on_error = "failed" }
review_docs = { on_approved = "commit", on_changes = "generate_docs" }
commit = { on_success = "done" }
```

Validation must run during config loading (T-009) and before workflow execution (T-045). The validator produces structured error and warning messages suitable for display by `raven config validate`.

## Technical Specifications
### Implementation Approach
Create validation functions in `internal/workflow/validate.go` that take a `WorkflowDefinition` and optionally a `Registry` (for handler existence checks). Implement graph analysis algorithms: reachability analysis (BFS/DFS from initial step), cycle detection (DFS with coloring), and completeness checks (all steps have handlers, all transition targets exist).

### Key Components
- **ValidateDefinition**: Main validation function returning structured errors and warnings
- **Reachability analysis**: BFS from initial step to find unreachable steps
- **Cycle detection**: DFS with three-color marking (white/gray/black) on the transition graph
- **Handler resolution check**: Verify all step names have registered handlers
- **Transition target check**: Verify all transition targets reference valid steps or terminal pseudo-steps
- **ValidationResult**: Structured result with errors (fatal) and warnings (informational)

### API/Interface Contracts
```go
// internal/workflow/validate.go

// ValidationResult contains the outcome of validating a workflow definition.
type ValidationResult struct {
    Errors   []ValidationIssue // Fatal: workflow cannot execute
    Warnings []ValidationIssue // Non-fatal: workflow may have issues
}

// ValidationIssue describes a single validation problem.
type ValidationIssue struct {
    Code    string // e.g., "UNREACHABLE_STEP", "MISSING_HANDLER", "CYCLE_DETECTED"
    Step    string // step name involved, if applicable
    Message string // human-readable description
}

// IsValid returns true if there are no errors (warnings are OK).
func (r *ValidationResult) IsValid() bool

// String returns a formatted multi-line string of all issues.
func (r *ValidationResult) String() string

// ValidateDefinition checks a workflow definition for structural errors.
// If registry is non-nil, also checks that all steps have registered handlers.
func ValidateDefinition(def *WorkflowDefinition, registry *Registry) *ValidationResult

// ValidateDefinitions checks all workflow definitions (e.g., from config).
func ValidateDefinitions(defs map[string]*WorkflowDefinition, registry *Registry) map[string]*ValidationResult

// Issue code constants
const (
    IssueNoSteps           = "NO_STEPS"
    IssueMissingInitial    = "MISSING_INITIAL_STEP"
    IssueMissingHandler    = "MISSING_HANDLER"
    IssueInvalidTarget     = "INVALID_TRANSITION_TARGET"
    IssueUnreachableStep   = "UNREACHABLE_STEP"
    IssueCycleDetected     = "CYCLE_DETECTED"
    IssueNoTransitions     = "NO_TRANSITIONS"
    IssueDuplicateStep     = "DUPLICATE_STEP_NAME"
    IssueEmptyStepName     = "EMPTY_STEP_NAME"
)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/workflow (T-043) | - | WorkflowDefinition, StepDefinition types |
| internal/workflow (T-044) | - | Registry for handler existence checks |
| fmt | stdlib | Error message formatting |

## Acceptance Criteria
- [ ] Empty workflow definition (no steps) produces `NO_STEPS` error
- [ ] Missing initial step (initial step not in steps list) produces `MISSING_INITIAL_STEP` error
- [ ] Step name not in registry produces `MISSING_HANDLER` error (when registry provided)
- [ ] Transition target referencing nonexistent step produces `INVALID_TRANSITION_TARGET` error
- [ ] Transition to terminal pseudo-steps (StepDone, StepFailed) is valid
- [ ] Unreachable steps (not reachable from initial step) produce `UNREACHABLE_STEP` warning
- [ ] Cycles in transition graph produce `CYCLE_DETECTED` warning (not error -- some workflows intentionally loop)
- [ ] Duplicate step names produce `DUPLICATE_STEP_NAME` error
- [ ] Steps with no transitions produce `NO_TRANSITIONS` warning (may be terminal)
- [ ] ValidateDefinitions validates all definitions from config and returns per-definition results
- [ ] Error messages include step names and are actionable
- [ ] Unit tests achieve 95% coverage

## Testing Requirements
### Unit Tests
- Valid linear workflow: no errors, no warnings
- Valid branching workflow: no errors, no warnings
- Empty definition: NO_STEPS error
- Missing initial step: MISSING_INITIAL_STEP error
- Step with handler not in registry: MISSING_HANDLER error
- Transition to nonexistent step: INVALID_TRANSITION_TARGET error
- Transition to StepDone: valid, no error
- Transition to StepFailed: valid, no error
- Unreachable step: UNREACHABLE_STEP warning
- Simple cycle (A -> B -> A): CYCLE_DETECTED warning
- Self-loop (A -> A): CYCLE_DETECTED warning
- Complex cycle (A -> B -> C -> A): CYCLE_DETECTED warning
- Duplicate step names: DUPLICATE_STEP_NAME error
- Step with empty transitions: NO_TRANSITIONS warning
- Validation without registry (nil): skips handler checks
- Multiple issues in same definition: all reported

### Integration Tests
- Parse TOML workflow definition and validate: errors match expected
- Validate built-in workflow definitions: all pass validation

### Edge Cases to Handle
- Very large workflow (100+ steps): performance should be acceptable (<100ms)
- Workflow where all paths from initial step reach terminal: no warnings
- Step that is both reachable and part of a cycle (valid -- review-fix loop)
- Transition map with event keys that match reserved constants

## Implementation Notes
### Recommended Approach
1. Implement `ValidateDefinition` as a sequence of checks:
   a. Basic checks: non-empty steps, initial step exists, no duplicates
   b. Build adjacency list from transitions
   c. Reachability: BFS/DFS from initial step, unreachable steps are warnings
   d. Cycle detection: DFS with coloring on the adjacency list
   e. Handler checks: iterate steps, query registry (if provided)
   f. Target checks: iterate all transitions, verify targets exist in steps or are terminal
2. Cycles should be warnings, not errors, because intentional loops (review-fix cycle) are a valid pattern
3. Return `ValidationResult` with separate error and warning slices
4. `ValidateDefinitions` iterates a map and calls `ValidateDefinition` for each

### Potential Pitfalls
- Cycles vs intentional loops: the review-fix-review pattern is an intentional cycle. Classify cycles as warnings, not errors. The max iterations guard in the engine (T-045) is the safety net.
- Graph analysis must handle steps that transition to terminal pseudo-steps (StepDone, StepFailed) -- these are not in the steps list but are valid targets
- DFS cycle detection must not confuse DAG convergence (diamond) with cycles -- use three-color marking (white = unvisited, gray = in progress, black = complete)

### Security Considerations
- Workflow definitions from TOML are user-provided -- validation prevents execution of malformed definitions
- Ensure validation does not execute any step handlers (static analysis only)

## References
- [PRD Section 5.1 - Workflow definitions validated at load time](docs/prd/PRD-Raven.md)
- [Cycle detection with DFS coloring](https://en.wikipedia.org/wiki/Cycle_(graph_theory)#Cycle_detection)
- [Go graph algorithms - Kahn's algorithm](https://en.wikipedia.org/wiki/Topological_sorting#Kahn's_algorithm)