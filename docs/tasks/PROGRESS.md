# Raven Task Progress Log

## Summary

| Status | Count |
|--------|-------|
| Completed | 82 |
| In Progress | 0 |
| Not Started | 7 |

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

### T-066: Bubble Tea Application Scaffold and Elm Architecture Model

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - `FocusPanel` iota enum type with `FocusSidebar`, `FocusAgentPanel`, `FocusEventLog` constants
  - `AppConfig` struct holding `Version` and `ProjectName` strings for the TUI title bar
  - `App` top-level `tea.Model` implementing Elm architecture (`Init`, `Update`, `View`) with value receivers
  - `NewApp` constructor with defaults: focus=Sidebar, ready=false, quitting=false
  - `Update` dispatching `tea.WindowSizeMsg` (store dims, set ready=true) and quit `KeyMsg` (q/ctrl+c/ctrl+q → tea.Quit)
  - `View` with four branches: quitting (empty), not-ready ("Initializing"), too-small warning, full lipgloss title bar layout
  - `RunTUI` public entry point creating `tea.Program` with `WithAltScreen()` and `WithMouseCellMotion()`
  - 44-test suite achieving ≥80% coverage (remaining % is `RunTUI` which requires a live terminal)
- **Files created/modified:**
  - `internal/tui/app.go` -- top-level Elm architecture model, FocusPanel, AppConfig, App, RunTUI
  - `internal/tui/app_test.go` -- 44 tests covering all acceptance criteria and edge cases
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

### T-067: TUI Message Types and Event System

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - `AgentOutputMsg` — single line of agent stdout/stderr tagged with agent name, stream, and timestamp
  - `AgentStatus` iota enum (Idle/Running/Completed/Failed/RateLimited/Waiting) with `String()` method
  - `AgentStatusMsg` — agent lifecycle change signal with status, task ID, and detail
  - `WorkflowEventMsg` — workflow step transition event with step/prevStep/event/detail fields
  - `LoopEventType` iota enum (9 variants) with `String()` method covering all implementation loop events
  - `LoopEventMsg` — loop iteration event with type, taskID, iteration counters, and detail
  - `RateLimitMsg` — rate-limit event with provider, agent, ResetAfter duration, and ResetAt absolute time
  - `TaskProgressMsg` — task state change for progress bar updates with phase/completed/total counters
  - `TickMsg` — periodic timer tick for countdown displays and elapsed time
  - `ErrorMsg` — non-fatal error for display in the event log
  - `FocusChangedMsg` — keyboard focus panel change (uses `FocusPanel` from app.go)
  - `TickCmd` and `TickEvery` helper functions using `tea.Tick` (safe Elm-architecture pattern)
  - 47 unit tests + 4 benchmarks covering construction, String() enum values, type-switch dispatch, and edge cases
- **Files created/modified:**
  - `internal/tui/messages.go` — all TUI message types, enums, and tick helper functions
  - `internal/tui/messages_test.go` — 47 tests + 4 benchmarks covering all acceptance criteria
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

### T-068: Lipgloss Styles and Theme System

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - 11 `lipgloss.AdaptiveColor` package-level vars (`ColorPrimary`, `ColorSecondary`, `ColorAccent`, `ColorSuccess`, `ColorWarning`, `ColorError`, `ColorInfo`, `ColorMuted`, `ColorSubtle`, `ColorBorder`, `ColorHighlight`) with distinct Light/Dark hex values
  - `Theme` struct with 37 `lipgloss.Style` fields organized by component: title bar (4), sidebar (5), agent panel (5), event log (3), status bar (4), progress bars (4), status indicators (5), general (4), dividers (2)
  - `DefaultTheme()` constructor building all 37 styles using `lipgloss.AdaptiveColor` — no Width or Height set on any style
  - `Theme.StatusIndicator(AgentStatus)` returning Unicode symbol strings: `●` (running), `○` (idle), `✓` (completed), `!` (failed), `×` (rate-limited), `◌` (waiting)
  - `Theme.ProgressBar(filled float64, width int)` rendering U+2588/U+2591 block-character bars with per-segment styling, clamped fill, and width guard
  - 9 test functions (47+ assertions) covering: all 11 color vars non-empty, all 37 theme fields render non-empty, idempotent DefaultTheme, all 6 StatusIndicator variants non-empty with correct symbols, distinct symbols across statuses, ProgressBar width guard/clamping/accuracy
  - `stripANSI` test helper for inspecting raw symbol content independent of terminal color codes
