# Agent Adapters

Raven drives external AI CLI tools through a common `Agent` interface. This document describes the interface contract, each built-in adapter, rate-limit handling, and how to add a custom agent.

## Agent Interface

All agent adapters implement the `agent.Agent` interface defined in `internal/agent/agent.go`:

```go
type Agent interface {
    // Name returns the agent's identifier (e.g., "claude", "codex").
    // Names must be lowercase and contain only alphanumeric characters
    // and hyphens.
    Name() string

    // Run executes a prompt and returns the result.
    // The context is used for cancellation and timeout.
    Run(ctx context.Context, opts RunOpts) (*RunResult, error)

    // CheckPrerequisites verifies that the agent CLI is installed and
    // accessible on $PATH. Returns a descriptive error when missing.
    CheckPrerequisites() error

    // ParseRateLimit examines agent output for rate-limit signals.
    // Returns rate-limit info and true if a limit was detected.
    ParseRateLimit(output string) (*RateLimitInfo, bool)

    // DryRunCommand returns the command string that would be executed
    // without running it. Used for --dry-run mode.
    DryRunCommand(opts RunOpts) string
}
```

### RunOpts

```go
type RunOpts struct {
    Prompt       string            // Prompt text to send to the agent
    PromptFile   string            // Path to a file containing the prompt (used when Prompt is large)
    WorkDir      string            // Working directory for the subprocess
    Model        string            // Override the configured model
    OutputFormat string            // Output format: "text", "json", "stream-json"
    StreamEvents chan<- StreamEvent // Channel for real-time JSONL events (stream-json mode)
    ExtraEnv     []string          // Additional environment variables
}
```

### RunResult

```go
type RunResult struct {
    Stdout   string        // Captured standard output
    Stderr   string        // Captured standard error
    ExitCode int           // Process exit code
    Duration time.Duration // Wall-clock time of the subprocess
}
```

## Claude Adapter

The Claude adapter wraps the `claude` CLI (Claude Code). It is configured under `[agents.claude]` in `raven.toml`.

### Prerequisites

The `claude` CLI must be installed and authenticated:

```bash
# Install via npm (official method)
npm install -g @anthropic-ai/claude-code

# Verify
claude --version
```

### Configuration

```toml
[agents.claude]
command         = "claude"                # Default: "claude"
model           = "claude-sonnet-4-6"    # Passed as --model
effort          = "high"                 # Sets CLAUDE_CODE_EFFORT_LEVEL env var
prompt_template = "implement"            # Template name in prompts/
allowed_tools   = "Edit,Write,Bash"      # Passed as --allowedTools
```

### How It Works

The Claude adapter builds a subprocess command with these flags:

```
claude --permission-mode accept --print --model <model> \
       --allowedTools <tools> [--prompt-file <file>|--] \
       < <prompt>
```

For prompts larger than 100 KiB, the adapter writes the prompt to a temporary file and passes `--prompt-file <path>` instead of piping stdin. The temp file is cleaned up after the subprocess exits.

### Streaming Support

When `opts.OutputFormat == "stream-json"` and `opts.StreamEvents != nil`, the adapter activates JSONL streaming:

```
claude --output-format stream-json ...
```

The JSONL output is decoded in real-time using a `StreamDecoder` and forwarded to the `StreamEvents` channel. Full stdout is still captured in `RunResult.Stdout` for backward compatibility via `io.TeeReader`.

### Rate-Limit Detection

The adapter scans output for these patterns:

| Pattern | Example |
|---------|---------|
| `rate limit` / `too many requests` | `Error: rate limit exceeded` |
| `reset in N seconds/minutes` | `Rate limit will reset in 60 seconds` |
| `try again in N seconds/minutes` | `Please try again in 2 minutes` |

When a rate limit is detected, `ParseRateLimit` returns a `*RateLimitInfo` with the provider (`anthropic`) and estimated reset time.

### Model Values

| Identifier | Notes |
|------------|-------|
| `claude-sonnet-4-6` | Recommended for implementation (balance of speed and quality) |
| `claude-opus-4-6` | Highest quality; slower and more expensive |
| `claude-haiku-4-5` | Fastest; suitable for low-complexity tasks |

## Codex Adapter

The Codex adapter wraps the `codex` CLI (OpenAI Codex). It is configured under `[agents.codex]` in `raven.toml`.

### Prerequisites

The `codex` CLI must be installed and authenticated:

```bash
# Install via npm
npm install -g @openai/codex

# Verify
codex --version
```

### Configuration

```toml
[agents.codex]
command = "codex"     # Default: "codex"
model   = "o4-mini"  # Passed as --model
```

### How It Works

The Codex adapter builds:

```
codex exec --sandbox --ephemeral -a never --model <model> \
           [--prompt <text>|--prompt-file <file>]
```

The `--sandbox`, `--ephemeral`, and `-a never` flags run the agent in a restricted, non-interactive mode suitable for automation.

### Rate-Limit Detection

The Codex adapter handles three rate-limit response formats:

| Format | Example |
|--------|---------|
| Short decimal seconds | `Rate limited. Retry after 5.448s` |
| Long human-readable | `Rate limited. Retry after 1 days 2 hours 30 minutes` |
| Keyword fallback | `429 Too Many Requests` |

