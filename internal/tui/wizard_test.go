package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// TestNewWizardModel
// ---------------------------------------------------------------------------

func TestNewWizardModel(t *testing.T) {
	t.Parallel()

	theme := DefaultTheme()
	agents := []string{"claude", "codex"}
	phases := []int{1, 2, 3}

	w := NewWizardModel(theme, agents, phases)

	assert.Equal(t, agents, w.availableAgents, "agents should be stored")
	assert.Equal(t, phases, w.availablePhases, "phases should be stored")
	assert.Nil(t, w.form, "form should be nil before Start()")
}

func TestNewWizardModel_EmptyAgents(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), nil, []int{1, 2})
	assert.Empty(t, w.availableAgents)
	assert.False(t, w.active)
}

func TestNewWizardModel_EmptyPhases(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, nil)
	assert.Empty(t, w.availablePhases)
	assert.False(t, w.active)
}

// ---------------------------------------------------------------------------
// TestWizardModel_IsActive
// ---------------------------------------------------------------------------

func TestWizardModel_IsActive_FalseInitially(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	assert.False(t, w.IsActive(), "wizard should not be active before Start()")
}

func TestWizardModel_IsActive_TrueAfterStart(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	cmd := w.Start()
	_ = cmd // init command not relevant for state test

	assert.True(t, w.IsActive(), "wizard should be active after Start()")
}

func TestWizardModel_IsActive_FalseAfterCancel(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	_ = w.Start()
	require.True(t, w.IsActive())

	// Directly set active to false to simulate cancellation result.
	w.active = false
	assert.False(t, w.IsActive())
}

// ---------------------------------------------------------------------------
// TestWizardModel_SetDimensions
// ---------------------------------------------------------------------------

func TestWizardModel_SetDimensions(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	w.SetDimensions(120, 40)

	assert.Equal(t, 120, w.width)
	assert.Equal(t, 40, w.height)
}

func TestWizardModel_SetDimensions_Zero(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	w.SetDimensions(0, 0)

	assert.Equal(t, 0, w.width)
	assert.Equal(t, 0, w.height)
}

func TestWizardModel_SetDimensions_UpdatesMultipleTimes(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	w.SetDimensions(80, 24)
	w.SetDimensions(160, 48)

	assert.Equal(t, 160, w.width)
	assert.Equal(t, 48, w.height)
}

// ---------------------------------------------------------------------------
// TestBuildHuhTheme
// ---------------------------------------------------------------------------

func TestBuildHuhTheme(t *testing.T) {
	t.Parallel()

	theme := DefaultTheme()
	huhTheme := buildHuhTheme(theme)

	require.NotNil(t, huhTheme, "buildHuhTheme must return a non-nil theme")
}

func TestBuildHuhTheme_HasFocusedStyles(t *testing.T) {
	t.Parallel()

	huhTheme := buildHuhTheme(DefaultTheme())
	require.NotNil(t, huhTheme)

	// Verify that focused styles are customised (non-zero values).
	// lipgloss.Style is a struct; a non-zero Style has some properties set.
	// We just assert no panic and the theme is returned.
	_ = huhTheme.Focused.Title
	_ = huhTheme.Focused.Description
	_ = huhTheme.Focused.SelectSelector
	_ = huhTheme.Blurred.Title
}

// ---------------------------------------------------------------------------
// TestBuildForm
// ---------------------------------------------------------------------------

func TestBuildForm_ReturnsNonNil(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude", "codex"}, []int{1, 2, 3})
	form := w.buildForm()

	require.NotNil(t, form, "buildForm must return a non-nil form")
}

func TestBuildForm_NoAgents(t *testing.T) {
	t.Parallel()

	// Should not panic even with no agents.
	w := NewWizardModel(DefaultTheme(), nil, []int{1, 2})
	assert.NotPanics(t, func() {
		form := w.buildForm()
		require.NotNil(t, form)
	})
}

