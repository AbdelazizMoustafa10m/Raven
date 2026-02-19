# Command Reference

Complete reference for all Raven CLI commands, flags, and usage examples.

## raven implement

Run the AI implementation loop for a phase or a single task.

```
raven implement --agent <name> [--phase <n>|--task <id>] [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--agent` | (required) | Agent to use: `claude`, `codex`, `gemini` |
| `--phase` | | Phase to implement (integer or `all`); mutually exclusive with `--task` |
| `--task` | | Single task ID (e.g. `T-029`); mutually exclusive with `--phase` |
| `--max-iterations` | `50` | Maximum loop iterations |
| `--max-limit-waits` | `5` | Maximum rate-limit wait cycles |
| `--sleep` | `5` | Seconds between iterations |
| `--model` | | Override the configured model for this run |
| `--dry-run` | `false` | Print prompts and commands without invoking the agent |

**Examples:**

```bash
# Implement phase 2 with Claude
raven implement --agent claude --phase 2

# Implement all phases sequentially with Codex
raven implement --agent codex --phase all

# Implement a single task
raven implement --agent claude --task T-029

# Override model and increase iteration limit
raven implement --agent claude --phase 2 --model claude-opus-4-6 --max-iterations 100
```

## raven review

Run multi-agent code review on the current diff.

```
raven review [--agents <names>] [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--agents` | (from config) | Comma-separated agent names, e.g. `claude,codex` |
| `--concurrency` | `2` | Maximum concurrent review agents |
| `--mode` | `all` | Review mode: `all` (all agents review full diff) or `split` (diff split among agents) |
| `--base` | `main` | Git ref to diff against |
| `--output` | `review-report.md` | Output file for the review report |

**Examples:**

```bash
# Review with two agents in parallel
raven review --agents claude,codex --base main

# Split diff between agents (each agent reviews a subset of files)
raven review --agents claude,codex --mode split --base HEAD~3
```

## raven fix

Apply review findings using an AI agent, then re-run verification commands.

```
raven fix [--agent <name>] [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--agent` | (from config) | Agent for applying fixes |
| `--report` | `review-report.md` | Path to the review report to fix |
| `--max-cycles` | `3` | Maximum fix-verify cycles |

**Examples:**

```bash
raven fix --agent claude --report review-report.md
raven fix --agent claude --max-cycles 5
```

## raven pr

Generate an AI-written PR summary and create a pull request via `gh`.

```
raven pr [--base <branch>] [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--base` | `main` | Base branch for the PR |
| `--agent` | (from config) | Agent for PR summary generation |

**Examples:**

```bash
raven pr --base main --agent claude
```

## raven pipeline

Run the full implement → review → fix → PR pipeline for one or more phases.

```
raven pipeline [--phase <n>|--from-phase <n>] [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--phase` | | Phase specifier (integer or `all`); mutually exclusive with `--from-phase` |
| `--from-phase` | | Resume pipeline from this phase number |
| `--impl-agent` | | Agent for implementation |
| `--review-agent` | | Agent(s) for review (comma-separated) |
| `--fix-agent` | | Agent for fixes |
| `--review-concurrency` | `2` | Max concurrent review agents |
| `--max-review-cycles` | `3` | Max review-fix iterations per phase |
| `--skip-implement` | `false` | Skip the implementation stage |
| `--skip-review` | `false` | Skip the review stage |
| `--skip-fix` | `false` | Skip the fix stage |
| `--skip-pr` | `false` | Skip the PR creation stage |
| `--interactive` | `false` | Launch the configuration wizard (requires a TTY) |
| `--base` | `main` | Base branch for phase branches |
| `--sync-base` | `false` | Fetch from origin before execution |
| `--dry-run` | `false` | Describe planned execution without running |

**Examples:**

```bash
# Full pipeline for phase 2
raven pipeline --phase 2 --impl-agent claude --review-agent codex --fix-agent claude

# All phases, starting from phase 3
raven pipeline --from-phase 3 --impl-agent claude --review-agent claude

# Dry-run to preview the plan
raven pipeline --phase 1 --impl-agent claude --dry-run

# Interactive wizard (requires a terminal)
raven pipeline --interactive
```

## raven prd

Decompose a product requirements document into structured task files.

```
raven prd --file <path> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--file` | (required) | Path to the PRD markdown file |
| `--agent` | (from config) | Agent for decomposition |
| `--output-dir` | `docs/tasks` | Directory for generated task files |
| `--concurrency` | `3` | Parallel worker count |
| `--single-pass` | `false` | Single-pass mode (concurrency=1, no retries) |
| `--dry-run` | `false` | Show decomposition plan without writing files |

**Examples:**

```bash
raven prd --file docs/PRD.md --agent claude
raven prd --file docs/PRD.md --agent claude --concurrency 5
raven prd --file docs/PRD.md --agent claude --dry-run
```

## raven status

Show task progress and phase completion.

```
raven status [--phase <n>] [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--phase` | | Show status for a specific phase |
| `--json` | `false` | Output as JSON |
| `--verbose` | `false` | Show individual task details |

**Examples:**

```bash
raven status
raven status --phase 2 --verbose
raven status --json | jq '.phases[0].completion'
```

## raven resume

Manage workflow checkpoints and resume interrupted runs.

```
raven resume [--run <id>] [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--run` | | Resume a specific run ID |
| `--list` | `false` | List all available checkpoints |
| `--clean` | | Remove a specific checkpoint by run ID |
| `--clean-all` | `false` | Remove all checkpoints |

**Examples:**

```bash
# List all checkpoints
raven resume --list

# Resume a specific run
raven resume --run abc123

# Clean up old checkpoints
raven resume --clean-all
```

## raven init

Initialize a new Raven project from a template.

```
raven init [template] [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | (directory name) | Project name |
| `--force` | `false` | Overwrite existing files |

**Examples:**

```bash
# Initialize with the default go-cli template
raven init

# Initialize with a custom project name
raven init go-cli --name my-project
```

## raven config

Inspect and validate the resolved configuration.

```
raven config <subcommand>
```

| Subcommand | Description |
|------------|-------------|
| `debug` | Print the resolved configuration with source annotations (CLI/env/file/default) |
| `validate` | Validate the configuration and report errors and warnings |

**Examples:**

```bash
raven config debug
raven config validate
```

## raven dashboard

Launch the interactive TUI dashboard.

```
raven dashboard [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | `false` | Display the dashboard layout without starting any workflows |

**Examples:**

```bash
raven dashboard
```

## raven version

Print version information.

```
raven version [--json]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON |

**Examples:**

```bash
raven version
raven version --json
```

## raven completion

Generate shell completion scripts.

```
raven completion [bash|zsh|fish|powershell]
```

See [Shell Completions](shell-completions.md) for installation instructions.

## Global Flags

These flags are available on all commands:

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `raven.toml` | Path to configuration file |
| `--verbose` | `false` | Enable debug output |
| `--quiet` | `false` | Suppress all output except errors |
| `--no-color` | `false` | Disable colored output |

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Error |
| `2` | Partial success (e.g., review found issues) |
| `3` | User-cancelled (Ctrl+C) |
