# T-022: Claude Agent Adapter

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 8-12hrs |
| Dependencies | T-004, T-005, T-009, T-021, T-025 |
| Blocked By | T-021 |
| Blocks | T-027, T-029 |

## Goal
Implement the Claude agent adapter that wraps the `claude` CLI tool behind the Agent interface. This adapter handles command construction from configuration and runtime options, subprocess execution with real-time output streaming, rate-limit detection via regex-based output parsing, and prerequisite checking. Claude is the primary agent for Raven's implementation loop, making this adapter critical path.

## Background
Per PRD Section 5.2, the Claude adapter supports: `--model`, `--permission-mode`, `--allowedTools`, `--append-system-prompt`, `--output-format json`, and effort via `CLAUDE_CODE_EFFORT_LEVEL` env var. It parses rate-limit messages like "Your rate limit will reset" and "Too many requests" and extracts reset times. The adapter uses `os/exec` for subprocess management with `StdoutPipe()` and `StderrPipe()` for real-time streaming. The `claude` CLI is the Claude Code interactive coding agent.

Per the raven.toml configuration:
```toml
[agents.claude]
command = "claude"
model = "claude-opus-4-6"
effort = "high"
prompt_template = "prompts/implement-claude.md"
allowed_tools = "Edit,Write,Read,Glob,Grep,Bash(go*),Bash(git*)"
```

## Technical Specifications
### Implementation Approach
Create `internal/agent/claude.go` containing the `ClaudeAgent` struct that implements the `Agent` interface. Command construction assembles the `claude` CLI invocation from AgentConfig defaults merged with RunOpts overrides. Output capture uses `io.MultiWriter` to simultaneously stream to the caller and capture for rate-limit parsing. Rate-limit detection uses pre-compiled regexes. Prerequisite checking verifies the `claude` command is on PATH via `exec.LookPath`.

### Key Components
- **ClaudeAgent**: Agent implementation for the Claude Code CLI
- **buildCommand**: Constructs the exec.Cmd with all flags and environment
- **captureOutput**: Streams stdout/stderr while capturing for post-processing
- **Rate-limit regexes**: Pre-compiled patterns for Claude-specific rate-limit messages

