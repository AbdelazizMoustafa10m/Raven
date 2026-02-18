# Raven Task Progress Log

## Summary

| Status | Count |
|--------|-------|
| Completed | 27 |
| In Progress | 0 |
| Not Started | 59 |

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
| Task spec markdown parser | T-016 | `ParseTaskSpec`, `ParseTaskFile`, `DiscoverTasks`, `ParsedTaskSpec.ToTask()`, pre-compiled regexes, 1 MiB size guard, CRLF/BOM normalisation |
| Task state management | T-017 | `StateManager` with `Load`, `LoadMap`, `Get`, `Update`, `UpdateStatus`, `Initialize`, `StatusCounts`, `TasksWithStatus`; pipe-delimited `task-state.conf`; atomic write + mutex for concurrent safety; `ValidStatuses()` and `IsValid()` on `TaskStatus` |
| Phase configuration parser | T-018 | `LoadPhases`, `ParsePhaseLine` (4-field and 6-field format auto-detection), `PhaseForTask`, `PhaseByID`, `TaskIDNumber`, `TasksInPhase`, `FormatPhaseLine`, `ValidatePhases`; reads `phases.conf`; sorts phases by ID; validates non-overlapping ranges |
| Dependency resolution & next-task selection | T-019 | `TaskSelector` with `NewTaskSelector`, `SelectNext`, `SelectNextInRange`, `SelectByID`, `GetPhaseProgress`, `GetAllProgress`, `IsPhaseComplete`, `BlockedTasks`, `CompletedTaskIDs`, `RemainingTaskIDs`; `PhaseProgress` aggregate struct; O(1) spec lookup via specMap; read-only (never mutates state) |
| Status command with progress bars | T-020 | `raven status` CLI command with `--phase`, `--json`, `--verbose` flags; `renderPhaseProgress` (bubbles/progress.ViewAs static bar), `renderSummary`, `renderTaskDetails`; `buildUngroupedProgress` for no-phases mode; graceful handling of missing phases.conf; JSON output via `statusOutput`/`statusPhaseOutput` structs |
| Agent interface & registry | T-021 | `Agent` interface (5 methods: Name, Run, CheckPrerequisites, ParseRateLimit, DryRunCommand); `Registry` type with Register, Get, MustGet, List, Has; `AgentConfig` struct for raven.toml `[agents.*]` sections; `MockAgent` with builder methods (WithRunFunc, WithRateLimit, WithPrereqError); sentinel errors (ErrNotFound, ErrDuplicateName, ErrInvalidName); agent name validation via regex; compile-time interface check |
| Claude agent adapter | T-022 | `ClaudeAgent` struct implementing `Agent`; `NewClaudeAgent(config, logger)`; `buildCommand` and `buildArgs` helpers; `--permission-mode accept --print` flags; model/allowedTools/outputFormat flag injection with RunOpts-over-config precedence; `CLAUDE_CODE_EFFORT_LEVEL` env var; large-prompt temp-file spill; `ParseRateLimit` with `reClaudeRateLimit`/`reClaudeResetTime`/`reClaudeTryAgain` regexes; `parseResetDuration` unit parser; `DryRunCommand` with prompt truncation; injected `claudeLogger` interface; compile-time `var _ Agent = (*ClaudeAgent)(nil)` |
| Codex agent adapter | T-023 | `CodexAgent` struct implementing `Agent`; `NewCodexAgent(config, logger)`; `buildCommand` with `codex exec --sandbox --ephemeral -a never` flags; model flag with RunOpts-over-config precedence; prompt via `--prompt` or `--prompt-file`; three-tier `ParseRateLimit`: short decimal-seconds (`5.448s`), long format (`1 days 2 hours`), fallback keyword; `parseCodexDuration` helper; `DryRunCommand` with Unicode-safe prompt truncation; `codexLogger` interface; compile-time `var _ Agent = (*CodexAgent)(nil)` |
| Gemini agent stub | T-024 | `GeminiAgent` stub struct implementing `Agent`; `NewGeminiAgent(config AgentConfig)`; `Run` and `CheckPrerequisites` return `ErrNotImplemented`; `ParseRateLimit` always returns nil/false; `DryRunCommand` returns placeholder comment; `ErrNotImplemented` sentinel error; compile-time `var _ Agent = (*GeminiAgent)(nil)`; no os/exec imports (pure stub) |
| Rate-limit detection & coordination | T-025 | `RateLimitCoordinator` with `sync.RWMutex` for thread safety; `ProviderState` per-provider tracking (IsLimited, ResetAt, WaitCount, LastMessage, UpdatedAt); `BackoffConfig` (DefaultWait, MaxWaits, JitterFactor); `DefaultBackoffConfig()` (60s/5/0.1); provider constants (ProviderAnthropic/OpenAI/Google); `AgentProvider` map (claude->anthropic, codex->openai, gemini->google); `providerForAgent` fallback to agent name; `RecordRateLimit` (creates/updates state, increments WaitCount, extends ResetAt to max, captures callback before unlock); `ClearRateLimit` (sets IsLimited=false, preserves WaitCount); `ShouldWait` (read lock, returns copy if limited and reset is future); `WaitForReset` (checks ShouldWait, ExceededMaxWaits, timer+ctx.Done select); `ExceededMaxWaits` (MaxWaits=0 always true, WaitCount>=MaxWaits); `GetState`/`AllStates` (sorted snapshot); `computeWaitDuration` with jitter; `ErrMaxWaitsExceeded` sentinel; callback captured under lock (race-free) |
| Prompt template system | T-026 | `PromptContext` struct (task, phase, project, verification, progress, agent fields); `PromptGenerator` with custom `[[`/`]]` delimiters (avoids `{{`/`}}` conflicts in task spec content); `NewPromptGenerator(templateDir)` with directory validation; `LoadTemplate(name)` with directory-traversal guard and LRU caching; `Generate(templateName, ctx)` (empty name falls back to built-in default); `GenerateFromString(tmplStr, ctx)` for inline templates; `BuildContext(spec, phase, cfg, selector, agentName)` populating all fields from live state; `DefaultImplementTemplate` const; `formatIDSummary` helper ("None" for empty, ", "-joined otherwise); `VerificationString` = `strings.Join(cmds, " && ")`; model lookup from `cfg.Agents[agentName].Model`; debug logging of rendered output |
| Implementation loop runner | T-027 | `Runner` struct orchestrating the full implement loop; `RunConfig` (AgentName, PhaseID, TaskID, MaxIterations/50, MaxLimitWaits/5, SleepBetween/5s, DryRun, TemplateName); `NewRunner(selector, promptGen, agent, stateManager, rateLimiter, cfg, phases, events, logger)`; `Run(ctx, runCfg)` -- phase mode: select->prompt->invoke->detect->update->sleep->repeat until phase complete, max-iters, or ctx cancelled; `RunSingleTask(ctx, runCfg)` -- single-task mode via `--task T-007`; `DetectSignals(output)` exported scanner for PHASE_COMPLETE/TASK_BLOCKED/RAVEN_ERROR signals; internal helpers: `selectTask`, `generatePrompt`, `invokeAgent`, `invokeAgentWithRetry` (rate-limit wait+retry), `handleCompletion` (marks task completed/blocked based on signal), `handleDryRun` (print prompt+cmd to stderr, revert task, emit dry_run event), `emit` (non-blocking channel send with `select { default: }`); `applyDefaults` fills zero-value RunConfig fields; `sleepWithContext` with `time.NewTimer`+`select` for cancellable sleep; stale-task detection (3 consecutive same-task selections emits EventLoopError warning); `LoopEvent` struct with Type/Iteration/TaskID/AgentName/Message/Timestamp/Duration/WaitTime; 16 `LoopEventType` constants; `CompletionSignal` type with 3 signal constants |

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
| Gemini agent stub | `internal/agent/gemini.go` |
| Rate-limit coordinator | `internal/agent/ratelimit.go` |
| Implementation loop runner | `internal/loop/runner.go` |
| Dependency declarations | `tools.go` |

