# Product Requirements Document: Raven

**Version:** 2.0
**Author:** Zizo
**Date:** February 17, 2026
**Status:** Draft -- Ready for Development Planning

---

## 1. Executive Summary

Raven is a Go-based AI workflow orchestration command center that manages the full lifecycle of AI-assisted software development -- from PRD decomposition to implementation, code review, fix application, and pull request creation. It orchestrates multiple AI agents (Claude, Codex, Gemini) through configurable workflow pipelines, providing both a headless CLI for automation and an interactive TUI dashboard for real-time monitoring and control.

What sets Raven apart from ad-hoc scripts and single-purpose agent wrappers is its **generic workflow engine**: a state-machine-based runner that can execute any multi-step workflow -- not just code implementation. Combined with **concurrent agent orchestration**, **TOML-driven project configuration**, **rate-limit-aware execution with automatic recovery**, and a **Bubble Tea command center TUI**, Raven transforms from a collection of bash scripts into a unified platform for AI-powered development automation.

Raven compiles to a single, zero-dependency Go binary that runs on macOS, Linux, and Windows. It operates in two modes: **headless CLI** (default, optimized for CI/CD and scripted automation) and **interactive TUI** (visual dashboard with split panes, live agent output, progress tracking, and workflow control).

**Tagline:** *Your AI command center.*

---

## 2. Problem Statement

Developers orchestrating AI coding agents face compounding friction as workflows grow more sophisticated.

**Current pain points:**

- **Fragile bash orchestration.** Multi-agent workflows built in bash are brittle -- poor error handling, no type safety, difficult subprocess management, and platform-dependent behavior (bash 3.2 vs 5.x, macOS vs Linux).
- **No visibility into concurrent agents.** When running multiple AI agents in parallel (review passes, PRD decomposition workers), developers have no unified view of what each agent is doing, its progress, or whether it's rate-limited.
- **One-size-fits-all workflows.** Current tools hardcode the implement-review-fix-PR pipeline. But developers need the same loop/worker pattern for documentation generation, test creation, PRD decomposition, migration scripts, and security audits.
- **Rate limits destroy throughput.** AI agents hit API rate limits frequently. Without intelligent backoff coordination across concurrent agents, one rate-limited agent can stall an entire pipeline while others sit idle.
- **No resumability.** When a pipeline fails at step 3 of 5, developers restart from scratch. There's no checkpoint/resume mechanism to pick up where the workflow left off.
- **Agent-specific knowledge is scattered.** Each AI agent (Claude, Codex, Gemini) has different CLI flags, output formats, rate-limit signals, and error patterns. This knowledge lives in ad-hoc scripts rather than a unified adapter layer.
- **PRD-to-tasks is manual and slow.** Decomposing a PRD into implementation tasks is done by a single agent in one pass. For large PRDs, this produces shallow task definitions. A concurrent scatter-gather approach (shred into epics, decompose in parallel, merge and deduplicate) would be faster and higher quality.
- **Configuration is hardcoded.** Project-specific settings (task directories, branch templates, verification commands, agent preferences, review patterns) are scattered across environment variables, script arguments, and hardcoded paths.

**The opportunity:** A single Go binary with a generic workflow engine, concurrent agent orchestration, and a TUI command center that serves as the developer's central hub for all AI-powered automation.

---

## 3. Target Audience

### 3.1 Primary Users

**Developers with multi-agent AI workflows** who orchestrate Claude, Codex, or Gemini for implementation, review, and automation tasks. They need a reliable orchestrator that handles rate limits, recovery, and concurrent execution.

- Technically proficient (comfortable with CLI tools, TOML configuration, git workflows)
- Work on projects with 10-100+ implementation tasks organized into phases
- Already use or want to use AI agents for code generation, review, and fix automation
- Value reproducibility and resumability in long-running workflows

**Solo developers and small teams** who want to adopt AI-assisted development with minimal setup. They need a tool that provides sensible defaults and progressive complexity.

- Range from intermediate to advanced developers
- May start with single-agent implementation and grow into multi-agent review pipelines
- Want a beautiful TUI that makes complex workflows approachable

### 3.2 User Personas

**Persona 1: Zizo (Power User / Pipeline Architect)**
Manages multiple Go and TypeScript projects with sophisticated multi-agent review systems. Runs 5 AI agents across 2 review passes with concurrent implementation loops. Needs Raven to orchestrate entire phase pipelines unattended, with the TUI dashboard available for monitoring. Also uses Raven for PRD decomposition with concurrent epic shredding. Wants every workflow to be configurable, resumable, and observable.

**Persona 2: Sam (Team Lead / CI Integrator)**
Manages a team adopting AI-assisted development. Wants Raven to run in CI for automated review and fix cycles on PRs. Needs headless mode, clean exit codes, and structured logging. Values the single binary distribution for easy CI setup. Uses `raven init` to bootstrap new projects with consistent configuration.

**Persona 3: Maya (Individual Developer)**
Uses Claude Code daily for implementation tasks. Wants a nicer workflow than running the same commands manually. Appreciates the TUI for watching agent progress, switching between tasks, and understanding what's happening. Starts with `raven implement --task T-007` and grows into full pipeline usage.

---

## 4. Product Objectives & Success Metrics

| Objective | Success Metric | Target |
|-----------|---------------|--------|
| Workflow generality | Number of distinct workflow types supported | >= 4 (implement, review, prd-decompose, pipeline) |
| Agent orchestration | Concurrent agents with live output multiplexing | 5+ simultaneous agents |
| Rate-limit resilience | Automatic recovery without user intervention | Zero manual restarts for rate limits |
| Resumability | Resume failed workflows from last checkpoint | < 5 seconds to resume |
| Configuration | Time to configure a new project | < 5 minutes with `raven init` |
| TUI responsiveness | Dashboard update latency with 5 agents | < 100ms per frame |
| Binary distribution | External dependencies (runtime) | Zero (pure Go) |
| Binary size | Compiled binary | < 25MB |
| Usability | Time from install to first `raven implement` | < 3 minutes |
| Extensibility | Time to add a new workflow type | < 1 hour with step handler pattern |

---

## 5. Feature Specifications

### 5.1 Generic Workflow Engine

**Priority:** Must-Have
**Description:** A state-machine-based workflow runner that executes configurable multi-step workflows with checkpoint/resume, conditional branching, and parallel step execution. This is the core abstraction that makes Raven generic -- any workflow (implementation, review, PRD decomposition, custom) is defined as a sequence of steps with transitions.

**User Story:** As a developer, I want to define custom workflows beyond just implement-review-fix-PR so that I can use Raven's orchestration engine for any multi-step AI task.

**Acceptance Criteria:**
- Workflows are defined as named state machines with steps and transitions
- Each step has a handler that implements the `StepHandler` interface:
  ```go
  type StepHandler interface {
      Execute(ctx context.Context, state *WorkflowState) (Event, error)
      DryRun(state *WorkflowState) string
      Name() string
  }
  ```
- Supported transition events: `success`, `failure`, `blocked`, `rate_limited`, `needs_human`, `partial`
- Workflow state is checkpointed to disk after every state transition (`$PROJECT_ROOT/.raven/state/<workflow-run-id>.json`)
- `raven resume [--run <id>]` continues from the last checkpoint
- `raven resume --list` shows all resumable workflow runs
- Built-in workflows (shipped as defaults, overridable via `raven.toml`):
  - `implement` -- single-task or phase-based implementation loop
  - `implement-review-pr` -- implement -> review -> fix cycles -> PR creation
  - `pipeline` -- multi-phase orchestrator (chains `implement-review-pr` per phase)
  - `prd-decompose` -- PRD -> epics -> parallel task decomposition -> merge
