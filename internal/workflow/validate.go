package workflow

import (
	"fmt"
	"strings"
)

// Issue code constants classify each ValidationIssue by its structural category.
// Codes are stable strings so callers can switch on them without importing
// numeric iota values.
const (
	// IssueNoSteps is reported when a WorkflowDefinition has an empty Steps
	// slice; the engine cannot execute a workflow with no steps.
	IssueNoSteps = "NO_STEPS"

	// IssueMissingInitial is reported when InitialStep does not match any
	// step name in the Steps list.
	IssueMissingInitial = "MISSING_INITIAL_STEP"

	// IssueMissingHandler is reported (only when a Registry is provided) when
	// a step name has no registered StepHandler.
	IssueMissingHandler = "MISSING_HANDLER"

	// IssueInvalidTarget is reported when a transition target is neither a
	// defined step name nor one of the terminal pseudo-steps (StepDone /
	// StepFailed).
	IssueInvalidTarget = "INVALID_TRANSITION_TARGET"

	// IssueUnreachableStep is reported when a step cannot be reached via any
	// transition path starting from InitialStep.
	IssueUnreachableStep = "UNREACHABLE_STEP"

	// IssueCycleDetected is reported when the transition graph contains a
	// directed cycle. Cycles are warnings, not errors, because intentional
	// loops (e.g. review → fix → review) are common and valid.
	IssueCycleDetected = "CYCLE_DETECTED"

	// IssueNoTransitions is reported when a non-terminal step has an empty
	// Transitions map; the engine would stall at that step.
	IssueNoTransitions = "NO_TRANSITIONS"

	// IssueDuplicateStep is reported when two or more steps share the same
	// Name within a single WorkflowDefinition.
	IssueDuplicateStep = "DUPLICATE_STEP_NAME"

	// IssueEmptyStepName is reported when a step has an empty Name field.
	IssueEmptyStepName = "EMPTY_STEP_NAME"
)

// ValidationIssue describes a single structural problem found in a
// WorkflowDefinition. Issues with a non-empty Step field are associated with
// a specific step; others are definition-level concerns.
type ValidationIssue struct {
	// Code is one of the Issue* constants identifying the problem category.
	Code string

	// Step is the name of the step involved in the issue, or empty string for
	// definition-level issues (e.g. IssueNoSteps, IssueMissingInitial).
	Step string

	// Message is a human-readable description of the problem.
	Message string
}

// ValidationResult holds the outcome of validating a single WorkflowDefinition.
// Errors are fatal: the workflow cannot execute. Warnings are non-fatal: the
// workflow may run but could behave unexpectedly.
type ValidationResult struct {
	// Errors contains fatal issues that prevent workflow execution.
	Errors []ValidationIssue

	// Warnings contains non-fatal issues that may indicate design problems.
	Warnings []ValidationIssue
}

// IsValid reports whether the definition has no errors. Warnings alone do not
// make a definition invalid.
func (r *ValidationResult) IsValid() bool {
	return len(r.Errors) == 0
}

// String returns a multi-line human-readable summary of all validation issues.
// The format is:
//
//	Errors (N):
//	  [ERROR_CODE] step "stepname": message
//	Warnings (N):
//	  [WARN_CODE] step "stepname": message
func (r *ValidationResult) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "Errors (%d):\n", len(r.Errors))
	for _, issue := range r.Errors {
		if issue.Step != "" {
			fmt.Fprintf(&b, "  [%s] step %q: %s\n", issue.Code, issue.Step, issue.Message)
		} else {
			fmt.Fprintf(&b, "  [%s] %s\n", issue.Code, issue.Message)
		}
	}

	fmt.Fprintf(&b, "Warnings (%d):\n", len(r.Warnings))
	for _, issue := range r.Warnings {
		if issue.Step != "" {
			fmt.Fprintf(&b, "  [%s] step %q: %s\n", issue.Code, issue.Step, issue.Message)
		} else {
			fmt.Fprintf(&b, "  [%s] %s\n", issue.Code, issue.Message)
		}
	}

	return b.String()
}