#### Verification

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass

---

## In Progress Tasks

_None currently_

---

## Not Started Tasks

### Phase 2: Task System & Agent Adapters (T-016 to T-030)

- **Status:** In Progress
- **Tasks:** 15 (14 Must Have, 1 Should Have)
- **Estimated Effort:** 110-175 hours
- **PRD Roadmap:** Weeks 3-4

#### Task List

| Task | Name | Priority | Effort | Status |
|------|------|----------|--------|--------|
| T-016 | Task Spec Markdown Parser | Must Have | Medium (6-10hrs) | Completed |
| T-017 | Task State Management (task-state.conf) | Must Have | Medium (8-12hrs) | Completed |
| T-018 | Phase Configuration Parser (phases.conf) | Must Have | Small (3-5hrs) | Completed |
| T-019 | Dependency Resolution & Next-Task Selection | Must Have | Medium (8-12hrs) | Completed |
| T-020 | Status Command -- raven status | Must Have | Medium (6-10hrs) | Completed |
| T-021 | Agent Interface & Registry | Must Have | Medium (6-10hrs) | Completed |
| T-022 | Claude Agent Adapter | Must Have | Medium (8-12hrs) | Completed |
| T-023 | Codex Agent Adapter | Must Have | Medium (6-10hrs) | Completed |
| T-024 | Gemini Agent Stub | Should Have | Small (2-3hrs) | Completed |
| T-025 | Rate-Limit Detection & Coordination | Must Have | Medium (8-12hrs) | Completed |
| T-026 | Prompt Template System | Must Have | Medium (6-10hrs) | Completed |
| T-027 | Implementation Loop Runner | Must Have | Large (16-24hrs) | Completed |
| T-028 | Loop Recovery (Rate-Limit Wait, Dirty-Tree) | Must Have | Medium (8-12hrs) | Not Started |
| T-029 | Implementation CLI Command -- raven implement | Must Have | Medium (8-12hrs) | Not Started |
| T-030 | Progress File Generation -- PROGRESS.md | Must Have | Small (4-6hrs) | Not Started |

