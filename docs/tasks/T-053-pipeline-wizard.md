# T-053: Pipeline Interactive Wizard with charmbracelet/huh

## Metadata
| Field | Value |
|-------|-------|
| Priority | Should Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-050, T-009 |
| Blocked By | T-050 |
| Blocks | T-055 |

## Goal
Implement an interactive pipeline configuration wizard using charmbracelet/huh that guides users through pipeline setup when `raven pipeline --interactive` is used or when no flags are provided in an interactive terminal. The wizard collects phase selection, agent preferences, skip flags, concurrency settings, and review cycle limits through a guided form experience, then launches the pipeline with the collected configuration.

## Background
Per PRD Section 5.9, "Interactive wizard mode (TUI): when `--interactive` or no flags in a terminal, launches a guided wizard for pipeline configuration." The PRD specifies using `charmbracelet/huh` for form-based configuration (replacing the bash prototype's gum-based wizard).

The wizard should provide a smooth onboarding experience for users unfamiliar with all pipeline options. It collects the same information as CLI flags but with descriptions, defaults, and validation. After confirmation, it constructs `PipelineOpts` and passes them to the pipeline orchestrator.

## Technical Specifications
### Implementation Approach
Create `internal/pipeline/wizard.go` with a `RunWizard` function that constructs and runs a `huh.Form` with multiple groups (pages). The form collects: phase selection (select/multiselect), agent selection (per-stage dropdowns), skip flags (checkboxes), concurrency (number input), max review cycles (number input), and dry-run toggle. After the form completes, the wizard builds a `PipelineOpts` struct and returns it.

### Key Components
- **RunWizard**: Main function that runs the form and returns PipelineOpts
- **Phase selection group**: Select single phase, all phases, or from-phase
- **Agent selection group**: Per-stage agent selection from configured agents
- **Options group**: Skip flags, concurrency, max cycles, dry-run
- **Confirmation group**: Summary and confirm/cancel

### API/Interface Contracts
```go
// internal/pipeline/wizard.go

// WizardConfig provides the data needed to populate wizard choices.
type WizardConfig struct {
    Phases       []task.Phase   // Available phases from phases.conf
    Agents       []string       // Available agent names from config
    DefaultAgent string         // Default agent from config
    Config       *config.Config // Full config for defaults
}

// RunWizard displays the interactive pipeline wizard and returns configured options.
// Returns ErrWizardCancelled if the user cancels.
func RunWizard(cfg WizardConfig) (*PipelineOpts, error)

// ErrWizardCancelled is returned when the user cancels the wizard.
var ErrWizardCancelled = errors.New("wizard cancelled by user")

// The wizard flow:
// Page 1: Phase Selection
//   - "Which phases?" [All phases / Single phase / From phase]
//   - (conditional) Phase picker if single/from selected
//
// Page 2: Agent Configuration
//   - "Implementation agent:" [claude / codex / gemini]
//   - "Review agent:" [claude / codex / gemini]
//   - "Fix agent:" [claude / codex / gemini]
//
// Page 3: Pipeline Options
//   - "Review concurrency:" [1-10]
//   - "Max review/fix cycles:" [1-5]
//   - Skip toggles: [x] Skip implementation [ ] Skip review [ ] Skip fix [ ] Skip PR
//   - [ ] Dry run
//
// Page 4: Confirmation
//   - Summary of all selections
//   - [Run Pipeline] [Cancel]
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| charmbracelet/huh | v0.6+ | Interactive form framework |
| internal/pipeline (T-050) | - | PipelineOpts type |
| internal/task | - | Phase type for phase listing |
| internal/config (T-009) | - | Config for defaults and agent list |

## Acceptance Criteria
- [ ] Wizard launches when `raven pipeline --interactive` is used
- [ ] Wizard launches when `raven pipeline` is used with no flags in interactive terminal
- [ ] Phase selection offers: All phases, Single phase (with picker), From phase (with picker)
- [ ] Agent selection shows configured agents with defaults pre-selected
- [ ] Skip flags are presented as toggles with clear descriptions
- [ ] Concurrency and max cycles accept numeric input with validation (min/max)
- [ ] Confirmation page shows summary of all selections
- [ ] User can cancel the wizard (returns ErrWizardCancelled, exit code 3)
- [ ] Completed wizard returns correctly populated PipelineOpts
- [ ] Wizard works in terminals of varying widths (min 80 columns)
- [ ] Unit tests achieve 75% coverage (TUI interaction limits testability)

## Testing Requirements
### Unit Tests
- WizardConfig with 5 phases and 2 agents: verify form construction does not error
- Default values applied correctly from config
- ErrWizardCancelled returned on cancel (simulated via huh testing utilities)
- PipelineOpts correctly populated from form values
- Phase selection "all" sets PhaseID to "all"
- Phase selection "single" sets PhaseID to selected phase
- Phase selection "from" sets FromPhase to selected phase
- Agent selection populates ImplAgent, ReviewAgent, FixAgent
- Skip flags correctly mapped to PipelineOpts booleans

### Integration Tests
- Visual inspection test: wizard renders correctly (manual/golden test)

### Edge Cases to Handle
- No phases configured: wizard shows error and exits
- Only one agent configured: agent selection pre-filled, no dropdown
- Terminal too small: huh handles gracefully with scrolling
- User presses Ctrl+C: wizard exits cleanly with ErrWizardCancelled
- Zero phases selected: validation error before confirmation

## Implementation Notes
### Recommended Approach
1. Create form groups using `huh.NewGroup()`:
   - Group 1: `huh.NewSelect` for phase mode, conditional `huh.NewSelect` for phase picker
   - Group 2: Three `huh.NewSelect` for agent selection (impl, review, fix)
   - Group 3: `huh.NewMultiSelect` for skip flags, `huh.NewInput` for concurrency/cycles
   - Group 4: `huh.NewConfirm` with summary text
2. Use `huh.NewForm(groups...).Run()` to execute
3. After form completion, build `PipelineOpts` from collected values
4. Use `huh.ThemeCharm()` for consistent Charm styling
5. For conditional fields (phase picker only shown when not "all"), use dynamic form features or separate form runs

### Potential Pitfalls
- `huh` dynamic forms: conditionally showing/hiding fields based on previous answers requires `OptionsFunc`/`TitleFunc` (available in v0.6+). Test this pattern carefully.
- Numeric input for concurrency: `huh.NewInput` returns strings -- parse to int with validation
- The wizard must not block on non-interactive terminals -- check `os.Stdin` isatty before running
- `huh` v0.6+ vs v0.8+: API may have changed. The PRD specifies v0.6+; verify compatibility with the actual imported version.

### Security Considerations
- Wizard input is validated before use (numeric ranges, valid agent names)
- No sensitive data enters the wizard (no credentials, no secrets)

## References
- [PRD Section 5.9 - Interactive wizard mode](docs/prd/PRD-Raven.md)
- [charmbracelet/huh documentation](https://github.com/charmbracelet/huh)
- [charmbracelet/huh v0.6+ releases](https://github.com/charmbracelet/huh/releases)
- [huh form examples](https://github.com/charmbracelet/huh/tree/main/examples)