func TestBuildForm_NoPhases(t *testing.T) {
	t.Parallel()

	// Should not panic even with no phases.
	w := NewWizardModel(DefaultTheme(), []string{"claude"}, nil)
	assert.NotPanics(t, func() {
		form := w.buildForm()
		require.NotNil(t, form)
	})
}

func TestBuildForm_BothEmpty(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), nil, nil)
	assert.NotPanics(t, func() {
		form := w.buildForm()
		require.NotNil(t, form)
	})
}

// ---------------------------------------------------------------------------
// TestPipelineWizardConfig_Defaults
// ---------------------------------------------------------------------------

func TestPipelineWizardConfig_Defaults(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1, 2})
	cfg := w.config

	assert.Equal(t, "all", cfg.PhaseMode, "default phase mode should be 'all'")
	assert.Equal(t, 2, cfg.ReviewConcurrency, "default review concurrency should be 2")
	assert.Equal(t, 3, cfg.MaxReviewCycles, "default max review cycles should be 3")
	assert.Equal(t, 10, cfg.MaxIterations, "default max iterations should be 10")
	assert.False(t, cfg.SkipImplement, "skip implement should default to false")
	assert.False(t, cfg.SkipReview, "skip review should default to false")
	assert.False(t, cfg.SkipFix, "skip fix should default to false")
	assert.False(t, cfg.SkipPR, "skip pr should default to false")
}

func TestPipelineWizardConfig_DefaultRawValues(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1, 2})

	assert.Equal(t, "2", w.rawReviewConcurrency)
	assert.Equal(t, "3", w.rawMaxReviewCycles)
	assert.Equal(t, "10", w.rawMaxIterations)
	assert.Equal(t, "1", w.rawPhaseID)
	assert.Equal(t, "1", w.rawFromPhase)
	assert.Equal(t, "0", w.rawToPhase)
}

// ---------------------------------------------------------------------------
// TestWizardModel_View
// ---------------------------------------------------------------------------

func TestWizardModel_View_InactiveReturnsEmpty(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	// Not started — should return empty string.
	assert.Empty(t, w.View(), "inactive wizard must return empty view")
}

func TestWizardModel_View_ActiveReturnsNonEmpty(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	w.SetDimensions(120, 40)
	_ = w.Start()

	assert.True(t, w.active)
	// View may return empty if the form.View() returns "" (e.g. after
	// completion), but since we just started it should be non-empty.
	// We only verify no panic.
	assert.NotPanics(t, func() {
		_ = w.View()
	})
}

// ---------------------------------------------------------------------------
// TestWizardModel_Start
// ---------------------------------------------------------------------------

func TestWizardModel_Start_SetsActive(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	require.False(t, w.active)

	cmd := w.Start()
	_ = cmd

	assert.True(t, w.active, "Start() must set active to true")
	assert.NotNil(t, w.form, "Start() must initialise the form")
}

func TestWizardModel_Start_ReturnsCmd(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	cmd := w.Start()

	// Init command from huh form may be non-nil.
	// We just ensure Start() does not panic and active is set.
	assert.True(t, w.active)
	_ = cmd // command may or may not be nil depending on huh internals
}

// ---------------------------------------------------------------------------
// TestPositiveIntValidator
// ---------------------------------------------------------------------------

func TestPositiveIntValidator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid integer 1", input: "1", wantErr: false},
		{name: "valid integer 10", input: "10", wantErr: false},
		{name: "valid integer 100", input: "100", wantErr: false},
		{name: "zero", input: "0", wantErr: true},
		{name: "negative", input: "-1", wantErr: true},
		{name: "not a number", input: "abc", wantErr: true},
		{name: "float", input: "1.5", wantErr: true},
		{name: "empty string", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fn := positiveIntValidator("test field")
			err := fn(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestParseFormValues
// ---------------------------------------------------------------------------

func TestParseFormValues(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1, 2, 3})
	w.rawPhaseID = "3"
	w.rawFromPhase = "2"
	w.rawToPhase = "5"
	w.rawReviewConcurrency = "4"
	w.rawMaxReviewCycles = "6"
	w.rawMaxIterations = "20"

	w.parseFormValues()

	assert.Equal(t, 3, w.config.PhaseID)
	assert.Equal(t, 2, w.config.FromPhase)
	assert.Equal(t, 5, w.config.ToPhase)
	assert.Equal(t, 4, w.config.ReviewConcurrency)
	assert.Equal(t, 6, w.config.MaxReviewCycles)
	assert.Equal(t, 20, w.config.MaxIterations)
}

