package pipeline

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
	"github.com/AbdelazizMoustafa10m/Raven/internal/task"
)

// ErrWizardCancelled is returned when the user cancels the interactive wizard
// (either by pressing Ctrl+C or declining the confirmation).
var ErrWizardCancelled = errors.New("wizard cancelled by user")

// wizardWidth is the fixed form width used by the wizard. 80 columns covers
// the minimum terminal requirement specified in the acceptance criteria.
const wizardWidth = 80

// phaseModeAll selects every available phase.
const phaseModeAll = "all"

// phaseModeFrom restricts execution to phases from a chosen phase onwards.
const phaseModeFrom = "from"

// phaseModeSingle restricts execution to a single chosen phase.
const phaseModeSingle = "single"

// WizardConfig provides the data needed to populate wizard choices.
type WizardConfig struct {
	// Phases contains the available phases loaded from phases.conf.
	Phases []task.Phase

	// Agents lists the names of agents available in the current configuration.
	Agents []string

	// DefaultAgent is the agent name pre-selected in agent dropdowns.
	DefaultAgent string

	// Config is the full project configuration, used for derived defaults.
	Config *config.Config
}

// RunWizard displays the interactive pipeline configuration wizard and returns
// a PipelineOpts populated from the user's selections.
//
// The wizard is split into four pages:
//  1. Phase selection  — all / single / from-phase
//  2. Agent selection  — impl, review, fix agents
//  3. Pipeline options — concurrency, max cycles, skip flags, dry-run
//  4. Confirmation     — summary + run/cancel
//
// Returns ErrWizardCancelled if the user presses Ctrl+C or declines the
// confirmation on the final page.
func RunWizard(cfg WizardConfig) (*PipelineOpts, error) {
	if len(cfg.Phases) == 0 {
		return nil, fmt.Errorf("wizard: no phases configured")
	}

	agents := cfg.Agents
	if len(agents) == 0 {
		agents = []string{"claude", "codex", "gemini"}
	}

	defaultAgent := cfg.DefaultAgent
	if defaultAgent == "" || !containsString(agents, defaultAgent) {
		defaultAgent = agents[0]
	}

	// ------------------------------------------------------------------
	// Page 1: phase selection
	// ------------------------------------------------------------------
	phaseMode := phaseModeAll
	if err := runPhaseModePage(&phaseMode); err != nil {
		return nil, mapWizardErr(err)
	}

	// When the user wants a specific phase or a "from" phase, prompt for it.
	selectedPhaseID := ""
	if phaseMode == phaseModeSingle || phaseMode == phaseModeFrom {
		if err := runPhasePickerPage(cfg.Phases, phaseMode, &selectedPhaseID); err != nil {
			return nil, mapWizardErr(err)
		}
	}

	// ------------------------------------------------------------------
	// Page 2: agent selection
	// ------------------------------------------------------------------
	implAgent := defaultAgent
	reviewAgent := defaultAgent
	fixAgent := defaultAgent

	if len(agents) > 1 {
		if err := runAgentPage(agents, &implAgent, &reviewAgent, &fixAgent); err != nil {
			return nil, mapWizardErr(err)
		}
	}

	// ------------------------------------------------------------------
	// Page 3: pipeline options
	// ------------------------------------------------------------------
	concurrencyStr := "3"
	maxCyclesStr := "3"
	var skipFlags []string
	dryRun := false

	if err := runOptionsPage(&concurrencyStr, &maxCyclesStr, &skipFlags, &dryRun); err != nil {
		return nil, mapWizardErr(err)
	}

	concurrency, _ := strconv.Atoi(concurrencyStr)
	maxCycles, _ := strconv.Atoi(maxCyclesStr)

	opts := buildOpts(
		phaseMode,
		selectedPhaseID,
		implAgent,
		reviewAgent,
		fixAgent,
		concurrency,
		maxCycles,
		skipFlags,
		dryRun,
	)

	// ------------------------------------------------------------------
	// Page 4: confirmation
	// ------------------------------------------------------------------
	confirmed := false
	summary := buildSummary(opts, cfg.Phases)
	if err := runConfirmPage(summary, &confirmed); err != nil {
		return nil, mapWizardErr(err)
	}
	if !confirmed {
		return nil, ErrWizardCancelled
	}

	return opts, nil
}