### API/Interface Contracts
```go
// internal/agent/claude.go
package agent

import (
    "context"
    "os/exec"
    "regexp"
    "time"
)

// Claude rate-limit detection patterns.
var (
    // Matches "Your rate limit will reset in X seconds" or similar
    reClaudeRateLimit = regexp.MustCompile(
        `(?i)(?:rate limit|too many requests|rate.?limited)`,
    )

    // Extracts reset duration: "reset in 30 seconds", "reset in 2 minutes"
    reClaudeResetTime = regexp.MustCompile(
        `(?i)reset\s+(?:in\s+)?(\d+)\s*(seconds?|minutes?|hours?)`,
    )

    // Matches "try again in X" pattern
    reClaudeTryAgain = regexp.MustCompile(
        `(?i)try\s+again\s+in\s+(\d+)\s*(seconds?|minutes?|hours?)`,
    )
)

// ClaudeAgent wraps the Claude Code CLI tool.
type ClaudeAgent struct {
    config AgentConfig
    logger interface{ Debug(msg string, keyvals ...interface{}) }
}

// NewClaudeAgent creates a Claude agent adapter with the given configuration.
func NewClaudeAgent(config AgentConfig, logger interface{ Debug(msg string, keyvals ...interface{}) }) *ClaudeAgent

// Compile-time interface check.
var _ Agent = (*ClaudeAgent)(nil)

func (c *ClaudeAgent) Name() string { return "claude" }

// Run executes the claude CLI with the given options.
// It streams output in real-time while capturing stdout and stderr.
// After execution, it scans output for rate-limit signals.
func (c *ClaudeAgent) Run(ctx context.Context, opts RunOpts) (*RunResult, error)

// CheckPrerequisites verifies the claude CLI is installed and accessible.
func (c *ClaudeAgent) CheckPrerequisites() error

// ParseRateLimit scans output for Claude-specific rate-limit patterns.
func (c *ClaudeAgent) ParseRateLimit(output string) (*RateLimitInfo, bool)

// DryRunCommand returns the full command that would be executed.
func (c *ClaudeAgent) DryRunCommand(opts RunOpts) string

// buildCommand constructs the exec.Cmd for a claude invocation.
// Merges config defaults with RunOpts overrides.
// CLI flags: --model, --permission-mode accept, --allowedTools,
// --output-format json, --print (non-interactive mode).
// Environment: CLAUDE_CODE_EFFORT_LEVEL for effort setting.
func (c *ClaudeAgent) buildCommand(ctx context.Context, opts RunOpts) *exec.Cmd

// parseResetDuration extracts a time.Duration from a matched reset time string.
// "30 seconds" -> 30s, "2 minutes" -> 2m, "1 hour" -> 1h.
func parseResetDuration(amount string, unit string) time.Duration
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| os/exec | stdlib | Subprocess management |
| context | stdlib | Cancellation and timeout |
| regexp | stdlib | Rate-limit pattern matching |
| io | stdlib | MultiWriter for dual output capture |
| bufio | stdlib | Line-by-line output scanning |
| bytes | stdlib | Output buffering |
| time | stdlib | Duration parsing, execution timing |
| strings | stdlib | String manipulation |
| fmt | stdlib | Error wrapping |
| charmbracelet/log | latest | Structured debug logging |

## Acceptance Criteria
- [ ] ClaudeAgent implements the Agent interface (compile-time check passes)
- [ ] Run executes the `claude` CLI as a subprocess with correct flags
- [ ] `--model` flag set from config or RunOpts override
- [ ] `--permission-mode accept` always set (non-interactive)
- [ ] `--allowedTools` set from config when specified
- [ ] `--output-format json` set when RunOpts.OutputFormat is "json"
- [ ] `--print` flag set for non-interactive mode (prompt via stdin or --prompt)
- [ ] `CLAUDE_CODE_EFFORT_LEVEL` environment variable set from effort config
- [ ] Prompt passed via `--prompt` flag or stdin
- [ ] WorkDir sets the subprocess working directory
- [ ] Extra Env variables are merged with parent environment
- [ ] stdout and stderr are captured in RunResult
- [ ] ExitCode is correctly extracted
- [ ] Duration is measured from start to finish
- [ ] ParseRateLimit detects "rate limit" messages and extracts reset time
- [ ] ParseRateLimit detects "too many requests" messages
- [ ] ParseRateLimit returns false for non-rate-limit output
- [ ] CheckPrerequisites returns nil when `claude` is on PATH
- [ ] CheckPrerequisites returns descriptive error when `claude` is not found
- [ ] DryRunCommand returns the full command string with all flags
- [ ] Context cancellation kills the subprocess
- [ ] Unit tests achieve 85% coverage

## Testing Requirements
### Unit Tests
- buildCommand with minimal opts: correct base command
- buildCommand with all opts set: all flags present
- buildCommand with effort "high": CLAUDE_CODE_EFFORT_LEVEL=high in env
- buildCommand with allowed_tools: --allowedTools flag present
- buildCommand with output_format "json": --output-format json present
- buildCommand with work_dir: Cmd.Dir set correctly
- buildCommand with extra env: merged with parent env
- ParseRateLimit("Your rate limit will reset in 30 seconds"): IsLimited=true, ResetAfter=30s
- ParseRateLimit("Too many requests"): IsLimited=true (no specific duration)
- ParseRateLimit("try again in 2 minutes"): IsLimited=true, ResetAfter=2m
- ParseRateLimit("Task completed successfully"): returns false
- ParseRateLimit with mixed case: case-insensitive matching
- parseResetDuration("30", "seconds"): 30s
- parseResetDuration("2", "minutes"): 2m
- parseResetDuration("1", "hour"): 1h
- DryRunCommand includes all flags and prompt
- CheckPrerequisites: test with exec.LookPath mock (or skip if claude not installed)
- Name() returns "claude"

### Integration Tests
- Run with a mock claude script that outputs canned text: verify RunResult fields
- Run with a mock claude script that outputs rate-limit message: verify RateLimit detected
- Run with context cancellation: verify subprocess is killed within 5 seconds
- Run with non-zero exit code: verify ExitCode captured correctly

### Edge Cases to Handle
- Very long prompts (>100KB): pass via temp file with `--prompt-file` instead of --prompt flag
- Prompt containing shell-special characters: os/exec handles escaping automatically
- Claude CLI not installed: CheckPrerequisites returns clear installation instructions
- Claude exits with signal (killed): ExitCode should be -1 or signal code
- Claude output with ANSI escape codes: capture as-is, let consumers handle
- Claude output with no newline at end: handle gracefully
- Empty stdout (agent produced no output): RunResult.Stdout is empty string
- Rate-limit message split across stdout and stderr: check both

## Implementation Notes
### Recommended Approach
1. Start with buildCommand -- this is the most important function to get right
2. Use `exec.CommandContext(ctx, command, args...)` for cancellation support
3. For output streaming: use `cmd.StdoutPipe()` and `cmd.StderrPipe()`
4. Start goroutines to read from pipes using `bufio.Scanner` before `cmd.Start()`
5. Collect output into `bytes.Buffer` while optionally writing to a log file
6. After `cmd.Wait()`, scan captured output for rate-limit patterns
7. Use `time.Now()` before and after Run to compute Duration
8. For DryRunCommand: construct the same command but return it as a string instead of executing
9. Test with mock scripts in `testdata/mock-agents/claude` that simulate various behaviors

### Potential Pitfalls
- Must start pipe readers BEFORE calling `cmd.Start()` to avoid deadlock when buffers fill
- Must call `cmd.Wait()` AFTER pipe readers finish to avoid data loss
- `exec.CommandContext` sends SIGKILL on context cancel -- consider using a gentler approach (SIGTERM first, then SIGKILL after timeout) for clean Claude shutdown
- The `claude` CLI uses `--print` for non-interactive mode (sends prompt, gets response, exits). Without it, claude starts an interactive REPL
- Prompt via `--prompt` flag has shell length limits on some systems. For long prompts, write to a temp file and use `--prompt-file`
- Do not use `cmd.CombinedOutput()` -- it merges streams and you lose the ability to distinguish stdout from stderr

### Security Considerations
- Agent commands are constructed from config and runtime opts -- no user-controlled strings are directly interpolated into shell commands (os/exec handles this safely)
- Environment variables passed to subprocess should not include sensitive credentials beyond what the parent process already has
- Log agent commands at debug level only (may contain prompts with proprietary code)

## References
- [PRD Section 5.2 - Agent Adapter System](docs/prd/PRD-Raven.md)
- [Go os/exec documentation](https://pkg.go.dev/os/exec)
- [os/exec patterns for subprocess management](https://www.dolthub.com/blog/2022-11-28-go-os-exec-patterns/)
- [Running External Programs in Go](https://medium.com/@caring_smitten_gerbil_914/running-external-programs-in-go-the-right-way-38b11d272cd1)
- [Claude Code CLI documentation](https://docs.anthropic.com/en/docs/claude-code)
