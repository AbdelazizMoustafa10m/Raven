# T-054: Pipeline and Workflow Dry-Run Mode

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-045, T-049, T-050 |
| Blocked By | T-045, T-050 |
| Blocks | T-055 |

## Goal
Implement comprehensive dry-run support for all workflows and the pipeline orchestrator. When `--dry-run` is passed, Raven shows the planned execution steps, branch names, agent commands, and workflow transitions without executing any step handlers, creating branches, or invoking agents. This gives users confidence in their configuration before committing to a potentially long-running pipeline.

## Background
Per PRD Section 5.1, the workflow engine "Supports `--dry-run` on any workflow to show planned steps without execution." Per Section 5.9, the pipeline orchestrator also has "Dry-run mode: shows planned pipeline steps without execution." Per Section 5.4, the implementation loop supports "`--dry-run` mode: generates and displays the prompt without invoking the agent."

Dry-run is already partially supported by the workflow engine (T-045, which calls `StepHandler.DryRun()` instead of `Execute()`). This task ensures dry-run works end-to-end across the pipeline orchestrator, produces well-formatted output, and covers all subsystems.

## Technical Specifications
### Implementation Approach
Implement dry-run at two levels: (1) workflow engine level -- already supported via `WithDryRun(true)` in T-045, which calls `DryRun()` on each step handler; (2) pipeline orchestrator level -- shows planned phases, branch names, and per-phase workflow without executing. Create a `DryRunFormatter` that produces a structured, readable output of the planned execution.

### Key Components
- **DryRunFormatter**: Formats dry-run output with indentation, sections, and visual hierarchy
- **Pipeline dry-run**: Shows phases, branch names, skip flags, agent assignments
- **Workflow dry-run**: Shows step sequence, transitions, handler descriptions
- **Step handler DryRun implementations**: Each handler returns a description of what it would do
- **Output format**: Structured text to stderr with lipgloss styling

### API/Interface Contracts
```go
// internal/workflow/dryrun.go

// DryRunFormatter formats dry-run output for display.
type DryRunFormatter struct {
    writer io.Writer
    styled bool // use lipgloss styling
}

func NewDryRunFormatter(w io.Writer, styled bool) *DryRunFormatter

// FormatWorkflowDryRun formats the dry-run output for a single workflow.
func (f *DryRunFormatter) FormatWorkflowDryRun(def *WorkflowDefinition, state *WorkflowState, stepOutputs map[string]string) string

// FormatPipelineDryRun formats the dry-run output for a full pipeline.
func (f *DryRunFormatter) FormatPipelineDryRun(meta *pipeline.PipelineMetadata, phaseDetails []PhaseDryRunDetail) string

// PhaseDryRunDetail contains dry-run info for a single phase.
type PhaseDryRunDetail struct {
    PhaseID     int
    PhaseName   string
    BranchName  string
    Skipped     []string // list of skipped stages
    ImplAgent   string
    ReviewAgent string
    FixAgent    string
    Steps       []StepDryRunDetail
}

// StepDryRunDetail contains dry-run info for a single step.
type StepDryRunDetail struct {
    StepName    string
    Description string // from StepHandler.DryRun()
    Transitions map[string]string // event -> next step
}
```