- **Files created/modified:**
  - `internal/tui/styles.go` — color palette, Theme struct, DefaultTheme(), StatusIndicator(), ProgressBar()
  - `internal/tui/styles_test.go` — comprehensive test suite covering all acceptance criteria
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

### T-069: Split-Pane Layout Manager

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - 6 exported constants: `MinTerminalWidth=80`, `MinTerminalHeight=24`, `DefaultSidebarWidth=22`, `TitleBarHeight=1`, `StatusBarHeight=1`, `BorderWidth=1`
  - `PanelDimensions` struct holding `Width` and `Height` in terminal cell units
  - `Layout` struct with unexported `termWidth`, `termHeight`, `sidebarWidth`, `agentSplit` fields and 5 exported `PanelDimensions` fields (`TitleBar`, `Sidebar`, `AgentPanel`, `EventLog`, `StatusBar`)
  - `NewLayout()` constructor initializing `sidebarWidth=22` and `agentSplit=0.65` with all panel dimensions zero-initialised
  - `Resize(width, height int) bool` method recording terminal dimensions and recalculating all panel dimensions with clamping (`contentHeight`, `mainWidth`, `agentHeight`, `eventHeight` all min-1); returns false without updating panels when below minimum dimensions
  - `IsTooSmall() bool` method checking recorded terminal dimensions against minimums
  - `TerminalSize() (int, int)` returning last recorded terminal width and height
  - `Render(theme Theme, titleBar, sidebar, agentPanel, eventLog, statusBar string) string` assembling the 5-panel frame using lipgloss styles for sizing, a `"|"` vertical divider with `ColorBorder` foreground, `JoinVertical`/`JoinHorizontal` composition
  - `RenderTooSmall(theme Theme) string` rendering a centered resize message via `lipgloss.Place` when terminal size is known, or plain `theme.ErrorText` when not
  - 25-test suite covering defaults, resize success/failure/clamping, IsTooSmall, TerminalSize, Render, RenderTooSmall
- **Files created/modified:**
  - `internal/tui/layout.go` -- Layout type, constants, PanelDimensions, all 6 exported methods
  - `internal/tui/layout_test.go` -- 25 table-driven tests covering all acceptance criteria and edge cases
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

### T-070: Sidebar -- Workflow List with Status Indicators

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - `WorkflowStatus` iota enum (Idle/Running/Paused/Completed/Failed) with `String()` and `workflowStatusFromEvent()` helpers
  - `WorkflowEntry` struct holding ID, Name, Status, StartedAt, and Detail fields
  - `SidebarModel` Bubble Tea sub-model with workflow list, selectedIdx, scrollOffset, workflowIndex map for O(1) dedup
  - `NewSidebarModel`, `SetDimensions`, `SetFocused`, `SelectedWorkflow` API methods
  - `Update` handling `WorkflowEventMsg` (add/update workflows), `FocusChangedMsg` (focus tracking), `tea.KeyMsg` (j/k/up/down navigation when focused)
  - `workflowListView` rendering "WORKFLOWS" header, status indicators (●○◌✓✗), truncated names with ellipsis
  - Scrolling support: `adjustScroll` keeps selected row visible; `clampIdx` keeps selection in bounds
  - `View` composing workflow list + AGENTS and PROGRESS placeholder sections, padding to full height, applying `SidebarContainer` style with border-aware width calculation
  - Fixed border width accounting: `Width(m.width - 1)` so right border (`│`) doesn't push total beyond m.width
  - 40+ unit and integration tests achieving 93.2% statement coverage
- **Files created/modified:**
  - `internal/tui/sidebar.go` -- SidebarModel, WorkflowStatus, WorkflowEntry, all rendering and navigation logic
  - `internal/tui/sidebar_test.go` -- comprehensive test suite covering all acceptance criteria and edge cases
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

