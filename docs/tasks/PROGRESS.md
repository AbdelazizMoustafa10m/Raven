# Raven Task Progress Log

## Summary

| Status | Count |
|--------|-------|
| Completed | 12 |
| In Progress | 0 |
| Not Started | 75 |

---

## Completed Tasks

### T-001: Go Project Initialization and Module Setup

- **Status:** Completed
- **Date:** 2026-02-17

**What was built:**

- Go module initialized with `github.com/AbdelazizMoustafa10m/Raven` module path and Go 1.24 directive
- Minimal entry point at `cmd/raven/main.go` with placeholder output
- All 12 internal subpackages with `doc.go` stubs: cli, config, workflow, agent, task, loop, review, prd, pipeline, git, tui, buildinfo
- `tools.go` with `//go:build tools` tag to declare all direct dependencies
- 11 direct dependencies declared: cobra, bubbletea, lipgloss, bubbles, huh, log, toml, sync, doublestar, testify, xxhash
- `testdata/` and `templates/go-cli/` directories
- Comprehensive test suite covering all acceptance criteria (25+ test cases)

**Files created/modified:**

- `go.mod` - Module declaration with all direct dependencies
- `go.sum` - Dependency checksums
- `cmd/raven/main.go` - Minimal entry point
- `cmd/raven/main_test.go` - Build, run, vet, tidy verification tests
- `internal/{cli,config,workflow,agent,task,loop,review,prd,pipeline,git,tui,buildinfo}/doc.go` - Package stubs
- `internal/project_test.go` - Project structure and dependency tests
- `tools.go` - Dependency declarations with build tag
- `templates/go-cli/.gitkeep` - Template directory placeholder
- `testdata/.gitkeep` - Test fixture directory placeholder
- `.gitignore` - Added `*.coverprofile`

**Verification:**

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass (all 25+ tests)
- `go mod tidy` no drift

---

### T-002: Makefile with Build Targets and ldflags

- **Status:** Completed
- **Date:** 2026-02-17

**What was built:**

- GNU Makefile with 12 targets: `all`, `build`, `test`, `vet`, `lint`, `tidy`, `fmt`, `clean`, `install`, `bench`, `run-version`, `build-debug`
- Version injection via ldflags `-X` flags for `internal/buildinfo.Version`, `.Commit`, `.Date`
- `CGO_ENABLED=0` for all build and install targets (pure Go cross-compilation)
- Separate `LDFLAGS_DEBUG` without `-s -w` for debug builds with `dlv` support
- Added `Version`, `Commit`, `Date` variables to `internal/buildinfo` package with defaults (`"dev"`, `"unknown"`, `"unknown"`)
- Comprehensive test suite: static analysis tests for Makefile content + integration tests for build/clean/debug targets and ldflags injection

**Files created/modified:**

- `Makefile` - GNU Makefile with build targets and ldflags
- `internal/buildinfo/doc.go` - Added Version, Commit, Date variables for ldflags injection
- `internal/buildinfo/buildinfo_test.go` - Default values test (table-driven)
- `Makefile_test.go` - Makefile content tests + integration tests for make targets

**Verification:**

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass (all tests including new Makefile tests)
- `go mod tidy` no drift
- `make build` produces `dist/raven` with injected version info
- `make build-debug` produces `dist/raven-debug` with debug symbols
- `make clean` removes `dist/` directory

---

### T-003: Build Info Package -- internal/buildinfo

- **Status:** Completed
- **Date:** 2026-02-17

**What was built:**

- `Info` struct with JSON struct tags for structured build information
- `GetInfo()` function returning populated `Info` from package-level variables
- `Info.String()` method returning human-readable format: `raven v{version} (commit: {commit}, built: {date})`
- Comprehensive test suite with 11 test functions and 3 benchmarks achieving 100% coverage

**Files created/modified:**

- `internal/buildinfo/buildinfo.go` - Info struct, GetInfo() accessor, String() formatter
- `internal/buildinfo/buildinfo_test.go` - Comprehensive tests: defaults, String formatting, JSON marshal/unmarshal, round-trip, edge cases, struct tags, zero values

**Verification:**

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass (all tests)
- `go test -race -cover` pass (100% coverage, no races)
- `go mod tidy` no drift

---

### T-004: Central Data Types -- WorkflowState, RunOpts, RunResult, Task, Phase, StepRecord

- **Status:** Completed
- **Date:** 2026-02-17

