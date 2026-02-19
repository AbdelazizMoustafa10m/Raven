# Raven Task Progress Log

## Summary

| Status | Count |
|--------|-------|
| Completed | 89 |
| In Progress | 0 |
| Not Started | 2 |

---

## Completed Tasks

### Phase 1: Foundation (T-001 to T-015)

- **Status:** Completed
- **Date:** 2026-02-18
- **Tasks Completed:** 15 tasks

#### Features Implemented

| Feature | Tasks | Description |
| ------- | ----- | ----------- |
| Go module & project scaffold | T-001 | Module initialization, 12 internal package stubs, dependency declarations, `testdata/` and `templates/` directories |
| Makefile & build targets | T-002 | 12 GNU make targets, ldflags version injection, `CGO_ENABLED=0`, debug build support |
| Build info package | T-003 | `Info` struct, `GetInfo()` accessor, `String()` formatter with JSON struct tags |
| Core data types | T-004 | `WorkflowState`, `StepRecord`, `RunOpts`, `RunResult`, `RateLimitInfo`, `Task`, `Phase`, `TaskStatus` with JSON serialization |
| Structured logging | T-005 | `internal/logging` package: `Setup()`, `New(component)`, `SetOutput()`, level constants, stderr-only output |
| Cobra root command | T-006 | Root command, 6 global persistent flags, `PersistentPreRunE` with env-var overrides, `Execute() int` |
| Version command | T-007 | `raven version` with `--json` flag, uses `buildinfo.GetInfo()` |
| Shell completion | T-008 | `raven completion <shell>` for bash/zsh/fish/PowerShell via Cobra built-ins |
| TOML config types & loading | T-009 | `Config` type hierarchy, `FindConfigFile()` dir-walk, `LoadFromFile()`, `NewDefaults()` |
| Config resolution | T-010 | Four-layer merge (CLI > env > file > defaults), `ResolvedConfig` with source tracking, `CLIOverrides`, `EnvFunc` injection |
| Config validation | T-011 | `Validate()`, `ValidationResult`/`ValidationIssue`/`ValidationSeverity` types, unknown-key detection via `toml.MetaData.Undecoded()` |
| Config debug & validate commands | T-012 | `raven config debug` (color-coded source annotations), `raven config validate` (errors/warnings), shared `loadAndResolveConfig()` helper |
| Embedded project templates | T-013 | `//go:embed all:templates`, `TemplateVars`, `ListTemplates()`, `TemplateExists()`, `RenderTemplate()` with `text/template` processing |
| Init command | T-014 | `raven init [template]` with `--name`/`--force` flags, path-traversal guard, PersistentPreRunE override skipping config load |
| Git client wrapper | T-015 | `GitClient` wrapping all git CLI ops, branch/status/stash/diff/log/push methods, `EnsureClean()` auto-stash recovery |

#### Key Technical Decisions

1. **BurntSushi/toml over encoding/toml** -- `MetaData.Undecoded()` enables unknown-key detection without reflection hacks
2. **Pointer types in CLIOverrides** -- `*string`/`*bool` fields distinguish "not set" from "set to zero value" for correct priority merging
3. **`all:` embed prefix for templates** -- Required to include dotfiles (`.github/`, `.gitkeep`) in the embedded FS
4. **`TaskStatus` as string constants, not iota** -- Values must round-trip through JSON/TOML without a custom marshaler
5. **`runSilent` helper in GitClient** -- Separates exec failures (exitCode=-1) from non-zero git exit codes for accurate error handling
6. **`CGO_ENABLED=0` for all builds** -- Pure Go cross-compilation; no C dependencies anywhere in the stack
7. **charmbracelet/log over slog** -- Pretty terminal output with component prefixes, consistent with the TUI ecosystem

#### Key Files Reference

| Purpose | Location |
| ------- | -------- |
| Entry point | `cmd/raven/main.go` |
| Build targets & ldflags | `Makefile` |
| Build info variables | `internal/buildinfo/buildinfo.go` |
| WorkflowState, StepRecord | `internal/workflow/state.go` |
| RunOpts, RunResult, RateLimitInfo | `internal/agent/types.go` |
| Task, Phase, TaskStatus | `internal/task/types.go` |
| Logger factory | `internal/logging/logging.go` |
| Root command & global flags | `internal/cli/root.go` |
| Version command | `internal/cli/version.go` |
| Shell completion command | `internal/cli/completion.go` |
| Config type hierarchy | `internal/config/config.go` |
| Default config values | `internal/config/defaults.go` |
| Config file discovery & loading | `internal/config/load.go` |
| Four-layer config resolution | `internal/config/resolve.go` |
| Config validation | `internal/config/validate.go` |
| Config debug/validate commands | `internal/cli/config_cmd.go` |
| Template embedding & rendering | `internal/config/templates.go` |
| Embedded go-cli template | `internal/config/templates/go-cli/raven.toml.tmpl` |
| Init command | `internal/cli/init_cmd.go` |
| Git client wrapper | `internal/git/client.go` |
| Git auto-stash recovery | `internal/git/recovery.go` |

#### Verification

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass

---

### Phase 2: Task System & Agent Adapters (T-016 to T-030)

- **Status:** Completed
- **Date:** 2026-02-18
- **Tasks Completed:** 15 tasks

#### Features Implemented

