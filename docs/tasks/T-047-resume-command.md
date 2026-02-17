# T-047: Resume Command -- List and Resume Interrupted Workflows

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-006, T-045, T-046 |
| Blocked By | T-046 |
| Blocks | T-050 |

## Goal
Implement the `raven resume` CLI command that lists all resumable workflow runs and resumes a specific interrupted workflow from its last checkpoint. This is the user-facing interface to the checkpoint/resume system, providing the "zero-restart" experience for failed or interrupted workflows.

## Background
Per PRD Section 5.1:
- `raven resume [--run <id>]` continues from the last checkpoint
- `raven resume --list` shows all resumable workflow runs
- Resumability target: "< 5 seconds to resume" (PRD Section 4)

Per PRD Section 5.11, `raven resume` is a core CLI command. When invoked without `--run`, it resumes the most recently interrupted workflow. With `--list`, it displays a table of all persisted runs with their workflow name, current step, status, and last update time.

The command loads the persisted `WorkflowState`, resolves the `WorkflowDefinition` (by workflow name from state), and passes both to the workflow engine to continue execution from `state.CurrentStep`.

## Technical Specifications
### Implementation Approach
Create `internal/cli/resume.go` with a Cobra command that supports three modes: (1) `raven resume` with no flags resumes the most recent interrupted run, (2) `raven resume --run <id>` resumes a specific run by ID, (3) `raven resume --list` displays all runs. The command uses `StateStore` (T-046) to load/list runs and the workflow `Engine` (T-045) to execute the resumed workflow. It resolves the workflow definition from the built-in definitions registry (T-049) using `state.WorkflowName`.

### Key Components
- **resumeCmd**: Cobra command for `raven resume`
- **List mode**: Formats and displays all runs from StateStore
- **Resume mode**: Loads state, resolves definition, runs engine
- **Output formatting**: Table format for `--list`, status messages for resume

### API/Interface Contracts
```go
// internal/cli/resume.go

func newResumeCmd() *cobra.Command
// Returns a cobra.Command with:
//   Usage: "resume"
//   Short: "Resume an interrupted workflow"
//   Flags:
//     --run <id>     Resume a specific workflow run
//     --list         List all resumable workflow runs
//     --dry-run      Show what would be resumed without executing
//     --clean <id>   Delete a checkpoint (cleanup)
//     --clean-all    Delete all checkpoints

// The command performs:
// 1. If --list: call StateStore.List(), format as table, print to stdout
// 2. If --run <id>: call StateStore.Load(id), resolve definition, run engine
// 3. If neither: call StateStore.LatestRun(), resume that
// 4. If --clean: call StateStore.Delete(id)
// 5. If --clean-all: delete all checkpoints
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| spf13/cobra | v1.10+ | CLI command framework |
| internal/workflow (T-045) | - | Engine for executing resumed workflow |
| internal/workflow (T-046) | - | StateStore for loading/listing checkpoints |
| internal/workflow (T-049) | - | Built-in workflow definitions |
| internal/config (T-009) | - | Config loading for state directory path |
| charmbracelet/lipgloss | v1.0+ | Table formatting for --list output |
| charmbracelet/log | latest | Structured logging |

## Acceptance Criteria
- [ ] `raven resume --list` displays a formatted table of all resumable runs
- [ ] Table includes: Run ID, Workflow Name, Current Step, Status, Last Updated, Steps Completed
- [ ] `raven resume --run <id>` loads checkpoint and resumes execution
- [ ] `raven resume` (no flags) resumes the most recently updated run
- [ ] Resume starts execution from the persisted `CurrentStep`
- [ ] `--dry-run` shows what would be resumed without executing
- [ ] `--clean <id>` deletes a specific checkpoint
- [ ] `--clean-all` deletes all checkpoints with confirmation prompt
- [ ] Error message when no runs exist: "No resumable workflow runs found"
- [ ] Error message when specified run ID does not exist
- [ ] Exit code 0 on success, 1 on error
- [ ] All status/progress output goes to stderr, structured output to stdout
- [ ] Unit tests achieve 85% coverage

## Testing Requirements
### Unit Tests
- List mode with 0 runs: prints "No resumable workflow runs found"
- List mode with 3 runs: prints formatted table sorted by UpdatedAt desc
- Resume with --run <id>: loads correct state, engine receives it
- Resume without --run: loads latest run
- Resume nonexistent run ID: returns error
- Dry-run mode: shows planned resumption without executing
- Clean mode: deletes specified checkpoint
- Clean-all mode: deletes all checkpoints
- Workflow name in state references a registered built-in definition

### Integration Tests
- Full cycle: start workflow, interrupt (context cancel), resume, verify completion
- List after creating multiple workflows: all appear in list

### Edge Cases to Handle
- Run with status "completed" (should warn: "Run <id> already completed")
- Run whose workflow definition no longer exists (definition was removed between versions)
- State directory does not exist yet (first use -- StateStore creates it)
- Corrupt checkpoint file (log warning, skip in list, error on resume)
- Very long run IDs (truncate in table display)

## Implementation Notes
### Recommended Approach
1. Create `newResumeCmd()` returning `*cobra.Command` with `--run`, `--list`, `--dry-run`, `--clean`, `--clean-all` flags
2. In RunE, load config to get state directory path (default `.raven/state/`)
3. Create `StateStore` from config path
4. Branch on flags: list, clean, or resume
5. For list: use `lipgloss` or `text/tabwriter` for table formatting
6. For resume: load state, look up workflow definition by `state.WorkflowName` from built-in definitions, create engine, call `engine.Run(ctx, def, state)`
7. Register command with root cmd in `root.go`

### Potential Pitfalls
- The workflow definition must be resolved by name -- if custom workflow definitions change between runs, the resumed workflow may behave differently. Document this behavior.
- Ensure the resume command re-applies the same engine options (checkpointing, event channel) as the original run
- `--clean-all` should prompt for confirmation in interactive terminals (check `os.Stdin` is terminal)
- The table formatting must handle terminal width -- truncate long fields

### Security Considerations
- Checkpoint files may contain sensitive metadata -- `--clean-all` should actually delete files, not just unlink
- Do not allow `--run` flag to specify arbitrary file paths (validate it is a simple ID, not a path)

## References
- [PRD Section 5.1 - raven resume](docs/prd/PRD-Raven.md)
- [PRD Section 5.11 - CLI commands](docs/prd/PRD-Raven.md)
- [spf13/cobra documentation](https://pkg.go.dev/github.com/spf13/cobra)
- [charmbracelet/lipgloss table rendering](https://github.com/charmbracelet/lipgloss)