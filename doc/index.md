# Raven

> AI workflow orchestration command center.

Raven is a Go-based CLI that manages the full lifecycle of AI-assisted software development. It coordinates multiple AI agents (Claude, Codex, Gemini) through configurable workflow pipelines: from decomposing a PRD into task files, through iterative implementation, multi-agent code review, automated fix cycles, and pull request creation.

Single binary. Zero runtime dependencies. Cross-platform (macOS/Linux/Windows).

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

## Quick Links

<div class="grid cards" markdown>

- :material-download: **[Installation](getting-started/installation.md)** -- Download a pre-built binary or build from source
- :material-rocket-launch: **[Quick Start](getting-started/quickstart.md)** -- Initialize a project and run your first workflow
- :material-console: **[Commands](reference/commands.md)** -- Full flag tables and usage examples
- :material-cog: **[Configuration](reference/configuration.md)** -- Complete `raven.toml` field reference
- :material-sitemap: **[Architecture](architecture/overview.md)** -- Package layout, core abstractions, design decisions
- :material-pipe: **[Workflow Engine](architecture/workflows.md)** -- State machine design, built-in workflows, custom workflows

</div>

## Commands at a Glance

| Command | Description |
|---------|-------------|
| `raven implement` | Run the AI implementation loop for a phase or task |
| `raven review` | Run multi-agent code review on the current diff |
| `raven fix` | Apply review findings and re-run verification |
| `raven pr` | Generate a PR summary and open a pull request |
| `raven pipeline` | Run the full implement → review → fix → PR pipeline |
| `raven prd` | Decompose a PRD into structured task files |
| `raven status` | Show task progress and phase completion |
| `raven resume` | List, resume, or clean workflow checkpoints |
| `raven init` | Initialize a new Raven project from a template |
| `raven config` | Inspect and validate the resolved configuration |
| `raven dashboard` | Launch the interactive TUI dashboard |
| `raven version` | Print version information |
| `raven completion` | Generate shell completion scripts |
