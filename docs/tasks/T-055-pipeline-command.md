# T-055: Pipeline CLI Command

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-006, T-050, T-051, T-052, T-053, T-054 |
| Blocked By | T-050, T-054 |
| Blocks | None |

## Goal
Implement the `raven pipeline` CLI command that wires together the pipeline orchestrator, branch manager, metadata tracker, wizard, and dry-run formatter into a single Cobra command with comprehensive flag support. This is the user-facing entry point for Raven's most powerful feature: end-to-end automated multi-phase pipelines.

## Background
Per PRD Section 5.11, `raven pipeline` supports:
- `raven pipeline --phase <id>` -- Run pipeline for a single phase
- `raven pipeline --phase all` -- Run pipeline for all phases
- `raven pipeline --from-phase <id>` -- Start from a specific phase
- `raven pipeline --interactive` -- Launch pipeline wizard

Per PRD Section 5.9, the command accepts skip flags, agent selection flags, concurrency settings, and max review cycle limits. It also supports `--dry-run` mode.

This command assembles all pipeline subsystem components created in T-050 through T-054 and connects them to the Cobra CLI framework with proper flag parsing, config resolution, and error handling.

## Technical Specifications
### Implementation Approach
Create `internal/cli/pipeline.go` with a `newPipelineCmd()` function that returns a configured `*cobra.Command`. The command resolves configuration, validates inputs, detects interactive mode (--interactive or no flags in a TTY), creates all subsystem instances, and delegates to the pipeline orchestrator. Exit codes follow the Raven convention: 0 (success), 1 (error), 2 (partial success), 3 (user-cancelled from wizard).

### Key Components
- **pipelineCmd**: Cobra command with all flags
- **Flag parsing**: Phase selection, skip flags, agent flags, concurrency, max cycles, dry-run, interactive
- **Config resolution**: CLI flags override config file values
- **Interactive detection**: Launch wizard when appropriate
- **Subsystem assembly**: Create orchestrator, branch manager, metadata tracker from config
- **Output handling**: Progress to stderr, results to stdout

