# T-020: Status Command with Progress Bars

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-006, T-009, T-016, T-017, T-018, T-019 |
| Blocked By | T-006, T-009, T-019 |
| Blocks | None |

## Goal
Implement the `raven status` CLI command that displays phase-by-phase task progress with progress bars, completion counts, and per-task status details. This provides developers with an at-a-glance view of project progress without needing to manually inspect task state files or count completed tasks.

## Background
Per PRD Section 5.3 and 5.11, `raven status` shows phase-by-phase progress with progress bars. This is a read-only command that loads task specs, task state, and phase configuration to compute and display progress statistics. The command outputs to stderr for progress/status display and can optionally output structured data to stdout for piping. Per PRD conventions, progress output uses `charmbracelet/lipgloss` for terminal styling and `charmbracelet/bubbles/progress` for progress bar rendering (static rendering via `ViewAs`, not animated).

## Technical Specifications
### Implementation Approach
Create `internal/cli/status.go` containing the Cobra command for `raven status`. The command loads configuration, discovers tasks, loads state, loads phases, and uses the TaskSelector's progress methods to compute statistics. Renders output using lipgloss for styling and the bubbles progress component's `ViewAs` method for static progress bar rendering. Supports `--phase <id>` to show a single phase and `--json` for structured output.

### Key Components
- **statusCmd**: Cobra command registered as a subcommand of root
- **renderPhaseProgress**: Formats a single phase's progress with progress bar
- **renderTaskList**: Formats individual task statuses within a phase
- **renderSummary**: Formats overall project summary

### API/Interface Contracts
```go
// internal/cli/status.go
package cli

import (
    "github.com/spf13/cobra"
)

// newStatusCmd creates the "raven status" command.
func newStatusCmd() *cobra.Command

// statusFlags holds the flag values for the status command.
type statusFlags struct {
    Phase   int    // --phase <id>, 0 means all phases
    JSON    bool   // --json for structured output
    Verbose bool   // --verbose for per-task details
}

// runStatus is the command's RunE function.
// Loads config, discovers tasks, computes progress, renders output.
func runStatus(cmd *cobra.Command, args []string, flags statusFlags) error
```

```go
// Output rendering (internal to status.go or a helper file)

// renderPhaseProgress returns a styled string for a single phase:
//
//   Phase 2: Core Implementation
//   ████████████░░░░░░░░ 60% (12/20)
//   In progress: T-013 (claude)
//   Blocked: T-015 (waiting on T-014)
//
func renderPhaseProgress(progress task.PhaseProgress, verbose bool) string

// renderSummary returns an overall summary:
//
//   Raven Status - my-project
//   ═══════════════════════════
//   Overall: 45/87 tasks completed (51%)
//   Current Phase: 3 - Review Pipeline
//
func renderSummary(allProgress []task.PhaseProgress, projectName string) string
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| spf13/cobra | v1.10+ | CLI command framework |
| charmbracelet/lipgloss | v1.0+ | Terminal styling |
| charmbracelet/bubbles/progress | latest | Progress bar ViewAs rendering |
| encoding/json | stdlib | --json output mode |
| internal/task (T-016) | - | DiscoverTasks |
| internal/task (T-017) | - | StateManager |
| internal/task (T-018) | - | LoadPhases |
| internal/task (T-019) | - | TaskSelector, PhaseProgress |
| internal/config (T-009) | - | Config loading |

## Acceptance Criteria
- [ ] `raven status` shows all phases with progress bars and completion counts
- [ ] Progress bars render correctly using bubbles progress component (ViewAs)
- [ ] Each phase shows: phase name, progress bar, percentage, completion fraction
- [ ] `--verbose` shows per-task status within each phase (task ID, title, status, agent)
- [ ] `--phase <id>` filters to a single phase
- [ ] `--json` outputs structured JSON to stdout (phases array with progress data)
- [ ] In-progress tasks show which agent is working on them
- [ ] Blocked tasks show which dependency they are waiting on
- [ ] Overall summary shows total completion across all phases
- [ ] Output is styled with lipgloss (colors, bold headers) in terminal mode
- [ ] Colors auto-disabled when output is piped (lipgloss handles this)
- [ ] Command is registered as subcommand of root (accessible as `raven status`)
- [ ] Unit tests cover rendering functions
- [ ] Graceful handling when no tasks or phases exist

## Testing Requirements
### Unit Tests
- renderPhaseProgress with 0% progress: empty progress bar
- renderPhaseProgress with 50% progress: half-filled bar
- renderPhaseProgress with 100% progress: fully filled bar
- renderPhaseProgress with in-progress tasks: shows current task
- renderPhaseProgress with blocked tasks and verbose flag: shows dependency info
- renderSummary with multiple phases at different progress levels
- --json output matches expected JSON schema
- Status with no tasks: shows "No tasks found" message
- Status with no phases config: shows tasks without phase grouping

### Integration Tests
- Full status command with testdata project: verify output contains expected phase names and counts
- Status --json | jq: verify valid JSON output

### Edge Cases to Handle
- No task-state.conf file (all tasks implicitly not_started)
- No phases.conf file (show tasks without phase grouping)
- Phase with zero tasks (empty range)
- Terminal width less than progress bar minimum (truncate gracefully)
- Very long phase names (truncate or wrap)
- 100% completed project (all phases green)

## Implementation Notes
### Recommended Approach
1. Register `statusCmd` in root command's init
2. Load config to get file paths (tasks_dir, task_state_file, phases_conf)
3. DiscoverTasks, create StateManager, LoadPhases, create TaskSelector
4. Call GetAllProgress() to get stats for all phases
5. If --phase flag set, filter to that phase
6. If --json, marshal progress data to JSON and write to stdout
7. Otherwise, render each phase's progress using lipgloss-styled output
8. Use `progress.New(progress.WithDefaultGradient())` and call `ViewAs(percent)` for static rendering
9. Write all styled output to stderr (per PRD convention)
10. Return exit code 0

### Potential Pitfalls
- The bubbles progress component is designed for Bubble Tea apps (animated). For CLI output, use `ViewAs(percent)` which returns a static string -- do not try to use `Update()` or commands
- Lipgloss styles must be width-aware: use `lipgloss.Width()` to check terminal width
- Progress percentage calculation: handle division by zero when phase has zero tasks
- The status command must work even when config is partially set up (missing phases.conf should degrade gracefully, not crash)

### Security Considerations
- Read-only command, no security concerns
- JSON output should not include file system paths in production (or mark as internal)

## References
- [PRD Section 5.3 - raven status](docs/prd/PRD-Raven.md)
- [PRD Section 5.11 - CLI Interface](docs/prd/PRD-Raven.md)
- [bubbles progress component](https://pkg.go.dev/github.com/charmbracelet/bubbles/progress)
- [lipgloss styling](https://github.com/charmbracelet/lipgloss)
