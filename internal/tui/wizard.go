package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// PipelineWizardConfig
// ---------------------------------------------------------------------------

// PipelineWizardConfig holds the configuration collected by the pipeline
// setup wizard. The caller is responsible for interpreting PhaseMode to
// determine which phase fields are relevant.
type PipelineWizardConfig struct {
	// PhaseMode controls which phases to run: "single", "range", or "all".
	PhaseMode string
	// PhaseID is the target phase number when PhaseMode == "single".
	PhaseID int
	// FromPhase is the starting phase number when PhaseMode == "range".
	FromPhase int
	// ToPhase is the ending phase number when PhaseMode == "range".
	// A value of 0 means "to the last phase".
	ToPhase int

	// ImplAgent is the agent to use for the implementation step.
	ImplAgent string
	// ReviewAgents is the list of agents to use for the parallel review step.
	ReviewAgents []string
	// FixAgent is the agent to use for the fix step.
	FixAgent string

	// ReviewConcurrency is the maximum number of review agents to run in
	// parallel.
	ReviewConcurrency int
	// MaxReviewCycles is the maximum number of review-fix cycles per task.
	MaxReviewCycles int
	// MaxIterations is the maximum number of implementation iterations per
	// task.
	MaxIterations int

	// SkipImplement omits the implementation step when true.
	SkipImplement bool
	// SkipReview omits the review step when true.
	SkipReview bool
	// SkipFix omits the fix step when true.
	SkipFix bool
	// SkipPR omits the pull-request creation step when true.
	SkipPR bool

	// TotalPhases is the total number of available phases loaded from
	// phases.conf. It is set by the wizard on form completion so that
	// startPipeline can correctly configure the workflow state for "all"
	// and "range" modes when ToPhase is 0 (meaning "to the last phase").
	TotalPhases int
}

// ---------------------------------------------------------------------------
// Wizard messages
// ---------------------------------------------------------------------------

// WizardCompleteMsg is dispatched when the user finishes the pipeline wizard.
// The collected configuration is embedded in the message.
type WizardCompleteMsg struct {
	// Config is the configuration collected from the wizard form.
	Config PipelineWizardConfig
}

// WizardCancelledMsg is dispatched when the user cancels the pipeline wizard
// by pressing Esc or the abort key.
type WizardCancelledMsg struct{}

// ---------------------------------------------------------------------------
// WizardModel
// ---------------------------------------------------------------------------

// WizardModel is the Bubble Tea sub-model for the pipeline setup wizard.
// It wraps a charmbracelet/huh form and manages the wizard lifecycle.
// When active, it renders the form and emits WizardCompleteMsg or
// WizardCancelledMsg on completion/cancellation.
type WizardModel struct {
	theme           Theme
	form            *huh.Form
	width           int
	height          int
	active          bool
	config          PipelineWizardConfig
	availableAgents []string
	availablePhases []int

	// Intermediate string values used by huh.Input for numeric fields.
	// Parsed into config when the form completes.
	rawPhaseID           string
	rawFromPhase         string
	rawToPhase           string
	rawReviewConcurrency string
	rawMaxReviewCycles   string
	rawMaxIterations     string
}

// NewWizardModel creates a WizardModel with sensible defaults. The wizard
// starts inactive; call Start() to build the form and activate it.
//
// agents is the list of available AI agent names (e.g. "claude", "codex").
// phases is the list of available phase numbers.
func NewWizardModel(theme Theme, agents []string, phases []int) WizardModel {
	return WizardModel{
		theme:           theme,
		availableAgents: agents,
		availablePhases: phases,
		config: PipelineWizardConfig{
			PhaseMode:         "all",
			ReviewConcurrency: 2,
			MaxReviewCycles:   3,
			MaxIterations:     10,
		},
		rawReviewConcurrency: "2",
		rawMaxReviewCycles:   "3",
		rawMaxIterations:     "10",
		rawPhaseID:           "1",
		rawFromPhase:         "1",
		rawToPhase:           "0",
	}
}