| Feature | Tasks | Description |
| ------- | ----- | ----------- |
| Task spec markdown parser | T-016 | `ParseTaskSpec`, `ParseTaskFile`, `DiscoverTasks`, `ParsedTaskSpec.ToTask()`, pre-compiled regexes, 1 MiB size guard, CRLF/BOM normalisation |
| Task state management | T-017 | `StateManager` with `Load`, `LoadMap`, `Get`, `Update`, `UpdateStatus`, `Initialize`, `StatusCounts`, `TasksWithStatus`; pipe-delimited `task-state.conf`; atomic write + mutex for concurrent safety; `ValidStatuses()` and `IsValid()` on `TaskStatus` |
| Phase configuration parser | T-018 | `LoadPhases`, `ParsePhaseLine` (4-field and 6-field format auto-detection), `PhaseForTask`, `PhaseByID`, `TaskIDNumber`, `TasksInPhase`, `FormatPhaseLine`, `ValidatePhases`; reads `phases.conf`; sorts phases by ID; validates non-overlapping ranges |
| Dependency resolution & next-task selection | T-019 | `TaskSelector` with `NewTaskSelector`, `SelectNext`, `SelectNextInRange`, `SelectByID`, `GetPhaseProgress`, `GetAllProgress`, `IsPhaseComplete`, `BlockedTasks`, `CompletedTaskIDs`, `RemainingTaskIDs`; `PhaseProgress` aggregate struct; O(1) spec lookup via specMap; read-only (never mutates state) |
| Status command with progress bars | T-020 | `raven status` CLI command with `--phase`, `--json`, `--verbose` flags; `renderPhaseProgress` (bubbles/progress.ViewAs static bar), `renderSummary`, `renderTaskDetails`; `buildUngroupedProgress` for no-phases mode; graceful handling of missing phases.conf; JSON output via `statusOutput`/`statusPhaseOutput` structs |
| Agent interface & registry | T-021 | `Agent` interface (5 methods: Name, Run, CheckPrerequisites, ParseRateLimit, DryRunCommand); `Registry` type with Register, Get, MustGet, List, Has; `AgentConfig` struct for raven.toml `[agents.*]` sections; `MockAgent` with builder methods (WithRunFunc, WithRateLimit, WithPrereqError); sentinel errors (ErrNotFound, ErrDuplicateName, ErrInvalidName); agent name validation via regex; compile-time interface check |
| Claude agent adapter | T-022 | `ClaudeAgent` struct implementing `Agent`; `NewClaudeAgent(config, logger)`; `buildCommand` and `buildArgs` helpers; `--permission-mode accept --print` flags; model/allowedTools/outputFormat flag injection with RunOpts-over-config precedence; `CLAUDE_CODE_EFFORT_LEVEL` env var; large-prompt temp-file spill; `ParseRateLimit` with `reClaudeRateLimit`/`reClaudeResetTime`/`reClaudeTryAgain` regexes; `parseResetDuration` unit parser; `DryRunCommand` with prompt truncation; injected `claudeLogger` interface; compile-time `var _ Agent = (*ClaudeAgent)(nil)` |
| Codex agent adapter | T-023 | `CodexAgent` struct implementing `Agent`; `NewCodexAgent(config, logger)`; `buildCommand` with `codex exec --sandbox --ephemeral -a never` flags; model flag with RunOpts-over-config precedence; prompt via `--prompt` or `--prompt-file`; three-tier `ParseRateLimit`: short decimal-seconds (`5.448s`), long format (`1 days 2 hours`), fallback keyword; `parseCodexDuration` helper; `DryRunCommand` with Unicode-safe prompt truncation; `codexLogger` interface; compile-time `var _ Agent = (*CodexAgent)(nil)` |
| Gemini agent stub | T-024 | `GeminiAgent` stub struct implementing `Agent`; `NewGeminiAgent(config AgentConfig)`; `Run` and `CheckPrerequisites` return `ErrNotImplemented`; `ParseRateLimit` always returns nil/false; `DryRunCommand` returns placeholder comment; `ErrNotImplemented` sentinel error; compile-time `var _ Agent = (*GeminiAgent)(nil)`; no os/exec imports (pure stub) |
| Rate-limit detection & coordination | T-025 | `RateLimitCoordinator` with `sync.RWMutex` for thread safety; `ProviderState` per-provider tracking (IsLimited, ResetAt, WaitCount, LastMessage, UpdatedAt); `BackoffConfig` (DefaultWait, MaxWaits, JitterFactor); `DefaultBackoffConfig()` (60s/5/0.1); provider constants (ProviderAnthropic/OpenAI/Google); `AgentProvider` map; `RecordRateLimit`, `ClearRateLimit`, `ShouldWait`, `WaitForReset`, `ExceededMaxWaits`; `computeWaitDuration` with jitter; `ErrMaxWaitsExceeded` sentinel; callback captured under lock (race-free) |
| Prompt template system | T-026 | `PromptContext` struct (task, phase, project, verification, progress, agent fields); `PromptGenerator` with custom `[[`/`]]` delimiters (avoids `{{`/`}}` conflicts in task spec content); `NewPromptGenerator(templateDir)` with directory validation; `LoadTemplate(name)` with directory-traversal guard and LRU caching; `Generate`, `GenerateFromString`; `BuildContext(spec, phase, cfg, selector, agentName)` populating all fields from live state; `DefaultImplementTemplate` const |
| Implementation loop runner | T-027 | `Runner` struct orchestrating the full implement loop; `RunConfig` (AgentName, PhaseID, TaskID, MaxIterations/50, MaxLimitWaits/5, SleepBetween/5s, DryRun, TemplateName); `Run(ctx, runCfg)` phase mode and `RunSingleTask(ctx, runCfg)` single-task mode; `DetectSignals(output)` exported scanner for PHASE_COMPLETE/TASK_BLOCKED/RAVEN_ERROR; stale-task detection; 16 `LoopEventType` constants; non-blocking event channel sends |
| Loop recovery | T-028 | `RateLimitWaiter` with countdown display (`\r` in-place updates); `DirtyTreeRecovery` with `CheckAndStash`/`RestoreStash`/`EnsureCleanTree`; `AgentErrorRecovery` tracking consecutive errors with configurable max threshold; 7 `RecoveryEventType` constants; all structs nil-guard optional logger and events channel |
| Implementation CLI command | T-029 | `raven implement` CLI command; `implementFlags` struct (Agent required, PhaseStr/Task mutually exclusive); `runImplement()` 16-step composition root wiring all Phase 2 components; `validateImplementFlags()`, `buildAgentRegistry()`, `runAllPhases()`; SIGINT/SIGTERM graceful shutdown via `signal.NotifyContext`; `init()` registers on rootCmd |
| Progress file generation | T-030 | `ProgressGenerator` struct with `Generate`, `WriteTo`, `WriteFile` (atomic write via tmp+rename); `ProgressData`/`PhaseProgressData`/`TaskProgressData` structs; embedded `progressTemplate`; `textProgressBar` using `strings.Repeat("█"/"░")`; `escapePipeChars` for markdown tables; phases sorted by ID, tasks ordered by ID within phases |

#### Key Technical Decisions

1. **`MockAgent` builder pattern** -- `WithRunFunc`/`WithRateLimit`/`WithPrereqError` enable precise test scenarios without a real agent process
2. **Custom `[[`/`]]` template delimiters** -- Avoids conflicts with `{{`/`}}` appearing in task spec Markdown content passed as template data
3. **Per-provider rate-limit tracking** -- Maps multiple agents sharing the same API key (claude→anthropic, codex→openai) so one rate limit blocks all agents on that provider
4. **`DetectSignals` as exported function** -- Keeps signal parsing testable independently of the loop runner's internal state
5. **Atomic `WriteFile` (tmp → rename)** -- Prevents partial progress files if the process is killed mid-write

#### Key Files Reference

| Purpose | Location |
| ------- | -------- |
| Task spec markdown parser | `internal/task/parser.go` |
| Task spec test fixtures | `internal/task/testdata/task-specs/` |
| Task state manager | `internal/task/state.go` |
| Task state test fixtures | `internal/task/testdata/state/` |
| Phase configuration parser | `internal/task/phases.go` |
| Phase config test fixtures | `internal/task/testdata/phases/` |
| Dependency resolver & task selector | `internal/task/selector.go` |
| Status command | `internal/cli/status.go` |
| Agent interface & registry | `internal/agent/agent.go` |
| Mock agent for testing | `internal/agent/mock.go` |
| Claude agent adapter | `internal/agent/claude.go` |
| Codex agent adapter | `internal/agent/codex.go` |
| Gemini agent stub | `internal/agent/gemini.go` |
| Rate-limit coordinator | `internal/agent/ratelimit.go` |
| Prompt template system | `internal/loop/prompt.go` |
| Implementation loop runner | `internal/loop/runner.go` |
| Loop recovery (rate-limit, dirty-tree) | `internal/loop/recovery.go` |
| Implementation CLI command | `internal/cli/implement.go` |
| Dependency declarations | `tools.go` |

#### Verification

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass
- `go test -race ./...` pass
- `go mod tidy` produces no diff

---

### Phase 3: Review Pipeline (T-031 to T-042)

- **Status:** Completed
- **Date:** 2026-02-18
- **Tasks Completed:** 13 tasks (T-031 to T-042, including T-058, T-059)

#### Features Implemented

