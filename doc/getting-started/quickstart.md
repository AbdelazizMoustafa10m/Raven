# Quick Start

This guide walks you through initializing a project and running your first AI-assisted workflow.

## Prerequisites

- Raven installed ([Installation](installation.md))
- At least one AI agent CLI installed: `claude`, `codex`, or `gemini`
- A git repository to work in

## 1. Initialize a Project

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

## 2. Implement Your First Task

Run the implementation loop for a phase:

```bash
raven implement --agent claude --phase 1
```

Use `--dry-run` to preview the generated prompt without invoking the agent:

```bash
raven implement --agent claude --phase 1 --dry-run
```

## 3. Run a Review

Run multi-agent review on your changes:

```bash
raven review --agents claude,codex --base main
```

## 4. Fix and Create a PR

Apply fixes from the review, then open a pull request:

```bash
raven fix --agent claude --report review-report.md
raven pr --base main --agent claude
```

## Run the Full Pipeline

Or run everything in one command:

```bash
raven pipeline --phase 1 --impl-agent claude --review-agent codex --fix-agent claude
```

## Launch the Dashboard

For an interactive view of running workflows:

```bash
raven dashboard
```

## Next Steps

- [Command Reference](../reference/commands.md) -- full flag tables and usage examples
- [Configuration Reference](../reference/configuration.md) -- all `raven.toml` options
- [Architecture](../architecture/overview.md) -- how Raven is designed
