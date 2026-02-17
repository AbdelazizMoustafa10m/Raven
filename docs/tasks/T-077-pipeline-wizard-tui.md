# T-077: Pipeline Wizard TUI Integration (charmbracelet/huh)

## Metadata
| Field | Value |
|-------|-------|
| Priority | Should Have |
| Estimated Effort | Medium: 8-12hrs |
| Dependencies | T-066, T-068, T-069, T-076 |
| Blocked By | T-066 |
| Blocks | T-078 |

## Goal
Implement the pipeline wizard TUI component in `internal/tui/wizard.go` using `charmbracelet/huh` forms, allowing users to configure and launch pipelines interactively from within the TUI dashboard. The wizard collects pipeline configuration (phase selection, agent selection, review settings, skip flags) through a multi-step form and produces a `PipelineConfig` that can be passed to the pipeline orchestrator.

## Background
Per PRD Section 5.9, `raven pipeline --interactive` launches a guided wizard for pipeline configuration. Per PRD Section 5.12, the pipeline wizard can be launched from the TUI dashboard. The `charmbracelet/huh` library provides form-based input collection that integrates with Bubble Tea's Elm architecture.

The wizard replaces the bash prototype's `gum`-based wizard with a native Go implementation. It collects:
1. Phase selection (single phase, range, or all)
2. Implementation agent selection
3. Review agent(s) selection with concurrency setting
4. Fix agent selection
5. Skip flags (skip-implement, skip-review, skip-fix, skip-pr)
6. Max review cycles and max iterations
7. Confirmation before launch

The `huh` library supports running forms as Bubble Tea sub-models via `huh.NewForm().WithShowHelp(true)`, making them embeddable in the existing TUI layout.

## Technical Specifications
### Implementation Approach
Create `internal/tui/wizard.go` containing a `WizardModel` that wraps a `huh.Form`. The wizard is a modal overlay that takes over the main content area when active. On completion, it emits a `WizardCompleteMsg` containing the collected pipeline configuration. On cancellation (Esc), it emits a `WizardCancelledMsg`. The wizard uses the TUI theme for consistent styling via `huh.ThemeCustom`.

### Key Components
- **WizardModel**: Bubble Tea sub-model wrapping the huh form
- **PipelineWizardConfig**: Result struct containing all wizard selections
- **WizardCompleteMsg**: Message emitted when wizard completes successfully
- **WizardCancelledMsg**: Message emitted when wizard is cancelled
- **Custom huh theme**: Maps the Raven TUI theme to huh's theme system

