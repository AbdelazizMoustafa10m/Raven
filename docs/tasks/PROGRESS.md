# Raven Task Progress Log

## Summary

| Status | Count |
|--------|-------|
| Completed | 52 |
| In Progress | 0 |
| Not Started | 37 |

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
- **Tasks Completed:** 12 tasks (T-031 to T-042, including T-058)

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

### T-043: Workflow Event Types and Constants

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - Six transition event constants (`EventSuccess`, `EventFailure`, `EventBlocked`, `EventRateLimited`, `EventNeedsHuman`, `EventPartial`) in `internal/workflow/events.go`
  - Two terminal pseudo-step constants (`StepDone = "__done__"`, `StepFailed = "__failed__"`) with `__` prefix to prevent collisions
  - Nine `WE*` lifecycle constants for TUI/logging consumption (`WEStepStarted`, `WEStepCompleted`, `WEStepFailed`, `WEWorkflowStarted`, `WEWorkflowCompleted`, `WEWorkflowFailed`, `WEWorkflowResumed`, `WEStepSkipped`, `WECheckpoint`)
  - `StepHandler` interface with `Execute(ctx, state) (string, error)`, `DryRun(state) string`, and `Name() string`
  - `WorkflowEvent` struct (JSON-serializable, `error` field uses `omitempty`)
  - `WorkflowDefinition` and `StepDefinition` types with JSON + TOML struct tags
  - 25 table-driven unit tests covering all constants, JSON round-trips, omitempty behavior, and interface satisfaction
- **Files created/modified:**
  - `internal/workflow/events.go` -- all workflow event constants, interface, and data types with full godoc
  - `internal/workflow/events_test.go` -- 478-line comprehensive test suite
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

### T-044: Step Handler Registry

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - `Registry` struct with internal `map[string]StepHandler` (no mutex -- single-threaded init-time registration)
  - `NewRegistry() *Registry` constructor
  - `Register(handler StepHandler)` -- panics on nil handler, empty name, or duplicate name with clear messages
  - `Get(name string) (StepHandler, error)` -- returns `ErrStepNotFound` wrapped with step name for `errors.Is` detection
  - `Has(name string) bool` -- O(1) map presence check
  - `List() []string` -- alphabetically sorted handler names via `sort.Strings`
  - `MustGet(name string) StepHandler` -- panics on missing handler, for use in init code
  - `ErrStepNotFound` sentinel error (`errors.New`)
  - `DefaultRegistry` package-level singleton and four delegation functions: `Register`, `GetHandler`, `HasHandler`, `ListHandlers`
  - 20 table-driven unit tests covering all methods, all panic paths, sentinel unwrapping via `errors.Is`, and DefaultRegistry delegation with hermetic restore pattern
- **Files created/modified:**
  - `internal/workflow/registry.go` -- full implementation with godoc
  - `internal/workflow/registry_test.go` -- 275-line comprehensive test suite
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

### T-045: Workflow Engine Core -- State Machine Runner

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - `Engine` struct with functional options (`WithDryRun`, `WithSingleStep`, `WithEventChannel`, `WithLogger`, `WithMaxIterations`)
  - `Run()` method: full state machine loop with context cancellation, step resolution, event emission, StepRecord tracking, terminal step detection, and max-iterations guard (default 1000)
  - `RunStep()` method: single-step isolation via sub-Engine with `WithSingleStep`
  - `Validate()` method: checks handler registration, transition target validity, and initial step existence
  - `safeExecute()`: `recover()`-wrapped Execute() converting panics to descriptive errors
  - `emit()`: nil-safe, non-blocking event channel send
  - Workflow events emitted: `WEWorkflowStarted`/`WEWorkflowResumed`, `WEStepStarted`, `WEStepCompleted`/`WEStepFailed`/`WEStepSkipped`, `WEWorkflowCompleted`/`WEWorkflowFailed`
  - 48 unit tests + 3 benchmarks achieving 98.6% statement coverage
- **Files created/modified:**
  - `internal/workflow/engine.go` -- full Engine implementation with godoc
  - `internal/workflow/engine_test.go` -- comprehensive test suite (48 tests + 3 benchmarks)
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