// SetDimensions updates the terminal dimensions used to size the wizard form.
// Call this whenever the parent App receives a tea.WindowSizeMsg.
func (w *WizardModel) SetDimensions(width, height int) {
	w.width = width
	w.height = height
	if w.form != nil && w.active {
		w.form = w.form.WithWidth(width)
	}
}

// IsActive reports whether the wizard is currently displayed.
func (w WizardModel) IsActive() bool {
	return w.active
}

// Start builds the huh form, marks the wizard active, and returns the form's
// Init command. The caller must forward the returned tea.Cmd to the runtime.
func (w *WizardModel) Start() tea.Cmd {
	w.form = w.buildForm()
	w.active = true
	return w.form.Init()
}

// Update processes incoming messages while the wizard is active.
// It forwards all messages to the underlying huh form and transitions on
// form completion or abort.
//
// Returns:
//   - WizardCompleteMsg  when the user finishes the form.
//   - WizardCancelledMsg when the user presses Esc / abort key.
func (w WizardModel) Update(msg tea.Msg) (WizardModel, tea.Cmd) {
	if !w.active || w.form == nil {
		return w, nil
	}

	// Handle Esc directly to allow cancellation even if huh absorbs it.
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.Type == tea.KeyEsc {
			w.active = false
			return w, func() tea.Msg { return WizardCancelledMsg{} }
		}
	}

	form, cmd := w.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		w.form = f
	}

	switch w.form.State {
	case huh.StateCompleted:
		w.active = false
		w.parseFormValues()
		cfg := w.config
		return w, func() tea.Msg { return WizardCompleteMsg{Config: cfg} }

	case huh.StateAborted:
		w.active = false
		return w, func() tea.Msg { return WizardCancelledMsg{} }

	default:
	}

	return w, cmd
}

// View renders the wizard overlay. Returns an empty string when inactive.
func (w WizardModel) View() string {
	if !w.active || w.form == nil {
		return ""
	}

	formView := w.form.View()
	if formView == "" {
		return ""
	}

	// Wrap the form in a styled container centered on the terminal.
	containerStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(1, 2)

	boxed := containerStyle.Render(formView)

	if w.width > 0 && w.height > 0 {
		return lipgloss.Place(
			w.width, w.height,
			lipgloss.Center, lipgloss.Center,
			boxed,
		)
	}

	return boxed
}

// ---------------------------------------------------------------------------
// buildForm
// ---------------------------------------------------------------------------

// buildForm constructs the huh.Form with 5 groups:
//  1. Phase selection
//  2. Agent selection
//  3. Numeric settings
//  4. Skip flags
//  5. Confirmation summary
func (w *WizardModel) buildForm() *huh.Form {
	huhTheme := buildHuhTheme(w.theme)

	groups := []*huh.Group{
		w.buildPhaseGroup(),
		w.buildAgentGroup(),
		w.buildSettingsGroup(),
		w.buildSkipFlagsGroup(),
		w.buildConfirmGroup(),
	}

	formWidth := w.width
	if formWidth <= 0 {
		formWidth = 80
	}
	// Cap form width to avoid an overly wide form on large terminals.
	if formWidth > 100 {
		formWidth = 100
	}

	return huh.NewForm(groups...).
		WithTheme(huhTheme).
		WithWidth(formWidth).
		WithShowHelp(true)
}