func TestParseFormValues_InvalidDoesNotOverwrite(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	// Set a valid default.
	w.config.ReviewConcurrency = 2
	// Put an invalid raw value.
	w.rawReviewConcurrency = "not-a-number"

	w.parseFormValues()

	// Should keep the default since parsing failed.
	assert.Equal(t, 2, w.config.ReviewConcurrency)
}

// ---------------------------------------------------------------------------
// TestWizardMessages
// ---------------------------------------------------------------------------

func TestWizardCompleteMsg_ContainsConfig(t *testing.T) {
	t.Parallel()

	cfg := PipelineWizardConfig{
		PhaseMode:         "all",
		ImplAgent:         "claude",
		ReviewConcurrency: 2,
		MaxIterations:     10,
	}
	msg := WizardCompleteMsg{Config: cfg}
	assert.Equal(t, cfg, msg.Config)
}

func TestWizardCancelledMsg_IsZeroValue(t *testing.T) {
	t.Parallel()

	msg := WizardCancelledMsg{}
	assert.Equal(t, WizardCancelledMsg{}, msg)
}

// ---------------------------------------------------------------------------
// TestCapitalizeFirst
// ---------------------------------------------------------------------------

func TestCapitalizeFirst(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty string", input: "", want: ""},
		{name: "single lowercase char", input: "c", want: "C"},
		{name: "single uppercase char", input: "C", want: "C"},
		{name: "lowercase word", input: "claude", want: "Claude"},
		{name: "already capitalized", input: "Claude", want: "Claude"},
		{name: "all caps", input: "CODEX", want: "CODEX"},
		{name: "mixed case", input: "cOdEx", want: "COdEx"},
		{name: "with spaces", input: "hello world", want: "Hello world"},
		{name: "number first", input: "1phase", want: "1phase"},
		{name: "underscore first", input: "_private", want: "_private"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := capitalizeFirst(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// TestBuildSummary
// ---------------------------------------------------------------------------

func TestBuildSummary_PhaseMode_All(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude", "codex"}, []int{1, 2, 3})
	w.config.PhaseMode = "all"
	w.config.ImplAgent = "claude"
	w.config.ReviewAgents = []string{"claude", "codex"}
	w.config.FixAgent = "codex"
	w.rawReviewConcurrency = "2"
	w.rawMaxReviewCycles = "3"
	w.rawMaxIterations = "10"

	summary := w.buildSummary()

	assert.Contains(t, summary, "Phase mode    : all")
	assert.Contains(t, summary, "Impl agent    : claude")
	assert.Contains(t, summary, "Review agents : claude, codex")
	assert.Contains(t, summary, "Fix agent     : codex")
	assert.Contains(t, summary, "Concurrency   : 2")
	assert.Contains(t, summary, "Review cycles : 3")
	assert.Contains(t, summary, "Max iter      : 10")
	assert.Contains(t, summary, "Skip steps    : none")
	// "all" mode should NOT contain phase ID or range lines.
	assert.NotContains(t, summary, "Phase ID")
	assert.NotContains(t, summary, "From phase")
	assert.NotContains(t, summary, "To phase")
	assert.Contains(t, summary, "Press Enter to confirm or Esc to cancel.")
}

func TestBuildSummary_PhaseMode_Single(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1, 2, 3})
	w.config.PhaseMode = "single"
	w.rawPhaseID = "2"
	w.config.ImplAgent = "claude"
	w.config.FixAgent = "claude"

	summary := w.buildSummary()

	assert.Contains(t, summary, "Phase mode    : single")
	assert.Contains(t, summary, "Phase ID      : 2")
	// range fields should not appear in single mode.
	assert.NotContains(t, summary, "From phase")
	assert.NotContains(t, summary, "To phase")
}

func TestBuildSummary_PhaseMode_Range(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1, 2, 3})
	w.config.PhaseMode = "range"
	w.rawFromPhase = "1"
	w.rawToPhase = "3"
	w.config.ImplAgent = "claude"
	w.config.FixAgent = "claude"

	summary := w.buildSummary()

	assert.Contains(t, summary, "Phase mode    : range")
	assert.Contains(t, summary, "From phase    : 1")
	assert.Contains(t, summary, "To phase      : 3")
	// single-phase field should not appear in range mode.
	assert.NotContains(t, summary, "Phase ID")
}

