# Raven Task Progress Log

## Summary

| Status | Count |
|--------|-------|
| Completed | 62 |
| In Progress | 0 |
| Not Started | 27 |

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

### T-056: Epic JSON Schema and Types

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - `EpicBreakdown` and `Epic` structs for Phase 1 (shred) output with full JSON tags
  - `EpicTaskResult` and `TaskDef` structs for Phase 2 (scatter) output with full JSON tags
  - `ValidationError` structured error type for retry prompt augmentation
  - `(*EpicBreakdown).Validate()` — enforces ID format, required fields, duplicate IDs, cross-epic dep integrity
  - `(*EpicTaskResult).Validate(knownEpicIDs)` — enforces temp_id format, effort/priority enums, local/cross-epic dep refs
  - `FormatValidationErrors` — numbered list format suitable for LLM retry prompts
  - `ParseEpicBreakdown` / `ParseEpicTaskResult` — 10 MB size cap + unmarshal + validate helpers
  - Comprehensive test suite with 100% statement coverage; golden tests, fuzz tests, benchmarks
- **Files created/modified:**
  - `internal/prd/schema.go` — all types, validation methods, and parse helpers
  - `internal/prd/schema_test.go` — 80+ tests: table-driven, golden, fuzz, benchmarks
  - `internal/prd/testdata/valid_epic_breakdown.json` — 3-epic valid fixture
  - `internal/prd/testdata/valid_epic_task_result.json` — 3-task valid fixture for E-001
  - `internal/prd/testdata/invalid_epic_breakdown.json` — invalid fixture with multiple failure modes
  - `internal/prd/testdata/invalid_epic_task_result.json` — invalid task result fixture
  - `internal/prd/testdata/expected-output/epic_breakdown_golden.json` — golden snapshot
  - `internal/prd/testdata/expected-output/epic_task_result_golden.json` — golden snapshot
- **Verification:** `go build` ✓  `go vet` ✓  `go test` ✓  coverage: 100%

---

### T-057: PRD Shredder (Single Agent -> Epic JSON)

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - `Shredder` struct orchestrating a single-agent PRD-to-epics call
  - `ShredOpts` struct for per-call parameters (PRDPath, OutputFile, Model, Effort)
  - `ShredResult` struct with EpicBreakdown, Duration, Retries, OutputFile
  - `ShredderOption` functional option type with `WithMaxRetries`, `WithLogger`, `WithEvents`
  - `ShredEvent` / `ShredEventType` for progress tracking (started, completed, retry, failed)
  - Embedded prompt template using `[[`/`]]` delimiters; includes schema example and retry error injection
  - Retry loop: up to `maxRetries` (default 3) attempts; validation errors appended to retry prompt
  - JSON extraction: output file first, falls back to `jsonutil.ExtractInto` on stdout
  - 1 MB PRD file size cap; context cancellation honored at every iteration
  - Non-blocking event sends via `select { case ch <- evt: default: }`
  - Full test suite: success via file/stdout, retry paths, event ordering, prompt content, size caps, context cancellation
- **Files created/modified:**
  - `internal/prd/shredder.go` — Shredder, all option types, prompt template, extraction logic
  - `internal/prd/shredder_test.go` — 30+ table-driven tests covering all code paths
- **Verification:** `go build` ✓  `go vet` ✓  `go test ./internal/prd/...` ✓

---

### T-058: JSON Extraction Utility

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - `Extract(text string) (json.RawMessage, error)` — returns first valid JSON object or array using multi-strategy extraction
  - `ExtractAll(text string) []json.RawMessage` — returns all valid JSON objects/arrays in order of appearance
  - `ExtractInto(text string, target interface{}) error` — updated to delegate to multi-strategy `Extract`; backward compatible
  - `ExtractFromFile(path string, target interface{}) error` — reads file from disk and calls `ExtractInto`
  - `ExtractFirst(text string) (string, bool)` — retained unchanged for backward compatibility (object-only, no arrays)
  - Extraction strategies (in order): (1) markdown code fence `\`\`\`json` or `\`\`\`` with `TrimSpace` and `json.Valid`, (2) brace/bracket matching for top-level `{}` and `[]` structures
  - `sanitize()` helper: strips UTF-8 BOM, strips ANSI escape codes (`\x1b[...m`), enforces 10 MB cap
  - `fenceSpan` range tracking: prevents brace-matching from emitting the same JSON already found in a fence
  - `matchingDelimiter()`: handles both `{}`/`[]`, nested delimiters, quoted strings, backslash escapes
  - 40+ unit tests in `extract_test.go`: objects, arrays, code fences, ANSI stripping, BOM removal, empty fences, non-json language tags, file extraction, max size cap, benchmarks, fuzz seeds
  - All existing tests in `jsonutil_test.go` continue to pass unchanged
- **Files created/modified:**
  - `internal/jsonutil/extract.go` — new primary extraction API with multi-strategy engine
  - `internal/jsonutil/extract_test.go` — comprehensive tests for new API
  - `internal/jsonutil/jsonutil.go` — updated: removed old `ExtractInto` (now in extract.go), retained `ExtractFirst` and helpers with backward-compat doc comment
- **Verification:** `go build` ✓  `go vet` ✓  `go test ./internal/jsonutil/...` ✓