// buildPhaseGroup returns Group 1: phase selection (single/range/all).
func (w *WizardModel) buildPhaseGroup() *huh.Group {
	if len(w.availablePhases) == 0 {
		return huh.NewGroup(
			huh.NewNote().
				Title("Phase Selection").
				Description("No phases are configured. Add phases to your pipeline configuration first."),
		)
	}

	phaseOptions := []huh.Option[string]{
		huh.NewOption("All phases", "all"),
		huh.NewOption("Single phase", "single"),
		huh.NewOption("Phase range", "range"),
	}

	// Phase mode select is always included.
	// Phase ID / range inputs are shown alongside the selector so users can
	// pre-fill values; which inputs are actually used depends on PhaseMode.
	return huh.NewGroup(
		huh.NewSelect[string]().
			Title("Phase Selection").
			Description("Choose which phases to run in the pipeline.").
			Options(phaseOptions...).
			Value(&w.config.PhaseMode),
		huh.NewInput().
			Title("Phase ID (for single mode)").
			Description("Enter the phase number to run (e.g. 1).").
			Value(&w.rawPhaseID).
			Validate(func(s string) error {
				n, err := strconv.Atoi(s)
				if err != nil {
					return fmt.Errorf("must be a positive integer")
				}
				if n < 1 {
					return fmt.Errorf("phase ID must be >= 1")
				}
				return nil
			}),
		huh.NewInput().
			Title("From Phase (for range mode)").
			Description("Enter the starting phase number.").
			Value(&w.rawFromPhase).
			Validate(func(s string) error {
				n, err := strconv.Atoi(s)
				if err != nil {
					return fmt.Errorf("must be a positive integer")
				}
				if n < 1 {
					return fmt.Errorf("from phase must be >= 1")
				}
				return nil
			}),
		huh.NewInput().
			Title("To Phase (for range mode, 0 = end)").
			Description("Enter the ending phase number (0 means run to the last phase).").
			Value(&w.rawToPhase).
			Validate(func(s string) error {
				n, err := strconv.Atoi(s)
				if err != nil {
					return fmt.Errorf("must be an integer (0 = to end)")
				}
				if n < 0 {
					return fmt.Errorf("to phase must be >= 0")
				}
				return nil
			}),
	)
}

// buildAgentGroup returns Group 2: agent selection.
func (w *WizardModel) buildAgentGroup() *huh.Group {
	if len(w.availableAgents) == 0 {
		return huh.NewGroup(
			huh.NewNote().
				Title("Agent Selection").
				Description("No agents are configured. Add agents to your configuration first."),
		)
	}

	implOptions := make([]huh.Option[string], len(w.availableAgents))
	for i, a := range w.availableAgents {
		implOptions[i] = huh.NewOption(capitalizeFirst(a), a)
	}

	multiOptions := make([]huh.Option[string], len(w.availableAgents))
	for i, a := range w.availableAgents {
		multiOptions[i] = huh.NewOption(capitalizeFirst(a), a)
	}

	// Pre-select first agent when only one is available.
	if len(w.availableAgents) == 1 {
		w.config.ImplAgent = w.availableAgents[0]
		w.config.FixAgent = w.availableAgents[0]
	}

	return huh.NewGroup(
		huh.NewSelect[string]().
			Title("Implementation Agent").
			Description("Agent used for the implementation step.").
			Options(implOptions...).
			Value(&w.config.ImplAgent),
		huh.NewMultiSelect[string]().
			Title("Review Agents").
			Description("Agents used for parallel code review (select one or more).").
			Options(multiOptions...).
			Value(&w.config.ReviewAgents),
		huh.NewSelect[string]().
			Title("Fix Agent").
			Description("Agent used to apply review fixes.").
			Options(implOptions...).
			Value(&w.config.FixAgent),
	)
}

// buildSettingsGroup returns Group 3: numeric settings.
func (w *WizardModel) buildSettingsGroup() *huh.Group {
	return huh.NewGroup(
		huh.NewInput().
			Title("Review Concurrency").
			Description("Maximum number of review agents to run in parallel.").
			Value(&w.rawReviewConcurrency).
			Validate(positiveIntValidator("review concurrency")),
		huh.NewInput().
			Title("Max Review Cycles").
			Description("Maximum number of review-fix cycles per task.").
			Value(&w.rawMaxReviewCycles).
			Validate(positiveIntValidator("max review cycles")),
		huh.NewInput().
			Title("Max Iterations").
			Description("Maximum number of implementation iterations per task.").
			Value(&w.rawMaxIterations).
			Validate(positiveIntValidator("max iterations")),
	)
}