| Feature | Tasks | Description |
| ------- | ----- | ----------- |
| Review Types & Schema | T-031 | `Verdict`, `Severity`, `Finding`, `ReviewResult`, `ConsolidatedReview`, `ReviewConfig`, `ReviewOpts` types with validation and JSON round-trip support |
| Git Diff Generation | T-032 | `git.Client` interface, `DiffGenerator` with extension filtering, risk classification, `SplitFiles` round-robin distribution |
| Review Prompt Synthesis | T-033 | `ContextLoader`, `PromptBuilder`, embedded template with `[[`/`]]` delimiters, project brief/rules injection, diff truncation, path security |
| Finding Consolidation | T-034 | `Consolidator` with O(n) deduplication, severity escalation, verdict aggregation, agent attribution, `ConsolidationStats` |
| Multi-Agent Orchestrator | T-035 | `ReviewOrchestrator` fan-out via errgroup, per-agent error capture, `ReviewEvent` streaming, `DryRun` support |
| JSON Extraction | T-058 | `internal/jsonutil` package: `ExtractInto`/`ExtractFirst` brace-counting extraction from freeform AI output |
| Review Report Generation | T-036 | `ReportGenerator` producing markdown reports with findings table, by-file/severity breakdowns, agent breakdown, consolidation stats |
| Verification Runner | T-037 | `VerificationRunner` executing shell commands with per-command timeouts, `FormatReport()`/`FormatMarkdown()` output, two-tier truncation |
| Fix Engine | T-038 | `FixEngine` iterative fix-verify cycles, `FixPromptBuilder`, `FixEvent` streaming, `FixReport` aggregation |
| PR Body Generation | T-039 | `PRBodyGenerator` with AI summary, `GenerateTitle`, heading adjustment, 65,536-byte GitHub limit enforcement |
| PR Creation | T-040 | `PRCreator` wrapping `gh pr create`, `CheckPrerequisites`, `EnsureBranchPushed`, branch-name injection guard |
| `raven review` CLI | T-041 | Cobra command wiring full review pipeline; `--agents`, `--mode`, `--concurrency`, `--base`, `--output` flags; exit code semantics |
| `raven fix` & `raven pr` CLI | T-042 | Cobra commands for fix-verify cycles and PR creation; auto-detect review report; `StringArrayVar` for repeatable flags |

#### Key Technical Decisions

1. **`[[`/`]]` template delimiters** -- avoids conflicts with `{{`/`}}` appearing in JSON/code snippets within AI-generated finding descriptions; used consistently across all embedded templates in the review package
2. **`git.Client` interface in `git` package** -- prevents import cycles when `review` package injects mocks; compile-time checked via `var _ Client = (*GitClient)(nil)`
3. **Per-agent errors never abort the pipeline** -- errgroup workers always return `nil`; failures captured in `AgentError` slices so one flaky agent cannot cancel the others
4. **`collectCandidates` outer-first ordering** -- `jsonutil.ExtractInto` naturally prefers the outermost JSON blob, matching the typical AI response structure
5. **Pre-sorted key slices in report/PR body data structs** -- makes template rendering deterministic without requiring template-side logic
6. **Two-tier output truncation in `VerificationRunner`** -- byte-based for large-line inputs, line-based head+tail for high-line-count inputs
7. **`sh -c` / `cmd /c` shell wrapper** -- handles glob patterns (`./...`), pipes, and redirects in verification commands without custom parsing
8. **Agent errors in `GenerateSummary` silently swallowed** -- callers always receive a usable summary string; `PRBodyGenerator` never propagates agent failures to the caller
9. **`DiffNumStat` added to `git.Client` interface** -- `Generate` requires per-file line counts for `DiffStats`; binary files (`-1` sentinel) clamped to 0
10. **Branch-name injection guard via allowlist regex** -- `^[a-zA-Z0-9_./-]+$` prevents shell injection in `DiffGenerator` and `PRCreator`

#### Key Files Reference

| Purpose | Location |
| ------- | -------- |
| Review types, `Verdict`, `Severity`, `Finding`, `ReviewResult` | `internal/review/types.go` |
| `git.Client` interface, `DiffNumStat`, numstat parsers | `internal/git/client.go` |
| `DiffGenerator`, `SplitFiles`, `ChangedFile`, `DiffResult` | `internal/review/diff.go` |
| `ContextLoader`, `PromptBuilder`, `PromptData` | `internal/review/prompt.go` |
| Default review prompt template | `internal/review/review_template.tmpl` |
| `Consolidator`, `AggregateVerdicts`, `EscalateSeverity` | `internal/review/consolidate.go` |
| `ReviewOrchestrator`, `ReviewEvent`, `OrchestratorResult` | `internal/review/orchestrator.go` |
| `ExtractInto`, `ExtractFirst` JSON extraction | `internal/jsonutil/jsonutil.go` |
| `ReportGenerator`, `ReportData` | `internal/review/report.go` |
| Markdown report template | `internal/review/report_template.tmpl` |
| `VerificationRunner`, `CommandResult`, `VerificationReport` | `internal/review/verify.go` |
| `FixEngine`, `FixPromptBuilder`, `FixEvent`, `FixReport` | `internal/review/fix.go` |
| Fix prompt template | `internal/review/fix_template.tmpl` |
| `PRBodyGenerator`, `GenerateTitle`, `GenerateSummary` | `internal/review/prbody.go` |
| PR body template | `internal/review/prbody_template.tmpl` |
| `PRCreator`, `PRCreateOpts`, `EnsureBranchPushed` | `internal/review/pr.go` |
| `raven review` Cobra command | `internal/cli/review.go` |
| `raven fix` Cobra command | `internal/cli/fix.go` |
| `raven pr` Cobra command | `internal/cli/pr.go` |

#### Verification

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass

---

### Phase 8: Headless Observability (T-088, T-089)

- **Status:** Completed
- **Date:** 2026-02-18
- **Tasks Completed:** 2 tasks

#### Features Implemented

| Feature | Tasks | Description |
| ------- | ----- | ----------- |
| Stream-JSON event types & JSONL decoder | T-088 | `StreamEvent`, `StreamMessage`, `ContentBlock`, `StreamUsage` types mapping to Claude Code's `--output-format stream-json` JSONL schema; `StreamDecoder` with `Next()` iterator and `Decode()` channel producer; helper methods (`ToolUseBlocks()`, `TextContent()`, `ToolResultBlocks()`, `IsText()`, `IsToolUse()`, `IsToolResult()`, `InputString()`, `ContentString()`); `OutputFormatJSON` and `OutputFormatStreamJSON` constants; `StreamEvents` channel field in `RunOpts` for real-time event delivery |
| Stream-JSON wiring into adapters & loop runner | T-089 | `ClaudeAgent.Run()` streams JSONL via `io.TeeReader` + `StreamDecoder` when `opts.StreamEvents != nil && opts.OutputFormat == "stream-json"`; `RunResult.Stdout` still captures full output for backward compat; non-blocking channel sends drop events for slow consumers; `CodexAgent.Run()` documented as ignoring `StreamEvents`; loop runner `invokeAgent()` always passes a streaming channel, launches `consumeStreamEvents` goroutine; 4 new `LoopEventType` constants (`tool_started`, `tool_completed`, `agent_thinking`, `session_stats`); 4 new `LoopEvent` fields (`ToolName`, `CostUSD`, `TokensIn`, `TokensOut`); `DetectSignalsFromJSONL` exported helper for signal detection in stream-json output; `detectSignals` method falls through to JSONL scan when plain-text scan finds no signal |

#### Key Technical Decisions