---

### T-059: Parallel Epic Workers

- **Status:** Completed
- **Date:** 2026-02-18
- **What was built:**
  - `ScatterOrchestrator` struct orchestrating bounded-concurrency parallel decomposition of epics into tasks
  - `ScatterOption` functional option type with `WithScatterMaxRetries`, `WithScatterLogger`, `WithScatterEvents`, `WithConcurrency`, `WithRateLimiter`
  - `ScatterOpts` struct for per-call parameters (PRDContent, Breakdown, Model, Effort)
  - `ScatterResult` struct with Results ([]*EpicTaskResult sorted by epic ID), Failures, Duration
  - `ScatterFailure` struct capturing EpicID, last ValidationErrors, and underlying Err
  - `ScatterEvent` / `ScatterEventType` for progress tracking (worker_started, worker_completed, worker_retry, worker_failed, rate_limited)
  - `scatterValidationFailure` sentinel type: distinguishes validation exhaustion (non-fatal, other workers continue) from fatal errors (context cancel, rate-limit exceeded)
  - `errgroup.WithContext` + `g.SetLimit(concurrency)` for bounded parallel execution; workers always return nil so sibling goroutines are not cancelled on partial failure
  - `runWithRetry` per-epic worker: honors context cancellation, checks `rateLimiter.ShouldWait` before each attempt, calls `WaitForReset` if needed, emits `rate_limited` events, removes stale output files on retry, injects validation errors into retry prompt
  - `sanitizeEpicID` helper: strips all non-`[A-Za-z0-9_-]` characters using compiled regexp; `epicFilePath` verifies derived path stays inside `workDir`
  - `buildScatterPrompt` with embedded template using `[[`/`]]` delimiters; `buildOtherEpicsSummary` builds bulleted cross-epic reference list
  - `extractTaskResult`: output file first, falls back to `jsonutil.ExtractInto` on stdout, validates via `ParseEpicTaskResult`
  - Mutex-protected result/failure accumulators; results sorted by EpicID post-wait for determinism
  - Non-blocking event sends via `select { case ch <- evt: default: }`
  - Full test suite: success via file/stdout, sorted results, partial/total failure, retry paths, concurrency limiting, event types, stale-file removal, rate-limiter integration, sanitizeEpicID table tests, prompt content assertions
- **Test augmentation (2026-02-18):**
  - Fixed `MockAgent.Calls` data race: added `sync.Mutex` protection for concurrent `Run()` calls; added `GetCalls()` safe snapshot helper
  - Added `TestScatterOrchestrator_Scatter_ConcurrencyOne`: verifies serialized execution with concurrency=1
  - Added `TestScatterOrchestrator_Scatter_ThreeEpicsWithConcurrencyTwo`: explicit AC test — 3 epics, concurrency=2, all succeed, max concurrent <= 2
  - Added `TestScatterOrchestrator_Scatter_ContextCancelledMidScatter`: AC test — one fast worker cancels mid-scatter, partial results returned with `context.Canceled`
  - Added `TestScatterOrchestrator_WithScatterLogger`: verifies `WithScatterLogger` functional option stores logger
  - Added `TestScatterOrchestrator_Scatter_OneFailAllRetries_TwoSucceed`: AC test — 3 epics, E-001 exhausts all retries, E-002/E-003 succeed; ScatterResult has 2 results, 1 failure
  - Added `TestScatterOrchestrator_Scatter_OutputFileWritten`: verifies output file path is derived from epic ID, written inside workDir
  - Added `TestScatterOrchestrator_Scatter_OtherEpicsSummaryInPrompt`: verifies each prompt lists other epics but excludes self
  - Added `TestScatterValidationFailure_ErrorsPreserved`: verifies `ScatterFailure.Errors` is populated from `scatterValidationFailure` sentinel
- **Files created/modified:**
  - `internal/agent/mock.go` — added `sync.Mutex` to `MockAgent` for concurrent-safe `Calls` tracking; added `GetCalls()` helper
  - `internal/prd/worker.go` — ScatterOrchestrator, all option types, prompt template, per-epic worker logic (unchanged)
  - `internal/prd/worker_test.go` — 38 test functions (8 new) + 1 benchmark covering all acceptance criteria
- **Verification:** `go build` ✓  `go vet` ✓  `go test ./internal/prd/...` ✓  `go test -race ./internal/prd/...` ✓

---

## In Progress Tasks

_None currently_

---

## Not Started Tasks

### Phase 5: PRD Decomposition (T-056 to T-065)

- **Status:** In Progress (T-056, T-057, T-058, T-059 completed)
- **Tasks:** 10 (10 Must Have)
- **Estimated Effort:** 70-110 hours
- **PRD Roadmap:** Weeks 9-10

#### Task List

| Task | Name | Priority | Effort | Status |
|------|------|----------|--------|--------|
| T-056 | Epic JSON Schema and Types | Must Have | Small (2-4hrs) | Completed |
| T-057 | PRD Shredder (Single Agent -> Epic JSON) | Must Have | Medium (8-12hrs) | Completed |
| T-058 | JSON Extraction Utility | Must Have | Medium (6-10hrs) | Completed |
| T-059 | Parallel Epic Workers | Must Have | Medium (8-12hrs) | Completed |
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