### T-046: Workflow State Checkpointing and Persistence

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - `StateStore` struct with `NewStateStore`, `Save`, `Load`, `List`, `Delete`, `LatestRun` methods
  - Atomic file writes: marshal to `<id>.json.tmp`, fsync, rename to `<id>.json` (crash-safe)
  - `RunSummary` struct for lightweight run listing (`raven resume --list`)
  - `StatusFromState` function deriving "completed", "failed", "running", or "interrupted" from `WorkflowState`
  - `WithCheckpointing(store)` `EngineOption` that auto-saves state after each step (hook fires after `CurrentStep` is advanced)
  - `sanitizeID` helper replacing non-`[a-zA-Z0-9_-]` chars with `_` for filesystem safety
  - 30+ unit tests covering all 12 acceptance criteria, including concurrent saves, corrupt-file skipping, large metadata, and race-detector validation
- **Files created/modified:**
  - `internal/workflow/state.go` -- added `StateStore`, `RunSummary`, `StatusFromState`, `WithCheckpointing`, `sanitizeID`
  - `internal/workflow/engine.go` -- added `postStepHook` field; hook called after `CurrentStep` advance
  - `internal/workflow/state_test.go` -- comprehensive test suite (30+ tests + benchmarks)
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

### T-047: Resume Command -- List and Resume Interrupted Workflows

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - `newResumeCmd()` returning `*cobra.Command` with `--run`, `--list`, `--dry-run`, `--clean`, `--clean-all`, `--force` flags
  - `runResume()` entry point branching to list, clean-all, clean, or resume modes
  - `runListMode()` -- tabwriter-formatted table (RUN ID, WORKFLOW, STEP, STATUS, LAST UPDATED, STEPS) written to stdout
  - `runCleanMode()` -- deletes a single checkpoint by run ID
  - `runCleanAllMode()` -- deletes all checkpoints; requires `--force` in non-interactive mode, prompts otherwise
  - `runResumeMode()` -- loads checkpoint (latest or by ID), checks dry-run first (bypasses resolver), then resolves definition and drives `workflow.Engine`
  - `resolveDefinition()` -- stub returning `ErrWorkflowNotFound` with a T-049 reference; acts as the extension point for T-049
  - `definitionResolver` function type for dependency injection in tests
  - `formatRunTable()` -- text/tabwriter table writer (avoids lipgloss import-cycle)
  - `isTerminal()` -- pure stdlib TTY detection via `os.ModeCharDevice`
  - `runIDPattern` regex -- prevents path traversal via `--run` and `--clean` flags
  - `ErrWorkflowNotFound` sentinel error
  - 35+ unit tests covering: command structure, flag registration/defaults, run ID validation, all four operation modes, formatRunTable output, isTerminal, resolver stub, ordering, corrupt-file skip
- **Files created/modified:**
  - `internal/cli/resume.go` -- full implementation (344 lines)
  - `internal/cli/resume_test.go` -- comprehensive test suite (35+ tests)
  - `docs/tasks/PROGRESS.md` -- this entry
- **Key Decisions:**
  1. **Dry-run before resolver** -- The dry-run check runs before calling `resolveDefinition`, so `--dry-run` works even before T-049 is implemented
  2. **`definitionResolver` function type** -- Dependency injection enables clean test mocking without modifying the package-level `resolveDefinition` stub
  3. **text/tabwriter over lipgloss** -- Avoids import-cycle issues when the TUI package also imports `internal/cli`
  4. **`--force` required in non-interactive mode** -- `--clean-all` without `--force` on a pipe/non-TTY stdin returns an error rather than silently destroying checkpoints
  5. **Security: `runIDPattern` allowlist** -- Only `[a-zA-Z0-9_-]` permitted in `--run` and `--clean` values to prevent path traversal to arbitrary JSON files
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

### T-048: Workflow Definition Validation

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - Nine `Issue*` string constants (`IssueNoSteps`, `IssueMissingInitial`, `IssueMissingHandler`, `IssueInvalidTarget`, `IssueUnreachableStep`, `IssueCycleDetected`, `IssueNoTransitions`, `IssueDuplicateStep`, `IssueEmptyStepName`) -- stable string values for switch-based caller handling
  - `ValidationIssue` struct: `Code`, `Step` (empty for definition-level issues), `Message`
  - `ValidationResult` struct with `Errors` (fatal) and `Warnings` (non-fatal) slices, `IsValid() bool`, `String() string` methods
  - `ValidateDefinition(def, registry) *ValidationResult` -- 6-phase validation: (1) empty steps / empty names / duplicates / missing initial step; (2) invalid transition targets; (3) handler checks against registry (nil registry skips this phase); (4) BFS reachability from `InitialStep`; (5) three-color DFS cycle detection with cycle-path capture in warning messages; (6) no-transitions warning for non-terminal steps
  - `ValidateDefinitions(defs map[string]*WorkflowDefinition, registry) map[string]*ValidationResult` -- batch validation over a map
  - Cycles are warnings (not errors): intentional review-fix loops are valid by design
  - Terminal pseudo-steps (`StepDone`/`StepFailed`) are always valid transition targets; they are excluded from adjacency graph to avoid false positives
  - Diamond-shaped (convergent) graphs correctly identified as non-cyclic by the three-color DFS
  - 40+ unit tests covering all issue codes, graph analysis edge cases (diamond, linear chain, cycle, orphan), nil definition, empty registry, batch validation