### Model Values

| Identifier | Notes |
|------------|-------|
| `o4-mini` | Recommended default; fast reasoning model |
| `o3` | Higher quality; slower |

## Gemini Adapter (Stub)

The Gemini adapter (`[agents.gemini]`) is currently a stub. All methods return `ErrNotImplemented` except `ParseRateLimit` (which always returns `nil, false`) and `DryRunCommand` (which returns a placeholder string).

Full Gemini support is planned for a future release. Track progress at [GitHub Issues](https://github.com/AbdelazizMoustafa10m/Raven/issues).

```go
// ErrNotImplemented is returned by the Gemini adapter's Run and
// CheckPrerequisites methods until the adapter is fully implemented.
var ErrNotImplemented = errors.New("gemini adapter not yet implemented")
```

## Rate-Limit Coordination

Raven coordinates rate limits across all agents that share an API provider. Multiple agents using the same provider (e.g., a `claude` and a `claude-haiku` both hitting Anthropic) are coordinated so that a rate limit on one blocks all agents on that provider.

### Provider Mapping

| Agent Name | Provider |
|------------|----------|
| `claude` | `anthropic` |
| `codex` | `openai` |
| `gemini` | `google` |

### RateLimitCoordinator

The `RateLimitCoordinator` tracks per-provider state:

```go
type ProviderState struct {
    IsLimited   bool
    ResetAt     time.Time
    WaitCount   int
    LastMessage string
    UpdatedAt   time.Time
}
```

The coordinator uses a `sync.RWMutex` for thread-safe access across concurrent review agents. When a rate limit is detected:

1. The implementing goroutine calls `RecordRateLimit(provider, info)`.
2. Other goroutines on the same provider call `ShouldWait(provider)` and receive `true`.
3. All waiters call `WaitForReset(ctx, provider)` which sleeps until `ResetAt` plus a small jitter.
4. After `MaxWaits` consecutive rate-limit cycles, `ExceededMaxWaits` returns `true` and the loop runner returns `ErrMaxWaitsExceeded`.

### Backoff Configuration

```go
type BackoffConfig struct {
    DefaultWait  time.Duration // Default wait when no reset time is available (default: 60s)
    MaxWaits     int           // Maximum wait cycles before giving up (default: 5)
    JitterFactor float64       // Random jitter fraction added to wait (default: 0.1)
}
```

## Agent Registry

Agents are registered in a `Registry` at startup. The registry provides lookup-by-name and validates that names are unique:

```go
registry := agent.NewRegistry()
registry.Register(agent.NewClaudeAgent(claudeCfg, logger))
registry.Register(agent.NewCodexAgent(codexCfg, logger))

ag, err := registry.Get("claude")
```

### Sentinel Errors

| Error | Description |
|-------|-------------|
| `ErrNotFound` | No agent with that name is registered |
| `ErrDuplicateName` | An agent with the same name was already registered |
| `ErrInvalidName` | The agent name is empty or contains invalid characters |

## Adding a Custom Agent

To integrate a new AI CLI tool:

1. Create `internal/agent/<name>.go` with a struct that implements `Agent`.
2. Add a compile-time check: `var _ Agent = (*MyAgent)(nil)`.
3. Register the adapter in `buildAgentRegistry` in `internal/cli/implement.go`.
4. Add shell completion hints for the `--agent` flag in `implement.go` and `pipeline.go`.
5. Map the agent name to its API provider in `internal/agent/ratelimit.go`'s `AgentProvider` map.

### Minimal Stub

```go
package agent

import (
    "context"
    "fmt"
    "os/exec"
)

var _ Agent = (*MyAgent)(nil)

type MyAgent struct {
    config AgentConfig
}

func NewMyAgent(cfg AgentConfig) *MyAgent {
    return &MyAgent{config: cfg}
}

func (a *MyAgent) Name() string { return "my-agent" }

func (a *MyAgent) Run(ctx context.Context, opts RunOpts) (*RunResult, error) {
    // Build and run the CLI subprocess
    cmd := exec.CommandContext(ctx, a.config.Command, "--prompt", opts.Prompt)
    out, err := cmd.CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("my-agent: %w", err)
    }
    return &RunResult{Stdout: string(out), ExitCode: 0}, nil
}

func (a *MyAgent) CheckPrerequisites() error {
    if _, err := exec.LookPath(a.config.Command); err != nil {
        return fmt.Errorf("my-agent CLI not found: %w", err)
    }
    return nil
}

func (a *MyAgent) ParseRateLimit(output string) (*RateLimitInfo, bool) {
    // Return nil, false if the agent does not enforce rate limits
    return nil, false
}

func (a *MyAgent) DryRunCommand(opts RunOpts) string {
    return fmt.Sprintf("%s --prompt %q", a.config.Command, opts.Prompt[:min(len(opts.Prompt), 80)])
}
```

## Security Notes

- Raven does not store, log, or transmit API credentials. Keys are managed by each AI CLI tool via its own environment variables or keychain.
- Subprocess arguments are passed as explicit string slices to `exec.Command`; no shell interpolation occurs.
- Prompt content is never logged at INFO level; only `debug` messages include prompt excerpts.
- Large prompts (>100 KiB) are written to a temporary file under `os.TempDir()` and removed after use.