1. **`--output-format stream-json` over `--output-format json`** -- Real-time JSONL streaming gives per-tool-call observability without direct API access, staying within the v2.0 "shell out to CLI" architecture
2. **`json.RawMessage` for `Input` and `Content` fields** -- Tool inputs and results have varying JSON shapes; `RawMessage` defers parsing to consumers
3. **1MB scanner buffer** -- Claude Code tool results can exceed the default 64KB `bufio.Scanner` limit
4. **Non-blocking `Decode()` with context cancellation** -- Prevents goroutine leaks when the TUI or automation script stops consuming events
5. **`io.TeeReader` in ClaudeAgent.Run()** -- Allows the `StreamDecoder` to read from the pipe while simultaneously writing all bytes to `stdoutBuf` for backward-compatible `RunResult.Stdout`
6. **Guard condition: both `StreamEvents != nil` AND `OutputFormat == "stream-json"`** -- Avoids accidentally activating JSONL parsing on plain-text or JSON-format output
7. **Channel owned by `invokeAgent`, closed after `Run()` returns** -- Per spec, the agent adapter never closes the channel; `invokeAgent` closes it and awaits `consumerDone` to drain remaining buffered events before returning
8. **`DetectSignalsFromJSONL` as two-pass fallback** -- `detectSignals` first tries plain-text scan (backward compat), then falls back to JSONL scan; signals embedded in assistant text blocks are found in either mode

#### Key Files Reference

| Purpose | Location |
| ------- | -------- |
| Stream event types & decoder | `internal/agent/stream.go` |
| Stream decoder tests (35 tests) | `internal/agent/stream_test.go` |
| Agent types with streaming support | `internal/agent/types.go` |
| ClaudeAgent with streaming integration | `internal/agent/claude.go` |
| CodexAgent (StreamEvents doc comment) | `internal/agent/codex.go` |
| Loop runner with consumeStreamEvents | `internal/loop/runner.go` |
| Full session JSONL fixture | `testdata/stream-json/session-full.jsonl` |
| Error session fixture | `testdata/stream-json/session-error.jsonl` |
| Malformed/mixed fixture | `testdata/stream-json/malformed-mixed.jsonl` |
| Task specification (T-088) | `docs/tasks/T-088-headless-observability.md` |