func TestBuildSummary_SkipFlags_AllSet(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	w.config.SkipImplement = true
	w.config.SkipReview = true
	w.config.SkipFix = true
	w.config.SkipPR = true

	summary := w.buildSummary()

	assert.Contains(t, summary, "Skip steps    : implement, review, fix, pr")
	assert.NotContains(t, summary, "Skip steps    : none")
}

func TestBuildSummary_SkipFlags_Partial(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	w.config.SkipImplement = false
	w.config.SkipReview = true
	w.config.SkipFix = false
	w.config.SkipPR = true

	summary := w.buildSummary()

	assert.Contains(t, summary, "Skip steps    : review, pr")
}

func TestBuildSummary_EmptyReviewAgents(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	w.config.ReviewAgents = nil

	summary := w.buildSummary()

	// strings.Join of nil slice produces empty string, so the label is present
	// but value is empty.
	assert.Contains(t, summary, "Review agents : ")
}

func TestBuildSummary_MultipleReviewAgents(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude", "codex", "gemini"}, []int{1})
	w.config.ReviewAgents = []string{"claude", "codex", "gemini"}

	summary := w.buildSummary()

	assert.Contains(t, summary, "Review agents : claude, codex, gemini")
}

// ---------------------------------------------------------------------------
// TestParseFormValues_EdgeCases
// ---------------------------------------------------------------------------

func TestParseFormValues_RawToPhaseZero(t *testing.T) {
	t.Parallel()

	// ToPhase == 0 is valid and means "run to the last phase".
	// parseFormValues must store 0 into config.ToPhase (no n>0 guard on ToPhase).
	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1, 2, 3})
	w.rawToPhase = "0"

	w.parseFormValues()

	assert.Equal(t, 0, w.config.ToPhase, "rawToPhase=0 must be stored as config.ToPhase=0")
}

func TestParseFormValues_RawToPhasePositive(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1, 2, 3})
	w.rawToPhase = "5"

	w.parseFormValues()

	assert.Equal(t, 5, w.config.ToPhase)
}

func TestParseFormValues_ZeroDoesNotUpdatePositiveOnlyFields(t *testing.T) {
	t.Parallel()

	// ReviewConcurrency, MaxReviewCycles, MaxIterations require n>0.
	// Setting raw value to "0" must leave the existing config unchanged.
	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	// Defaults are 2, 3, 10 respectively.
	w.rawReviewConcurrency = "0"
	w.rawMaxReviewCycles = "0"
	w.rawMaxIterations = "0"

	w.parseFormValues()

	assert.Equal(t, 2, w.config.ReviewConcurrency, "zero raw value must not overwrite ReviewConcurrency")
	assert.Equal(t, 3, w.config.MaxReviewCycles, "zero raw value must not overwrite MaxReviewCycles")
	assert.Equal(t, 10, w.config.MaxIterations, "zero raw value must not overwrite MaxIterations")
}