// buildSkipFlagsGroup returns Group 4: skip flags (confirm toggles).
func (w *WizardModel) buildSkipFlagsGroup() *huh.Group {
	return huh.NewGroup(
		huh.NewConfirm().
			Title("Skip Implementation Step?").
			Description("When enabled, the implementation step is bypassed.").
			Value(&w.config.SkipImplement),
		huh.NewConfirm().
			Title("Skip Review Step?").
			Description("When enabled, the code review step is bypassed.").
			Value(&w.config.SkipReview),
		huh.NewConfirm().
			Title("Skip Fix Step?").
			Description("When enabled, the fix application step is bypassed.").
			Value(&w.config.SkipFix),
		huh.NewConfirm().
			Title("Skip PR Step?").
			Description("When enabled, pull-request creation is bypassed.").
			Value(&w.config.SkipPR),
	)
}

// buildConfirmGroup returns Group 5: confirmation summary (Note field).
// The description is rendered dynamically via DescriptionFunc so it reflects
// the user's actual selections when they reach the final group. The binding
// is w.config (a plain struct) so hashstructure hashes only the configuration
// fields, not the embedded *huh.Form.
func (w *WizardModel) buildConfirmGroup() *huh.Group {
	return huh.NewGroup(
		huh.NewNote().
			Title("Configuration Summary").
			DescriptionFunc(func() string { return w.buildSummary() }, &w.config),
	)
}

// buildSummary produces a human-readable summary of the current configuration
// for display in the confirmation group.
func (w *WizardModel) buildSummary() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Phase mode    : %s\n", w.config.PhaseMode))
	if w.config.PhaseMode == "single" {
		sb.WriteString(fmt.Sprintf("Phase ID      : %s\n", w.rawPhaseID))
	} else if w.config.PhaseMode == "range" {
		sb.WriteString(fmt.Sprintf("From phase    : %s\n", w.rawFromPhase))
		sb.WriteString(fmt.Sprintf("To phase      : %s\n", w.rawToPhase))
	}
	sb.WriteString(fmt.Sprintf("Impl agent    : %s\n", w.config.ImplAgent))
	sb.WriteString(fmt.Sprintf("Review agents : %s\n", strings.Join(w.config.ReviewAgents, ", ")))
	sb.WriteString(fmt.Sprintf("Fix agent     : %s\n", w.config.FixAgent))
	sb.WriteString(fmt.Sprintf("Concurrency   : %s\n", w.rawReviewConcurrency))
	sb.WriteString(fmt.Sprintf("Review cycles : %s\n", w.rawMaxReviewCycles))
	sb.WriteString(fmt.Sprintf("Max iter      : %s\n", w.rawMaxIterations))

	var skips []string
	if w.config.SkipImplement {
		skips = append(skips, "implement")
	}
	if w.config.SkipReview {
		skips = append(skips, "review")
	}
	if w.config.SkipFix {
		skips = append(skips, "fix")
	}
	if w.config.SkipPR {
		skips = append(skips, "pr")
	}
	if len(skips) > 0 {
		sb.WriteString(fmt.Sprintf("Skip steps    : %s\n", strings.Join(skips, ", ")))
	} else {
		sb.WriteString("Skip steps    : none\n")
	}

	sb.WriteString("\nPress Enter to confirm or Esc to cancel.")
	return sb.String()
}

// parseFormValues converts the raw string fields collected by huh.Input
// widgets into the typed fields in w.config.
func (w *WizardModel) parseFormValues() {
	if n, err := strconv.Atoi(w.rawPhaseID); err == nil {
		w.config.PhaseID = n
	}
	if n, err := strconv.Atoi(w.rawFromPhase); err == nil {
		w.config.FromPhase = n
	}
	if n, err := strconv.Atoi(w.rawToPhase); err == nil {
		w.config.ToPhase = n
	}
	if n, err := strconv.Atoi(w.rawReviewConcurrency); err == nil && n > 0 {
		w.config.ReviewConcurrency = n
	}
	if n, err := strconv.Atoi(w.rawMaxReviewCycles); err == nil && n > 0 {
		w.config.MaxReviewCycles = n
	}
	if n, err := strconv.Atoi(w.rawMaxIterations); err == nil && n > 0 {
		w.config.MaxIterations = n
	}

	// Derive TotalPhases from the wizard's available phases list so that
	// callers (e.g. startPipeline) can set workflow metadata correctly.
	if len(w.availablePhases) > 0 {
		maxPhase := w.availablePhases[0]
		for _, p := range w.availablePhases[1:] {
			if p > maxPhase {
				maxPhase = p
			}
		}
		w.config.TotalPhases = maxPhase
	}
}