### T-071: Sidebar -- Task Progress Bars and Phase Progress

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - `TaskProgressSection` value-type struct tracking overall task completion (`totalTasks`, `completedTasks`) and per-phase progress (`currentPhase`, `totalPhases`, `phaseTasks`, `phaseCompleted`)
  - `NewTaskProgressSection(theme Theme)` constructor with zero-initialised counters
  - `SetTotals(totalTasks, totalPhases int)` pointer-receiver mutator with negative-value guards
  - `SetPhase(phase, phaseTasks, phaseCompleted int)` pointer-receiver mutator with negative-value guards
  - `Update(msg tea.Msg) TaskProgressSection` value-receiver returning updated copy; handles `TaskProgressMsg` (sets `completedTasks`/`totalTasks`, clamps completed to total, guards negatives) and `LoopEventMsg` (`LoopPhaseComplete` increments `currentPhase` + resets `phaseCompleted`; `LoopTaskCompleted` increments `phaseCompleted` and `completedTasks` when below total)
  - `View(width int) string` rendering two sub-sections: "Tasks" (header + progress bar + percentage + "N/M done" label, or "No tasks" placeholder when total=0) and "Phase: N/M" (header + bar + percentage, or "No phases" placeholder when totalPhases=0); bar width calculated as `width - 2` with a floor of 1; uses `Theme.ProgressBar`, `Theme.ProgressPercent`, `Theme.ProgressLabel`, `Theme.SidebarTitle`, `Theme.SidebarItem`
  - Division-by-zero guard: progress bar only rendered when total > 0
  - Clamping: `completedTasks` and `phaseCompleted` clamped to their respective totals in `View`
  - Integrated into `SidebarModel`: added `taskProgress TaskProgressSection` field, initialised in `NewSidebarModel`, `Update` delegates `TaskProgressMsg` and `LoopEventMsg` to `m.taskProgress.Update(msg)`, `View()` replaces the `"(task progress)"` placeholder with `m.taskProgress.View(m.width)`
  - `SidebarModel.SetTotals` and `SidebarModel.SetPhase` public delegating methods
  - 25 new unit and integration tests covering all edge cases: zero totals, negative values, clamping, LoopPhaseComplete sequences, LoopTaskCompleted bounded increment, View placeholders, View with real data, delegation from SidebarModel, SidebarModel View rendering
- **Files created/modified:**
  - `internal/tui/sidebar.go` -- `TaskProgressSection` type with all methods; `SidebarModel` integration (field, init, Update cases, View replacement, SetTotals/SetPhase delegates); added `"fmt"` import
  - `internal/tui/sidebar_test.go` -- 25 new table-driven and unit tests for `TaskProgressSection` and `SidebarModel` integration
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

### T-072: Sidebar -- Rate-Limit Status Display with Countdown

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - `ProviderRateLimit` value struct tracking per-provider rate-limit state: `Provider`, `Agent`, `ResetAt`, `Remaining`, `Active`
  - `RateLimitSection` value-type struct with `theme Theme`, `providers map[string]*ProviderRateLimit`, and `order []string` (stable insertion-order rendering)
  - `NewRateLimitSection(theme Theme) RateLimitSection` constructor with empty provider map
  - `Update(msg tea.Msg) (RateLimitSection, tea.Cmd)` value-receiver handling:
    - `RateLimitMsg`: calls `applyRateLimitMsg` to register/update provider, always returns `TickCmd(time.Second)`
    - `TickMsg`: calls `tick()` to recalculate `Remaining = time.Until(ResetAt)` for each active provider; deactivates expired ones; returns `TickCmd(time.Second)` if any still active, nil otherwise
  - `applyRateLimitMsg`: copies providers map + order slice for value-receiver immutability; derives `ResetAt` from `msg.ResetAt` (if non-zero) or `msg.Timestamp + msg.ResetAfter`; uses `Provider` as key, falling back to `Agent`
  - `tick()`: copies providers map; recalculates `Remaining` via `time.Until`; zeroes and deactivates expired providers
  - `HasActiveLimit() bool`: returns true if any provider has `Active == true`
  - `View(width int) string`: renders "Rate Limits" header + per-provider lines (`{name}: OK` in green for inactive, `{name}: WAIT M:SS` in yellow for active); "No limits" placeholder when no providers; name truncated to fit width with room for the suffix
  - `formatCountdown(d time.Duration) string`: "0:00" for non-positive; "M:SS" for under 1 hour; "H:MM:SS" for 1 hour or more
  - Integrated `rateLimits RateLimitSection` field into `SidebarModel`: initialised in `NewSidebarModel`, `Update` handles `RateLimitMsg` and `TickMsg` by delegating and propagating the returned `tea.Cmd`, `View()` renders the rate limits section between AGENTS placeholder and PROGRESS section
  - 30+ unit and integration tests covering: formatCountdown edge cases, RateLimitSection constructor, RateLimitMsg handling (add/update/multi-provider/stable-order/ResetAfter fallback/agent-key fallback), TickMsg handling (nil cmd when no limits, TickCmd when active, deactivation on expiry, mixed provider states), View rendering (header, placeholder, OK/WAIT states, countdown format, stable order, zero-width no-panic), HasActiveLimit, SidebarModel integration (RateLimitMsg cmd propagation, TickMsg propagation, sidebar view showing rate limits)