**Deliverable:** `raven implement --agent claude --phase 1` runs the full implementation loop for a phase.

---

### Phase 3: Review Pipeline (T-031 to T-042)

- **Status:** Not Started
- **Tasks:** 12 (12 Must Have)
- **Estimated Effort:** 96-136 hours
- **PRD Roadmap:** Weeks 5-6

#### Task List

| Task | Name | Priority | Effort | Status |
|------|------|----------|--------|--------|
| T-031 | Review Finding Types and Schema | Must Have | Small (2-4hrs) | Not Started |
| T-032 | Git Diff Generation and Risk Classification | Must Have | Medium (6-10hrs) | Not Started |
| T-033 | Review Prompt Synthesis | Must Have | Medium (6-10hrs) | Not Started |
| T-034 | Finding Consolidation and Deduplication | Must Have | Medium (6-10hrs) | Not Started |
| T-035 | Multi-Agent Parallel Review Orchestrator | Must Have | Large (14-20hrs) | Not Started |
| T-036 | Review Report Generation (Markdown) | Must Have | Medium (6-10hrs) | Not Started |
| T-037 | Verification Command Runner | Must Have | Medium (6-10hrs) | Not Started |
| T-038 | Review Fix Engine | Must Have | Large (14-20hrs) | Not Started |
| T-039 | PR Body Generation with AI Summary | Must Have | Medium (6-10hrs) | Not Started |
| T-040 | PR Creation via gh CLI | Must Have | Medium (6-10hrs) | Not Started |
| T-041 | CLI Command -- raven review | Must Have | Medium (6-10hrs) | Not Started |
| T-042 | CLI Commands -- raven fix and raven pr | Must Have | Medium (6-10hrs) | Not Started |

**Deliverable:** `raven review --agents claude,codex --concurrency 4` produces a consolidated review report.

---

### Phase 4: Workflow Engine & Pipeline (T-043 to T-055)

- **Status:** Not Started
- **Tasks:** 13 (12 Must Have, 1 Should Have)
- **Estimated Effort:** 96-144 hours
- **PRD Roadmap:** Weeks 7-8

#### Task List

| Task | Name | Priority | Effort | Status |
|------|------|----------|--------|--------|
| T-043 | Workflow Event Types and Constants | Must Have | Small (2-4hrs) | Not Started |
| T-044 | Step Handler Registry | Must Have | Small (2-4hrs) | Not Started |
| T-045 | Workflow Engine Core -- State Machine Runner | Must Have | Large (14-20hrs) | Not Started |
| T-046 | Workflow State Checkpointing and Persistence | Must Have | Medium (6-10hrs) | Not Started |
| T-047 | Resume Command -- List and Resume Interrupted Workflows | Must Have | Medium (6-10hrs) | Not Started |
| T-048 | Workflow Definition Validation | Must Have | Medium (6-10hrs) | Not Started |
| T-049 | Built-in Workflow Definitions and Step Handlers | Must Have | Large (14-20hrs) | Not Started |
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

### Phase 6: TUI Command Center (T-066 to T-078)

- **Status:** Not Started
- **Tasks:** 13 (12 Must Have, 1 Should Have)
- **Estimated Effort:** 88-136 hours
- **PRD Roadmap:** Weeks 11-13

#### Task List

| Task | Name | Priority | Effort | Status |
|------|------|----------|--------|--------|
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

_Last updated: 2026-02-18_ (T-027 completed)
