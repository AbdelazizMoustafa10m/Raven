package workflow

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// DryRunFormatter formats dry-run output for workflows and pipelines.
// When styled is true, lipgloss ANSI styling is applied; when false, plain
// text is emitted. Output is written to the embedded io.Writer via Write.
type DryRunFormatter struct {
	writer io.Writer
	styled bool
}

// NewDryRunFormatter creates a new DryRunFormatter writing to w.
// When styled is true, lipgloss ANSI styling is applied; when false, plain
// text is emitted.
func NewDryRunFormatter(w io.Writer, styled bool) *DryRunFormatter {
	return &DryRunFormatter{writer: w, styled: styled}
}

// PhaseDryRunDetail contains dry-run info for a single pipeline phase.
type PhaseDryRunDetail struct {
	// PhaseID is the numeric identifier for the phase.
	PhaseID int

	// PhaseName is the human-readable name of the phase.
	PhaseName string

	// BranchName is the git branch that would be created for this phase.
	BranchName string

	// BaseBranch is the base branch for this phase (previous phase branch or main).
	BaseBranch string

	// Skipped lists the stage names that would be skipped (e.g. "review", "fix", "pr").
	Skipped []string

	// ImplAgent is the agent that would perform implementation.
	ImplAgent string

	// ReviewAgent is the agent that would perform review.
	ReviewAgent string

	// FixAgent is the agent that would perform fixes.
	FixAgent string

	// Steps holds the step-level dry-run detail for this phase.
	Steps []StepDryRunDetail
}

// StepDryRunDetail contains dry-run info for a single workflow step.
type StepDryRunDetail struct {
	// StepName is the name of the step.
	StepName string

	// Description is a human-readable description of what the step would do,
	// typically sourced from StepHandler.DryRun().
	Description string

	// Transitions maps event names to next step names for this step.
	Transitions map[string]string
}

// PipelineDryRunInfo contains all pipeline-level information needed for
// dry-run formatting. It avoids a circular import dependency with the pipeline
// package by not referencing any pipeline types directly.
type PipelineDryRunInfo struct {
	// TotalPhases is the total number of phases in this pipeline run.
	TotalPhases int

	// Phases holds per-phase dry-run detail in execution order.
	Phases []PhaseDryRunDetail
}

// Write writes the formatted string s to f.writer.
func (f *DryRunFormatter) Write(s string) {
	fmt.Fprint(f.writer, s)
}