// ValidateDefinition checks a workflow definition for structural errors and
// design warnings. If registry is non-nil, handler registration is also
// verified. The function always returns a non-nil ValidationResult.
//
// Validation sequence:
//  1. Basic checks: empty steps, empty step names, duplicate names, missing
//     initial step.
//  2. Transition target checks: all targets must be defined steps or terminal
//     pseudo-steps (StepDone / StepFailed).
//  3. Handler checks (only when registry != nil): every step must have a
//     registered handler.
//  4. Reachability: BFS from InitialStep; unreachable steps produce warnings.
//  5. Cycle detection: DFS three-color marking; cycles produce warnings (not
//     errors) because intentional loops are valid.
//  6. No-transitions: steps with empty transition maps (that are not already
//     terminal) produce warnings.
func ValidateDefinition(def *WorkflowDefinition, registry *Registry) *ValidationResult {
	result := &ValidationResult{}

	// Gracefully handle nil definition.
	if def == nil || len(def.Steps) == 0 {
		result.Errors = append(result.Errors, ValidationIssue{
			Code:    IssueNoSteps,
			Message: "workflow definition has no steps",
		})
		return result
	}

	// -----------------------------------------------------------------------
	// Phase 1: Basic checks
	// -----------------------------------------------------------------------

	// Detect empty step names and build the name→index map simultaneously.
	stepIndex := make(map[string]int, len(def.Steps)) // name → position
	for i, sd := range def.Steps {
		if sd.Name == "" {
			result.Errors = append(result.Errors, ValidationIssue{
				Code:    IssueEmptyStepName,
				Message: fmt.Sprintf("step at index %d has an empty name", i),
			})
			continue
		}
		if _, exists := stepIndex[sd.Name]; exists {
			result.Errors = append(result.Errors, ValidationIssue{
				Code:    IssueDuplicateStep,
				Step:    sd.Name,
				Message: fmt.Sprintf("step name %q appears more than once", sd.Name),
			})
			continue
		}
		stepIndex[sd.Name] = i
	}

	// Build the set of valid transition targets (user steps + terminals).
	validTargets := make(map[string]struct{}, len(stepIndex)+2)
	for name := range stepIndex {
		validTargets[name] = struct{}{}
	}
	validTargets[StepDone] = struct{}{}
	validTargets[StepFailed] = struct{}{}

	// Check that InitialStep is in the step list.
	if def.InitialStep == "" {
		result.Errors = append(result.Errors, ValidationIssue{
			Code:    IssueMissingInitial,
			Message: "initial_step is empty; must reference a defined step",
		})
	} else if _, ok := stepIndex[def.InitialStep]; !ok {
		result.Errors = append(result.Errors, ValidationIssue{
			Code:    IssueMissingInitial,
			Step:    def.InitialStep,
			Message: fmt.Sprintf("initial_step %q is not defined in the steps list", def.InitialStep),
		})
	}

	// -----------------------------------------------------------------------
	// Phase 2: Transition target checks
	// -----------------------------------------------------------------------

	for _, sd := range def.Steps {
		if sd.Name == "" {
			continue // already flagged above; skip transition checks
		}
		for event, target := range sd.Transitions {
			if _, ok := validTargets[target]; !ok {
				result.Errors = append(result.Errors, ValidationIssue{
					Code:    IssueInvalidTarget,
					Step:    sd.Name,
					Message: fmt.Sprintf("transition %q targets unknown step %q", event, target),
				})
			}
		}
	}

	// -----------------------------------------------------------------------
	// Phase 3: Handler checks (only when registry is provided)
	// -----------------------------------------------------------------------

	if registry != nil {
		for _, sd := range def.Steps {
			if sd.Name == "" {
				continue
			}
			if !registry.Has(sd.Name) {
				result.Errors = append(result.Errors, ValidationIssue{
					Code:    IssueMissingHandler,
					Step:    sd.Name,
					Message: fmt.Sprintf("step %q has no registered handler", sd.Name),
				})
			}
		}
	}

	// -----------------------------------------------------------------------
	// Phases 4–6 require a valid initial step; skip graph analysis otherwise.
	// -----------------------------------------------------------------------

	if def.InitialStep == "" {
		return result
	}
	if _, ok := stepIndex[def.InitialStep]; !ok {
		return result
	}

	// Build adjacency list: name → []name (only user-defined targets, no
	// terminal pseudo-steps because they have no outgoing edges).
	adjacency := make(map[string][]string, len(stepIndex))
	for name := range stepIndex {
		adjacency[name] = nil
	}
	for _, sd := range def.Steps {
		if sd.Name == "" {
			continue
		}
		for _, target := range sd.Transitions {
			if target == StepDone || target == StepFailed {
				continue
			}
			adjacency[sd.Name] = append(adjacency[sd.Name], target)
		}
	}

	// -----------------------------------------------------------------------
	// Phase 4: Reachability – BFS from InitialStep
	// -----------------------------------------------------------------------

	reachable := make(map[string]bool, len(stepIndex))
	queue := []string{def.InitialStep}
	reachable[def.InitialStep] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, neighbor := range adjacency[current] {
			if !reachable[neighbor] {
				reachable[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
	}

	for name := range stepIndex {
		if !reachable[name] {
			result.Warnings = append(result.Warnings, ValidationIssue{
				Code:    IssueUnreachableStep,
				Step:    name,
				Message: fmt.Sprintf("step %q cannot be reached from initial step %q", name, def.InitialStep),
			})
		}
	}

	// -----------------------------------------------------------------------
	// Phase 5: Cycle detection – DFS with three-color marking
	//
	// Colors:
	//   white (0) = unvisited
	//   gray  (1) = in current DFS path (ancestor stack)
	//   black (2) = fully processed
	//
	// A back-edge (gray → gray) indicates a cycle.
	// -----------------------------------------------------------------------

	const (
		colorWhite = 0
		colorGray  = 1
		colorBlack = 2
	)

	color := make(map[string]int, len(stepIndex))
	cyclesReported := make(map[string]bool) // deduplicate by cycle-start node

	var dfs func(node string, path []string)
	dfs = func(node string, path []string) {
		color[node] = colorGray
		path = append(path, node)

		for _, neighbor := range adjacency[node] {
			switch color[neighbor] {
			case colorGray:
				// Back-edge: neighbor is on the current path → cycle found.
				if !cyclesReported[neighbor] {
					cyclesReported[neighbor] = true
					// Collect the cycle portion of the path.
					cycleStart := -1
					for i, p := range path {
						if p == neighbor {
							cycleStart = i
							break
						}
					}
					var cycleNodes []string
					if cycleStart >= 0 {
						cycleNodes = append(cycleNodes, path[cycleStart:]...)
					}
					cycleNodes = append(cycleNodes, neighbor) // close the loop
					result.Warnings = append(result.Warnings, ValidationIssue{
						Code:    IssueCycleDetected,
						Step:    neighbor,
						Message: fmt.Sprintf("cycle detected involving steps: %s", strings.Join(cycleNodes, " → ")),
					})
				}
			case colorWhite:
				dfs(neighbor, path)
			}
			// colorBlack: already fully processed; no action needed.
		}

		color[node] = colorBlack
	}

	// Start DFS from every unvisited node so we catch disconnected sub-graphs.
	for name := range stepIndex {
		if color[name] == colorWhite {
			dfs(name, nil)
		}
	}

	// -----------------------------------------------------------------------
	// Phase 6: No-transitions warning
	// -----------------------------------------------------------------------

	for _, sd := range def.Steps {
		if sd.Name == "" {
			continue
		}
		if len(sd.Transitions) == 0 {
			result.Warnings = append(result.Warnings, ValidationIssue{
				Code:    IssueNoTransitions,
				Step:    sd.Name,
				Message: fmt.Sprintf("step %q has no transitions; the engine will stall here", sd.Name),
			})
		}
	}

	return result
}

// ValidateDefinitions validates every workflow definition in defs, returning
// a map from workflow name to its ValidationResult. The registry argument is
// passed through to ValidateDefinition unchanged; nil is permitted.
func ValidateDefinitions(defs map[string]*WorkflowDefinition, registry *Registry) map[string]*ValidationResult {
	results := make(map[string]*ValidationResult, len(defs))
	for name, def := range defs {
		results[name] = ValidateDefinition(def, registry)
	}
	return results
}