**What was built:**

- `WorkflowState` and `StepRecord` types in `internal/workflow/state.go` with constructor, `AddStepRecord`, and `LastStep` methods
- `RunOpts`, `RunResult`, and `RateLimitInfo` types in `internal/agent/types.go` with `Success()` and `WasRateLimited()` methods
- `Task`, `Phase`, and `TaskStatus` types in `internal/task/types.go` with `IsReady()` and `ValidStatus()` functions
- Five `TaskStatus` string constants (not iota): `not_started`, `in_progress`, `completed`, `blocked`, `skipped`
- All types have JSON struct tags matching PRD Section 6.4
- Empty slices serialize as `[]` (not `null`), empty maps as `{}` (not `null`)
- `time.Duration` serializes as nanoseconds (int64) -- documented in type comments

**Files created/modified:**

- `internal/workflow/state.go` - WorkflowState, StepRecord, NewWorkflowState, AddStepRecord, LastStep
- `internal/workflow/state_test.go` - 9 tests: constructor, JSON round-trip, struct tags, omitempty, duration nanoseconds
- `internal/agent/types.go` - RunOpts, RunResult, RateLimitInfo, Success, WasRateLimited
- `internal/agent/types_test.go` - 9 tests: Success/WasRateLimited table-driven, JSON round-trip, omitempty, struct tags
- `internal/task/types.go` - Task, Phase, TaskStatus, IsReady, ValidStatus
- `internal/task/types_test.go` - 9 tests: IsReady (8 cases), ValidStatus (9 cases), JSON round-trip, serialization

**Verification:**

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass (all tests)
- `go test -race -cover` pass (100% coverage, no races)
- `go mod tidy` no drift

---

### T-005: Structured Logging with charmbracelet/log

- **Status:** Completed
- **Date:** 2026-02-17

**What was built:**

- `internal/logging` package providing centralized logger factory with component prefixes
- `Setup(verbose, quiet, jsonFormat bool)` function for global logging configuration (called once during CLI init)
- `New(component string)` factory function creating loggers with component prefixes via `log.WithPrefix()`
- `SetOutput(w io.Writer)` for test output capture
- Re-exported level constants (`LevelDebug`, `LevelInfo`, `LevelWarn`, `LevelError`, `LevelFatal`)
- All output goes to stderr; stdout reserved for structured output
- Quiet wins over verbose when both flags set
- Comprehensive test suite: 17 test functions including 8-case table-driven level filtering, concurrent logging with 10 goroutines, JSON/text formatter toggling, stdout isolation verification

**Files created/modified:**

- `internal/logging/logging.go` - Logger factory with Setup(), New(), SetOutput(), and level constants
- `internal/logging/logging_test.go` - 17 test functions with 100% coverage
- `internal/project_test.go` - Updated subpackage count assertion for new logging package

**Verification:**

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass (all tests)
- `go test -race -cover` pass (100% coverage, no races)
- `go mod tidy` no drift

---

### T-006: Cobra CLI Root Command and Global Flags

- **Status:** Completed
- **Date:** 2026-02-17

**What was built:**

- Cobra root command (`rootCmd`) in `internal/cli/root.go` with Use "raven", Short and Long descriptions
- Six global persistent flags: `--verbose/-v`, `--quiet/-q`, `--config`, `--dir`, `--dry-run`, `--no-color`
- `PersistentPreRunE` hook that initializes logging via `logging.Setup()`, checks environment variable overrides (`RAVEN_VERBOSE`, `RAVEN_QUIET`, `NO_COLOR`, `RAVEN_NO_COLOR`, `RAVEN_LOG_FORMAT`), disables color via `lipgloss.SetColorProfile(termenv.Ascii)`, and resolves `--dir` working directory changes
- `Execute() int` public function returning exit codes (0 success, 1 error)
- `RunE` that displays full help (Usage + Flags) when invoked with no subcommand
- `SilenceUsage: true` and `SilenceErrors: true` for Raven's own error handling
- Environment variable precedence: CLI flags > env vars > defaults
- Updated `cmd/raven/main.go` to call `cli.Execute()` with `os.Exit()`
- Comprehensive test suite: 27 test functions covering flag registration, PreRun behavior, env var propagation, directory handling, edge cases

**Files created/modified:**