// FormatWorkflowDryRun formats the dry-run output for a single workflow
// definition. It walks the definition graph from the initial step in BFS
// order, collecting step descriptions and transitions. Cycles are detected
// and shown as "(cycles back to step N)" rather than causing infinite
// recursion.
//
// The stepOutputs map keys are step names; values are the description strings
// returned by StepHandler.DryRun() for that step. When a step name is absent
// from stepOutputs a generic "step N" fallback is used.
//
// The method returns a formatted string; it does not write to f.writer.
func (f *DryRunFormatter) FormatWorkflowDryRun(
	def *WorkflowDefinition,
	_ *WorkflowState,
	stepOutputs map[string]string,
) string {
	if def == nil || len(def.Steps) == 0 {
		return "No steps defined.\n"
	}

	// Build a name-keyed lookup for O(1) step access.
	stepByName := make(map[string]*StepDefinition, len(def.Steps))
	for i := range def.Steps {
		sd := &def.Steps[i]
		stepByName[sd.Name] = sd
	}

	// BFS from InitialStep, preserving visit order so step numbers are stable.
	visited := make(map[string]int) // name -> 1-based step number
	var ordered []string            // step names in BFS visit order

	queue := []string{def.InitialStep}
	visited[def.InitialStep] = 1
	ordered = append(ordered, def.InitialStep)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		sd, ok := stepByName[current]
		if !ok {
			continue
		}

		// Sort transition events for deterministic output.
		events := sortedKeys(sd.Transitions)
		for _, ev := range events {
			target := sd.Transitions[ev]

			// Skip terminal pseudo-steps -- they are not real steps to visit.
			if target == StepDone || target == StepFailed {
				continue
			}

			if _, seen := visited[target]; !seen {
				n := len(ordered) + 1
				visited[target] = n
				ordered = append(ordered, target)
				queue = append(queue, target)
			}
		}
	}

	// Styles.
	headerStyle := lipgloss.NewStyle()
	stepNameStyle := lipgloss.NewStyle()
	transitionStyle := lipgloss.NewStyle()

	if f.styled {
		headerStyle = headerStyle.Bold(true).Foreground(lipgloss.Color("12")) // bright blue
		stepNameStyle = stepNameStyle.Bold(true)
		transitionStyle = transitionStyle.Faint(true)
	}

	var sb strings.Builder

	// Header.
	header := fmt.Sprintf("Workflow: %s", def.Name)
	underline := strings.Repeat("=", len(header))
	sb.WriteString(headerStyle.Render(header))
	sb.WriteString("\n")
	sb.WriteString(underline)
	sb.WriteString("\n\n")

	// Render each step in BFS order.
	for _, stepName := range ordered {
		stepNum := visited[stepName]
		sd := stepByName[stepName]

		desc, hasDesc := stepOutputs[stepName]
		if !hasDesc || desc == "" {
			desc = fmt.Sprintf("step %d", stepNum)
		}

		// Step header line: "  N. step_name: description"
		stepHeader := fmt.Sprintf("%s: %s", stepName, desc)
		sb.WriteString(fmt.Sprintf("  %d. %s\n", stepNum, stepNameStyle.Render(stepHeader)))

		if sd == nil {
			continue
		}

		// Transitions -- sorted for deterministic output.
		events := sortedKeys(sd.Transitions)
		for _, ev := range events {
			target := sd.Transitions[ev]

			var targetDisplay string
			switch target {
			case StepDone:
				targetDisplay = "DONE"
			case StepFailed:
				targetDisplay = "FAILED"
			default:
				targetNum, alreadySeen := visited[target]
				if alreadySeen && targetNum < stepNum {
					// Cycle: target is an earlier step.
					targetDisplay = fmt.Sprintf("%s (cycles back to step %d)", target, targetNum)
				} else if alreadySeen && targetNum == stepNum {
					// Self-loop edge case.
					targetDisplay = fmt.Sprintf("%s (cycles back to step %d)", target, targetNum)
				} else {
					targetDisplay = target
				}
			}

			transLine := fmt.Sprintf("     -> %s: %s", ev, targetDisplay)
			sb.WriteString(transitionStyle.Render(transLine))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// FormatPipelineDryRun formats the dry-run output for a full pipeline
// described by info. It returns a formatted string; it does not write to
// f.writer.
func (f *DryRunFormatter) FormatPipelineDryRun(info PipelineDryRunInfo) string {
	// Styles.
	mainHeaderStyle := lipgloss.NewStyle()
	phaseHeaderStyle := lipgloss.NewStyle()
	skipStyle := lipgloss.NewStyle()

	if f.styled {
		mainHeaderStyle = mainHeaderStyle.Bold(true).Foreground(lipgloss.Color("14")) // cyan
		phaseHeaderStyle = phaseHeaderStyle.Bold(true).Foreground(lipgloss.Color("12"))
		skipStyle = skipStyle.Foreground(lipgloss.Color("11")) // yellow
	}

	var sb strings.Builder

	// Top-level header.
	const mainHeader = "Pipeline Dry Run"
	sb.WriteString(mainHeaderStyle.Render(mainHeader))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("=", len(mainHeader)))
	sb.WriteString("\n\n")

	for _, ph := range info.Phases {
		// Phase heading.
		phaseHeading := fmt.Sprintf("Phase %d: %s", ph.PhaseID, ph.PhaseName)
		sb.WriteString(phaseHeaderStyle.Render(phaseHeading))
		sb.WriteString("\n")

		// Branch info.
		sb.WriteString(fmt.Sprintf("  Branch: %s (from %s)\n", ph.BranchName, ph.BaseBranch))

		// Skipped stages.
		skippedSet := make(map[string]bool, len(ph.Skipped))
		for _, s := range ph.Skipped {
			skippedSet[s] = true
		}

		if len(ph.Skipped) > 0 {
			skipLine := fmt.Sprintf("  [SKIPPED: %s]", strings.Join(ph.Skipped, ", "))
			sb.WriteString(skipStyle.Render(skipLine))
			sb.WriteString("\n")
		}

		// Agent assignments (omit stages that are skipped).
		sb.WriteString(fmt.Sprintf("  Implementation: %s\n", ph.ImplAgent))
		if !skippedSet["review"] {
			sb.WriteString(fmt.Sprintf("  Review: %s\n", ph.ReviewAgent))
		}
		if !skippedSet["fix"] {
			sb.WriteString(fmt.Sprintf("  Fix: %s\n", ph.FixAgent))
		}

		// Step detail, if provided.
		if len(ph.Steps) > 0 {
			sb.WriteString("\n  Steps:\n")

			// Assign step numbers in order.
			stepNums := make(map[string]int, len(ph.Steps))
			for i, s := range ph.Steps {
				stepNums[s.StepName] = i + 1
			}

			for i, step := range ph.Steps {
				stepNum := i + 1
				desc := step.Description
				if desc == "" {
					desc = fmt.Sprintf("step %d", stepNum)
				}

				sb.WriteString(fmt.Sprintf("    %d. %s: %s\n", stepNum, step.StepName, desc))

				// Transitions -- sorted for deterministic output.
				events := sortedKeys(step.Transitions)
				for _, ev := range events {
					target := step.Transitions[ev]

					var targetDisplay string
					switch target {
					case StepDone:
						targetDisplay = "DONE"
					case StepFailed:
						targetDisplay = "FAILED"
					default:
						targetNum, known := stepNums[target]
						if known && targetNum < stepNum {
							targetDisplay = fmt.Sprintf("%s (cycles back to step %d)", target, targetNum)
						} else {
							targetDisplay = target
						}
					}

					sb.WriteString(fmt.Sprintf("       -> %s: %s\n", ev, targetDisplay))
				}
			}
		}

		sb.WriteString("\n")
	}

	// Footer.
	sb.WriteString(fmt.Sprintf("Total phases: %d\n", info.TotalPhases))

	return sb.String()
}

// sortedKeys returns the keys of m sorted alphabetically.
// It is used throughout DryRunFormatter to ensure deterministic output.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
