# Raven

> AI workflow orchestration command center.

[![CI](https://github.com/AbdelazizMoustafa10m/Raven/actions/workflows/ci.yml/badge.svg)](https://github.com/AbdelazizMoustafa10m/Raven/actions/workflows/ci.yml)
[![Release](https://github.com/AbdelazizMoustafa10m/Raven/actions/workflows/release.yml/badge.svg)](https://github.com/AbdelazizMoustafa10m/Raven/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/AbdelazizMoustafa10m/Raven)](https://goreportcard.com/report/github.com/AbdelazizMoustafa10m/Raven)

<!-- placeholder: screenshot or GIF of the TUI dashboard -->

## What is Raven?

Raven is a Go-based AI workflow orchestration command center that manages the full lifecycle of AI-assisted software development. It coordinates multiple AI agents (Claude, Codex, Gemini) through configurable workflow pipelines: from decomposing a product requirements document into task files, through iterative implementation, multi-agent code review, automated fix cycles, and pull request creation.

Raven ships as a single, self-contained binary with zero runtime dependencies. It orchestrates external AI CLI tools (the `claude`, `codex`, and `gemini` executables) via subprocess calls -- the same approach used by tools like `lazygit` and the GitHub CLI -- so it never stores credentials and imposes no constraints on which AI provider you use.

It provides two interfaces: a headless CLI for scripted automation and CI integration, and an interactive TUI dashboard built with Bubble Tea for real-time monitoring and control of running workflows.

## Features

- **PRD decomposition** -- shreds a product requirements document into structured task files with dependency graphs and phase assignments
- **Implementation loop** -- drives an AI agent through tasks in a phase, handling rate limits, retries, dirty-tree recovery, and SIGINT graceful shutdown
- **Multi-agent review** -- runs multiple AI agents in parallel on a git diff, consolidates findings, deduplicates issues by severity, and generates a structured markdown report
- **Fix-verify cycles** -- applies review findings using an AI agent, re-runs verification commands, and loops until the code is clean
- **Pull request creation** -- generates an AI-written PR summary and opens a PR via the `gh` CLI
- **Phase pipeline** -- chains implement, review, fix, and PR stages into a single command with branch management and checkpoint/resume support
- **Interactive TUI dashboard** -- real-time split-pane view of agent output, task progress bars, rate-limit countdowns, and event logs
- **Workflow engine** -- lightweight state machine with JSON checkpoints; interrupted runs can be listed and resumed
- **Shell completions** -- bash, zsh, fish, and PowerShell completion scripts
- **Man pages** -- auto-generated Section 1 man pages for every command

## Installation

### Binary Downloads

Download the latest pre-built binary for your platform from the [GitHub Releases page](https://github.com/AbdelazizMoustafa10m/Raven/releases).

```bash
# macOS (Apple Silicon)
curl -Lo raven.tar.gz https://github.com/AbdelazizMoustafa10m/Raven/releases/latest/download/raven_<VERSION>_darwin_arm64.tar.gz
tar -xzf raven.tar.gz
sudo install -m 755 raven /usr/local/bin/raven

# macOS (Intel)
curl -Lo raven.tar.gz https://github.com/AbdelazizMoustafa10m/Raven/releases/latest/download/raven_<VERSION>_darwin_amd64.tar.gz
tar -xzf raven.tar.gz
sudo install -m 755 raven /usr/local/bin/raven

# Linux (x86-64)
curl -Lo raven.tar.gz https://github.com/AbdelazizMoustafa10m/Raven/releases/latest/download/raven_<VERSION>_linux_amd64.tar.gz
tar -xzf raven.tar.gz
sudo install -m 755 raven /usr/local/bin/raven
```

Verify the checksum against `checksums.txt` included in the release:

```bash
sha256sum -c checksums.txt --ignore-missing
```

### From Source

Requires Go 1.24 or later.

```bash
git clone https://github.com/AbdelazizMoustafa10m/Raven.git
cd Raven
CGO_ENABLED=0 go build -o raven ./cmd/raven
sudo install -m 755 raven /usr/local/bin/raven
```

### Homebrew (coming soon)

```bash
# Not yet available. Track https://github.com/AbdelazizMoustafa10m/Raven/issues
# brew install abdelazizmoustafa10m/tap/raven
```

## Quick Start

This section takes you from a fresh install to running `raven implement` in under three minutes.

### 1. Initialize a Project

Navigate to your project repository and run:

```bash
cd my-project
raven init
```

This creates a `raven.toml` configuration file with sensible defaults. Open it and fill in your project name and at least one agent:

```toml
[project]
name = "my-project"
language = "go"

[agents.claude]
command = "claude"
model   = "claude-sonnet-4-6"
effort  = "high"
```

### 2. Implement Your First Task

Make sure the AI agent CLI is installed (`claude`, `codex`, or `gemini`). Then run the implementation loop against a phase:

```bash
raven implement --agent claude --phase 1
```

To target a single task:

```bash
raven implement --agent claude --task T-001
```

Use `--dry-run` to preview the generated prompt and agent command without invoking the agent:

```bash
raven implement --agent claude --phase 1 --dry-run
```

### 3. Run a Review

After implementation, run multi-agent code review against the diff from `main`:

```bash
raven review --agents claude,codex --base main
```

Raven runs both agents in parallel, consolidates findings, and writes a markdown report to `review-report.md`.

### 4. Create a PR

Apply any fixes and open a pull request:

```bash
raven fix --agent claude --report review-report.md
raven pr --base main --agent claude
```

Or run the entire pipeline in one command:

```bash
raven pipeline --phase 1 --impl-agent claude --review-agent codex --fix-agent claude
```

## Commands

### raven implement

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

### raven review

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

### raven fix

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

### raven pr

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

### raven pipeline

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

### raven prd

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

### raven status

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

### raven resume

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

### raven init

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

### raven config

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

### raven dashboard

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

### raven version

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

### raven completion

Generate shell completion scripts.

```
raven completion [bash|zsh|fish|powershell]
```

See [Shell Completions](#shell-completions) for installation instructions.

## Configuration

Raven reads configuration from `raven.toml` in the current directory (or a parent directory) by default. Use `--config` to specify an alternate path.

### raven.toml Reference

```toml
[project]
name        = "my-project"           # Project name (used in branch names, prompts)
language    = "go"                   # Primary language (used in prompts)
tasks_dir   = "docs/tasks"           # Directory containing T-NNN-*.md task files
task_state_file = "docs/tasks/task-state.conf"  # Task status tracking file
phases_conf = "docs/tasks/phases.conf"          # Phase assignment configuration
progress_file = "docs/tasks/PROGRESS.md"        # Generated progress report path
log_dir     = "scripts/logs"         # Directory for agent output logs
prompt_dir  = "prompts"              # Directory for custom prompt templates
branch_template = "phase/{phase_id}-{slug}"     # Git branch naming template
verification_commands = [            # Commands run after each implementation
  "go build ./...",
  "go test ./...",
]

[agents.claude]
command         = "claude"                # CLI executable name
model           = "claude-sonnet-4-6"    # Model identifier
effort          = "high"                 # Effort/reasoning level: high, medium, low
prompt_template = "implement"            # Prompt template name
allowed_tools   = "Edit,Write,Bash"      # Comma-separated allowed tool names

[agents.codex]
command = "codex"
model   = "o4-mini"

[review]
extensions        = ".go,.ts,.py"        # File extensions to include in review diff
risk_patterns     = "auth,secret,password"  # Patterns flagging high-risk files
prompts_dir       = "prompts/review"     # Directory for review prompt templates
rules_dir         = "rules"             # Directory for review rule files
project_brief_file = "PROJECT_BRIEF.md" # Project context injected into review prompts

[workflows.implement-review-pr]
description = "Full implement -> review -> PR workflow"
steps = ["implement", "review", "fix", "pr"]
```

### Agent Configuration

Each agent is configured under `[agents.<name>]`. The name must be lowercase and contain only alphanumeric characters and hyphens (e.g., `claude`, `codex`, `my-agent`).

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `command` | string | agent name | CLI executable (must be on `$PATH`) |
| `model` | string | | Model identifier passed via `--model` flag |
| `effort` | string | | Effort level: `high`, `medium`, `low` (Claude-specific) |
| `prompt_template` | string | `implement` | Template name loaded from `prompts/` |
| `allowed_tools` | string | | Comma-separated tools the agent may invoke |

### Workflow Configuration

Custom workflows extend the four built-in workflows. See [docs/workflows.md](docs/workflows.md) for the full reference.

| Field | Type | Description |
|-------|------|-------------|
| `description` | string | Human-readable workflow purpose |
| `steps` | []string | Ordered step names |
| `transitions` | map | Per-step event-to-step transition map |

### Environment Variables

| Variable | Equivalent Flag | Description |
|----------|-----------------|-------------|
| `RAVEN_VERBOSE` | `--verbose` | Enable debug output |
| `RAVEN_QUIET` | `--quiet` | Suppress all output except errors |
| `RAVEN_NO_COLOR` | `--no-color` | Disable colored output |
| `NO_COLOR` | `--no-color` | Standard no-color convention |
| `RAVEN_LOG_FORMAT=json` | | Emit logs as JSON (for CI log parsers) |

## Architecture

Raven is organized as a set of focused internal packages under `internal/`:

```
cmd/raven/main.go          Entry point (calls cli.Execute, exits with code)
internal/
  cli/                     Cobra command definitions (one file per command)
  config/                  TOML loading, defaults, four-layer resolution, validation
  workflow/                State machine engine, step registry, checkpointing
  agent/                   Agent interface, Claude/Codex/Gemini adapters, rate-limit coordinator
  task/                    Task spec parser, state manager, phase config, dependency resolver
  loop/                    Implementation loop runner, prompt generator, recovery handlers
  review/                  Diff generator, multi-agent orchestrator, consolidator, fix engine
  prd/                     PRD shredder, parallel scatter workers, merger, task file emitter
  pipeline/                Phase pipeline orchestrator, branch manager, interactive wizard
  git/                     Git operations wrapper (git and gh CLI)
  tui/                     Bubble Tea app, layout, panels, event bridge
  buildinfo/               Version, commit, date (injected via ldflags)
```

For detailed design documentation see [docs/workflows.md](docs/workflows.md) and [docs/agents.md](docs/agents.md).

## Shell Completions

### Bash

```bash
# Generate and install for the current user (Linux)
mkdir -p ~/.local/share/bash-completion/completions
raven completion bash > ~/.local/share/bash-completion/completions/raven

# macOS with bash-completion@2 via Homebrew
raven completion bash > $(brew --prefix)/etc/bash_completion.d/raven
```

### Zsh

```zsh
# Add to ~/.zshrc if $fpath is already set up:
raven completion zsh > "${fpath[1]}/_raven"

# Or generate to a completions directory:
mkdir -p ~/.zsh/completions
raven completion zsh > ~/.zsh/completions/_raven
# Add to ~/.zshrc: fpath=(~/.zsh/completions $fpath)
# Then: autoload -Uz compinit && compinit
```

### Fish

```fish
raven completion fish > ~/.config/fish/completions/raven.fish
```

### PowerShell

```powershell
raven completion powershell | Out-String | Invoke-Expression
# To persist, add the above line to your $PROFILE.
```

### Install Script

The release archive includes an install script that auto-detects your shell:

```bash
./scripts/completions/install.sh
```

## Man Pages

Generate and install man pages (requires the release binary or a source build):

```bash
# Generate to man/man1/
go run ./scripts/gen-manpages man/man1

# Install system-wide (requires sudo)
sudo ./scripts/install-manpages.sh

# View a man page
man raven-implement
```

Man pages are also included in the release archive at `man/man1/`.

## CI/CD Integration

### GitHub Actions Example

```yaml
name: AI Implementation

on:
  workflow_dispatch:
    inputs:
      phase:
        description: "Phase number to implement"
        required: true
        default: "1"

jobs:
  implement:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install Raven
        run: |
          curl -Lo raven.tar.gz \
            https://github.com/AbdelazizMoustafa10m/Raven/releases/latest/download/raven_latest_linux_amd64.tar.gz
          tar -xzf raven.tar.gz
          sudo install -m 755 raven /usr/local/bin/raven

      - name: Implement phase
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          RAVEN_LOG_FORMAT: json
        run: |
          raven implement \
            --agent claude \
            --phase ${{ github.event.inputs.phase }}

      - name: Review changes
        run: |
          raven review --agents claude --base main --output review.md

      - name: Upload review report
        uses: actions/upload-artifact@v4
        with:
          name: review-report
          path: review.md
```

### Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Error |
| `2` | Partial success (e.g., review found issues) |
| `3` | User-cancelled (Ctrl+C) |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing instructions, and code standards.

## License

Raven is released under the [MIT License](LICENSE).