### API/Interface Contracts
```go
// internal/tui/wizard.go

import (
    "github.com/charmbracelet/huh"
    tea "github.com/charmbracelet/bubbletea"
)

// PipelineWizardConfig holds the configuration collected by the wizard.
type PipelineWizardConfig struct {
    // Phase selection
    PhaseMode    string   // "single", "range", "all"
    PhaseID      int      // for single mode
    FromPhase    int      // for range mode
    ToPhase      int      // for range mode (0 = to end)

    // Agent selection
    ImplAgent    string   // e.g., "claude"
    ReviewAgents []string // e.g., ["claude", "codex"]
    FixAgent     string   // e.g., "claude"

    // Review settings
    ReviewConcurrency int
    MaxReviewCycles   int

    // Implementation settings
    MaxIterations int

    // Skip flags
    SkipImplement bool
    SkipReview    bool
    SkipFix       bool
    SkipPR        bool
}

// WizardCompleteMsg is sent when the wizard completes successfully.
type WizardCompleteMsg struct {
    Config PipelineWizardConfig
}

// WizardCancelledMsg is sent when the wizard is cancelled.
type WizardCancelledMsg struct{}

// WizardModel manages the pipeline wizard form.
type WizardModel struct {
    theme    Theme
    form     *huh.Form
    width    int
    height   int
    active   bool
    config   PipelineWizardConfig

    // Available options (populated from raven.toml config)
    availableAgents []string
    availablePhases []int
}

// NewWizardModel creates a wizard with available agents and phases.
func NewWizardModel(theme Theme, agents []string, phases []int) WizardModel

// SetDimensions updates the wizard display area.
func (w *WizardModel) SetDimensions(width, height int)

// Start activates the wizard and initializes the form.
func (w *WizardModel) Start() tea.Cmd

// IsActive returns whether the wizard is currently displayed.
func (w WizardModel) IsActive() bool

// Update processes form input events.
func (w WizardModel) Update(msg tea.Msg) (WizardModel, tea.Cmd)

// View renders the wizard form.
func (w WizardModel) View() string

// buildForm constructs the huh.Form with all wizard fields.
func (w *WizardModel) buildForm() *huh.Form

// buildHuhTheme creates a huh theme from the Raven TUI theme.
func buildHuhTheme(theme Theme) *huh.Theme
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| charmbracelet/huh | v0.6+ | Form/wizard builder |
| charmbracelet/bubbletea | v1.2+ | Elm architecture integration |
| charmbracelet/lipgloss | v1.0+ | Theme mapping |
| internal/tui (T-068) | - | Theme styles |

## Acceptance Criteria
- [ ] Wizard presents a multi-group form with logical sections
- [ ] Group 1: Phase selection (select: single/range/all, then conditional inputs for phase ID or range)
- [ ] Group 2: Agent selection (select for impl agent, multi-select for review agents, select for fix agent)
- [ ] Group 3: Settings (number inputs for concurrency, max review cycles, max iterations)
- [ ] Group 4: Skip flags (confirm/toggle for each skip option)
- [ ] Group 5: Confirmation summary showing all selections
- [ ] Form uses the Raven TUI theme for consistent styling
- [ ] Completing the form emits `WizardCompleteMsg` with the collected config
- [ ] Pressing Esc at any point emits `WizardCancelledMsg`
- [ ] Wizard takes over the main content area while active
- [ ] Normal TUI key bindings (q, p, s) are suppressed while wizard is active
- [ ] Available agents and phases are populated from the raven.toml configuration
- [ ] Unit tests achieve 80% coverage (form interaction testing is limited)

## Testing Requirements
### Unit Tests
- `NewWizardModel` creates model with correct available agents and phases
- `buildForm` returns a non-nil huh.Form
- `IsActive` returns false initially, true after `Start()`
- `PipelineWizardConfig` fields default to sensible values
- `buildHuhTheme` returns a non-nil huh.Theme
- `WizardCompleteMsg` carries the expected config after form completion
- Available agents list is reflected in form select options
- Available phases list is reflected in form select options

### Integration Tests
- Instantiate wizard, verify it renders without panic at various terminal sizes
- Verify form group structure matches expected wizard flow

### Edge Cases to Handle
- No agents configured (show error message, do not show empty select)
- No phases configured (show error message)
- Single agent available (pre-select it, skip agent selection group)
- Very narrow terminal (wizard should still be usable at 80 chars wide)
- User navigates back through form groups (huh supports this natively)
- Wizard cancelled on first group (no partial config should leak)

## Implementation Notes
### Recommended Approach
1. Define `PipelineWizardConfig` struct with all config fields
2. Define `WizardCompleteMsg` and `WizardCancelledMsg` message types
3. Implement `buildHuhTheme` mapping Raven's Theme colors to huh's theme fields:
   ```go
   func buildHuhTheme(theme Theme) *huh.Theme {
       t := huh.ThemeBase()
       t.Focused.Title = theme.SidebarTitle
       t.Focused.Description = lipgloss.NewStyle().Foreground(ColorMuted)
       // ... map other fields
       return t
   }
   ```
4. Implement `buildForm` constructing the multi-group form:
   ```go
   huh.NewForm(
       huh.NewGroup(
           huh.NewSelect[string]().
               Title("Phase Selection").
               Options(
                   huh.NewOption("Single Phase", "single"),
                   huh.NewOption("Phase Range", "range"),
                   huh.NewOption("All Phases", "all"),
               ).
               Value(&w.config.PhaseMode),
       ),
       huh.NewGroup(
           huh.NewSelect[string]().
               Title("Implementation Agent").
               Options(agentOptions...).
               Value(&w.config.ImplAgent),
           huh.NewMultiSelect[string]().
               Title("Review Agents").
               Options(agentOptions...).
               Value(&w.config.ReviewAgents),
       ),
       // ... more groups
   ).WithTheme(buildHuhTheme(w.theme)).
     WithWidth(w.width).
     WithHeight(w.height)
   ```
5. Implement `Start()`:
   - Set `active = true`
   - Build the form
   - Return the form's `Init()` command
6. Implement `Update`:
   - If form completed (check form state), emit `WizardCompleteMsg`
   - If Esc pressed, emit `WizardCancelledMsg`
   - Otherwise, forward to form's Update
7. Implement `View`: return form.View() padded to dimensions
8. In `App.Update`, when wizard is active:
   - Only forward messages to wizard, not to other panels
   - On `WizardCompleteMsg`, deactivate wizard, start pipeline
   - On `WizardCancelledMsg`, deactivate wizard

### Potential Pitfalls
- `huh.Form` is itself a `tea.Model` in Bubble Tea mode. Use `form.Run()` for standalone or embed via `form.Init()`/`form.Update()`/`form.View()` when used as a sub-model.
- The huh form handles its own key events (Enter to confirm, Esc to cancel, Tab to next field). Do NOT intercept these in the App's key handler while the wizard is active.
- `huh.NewMultiSelect` returns a `[]string` value. Initialize the value pointer before building the form.
- Form width and height must be set to fit within the available panel area. Use the main area dimensions (width - sidebar width).
- When the wizard is active, the sidebar and status bar should still be visible, but the event log and agent panel are replaced by the wizard.

### Security Considerations
- Agent names and phase numbers come from trusted config; no user input sanitization needed for these values
- The wizard does not directly invoke any agents or modify files

## References
- [charmbracelet/huh GitHub Repository](https://github.com/charmbracelet/huh)
- [huh Documentation](https://pkg.go.dev/github.com/charmbracelet/huh)
- [huh Bubble Tea Integration Example](https://github.com/charmbracelet/huh/tree/main/examples/bubbletea)
- [PRD Section 5.9 - Pipeline Interactive Wizard](docs/prd/PRD-Raven.md)
- [PRD Section 5.12 - Pipeline Wizard Integration](docs/prd/PRD-Raven.md)