- Custom workflows can be defined in `raven.toml`:
  ```toml
  [workflows.my-custom]
  description = "Documentation generation"
  steps = ["scan_modules", "generate_docs", "review_docs", "commit"]

  [workflows.my-custom.transitions]
  scan_modules = { on_success = "generate_docs" }
  generate_docs = { on_success = "review_docs", on_error = "failed" }
  review_docs = { on_approved = "commit", on_changes = "generate_docs" }
  commit = { on_success = "done" }
  ```
- Workflow execution emits structured events for TUI consumption (`WorkflowEvent` channel)
- Supports `--dry-run` on any workflow to show planned steps without execution
- Supports `--step <name>` to run a single step in isolation (useful for debugging)
- Step handlers can declare parallel execution (`Parallel: true`) for fan-out patterns

**Technical Considerations:**
- Implement as a lightweight state machine -- no external framework (Temporal/Prefect are overkill for single-machine CLI)
- State checkpoint format: JSON with workflow ID, current step, step history (timestamps, events, durations), metadata map
- Use `context.Context` for cancellation propagation across all steps
- The workflow engine is a Go library (`internal/workflow/`) that the CLI and TUI both consume
- Step handlers are registered in a global registry; built-in handlers are registered at init, custom handlers loaded from plugins (v2.1)
- Workflow definitions in TOML are validated at load time (unknown steps, unreachable states, cycles)

---

### 5.2 Agent Adapter System

**Priority:** Must-Have
**Description:** A pluggable adapter layer that abstracts AI agent differences (CLI flags, output formats, rate-limit signals, error patterns) behind a common interface. Each adapter handles the full lifecycle of invoking its agent: prerequisite checking, command construction, output parsing, and rate-limit detection.

**User Story:** As a developer, I want to switch between Claude, Codex, and Gemini without changing my workflow configuration so that I can use the best agent for each task type.

**Acceptance Criteria:**
- All agents implement the `Agent` interface:
  ```go
  type Agent interface {
      Name() string
      Run(ctx context.Context, opts RunOpts) (*RunResult, error)
      CheckPrerequisites() error
      ParseRateLimit(output string) (*RateLimitInfo, bool)
      DryRunCommand(opts RunOpts) string
  }
  ```
- `RunOpts` includes: prompt (string or file path), model, effort/reasoning level, allowed tools, output format, working directory, environment variables
- `RunResult` includes: stdout, stderr, exit code, duration, token usage (if available), rate-limit info
- Built-in adapters:
  - **Claude** (`claude` CLI): Supports `--model`, `--permission-mode`, `--allowedTools`, `--append-system-prompt`, `--output-format json`, effort via `CLAUDE_CODE_EFFORT_LEVEL` env var. Parses rate-limit messages ("Your rate limit will reset", "Too many requests") and extracts reset times.
  - **Codex** (`codex` CLI): Supports `exec` mode with `--sandbox`, `-a never`, `--ephemeral`, `--model`. Parses "try again in X days Y minutes" rate-limit messages.
  - **Gemini** (`gemini` CLI): Adapter stub for future integration. Implements the interface with `ErrNotImplemented` for unsupported features.
- Agent selection via `--agent <name>` flag or `raven.toml` config per workflow
- Agent configuration in `raven.toml`:
  ```toml
  [agents.claude]
  command = "claude"
  model = "claude-opus-4-6"
  effort = "high"
  prompt_template = "prompts/implement-claude.md"
  allowed_tools = "Edit,Write,Read,Glob,Grep,Bash(go*),Bash(git*)"

  [agents.codex]
  command = "codex"
  model = "gpt-5.3-codex"
  effort = "high"
  prompt_template = "prompts/implement-codex.md"
  ```
- Agent output is streamed in real-time to both log files and TUI panels
- Rate-limit coordination: when one agent hits a rate limit, all agents for the same provider pause (shared rate-limit state)

**Technical Considerations:**
- Use `os/exec` for subprocess management with `StdoutPipe()` and `StderrPipe()` for real-time streaming
- Each agent runs in a goroutine that sends `AgentEvent` messages (output lines, status changes, rate-limit signals) to a channel consumed by the TUI
- Rate-limit parsing uses compiled regexes specific to each agent's known error formats
- Agent commands are constructed from config + runtime overrides (CLI flags always win)
- Test agents with mock executables that simulate rate limits, errors, and structured output

---

### 5.3 Task Management System

**Priority:** Must-Have
**Description:** A system for discovering, tracking, and selecting tasks from markdown-based task specifications. Supports dependency resolution, phase-based grouping, and multiple task state tracking.

**User Story:** As a developer, I want Raven to automatically pick the next available task based on dependencies and completion status so that I can run `raven implement --phase 2` and have it work through all phase 2 tasks in order.

**Acceptance Criteria:**
- Tasks are defined as markdown files (`T-XXX-description.md`) in a configurable tasks directory
- Task state is tracked in a pipe-delimited state file (`task-state.conf`) with columns: `task_id | status | agent | timestamp | notes`
- Supported task statuses: `not_started`, `in_progress`, `completed`, `blocked`, `skipped`
- Dependencies are declared in task spec files via `**Dependencies:** T-001, T-003` header
- `select_next_task(phase_range)` returns the first `not_started` task whose dependencies are all `completed`
- Phase configuration via `phases.conf`:
  ```
  1|Foundation & Setup|T-001|T-010
  2|Core Implementation|T-011|T-020
  ```
- `raven status` shows phase-by-phase progress with progress bars
- `raven implement --task T-007` runs a single specific task
- `raven implement --phase 2` runs all tasks in phase 2 sequentially
- `raven implement --phase all` runs all phases sequentially
- Progress tracking: `PROGRESS.md` updated after each task completion
- Task selection supports parallel execution of independent tasks within a phase (tasks with no interdependency can run concurrently with `--parallel`)

**Technical Considerations:**
- Parse task specs with a simple markdown parser (regex for `**Dependencies:**` line, `# T-XXX:` title)
- Task state file uses file locking (`flock`) for concurrent access safety
- Phase ranges are loaded from `phases.conf` into a `[]Phase` struct
- Dependency resolution is a simple topological check (not a full sort) -- check that all dependencies are `completed` before selecting a task
- For parallel task execution, build a dependency graph and identify independent sets that can run concurrently

---

### 5.4 Implementation Loop Engine

**Priority:** Must-Have
**Description:** The core loop that picks tasks, generates prompts, invokes agents, handles rate limits and errors, detects completion signals, and commits results. This is the workhorse of Raven -- the existing `run_raven_loop` logic from the bash prototype, reimplemented in Go with proper error handling and concurrency.

**User Story:** As a developer, I want to run `raven implement --agent claude --phase 1` and have it work through all phase 1 tasks autonomously, handling rate limits and errors without my intervention.

**Acceptance Criteria:**
- Iterates through tasks in a phase (or a single task), running the agent for each
- Prompt generation with template placeholders:
  - `{{TASK_SPEC}}` -- contents of the task markdown file
  - `{{PHASE_INFO}}` -- current phase name and range
  - `{{PROJECT_NAME}}` -- from `raven.toml`
  - `{{VERIFICATION_COMMANDS}}` -- from `raven.toml`
  - `{{COMPLETED_TASKS}}` -- list of already-completed tasks for context
  - `{{REMAINING_TASKS}}` -- list of remaining tasks