// runPhaseModePage runs the first wizard page: choosing between "all phases",
// "single phase", or "from phase".
func runPhaseModePage(phaseMode *string) error {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Which phases would you like to run?").
				Description("Select the scope of phases to execute in this pipeline run.").
				Options(
					huh.NewOption("All phases", phaseModeAll),
					huh.NewOption("Single phase (choose one)", phaseModeSingle),
					huh.NewOption("From phase (run from chosen phase onwards)", phaseModeFrom),
				).
				Value(phaseMode),
		),
	).
		WithTheme(huh.ThemeCharm()).
		WithWidth(wizardWidth).
		Run()
}

// runPhasePickerPage presents a select with all configured phases. The title
// changes depending on whether the user is picking a single phase or a
// "from" phase.
func runPhasePickerPage(phases []task.Phase, mode string, selectedID *string) error {
	options := make([]huh.Option[string], len(phases))
	for i, ph := range phases {
		label := fmt.Sprintf("Phase %d: %s", ph.ID, ph.Name)
		options[i] = huh.NewOption(label, strconv.Itoa(ph.ID))
	}
	// Pre-select the first phase.
	if len(options) > 0 {
		*selectedID = options[0].Value
	}

	title := "Which phase?"
	if mode == phaseModeFrom {
		title = "Start from which phase?"
	}

	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(options...).
				Value(selectedID),
		),
	).
		WithTheme(huh.ThemeCharm()).
		WithWidth(wizardWidth).
		Run()
}

// runAgentPage shows three select dropdowns for the implementation, review,
// and fix agents. It is only shown when more than one agent is available.
func runAgentPage(agents []string, implAgent, reviewAgent, fixAgent *string) error {
	options := make([]huh.Option[string], len(agents))
	for i, a := range agents {
		options[i] = huh.NewOption(a, a)
	}

	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Implementation agent:").
				Description("Agent used for the implementation step.").
				Options(options...).
				Value(implAgent),
			huh.NewSelect[string]().
				Title("Review agent:").
				Description("Agent used for the code review step.").
				Options(options...).
				Value(reviewAgent),
			huh.NewSelect[string]().
				Title("Fix agent:").
				Description("Agent used for the fix step.").
				Options(options...).
				Value(fixAgent),
		),
	).
		WithTheme(huh.ThemeCharm()).
		WithWidth(wizardWidth).
		Run()
}

// skipFlagImplement is the skip-flag key for the implementation step.
const skipFlagImplement = "skip_implement"

// skipFlagReview is the skip-flag key for the review step.
const skipFlagReview = "skip_review"

// skipFlagFix is the skip-flag key for the fix step.
const skipFlagFix = "skip_fix"

// skipFlagPR is the skip-flag key for the PR creation step.
const skipFlagPR = "skip_pr"

// runOptionsPage collects numeric pipeline options and skip toggles.
func runOptionsPage(concurrencyStr, maxCyclesStr *string, skipFlags *[]string, dryRun *bool) error {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Review concurrency (1-10):").
				Description("Maximum number of concurrent review agents.").
				Value(concurrencyStr).
				Validate(validateConcurrency),
			huh.NewInput().
				Title("Max review/fix cycles (1-5):").
				Description("Maximum number of review-then-fix iterations per phase.").
				Value(maxCyclesStr).
				Validate(validateMaxCycles),
			huh.NewMultiSelect[string]().
				Title("Skip steps (select steps to skip):").
				Description("Use space to toggle. Skipped steps are not executed.").
				Options(
					huh.NewOption("Skip implementation", skipFlagImplement),
					huh.NewOption("Skip review", skipFlagReview),
					huh.NewOption("Skip fix", skipFlagFix),
					huh.NewOption("Skip PR creation", skipFlagPR),
				).
				Value(skipFlags),
			huh.NewConfirm().
				Title("Dry run?").
				Description("Describe what would happen without making any changes.").
				Value(dryRun),
		),
	).
		WithTheme(huh.ThemeCharm()).
		WithWidth(wizardWidth).
		Run()
}

// runConfirmPage shows a final summary and asks for confirmation before
// launching the pipeline.
func runConfirmPage(summary string, confirmed *bool) error {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Run pipeline?").
				Description(summary).
				Affirmative("Run Pipeline").
				Negative("Cancel").
				Value(confirmed),
		),
	).
		WithTheme(huh.ThemeCharm()).
		WithWidth(wizardWidth).
		Run()
}

