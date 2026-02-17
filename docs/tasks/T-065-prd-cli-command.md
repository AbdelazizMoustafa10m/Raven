# T-065: Raven PRD CLI Command

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Large: 16-24hrs |
| Dependencies | T-006, T-056, T-057, T-058, T-059, T-060, T-061, T-062, T-063, T-064 |
| Blocked By | T-006, T-064 |
| Blocks | None |

## Goal
Implement the `raven prd` CLI command that wires together the entire PRD decomposition pipeline: shred (Phase 1) -> scatter (Phase 2) -> merge with dedup and DAG validation (Phase 3) -> emit output files. This is the user-facing command that exposes the three-phase workflow through a clean CLI interface with flags for concurrency control, agent selection, output configuration, single-pass mode, and dry-run preview.

## Background
Per PRD Section 5.8 and 5.11, the `raven prd --file <path>` command decomposes a Product Requirements Document into implementation tasks. The user story is: "As a developer, I want to run `raven prd --file docs/prd/PRD.md --concurrent --concurrency 3` and get a complete set of task files with dependencies, phases, and progress tracking."

The command orchestrates all PRD decomposition components built in T-056 through T-064:
1. **Shred** (T-057): Single agent call to produce epic-level JSON
2. **Scatter** (T-059): N concurrent agent workers to decompose each epic into tasks
3. **Merge** (T-060, T-061, T-062, T-063): Global ID assignment, dependency remapping, deduplication, DAG validation
4. **Emit** (T-064): Generate task files, phases.conf, task-state.conf, PROGRESS.md, INDEX.md

The command also supports a `--single-pass` mode for small PRDs where the entire decomposition is done in one agent call (skipping the scatter-gather phases), and a `--dry-run` mode that shows planned actions without execution.

## Technical Specifications
### Implementation Approach
Create `internal/cli/prd.go` containing a Cobra command definition for `raven prd`. The command validates inputs, loads configuration from `raven.toml`, constructs the appropriate agent adapter, creates a temporary working directory for intermediate files, and orchestrates the pipeline stages with progress reporting to stderr. On success, it writes all output files to the designated output directory and prints a summary.

### Key Components
- **prdCmd**: Cobra command definition with flag bindings
- **prdRunE**: Main command handler that orchestrates the pipeline
- **prdProgress**: Progress reporter for stderr output during execution
- **SinglePassDecomposer**: Alternative path for --single-pass mode (entire PRD in one agent call)