- `internal/cli/root.go` - Root command, global flags, PersistentPreRunE, Execute()
- `internal/cli/root_test.go` - 27 tests: metadata, flags, exit codes, env vars, --dir edge cases, help output
- `cmd/raven/main.go` - Updated to call cli.Execute() instead of fmt.Println
- `cmd/raven/main_test.go` - Updated binary tests for Cobra help output
- `go.mod` - Added muesli/termenv and spf13/pflag as direct dependencies

**Verification:**

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass (all tests)
- `go mod tidy` no drift

---

### T-007: Version Command -- raven version

- **Status:** Completed
- **Date:** 2026-02-17

**What was built:**

- `raven version` command printing human-readable version string to stdout
- `--json` flag for structured JSON output with indentation
- `cobra.NoArgs` validation rejecting extra positional arguments
- Command registered as subcommand of root, visible in `raven --help`
- Comprehensive test suite with 11 tests covering all acceptance criteria

**Files created/modified:**

- `internal/cli/version.go` - Version command with `--json` flag, uses `buildinfo.GetInfo()`
- `internal/cli/version_test.go` - 11 tests: human-readable output, defaults, JSON validity, indentation, extra args rejection, registration, help visibility, metadata, flag registration, stdout isolation, JSON round-trip

**Verification:**

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass (all tests)
- `go mod tidy` no drift

---

### T-008: Shell Completion Command -- raven completion

- **Status:** Completed
- **Date:** 2026-02-17

**What was built:**

- `raven completion <shell>` command generating shell completion scripts for bash, zsh, fish, and PowerShell
- Uses Cobra's built-in `GenBashCompletionV2`, `GenZshCompletion`, `GenFishCompletion`, `GenPowerShellCompletionWithDesc` for shell-specific generation
- `ValidArgs` + `cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs)` for strict argument validation
- Long description includes installation examples for all four shells across Linux, macOS, and Windows
- `DisableFlagsInUseLine: true` since command takes no flags
- Comprehensive test suite with 16 test functions covering all acceptance criteria

**Files created/modified:**

- `internal/cli/completion.go` - Completion command with shell-specific generation via Cobra's built-in methods
- `internal/cli/completion_test.go` - 16 tests: per-shell output validation, table-driven all-shells test, no-args error, invalid shell error, extra args error, case sensitivity, registration, help visibility, metadata, ValidArgs, install examples, stdout isolation

**Verification:**

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass (all tests)
- `go mod tidy` no drift

---

### T-009: TOML Configuration Types and Loading with BurntSushi/toml

- **Status:** Completed
- **Date:** 2026-02-17

**What was built:**

- Complete TOML configuration type hierarchy: `Config`, `ProjectConfig`, `AgentConfig`, `ReviewConfig`, `WorkflowConfig` with `toml` struct tags
- `FindConfigFile(startDir)` walks up directories from CWD to find `raven.toml`, stops at filesystem root
- `LoadFromFile(path)` parses TOML via `toml.DecodeFile` and returns `*Config` plus `toml.MetaData` for unknown-key detection
- `NewDefaults()` returns config with all PRD-specified defaults (TasksDir, TaskStateFile, PhasesConf, ProgressFile, LogDir, PromptDir, BranchTemplate)
- 9 test fixture TOML files covering valid, partial, empty, UTF-8, multiline, special agent names, malformed, and unknown keys
- 21 tests across load and defaults test files with race-safe parallel execution

**Files created/modified:**

- `internal/config/config.go` - Configuration type hierarchy (Config, ProjectConfig, AgentConfig, ReviewConfig, WorkflowConfig)
- `internal/config/defaults.go` - NewDefaults() with PRD-specified default values
- `internal/config/load.go` - FindConfigFile() and LoadFromFile() with BurntSushi/toml
- `internal/config/load_test.go` - 17 tests: LoadFromFile (valid, partial, agents, malformed, nonexistent, metadata, empty, comments, UTF-8, multiline, special names) and FindConfigFile (current dir, parent dir, not found, root, deeply nested, absolute path)
- `internal/config/defaults_test.go` - 4 tests: default values, empty agents, empty workflows, zero review
- `testdata/valid-full.toml` - Full config matching PRD Section 5.10
- `testdata/valid-partial.toml` - Project-only config
- `testdata/valid-empty.toml` - Empty file
- `testdata/valid-comments-only.toml` - Comments only
- `testdata/valid-utf8.toml` - UTF-8 characters
- `testdata/valid-multiline.toml` - Multi-line strings
- `testdata/valid-special-agent-names.toml` - Agent names with hyphens/dots
- `testdata/invalid-malformed.toml` - Malformed TOML
- `testdata/valid-unknown-keys.toml` - Unknown keys for MetaData.Undecoded