- Rate-limit detection and recovery:
  - Detects rate-limit signals in agent output (provider-specific patterns)
  - Computes wait time from agent output or uses configurable default backoff
  - Displays countdown timer during wait
  - Automatically retries after rate-limit reset
  - Configurable `--max-limit-waits <n>` to cap wait cycles before aborting
- Error recovery:
  - Dirty-tree detection (uncommitted changes after agent run)
  - Auto-stash and recovery on unexpected state
  - Configurable `--max-iterations <n>` to cap total loop iterations
- Completion detection:
  - Scans agent output for `PHASE_COMPLETE`, `TASK_BLOCKED`, `RAVEN_ERROR` signals
  - Checks if task state changed (new commits, state file updates)
  - Detects when no progress is being made (same task selected N times in a row)
- `--dry-run` mode: generates and displays the prompt without invoking the agent
- Emits `LoopEvent` messages for TUI consumption (iteration count, task selected, agent status, wait timers)
- Configurable sleep between iterations (`--sleep <seconds>`, default: 5)

**Technical Considerations:**
- The loop is a `LoopRunner` struct with methods for each phase of the iteration
- Rate-limit wait uses `time.Timer` with cancellation via `context.Context` (so Ctrl+C during wait works)
- Git operations (commit check, stash, recovery) use `os/exec` calling `git` CLI (same approach as `gh` and `lazygit`)
- Loop state (current iteration, current task, rate-limit count) is persisted as part of workflow checkpoint
- The loop engine is generic -- it takes a `TaskSelector`, `PromptGenerator`, and `Agent` as dependencies (strategy pattern)

---

### 5.5 Multi-Agent Review Pipeline

**Priority:** Must-Have
**Description:** A parallel multi-agent code review system that generates diffs, fans out review requests to multiple agents concurrently, collects structured JSON findings, consolidates and deduplicates results, and produces a unified review report.

**User Story:** As a developer, I want to run `raven review --agents claude,codex --concurrency 4` and get a consolidated review report from multiple AI agents reviewing my changes in parallel.

**Acceptance Criteria:**
- Generates git diff (changed files, risk classification)
- Fans out review requests to N agents concurrently with configurable concurrency limit
- Each review agent produces structured JSON findings:
  ```json
  {
    "findings": [
      {
        "severity": "high",
        "category": "security",
        "file": "internal/auth/handler.go",
        "line": 42,
        "description": "...",
        "suggestion": "..."
      }
    ],
    "verdict": "APPROVED" | "CHANGES_NEEDED" | "BLOCKING"
  }
  ```
- JSON extraction from freeform agent output using a robust extractor (handles markdown fencing, partial output)
- Consolidation: deduplicates findings by file+line+category composite key, escalates severity on duplicates
- Produces a unified review report (markdown)
- Review verdict aggregation: if any agent says `BLOCKING`, final verdict is `BLOCKING`
- Supports review modes: `all` (all agents review everything), `split` (each agent reviews different files)
- Configuration in `raven.toml`:
  ```toml
  [review]
  extensions = '(\.go$|\.ts$|\.py$)'
  risk_patterns = '^(cmd/|internal/|lib/)'
  prompts_dir = ".github/review/prompts"
  rules_dir = ".github/review/rules"
  project_brief_file = ".github/review/PROJECT_BRIEF.md"
  ```

**Technical Considerations:**
- Use `errgroup.Group` with `SetLimit(concurrency)` for bounded parallel review execution
- Each review goroutine captures agent output to a file, extracts JSON, and sends results to a collector channel
- JSON extraction: port the existing `json-extract.js` logic to Go (regex-based candidate extraction, `encoding/json` validation)
- Consolidation uses a `map[string]*Finding` keyed by `file:line:category` for O(n) deduplication
- Review prompts are loaded from template files with project-specific context injection

---

### 5.6 Review Fix Engine

**Priority:** Must-Have
**Description:** An automated fix engine that takes review findings and applies fixes using an AI agent, then runs verification commands to confirm the fixes are correct.

**User Story:** As a developer, I want Raven to automatically apply review fixes and verify they don't break anything so that the review-fix cycle is fully automated.

**Acceptance Criteria:**
- Takes consolidated review findings as input
- Generates a fix prompt with: findings, affected file contents, project conventions, verification commands
- Invokes an agent to apply fixes
- Runs configurable verification commands (`go build ./...`, `go test ./...`, etc.)
- Reports verification results (pass/fail per command)
- Supports `--max-fix-cycles <n>` for iterative fix-review loops
- Dry-run mode shows the fix prompt without executing

**Technical Considerations:**
- Verification commands from `raven.toml` (`project.verification_commands` array)
- Each verification command runs via `os/exec` with timeout
- Fix prompt includes the git diff of what changed, so the agent knows what to fix

---

### 5.7 PR Creation

**Priority:** Must-Have
**Description:** Automated pull request creation with AI-generated descriptions that summarize all implementation work, review findings, and fix cycles.

**User Story:** As a developer, I want Raven to create a well-documented PR after completing a phase so that the review history is captured in the PR description.

**Acceptance Criteria:**
- Creates a GitHub PR using `gh pr create`
- AI-generated PR body includes: summary of changes, tasks completed, review findings resolved, verification results
- Supports PR template integration (`.github/PULL_REQUEST_TEMPLATE.md`)
- Configurable base branch (default: `main`)
- Supports `--draft` flag for draft PRs
- Dry-run mode shows the PR body without creating the PR

**Technical Considerations:**
- Use `os/exec` to call `gh` CLI (well-established pattern, avoids GitHub API authentication complexity)
- PR body generation via agent call with structured prompt

---

### 5.8 PRD Decomposition Workflow

**Priority:** Must-Have
**Description:** A three-phase map-reduce workflow that decomposes a Product Requirements Document into implementation tasks using concurrent AI agents. This is the first non-implementation workflow, proving Raven's generic workflow engine.

**User Story:** As a developer, I want to run `raven prd --file docs/prd/PRD.md --concurrent --concurrency 3` and get a complete set of task files with dependencies, phases, and progress tracking.

**Acceptance Criteria:**
- **Phase 1 -- Shred (single agent call):** Reads the PRD and produces an epic-level breakdown as structured JSON:
  ```json
  {
    "epics": [
      {
        "id": "E-001",
        "title": "Authentication System",
        "description": "...",
        "prd_sections": ["Section 3.1", "Section 3.2"],
        "estimated_task_count": 8,
        "dependencies_on_epics": []
      }
    ]
  }
  ```
- **Phase 2 -- Scatter (N concurrent agent calls):** For each epic, spawns a worker agent with the PRD context and epic definition. Each worker produces per-epic task JSON:
  ```json
  {
    "epic_id": "E-001",
    "tasks": [
      {
        "temp_id": "E001-T01",
        "title": "Set up authentication middleware",
        "description": "...",
        "acceptance_criteria": ["..."],
        "local_dependencies": ["E001-T02"],
        "cross_epic_dependencies": ["E-003:database-schema"],
        "effort": "medium",
        "priority": "must-have"
      }
    ]
  }
  ```
- **Phase 3 -- Gather (deterministic, no LLM):** Merges all per-epic task JSON:
  1. Assigns global sequential IDs (`T-001`, `T-002`, ...) across epics, ordered by epic dependency
  2. Remaps local and cross-epic dependencies to global IDs
  3. Deduplicates by title similarity (normalized string comparison)
  4. Validates the dependency graph is a DAG (using topological sort; cycle detection)
  5. Auto-generates `phases.conf` from topological depth
  6. Emits: `T-XXX-description.md` files, `task-state.conf`, `phases.conf`, `PROGRESS.md`, `INDEX.md`