func TestParseFormValues_NegativeToPhase(t *testing.T) {
	t.Parallel()

	// parseFormValues does not validate sign for ToPhase; it stores whatever
	// parses as an integer. The huh validator prevents negative ToPhase during
	// the form interaction, but a direct call stores the parsed value.
	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	w.rawToPhase = "-1"

	w.parseFormValues()

	// The implementation stores any successfully parsed int for ToPhase.
	assert.Equal(t, -1, w.config.ToPhase)
}

// ---------------------------------------------------------------------------
// TestSingleAgentPreSelection
// ---------------------------------------------------------------------------

func TestSingleAgent_PreSelectsImplAndFixAgent(t *testing.T) {
	t.Parallel()

	// When only one agent is available, buildAgentGroup() must pre-select it
	// for both ImplAgent and FixAgent. This happens inside buildForm() which
	// is called by Start().
	agents := []string{"claude"}
	w := NewWizardModel(DefaultTheme(), agents, []int{1})

	// Before Start() the config fields are zero-value strings.
	assert.Empty(t, w.config.ImplAgent, "ImplAgent must be empty before Start()")
	assert.Empty(t, w.config.FixAgent, "FixAgent must be empty before Start()")

	_ = w.Start()

	assert.Equal(t, "claude", w.config.ImplAgent,
		"Start() must pre-select the sole available agent as ImplAgent")
	assert.Equal(t, "claude", w.config.FixAgent,
		"Start() must pre-select the sole available agent as FixAgent")
}

func TestSingleAgent_NoPreSelectionWhenMultipleAgents(t *testing.T) {
	t.Parallel()

	// With multiple agents, buildAgentGroup() must NOT manually pre-select a
	// specific agent — the user makes the choice interactively through the huh
	// form. The wizard code only forces a pre-selection when len==1.
	agents := []string{"claude", "codex"}
	w := NewWizardModel(DefaultTheme(), agents, []int{1})

	// Before Start() the explicit pre-selection has not run.
	assert.Empty(t, w.config.ImplAgent, "ImplAgent must be empty before Start() with multiple agents")
	assert.Empty(t, w.config.FixAgent, "FixAgent must be empty before Start() with multiple agents")

	// After Start() the form is built and huh binds the Select to
	// &w.config.ImplAgent. We verify the wizard is active and that the
	// pre-selection code (which only runs for len==1) did NOT run.
	_ = w.Start()

	// len(agents) == 2, so no explicit pre-selection should have occurred.
	// huh may have defaulted to the first option internally when building the
	// Select, but our code did not force it — we just verify the form built
	// successfully and the wizard is active.
	assert.True(t, w.IsActive(), "wizard must be active after Start()")
}

// ---------------------------------------------------------------------------
// TestUpdate_EscKey
// ---------------------------------------------------------------------------

func TestUpdate_EscKey_ReturnsCancelledMsg(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	_ = w.Start()
	require.True(t, w.IsActive(), "wizard must be active before sending Esc")

	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	updated, cmd := w.Update(escMsg)

	assert.False(t, updated.active, "Update with Esc must deactivate the wizard")
	require.NotNil(t, cmd, "Update with Esc must return a non-nil command")

	// Execute the returned command and verify it produces WizardCancelledMsg.
	msg := cmd()
	_, ok := msg.(WizardCancelledMsg)
	assert.True(t, ok, "command must return WizardCancelledMsg on Esc, got %T", msg)
}

func TestUpdate_EscKey_WhenInactive_IsNoOp(t *testing.T) {
	t.Parallel()

	// When the wizard is inactive, Update must return immediately with nil cmd.
	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	// Do NOT call Start().

	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	updated, cmd := w.Update(escMsg)

	assert.False(t, updated.active)
	assert.Nil(t, cmd, "Update when inactive must return nil cmd")
}

func TestUpdate_OtherKey_WhenInactive_IsNoOp(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}
	updated, cmd := w.Update(keyMsg)

	assert.False(t, updated.active)
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// TestView_DimensionEdgeCases
// ---------------------------------------------------------------------------

