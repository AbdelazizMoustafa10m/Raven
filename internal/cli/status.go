package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
	"github.com/AbdelazizMoustafa10m/Raven/internal/task"
)

// statusFlags holds the flag values for the status command.
type statusFlags struct {
	Phase   int  // --phase <id>, 0 means all phases
	JSON    bool // --json for structured output
	Verbose bool // --verbose for per-task details (overrides global --verbose for output control)
}

// statusPhaseOutput is the JSON output type for a single phase.
type statusPhaseOutput struct {
	PhaseID    int     `json:"phase_id"`
	PhaseName  string  `json:"phase_name"`
	Total      int     `json:"total"`
	Completed  int     `json:"completed"`
	InProgress int     `json:"in_progress"`
	Blocked    int     `json:"blocked"`
	Skipped    int     `json:"skipped"`
	NotStarted int     `json:"not_started"`
	Percent    float64 `json:"percent"`
}

// statusOutput is the top-level JSON output type for the status command.
type statusOutput struct {
	ProjectName  string              `json:"project_name"`
	TotalTasks   int                 `json:"total_tasks"`
	TotalDone    int                 `json:"total_done"`
	OverallPct   float64             `json:"overall_percent"`
	CurrentPhase int                 `json:"current_phase"`
	Phases       []statusPhaseOutput `json:"phases"`
}

