# Configuration Reference

Raven is configured via a `raven.toml` file in the project directory. This document describes every field in the configuration hierarchy.

## File Discovery

Raven searches for `raven.toml` in the following order:

1. The path specified by `--config` (if provided)
2. The current working directory
3. Each parent directory, walking up to the filesystem root

Run `raven config debug` to see which file was loaded and what source each field came from (CLI flag, environment variable, config file, or built-in default).

## Four-Layer Configuration Resolution

Values are merged in priority order (highest to lowest):

1. **CLI flags** -- values passed directly on the command line
2. **Environment variables** -- values from `RAVEN_*` env vars
3. **Config file** -- values from `raven.toml`
4. **Built-in defaults** -- hardcoded fallbacks

## Complete Example

```toml
[project]
name        = "my-project"
language    = "go"
tasks_dir   = "docs/tasks"
task_state_file = "docs/tasks/task-state.conf"
phases_conf = "docs/tasks/phases.conf"
progress_file = "docs/tasks/PROGRESS.md"
log_dir     = "scripts/logs"
prompt_dir  = "prompts"
branch_template = "phase/{phase_id}-{slug}"
verification_commands = [
  "go build ./...",
  "go vet ./...",
  "go test ./...",
]

[agents.claude]
command         = "claude"
model           = "claude-sonnet-4-6"
effort          = "high"
prompt_template = "implement"
allowed_tools   = "Edit,Write,Bash"

[agents.codex]
command = "codex"
model   = "o4-mini"

[review]
extensions         = ".go,.ts,.py"
risk_patterns      = "auth,secret,password,token,key"
prompts_dir        = "prompts/review"
rules_dir          = "rules"
project_brief_file = "PROJECT_BRIEF.md"

[workflows.implement-review-pr]
description = "Full implement -> review -> PR workflow"
steps       = ["implement", "review", "fix", "pr"]

[workflows.implement-only]
description = "Implementation without review"
steps       = ["implement"]
```

## [project] Section

The `[project]` section describes the repository layout that Raven should use.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | `""` | Project name used in prompts and branch names |
| `language` | string | `""` | Primary programming language, injected into agent prompts |
| `tasks_dir` | string | `"docs/tasks"` | Directory containing `T-NNN-*.md` task specification files |
| `task_state_file` | string | `"docs/tasks/task-state.conf"` | Pipe-delimited file tracking task statuses |
| `phases_conf` | string | `"docs/tasks/phases.conf"` | Phase assignment configuration file |
| `progress_file` | string | `"docs/tasks/PROGRESS.md"` | Path where the generated progress report is written |
| `log_dir` | string | `"scripts/logs"` | Directory for agent invocation logs |
| `prompt_dir` | string | `"prompts"` | Directory searched for custom prompt templates |
| `branch_template` | string | `"phase/{phase_id}-{slug}"` | Template for git branch names; supports `{phase_id}` and `{slug}` |
| `verification_commands` | []string | `[]` | Shell commands run after each implementation to verify correctness |

### branch_template Variables

| Variable | Description |
|----------|-------------|
| `{phase_id}` | The integer phase ID (e.g. `2`) |
| `{slug}` | A lowercase, hyphenated slug derived from the phase name |

### tasks_dir Layout

Raven expects task specification files named `T-NNN-<slug>.md` (e.g. `T-001-project-scaffold.md`). The parser reads the YAML-like metadata block at the top of each file. Run `raven prd` to generate these files from a PRD.

### task-state.conf Format

The task state file is a plain-text pipe-delimited file, one entry per line:

```
T-001|completed
T-002|in_progress
T-003|not_started
```

Valid status values: `not_started`, `in_progress`, `completed`, `blocked`, `skipped`.

### phases.conf Format

```
1|Foundation|T-001|T-015
2|Task System|T-016|T-030
3|Review Pipeline|T-031|T-042
```

Each line: `<phase_id>|<phase_name>|<first_task_id>|<last_task_id>`.

## [agents.NAME] Section