- **Files created/modified:**
  - `internal/workflow/validate.go` -- full implementation with godoc
  - `internal/workflow/validate_test.go` -- comprehensive test suite (40+ tests)
- **Key Decisions:**
  1. **Cycles as warnings** -- `review → fix → review` loops are intentional; callers decide whether to reject a workflow with cycles; `IsValid()` is true when there are zero errors
  2. **Six-phase sequential validation** -- Each phase builds on the previous; graph analysis (phases 4-6) is gated on a valid `InitialStep` so BFS/DFS do not panic on empty inputs
  3. **Terminal pseudo-steps excluded from adjacency** -- `StepDone` and `StepFailed` have no outgoing transitions; including them in adjacency would incorrectly suppress cross-branch reachability warnings
  4. **`cyclesReported` deduplication map** -- Prevents duplicate `CYCLE_DETECTED` warnings when the DFS discovers the same back-edge through different entry points in disconnected sub-graphs
  5. **`String()` omits `step ""` for definition-level issues** -- Issues without a `Step` field render as `[CODE] message` to avoid confusing empty-string quotes in CLI output
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

### T-049: Built-in Workflow Definitions and Step Handlers

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - Four built-in workflow name constants (`WorkflowImplement`, `WorkflowImplementReview`, `WorkflowPipeline`, `WorkflowPRDDecompose`)
  - `BuiltinDefinitions()` returning all four workflow definitions as a shallow-copy map
  - `GetDefinition(name string)` returning a workflow definition by name (nil if not found)
  - `RegisterBuiltinHandlers(registry)` registering all 11 built-in step handlers
  - `ImplementHandler` wrapping the loop runner for single-task or phase-based implementation
  - `ReviewHandler` wrapping the multi-agent review orchestrator
  - `CheckReviewHandler` mapping review verdict to `EventSuccess` (approved) or `EventNeedsHuman` (changes needed/blocking)
  - `FixHandler` wrapping the review fix engine
  - `PRHandler` wrapping PR creation via `review.PRCreator`
  - `InitPhaseHandler`, `RunPhaseWorkflowHandler`, `AdvancePhaseHandler` for multi-phase pipeline iteration
  - `ShredHandler`, `ScatterHandler`, `GatherHandler` as stubs for future PRD decomposition (T-056+)
  - Metadata helpers: `metaString`, `metaInt`, `metaBool`, `resolveAgents` for safe cross-step data passing
  - All handlers use compile-time interface checks (`var _ StepHandler = (*Handler)(nil)`)
  - 86.9% test coverage (exceeds 80% requirement)
- **Files created/modified:**
  - `internal/workflow/builtin.go` -- workflow definitions, constants, and registration
  - `internal/workflow/handlers.go` -- all 11 step handler implementations
  - `internal/workflow/builtin_test.go` -- tests for definitions and registry
  - `internal/workflow/handlers_test.go` -- tests for all handlers
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓

---

## In Progress Tasks

_None currently_

---

## Not Started Tasks

### Phase 4: Workflow Engine & Pipeline (T-043 to T-055)

- **Status:** In Progress (6/13 complete)
- **Tasks:** 13 (12 Must Have, 1 Should Have)
- **Estimated Effort:** 96-144 hours
- **PRD Roadmap:** Weeks 7-8

#### Task List