- **Files created/modified:**
  - `internal/tui/sidebar.go` -- `ProviderRateLimit`, `RateLimitSection` types and all methods; `SidebarModel` integration (field, init, Update cases, View section); `formatCountdown` helper
  - `internal/tui/sidebar_test.go` -- 30+ new table-driven and integration tests for T-072
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

### T-073: Agent Output Panel with Viewport Scrolling and Tabbed Multi-Agent View

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - `OutputBuffer` ring buffer capped at 1000 lines per agent with O(1) append and eviction
  - `AgentView` per-agent display state with `viewport.Model` for scrollable output and `autoScroll` tracking
  - `AgentPanelModel` top-level sub-model with tab management, focus handling, and Elm-style Update/View
  - Tab bar rendered only when 2+ agents are active, with active tab visually distinguished
  - Agent header showing status indicator (●/✓/!/×/◌/○), agent name, and current task ID
  - Full keyboard navigation: j/k, up/down, PgUp/PgDn, Home/End for scrolling; Tab/Shift-Tab for agent tab switching
  - Auto-scroll to bottom on new output; stops on manual scroll up; resumes when scrolling back to bottom
  - Tab key passthrough to parent when only one agent is present (enables focus cycling)
  - 31 unit and integration tests covering all acceptance criteria + edge cases (85%+ coverage)
- **Files created/modified:**
  - `internal/tui/agent_panel.go` -- `OutputBuffer`, `AgentView`, `AgentPanelModel` with full Elm architecture
  - `internal/tui/agent_panel_test.go` -- 31 tests covering ring buffer, tab switching, auto-scroll, edge cases, integration
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

### T-074: Event Log Panel for Workflow Milestones

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - `EventCategory` enum (Info, Success, Warning, Error, Debug) for styled display
  - `EventEntry` struct with timestamp, category, and message
  - `EventLogModel` sub-model with bounded 500-entry buffer, viewport scrolling, and visibility toggle
  - `classifyWorkflowEvent`, `classifyLoopEvent`, `classifyAgentStatus` helper functions mapping all message types to human-readable entries
  - `RateLimitMsg` formatted as "Rate limit: {provider}, waiting M:SS" using shared `formatCountdown`
  - `ErrorMsg` handling with fallback from Detail to Source field
  - Auto-scroll to newest entry; disabled on k/up/pgup scroll; re-enabled on G/End
  - `l` key toggles panel visibility; hidden panel returns `""` from `View()`
  - Header "Event Log" always rendered when visible; "No events yet" placeholder for empty log
  - Focused state highlighted with `ColorPrimary` border
  - 40+ unit and integration tests achieving 90%+ coverage
- **Files created/modified:**
  - `internal/tui/event_log.go` -- `EventLogModel`, `EventEntry`, `EventCategory`, classifier functions
  - `internal/tui/event_log_test.go` -- 40+ tests covering all acceptance criteria, edge cases, and integration scenarios
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