#### Verification

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./internal/agent/` pass
- `go test ./internal/loop/` pass

---


---

### Phase 4: Workflow Engine & Pipeline (T-043 to T-055)

- **Status:** Completed
- **Date:** 2026-02-18
- **Tasks Completed:** 12 tasks

#### Features Implemented

| Feature | Tasks | Description |
| ------- | ----- | ----------- |
| Workflow Event Types & Constants | T-043 | Six transition events, nine lifecycle constants, `StepHandler` interface, `WorkflowEvent`/`WorkflowDefinition`/`StepDefinition` types |
| Step Handler Registry | T-044 | `Registry` struct with register/get/list/has, `ErrStepNotFound` sentinel, `DefaultRegistry` singleton and delegation functions |
| Workflow Engine (State Machine) | T-045 | `Engine` struct with functional options, `Run`/`RunStep`/`Validate` methods, panic-safe `safeExecute`, non-blocking event emission |
| State Checkpointing & Persistence | T-046 | `StateStore` with atomic file writes (tmp→fsync→rename), `RunSummary`, `StatusFromState`, `WithCheckpointing` engine option |
| Resume Command | T-047 | `resume` Cobra command with list/clean/clean-all/resume modes, tabwriter output, TTY detection, `runIDPattern` path-traversal guard |
| Workflow Definition Validation | T-048 | `ValidateDefinition` with 6-phase BFS/DFS analysis, nine `Issue*` constants, `ValidationResult`/`ValidationIssue` structs, cycles as warnings |
| Built-in Workflows & Step Handlers | T-049 | Four built-in workflow definitions, 11 step handlers (`ImplementHandler`, `ReviewHandler`, `FixHandler`, `PRHandler`, phase handlers, PRD stubs), `RegisterBuiltinHandlers` |
| Pipeline Orchestrator Core | T-050 | `PipelineOrchestrator` with multi-phase lifecycle, `PipelineOpts`/`PhaseResult`/`PipelineResult`, skip flags, resume-from-checkpoint, dry-run plan |
| Pipeline Branch Management | T-051 | `BranchManager` with template-based branch naming, `slugify`, `EnsureBranch` idempotent create-or-switch, integration + fuzz tests |
| Pipeline Metadata Tracking | T-052 | `PipelineMetadata`/`PhaseMetadata` structs, JSON round-trip for `WorkflowState.Metadata`, stage-level status updates, `NextIncompletePhase`/`IsComplete`/`Summary` |
| Pipeline Interactive Wizard | T-053 | `RunWizard` with 4 sequential `huh.Form` pages for phase mode, agent selection, options, and confirmation; `ErrWizardCancelled` sentinel |
| Dry-Run Formatting | T-054 | `DryRunFormatter` with BFS graph walk, cycle annotation, `FormatWorkflowDryRun`/`FormatPipelineDryRun`, styled/plain output, `PipelineDryRunInfo` decoupling struct |
| Pipeline CLI Command | T-055 | `pipeline` Cobra command with 15 flags, wizard integration, TTY detection, flag validation, shell completions, exit-code mapping (0/1/2/3), dry-run mode |

#### Key Technical Decisions

1. **Cycles as warnings, not errors** -- `review → fix → review` loops are intentional; `ValidateDefinition` emits `IssueCycleDetected` as a warning so `IsValid()` remains true
2. **Atomic checkpoint writes** -- `StateStore.Save` marshals to `<id>.json.tmp`, fsyncs, then renames; crash-safe without external locking
3. **`applySkipFlags` deep-copies builtin definition** -- `GetDefinition` returns a pointer into a package-level singleton; mutation would corrupt all subsequent calls
4. **`PipelineDryRunInfo` plain struct breaks import cycle** -- `pipeline` imports `workflow`; a plain struct in `workflow` avoids the reverse dependency
5. **Multiple sequential `huh.Form` runs in wizard** -- Conditional pages (phase picker, agent selection) are cleanest as separate forms; avoids complex `HideFunc` logic
6. **`int64` nanoseconds for `PhaseMetadata.Duration`** -- Explicit `_ns` suffix makes unit unambiguous for non-Go consumers; avoids silent precision loss
7. **Fresh `workflow.NewRegistry()` per pipeline run** -- Prevents registration panics from the package-level `DefaultRegistry` singleton being populated by `init()` functions
8. **TTY detection via `os.ModeCharDevice`** -- Avoids adding `golang.org/x/term`; stdlib-only `os.Stdin.Stat()` / `os.Stdout.Stat()` suffice
9. **`runIDPattern` allowlist in resume command** -- Only `[a-zA-Z0-9_-]` permitted in `--run`/`--clean` to prevent path traversal to arbitrary JSON files
10. **Terminal pseudo-steps excluded from adjacency graph** -- `StepDone`/`StepFailed` have no outgoing transitions; including them would incorrectly suppress reachability warnings

#### Key Files Reference

| Purpose | Location |
| ------- | -------- |
| Workflow event constants, `StepHandler` interface, `WorkflowEvent`/`WorkflowDefinition` types | `internal/workflow/events.go` |
| Step handler `Registry`, `ErrStepNotFound`, `DefaultRegistry` | `internal/workflow/registry.go` |
| `Engine` state machine runner with functional options | `internal/workflow/engine.go` |
| `StateStore` atomic persistence, `WithCheckpointing` option | `internal/workflow/state.go` |
| `ValidateDefinition` BFS/DFS validation, `ValidationResult` | `internal/workflow/validate.go` |
| Built-in workflow definitions and name constants | `internal/workflow/builtin.go` |
| 11 built-in step handlers (`ImplementHandler`, `ReviewHandler`, etc.) | `internal/workflow/handlers.go` |
| `DryRunFormatter`, `FormatWorkflowDryRun`, `FormatPipelineDryRun` | `internal/workflow/dryrun.go` |
| `PipelineOrchestrator`, `PipelineOpts`, `PhaseResult`, `PipelineResult` | `internal/pipeline/orchestrator.go` |
| `BranchManager`, `slugify`, `EnsureBranch` | `internal/pipeline/branch.go` |
| `PipelineMetadata`, `PhaseMetadata`, stage-level status helpers | `internal/pipeline/metadata.go` |
| `RunWizard` interactive pipeline configuration wizard | `internal/pipeline/wizard.go` |
| `pipeline` Cobra command with 15 flags and wizard integration | `internal/cli/pipeline.go` |
| `resume` Cobra command with list/clean/resume modes | `internal/cli/resume.go` |

#### Verification

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass

---

### Phase 5: PRD Decomposition (T-056 to T-065)

- **Status:** Completed
- **Date:** 2026-02-18
- **Tasks Completed:** 10 tasks

#### Features Implemented

| Feature | Tasks | Description |
| ------- | ----- | ----------- |
| Epic JSON Schema & Types | T-056 | `EpicBreakdown`, `Epic`, `EpicTaskResult`, `TaskDef`, `ValidationError` structs with validation, parse helpers, and golden/fuzz tests |
| PRD Shredder | T-057 | Single-agent PRD-to-epics call with retry loop, embedded prompt template, event tracking, and JSON extraction from file or stdout |
| JSON Extraction Utility | T-058 | Multi-strategy `Extract`/`ExtractAll`/`ExtractInto`/`ExtractFromFile` API: markdown fences, brace/bracket matching, ANSI stripping, BOM removal, 10 MB cap |
| Parallel Epic Workers | T-059 | `ScatterOrchestrator` with bounded `errgroup` concurrency, per-epic retry loop, rate-limiter integration, partial-failure handling, and concurrent-safe `MockAgent` |
| Merge: Global ID Assignment | T-060 | Kahn's topological sort of epics (`SortEpicsByDependency`) + sequential `T-NNN` assignment (`AssignGlobalIDs`) with deterministic lexicographic ordering |
| Merge: Dependency Remapping | T-061 | `RemapDependencies` resolving local temp IDs and cross-epic `E-NNN:label` references to global IDs, with unresolved/ambiguous reporting |
| Merge: Title Deduplication | T-062 | `DeduplicateTasks` with `NormalizeTitle` (prefix stripping, punctuation removal), AC merging, dependency rewriting, and `DedupReport` |
| Merge: DAG Validation | T-063 | `ValidateDAG` (Kahn + DFS cycle tracing), `TopologicalDepths`, dangling/self/cycle error reporting, 10 000-task guard |
| Task File Emitter | T-064 | `Emitter` producing per-task `.md` files, `task-state.conf`, `phases.conf`, `PROGRESS.md`, `INDEX.md` (with Mermaid graph), atomic tmp+rename writes |
| PRD CLI Command | T-065 | `raven prd` Cobra command: 4-phase pipeline (Shred→Scatter→Merge→Emit), single-pass mode, dry-run, partial-success exit code 2, SIGINT/SIGTERM handling |

#### Key Technical Decisions

1. **Kahn's algorithm for epic and task DAG** -- produces cleaner cycle reports ("unprocessed nodes") vs DFS; queue re-sorted after each step for lexicographic determinism
2. **Custom template delimiters `[[` `]]`** -- prevents conflicts when task descriptions/ACs contain Go `{{ }}` template syntax
3. **`errgroup.SetLimit` for scatter concurrency** -- bounded parallel execution; workers return nil so sibling goroutines continue on partial failure
4. **`scatterValidationFailure` sentinel** -- distinguishes validation exhaustion (non-fatal) from fatal errors (context cancel, rate-limit exceeded)
5. **Multi-strategy JSON extraction** -- code fence first, then brace/bracket matching; `fenceSpan` tracking prevents double-emitting fence content via brace scanner
6. **Single-pass mode as `concurrency=1`** -- reuses all pipeline stages rather than a separate prompt path; simpler and correct
7. **`errPartialSuccess` custom type** -- carries structured data (totalEpics, failedEpics) for future exit-code mapping; type-asserted (not `errors.As`) since it is never wrapped
8. **Atomic write via tmp+rename** -- all emitted files written to a temp path then renamed, preventing partial writes on error
9. **`ResequenceIDs` closes dedup gaps** -- remaps all `Dependencies` references through `IDMapping` after re-sequencing; skips remap when no gaps exist

#### Key Files Reference

| Purpose | Location |
| ------- | -------- |
| Epic/task JSON types, validation, parse helpers | `internal/prd/schema.go` |
| Schema tests, golden snapshots, fuzz seeds | `internal/prd/schema_test.go` |
| PRD shredder (single-agent, retry, events) | `internal/prd/shredder.go` |
| Shredder tests | `internal/prd/shredder_test.go` |
| Parallel scatter orchestrator | `internal/prd/worker.go` |
| Scatter tests | `internal/prd/worker_test.go` |
| Merger: ID assignment, dep remapping, dedup, DAG | `internal/prd/merger.go` |
| Merger tests | `internal/prd/merger_test.go` |
| Task file emitter | `internal/prd/emitter.go` |
| Emitter tests | `internal/prd/emitter_test.go` |
| `raven prd` CLI command | `internal/cli/prd.go` |
| PRD CLI tests | `internal/cli/prd_test.go` |
| JSON extraction utility | `internal/jsonutil/extract.go` |
| JSON extraction tests | `internal/jsonutil/extract_test.go` |
| Concurrent-safe mock agent | `internal/agent/mock.go` |
| Test fixtures (valid/invalid JSON) | `internal/prd/testdata/` |

#### Verification

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass

---

### Phase 6: TUI Command Center -- Initial Task (T-066)

- **Status:** Completed (partial phase)
- **Date:** 2026-02-18
- **Tasks Completed:** 1 task (T-066)

#### Features Implemented

| Feature | Tasks | Description |
| ------- | ----- | ----------- |
| Bubble Tea App Scaffold | T-066 | `FocusPanel` enum (Sidebar/AgentPanel/EventLog), `AppConfig` struct, `App` top-level `tea.Model`; `NewApp` with default focus=Sidebar, ready=false, quitting=false; `Init()` returning nil; `Update()` handling WindowSizeMsg (store dims, set ready=true) and quit KeyMsgs (q/ctrl+c/ctrl+q → tea.Quit); `View()` with quitting/not-ready/too-small/full branches using lipgloss title bar; `RunTUI` with WithAltScreen + WithMouseCellMotion |

#### Key Technical Decisions

1. **Value receiver for App methods** -- Elm architecture is purely functional; each Update returns a new model copy, matching bubbletea's serialized state update guarantees
2. **`tea.WithMouseCellMotion()` not `WithMouseAllMotion()`** -- preserves user ability to select and copy text from the terminal
3. **`Init()` returns nil** -- bubbletea v1.x sends `WindowSizeMsg` automatically on startup; no explicit request needed
4. **Minimum terminal size guard (80x24) in `View()`** -- prevents garbled layout when the terminal is too narrow/short; shown before the full render path
5. **lipgloss for all rendering** -- consistent with the charmbracelet ecosystem; color constants use terminal-256 palette codes to stay within CGO_ENABLED=0

#### Key Files Reference

| Purpose | Location |
| ------- | -------- |
| TUI App Scaffold | `internal/tui/app.go` |
| TUI App Tests | `internal/tui/app_test.go` |
| Package doc | `internal/tui/doc.go` |

#### Verification

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./internal/tui/...` pass

---

### Phase 6: TUI Command Center (T-066 to T-078)

- **Status:** Completed
- **Date:** 2026-02-19
- **Tasks Completed:** 13 tasks

#### Features Implemented

