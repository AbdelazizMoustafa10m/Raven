# T-041: CLI Command -- raven review

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-035, T-009 |
| Blocked By | T-035 |
| Blocks | T-042 |

## Goal
Implement the `raven review` Cobra command that serves as the CLI entry point for the multi-agent review pipeline. This command parses flags for agent selection, concurrency, review mode, and base branch, resolves configuration from CLI flags, environment variables, and `raven.toml`, wires up the ReviewOrchestrator (T-035), and drives the review pipeline from diff generation through report output.

## Background
Per PRD Section 5.11, `raven review --agents <list> --concurrency <n>` runs the multi-agent review pipeline. The CLI command is responsible for parsing user inputs, resolving config, constructing all pipeline dependencies (DiffGenerator, PromptBuilder, Consolidator, ReviewOrchestrator), executing the pipeline, writing the review report to stdout or a file, and setting the appropriate exit code. It supports `--dry-run` to show planned actions without execution. All progress/status goes to stderr; the review report goes to stdout (piping-friendly).

## Technical Specifications
### Implementation Approach
Create `internal/cli/review.go` containing the Cobra command definition for `raven review`. The command constructs the full review pipeline by wiring together: config loading, agent registry lookup, DiffGenerator, PromptBuilder, Consolidator, and ReviewOrchestrator. Execute the pipeline and output the review report. Handle errors with appropriate exit codes (0=success/approved, 1=error, 2=changes_needed/blocking).

### Key Components
- **reviewCmd**: Cobra command definition with flags
- **reviewRunE**: Main execution function that wires up and runs the pipeline
- **Flag resolution**: CLI flags > env vars > raven.toml > defaults

### API/Interface Contracts
```go
// internal/cli/review.go

// NewReviewCmd creates the "raven review" Cobra command.
func NewReviewCmd() *cobra.Command

// Flags:
// --agents <list>       Comma-separated list of agent names (default: from config)
// --concurrency <n>     Maximum concurrent review agents (default: 2)
// --mode <all|split>    Review mode (default: "all")
// --base <branch>       Base branch for diff (default: "main")
// --output <file>       Write report to file instead of stdout
// --dry-run             Show planned actions without executing
// --verbose             Show detailed progress

// Exit codes:
// 0 - Review completed, verdict is APPROVED
// 1 - Error during review execution
// 2 - Review completed, verdict is CHANGES_NEEDED or BLOCKING
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| spf13/cobra | v1.10+ | CLI framework |
| internal/config (T-009) | - | Configuration loading and resolution |
| internal/agent (T-021) | - | Agent registry for looking up review agents |
| internal/review (T-035) | - | ReviewOrchestrator and all review pipeline components |
| charmbracelet/log | latest | Structured logging to stderr |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] `raven review --agents claude,codex --concurrency 4` runs the full review pipeline
- [ ] `raven review` with no flags uses defaults from config (agents from [agents] section, concurrency=2)
- [ ] `--agents` flag accepts comma-separated agent names and validates each exists in registry
- [ ] `--concurrency` flag sets the errgroup limit
- [ ] `--mode all` sends all files to every agent; `--mode split` distributes files
- [ ] `--base` overrides the default base branch for diff generation
- [ ] `--output report.md` writes the report to a file instead of stdout
- [ ] `--dry-run` shows planned review actions (agents, files, concurrency) without executing
- [ ] Review report is written to stdout (or file); progress/status to stderr
- [ ] Exit code 0 when verdict is APPROVED
- [ ] Exit code 2 when verdict is CHANGES_NEEDED or BLOCKING
- [ ] Exit code 1 on execution error
- [ ] `--verbose` shows detailed progress including agent output excerpts
- [ ] Handles missing config gracefully with sensible defaults
- [ ] Unit tests achieve 85% coverage

## Testing Requirements
### Unit Tests
- Flag parsing: --agents "claude,codex" parsed into []string{"claude", "codex"}
- Flag parsing: --concurrency 4 sets concurrency to 4
- Flag parsing: --mode split sets review mode
- Flag parsing: defaults applied when no flags given
- Invalid agent name: error with helpful message listing available agents
- Invalid mode value: error with valid options listed
- DryRun: expected output without pipeline execution
- Exit code mapping: APPROVED -> 0, CHANGES_NEEDED -> 2, BLOCKING -> 2

### Integration Tests
- Full command execution with mock agents in a test git repository
- Command with --output writes report to file

### Edge Cases to Handle
- No agents configured and no --agents flag: error with setup instructions
- Agent not found in registry: clear error naming the missing agent
- No changes to review (empty diff): report "no changes" and exit 0
- Config file not found: use defaults, log warning
- Base branch does not exist: error from git before starting review

## Implementation Notes
### Recommended Approach
1. Register `reviewCmd` as a subcommand of the root command in `internal/cli/root.go`
2. In `RunE`:
   a. Load config via config package (T-009)
   b. Parse and validate flags
   c. Look up agents in the agent registry
   d. Construct DiffGenerator with config.Review settings
   e. Construct PromptBuilder with config.Review settings
   f. Construct Consolidator
   g. Construct ReviewOrchestrator with all dependencies
   h. If --dry-run, call DryRun and output to stdout
   i. Otherwise, call Run and handle result
   j. Output report to stdout or --output file
   k. Map verdict to exit code
3. Use `charmbracelet/log` for progress messages to stderr
4. Use `cobra.SilenceUsage = true` to avoid printing usage on runtime errors

### Potential Pitfalls
- Cobra's default error handling prints usage -- disable with `SilenceUsage` and `SilenceErrors`
- Exit codes must be set via `os.Exit()` after cleanup -- Cobra's `RunE` error return does not set custom exit codes by default
- Agent names in --agents flag should be trimmed of whitespace
- The --output flag creates the file early -- if the pipeline fails, clean up the empty file

### Security Considerations
- Validate that --output path is writable and within a reasonable directory
- Agent names from --agents flag should be validated against the registry, not used as arbitrary strings

## References
- [PRD Section 5.11 - CLI Interface, raven review](docs/prd/PRD-Raven.md)
- [spf13/cobra documentation](https://pkg.go.dev/github.com/spf13/cobra)
- [Cobra best practices](https://github.com/spf13/cobra/blob/main/site/content/user_guide.md)