### API/Interface Contracts
```go
// internal/cli/prd.go

// NewPRDCmd creates the "raven prd" command.
func NewPRDCmd() *cobra.Command

// prdCmd flags:
// --file <path>        (required) Path to the PRD markdown file
// --concurrent         (default: true) Enable concurrent epic decomposition
// --concurrency <n>    (default: 3) Max concurrent workers in scatter phase
// --single-pass        (default: false) Decompose entire PRD in one agent call
// --output-dir <path>  (default: from raven.toml project.tasks_dir) Output directory for task files
// --agent <name>       (default: from raven.toml) Agent to use for decomposition
// --dry-run            (default: false) Show planned pipeline steps without executing
// --force              (default: false) Overwrite existing files in output directory
// --start-id <n>       (default: 1) Starting task number for ID assignment

// Internal orchestration:
type prdPipeline struct {
    cfg        *config.Config
    agent      agent.Agent
    logger     *log.Logger
    prdPath    string
    outputDir  string
    workDir    string
    concurrent bool
    concurrency int
    singlePass bool
    dryRun     bool
    force      bool
    startID    int
}

// run executes the full PRD decomposition pipeline.
func (p *prdPipeline) run(ctx context.Context) error

// runConcurrent executes the three-phase scatter-gather pipeline.
func (p *prdPipeline) runConcurrent(ctx context.Context) error

// runSinglePass executes the single-pass decomposition (one agent call).
func (p *prdPipeline) runSinglePass(ctx context.Context) error

// printDryRun shows the planned pipeline steps without execution.
func (p *prdPipeline) printDryRun() error

// printSummary prints a summary of the decomposition results to stderr.
func (p *prdPipeline) printSummary(result *EmitResult, shredDuration, scatterDuration, mergeDuration, emitDuration time.Duration)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| github.com/spf13/cobra | v1.10+ | CLI command framework |
| internal/cli (T-006) | - | Root command, global flags, config loading |
| internal/config | - | Configuration loading and resolution |
| internal/agent | - | Agent interface and adapter construction |
| internal/prd (T-056) | - | Schema types |
| internal/prd (T-057) | - | Shredder for Phase 1 |
| internal/prd (T-058) | - | JSON extraction utility |
| internal/prd (T-059) | - | ScatterOrchestrator for Phase 2 |
| internal/prd (T-060) | - | Global ID assignment |
| internal/prd (T-061) | - | Dependency remapping |
| internal/prd (T-062) | - | Title deduplication |
| internal/prd (T-063) | - | DAG validation |
| internal/prd (T-064) | - | Task file emitter |
| charmbracelet/log | - | Structured logging |
| os | stdlib | File I/O, temp directory |
| time | stdlib | Pipeline timing |

## Acceptance Criteria
- [ ] `raven prd --file docs/prd/PRD.md` runs the full three-phase pipeline and generates task files
- [ ] `--file` flag is required; command fails with clear error if omitted
- [ ] `--file` path is validated to exist and be readable before pipeline starts
- [ ] `--concurrent` flag (default true) enables parallel scatter phase
- [ ] `--concurrency 5` sets worker concurrency to 5 in scatter phase
- [ ] `--single-pass` mode decomposes entire PRD in one agent call, producing task files directly
- [ ] `--output-dir ./my-tasks` writes output to specified directory instead of default
- [ ] `--agent codex` uses Codex agent instead of default from config
- [ ] `--dry-run` shows planned pipeline steps (phase descriptions, agent, concurrency) without execution
- [ ] `--force` allows overwriting existing files in output directory
- [ ] `--start-id 50` starts task numbering at T-050 (for appending to existing task sets)
- [ ] Progress is reported to stderr during execution (phase transitions, worker status, timing)
- [ ] Summary printed on success: total tasks, total phases, output directory, timing breakdown
- [ ] Exit code 0 on success, 1 on error, 2 on partial success (some epics failed)
- [ ] Intermediate files are cleaned up (temp working directory removed on success)
- [ ] Context cancellation (Ctrl+C) gracefully stops the pipeline and cleans up
- [ ] Unit tests achieve 85% coverage for command setup and validation logic

## Testing Requirements
### Unit Tests
- Command creation: verify all flags registered with correct defaults
- Missing --file flag: returns error with usage hint
- Nonexistent PRD file: returns clear error before pipeline starts
- --dry-run: prints pipeline plan without invoking any agents
- --single-pass and --concurrent are mutually exclusive: error if both set
- --concurrency < 1: returns validation error
- --concurrency ignored when --single-pass is set (warning logged)
- --output-dir resolves relative paths against working directory
- --agent flag overrides config default
- --start-id with valid number sets starting ID correctly
- --force flag allows overwrite of existing output directory
- Config loading: agent name resolves to configured agent in raven.toml
- Config loading: output-dir defaults to project.tasks_dir from raven.toml

### Integration Tests
- Full pipeline with mock agent: shred -> scatter -> merge -> emit produces correct file structure
- Single-pass pipeline with mock agent: one call -> emit produces correct files
- Pipeline with agent that returns rate-limit on first call, succeeds on retry
- Pipeline with agent that fails on one epic: partial success exit code 2, remaining epics emitted
- Ctrl+C during scatter phase: graceful shutdown, temp files cleaned up
- Output directory already contains task files: error without --force, success with --force

### Edge Cases to Handle
- PRD file is empty: clear error message ("PRD file is empty")
- PRD file is very large (>1MB): warning logged but proceeds
- Agent not configured in raven.toml and not specified via --agent: clear error
- raven.toml not found: use defaults for all config values
- Output directory is a file (not directory): clear error
- Output directory is on read-only filesystem: clear error on first write attempt
- Network error during agent invocation: retry with backoff, eventual clear error
- Scatter phase produces 0 tasks for an epic: warning logged, epic skipped
- All scatter workers fail: exit code 1 with aggregated error details

## Implementation Notes
### Recommended Approach
1. **Command setup**: Register `prdCmd` as subcommand of root, bind all flags
2. **RunE handler**:
   a. Validate --file exists and is readable
   b. Load config, resolve agent name to Agent interface
   c. Resolve output directory (flag > config > default "docs/tasks")
   d. Create temp working directory for intermediate files
   e. If --dry-run, call printDryRun() and return
   f. If --single-pass, call runSinglePass()
   g. Else call runConcurrent()
   h. On success, print summary and clean up temp dir
   i. On error, preserve temp dir for debugging (log its path)
3. **runConcurrent**:
   a. Phase 1: Create Shredder, call Shred() -> EpicBreakdown
   b. Log: "Phase 1 complete: N epics identified in Xs"
   c. Phase 2: Create ScatterOrchestrator, call Scatter() -> ScatterResult
   d. Log: "Phase 2 complete: N tasks across M epics in Xs (F failures)"
   e. Phase 3a: SortEpicsByDependency + AssignGlobalIDs -> []MergedTask, IDMapping
   f. Phase 3b: RemapDependencies -> []MergedTask, RemapReport
   g. Phase 3c: DeduplicateTasks -> []MergedTask, DedupReport
   h. Phase 3d: ValidateDAG -> DAGValidation (if invalid, report errors and exit)
   i. Phase 3e: Emit -> EmitResult
   j. Return nil on success
4. **runSinglePass**: Single agent call that produces all tasks directly (uses a combined prompt that asks for complete task breakdown, not epic breakdown). Parse output as []TaskDef, assign IDs, validate, emit
5. **printSummary**: Table showing phase timing, task/phase counts, output paths

### Potential Pitfalls
- Flag binding: use `cmd.Flags()` not `cmd.PersistentFlags()` for prd-specific flags; global flags come from root
- Agent construction requires config loading -- if config is not found, fall back to defaults but require --agent flag
- Working directory for intermediate files should use `os.MkdirTemp("", "raven-prd-*")` for uniqueness
- Single-pass mode needs its own prompt template (different from shred prompt) -- it asks for a flat task list, not epics
- Partial success (exit code 2) should still emit whatever tasks were successfully decomposed
- Progress output must go to stderr (stdout is reserved for structured output per project conventions)
- The --start-id flag is useful for projects that already have tasks and want to append new ones without ID collisions
- Clean up temp directory on success but preserve on failure (user may want to inspect intermediate JSON for debugging)

### Security Considerations
- Validate --file path is within project directory or an explicit absolute path (no path traversal via config)
- Validate --output-dir does not point outside project directory
- Temp directory should use restricted permissions (0700)
- Do not log full agent prompts at info level (may contain sensitive PRD content); use debug level

## References
- [PRD Section 5.8 - PRD Decomposition Workflow](docs/prd/PRD-Raven.md)
- [PRD Section 5.11 - CLI Interface, `raven prd` command](docs/prd/PRD-Raven.md)
- [Cobra user guide](https://github.com/spf13/cobra/blob/main/site/content/user_guide.md)
- [Cobra RunE best practices](https://cobra.dev/docs/how-to-guides/working-with-commands/)
- [Go os.MkdirTemp documentation](https://pkg.go.dev/os#MkdirTemp)