**Verification:**

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass (all tests)
- `go test -race` pass (no races)
- `go mod tidy` no drift

---

### T-010: Config Resolution -- CLI > env > file > defaults

- **Status:** Completed
- **Date:** 2026-02-17

**What was built:**

- Four-layer configuration resolution system: CLI flags > env vars > raven.toml > defaults
- `Resolve()` function with explicit field-by-field merging (no reflection) for type safety
- `ConfigSource` enum (default, file, env, cli) with source tracking for every resolved field
- `ResolvedConfig` wrapper pairing merged `Config` with `Sources` map for `raven config debug` (T-012)
- `CLIOverrides` with pointer types (`*string`, `*bool`) to distinguish "not set" from "set to zero value"
- `EnvFunc` injection for testable environment variable resolution
- 7 RAVEN_* environment variables: PROJECT_NAME, TASKS_DIR, LOG_DIR, PROMPT_DIR, BRANCH_TEMPLATE, AGENT_MODEL, AGENT_EFFORT
- Agent map merging: file agents additive on top of defaults; env/CLI agent overrides apply to ALL agents
- Deep copy of agents and workflows to prevent shared reference mutation
- 30 test functions with table-driven priority tests, edge cases, and deep copy verification

**Files created/modified:**

- `internal/config/resolve.go` - Resolve(), ConfigSource, ResolvedConfig, CLIOverrides, EnvFunc, layer merge helpers
- `internal/config/resolve_test.go` - 30 tests: priority ordering, env var mapping, agent merging, edge cases, deep copy safety

**Verification:**

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass (all tests)
- `go test -race` pass (no races)
- `go mod tidy` no drift

---

### T-012: Config Debug and Validate Commands

- **Status:** Completed
- **Date:** 2026-02-17

**What was built:**

- `raven config` parent command (namespace-only, shows help with subcommands)
- `raven config debug` command printing fully-resolved configuration with source annotations (color-coded: green=default, blue=file, yellow=env, red=cli)
- `raven config validate` command running the full validation suite and reporting errors/warnings with colored labels
- `loadAndResolveConfig()` shared helper returning `(*ResolvedConfig, *toml.MetaData, error)` -- handles `--config` flag, auto-detection via `FindConfigFile`, and nil metadata when no file is found
- `printResolvedConfig()` helper formatting all config sections ([project], [agents.*], [review], [workflows.*]) with aligned columns and lipgloss source labels
- `printValidationResult()` helper formatting errors (red), warnings (yellow), and "No issues found." (green)
- Fixed-width field column (24 chars) for readable alignment
- Agents and workflows sorted alphabetically for deterministic output
- `--no-color` respected automatically (lipgloss Ascii profile set by root PersistentPreRunE)
- 35+ unit tests covering registration, output routing, format helpers, loadAndResolveConfig edge cases, and end-to-end command invocations

**Files created/modified:**

- `internal/cli/config_cmd.go` - configCmd, configDebugCmd, configValidateCmd, loadAndResolveConfig, printResolvedConfig, printValidationResult, fmtStr, fmtSlice, sourceStyle, style vars
- `internal/cli/config_cmd_test.go` - 35+ tests: registration, metadata, help output, debug/validate with file/no-file/explicit-flag, validation errors/warnings/unknown-keys, output routing, fmtStr/fmtSlice table-driven, loadAndResolveConfig unit tests, sourceStyle tests

**Verification:**

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass (all tests)
- `go mod tidy` no drift

---

### T-011: Configuration Validation and Unknown Key Detection

- **Status:** Completed
- **Date:** 2026-02-17

**What was built:**

- `Validate()` function performing structural, semantic, and unknown-key validation of resolved config
- `ValidationSeverity`, `ValidationIssue`, `ValidationResult` types with `HasErrors()`, `HasWarnings()`, `Errors()`, `Warnings()` methods
- Project validation: required name, recognized language set, non-empty verification commands
- Agent validation: required command, recognized effort values, prompt_template existence warnings
- Review validation: regex compilation checks for extensions and risk_patterns, path existence warnings
- Workflow validation: non-empty steps, transition keys and targets must reference defined steps
- Unknown key detection via `toml.MetaData.Undecoded()` with nil-safe guard
- Comprehensive test suite with 50+ test functions covering all acceptance criteria, edge cases, and integration tests

