# CI/CD Integration

Raven is designed to run in headless CI environments. This page covers GitHub Actions integration, exit codes, environment variables, and automation patterns.

## GitHub Actions Example

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

## Exit Codes

All Raven commands use consistent exit codes for scripting:

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Error |
| `2` | Partial success (e.g., review found issues) |
| `3` | User-cancelled (Ctrl+C) |

Use these in CI scripts to distinguish between hard failures and review findings:

```bash
raven review --agents claude --base main || {
  code=$?
  if [ "$code" -eq 2 ]; then
    echo "Review completed with findings"
  else
    echo "Review failed"
    exit $code
  fi
}
```

## Environment Variables

These environment variables control Raven's output behavior, which is useful for CI log parsing:

| Variable | Equivalent Flag | Description |
|----------|-----------------|-------------|
| `RAVEN_VERBOSE` | `--verbose` | Enable debug output |
| `RAVEN_QUIET` | `--quiet` | Suppress all output except errors |
| `RAVEN_NO_COLOR` | `--no-color` | Disable colored output |
| `NO_COLOR` | `--no-color` | Standard no-color convention |
| `RAVEN_LOG_FORMAT=json` | | Emit logs as JSON (for CI log parsers) |

## Automation Patterns

### Full Pipeline in CI

Run the complete implement → review → fix → PR pipeline:

```yaml
- name: Run pipeline
  run: |
    raven pipeline \
      --phase ${{ github.event.inputs.phase }} \
      --impl-agent claude \
      --review-agent codex \
      --fix-agent claude \
      --base main
```

### Review-Only on Pull Requests

Add Raven review as a PR check:

```yaml
on:
  pull_request:
    branches: [main]

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Install Raven
        run: |
          curl -Lo raven.tar.gz \
            https://github.com/AbdelazizMoustafa10m/Raven/releases/latest/download/raven_latest_linux_amd64.tar.gz
          tar -xzf raven.tar.gz
          sudo install -m 755 raven /usr/local/bin/raven

      - name: Review PR diff
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          RAVEN_NO_COLOR: "1"
          RAVEN_LOG_FORMAT: json
        run: |
          raven review \
            --agents claude \
            --base origin/${{ github.base_ref }} \
            --output review.md

      - name: Upload review
        uses: actions/upload-artifact@v4
        with:
          name: review-report
          path: review.md
```

### Dry-Run Validation

Validate configuration and prompts without invoking agents:

```bash
raven config validate
raven implement --agent claude --phase 1 --dry-run
raven pipeline --phase 1 --impl-agent claude --dry-run
```