| Task | Name | Priority | Effort | Status |
|------|------|----------|--------|--------|
| T-043 | Workflow Event Types and Constants | Must Have | Small (2-4hrs) | Completed |
| T-044 | Step Handler Registry | Must Have | Small (2-4hrs) | Completed |
| T-045 | Workflow Engine Core -- State Machine Runner | Must Have | Large (14-20hrs) | Completed |
| T-046 | Workflow State Checkpointing and Persistence | Must Have | Medium (6-10hrs) | Completed |
| T-047 | Resume Command -- List and Resume Interrupted Workflows | Must Have | Medium (6-10hrs) | Completed |
| T-048 | Workflow Definition Validation | Must Have | Medium (6-10hrs) | Completed |
| T-049 | Built-in Workflow Definitions and Step Handlers | Must Have | Large (14-20hrs) | Completed |
| T-050 | Pipeline Orchestrator Core -- Multi-Phase Lifecycle | Must Have | Large (14-20hrs) | Not Started |
| T-051 | Pipeline Branch Management | Must Have | Medium (6-10hrs) | Not Started |
| T-052 | Pipeline Metadata Tracking | Must Have | Small (2-4hrs) | Not Started |
| T-053 | Pipeline Interactive Wizard | Should Have | Medium (6-10hrs) | Not Started |
| T-054 | Pipeline and Workflow Dry-Run Mode | Must Have | Medium (6-10hrs) | Not Started |
| T-055 | Pipeline CLI Command | Must Have | Medium (6-10hrs) | Not Started |

**Deliverable:** `raven pipeline --phase all` orchestrates the full lifecycle with checkpoint/resume.

---

### Phase 5: PRD Decomposition (T-056 to T-065)

- **Status:** Not Started
- **Tasks:** 10 (10 Must Have)
- **Estimated Effort:** 70-110 hours
- **PRD Roadmap:** Weeks 9-10

#### Task List

| Task | Name | Priority | Effort | Status |
|------|------|----------|--------|--------|
| T-056 | Epic JSON Schema and Types | Must Have | Small (2-4hrs) | Not Started |
| T-057 | PRD Shredder (Single Agent -> Epic JSON) | Must Have | Medium (8-12hrs) | Not Started |
| T-058 | JSON Extraction Utility | Must Have | Medium (6-10hrs) | Not Started |
| T-059 | Parallel Epic Workers | Must Have | Medium (8-12hrs) | Not Started |
| T-060 | Merge -- Global ID Assignment | Must Have | Medium (6-10hrs) | Not Started |
| T-061 | Merge -- Dependency Remapping | Must Have | Medium (6-10hrs) | Not Started |
| T-062 | Merge -- Title Deduplication | Must Have | Medium (6-10hrs) | Not Started |
| T-063 | Merge -- DAG Validation | Must Have | Medium (6-10hrs) | Not Started |
| T-064 | Task File Emitter | Must Have | Medium (8-12hrs) | Not Started |
| T-065 | PRD CLI Command -- raven prd | Must Have | Medium (8-12hrs) | Not Started |

**Deliverable:** `raven prd --file docs/prd/PRD.md --concurrent` produces a complete task breakdown.

---

### Phase 6: TUI Command Center (T-066 to T-078, T-089)

- **Status:** Not Started
- **Tasks:** 14 (13 Must Have, 1 Should Have)
- **Estimated Effort:** 96-148 hours
- **PRD Roadmap:** Weeks 11-13

#### Task List

| Task | Name | Priority | Effort | Status |
|------|------|----------|--------|--------|
| T-089 | Stream-JSON Integration -- Wire into Adapters & Loop | Must Have | Medium (8-12hrs) | Completed |
| T-066 | Bubble Tea Application Scaffold and Elm Architecture | Must Have | Medium (8-12hrs) | Not Started |
| T-067 | TUI Message Types and Event System | Must Have | Medium (6-10hrs) | Not Started |
| T-068 | Lipgloss Styles and Theme System | Must Have | Medium (6-8hrs) | Not Started |
| T-069 | Split-Pane Layout Manager | Must Have | Medium (8-12hrs) | Not Started |
| T-070 | Sidebar -- Workflow List with Status Indicators | Must Have | Medium (6-8hrs) | Not Started |
| T-071 | Sidebar -- Task Progress Bars and Phase Progress | Must Have | Medium (6-8hrs) | Not Started |
| T-072 | Sidebar -- Rate-Limit Status Display with Countdown | Must Have | Medium (6-8hrs) | Not Started |
| T-073 | Agent Output Panel with Viewport and Tabbed View | Must Have | Large (16-24hrs) | Not Started |
| T-074 | Event Log Panel for Workflow Milestones | Must Have | Medium (6-10hrs) | Not Started |
| T-075 | Status Bar with Current State, Iteration, and Timer | Must Have | Small (4-6hrs) | Not Started |
| T-076 | Keyboard Navigation and Help Overlay | Must Have | Medium (8-12hrs) | Not Started |
| T-077 | Pipeline Wizard TUI Integration (huh) | Should Have | Medium (8-12hrs) | Not Started |
| T-078 | Raven Dashboard Command and TUI Integration Testing | Must Have | Large (16-24hrs) | Not Started |

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