**Files created/modified:**

- `internal/config/validate.go` - Validation types and Validate() function with section validators
- `internal/config/validate_test.go` - 50+ tests: ValidationResult methods, project/agent/review/workflow validation, unknown keys, filesystem warnings, edge cases, integration with testdata fixtures

**Verification:**

- `go build ./cmd/raven/` pass
- `go vet ./...` pass
- `go test ./...` pass (all tests)
- `go mod tidy` no drift

---

## In Progress Tasks

_None currently_

---

## Not Started Tasks

### Phase 1: Foundation (T-001 to T-015)

- **Status:** Not Started
- **Tasks:** 15 (15 Must Have)
- **Estimated Effort:** 50-90 hours
- **PRD Roadmap:** Weeks 1-2

#### Task List

| Task | Name | Priority | Effort | Status |
|------|------|----------|--------|--------|
| T-001 | Go Project Initialization and Module Setup | Must Have | Medium (4-8hrs) | Completed |
| T-002 | Makefile with Build Targets and ldflags | Must Have | Small (2-4hrs) | Completed |
| T-003 | Build Info Package -- internal/buildinfo | Must Have | Small (1-2hrs) | Completed |
| T-004 | Central Data Types (WorkflowState, RunOpts, RunResult, Task, Phase) | Must Have | Medium (4-8hrs) | Completed |
| T-005 | Structured Logging with charmbracelet/log | Must Have | Small (2-4hrs) | Completed |
| T-006 | Cobra CLI Root Command and Global Flags | Must Have | Medium (4-8hrs) | Completed |
| T-007 | Version Command -- raven version | Must Have | Small (1-2hrs) | Completed |
| T-008 | Shell Completion Command -- raven completion | Must Have | Small (2-3hrs) | Completed |
| T-009 | TOML Configuration Types and Loading | Must Have | Medium (6-10hrs) | Completed |
| T-010 | Config Resolution -- CLI > env > file > defaults | Must Have | Medium (6-10hrs) | Completed |
| T-011 | Configuration Validation and Unknown Key Detection | Must Have | Medium (4-6hrs) | Completed |
| T-012 | Config Debug and Validate Commands | Must Have | Medium (4-6hrs) | Completed |
| T-013 | Embedded Project Templates -- go-cli | Must Have | Medium (4-8hrs) | Not Started |
| T-014 | Init Command -- raven init [template] | Must Have | Medium (4-6hrs) | Not Started |
| T-015 | Git Client Wrapper -- internal/git/client.go | Must Have | Medium (6-10hrs) | Not Started |

**Deliverable:** `raven version`, `raven init go-cli`, and `raven config debug` work correctly.

---

### Phase 2: Task System & Agent Adapters (T-016 to T-030)

- **Status:** Not Started
- **Tasks:** 15 (14 Must Have, 1 Should Have)
- **Estimated Effort:** 110-175 hours
- **PRD Roadmap:** Weeks 3-4

#### Task List

| Task | Name | Priority | Effort | Status |
|------|------|----------|--------|--------|
| T-016 | Task Spec Markdown Parser | Must Have | Medium (6-10hrs) | Not Started |
| T-017 | Task State Management (task-state.conf) | Must Have | Medium (8-12hrs) | Not Started |
| T-018 | Phase Configuration Parser (phases.conf) | Must Have | Small (3-5hrs) | Not Started |
| T-019 | Dependency Resolution & Next-Task Selection | Must Have | Medium (8-12hrs) | Not Started |
| T-020 | Status Command -- raven status | Must Have | Medium (6-10hrs) | Not Started |
| T-021 | Agent Interface & Registry | Must Have | Medium (6-10hrs) | Not Started |
| T-022 | Claude Agent Adapter | Must Have | Medium (8-12hrs) | Not Started |
| T-023 | Codex Agent Adapter | Must Have | Medium (6-10hrs) | Not Started |
| T-024 | Gemini Agent Stub | Should Have | Small (2-3hrs) | Not Started |
| T-025 | Rate-Limit Detection & Coordination | Must Have | Medium (8-12hrs) | Not Started |
| T-026 | Prompt Template System | Must Have | Medium (6-10hrs) | Not Started |
| T-027 | Implementation Loop Runner | Must Have | Large (16-24hrs) | Not Started |
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

_Last updated: 2026-02-17_