// buildOpts constructs a PipelineOpts from the values collected by the wizard.
func buildOpts(
	phaseMode string,
	selectedPhaseID string,
	implAgent, reviewAgent, fixAgent string,
	concurrency, maxCycles int,
	skipFlags []string,
	dryRun bool,
) *PipelineOpts {
	opts := &PipelineOpts{
		ImplAgent:         implAgent,
		ReviewAgent:       reviewAgent,
		FixAgent:          fixAgent,
		ReviewConcurrency: concurrency,
		MaxReviewCycles:   maxCycles,
		DryRun:            dryRun,
		Interactive:       true,
	}

	switch phaseMode {
	case phaseModeAll:
		opts.PhaseID = ""
	case phaseModeSingle:
		opts.PhaseID = selectedPhaseID
	case phaseModeFrom:
		opts.FromPhase = selectedPhaseID
	}

	skipSet := make(map[string]bool, len(skipFlags))
	for _, f := range skipFlags {
		skipSet[f] = true
	}
	opts.SkipImplement = skipSet[skipFlagImplement]
	opts.SkipReview = skipSet[skipFlagReview]
	opts.SkipFix = skipSet[skipFlagFix]
	opts.SkipPR = skipSet[skipFlagPR]

	return opts
}

// buildSummary returns a human-readable summary of the wizard selections
// suitable for display on the confirmation page.
func buildSummary(opts *PipelineOpts, phases []task.Phase) string {
	var sb strings.Builder

	// Phase scope.
	switch {
	case opts.FromPhase != "":
		sb.WriteString(fmt.Sprintf("Phases:         from phase %s\n", opts.FromPhase))
	case opts.PhaseID != "":
		name := phaseNameByID(phases, opts.PhaseID)
		sb.WriteString(fmt.Sprintf("Phase:          %s (%s)\n", opts.PhaseID, name))
	default:
		sb.WriteString(fmt.Sprintf("Phases:         all (%d total)\n", len(phases)))
	}

	// Agents.
	sb.WriteString(fmt.Sprintf("Impl agent:     %s\n", opts.ImplAgent))
	sb.WriteString(fmt.Sprintf("Review agent:   %s\n", opts.ReviewAgent))
	sb.WriteString(fmt.Sprintf("Fix agent:      %s\n", opts.FixAgent))

	// Numeric options.
	sb.WriteString(fmt.Sprintf("Concurrency:    %d\n", opts.ReviewConcurrency))
	sb.WriteString(fmt.Sprintf("Max cycles:     %d\n", opts.MaxReviewCycles))

	// Skip flags.
	var skipped []string
	if opts.SkipImplement {
		skipped = append(skipped, "implement")
	}
	if opts.SkipReview {
		skipped = append(skipped, "review")
	}
	if opts.SkipFix {
		skipped = append(skipped, "fix")
	}
	if opts.SkipPR {
		skipped = append(skipped, "PR")
	}
	if len(skipped) > 0 {
		sb.WriteString(fmt.Sprintf("Skip:           %s\n", strings.Join(skipped, ", ")))
	}

	if opts.DryRun {
		sb.WriteString("Mode:           dry run\n")
	}

	return sb.String()
}

// mapWizardErr converts huh-specific errors into ErrWizardCancelled so callers
// do not need to import the huh package.
func mapWizardErr(err error) error {
	if errors.Is(err, huh.ErrUserAborted) {
		return ErrWizardCancelled
	}
	return fmt.Errorf("wizard: %w", err)
}

// validateConcurrency validates that a string represents an integer in [1, 10].
func validateConcurrency(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return errors.New("must be a number")
	}
	if n < 1 || n > 10 {
		return errors.New("must be between 1 and 10")
	}
	return nil
}

// validateMaxCycles validates that a string represents an integer in [1, 5].
func validateMaxCycles(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return errors.New("must be a number")
	}
	if n < 1 || n > 5 {
		return errors.New("must be between 1 and 5")
	}
	return nil
}

// containsString reports whether slice contains the given target string.
func containsString(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// phaseNameByID returns the human-readable name for the phase whose numeric ID
// string matches id, or the empty string when no match is found.
func phaseNameByID(phases []task.Phase, id string) string {
	for _, ph := range phases {
		if strconv.Itoa(ph.ID) == id {
			return ph.Name
		}
	}
	return ""
}