### API/Interface Contracts
```go
// internal/cli/pipeline.go

func newPipelineCmd() *cobra.Command
// Returns a cobra.Command with:
//   Usage: "pipeline"
//   Short: "Run the full implement-review-fix-PR pipeline across phases"
//   Long: [detailed description with examples]
//   Example:
//     raven pipeline --phase 1
//     raven pipeline --phase all --impl-agent claude --review-agent codex
//     raven pipeline --from-phase 3 --skip-review --skip-fix
//     raven pipeline --interactive
//     raven pipeline --phase all --dry-run
//
//   Flags:
//     --phase <id|all>           Phase to run (single ID or "all")
//     --from-phase <id>          Start from this phase (inclusive)
//     --impl-agent <name>        Agent for implementation (default: config)
//     --review-agent <name>      Agent for review (default: config)
//     --fix-agent <name>         Agent for fix (default: config)
//     --review-concurrency <n>   Concurrent review agents (default: 2)
//     --max-review-cycles <n>    Max review/fix iterations (default: 3)
//     --skip-implement           Skip implementation stage
//     --skip-review              Skip review stage
//     --skip-fix                 Skip fix stage
//     --skip-pr                  Skip PR creation stage
//     --interactive              Launch configuration wizard
//     --dry-run                  Show planned execution without running
//     --base <branch>            Base branch (default: main)
//     --sync-base                Fetch and fast-forward base from origin
//
// The command flow:
// 1. Load config
// 2. If --interactive or (no phase flags and isatty): run wizard
// 3. Validate inputs (phase exists, agents exist)
// 4. Create subsystem instances
// 5. If --dry-run: run dry-run formatter and exit
// 6. Run pipeline orchestrator
// 7. Print results summary
// 8. Exit with appropriate code
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| spf13/cobra | v1.10+ | CLI command framework |
| internal/pipeline (T-050) | - | PipelineOrchestrator |
| internal/pipeline (T-051) | - | BranchManager |
| internal/pipeline (T-052) | - | PipelineMetadata |
| internal/pipeline (T-053) | - | RunWizard |
| internal/workflow (T-054) | - | DryRunFormatter |
| internal/workflow (T-045) | - | Engine |
| internal/workflow (T-046) | - | StateStore |
| internal/config (T-009) | - | Config loading |
| internal/task | - | Phase resolution |
| internal/agent | - | Agent resolution |
| internal/git (T-015) | - | Git client |
| charmbracelet/log | latest | Structured logging |
| golang.org/x/term | latest | TTY detection for interactive mode |

## Acceptance Criteria
- [ ] `raven pipeline --phase 1` runs pipeline for phase 1
- [ ] `raven pipeline --phase all` runs pipeline for all phases
- [ ] `raven pipeline --from-phase 3` starts from phase 3
- [ ] `raven pipeline --interactive` launches wizard then runs pipeline
- [ ] No flags in interactive terminal launches wizard
- [ ] No flags in non-interactive terminal prints usage and exits with error
- [ ] `--dry-run` shows planned execution without running
- [ ] Skip flags correctly passed to orchestrator
- [ ] Agent flags override config values
- [ ] `--review-concurrency` and `--max-review-cycles` validated (positive integers)
- [ ] `--base` overrides default base branch
- [ ] `--sync-base` triggers fetch before pipeline start
- [ ] Exit code 0 on full success, 2 on partial success, 1 on error, 3 on wizard cancel
- [ ] Progress/status output goes to stderr
- [ ] Result summary (phase statuses, PR URLs) printed to stdout on completion
- [ ] Invalid phase ID produces clear error
- [ ] Invalid agent name produces clear error with list of available agents
- [ ] All flags documented in command help text
- [ ] Unit tests achieve 80% coverage

## Testing Requirements
### Unit Tests
- Flag parsing: all flags parsed correctly from command line
- --phase "all" sets PhaseID to "all"
- --phase "3" sets PhaseID to "3"
- --from-phase "2" sets FromPhase to "2"
- --phase and --from-phase mutually exclusive: error
- Skip flags set corresponding PipelineOpts booleans
- Agent flags override config defaults
- --review-concurrency 0: validation error
- --review-concurrency -1: validation error
- --max-review-cycles 0: validation error
- Interactive mode detection: TTY + no flags -> wizard
- Interactive mode detection: non-TTY + no flags -> error
- --interactive forces wizard regardless of TTY
- Dry-run mode: orchestrator.Run not called, formatter used
- Invalid phase ID: error with available phases listed
- Invalid agent name: error with available agents listed

### Integration Tests
- Full pipeline run with mock agents: verify phase results
- Pipeline with --dry-run: verify output format and no side effects
- Pipeline with wizard: verify PipelineOpts from wizard passed to orchestrator

### Edge Cases to Handle
- Config file not found (use defaults)
- No phases configured: error "No phases found. Run raven init or configure phases.conf"
- phases.conf not found: error with helpful message
- Agent specified but not in config: error with available agents
- All stages skipped: warning "All stages skipped; nothing to execute"
- Ctrl+C during execution: graceful shutdown with partial results

## Implementation Notes
### Recommended Approach
1. Create `newPipelineCmd()` with all flags registered
2. In `RunE`:
   a. Load config (T-009)
   b. Check interactive mode: `--interactive` flag OR (no phase flags AND `term.IsTerminal(os.Stdin.Fd())`)
   c. If interactive, run wizard (T-053); if wizard returns cancel, exit code 3
   d. Validate phase range against configured phases
   e. Validate agent names against configured agents
   f. Create git client (T-015)
   g. Create workflow engine (T-045) with checkpoint store (T-046)
   h. Create branch manager (T-051)
   i. Create pipeline orchestrator (T-050)
   j. If `--dry-run`, format and print dry-run output (T-054), exit 0
   k. Run orchestrator
   l. Print result summary
   m. Return exit code based on PipelineResult.Status
3. Register command with root cmd in `root.go`
4. Add shell completion hints for `--phase` (phase IDs + "all"), `--impl-agent`, `--review-agent`, `--fix-agent` (agent names)

### Potential Pitfalls
- Flag conflict: `--phase` and `--from-phase` are mutually exclusive -- enforce with `cobra.MarkFlagsMutuallyExclusive` or manual validation
- TTY detection requires `golang.org/x/term` package -- add to go.mod
- The wizard returns `PipelineOpts` that may override flags -- wizard output always wins when in interactive mode
- Error messages should include suggested fixes: "Agent 'gemeni' not found. Did you mean 'gemini'? Available agents: claude, codex, gemini"
- Ensure the command works with global flags: `--verbose`, `--quiet`, `--config`, `--dir`, `--no-color`

### Security Considerations
- CLI flags from user input should be validated (no path traversal in phase names, no shell injection in agent names)
- The `--base` branch flag should be validated as a safe git ref name
- Do not log full agent commands in non-verbose mode (may contain prompt paths)

## References
- [PRD Section 5.9 - Phase Pipeline Orchestrator](docs/prd/PRD-Raven.md)
- [PRD Section 5.11 - CLI commands](docs/prd/PRD-Raven.md)
- [spf13/cobra v1.10+ documentation](https://pkg.go.dev/github.com/spf13/cobra)
- [golang.org/x/term - terminal detection](https://pkg.go.dev/golang.org/x/term)
- [Cobra flag mutual exclusivity](https://github.com/spf13/cobra/blob/main/site/content/user_guide.md)