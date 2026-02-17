# T-042: CLI Commands -- raven fix and raven pr

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-038, T-040, T-041 |
| Blocked By | T-038, T-040 |
| Blocks | None |

## Goal
Implement the `raven fix` and `raven pr` Cobra commands that serve as CLI entry points for the review fix engine and PR creation, respectively. These are the final two commands in the Phase 3 review pipeline, completing the `review -> fix -> pr` workflow. Both commands parse flags, resolve configuration, wire up their respective engines, and handle output and exit codes.

## Background
Per PRD Section 5.11, `raven fix --agent <name>` runs the fix engine to apply review fixes, and `raven pr` creates a pull request. These commands can be run standalone or as part of the `pipeline` orchestrator (Phase 4). The `fix` command reads the review report from a previous `raven review` run (or accepts findings inline), applies fixes, and runs verification. The `pr` command generates a PR body and creates the PR via `gh`. Both support `--dry-run` mode.

## Technical Specifications
### Implementation Approach
Create `internal/cli/fix.go` for the `raven fix` command and `internal/cli/pr.go` for the `raven pr` command. Each command follows the same pattern as `raven review` (T-041): parse flags, load config, construct dependencies, execute, handle output and exit codes.

### Key Components
- **fixCmd**: Cobra command for `raven fix`
- **prCmd**: Cobra command for `raven pr`
- **Fix flag resolution**: agent, max-fix-cycles, review-report path, dry-run
- **PR flag resolution**: base branch, draft, title, labels, dry-run