// ---------------------------------------------------------------------------
// buildHuhTheme
// ---------------------------------------------------------------------------

// buildHuhTheme translates the Raven TUI Theme into a huh.Theme so that the
// wizard form inherits the application's color palette.
func buildHuhTheme(theme Theme) *huh.Theme {
	t := huh.ThemeBase()

	// Derive colors from the Raven theme for consistent branding.
	t.Focused.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary)
	t.Focused.NoteTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		MarginBottom(1)
	t.Focused.Description = lipgloss.NewStyle().
		Foreground(ColorMuted)
	t.Focused.SelectSelector = lipgloss.NewStyle().
		Foreground(ColorAccent).
		SetString("> ")
	t.Focused.SelectedOption = lipgloss.NewStyle().
		Foreground(ColorAccent)
	t.Focused.UnselectedOption = lipgloss.NewStyle().
		Foreground(ColorMuted)
	t.Focused.FocusedButton = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(ColorPrimary).
		Padding(0, 2).
		MarginRight(1)
	t.Focused.BlurredButton = lipgloss.NewStyle().
		Foreground(ColorMuted).
		Background(ColorHighlight).
		Padding(0, 2).
		MarginRight(1)
	t.Focused.TextInput.Text = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#1F2937", Dark: "#E5E7EB"})
	t.Focused.TextInput.Placeholder = lipgloss.NewStyle().
		Foreground(ColorSubtle)
	t.Focused.TextInput.Cursor = lipgloss.NewStyle().
		Foreground(ColorAccent)
	t.Focused.Base = lipgloss.NewStyle().
		PaddingLeft(1).
		BorderStyle(lipgloss.ThickBorder()).
		BorderLeft(true).
		BorderForeground(ColorPrimary)

	// Blurred (non-focused) variants.
	t.Blurred.Title = lipgloss.NewStyle().
		Foreground(ColorMuted)
	t.Blurred.NoteTitle = lipgloss.NewStyle().
		Foreground(ColorMuted).
		MarginBottom(1)
	t.Blurred.Description = lipgloss.NewStyle().
		Foreground(ColorSubtle)
	t.Blurred.SelectSelector = lipgloss.NewStyle().
		Foreground(ColorSubtle).
		SetString("  ")
	t.Blurred.SelectedOption = lipgloss.NewStyle().
		Foreground(ColorMuted)
	t.Blurred.UnselectedOption = lipgloss.NewStyle().
		Foreground(ColorSubtle)
	t.Blurred.TextInput.Text = lipgloss.NewStyle().
		Foreground(ColorMuted)
	t.Blurred.TextInput.Placeholder = lipgloss.NewStyle().
		Foreground(ColorSubtle)
	t.Blurred.Base = lipgloss.NewStyle().
		PaddingLeft(1).
		BorderStyle(lipgloss.HiddenBorder()).
		BorderLeft(true)

	// Apply group-level header styles to match the focused field title.
	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description

	_ = theme // theme is available for future palette expansion

	return t
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// positiveIntValidator returns a validation function that ensures the input
// string parses as a positive integer. The fieldName is used in the error
// message.
func positiveIntValidator(fieldName string) func(string) error {
	return func(s string) error {
		n, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("%s must be a number", fieldName)
		}
		if n < 1 {
			return fmt.Errorf("%s must be >= 1", fieldName)
		}
		return nil
	}
}

// capitalizeFirst returns s with its first Unicode rune uppercased.
// Returns s unchanged if it is empty.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