### T-075: Status Bar with Current State, Iteration, and Timer

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - `StatusBarModel` sub-model tracking phase, task, iteration, elapsed time, paused state, workflow name, and mode
  - `NewStatusBarModel(theme Theme) StatusBarModel` initializes with mode="idle" and zero dynamic state
  - `Update(msg tea.Msg) StatusBarModel` handles `LoopEventMsg`, `WorkflowEventMsg`, and `TickMsg`
  - `View() string` renders single-line status bar with left-aligned segments and right-aligned "? help" hint
  - Two-pass segment layout: mandatory (mode, task) always shown; optional (phase, iter, timer) dropped when too narrow
  - `formatElapsed(d time.Duration) string` returns "HH:MM:SS" format
  - Prominent "PAUSED" badge (amber background) when `LoopWaitingForRateLimit` fires; cleared on `LoopResumedAfterWait`
  - Elapsed timer uses `tickMsg.Time.Sub(startTime)` for deterministic test behavior
  - Fixed lipgloss border-box Width behavior: uses `Width(sb.width)` (total) not `Width(innerWidth)` (content)
  - 45 unit/integration tests + 2 benchmarks covering all acceptance criteria and edge cases
- **Files created/modified:**
  - `internal/tui/status_bar.go` -- `StatusBarModel` with full Elm architecture, segment helpers, `formatElapsed`
  - `internal/tui/status_bar_test.go` -- 45+ tests for all acceptance criteria, edge cases, and integration lifecycle
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

### T-076: Keyboard Navigation and Help Overlay

- **Status:** Completed
- **Date:** 2026-02-19
- **What was built:**
  - `KeyMap` struct with all 15 keybindings using `charmbracelet/bubbles/key` (Quit, Help, Pause, Skip, ToggleLog, FocusNext, FocusPrev, Up, Down, PageUp, PageDown, Home, End, NextAgent, PrevAgent)
  - `DefaultKeyMap()` function populating all bindings with correct Bubble Tea key names ("tab", "shift+tab", "pgup", "pgdown", "ctrl+c", etc.)
  - `NextFocus(FocusPanel) FocusPanel` and `PrevFocus(FocusPanel) FocusPanel` modular arithmetic focus cycling over 3 panels
  - `PauseRequestMsg{}` and `SkipRequestMsg{}` control message types for workflow integration
  - `HelpOverlay` struct with `NewHelpOverlay`, `SetDimensions`, `Toggle`, `IsVisible`, `Update`, `View` methods
  - `HelpOverlay.View()` renders a centered, rounded-border box with three keybinding categories (Navigation, Actions, Scrolling) using `lipgloss.Place`
  - `HelpOverlay.Update()` handles `?` and `Esc` to dismiss; all other keys are consumed without action
  - `App` struct updated with `keyMap KeyMap` and `helpOverlay HelpOverlay` fields
  - `App.Update()` fully overhauled: help overlay visible path delegates to overlay; `key.Matches` dispatch for all global keys; `FocusChangedMsg` sent on Tab/Shift+Tab; `PauseRequestMsg`/`SkipRequestMsg` returned as commands; scrolling keys consumed
  - `App.View()` renders help overlay on top when visible via `helpOverlay.View()`
  - `App.Init()` unchanged (returns nil)
  - 40+ unit and integration tests covering all acceptance criteria
- **Files created/modified:**
  - `internal/tui/keybindings.go` -- `KeyMap`, `DefaultKeyMap`, `NextFocus`, `PrevFocus`, `PauseRequestMsg`, `SkipRequestMsg`, `HelpOverlay` (all methods)
  - `internal/tui/keybindings_test.go` -- 40+ tests for keybindings, focus cycling, overlay behavior, and App integration
  - `internal/tui/app.go` -- Added `keyMap`/`helpOverlay` fields, integrated in `NewApp`, `Update`, `View`
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

### T-077: Pipeline Wizard TUI Integration