### API/Interface Contracts
```go
// internal/cli/fix.go

// NewFixCmd creates the "raven fix" Cobra command.
func NewFixCmd() *cobra.Command

// Flags:
// --agent <name>           Agent to use for fixes (default: from config)
// --max-fix-cycles <n>     Maximum fix-verify cycles (default: 3)
// --review-report <file>   Path to review report from raven review (default: auto-detect)
// --dry-run                Show fix prompt without executing
// --verbose                Show detailed progress

// Exit codes:
// 0 - All fixes applied and verification passed
// 1 - Error during fix execution
// 2 - Fixes applied but verification still failing after max cycles

// internal/cli/pr.go

// NewPRCmd creates the "raven pr" Cobra command.
func NewPRCmd() *cobra.Command

// Flags:
// --base <branch>          Base branch for PR (default: "main")
// --draft                  Create as draft PR
// --title <title>          PR title (default: auto-generated)
// --label <label>          Labels to add (can be repeated)
// --assignee <user>        Assignees (can be repeated)
// --review-report <file>   Path to review report to include in body
// --dry-run                Show PR body without creating PR
// --no-summary             Skip AI summary generation
// --verbose                Show detailed progress

// Exit codes:
// 0 - PR created successfully
// 1 - Error during PR creation
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| spf13/cobra | v1.10+ | CLI framework |
| internal/config (T-009) | - | Configuration loading |
| internal/agent (T-021) | - | Agent registry for fix agent |
| internal/review/fix (T-038) | - | FixEngine for applying fixes |
| internal/review/pr (T-040) | - | PRCreator for creating PRs |
| internal/review/prbody (T-039) | - | PRBodyGenerator for PR body |
| internal/review/verify (T-037) | - | VerificationRunner for fix verification |
| charmbracelet/log | latest | Structured logging to stderr |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria

### raven fix
- [ ] `raven fix --agent claude` runs the fix engine with the specified agent
- [ ] `raven fix` with no flags uses the default agent from config
- [ ] `--max-fix-cycles 5` sets the maximum number of fix-verify iterations
- [ ] `--review-report report.md` reads findings from the specified file
- [ ] Without `--review-report`, auto-detects the most recent review report
- [ ] `--dry-run` shows the fix prompt without invoking the agent
- [ ] Exit code 0 when fixes pass verification
- [ ] Exit code 2 when fixes applied but verification still fails
- [ ] Exit code 1 on execution error
- [ ] Progress output (cycle number, verification status) goes to stderr

### raven pr
- [ ] `raven pr` creates a PR with an auto-generated title and body
- [ ] `--base develop` sets the base branch for the PR
- [ ] `--draft` creates a draft PR
- [ ] `--title "My PR"` overrides the auto-generated title
- [ ] `--label "review"` adds labels (repeatable flag)
- [ ] `--review-report report.md` includes review results in PR body
- [ ] `--dry-run` shows the PR body and command without creating the PR
- [ ] `--no-summary` skips the AI summary generation (faster, no agent call)
- [ ] Exit code 0 on successful PR creation
- [ ] Exit code 1 on error (gh not installed, auth failure, etc.)
- [ ] PR URL is printed to stdout on success

### General
- [ ] Both commands are registered as subcommands of the root command
- [ ] Both commands respect `--verbose` and `--quiet` global flags
- [ ] Both commands handle missing config gracefully
- [ ] Unit tests achieve 85% coverage for flag parsing and wiring logic

## Testing Requirements
### Unit Tests

#### raven fix
- Flag parsing: --agent "claude" sets agent name
- Flag parsing: --max-fix-cycles 5 sets cycle limit
- Flag parsing: defaults applied when no flags given
- Invalid agent name: error with available agents listed
- DryRun: expected output without engine execution
- Exit code mapping: verification passed -> 0, still failing -> 2
- Review report file not found: clear error message

#### raven pr
- Flag parsing: --base "develop" sets base branch
- Flag parsing: --draft sets draft mode
- Flag parsing: --label "bug" --label "review" sets multiple labels
- Flag parsing: --title "Custom Title" overrides auto-generation
- DryRun: expected PR body and command output
- Exit code mapping: created -> 0, error -> 1
- PR URL printed to stdout after creation

### Integration Tests
- Full fix command with mock agent in test project
- Full pr command dry-run in test git repository
- Pipeline sequence: review -> fix -> pr in test project

### Edge Cases to Handle
- No review report exists (first run without raven review): prompt user to run review first
- Review report is empty or malformed: error with helpful message
- Agent not available (not installed): error before starting fix
- gh CLI not installed: error before attempting PR creation
- PR already exists for this branch: helpful error message
- No commits to create PR: error from gh
- Fix command run on clean working tree with no review findings: no-op with success

## Implementation Notes
### Recommended Approach

#### raven fix
1. Register `fixCmd` as a subcommand in root.go
2. In `RunE`:
   a. Load config, parse flags, resolve agent
   b. If --review-report provided, read and parse findings
   c. If not, look for most recent review report in log directory
   d. Construct VerificationRunner from config.Project.VerificationCommands
   e. Construct FixEngine with agent, verifier, and max cycles
   f. If --dry-run, call DryRun and output prompt to stdout
   g. Otherwise, call Fix and handle result
   h. Output fix summary to stderr, set exit code

#### raven pr
1. Register `prCmd` as a subcommand in root.go
2. In `RunE`:
   a. Load config, parse flags
   b. Construct PRBodyGenerator with optional agent for summary
   c. Gather pipeline artifacts (review report, fix report, verification report)
   d. Generate PR body
   e. If --dry-run, output body and planned command to stdout
   f. Otherwise, construct PRCreator and create PR
   g. Print PR URL to stdout

### Potential Pitfalls
- Review report auto-detection needs a convention for report file naming and location -- use `scripts/logs/review-report-<timestamp>.md` or similar
- The --label flag needs `cobra.StringSliceVar` or `cobra.StringArrayVar` for multiple values -- use `StringArrayVar` to avoid comma splitting within values
- Exit codes require `os.Exit()` called after Cobra returns -- use a deferred function or a wrapper
- The pr command should check prerequisites (gh auth) before spending time generating the body

### Security Considerations
- Review report path should be validated to prevent reading arbitrary files
- PR title and labels should be sanitized (no injection into gh command arguments)
- Body file for gh is written to a temp directory with restricted permissions

## References
- [PRD Section 5.6 - Review Fix Engine](docs/prd/PRD-Raven.md)
- [PRD Section 5.7 - PR Creation](docs/prd/PRD-Raven.md)
- [PRD Section 5.11 - CLI Interface](docs/prd/PRD-Raven.md)
- [spf13/cobra documentation](https://pkg.go.dev/github.com/spf13/cobra)
- [gh pr create documentation](https://cli.github.com/manual/gh_pr_create)