| Feature | Tasks | Description |
| ------- | ----- | ----------- |
| Elm Architecture Scaffold | T-066 | `App` top-level `tea.Model` with `Init`/`Update`/`View`, `FocusPanel` enum, `AppConfig`, `RunTUI` entry point |
| TUI Message Types and Event System | T-067 | `AgentOutputMsg`, `AgentStatusMsg`, `WorkflowEventMsg`, `LoopEventMsg`, `RateLimitMsg`, `TaskProgressMsg`, `TickMsg`, `ErrorMsg`, `FocusChangedMsg`; `AgentStatus`/`LoopEventType` enums; `TickCmd`/`TickEvery` helpers |
| Lipgloss Styles and Theme System | T-068 | 11 `AdaptiveColor` palette vars, `Theme` struct with 37 `lipgloss.Style` fields, `DefaultTheme()`, `StatusIndicator(AgentStatus)`, `ProgressBar(filled, width)` |
| Split-Pane Layout Manager | T-069 | `Layout` with `Resize`/`IsTooSmall`/`TerminalSize`/`Render`/`RenderTooSmall`; `PanelDimensions`; 6 exported dimension constants |
| Sidebar: Workflow List | T-070 | `SidebarModel` with `WorkflowEntry`/`WorkflowStatus`, O(1) dedup via map, j/k navigation, scroll-tracking, status indicators |
| Sidebar: Task Progress Bars | T-071 | `TaskProgressSection` with `SetTotals`/`SetPhase`/`Update`/`View`; handles `TaskProgressMsg` and `LoopEventMsg`; integrated into `SidebarModel` |
| Sidebar: Rate-Limit Countdown | T-072 | `RateLimitSection` with `ProviderRateLimit`, per-provider WAIT M:SS countdown, `TickCmd` self-scheduling, `formatCountdown`; integrated into `SidebarModel` |
| Agent Output Panel | T-073 | `AgentPanelModel` with `OutputBuffer` ring buffer (1000 lines), `AgentView` per-agent viewport, tab bar, auto-scroll, Tab passthrough |
| Event Log Panel | T-074 | `EventLogModel` with 500-entry bounded buffer, `EventEntry`/`EventCategory`, classifier functions for all message types, visibility toggle (`l`), auto-scroll |
| Status Bar | T-075 | `StatusBarModel` tracking phase/task/iteration/elapsed/paused/mode; two-pass segment layout; PAUSED badge; `formatElapsed` |
| Keyboard Navigation and Help Overlay | T-076 | `KeyMap` with 15 bindings via `charmbracelet/bubbles/key`, `DefaultKeyMap`, `NextFocus`/`PrevFocus` cycling, `HelpOverlay` with centered rounded-border box, `PauseRequestMsg`/`SkipRequestMsg` |
| Pipeline Wizard | T-077 | `WizardModel` wrapping 5-group `huh.Form`; `PipelineWizardConfig`; `WizardCompleteMsg`/`WizardCancelledMsg`; `buildHuhTheme`; `positiveIntValidator`; `parseFormValues` |
| Dashboard Command and Integration | T-078 | `NewDashboardCmd` Cobra subcommand with `--dry-run` support; `EventBridge` with `WorkflowEventCmd`/`LoopEventCmd`/`AgentOutputCmd`/`TaskProgressCmd` and goroutine-safe push functions; `App` fully integrated with all sub-models |

#### Key Technical Decisions

1. **Value-receiver sub-models** -- `TaskProgressSection`, `RateLimitSection`, and `StatusBarModel` use value receivers returning updated copies, matching Elm architecture immutability guarantees and avoiding TUI race conditions
2. **Ring buffer capped at 1000 lines** -- `OutputBuffer` in `AgentPanelModel` uses O(1) append/eviction to bound memory regardless of agent verbosity
3. **`TickCmd` self-scheduling** -- Rate-limit countdown returns `TickCmd(time.Second)` only while a limit is active, eliminating unnecessary timer ticks when all providers are clear
4. **Two-pass status bar layout** -- Mandatory segments (mode, task) always rendered; optional segments (phase, iter, timer) dropped when width is insufficient, avoiding truncation artifacts
5. **`buildHuhTheme`** -- Translates Raven's `lipgloss.AdaptiveColor` palette into a custom `huh.ThemeBase()` derivative so the wizard respects the TUI's light/dark color scheme
6. **`mapLoopEventType`** -- `EventBridge` converts `loop.LoopEventType` string constants to TUI `LoopEventType` int iota values, decoupling backend and TUI event systems
7. **Border-box width accounting** -- Sidebar and status bar use `Width(m.width - 1)` / `Width(sb.width)` (total terminal cells) rather than inner content width to prevent lipgloss border-box overflow

#### Key Files Reference

| Purpose | Location |
| ------- | -------- |
| Top-level Elm model, `FocusPanel`, `AppConfig`, `RunTUI` | `internal/tui/app.go` |
| All TUI message types, enums, `TickCmd`/`TickEvery` | `internal/tui/messages.go` |
| Color palette, `Theme`, `DefaultTheme`, `StatusIndicator`, `ProgressBar` | `internal/tui/styles.go` |
| `Layout`, `PanelDimensions`, dimension constants | `internal/tui/layout.go` |
| `SidebarModel`, `WorkflowEntry`, `TaskProgressSection`, `RateLimitSection` | `internal/tui/sidebar.go` |
| `AgentPanelModel`, `OutputBuffer`, `AgentView` | `internal/tui/agent_panel.go` |
| `EventLogModel`, `EventEntry`, `EventCategory`, classifier functions | `internal/tui/event_log.go` |
| `StatusBarModel`, `formatElapsed` | `internal/tui/status_bar.go` |
| `KeyMap`, `DefaultKeyMap`, `NextFocus`/`PrevFocus`, `HelpOverlay`, `PauseRequestMsg`, `SkipRequestMsg` | `internal/tui/keybindings.go` |
| `WizardModel`, `PipelineWizardConfig`, `WizardCompleteMsg`, `WizardCancelledMsg`, `buildHuhTheme` | `internal/tui/wizard.go` |
| `EventBridge`, `mapLoopEventType`, Cmd-based and goroutine-safe bridge helpers | `internal/tui/bridge.go` |
| `NewDashboardCmd`, `dashboardRun` | `internal/cli/dashboard.go` |
| Root command with `NewDashboardCmd` registration | `internal/cli/root.go` |

#### Verification

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass

---

## In Progress Tasks

_None currently_

---

### T-079: GoReleaser Configuration for Cross-Platform Builds

- **Status:** Completed
- **Date:** 2026-02-19
- **What was built:**
  - `.goreleaser.yaml` (GoReleaser v2) at project root with `version: 2` schema
  - Cross-platform build matrix: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64 (windows/arm64 excluded)
  - `CGO_ENABLED=0` for pure-Go cross-compilation with `-s -w` symbol stripping
  - ldflags version injection for `internal/buildinfo.Version`, `.Commit`, `.Date` using full module path
  - Archives: `.tar.gz` for macOS/Linux, `.zip` for Windows with version-stamped names
  - SHA256 `checksums.txt` generation
  - Changelog filter excluding docs/test/ci/chore commits
  - GitHub release block with `GITHUB_OWNER` env variable and `prerelease: auto`
  - `release-snapshot` Makefile target invoking `goreleaser build --snapshot --clean`
- **Files created/modified:**
  - `.goreleaser.yaml` -- GoReleaser v2 configuration for cross-platform builds and release packaging
  - `Makefile` -- added `release-snapshot` target and updated `.PHONY` list
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

### T-080: GitHub Actions Release Automation Workflow