- **Status:** Completed
- **Date:** 2026-02-19
- **What was built:**
  - `PipelineWizardConfig` struct holding all pipeline parameters: PhaseMode (single/range/all), PhaseID, FromPhase, ToPhase, ImplAgent, ReviewAgents ([]string), FixAgent, ReviewConcurrency, MaxReviewCycles, MaxIterations, and four skip flags (SkipImplement, SkipReview, SkipFix, SkipPR)
  - `WizardCompleteMsg` and `WizardCancelledMsg` Bubble Tea message types for lifecycle signalling
  - `WizardModel` Bubble Tea sub-model with `theme`, `form *huh.Form`, `width/height`, `active`, `config`, `availableAgents`, `availablePhases`, and raw string fields for numeric huh.Input values
  - `NewWizardModel(theme, agents, phases)` constructor with sensible defaults: PhaseMode="all", ReviewConcurrency=2, MaxReviewCycles=3, MaxIterations=10
  - `SetDimensions(width, height)` pointer-receiver method updating form width on resize
  - `IsActive() bool` value-receiver predicate
  - `Start() tea.Cmd` building the form and returning `form.Init()`
  - `Update(msg) (WizardModel, tea.Cmd)` forwarding to huh, handling Esc for cancellation, emitting WizardCompleteMsg/WizardCancelledMsg on state transitions
  - `View() string` rendering the form wrapped in a rounded-border lipgloss container, centered with `lipgloss.Place` when dimensions are known
  - 5-group huh form: (1) Phase selection with mode select + 3 Input fields for IDs; (2) Agent selection with impl/review(multi)/fix selects — fallback Note when no agents; (3) Settings with 3 validated Input fields; (4) Skip flags with 4 Confirm toggles; (5) Confirmation Note with live `DescriptionFunc` summary
  - `buildHuhTheme(theme Theme) *huh.Theme` translating Raven color palette into a custom `huh.ThemeBase()` derivative
  - `positiveIntValidator(fieldName)` and `capitalizeFirst(s)` helper functions
  - `parseFormValues()` converting raw string inputs into typed config fields on completion
  - Edge cases: empty agents/phases show informational Note fields; single agent pre-selected; form width capped at 100
  - 25+ unit tests covering all acceptance criteria, defaults, validators, and edge cases
- **Files created/modified:**
  - `internal/tui/wizard.go` -- `PipelineWizardConfig`, `WizardCompleteMsg`, `WizardCancelledMsg`, `WizardModel` with all required methods and 5-group huh form construction
  - `internal/tui/wizard_test.go` -- 25+ tests for constructor, IsActive, SetDimensions, buildHuhTheme, buildForm, config defaults, parseFormValues, validators, and messages
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

### T-078: Raven Dashboard Command and TUI Integration Testing

- **Status:** Completed
- **Date:** 2026-02-19
- **What was built:**
  - `NewDashboardCmd() *cobra.Command` Cobra subcommand for `raven dashboard` registered in the root command; respects the global `--dry-run` flag and prints a dry-run message without launching the TUI
  - `dashboardRun` RunE function that constructs `tui.AppConfig` from `buildinfo.Version` and calls `tui.RunTUI`
  - `EventBridge` struct in `internal/tui/bridge.go` with static Cmd-based helpers converting backend events to TUI messages: `WorkflowEventCmd`, `LoopEventCmd`, `AgentOutputCmd`, `TaskProgressCmd`; plus goroutine-safe `SendWorkflowEvent`, `SendLoopEvent`, `SendAgentOutput` push functions; `mapLoopEventType` converting `loop.LoopEventType` (string) to TUI `LoopEventType` (int iota); rate-limit events converted to `RateLimitMsg` directly
  - `App` struct fully integrated with all sub-models: `sidebar SidebarModel`, `agentPanel AgentPanelModel`, `eventLog EventLogModel`, `statusBar StatusBarModel`, `wizard WizardModel`, `theme Theme`, `layout Layout`
  - `NewApp` initialises all sub-models with `DefaultTheme()` and sets sidebar as initially focused
  - `App.Update` routes every TUI message to appropriate sub-models, forwards keyboard events to the focused panel via `forwardKeyToFocused`, and batches all sub-model `tea.Cmd` returns with `tea.Batch()`
  - `App.View` renders wizard when active, help overlay when visible, or the full layout using `layout` panel dimensions; `fullView` composes sidebar, agent panel, event log, and status bar views
  - Dashboard command registered in root command's `init()` function
  - 11 unit tests for `NewDashboardCmd` (metadata, flags, dry-run, help), 10+ tests for `EventBridge` (type conversions, loop event mapping, closed channels, cancelled context)