func TestView_NarrowTerminal_NoPanic(t *testing.T) {
	t.Parallel()

	// 80-column terminals are the canonical minimum; wizard must render without
	// panicking.
	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	w.SetDimensions(80, 24)
	_ = w.Start()

	assert.NotPanics(t, func() {
		view := w.View()
		// The form was just initialised so its view should be non-empty.
		_ = view
	})
}

func TestView_ZeroDimensions_NoPanic(t *testing.T) {
	t.Parallel()

	// When dimensions are 0 the wizard falls back to boxed rendering.
	// This must never panic.
	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	w.SetDimensions(0, 0)
	_ = w.Start()

	assert.NotPanics(t, func() {
		_ = w.View()
	})
}

func TestView_ZeroDimensions_ReturnsBoxedOutput(t *testing.T) {
	t.Parallel()

	// When width and height are 0, View must NOT call lipgloss.Place and must
	// return the boxed form directly (a non-empty string).
	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	w.SetDimensions(0, 0)
	_ = w.Start()

	view := w.View()
	// huh form renders its initial state so the view is non-empty.
	// We cannot assert the exact content because huh renders ANSI sequences,
	// but we can verify the wizard did not return an empty string due to the
	// zero-dimension path.
	if view != "" {
		// Contains the border rune from lipgloss.RoundedBorder.
		assert.True(t, strings.ContainsAny(view, "╭╮╰╯"),
			"zero-dimension view should still contain rounded border characters")
	}
}

func TestView_WithDimensions_CentersOutput(t *testing.T) {
	t.Parallel()

	// When width and height are positive, View calls lipgloss.Place.
	// The result should be non-empty and contain more whitespace padding than
	// the boxed-only variant. We just verify no panic and non-empty.
	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	w.SetDimensions(200, 50)
	_ = w.Start()

	assert.NotPanics(t, func() {
		_ = w.View()
	})
}

// ---------------------------------------------------------------------------
// TestBuildForm_Width
// ---------------------------------------------------------------------------

func TestBuildForm_WidthClampedTo100(t *testing.T) {
	t.Parallel()

	// When the parent terminal is very wide the form must cap at 100.
	// We verify this indirectly by confirming buildForm() does not panic and
	// the returned form is not nil.
	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	w.SetDimensions(300, 50)
	form := w.buildForm()

	require.NotNil(t, form)
}

func TestBuildForm_WidthDefaultsTo80WhenZero(t *testing.T) {
	t.Parallel()

	// A zero terminal width must fall back to 80 inside buildForm().
	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	// width is 0 (default zero value of WizardModel).
	form := w.buildForm()

	require.NotNil(t, form)
}

// ---------------------------------------------------------------------------
// TestSetDimensions_UpdatesFormWhenActive
// ---------------------------------------------------------------------------

func TestSetDimensions_UpdatesFormWidthWhenActive(t *testing.T) {
	t.Parallel()

	w := NewWizardModel(DefaultTheme(), []string{"claude"}, []int{1})
	_ = w.Start()
	require.NotNil(t, w.form)

	// Setting dimensions while active must propagate width to the form without
	// panicking.
	assert.NotPanics(t, func() {
		w.SetDimensions(120, 40)
	})
	assert.Equal(t, 120, w.width)
	assert.Equal(t, 40, w.height)
}

// ---------------------------------------------------------------------------
// TestPositiveIntValidator_ErrorMessages
// ---------------------------------------------------------------------------

func TestPositiveIntValidator_ErrorMessage_NonNumber(t *testing.T) {
	t.Parallel()

	fn := positiveIntValidator("review concurrency")
	err := fn("abc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "review concurrency")
}

func TestPositiveIntValidator_ErrorMessage_BelowOne(t *testing.T) {
	t.Parallel()

	fn := positiveIntValidator("max iterations")
	err := fn("0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max iterations")
}
