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

# Linux (x86-64)
curl -Lo raven.tar.gz https://github.com/AbdelazizMoustafa10m/Raven/releases/latest/download/raven_<VERSION>_linux_amd64.tar.gz
tar -xzf raven.tar.gz
sudo install -m 755 raven /usr/local/bin/raven
```

Verify the checksum against `checksums.txt` included in the release:

```bash
sha256sum -c checksums.txt --ignore-missing
```

> **Windows:** Download the `.zip` from the releases page. For the best experience on Windows, consider running Raven under [WSL](https://learn.microsoft.com/en-us/windows/wsl/).

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

### 1. Initialize a Project

```bash
cd my-project
raven init
```

This creates a `raven.toml` configuration file. Open it and configure your project:

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

Make sure the AI agent CLI is installed (`claude`, `codex`, or `gemini`). Then run:

```bash
raven implement --agent claude --phase 1
```

Use `--dry-run` to preview the generated prompt without invoking the agent:

```bash
raven implement --agent claude --phase 1 --dry-run
```

### 3. Run a Review

```bash
raven review --agents claude,codex --base main
```

### 4. Create a PR

```bash
raven fix --agent claude --report review-report.md
raven pr --base main --agent claude
```

Or run the entire pipeline in one command:

```bash
raven pipeline --phase 1 --impl-agent claude --review-agent codex --fix-agent claude
```

## Commands

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

See the [Command Reference](doc/reference/commands.md) for full flag details and usage examples.

## Documentation

Full documentation is available at **[abdelazizmoustafa10m.github.io/Raven](https://abdelazizmoustafa10m.github.io/Raven/)**.

| Document | Description |
|----------|-------------|
| [Command Reference](doc/reference/commands.md) | Full flag tables and examples for every command |
| [Configuration Reference](doc/reference/configuration.md) | Complete `raven.toml` field reference |
| [Architecture](doc/architecture/overview.md) | Package layout, core abstractions, design decisions |
| [Workflow Engine](doc/architecture/workflows.md) | State machine design, built-in workflows, custom workflows |
| [Agent Adapters](doc/architecture/agents.md) | Agent interface, Claude/Codex/Gemini adapters, rate-limit coordination |
| [Shell Completions & Man Pages](doc/guides/shell-completions.md) | Installation instructions for bash, zsh, fish, PowerShell |
| [CI/CD Integration](doc/guides/ci-cd.md) | GitHub Actions examples, exit codes, automation patterns |
| [Release Checklist](doc/development/release-checklist.md) | Steps for cutting a new release |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing instructions, and code standards.

## License

Raven is released under the [MIT License](LICENSE).