// newStatusCmd creates the "raven status" command.
func newStatusCmd() *cobra.Command {
	var flags statusFlags

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show phase-by-phase task progress with progress bars",
		Long: `Display a summary of task progress for all phases or a single phase.
Each phase shows a progress bar, completion fraction, and counts.

Use --verbose to see per-task status details. Use --json for structured
output suitable for scripting.`,
		Example: `  # Show all phases
  raven status

  # Show only phase 2
  raven status --phase 2

  # Show per-task details
  raven status --verbose

  # Structured JSON output
  raven status --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, args, flags)
		},
	}

	cmd.Flags().IntVar(&flags.Phase, "phase", 0, "Filter to a single phase (0 = all phases)")
	cmd.Flags().BoolVar(&flags.JSON, "json", false, "Output structured JSON to stdout")
	cmd.Flags().BoolVar(&flags.Verbose, "verbose", false, "Show per-task status details within each phase")

	return cmd
}

func init() {
	rootCmd.AddCommand(newStatusCmd())
}

// runStatus is the command's RunE function. Loads config, discovers tasks,
// computes progress, and renders output.
func runStatus(cmd *cobra.Command, _ []string, flags statusFlags) error {
	// Load and resolve configuration.
	resolved, _, err := loadAndResolveConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	cfg := resolved.Config

	// Discover task specs.
	tasksDir := cfg.Project.TasksDir
	specs, err := task.DiscoverTasks(tasksDir)
	if err != nil {
		return fmt.Errorf("discovering tasks: %w", err)
	}

	// Load task state.
	stateManager := task.NewStateManager(cfg.Project.TaskStateFile)

	// Load phases -- gracefully handle missing phases.conf.
	var phases []task.Phase
	if cfg.Project.PhasesConf != "" {
		phases, err = task.LoadPhases(cfg.Project.PhasesConf)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("loading phases: %w", err)
		}
		// If file not found, phases remains nil -- handled gracefully below.
	}

	// Build task selector.
	selector := task.NewTaskSelector(specs, stateManager, phases)

	// Handle the case where there are no tasks.
	if len(specs) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "No tasks found.")
		return nil
	}

	// Get progress data.
	var allProgress []task.PhaseProgress

	if len(phases) == 0 {
		// No phases configured: show a single synthetic summary without phase grouping.
		stateMap, err := stateManager.LoadMap()
		if err != nil {
			return fmt.Errorf("loading task state: %w", err)
		}
		prog := buildUngroupedProgress(specs, stateMap)
		allProgress = []task.PhaseProgress{prog}
	} else {
		progressMap, err := selector.GetAllProgress()
		if err != nil {
			return fmt.Errorf("computing progress: %w", err)
		}

		// Filter to requested phase if --phase flag was set.
		if flags.Phase != 0 {
			p, ok := progressMap[flags.Phase]
			if !ok {
				return fmt.Errorf("phase %d not found", flags.Phase)
			}
			allProgress = []task.PhaseProgress{p}
		} else {
			// Collect all phases sorted by phase ID.
			allProgress = make([]task.PhaseProgress, 0, len(progressMap))
			for _, ph := range phases {
				if prog, ok := progressMap[ph.ID]; ok {
					allProgress = append(allProgress, prog)
				}
			}
		}
	}

	// JSON output mode: write to stdout.
	if flags.JSON {
		return renderJSON(cmd.OutOrStdout(), cfg, phases, allProgress)
	}

	// Human-readable output: write to stderr per PRD conventions.
	out := cmd.ErrOrStderr()
	projectName := cfg.Project.Name
	if projectName == "" {
		projectName = "raven"
	}

	fmt.Fprintln(out, renderSummary(allProgress, projectName))

	for _, prog := range allProgress {
		phaseName := phaseNameFor(phases, prog.PhaseID)
		fmt.Fprintln(out, renderPhaseProgress(prog, phaseName, flags.Verbose))

		// Per-task details when --verbose is set.
		if flags.Verbose && len(phases) > 0 {
			details, err := renderTaskDetails(stateManager, specs, phases, prog.PhaseID)
			if err != nil {
				// Non-fatal: log and continue.
				fmt.Fprintf(out, "  (error loading task details: %v)\n", err)
			} else if details != "" {
				fmt.Fprintln(out, details)
			}
		}
	}

	return nil
}

// renderJSON serialises progress data to JSON and writes it to w.
func renderJSON(w io.Writer, cfg *config.Config, phases []task.Phase, allProgress []task.PhaseProgress) error {
	phaseOutputs := make([]statusPhaseOutput, 0, len(allProgress))
	for _, prog := range allProgress {
		pct := 0.0
		if prog.Total > 0 {
			pct = float64(prog.Completed+prog.Skipped) / float64(prog.Total) * 100
		}
		phaseOutputs = append(phaseOutputs, statusPhaseOutput{
			PhaseID:    prog.PhaseID,
			PhaseName:  phaseNameFor(phases, prog.PhaseID),
			Total:      prog.Total,
			Completed:  prog.Completed,
			InProgress: prog.InProgress,
			Blocked:    prog.Blocked,
			Skipped:    prog.Skipped,
			NotStarted: prog.NotStarted,
			Percent:    pct,
		})
	}

	totalTasks, totalDone := 0, 0
	for _, prog := range allProgress {
		totalTasks += prog.Total
		totalDone += prog.Completed + prog.Skipped
	}
	overallPct := 0.0
	if totalTasks > 0 {
		overallPct = float64(totalDone) / float64(totalTasks) * 100
	}

	currentPhase := currentPhaseID(allProgress)

	out := statusOutput{
		ProjectName:  cfg.Project.Name,
		TotalTasks:   totalTasks,
		TotalDone:    totalDone,
		OverallPct:   overallPct,
		CurrentPhase: currentPhase,
		Phases:       phaseOutputs,
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// renderSummary returns an overall project summary header string.
//
//	Raven Status - my-project
//	=====================================
//	Overall: 45/87 tasks completed (51%)
//	Current Phase: 3 - Review Pipeline
func renderSummary(allProgress []task.PhaseProgress, projectName string) string {
	totalTasks, totalDone := 0, 0
	for _, prog := range allProgress {
		totalTasks += prog.Total
		totalDone += prog.Completed + prog.Skipped
	}

	overallPct := 0.0
	if totalTasks > 0 {
		overallPct = float64(totalDone) / float64(totalTasks) * 100
	}

	currentPhase := currentPhaseID(allProgress)

	headerStyle := lipgloss.NewStyle().Bold(true)
	sepStyle := lipgloss.NewStyle()

	title := fmt.Sprintf("Raven Status - %s", projectName)
	sep := strings.Repeat("=", len(title))

	var sb strings.Builder
	sb.WriteString(headerStyle.Render(title))
	sb.WriteString("\n")
	sb.WriteString(sepStyle.Render(sep))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Overall: %d/%d tasks completed (%.0f%%)", totalDone, totalTasks, overallPct))
	sb.WriteString("\n")

	if currentPhase > 0 {
		sb.WriteString(fmt.Sprintf("Current Phase: %d", currentPhase))
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderPhaseProgress returns a styled string for a single phase with a
// progress bar, percentage, and completion fraction.
//
//	Phase 2: Core Implementation
//	████████████░░░░░░░░ 60% (12/20)
func renderPhaseProgress(prog task.PhaseProgress, phaseName string, _ bool) string {
	const progressBarWidth = 40

	phaseStyle := lipgloss.NewStyle().Bold(true)
	completedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))  // green
	inProgressStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
	blockedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))     // red

	pct := 0.0
	if prog.Total > 0 {
		pct = float64(prog.Completed+prog.Skipped) / float64(prog.Total)
	}

	var header string
	if prog.PhaseID > 0 && phaseName != "" {
		header = phaseStyle.Render(fmt.Sprintf("Phase %d: %s", prog.PhaseID, phaseName))
	} else {
		header = phaseStyle.Render("All Tasks")
	}

	// Build static progress bar using bubbles/progress ViewAs.
	// Use WithoutPercentage so the bar does not duplicate the percentage we
	// render ourselves in the fraction line below.
	bar := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(progressBarWidth),
		progress.WithoutPercentage(),
	)
	barStr := bar.ViewAs(pct)

	fraction := fmt.Sprintf("%d/%d", prog.Completed+prog.Skipped, prog.Total)
	pctStr := fmt.Sprintf("%.0f%%", pct*100)

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n")
	sb.WriteString(barStr)
	sb.WriteString(" ")
	sb.WriteString(pctStr)
	sb.WriteString(" (")
	sb.WriteString(fraction)
	sb.WriteString(")")
	sb.WriteString("\n")

	// Summary counts.
	var countParts []string
	if prog.Completed > 0 {
		countParts = append(countParts, completedStyle.Render(fmt.Sprintf("%d completed", prog.Completed)))
	}
	if prog.InProgress > 0 {
		countParts = append(countParts, inProgressStyle.Render(fmt.Sprintf("%d in-progress", prog.InProgress)))
	}
	if prog.Blocked > 0 {
		countParts = append(countParts, blockedStyle.Render(fmt.Sprintf("%d blocked", prog.Blocked)))
	}
	if prog.NotStarted > 0 {
		countParts = append(countParts, fmt.Sprintf("%d not-started", prog.NotStarted))
	}
	if prog.Skipped > 0 {
		countParts = append(countParts, fmt.Sprintf("%d skipped", prog.Skipped))
	}

	if len(countParts) > 0 {
		sb.WriteString("  ")
		sb.WriteString(strings.Join(countParts, ", "))
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderTaskDetails loads per-task state for a phase and returns a formatted
// task list showing ID, title, status, and agent (if set).
func renderTaskDetails(
	stateManager *task.StateManager,
	specs []*task.ParsedTaskSpec,
	phases []task.Phase,
	phaseID int,
) (string, error) {
	ph := task.PhaseByID(phases, phaseID)
	if ph == nil {
		return "", nil
	}

	taskIDs := task.TasksInPhase(*ph)
	stateMap, err := stateManager.LoadMap()
	if err != nil {
		return "", fmt.Errorf("loading state map: %w", err)
	}

	// Build specMap for quick lookup.
	specMap := make(map[string]*task.ParsedTaskSpec, len(specs))
	for _, s := range specs {
		specMap[s.ID] = s
	}

	completedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))  // green
	inProgressStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
	blockedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))     // red
	skippedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))     // dark gray

	var sb strings.Builder
	for _, id := range taskIDs {
		spec, hasSpec := specMap[id]
		if !hasSpec {
			continue
		}

		status := task.StatusNotStarted
		agent := ""
		if ts, exists := stateMap[id]; exists {
			status = ts.Status
			agent = ts.Agent
		}

		// Format status label with color.
		var statusLabel string
		switch status {
		case task.StatusCompleted:
			statusLabel = completedStyle.Render("completed")
		case task.StatusInProgress:
			statusLabel = inProgressStyle.Render("in_progress")
		case task.StatusBlocked:
			statusLabel = blockedStyle.Render("blocked")
		case task.StatusSkipped:
			statusLabel = skippedStyle.Render("skipped")
		default:
			statusLabel = "not_started"
		}

		title := spec.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}

		line := fmt.Sprintf("  %s  %-50s  %s", id, title, statusLabel)
		if agent != "" {
			line += fmt.Sprintf("  (%s)", agent)
		}

		// Show blocked-by info for blocked tasks.
		if status == task.StatusBlocked || status == task.StatusNotStarted {
			var unmetDeps []string
			for _, depID := range spec.Dependencies {
				if ts, exists := stateMap[depID]; !exists || ts.Status != task.StatusCompleted {
					unmetDeps = append(unmetDeps, depID)
				}
			}
			if len(unmetDeps) > 0 {
				line += fmt.Sprintf("  [waiting on: %s]", strings.Join(unmetDeps, ", "))
			}
		}

		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// buildUngroupedProgress computes a synthetic PhaseProgress with PhaseID=0
// for projects that have no phases.conf configured.
func buildUngroupedProgress(specs []*task.ParsedTaskSpec, stateMap map[string]*task.TaskState) task.PhaseProgress {
	prog := task.PhaseProgress{
		PhaseID: 0,
		Total:   len(specs),
	}
	for _, spec := range specs {
		ts, exists := stateMap[spec.ID]
		status := task.StatusNotStarted
		if exists {
			status = ts.Status
		}
		switch status {
		case task.StatusCompleted:
			prog.Completed++
		case task.StatusInProgress:
			prog.InProgress++
		case task.StatusBlocked:
			prog.Blocked++
		case task.StatusSkipped:
			prog.Skipped++
		default:
			prog.NotStarted++
		}
	}
	return prog
}

// phaseNameFor returns the display name for the given phase ID by searching
// the phases slice. Returns an empty string if the phase is not found or if
// phaseID is 0 (ungrouped).
func phaseNameFor(phases []task.Phase, phaseID int) string {
	if phaseID == 0 {
		return ""
	}
	for _, ph := range phases {
		if ph.ID == phaseID {
			return ph.Name
		}
	}
	return fmt.Sprintf("Phase %d", phaseID)
}

// currentPhaseID returns the phase ID of the first phase that is not yet
// complete (has non-zero NotStarted or InProgress tasks). Returns 0 if all
// phases are complete.
func currentPhaseID(allProgress []task.PhaseProgress) int {
	sorted := make([]task.PhaseProgress, len(allProgress))
	copy(sorted, allProgress)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].PhaseID < sorted[j].PhaseID
	})
	for _, prog := range sorted {
		if prog.NotStarted > 0 || prog.InProgress > 0 {
			return prog.PhaseID
		}
	}
	return 0
}
