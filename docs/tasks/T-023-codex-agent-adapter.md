# T-023: Codex Agent Adapter

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-004, T-005, T-009, T-021, T-025 |
| Blocked By | T-021 |
| Blocks | T-027, T-029 |

## Goal
Implement the Codex agent adapter that wraps the `codex` CLI tool behind the Agent interface. This adapter handles Codex-specific command construction (exec mode, sandbox isolation, ephemeral sessions), subprocess execution with output streaming, and Codex-specific rate-limit parsing. Codex is the secondary agent for Raven, used as an alternative to Claude for implementation and as a parallel review agent.

## Background
Per PRD Section 5.2, the Codex adapter supports `exec` mode with `--sandbox`, `-a never` (auto-approve never), `--ephemeral`, and `--model` flags. It parses "try again in X days Y minutes" rate-limit messages. The `codex` CLI is OpenAI's Codex coding agent. Per the raven.toml configuration:
```toml
[agents.codex]
command = "codex"
model = "gpt-5.3-codex"
effort = "high"
prompt_template = "prompts/implement-codex.md"
```

Rate-limit messages from Codex follow the format: "Please try again in Xs" where X is a decimal number of seconds (e.g., "Please try again in 5.448s"), or longer formats like "try again in X days Y minutes."

## Technical Specifications
### Implementation Approach
Create `internal/agent/codex.go` containing the `CodexAgent` struct implementing the Agent interface. The structure mirrors ClaudeAgent (T-022) but with Codex-specific command flags and rate-limit patterns. Codex operates in `exec` mode where it receives a prompt and executes it in a sandboxed environment. The adapter constructs the appropriate `codex exec` command with sandbox and approval settings.

### Key Components
- **CodexAgent**: Agent implementation for the Codex CLI
- **buildCommand**: Constructs the `codex exec` command with appropriate flags
- **Rate-limit regexes**: Pre-compiled patterns for Codex-specific rate-limit messages