- **Status:** Completed
- **Date:** 2026-02-19
- **What was built:**
  - `.github/workflows/release.yml` GitHub Actions workflow triggered on `v*` tag pushes
  - `actions/checkout@v4` with `fetch-depth: 0` for full git history (required for changelog generation)
  - `actions/setup-go@v5` reading Go version from `go.mod` (no hardcoded version)
  - `go vet ./... && go test ./...` gate before any release artifact is built
  - Binary size verification step enforcing 25MB limit on linux/amd64 build
  - `goreleaser/goreleaser-action@v6` running `release --clean` with `GITHUB_TOKEN`
  - `permissions: contents: write` for GitHub Release creation
- **Files created/modified:**
  - `.github/workflows/release.yml` -- GitHub Actions release workflow for automated cross-platform builds and publishing
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

### T-081: Shell Completion Installation Scripts and Packaging

- **Status:** Completed
- **Date:** 2026-02-19
- **What was built:**
  - `scripts/completions/install.sh` universal shell completion installer that auto-detects bash/zsh/fish from `$SHELL` and installs to the correct user or system directory
  - `scripts/gen-completions/main.go` Go program that imports `cli.NewRootCmd()` and generates all four completion files (bash, zsh, fish, powershell) into a specified output directory
  - `cli.NewRootCmd()` exported function in `internal/cli/root.go` for use by external tools without the global rootCmd singleton
  - Updated `.goreleaser.yaml` to run `go run ./scripts/gen-completions completions` as a `before.hook` and include `completions/*` + `install.sh` in archives; added standalone `completions-archive` meta artifact
  - `make completions` Makefile target for local completion generation
  - Added `completions/` to `.gitignore` (generated artifact, not committed)
- **Files created/modified:**
  - `scripts/completions/install.sh` -- universal bash/zsh/fish completion installer with auto-detection and idempotent installs
  - `scripts/gen-completions/main.go` -- Go program generating all four shell completion files for GoReleaser packaging
  - `internal/cli/root.go` -- added `NewRootCmd()` exported function
  - `.goreleaser.yaml` -- added before hook, extra files in archives, standalone completions archive
  - `Makefile` -- added `completions` target
  - `.gitignore` -- added `completions/` entry
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

### T-082: Man Page Generation Using cobra/doc

- **Status:** Completed
- **Date:** 2026-02-19
- **What was built:**
  - `scripts/gen-manpages/main.go` Go program that imports `cli.NewRootCmd()` and calls `doc.GenManTree()` to generate troff-formatted Section 1 man pages for every Raven command into a configurable output directory (default `man/man1`)
  - `GenManHeader` configured with `Title:"RAVEN"`, `Section:"1"`, `Source:"Raven"`, `Manual:"Raven Manual"` so man pages display correctly under `man raven`, `man raven-implement`, etc.
  - `scripts/install-manpages.sh` bash installer that copies `man/man1/*.1` to `${1:-/usr/local/share/man/man1}`, prints each installed path, and runs `mandb`/`makewhatis` when available to update the man database
  - Updated `.goreleaser.yaml`: added `go run ./scripts/gen-manpages man/man1` as a `before.hook` (after completions hook); added `man/man1/*.1` and `scripts/install-manpages.sh` to the `raven-archive` extra files; added standalone `manpages-archive` meta archive
  - `make manpages` Makefile target for local man page generation
  - Added `/man/` to `.gitignore` (generated artifact, not committed)
- **Files created/modified:**
  - `scripts/gen-manpages/main.go` -- Go program generating troff man pages for all Raven commands
  - `scripts/install-manpages.sh` -- bash installer copying man pages to system man directory with db update
  - `.goreleaser.yaml` -- added before hook, man pages in archives, standalone manpages-archive meta
  - `Makefile` -- added `manpages` target and updated `.PHONY`
  - `.gitignore` -- added `/man/` entry
- **Key decisions:**
  - `github.com/spf13/cobra/doc` is a sub-package of the already-required `github.com/spf13/cobra` module (v1.10.2); no new module version needed. `go-md2man` (transitive dep) is added to go.mod/go.sum via `go mod tidy` when the new import is first resolved.
  - `nullglob` in the installer prevents a bare `*.1` glob from being passed to `cp` when the source directory is empty.
  - Man pages for hidden commands (e.g., `__complete`) are omitted automatically by cobra/doc because they are marked hidden.
- **Verification:** `go build ./cmd/raven/` ✓  `go vet ./...` ✓  `go test ./...` ✓  `go run ./scripts/gen-manpages man/man1` generates man pages ✓

---

### T-083: Performance Benchmarking Suite

- **Status:** Completed
- **Date:** 2026-02-19
- **What was built:**
  - `cmd/raven/bench_test.go` -- binary startup time benchmarks (`BenchmarkBinaryStartup`, `BenchmarkBinaryHelp`) measuring wall-clock time from process launch; establishes <200ms baseline
  - `internal/config/bench_test.go` -- config benchmarks: `LoadFromFile`, `Validate`, `NewDefaults`, `LoadAndValidate`, `DecodeAndValidate`, and stress variants with many agents/workflows
  - `internal/task/bench_test.go` -- task parsing (`ParseTaskSpec`, `ParseTaskFile`, `DiscoverTasks`), StateManager I/O (`Load`, `LoadMap`, `Get`), and selector benchmarks (`SelectNext`, `SelectNextInRange`, `IsPhaseComplete`, `BlockedTasks`) at 10- and 100-task scale
  - `internal/agent/bench_test.go` -- concurrent agent coordination overhead with 1, 3, 5, 10 agents via `errgroup`, plus `b.RunParallel` throughput benchmark
  - `internal/workflow/bench_test.go` -- state checkpoint/restore I/O: `Save`, `Load`, `List` (n=5/10/20), and pure JSON marshal benchmarks
  - `internal/tui/bench_test.go` -- view rendering at 120x40 (`BenchmarkAppView` ~218µs, well under <100ms target), message dispatch (`AgentOutputMsg`, `WorkflowEventMsg`), event log ring buffer, layout resize
  - `internal/review/bench_test.go` -- JSON extraction (`json.Unmarshal` on ReviewResult), finding consolidation with 3 and 5 agents, deduplication key computation
  - `scripts/bench-report.sh` -- shell script to run all benchmarks and format a header report with git ref, Go version, and configurable `-benchtime`/`-count`/`-bench` flags
  - Updated `Makefile` `bench` target to use `-benchtime=3s` per task spec
- **Files created/modified:**
  - `cmd/raven/bench_test.go` -- binary startup benchmarks using `os/exec`
  - `internal/config/bench_test.go` -- config load + validation benchmarks
  - `internal/task/bench_test.go` -- task parsing, state manager, selector benchmarks
  - `internal/agent/bench_test.go` -- concurrent agent coordination benchmarks
  - `internal/workflow/bench_test.go` -- workflow state checkpoint I/O benchmarks
  - `internal/tui/bench_test.go` -- TUI view rendering and message throughput benchmarks
  - `internal/review/bench_test.go` -- JSON extraction and consolidation benchmarks
  - `scripts/bench-report.sh` -- benchmark runner with formatted report output
  - `Makefile` -- updated `bench` target to `-benchtime=3s`
- **Verification:** `go build` ✓  `go vet` ✓  `go test ./...` ✓

---

### T-084: End-to-End Integration Test Suite with Mock Agents