Each AI agent is configured in its own `[agents.<name>]` table. The name must be lowercase and match the `--agent` flag value used on the command line.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `command` | string | (agent name) | CLI executable name; must be on `$PATH` |
| `model` | string | `""` | Model identifier passed to the agent via `--model` |
| `effort` | string | `""` | Effort/reasoning level; supported values depend on the agent |
| `prompt_template` | string | `"implement"` | Template name (file in `prompt_dir`) or built-in template name |
| `allowed_tools` | string | `""` | Comma-separated list of tools the agent may invoke |

### Claude-Specific Fields

| Field | Supported Values | Notes |
|-------|-----------------|-------|
| `effort` | `high`, `medium`, `low` | Sets `CLAUDE_CODE_EFFORT_LEVEL` environment variable |
| `allowed_tools` | e.g. `Edit,Write,Bash` | Passed as `--allowedTools` to the `claude` CLI |
| `model` | e.g. `claude-sonnet-4-6`, `claude-opus-4-6` | Passed as `--model` |

### Codex-Specific Fields

| Field | Supported Values | Notes |
|-------|-----------------|-------|
| `model` | e.g. `o4-mini`, `o3` | Passed as `--model` to the `codex` CLI |
| `effort` | (ignored) | Not used by the Codex adapter |

### Gemini Status

The Gemini adapter (`[agents.gemini]`) is currently a stub. `Run` and `CheckPrerequisites` return `ErrNotImplemented`. Full support is planned for a future release.

## [review] Section

The `[review]` section controls how `raven review` generates diffs and prompts.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `extensions` | string | `""` | Comma-separated file extensions to include in review diffs |
| `risk_patterns` | string | `""` | Comma-separated substrings flagging high-risk files (e.g. `auth,secret`) |
| `prompts_dir` | string | `""` | Directory containing custom review prompt templates |
| `rules_dir` | string | `""` | Directory containing review rule files injected into prompts |
| `project_brief_file` | string | `""` | Markdown file providing project context to review agents |

### extensions

When non-empty, only files whose extension matches one of the listed values are included in the diff sent to review agents. Example: `".go,.ts"`.

### risk_patterns

Files whose path contains any of the listed substrings are flagged as high-risk in the review report, causing higher severity findings to be escalated.

## [workflows.NAME] Section

Custom workflows extend the four built-in workflows. Each workflow is a named state machine.

| Field | Type | Description |
|-------|------|-------------|
| `description` | string | Human-readable purpose of this workflow |
| `steps` | []string | Ordered list of step names |
| `transitions` | map[string]map[string]string | Per-step event-to-step transition map |

### transitions Format

```toml
[workflows.my-workflow.transitions.step_a]
success = "step_b"
failure = "__failed__"

[workflows.my-workflow.transitions.step_b]
success = "__done__"
failure = "__failed__"
```

Terminal step names: `__done__` (success) and `__failed__` (failure).

Built-in transition event names: `success`, `failure`, `blocked`, `rate_limited`, `needs_human`, `partial`.

## Environment Variable Overrides

| Variable | Equivalent | Description |
|----------|-----------|-------------|
| `RAVEN_VERBOSE` | `--verbose` | Enable debug logging |
| `RAVEN_QUIET` | `--quiet` | Suppress all output except errors |
| `RAVEN_NO_COLOR` | `--no-color` | Disable ANSI color output |
| `NO_COLOR` | `--no-color` | Standard convention (https://no-color.org) |
| `RAVEN_LOG_FORMAT=json` | | Emit structured JSON log lines (useful for CI log parsers) |

## Security Notes

- Raven does **not** store, log, or transmit API keys or credentials. API keys are managed entirely by the AI CLI tools (`claude`, `codex`, `gemini`) and read from their own environment variables or config files.
- The `raven.toml` file should **not** contain secrets. Use the AI tool's native credential store.
- All agent CLI invocations use `os/exec` with explicit argument lists; there is no shell interpolation of user-supplied values.
- Branch names are validated against the allowlist pattern `^[a-zA-Z0-9_./-]+$` before being passed to git commands.