### API/Interface Contracts
```go
// internal/agent/codex.go
package agent

import (
    "context"
    "os/exec"
    "regexp"
    "time"
)

// Codex rate-limit detection patterns.
var (
    // Matches "Please try again in 5.448s" or "try again in 2.482s"
    reCodexTryAgain = regexp.MustCompile(
        `(?i)(?:please\s+)?try\s+again\s+in\s+([\d.]+)s`,
    )

    // Matches "try again in X days Y minutes" or "try again in X minutes Y seconds"
    reCodexTryAgainLong = regexp.MustCompile(
        `(?i)try\s+again\s+in\s+(?:(\d+)\s*days?)?\s*(?:(\d+)\s*hours?)?\s*(?:(\d+)\s*minutes?)?\s*(?:([\d.]+)\s*seconds?)?`,
    )

    // Matches "Rate limit reached" as a fallback
    reCodexRateLimit = regexp.MustCompile(
        `(?i)rate\s+limit\s+reached`,
    )
)

// CodexAgent wraps the Codex CLI tool.
type CodexAgent struct {
    config AgentConfig
    logger interface{ Debug(msg string, keyvals ...interface{}) }
}

// NewCodexAgent creates a Codex agent adapter with the given configuration.
func NewCodexAgent(config AgentConfig, logger interface{ Debug(msg string, keyvals ...interface{}) }) *CodexAgent

// Compile-time interface check.
var _ Agent = (*CodexAgent)(nil)

func (c *CodexAgent) Name() string { return "codex" }

// Run executes the codex CLI in exec mode with the given options.
func (c *CodexAgent) Run(ctx context.Context, opts RunOpts) (*RunResult, error)

// CheckPrerequisites verifies the codex CLI is installed and accessible.
func (c *CodexAgent) CheckPrerequisites() error

// ParseRateLimit scans output for Codex-specific rate-limit patterns.
func (c *CodexAgent) ParseRateLimit(output string) (*RateLimitInfo, bool)

// DryRunCommand returns the full command that would be executed.
func (c *CodexAgent) DryRunCommand(opts RunOpts) string

// buildCommand constructs the exec.Cmd for a codex exec invocation.
// Always uses exec mode: codex exec --sandbox --ephemeral -a never --model <model>
// Prompt is passed via stdin or --prompt flag.
func (c *CodexAgent) buildCommand(ctx context.Context, opts RunOpts) *exec.Cmd

// parseCodexDuration extracts a time.Duration from Codex's rate-limit format.
// "5.448s" -> 5.448s
// "1 days 2 hours 30 minutes" -> 26h30m
func parseCodexDuration(match []string) time.Duration
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| os/exec | stdlib | Subprocess management |
| context | stdlib | Cancellation and timeout |
| regexp | stdlib | Rate-limit pattern matching |
| io | stdlib | Output streaming |
| bufio | stdlib | Line-by-line output scanning |
| bytes | stdlib | Output buffering |
| time | stdlib | Duration parsing |
| strconv | stdlib | Float parsing for seconds |
| fmt | stdlib | Error wrapping |
| charmbracelet/log | latest | Structured debug logging |

## Acceptance Criteria
- [ ] CodexAgent implements the Agent interface (compile-time check)
- [ ] Run executes `codex exec` with correct flags
- [ ] `--model` flag set from config or RunOpts override
- [ ] `--sandbox` flag always set for security
- [ ] `-a never` flag set (no auto-approval of dangerous operations)
- [ ] `--ephemeral` flag set for clean sessions
- [ ] Prompt passed correctly (via stdin or flag)
- [ ] WorkDir sets subprocess working directory
- [ ] Extra Env variables merged with parent environment
- [ ] stdout and stderr captured in RunResult
- [ ] ExitCode correctly extracted
- [ ] Duration measured accurately
- [ ] ParseRateLimit detects "try again in 5.448s" and extracts duration
- [ ] ParseRateLimit detects "Rate limit reached" fallback pattern
- [ ] ParseRateLimit detects multi-unit durations ("2 days 3 hours")
- [ ] ParseRateLimit returns false for non-rate-limit output
- [ ] CheckPrerequisites returns nil when `codex` is on PATH
- [ ] CheckPrerequisites returns descriptive error when `codex` is not found
- [ ] DryRunCommand returns accurate command string
- [ ] Context cancellation kills the subprocess
- [ ] Unit tests achieve 85% coverage

## Testing Requirements
### Unit Tests
- buildCommand with minimal opts: `codex exec` base command with required flags
- buildCommand with model override: --model flag reflects override
- buildCommand with work_dir: Cmd.Dir set correctly
- ParseRateLimit("Please try again in 5.448s"): IsLimited=true, ResetAfter=~5.45s
- ParseRateLimit("try again in 2.482s"): IsLimited=true, ResetAfter=~2.48s
- ParseRateLimit("Rate limit reached for organization"): IsLimited=true, no specific duration
- ParseRateLimit("try again in 1 days 2 hours 30 minutes"): IsLimited=true, ResetAfter=26h30m
- ParseRateLimit("Task completed successfully"): returns false
- parseCodexDuration for "5.448" seconds: ~5.45s
- parseCodexDuration for multi-unit format
- DryRunCommand includes all flags
- CheckPrerequisites: test with exec.LookPath
- Name() returns "codex"

### Integration Tests
- Run with mock codex script that outputs canned text: verify RunResult
- Run with mock codex script that outputs rate-limit message: verify detection
- Run with context cancellation: subprocess killed
- Run with non-zero exit code: ExitCode captured

### Edge Cases to Handle
- Codex rate-limit with fractional seconds ("5.448s"): parse as float
- Codex rate-limit with zero duration: use default backoff
- Codex CLI not installed: clear installation instructions in error
- Very large output (>10MB): buffering should not cause OOM
- Codex exits with signal: handle exit code extraction
- Codex exec mode with no prompt: return error before executing

## Implementation Notes
### Recommended Approach
1. Follow the same structure as ClaudeAgent (T-022) for consistency
2. buildCommand: `exec.CommandContext(ctx, config.Command, "exec", "--sandbox", "--ephemeral", "-a", "never", "--model", model, "--prompt", prompt)`
3. Output streaming: same pipe-based approach as ClaudeAgent
4. Rate-limit parsing: try the short format regex first (most common), then long format, then fallback
5. For the fractional seconds format: `strconv.ParseFloat(match, 64)` then `time.Duration(float * float64(time.Second))`
6. Test with mock scripts in `testdata/mock-agents/codex`
7. Share output capture logic with ClaudeAgent via a helper function to avoid duplication

### Potential Pitfalls
- Codex's rate-limit duration is in decimal seconds (5.448s), not integer -- must parse as float
- The `exec` subcommand is required for non-interactive mode -- without it, codex starts an interactive session
- The `-a never` flag prevents auto-approval of potentially destructive operations -- always include it
- Output capture and pipe management is identical to ClaudeAgent -- extract shared logic into a helper to avoid duplication between the two adapters

### Security Considerations
- Always include `--sandbox` flag to isolate Codex execution
- Always include `-a never` to prevent unsupervised dangerous operations
- Do not log prompts at info level (may contain proprietary code)

## References
- [PRD Section 5.2 - Agent Adapter System](docs/prd/PRD-Raven.md)
- [Codex CLI rate limit issues](https://github.com/openai/codex/issues/157)
- [Codex CLI rate limit crash reports](https://github.com/openai/codex/issues/10094)
- [Go os/exec documentation](https://pkg.go.dev/os/exec)