- Rate-limit handling: workers that hit rate limits wait and retry (reusing implementation loop's rate-limit machinery)
- Progress reporting: real-time status of each worker (pending, running, completed, failed)
- `--single-pass` flag for non-concurrent mode (entire PRD in one agent call, for small PRDs)
- `--output-dir <path>` for where to write task files (default: from `raven.toml` `project.tasks_dir`)
- JSON schema enforcement: workers write structured JSON to files (`$WORK_DIR/epic-NNN.json`), not stdout
- Retry on malformed output: up to 3 retries per worker with augmented prompt

**Technical Considerations:**
- Use `errgroup.Group` with `SetLimit(concurrency)` for bounded parallel worker execution (same pattern as review pipeline)
- Each worker gets: the full PRD (for context), its epic definition, a summary of all other epics (for cross-referencing), and a JSON schema
- Merge phase is pure Go -- no LLM calls. Uses deterministic algorithms for renumbering and deduplication
- DAG validation uses Go stdlib `sort` for topological ordering (Kahn's algorithm)
- Task file generation uses Go `text/template` for consistent formatting
- The "write to file" pattern (agents write JSON to a file rather than stdout) maximizes reliability across agent types

---

### 5.9 Phase Pipeline Orchestrator

**Priority:** Must-Have
**Description:** A higher-level orchestrator that chains the implementation, review, fix, and PR workflows across multiple phases. Manages branch creation, phase sequencing, and multi-phase state.

**User Story:** As a developer, I want to run `raven pipeline --phase all` and have it work through all phases end-to-end: creating branches, implementing tasks, running reviews, applying fixes, and creating PRs for each phase.

**Acceptance Criteria:**
- Orchestrates the full lifecycle per phase: bootstrap branch -> implement -> review/fix cycles -> create PR
- Supports: `--phase <id>`, `--phase all`, `--from-phase <id>`
- Branch management: creates branches from a configurable template (`phase/{phase_id}-{slug}`)
- Phase chaining: in multi-phase mode, each phase's branch is based on the previous phase's branch
- Skip flags: `--skip-implement`, `--skip-review`, `--skip-fix`, `--skip-pr`
- Agent selection per stage: `--impl-agent claude`, `--review-agent codex`, `--fix-agent claude`
- Review concurrency: `--review-concurrency <n>`
- Maximum review/fix cycles: `--max-review-cycles <n>`
- Pipeline metadata tracking: implementation status, review verdict, fix status, PR URL per phase
- Interactive wizard mode (TUI): when `--interactive` or no flags in a terminal, launches a guided wizard for pipeline configuration
- Dry-run mode: shows planned pipeline steps without execution
- Resumability: pipeline state is checkpointed per-phase, resume picks up at the failed phase

**Technical Considerations:**
- Pipeline orchestration is itself a workflow (using the generic workflow engine)
- Each pipeline phase is a sub-workflow that runs the `implement-review-pr` workflow with phase-specific parameters
- Branch operations use `os/exec` calling `git` CLI
- Pipeline state stored in `.raven/state/pipeline-<run-id>.json`
- The interactive wizard uses `charmbracelet/huh` for form-based configuration

---

### 5.10 TOML Configuration System

**Priority:** Must-Have
**Description:** A comprehensive TOML-based configuration system that drives all of Raven's behavior, with sensible defaults and progressive disclosure.

**User Story:** As a developer, I want to run `raven init go-cli` to get a working configuration for my Go project and then customize it as needed.

**Acceptance Criteria:**
- Configuration file: `raven.toml` at project root
- Auto-detection: Raven walks up from CWD to find `raven.toml`
- Configuration sections:
  ```toml
  [project]
  name = "my-project"
  language = "go"
  tasks_dir = "docs/tasks"
  task_state_file = "docs/tasks/task-state.conf"
  phases_conf = "docs/tasks/phases.conf"
  progress_file = "docs/tasks/PROGRESS.md"
  log_dir = "scripts/logs"
  prompt_dir = "prompts"
  branch_template = "phase/{phase_id}-{slug}"
  verification_commands = ["go build ./...", "go test ./...", "go vet ./..."]

  [agents.claude]
  command = "claude"
  model = "claude-opus-4-6"
  effort = "high"
  prompt_template = "prompts/implement-claude.md"
  allowed_tools = "Edit,Write,Read,Glob,Grep,Bash(go*),Bash(git*)"

  [agents.codex]
  command = "codex"
  model = "gpt-5.3-codex"
  effort = "high"
  prompt_template = "prompts/implement-codex.md"

  [review]
  extensions = '(\.go$|go\.mod$|go\.sum$)'
  risk_patterns = '^(cmd/|internal/|scripts/)'
  prompts_dir = ".github/review/prompts"
  rules_dir = ".github/review/rules"
  project_brief_file = ".github/review/PROJECT_BRIEF.md"

  [workflows.implement-review-pr]
  # ... custom workflow definitions
  ```
- CLI flags always override config file values
- Environment variable overrides with `RAVEN_` prefix (`RAVEN_PROJECT_NAME`, `RAVEN_LOG_DIR`, etc.)
- `raven init [template]` scaffolds a new project:
  - `go-cli` -- Go CLI project template
  - `node-service` -- Node.js service template (v2.1)
  - `python-lib` -- Python library template (v2.1)
  - More templates added over time
- `raven config debug` shows resolved configuration with source annotations (which value came from CLI vs config vs default)
- `raven config validate` validates the config and warns on issues
- Config validation at load time with clear error messages

**Technical Considerations:**
- Use `BurntSushi/toml` v1.5.0 for TOML parsing -- simpler API than `pelletier/go-toml`, read-only use case
- Use `MetaData.Undecoded()` to detect unknown keys in config (typo detection)
- Config structs use Go struct tags: `toml:"field_name"`
- Config resolution order: CLI flags > env vars > `raven.toml` > defaults
- Config auto-detection walks up directories from CWD (same pattern as `config-lib.sh` in bash prototype)
- Template files are embedded in the binary using `//go:embed`

---

### 5.11 CLI Interface

**Priority:** Must-Have
**Description:** A clean, intuitive CLI with subcommands for every Raven capability, comprehensive help text, and shell completions.

**User Story:** As a developer, I want to run `raven help` and immediately understand all available commands, then run any command with `--help` for detailed usage.

**Acceptance Criteria:**
- **Core commands:**
  - `raven implement --agent <name> --phase <id>` -- Run implementation loop
  - `raven implement --agent <name> --task <id>` -- Run single task
  - `raven review --agents <list> --concurrency <n>` -- Run multi-agent review
  - `raven fix --agent <name>` -- Apply review fixes
  - `raven pr` -- Create pull request
  - `raven pipeline --phase <id>` -- Run full pipeline
  - `raven pipeline --interactive` -- Launch pipeline wizard
  - `raven prd --file <path>` -- Decompose PRD into tasks
  - `raven status` -- Show task/phase progress
  - `raven resume [--run <id>]` -- Resume interrupted workflow
  - `raven init [template]` -- Initialize project
- **Configuration commands:**
  - `raven config debug` -- Show resolved config
  - `raven config validate` -- Validate config
- **Utility commands:**
  - `raven version [--json]` -- Show version and build info
  - `raven completion <shell>` -- Generate shell completions (bash, zsh, fish, powershell)
  - `raven dashboard` -- Launch TUI command center
  - `raven help` -- Show help
- **Global flags:**
  - `--verbose / -v` -- Verbose output (debug level)
  - `--quiet / -q` -- Suppress all output except errors
  - `--config <path>` -- Explicit config file path
  - `--dir <path>` -- Override working directory
  - `--dry-run` -- Show planned actions without executing
  - `--no-color` -- Disable colored output
- Exit codes: 0 (success), 1 (error), 2 (partial success), 3 (user-cancelled)
- All progress/status goes to stderr; structured output goes to stdout (piping-friendly)
- Shell completions: intelligent completions for `--agent <TAB>`, `--phase <TAB>`, etc.

**Technical Considerations:**
- Use `spf13/cobra` v1.10+ for CLI framework (same as `gh`, `kubectl`, `docker`)
- Use `charmbracelet/lipgloss` for terminal styling (auto-disabled when piped)
- Use `charmbracelet/log` for pretty structured logging
- Environment variable overrides: `RAVEN_VERBOSE`, `RAVEN_QUIET`, `RAVEN_NO_COLOR`
- Progress output uses stderr exclusively so stdout is clean for piping

---

### 5.12 Interactive TUI Command Center

**Priority:** Must-Have
**Description:** A Bubble Tea-based terminal user interface that provides a real-time dashboard for monitoring and controlling Raven workflows. The TUI shows split panes with agent output streams, task progress, workflow state, and rate-limit status. This is Raven's signature feature -- the "command center" experience.

**User Story:** As a developer, I want to launch `raven dashboard` and see a live view of all running agents, their output, task progress, and rate-limit status in a single terminal window.

**Acceptance Criteria:**
- Launched via `raven dashboard` or `raven pipeline --interactive`
- **Layout (split panes):**
  ```
  ┌─────────────────────────────────────────────────┐
  │  Raven Command Center            v2.0  [Ctrl+Q] │
  ├──────────────┬──────────────────────────────────┤
  │ Workflows    │ [Active Agent Panel]              │
  │ ● pipeline   │ Agent: claude (implement)         │
  │ ○ review     │ Task: T-007                       │
  │              │ > Working on auth middleware...    │
  │ Tasks        │ > Modified internal/auth/...      │
  │ ████░░ 40%   │ > Committing changes...           │
  │ 12/30 done   │                                   │
  │              │──────────────────────────────────│
  │ Phase: 2/5   │ [Agent Log / Event Stream]        │
  │ ██░░░░ 28%   │ 14:23:01 Agent started T-007      │
  │              │ 14:23:45 Rate limit detected       │
  │ Rate Limits  │ 14:23:45 Waiting 120s...           │
  │ claude: OK   │ 14:25:45 Retrying...               │
  │ codex: WAIT  │ 14:26:02 T-007 completed           │
  ├──────────────┴──────────────────────────────────┤
  │ [Status Bar] Phase 2 | Task T-007 | Iter 3/20   │
  └─────────────────────────────────────────────────┘
  ```
- **Left panel (sidebar):**
  - Active workflows with status indicators
  - Task progress bar with completion count
  - Phase progress
  - Rate-limit status per provider (OK, WAITING with countdown)
- **Right panel (main area):**
  - Tabbed view: switch between active agents (Tab key)
  - Live-scrolling agent output (viewport with scroll)
  - Event stream showing workflow milestones
- **Controls:**
  - `Tab` / `Shift+Tab` -- Switch between agent panels
  - `p` -- Pause/resume workflow
  - `s` -- Skip current task
  - `q` / `Ctrl+C` -- Graceful shutdown (finish current agent call, checkpoint state)
  - `?` -- Help overlay
  - `l` -- Toggle log panel
  - Arrow keys / `j`/`k` -- Scroll agent output
- **Responsive layout:** adapts to terminal size (min 80x24)
- **Live updates:** TUI consumes events from the workflow engine and agent adapters via Go channels
- **Concurrent agent display:** when multiple agents run in parallel (review), show all panels simultaneously
- **Startup:** can launch existing workflows or start new ones from the TUI

**Technical Considerations:**
- Use `charmbracelet/bubbletea` v1.2+ for TUI framework (Elm architecture)
- Use `charmbracelet/lipgloss` for declarative styling
- Use `charmbracelet/bubbles` for reusable components: viewport (scrolling), spinner, progress bar, table
- Use `charmbracelet/huh` for the pipeline wizard form
- TUI consumes `tea.Msg` from goroutines via `p.Send()`:
  ```go
  // Agent output goroutine
  go func() {
      scanner := bufio.NewScanner(stdout)
      for scanner.Scan() {
          p.Send(AgentOutputMsg{Agent: "claude", Line: scanner.Text()})
      }
  }()
  ```
- The Elm architecture serializes all state updates through `Update()`, preventing race conditions with concurrent agents
- Agent output buffers are capped (last 1000 lines per agent) to prevent memory growth
- TUI renders the full screen as a string (Bubble Tea's model) -- for 5+ agents with moderate update rates, this is performant
- The TUI is a presentation layer only -- it calls the same workflow engine and agent adapters as the CLI

---

### 5.13 Git Integration

**Priority:** Must-Have
**Description:** Git operations for branch management, commit detection, dirty-tree recovery, and diff generation.

**User Story:** As a developer, I want Raven to manage git branches automatically during pipeline execution so that each phase's work is cleanly isolated.

**Acceptance Criteria:**
- Branch creation from configurable templates (`phase/{phase_id}-{slug}`)
- Branch chaining: each phase branches from the previous phase's branch
- Dirty-tree detection and auto-stash recovery
- Commit detection: detect when an agent has made commits (HEAD moved)
- Diff generation for review pipeline (changed files, risk classification)
- Support for `--base <branch>` to specify base branch (default: main)
- `--sync-base` flag to fetch and fast-forward base from origin

**Technical Considerations:**
- All git operations via `os/exec` calling the `git` CLI (same pattern as `gh`, `lazygit`)
- Git operations are wrapped in a `GitClient` struct with methods: `CreateBranch`, `CurrentBranch`, `HasUncommittedChanges`, `Stash`, `StashPop`, `DiffFiles`, `Log`
- Context-aware: all git commands accept `context.Context` for cancellation

---

## 6. Technical Architecture

### 6.1 Recommended Stack

| Layer | Technology | Version | Rationale |
|-------|------------|---------|-----------|
| Language | Go | 1.24+ | Single binary, goroutine parallelism, mature CLI ecosystem |
| CLI Framework | spf13/cobra | v1.10+ | Industry standard (gh, kubectl, docker) |
| TUI Framework | charmbracelet/bubbletea | v1.2+ | Best Go TUI ecosystem, Elm architecture for concurrent state |
| TUI Styling | charmbracelet/lipgloss | v1.0+ | Declarative terminal styling, auto-detection |
| TUI Components | charmbracelet/bubbles | latest | Viewport, spinner, progress, table, text input |
| TUI Forms | charmbracelet/huh | v0.6+ | Form/wizard builder (replaces gum-based wizard) |
| Logging | charmbracelet/log | latest | Pretty structured logging for terminal |
| TOML Parsing | BurntSushi/toml | v1.5.0 | Simpler API, read-only use case, MetaData.Undecoded() |
| Concurrency | golang.org/x/sync | v0.19+ | errgroup for bounded parallel execution |
| Glob Matching | bmatcuk/doublestar | v4.10+ | Doublestar glob patterns for file matching |
| Testing | stretchr/testify | v1.9+ | Assertions and test suites |
| Hashing | cespare/xxhash | v2 | Fast content hashing for state comparison |

### 6.2 Project Structure

```
raven/
├── cmd/
│   └── raven/
│       └── main.go                    # Entry point
├── internal/
│   ├── cli/
│   │   ├── root.go                    # Root command, global flags
│   │   ├── implement.go               # raven implement
│   │   ├── review.go                  # raven review
│   │   ├── fix.go                     # raven fix
│   │   ├── pr.go                      # raven pr
│   │   ├── pipeline.go                # raven pipeline
│   │   ├── prd.go                     # raven prd
│   │   ├── status.go                  # raven status
│   │   ├── resume.go                  # raven resume
│   │   ├── init_cmd.go                # raven init
│   │   ├── config_cmd.go              # raven config debug/validate
│   │   ├── dashboard.go               # raven dashboard (launches TUI)
│   │   ├── version.go                 # raven version
│   │   └── completion.go              # raven completion
│   ├── config/
│   │   ├── config.go                  # Configuration types and loading
│   │   ├── defaults.go                # Built-in default values
│   │   ├── resolve.go                 # Config resolution (CLI > env > file > defaults)
│   │   ├── validate.go                # Configuration validation
│   │   └── templates.go               # Embedded project templates
│   ├── workflow/
│   │   ├── engine.go                  # State machine workflow runner
│   │   ├── state.go                   # WorkflowState, checkpoint persistence
│   │   ├── registry.go                # Step handler registry
│   │   ├── events.go                  # WorkflowEvent types for TUI
│   │   └── builtin.go                 # Built-in workflow definitions
│   ├── agent/
│   │   ├── agent.go                   # Agent interface and RunOpts/RunResult types
│   │   ├── claude.go                  # Claude CLI adapter
│   │   ├── codex.go                   # Codex CLI adapter
│   │   ├── gemini.go                  # Gemini adapter stub
│   │   ├── ratelimit.go               # Shared rate-limit detection and coordination
│   │   └── mock.go                    # Mock agent for testing
│   ├── task/
│   │   ├── state.go                   # Task state management (task-state.conf)
│   │   ├── selector.go                # Next-task selection with dependency resolution
│   │   ├── parser.go                  # Task spec markdown parser
│   │   ├── phases.go                  # Phase configuration (phases.conf)
│   │   └── progress.go                # PROGRESS.md generation
│   ├── loop/
│   │   ├── runner.go                  # Implementation loop engine
│   │   ├── prompt.go                  # Prompt template generation
│   │   ├── recovery.go                # Rate-limit wait, dirty-tree recovery
│   │   └── events.go                  # LoopEvent types for TUI
│   ├── review/
│   │   ├── orchestrator.go            # Multi-agent parallel review
│   │   ├── diff.go                    # Git diff generation and risk classification
│   │   ├── prompt.go                  # Review prompt synthesis
│   │   ├── extract.go                 # JSON extraction from agent output
│   │   ├── consolidate.go             # Finding deduplication and consolidation
│   │   ├── report.go                  # Review report generation
│   │   └── fix.go                     # Review fix engine
│   ├── prd/
│   │   ├── decomposer.go             # PRD decomposition orchestrator
│   │   ├── shredder.go                # Phase 1: PRD -> epic JSON
│   │   ├── worker.go                  # Phase 2: epic -> tasks (parallel workers)
│   │   ├── merger.go                  # Phase 3: merge, dedup, renumber, DAG validation
│   │   └── emitter.go                 # Task file generation (T-XXX.md, phases.conf, etc.)
│   ├── pipeline/
│   │   ├── orchestrator.go            # Phase pipeline orchestration
│   │   ├── branch.go                  # Git branch management
│   │   ├── wizard.go                  # Interactive pipeline wizard (huh forms)
│   │   └── metadata.go                # Pipeline metadata tracking
│   ├── git/
│   │   ├── client.go                  # Git operations wrapper
│   │   └── recovery.go                # Stash, dirty-tree recovery
│   ├── tui/
│   │   ├── app.go                     # Bubble Tea application model
│   │   ├── layout.go                  # Split-pane layout management
│   │   ├── sidebar.go                 # Left panel (workflows, tasks, rate limits)
│   │   ├── agent_panel.go             # Agent output viewport
│   │   ├── event_log.go               # Event stream panel
│   │   ├── status_bar.go              # Bottom status bar
│   │   ├── wizard.go                  # Pipeline wizard TUI
│   │   ├── styles.go                  # Lipgloss styles and themes
│   │   └── keybindings.go             # Keyboard shortcuts
│   └── buildinfo/
│       └── buildinfo.go               # Version, commit, build date (ldflags)
├── templates/                          # Embedded project templates
│   └── go-cli/
│       ├── raven.toml
│       ├── prompts/
│       ├── .github/review/
│       └── docs/tasks/
├── testdata/                           # Test fixtures
│   ├── sample-project/
│   ├── task-specs/
│   └── review-fixtures/
├── .goreleaser.yml                     # Cross-platform release builds
├── go.mod
├── go.sum
├── Makefile
└── LICENSE
```

### 6.3 Concurrency Model

Raven uses Go's goroutines and channels as the primary concurrency mechanism, with `errgroup` for bounded parallel execution.

```
Workflow Engine (main goroutine)
       │
       ├── Step: Implementation Loop
       │       │
       │       └── Agent goroutine ──► stdout/stderr pipes ──► TUI channel
       │
       ├── Step: Multi-Agent Review
       │       │
       │       ├── Review Worker 1 ──► Agent goroutine ──► TUI channel
       │       ├── Review Worker 2 ──► Agent goroutine ──► TUI channel
       │       └── Review Worker N ──► Agent goroutine ──► TUI channel
       │       (errgroup with SetLimit(concurrency))
       │
       └── TUI Event Loop (Elm architecture)
               │
               ├── Receives: AgentOutputMsg, WorkflowEventMsg, LoopEventMsg
               ├── Update(): serializes state mutations (no races)
               └── View(): renders full screen from state
```

All long-running operations accept `context.Context` for cancellation. The TUI's `tea.Program` is the event loop that consumes all messages.

### 6.4 Central Data Types

```go
// Workflow types
type WorkflowState struct {
    ID            string                 `json:"id"`
    WorkflowName  string                 `json:"workflow_name"`
    CurrentStep   string                 `json:"current_step"`
    StepHistory   []StepRecord           `json:"step_history"`
    Metadata      map[string]interface{} `json:"metadata"`
    CreatedAt     time.Time              `json:"created_at"`
    UpdatedAt     time.Time              `json:"updated_at"`
}

type StepRecord struct {
    Step      string        `json:"step"`
    Event     string        `json:"event"`
    StartedAt time.Time     `json:"started_at"`
    Duration  time.Duration `json:"duration"`
    Error     string        `json:"error,omitempty"`
}

// Agent types
type RunOpts struct {
    Prompt       string
    PromptFile   string
    Model        string
    Effort       string
    AllowedTools string
    OutputFormat string
    WorkDir      string
    Env          []string
}

type RunResult struct {
    Stdout    string
    Stderr    string
    ExitCode  int
    Duration  time.Duration
    RateLimit *RateLimitInfo
}

type RateLimitInfo struct {
    IsLimited  bool
    ResetAfter time.Duration
    Message    string
}

// Task types
type Task struct {
    ID           string   `json:"id"`
    Title        string   `json:"title"`
    Status       string   `json:"status"`
    Phase        int      `json:"phase"`
    Dependencies []string `json:"dependencies"`
    SpecFile     string   `json:"spec_file"`
}

type Phase struct {
    ID        int    `json:"id"`
    Name      string `json:"name"`
    StartTask string `json:"start_task"`
    EndTask   string `json:"end_task"`
}
```

### 6.5 Logging & Diagnostics

- Use `charmbracelet/log` for pretty terminal output with component prefixes
- Log levels: `debug` (`--verbose`), `info` (default), `warn`, `error`, `fatal` (`--quiet` shows only `error`+`fatal`)
- `RAVEN_LOG_FORMAT=json` enables JSON-structured logs for CI
- `RAVEN_DEBUG=1` dumps: resolved config, per-step timing, agent command lines, rate-limit events
- All logs go to stderr; only structured output (status, PR URL) goes to stdout

---

## 7. Development Roadmap

### Phase 1: Foundation (Weeks 1-2)

**Goal:** Working Go binary with CLI framework, configuration system, and project initialization.

- Go project scaffolding (modules, Cobra CLI, Makefile, structured logging)
- Central data types (`WorkflowState`, `RunOpts`, `RunResult`, `Task`, `Phase`)
- TOML configuration loading with `BurntSushi/toml`
- Config resolution (CLI > env > file > defaults) and validation
- `raven init go-cli` with embedded templates
- `raven version`, `raven help`, `raven config debug`
- Shell completions (bash, zsh, fish, powershell)
- Unit tests for config loading, validation, template embedding

**Deliverable:** `raven version`, `raven init go-cli`, and `raven config debug` work correctly.

### Phase 2: Task System & Agent Adapters (Weeks 3-4)

**Goal:** Task management, agent adapters, and the implementation loop.

- Task spec parser (markdown -> `Task` struct)
- Task state management (read/write `task-state.conf`)
- Phase config parser (`phases.conf`)
- Dependency resolution and next-task selection
- `raven status` with progress bars
- Agent interface and Claude adapter (subprocess management, output streaming, rate-limit parsing)
- Codex adapter
- Prompt template system with placeholder substitution
- Implementation loop engine (task selection -> prompt -> agent -> verify -> iterate)
- Rate-limit detection, backoff, and countdown
- Dirty-tree recovery (stash/unstash)
- `raven implement --agent claude --phase 1` and `raven implement --task T-007`
- `--dry-run` mode
- Unit tests for task selection, rate-limit parsing, prompt generation

**Deliverable:** `raven implement --agent claude --phase 1` runs the full implementation loop for a phase.

### Phase 3: Review Pipeline (Weeks 5-6)

**Goal:** Multi-agent parallel review, fix engine, and PR creation.

- Git diff generation and risk classification
- Review prompt synthesis with project context injection
- JSON extraction from agent output (Go port of `json-extract.js` logic)
- Multi-agent parallel review with `errgroup` concurrency control
- Finding consolidation and deduplication
- Review report generation (markdown)
- Review fix engine with verification commands
- PR creation via `gh` CLI
- `raven review`, `raven fix`, `raven pr`
- Integration tests for review pipeline

**Deliverable:** `raven review --agents claude,codex --concurrency 4` produces a consolidated review report.

### Phase 4: Workflow Engine & Pipeline (Weeks 7-8)

**Goal:** Generic workflow engine, pipeline orchestrator, checkpoint/resume.

- State machine workflow runner
- Workflow state checkpointing (JSON to `.raven/state/`)
- Step handler registry with built-in handlers
- Built-in workflows: `implement`, `implement-review-pr`, `pipeline`
- Custom workflow definitions in `raven.toml`
- `raven resume` with run listing
- Phase pipeline orchestrator (branch management, phase chaining, metadata tracking)
- Pipeline interactive wizard using `charmbracelet/huh`
- `raven pipeline --phase 1`, `--phase all`, `--from-phase 2`
- `--dry-run` for all workflows
- Unit tests for state machine transitions, checkpoint/resume

**Deliverable:** `raven pipeline --phase all` orchestrates the full lifecycle across all phases with checkpoint/resume.

### Phase 5: PRD Decomposition (Weeks 9-10)

**Goal:** Three-phase concurrent PRD decomposition workflow.

- PRD shredder (single agent call -> epic JSON)
- Parallel epic decomposition workers
- Merge, dedup, renumber, DAG validation
- Task file emitter (`T-XXX.md`, `phases.conf`, `task-state.conf`, `PROGRESS.md`, `INDEX.md`)
- `raven prd --file PRD.md --concurrent --concurrency 3`
- `--single-pass` mode for small PRDs
- Rate-limit handling in workers (shared machinery with implementation loop)
- Integration tests with sample PRDs

**Deliverable:** `raven prd --file docs/prd/PRD.md --concurrent` produces a complete task breakdown.

### Phase 6: TUI Command Center (Weeks 11-13)

**Goal:** A polished Bubble Tea dashboard for real-time workflow monitoring and control.

- Bubble Tea application scaffold with Elm architecture
- Split-pane layout (sidebar + main area)
- Sidebar: workflow list, task progress bars, phase progress, rate-limit status
- Agent output panel: live-scrolling viewport with tabbed multi-agent view
- Event log panel: workflow milestones and status changes
- Status bar: current state, iteration count, timer
- Keyboard navigation (Tab, arrows, j/k, p for pause, s for skip, q for quit)
- Help overlay with keybinding reference
- Pipeline wizard integration (launch from TUI)
- `raven dashboard` command
- Responsive layout (adapts to terminal size)
- Lipgloss styling with light/dark terminal support
- Integration testing for TUI state management

**Deliverable:** `raven dashboard` launches a beautiful, responsive command center with live agent monitoring.

### Phase 7: Polish & Distribution (Week 14)

**Goal:** Release-ready binary with cross-platform builds and documentation.

- GoReleaser configuration: macOS (amd64/arm64), Linux (amd64/arm64), Windows (amd64)
- Shell completion installation scripts
- Comprehensive README with usage examples
- Man page generation
- Performance benchmarking (startup time, concurrent agent overhead, TUI frame rate)
- End-to-end integration test suite
- GitHub Release automation

**Deliverable:** Published v2.0.0 with signed binaries for all platforms.

### v2.1+ (Post-Release)

| Feature | Description | Priority |
|---------|-------------|----------|
| Gemini adapter | Full Gemini CLI agent adapter | High |
| Task source adapters | Jira, Linear, GitHub Issues task import | High |
| More templates | node-service, python-lib, monorepo | Medium |
| Parallel task execution | Run independent tasks within a phase concurrently | Medium |
| Plugin system | External step handlers loaded at runtime | Medium |
| SSH-served TUI | Serve dashboard over SSH via `charmbracelet/wish` | Medium |
| Homebrew formula | `brew install raven` | Medium |
| MCP server | `raven mcp serve` for agent integration | Low |
| Watch mode | Monitor for task state changes and auto-run | Low |

---

## 8. Migration from Bash Prototype

Raven v2 is a complete Go rewrite. The bash prototype (`lib/*.sh`, `commands/*.sh`, `agents/*.sh`) serves as a reference implementation but is NOT wrapped or called by the Go binary.

### What Transfers Directly

| Bash Component | Go Equivalent | Notes |
|----------------|--------------|-------|
| `lib/toml-parser.sh` (180 lines) | `BurntSushi/toml` (0 lines) | Library handles full TOML spec |
| `lib/config-lib.sh` (130 lines) | `internal/config/` | Type-safe structs, validation |
| `lib/raven-lib.sh` (1576 lines) | `internal/loop/runner.go` | Same logic, proper error handling |
| `lib/phases-lib.sh` (156 lines) | `internal/task/phases.go` | Struct-based, not string manipulation |
| `lib/task-state-lib.sh` (311 lines) | `internal/task/state.go` | File locking, typed statuses |
| `lib/review-lib.sh` (1044 lines) | `internal/review/` | errgroup concurrency, typed findings |
| `lib/phase-pipeline.sh` (1689 lines) | `internal/pipeline/` | State machine, not procedural script |
| `agents/claude.sh` (144 lines) | `internal/agent/claude.go` | Interface-based, testable |
| `agents/codex.sh` (149 lines) | `internal/agent/codex.go` | Interface-based, testable |
| `bin/raven` (110 lines) | `cmd/raven/main.go` | Cobra CLI dispatch |
| `commands/cmd-*.sh` (8 files) | `internal/cli/*.go` | Type-safe flag parsing |
| gum-based wizard | `charmbracelet/huh` forms | Same UX, native Go |
| ANSI escape codes (50+ lines) | `charmbracelet/lipgloss` | Declarative styling |

### What's New in v2

- Generic workflow engine (state machine runner)
- TUI command center (Bubble Tea dashboard)
- PRD concurrent decomposition
- Checkpoint/resume for all workflows
- Concurrent agent output multiplexing
- Rate-limit coordination across agents
- Type-safe configuration with validation
- Cross-platform single binary

---

## 9. Risks & Mitigations

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| TUI complexity exceeds estimates | Medium | Medium | Start with minimal layout (sidebar + one agent panel). Add features incrementally. Use bubbletea's component model for isolated development. |
| Agent CLI interfaces change | Medium | Low | Agent adapters are isolated behind interface. Changes are localized to one file per agent. |
| Rate-limit parsing is fragile | Medium | Medium | Comprehensive regex test suite per agent. Fallback to configurable default backoff when parsing fails. |
| Go binary size exceeds 25MB | Low | Low | No WASM or large embedded assets (unlike Harvx). Templates are small text files. Expected size: 10-15MB. |
| Workflow engine over-engineering | Medium | Medium | Ship v2.0 with only built-in workflows. Custom workflow TOML definitions are a stretch goal for v2.1 if not needed at launch. |
| Concurrent agent coordination edge cases | Medium | Medium | Extensive integration tests with mock agents that simulate rate limits, failures, and slow responses. |
| Loss of bash edge-case handling during rewrite | Medium | High | Port test cases from bash prototype. Keep bash prototype as reference during development. Side-by-side comparison testing. |
| TUI doesn't work in all terminals | Low | Medium | Use bubbletea/lipgloss which handle terminal capability detection. Test on: iTerm2, Terminal.app, Alacritty, tmux, SSH sessions. |

---

## 10. Open Questions (with Proposed Directions)

| # | Question | Proposed Direction |
|---|---------|-------------------|
| 1 | **Custom workflows in v2.0?** Should TOML-defined custom workflows ship in v2.0 or v2.1? | Start with built-in workflows only in v2.0. The engine supports custom workflows internally but expose the TOML definition DSL in v2.1 after validating the abstraction. |
| 2 | **Parallel task execution in v2.0?** Should independent tasks run concurrently within a phase? | v2.0: sequential (simpler, matches bash prototype behavior). v2.1: add `--parallel` flag for concurrent independent task execution. |
| 3 | **Agent API vs CLI?** Should Raven call agent APIs directly (Anthropic API, OpenAI API) or always shell out to CLI tools? | Always shell out to CLI tools in v2.0. The CLIs handle auth, streaming, tool use, and permissions. Direct API access is a v2.1+ consideration for headless/CI environments. |
| 4 | **TUI separate binary?** Should the TUI be a separate binary to reduce core binary size? | Single binary. The TUI adds ~2-3MB (bubbletea + lipgloss + bubbles). Not worth the distribution complexity. |
| 5 | **State directory?** Should Raven use `.raven/` or `.raven-state/` for runtime state? | `.raven/` -- short, clear, consistent with other tools (`.git/`, `.harvx/`). Add to default `.gitignore` template. |
| 6 | **License?** MIT vs Apache 2.0? | MIT for maximum adoption. |
| 7 | **Config merging or override?** When `raven.toml` has an `[agents.claude]` section and CLI passes `--model`, does the CLI model override the config model? | Yes, CLI always overrides config. Resolution order: CLI flags > env vars > `raven.toml` > defaults. |

---

## 11. Appendix

### A. Technology References

- [Bubble Tea -- Go TUI Framework](https://github.com/charmbracelet/bubbletea) (29K+ stars)
- [Lip Gloss -- Declarative Terminal Styling](https://github.com/charmbracelet/lipgloss) (8K+ stars)
- [Bubbles -- TUI Components](https://github.com/charmbracelet/bubbles) (5K+ stars)
- [Huh -- Go TUI Forms](https://github.com/charmbracelet/huh) (4K+ stars)
- [Cobra -- Go CLI Framework](https://github.com/spf13/cobra) (39K+ stars)
- [BurntSushi/toml -- Go TOML Parser](https://github.com/BurntSushi/toml)
- [errgroup -- Bounded Parallel Execution](https://pkg.go.dev/golang.org/x/sync/errgroup)
- [GoReleaser -- Cross-Platform Builds](https://goreleaser.com/)

### B. Existing System Reference

The bash prototype at `/Users/abdelazizmoustafa/prj/prod/raven/` contains the reference implementation:

| File | Lines | Purpose |
|------|-------|---------|
| `lib/raven-lib.sh` | ~1576 | Core implementation loop engine |
| `lib/phase-pipeline.sh` | ~1689 | Phase pipeline orchestrator |
| `lib/review-lib.sh` | ~1044 | Multi-agent review shared functions |
| `lib/review.sh` | ~302 | Review orchestrator |
| `lib/review-fix.sh` | ~290 | Review fix applier |
| `lib/create-pr.sh` | ~344 | PR creation |
| `lib/task-state-lib.sh` | ~311 | Task state management |
| `lib/phases-lib.sh` | ~156 | Phase definitions parser |
| `lib/toml-parser.sh` | ~180 | TOML parser (bash associative arrays) |
| `lib/config-lib.sh` | ~130 | Config resolution bridge |
| `agents/claude.sh` | ~144 | Claude agent adapter |
| `agents/codex.sh` | ~149 | Codex agent adapter |
| `bin/raven` | ~110 | CLI entry point |
| `commands/cmd-*.sh` | 8 files | Command wrappers |

### C. Glossary

| Term | Definition |
|------|-----------|
| **Workflow** | A named state machine defining a multi-step process with transitions |
| **Step** | A single unit of work within a workflow, executed by a step handler |
| **Agent** | An AI coding tool (Claude, Codex, Gemini) that executes prompts |
| **Adapter** | A Go implementation that wraps a specific agent's CLI interface |
| **Phase** | A group of related tasks executed together as a unit |
| **Pipeline** | A multi-phase workflow that chains implement-review-fix-PR per phase |
| **Rate limit** | API usage throttling by the AI provider, requiring backoff and retry |
| **Checkpoint** | A persisted snapshot of workflow state enabling resume after interruption |
| **TUI** | Terminal User Interface -- the interactive dashboard built with Bubble Tea |
| **Elm architecture** | A pattern (Model-Update-View) used by Bubble Tea for state management |
| **errgroup** | Go's `x/sync/errgroup` for bounded parallel execution with error propagation |
| **Step handler** | A Go type implementing `StepHandler` interface for a specific workflow step |
| **Fan-out/fan-in** | Parallel execution pattern: dispatch N workers, collect all results |
| **Scatter-gather** | Map-reduce pattern used in PRD decomposition and multi-agent review |