Example dry-run output:
```
Pipeline Dry Run
================

Phase 1: Foundation & Setup
  Branch: phase/1-foundation-setup (from main)
  Implementation: claude (claude-opus-4-6)
  Review: codex (gpt-5.3-codex)
  Fix: claude (claude-opus-4-6)
  
  Steps:
    1. run_implement: Run implementation loop for phase 1 tasks (T-001 to T-010)
       -> success: run_review
       -> failure: FAILED
    2. run_review: Run parallel review with codex (concurrency: 4)
       -> success: check_review
       -> failure: FAILED
    3. check_review: Check review verdict
       -> success (approved): create_pr
       -> needs_human (changes): run_fix
    4. run_fix: Apply review fixes with claude
       -> success: run_review (cycle back)
       -> failure: FAILED
    5. create_pr: Create pull request
       -> success: DONE
       -> failure: FAILED

Phase 2: Core Implementation
  Branch: phase/2-core-implementation (from phase/1-foundation-setup)
  [SKIPPED: --skip-review, --skip-fix]
  ...

Total phases: 5
Estimated branches: 5
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/workflow (T-045) | - | Engine dry-run mode |
| internal/pipeline (T-050) | - | PipelineOrchestrator, PipelineOpts |
| internal/pipeline (T-051) | - | Branch name resolution |
| internal/pipeline (T-052) | - | PipelineMetadata for summary |
| charmbracelet/lipgloss | v1.0+ | Styled output formatting |
| io | stdlib | Writer interface |

## Acceptance Criteria
- [ ] `--dry-run` on any workflow shows step sequence with handler descriptions
- [ ] `--dry-run` on pipeline shows all phases with branch names and agent assignments
- [ ] Skipped stages are clearly marked in dry-run output
- [ ] Step transitions are shown for each step (event -> next step)
- [ ] Agent names and models are included in step descriptions
- [ ] Branch names are resolved from template and shown for each phase
- [ ] Branch chaining is visible (each phase shows its base branch)
- [ ] Output is styled when connected to a terminal, plain text when piped
- [ ] DryRun output goes to stderr (stdout remains clean for piping)
- [ ] No external processes are invoked during dry-run (no git, no agents, no gh)
- [ ] Unit tests achieve 85% coverage

## Testing Requirements
### Unit Tests
- FormatWorkflowDryRun with 3-step workflow produces correct output
- FormatPipelineDryRun with 3 phases produces correct output
- Skipped stages appear as [SKIPPED] in output
- Branch names include correct base branch for chaining
- Styled output contains lipgloss formatting
- Plain output (styled=false) contains no ANSI codes
- Empty workflow (no steps) produces appropriate message
- Single-phase pipeline produces correct output

### Integration Tests
- Pipeline command with --dry-run: verify output format matches specification
- Workflow command with --dry-run: verify output format

### Edge Cases to Handle
- Phase with all stages skipped: shows "Phase N: [ALL STAGES SKIPPED]"
- Custom workflow from config: dry-run works with TOML-defined workflows
- Very long phase names: truncated in output
- Terminal width < 80: output wraps gracefully

## Implementation Notes
### Recommended Approach
1. Create `DryRunFormatter` in `internal/workflow/dryrun.go`
2. For pipeline dry-run:
   a. Resolve all phase branch names using BranchManager.ResolveBranchName (no git calls)
   b. For each phase, construct PhaseDryRunDetail with agents, skips, and step details
   c. For step details, call `handler.DryRun(state)` on each handler with simulated state
   d. Format and print
3. For workflow dry-run:
   a. Walk the definition graph from initial step
   b. For each step, call `handler.DryRun(state)` to get description
   c. Show transitions for each step
   d. Format and print
4. Use lipgloss for headers, indentation, and emphasis (bold for step names, dim for transitions)
5. Detect terminal capability: `lipgloss.HasDarkBackground()` for color scheme

### Potential Pitfalls
- DryRun graph walk may encounter cycles (review-fix loop). Show cycles with an indicator like "(cycles back to step N)" rather than infinite recursion
- Step handlers' DryRun() methods need access to WorkflowState.Metadata to produce meaningful descriptions. Create a synthetic state with expected metadata keys populated.
- Do not call any StepHandler.Execute() during dry-run -- only DryRun()
- Ensure dry-run output is deterministic (no timestamps, no random IDs) for golden testing

### Security Considerations
- Dry-run should not reveal any secrets from config (API keys, tokens)
- Agent commands shown in dry-run may include prompt file paths -- ensure these are relative paths, not absolute

## References
- [PRD Section 5.1 - --dry-run on any workflow](docs/prd/PRD-Raven.md)
- [PRD Section 5.9 - Pipeline dry-run mode](docs/prd/PRD-Raven.md)
- [PRD Section 5.4 - Implementation loop --dry-run](docs/prd/PRD-Raven.md)
- [charmbracelet/lipgloss styling](https://github.com/charmbracelet/lipgloss)