- **Status:** Completed
- **Date:** 2026-02-19
- **What was built:**
  - `testdata/mock-agents/claude` -- Mock Claude CLI bash script; accepts all real claude flags, emits `PHASE_COMPLETE`, supports `MOCK_EXIT_CODE`, `MOCK_OUTPUT_FILE`, `MOCK_STDERR_FILE`, `MOCK_RATE_LIMIT`, `MOCK_DELAY`, `MOCK_SIGNAL_FILE` env vars
  - `testdata/mock-agents/codex` -- Mock Codex CLI bash script with same protocol as claude mock
  - `testdata/mock-agents/rate-limited-agent` -- Agent that rate-limits first N invocations (controlled by `MOCK_RATE_LIMIT_UNTIL`), succeeds on subsequent calls
  - `testdata/mock-agents/failing-agent` -- Agent that always fails with configurable exit code (`MOCK_EXIT_CODE`)
  - `tests/e2e/helpers_test.go` -- `testProject` helper struct, `newTestProject` (builds raven binary + copies mock agents), `initGitRepo`, `minimalConfig`, `sampleTaskSpec`, `run`/`runExpectSuccess`/`runExpectFailure` methods
  - `tests/e2e/config_test.go` -- `raven version`, `raven version --json`, `raven init go-cli`, `raven config debug`, `raven config validate`, missing-config fallback, no-args help, config help
  - `tests/e2e/implement_test.go` -- dry-run, single task with mock agent, no tasks error, dry-run no agent needed, required flags validation, dry-run does not invoke agent assertion
  - `tests/e2e/review_test.go` -- dry-run, help, empty diff, staged change, no-agents-configured error, split mode, invalid mode
  - `tests/e2e/pipeline_test.go` -- help, dry-run requires phase, dry-run with phases.conf, mutually exclusive flags, all stages skipped, invalid concurrency
  - `tests/e2e/prd_test.go` -- help, required file flag, file must exist, dry-run, single-pass dry-run
  - `tests/e2e/resume_test.go` -- help, no checkpoint error, list with no checkpoints, clean-all, invalid run ID, status command, status JSON output
  - `tests/e2e/error_test.go` -- unknown subcommand, unknown agent, invalid config, global dry-run flag, global verbose flag, global no-color flag, phase/task mutual exclusivity, review concurrency validation
  - `Makefile` -- added `test-e2e` target: `go test -v -count=1 -timeout 10m ./tests/e2e/`
- **Key decisions:**
  - External test package (`package e2e_test`) with no non-test `.go` files -- Go supports this for external test packages
  - `runtime.Caller(0)` in `projectRoot()` yields the compile-time path of `helpers_test.go`; navigates two levels up to find the repo root reliably regardless of working directory
  - All mock scripts use `set -euo pipefail` with `${VAR:-default}` pattern to be safe with `-u` (unset variable errors)
  - PATH override in subprocess env (`mock-agents` prepended) shadows real claude/codex binaries; no real API calls are made
  - `NO_COLOR=1` and `RAVEN_LOG_FORMAT=json` set in subprocess env for clean, parseable test output
  - All tests guard with `if testing.Short() { t.Skip() }` for rapid unit-test cycles
  - Git repo initialised with `test@example.com` identity to avoid global git config dependency
- **Files created/modified:**
  - `testdata/mock-agents/claude` -- Mock Claude CLI script (executable)
  - `testdata/mock-agents/codex` -- Mock Codex CLI script (executable)
  - `testdata/mock-agents/rate-limited-agent` -- Rate-limiting mock agent script (executable)
  - `testdata/mock-agents/failing-agent` -- Always-failing mock agent script (executable)
  - `tests/e2e/helpers_test.go` -- Test infrastructure helpers
  - `tests/e2e/config_test.go` -- Config and init command E2E tests
  - `tests/e2e/implement_test.go` -- Implementation loop E2E tests
  - `tests/e2e/review_test.go` -- Review pipeline E2E tests
  - `tests/e2e/pipeline_test.go` -- Pipeline orchestration E2E tests
  - `tests/e2e/prd_test.go` -- PRD decomposition E2E tests
  - `tests/e2e/resume_test.go` -- Checkpoint/resume and status E2E tests
  - `tests/e2e/error_test.go` -- Error handling and CLI validation E2E tests
  - `Makefile` -- added `test-e2e` target and updated `.PHONY`
- **Verification:** `go build ./cmd/raven/` ✓  `go vet ./...` ✓  `go test -short ./tests/e2e/` ✓

### T-085: CI/CD Pipeline with GitHub Actions

- **Status:** Completed
- **Date:** 2026-02-19
- **What was built:**
  - `.github/workflows/ci.yml` — four-job CI workflow (lint, test, build, mod-tidy) triggering on PRs and pushes to `main`
  - `.golangci.yml` — golangci-lint configuration enabling 16 linters (errcheck, gosec, staticcheck, errorlint, etc.) with test-file and mock-path exclusions
  - Lint job uses `golangci/golangci-lint-action@v6` with pinned `v1.63` version and 5-minute timeout
  - Test job runs on Ubuntu + macOS matrix with Go 1.24, race detection (`-race`), and coverage artifact upload
  - Build job verifies cross-compilation for 5 platforms (darwin/linux/windows × amd64/arm64, excluding windows/arm64) with `CGO_ENABLED=0`
  - Module hygiene job ensures `go mod tidy` produces no diff
  - Workflow uses read-only `contents: read` permission (principle of least privilege)
- **Files created/modified:**
  - `.github/workflows/ci.yml` — Main CI workflow for PRs and main branch
  - `.golangci.yml` — golangci-lint configuration at project root
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

## Not Started Tasks

### Phase 7: Polish & Distribution (T-079 to T-087)

- **Status:** Not Started
- **Tasks:** 9 (8 Must Have, 1 Should Have)
- **Estimated Effort:** 64-96 hours
- **PRD Roadmap:** Week 14

#### Task List

| Task | Name | Priority | Effort | Status |
|------|------|----------|--------|--------|
| T-079 | GoReleaser Configuration for Cross-Platform Builds | Must Have | Medium (6-10hrs) | Completed |
| T-080 | GitHub Actions Release Automation Workflow | Must Have | Medium (6-10hrs) | Completed |
| T-081 | Shell Completion Installation Scripts and Packaging | Should Have | Small (3-4hrs) | Completed |
| T-082 | Man Page Generation Using cobra/doc | Should Have | Small (2-4hrs) | Completed |
| T-083 | Performance Benchmarking Suite | Should Have | Medium (8-12hrs) | Completed |
| T-084 | End-to-End Integration Test Suite with Mock Agents | Must Have | Large (20-30hrs) | Completed |
| T-085 | CI/CD Pipeline with GitHub Actions | Must Have | Medium (6-10hrs) | Completed |
| T-086 | Comprehensive README and User Documentation | Must Have | Medium (8-12hrs) | Not Started |
| T-087 | Final Binary Verification and Release Checklist | Must Have | Medium (6-8hrs) | Not Started |

**Deliverable:** Published v2.0.0 with signed binaries for all platforms.

---

## Notes

### Key Technical Decisions (from agent research)

1. **BurntSushi/toml v1.5.0** -- MetaData.Undecoded() for unknown-key detection, simpler API for read-only use case
2. **charmbracelet/log over slog** -- Pretty terminal output with component prefixes, consistent with TUI ecosystem
3. **Bubble Tea v1.2+** -- Stable release, Elm architecture maps perfectly to workflow state machine
4. **CGO_ENABLED=0** -- Pure Go for cross-compilation, no C dependencies
5. **os/exec for all external tools** -- Shell out to claude, codex, gemini, git, gh CLIs
6. **Lightweight state machine** -- No external framework (Temporal/Prefect are overkill)
7. **JSON checkpoints** -- Workflow state persisted to `.raven/state/` after every transition

_Last updated: 2026-02-18_ (Phase 2 complete)