- **Files created/modified:**
  - `internal/cli/dashboard.go` -- `NewDashboardCmd`, `dashboardRun`
  - `internal/cli/dashboard_test.go` -- unit tests for dashboard command
  - `internal/tui/bridge.go` -- `EventBridge` with all bridge methods and `mapLoopEventType`
  - `internal/tui/bridge_test.go` -- unit tests for EventBridge conversions
  - `internal/tui/app.go` -- fully integrated App struct with all sub-models replacing commented-out placeholders
  - `internal/tui/app_test.go` -- updated 3 tests to reflect real integrated view (title-bar-only checks)
  - `internal/cli/root.go` -- registered `NewDashboardCmd()` in `init()`
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

## In Progress Tasks

_None currently_

---

## Not Started Tasks

### Phase 6: TUI Command Center (T-066 to T-078, T-089)

- **Status:** In Progress
- **Tasks:** 14 (13 Must Have, 1 Should Have)
- **Estimated Effort:** 96-148 hours
- **PRD Roadmap:** Weeks 11-13

#### Task List

| Task | Name | Priority | Effort | Status |
|------|------|----------|--------|--------|
| T-089 | Stream-JSON Integration -- Wire into Adapters & Loop | Must Have | Medium (8-12hrs) | Completed |
| T-066 | Bubble Tea Application Scaffold and Elm Architecture | Must Have | Medium (8-12hrs) | Completed |
| T-067 | TUI Message Types and Event System | Must Have | Medium (6-10hrs) | Completed |
| T-068 | Lipgloss Styles and Theme System | Must Have | Medium (6-8hrs) | Completed |
| T-069 | Split-Pane Layout Manager | Must Have | Medium (8-12hrs) | Completed |
| T-070 | Sidebar -- Workflow List with Status Indicators | Must Have | Medium (6-8hrs) | Completed |
| T-071 | Sidebar -- Task Progress Bars and Phase Progress | Must Have | Medium (6-8hrs) | Completed |
| T-072 | Sidebar -- Rate-Limit Status Display with Countdown | Must Have | Medium (6-8hrs) | Completed |
| T-073 | Agent Output Panel with Viewport and Tabbed View | Must Have | Large (16-24hrs) | Completed |
| T-074 | Event Log Panel for Workflow Milestones | Must Have | Medium (6-10hrs) | Completed |
| T-075 | Status Bar with Current State, Iteration, and Timer | Must Have | Small (4-6hrs) | Completed |
| T-076 | Keyboard Navigation and Help Overlay | Must Have | Medium (8-12hrs) | Completed |
| T-077 | Pipeline Wizard TUI Integration (huh) | Should Have | Medium (8-12hrs) | Completed |
| T-078 | Raven Dashboard Command and TUI Integration Testing | Must Have | Large (16-24hrs) | Completed |

**Deliverable:** `raven dashboard` launches a beautiful, responsive command center with live agent monitoring.

---

### Phase 7: Polish & Distribution (T-079 to T-087)

- **Status:** Not Started
- **Tasks:** 9 (8 Must Have, 1 Should Have)
- **Estimated Effort:** 64-96 hours
- **PRD Roadmap:** Week 14

#### Task List

| Task | Name | Priority | Effort | Status |
|------|------|----------|--------|--------|
| T-079 | GoReleaser Configuration for Cross-Platform Builds | Must Have | Medium (6-10hrs) | Not Started |
| T-080 | GitHub Actions Release Automation Workflow | Must Have | Medium (6-10hrs) | Not Started |
| T-081 | Shell Completion Installation Scripts and Packaging | Should Have | Small (3-4hrs) | Not Started |
| T-082 | Man Page Generation Using cobra/doc | Should Have | Small (2-4hrs) | Not Started |
| T-083 | Performance Benchmarking Suite | Should Have | Medium (8-12hrs) | Not Started |
| T-084 | End-to-End Integration Test Suite with Mock Agents | Must Have | Large (20-30hrs) | Not Started |
| T-085 | CI/CD Pipeline with GitHub Actions | Must Have | Medium (6-10hrs) | Not Started |
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
