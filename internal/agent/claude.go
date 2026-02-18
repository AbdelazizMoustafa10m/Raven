package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Compile-time check that ClaudeAgent implements Agent.
var _ Agent = (*ClaudeAgent)(nil)

// claudeLogger is the minimal logging interface required by ClaudeAgent.
// It accepts a message and structured key-value pairs.
type claudeLogger interface {
	Debug(msg string, keyvals ...interface{})
}

// maxInlinePromptBytes is the threshold above which a prompt is written to a
// temp file instead of being passed directly on the command line.
const maxInlinePromptBytes = 100 * 1024 // 100 KiB

// maxDryRunPromptLen is the maximum number of runes shown inline in the
// DryRunCommand output before the prompt is truncated with "...".
const maxDryRunPromptLen = 120

var (
	// reClaudeRateLimit matches common rate-limit phrases in Claude output.
	reClaudeRateLimit = regexp.MustCompile(`(?i)(?:rate limit|too many requests|rate.?limited)`)

	// reClaudeResetTime matches "reset in N seconds/minutes/hours" patterns.
	reClaudeResetTime = regexp.MustCompile(`(?i)reset\s+(?:in\s+)?(\d+)\s*(seconds?|minutes?|hours?)`)

	// reClaudeTryAgain matches "try again in N seconds/minutes/hours" patterns.
	reClaudeTryAgain = regexp.MustCompile(`(?i)try\s+again\s+in\s+(\d+)\s*(seconds?|minutes?|hours?)`)
)

// ClaudeAgent is an Agent adapter that executes prompts via the Claude CLI.
// It wraps the claude command-line tool and handles argument construction,
// subprocess execution, output capture, and rate-limit detection.
type ClaudeAgent struct {
	config AgentConfig
	logger claudeLogger
}

// NewClaudeAgent creates a new ClaudeAgent with the given configuration and
// logger. The logger may be nil, in which case debug messages are silently
// discarded.
func NewClaudeAgent(config AgentConfig, logger claudeLogger) *ClaudeAgent {
	return &ClaudeAgent{
		config: config,
		logger: logger,
	}
}

// Name returns the agent identifier "claude".
func (c *ClaudeAgent) Name() string { return "claude" }

// CheckPrerequisites verifies that the Claude CLI executable can be found on
// the system PATH. It returns a descriptive error with installation hints when
// the binary is missing.
func (c *ClaudeAgent) CheckPrerequisites() error {
	cmd := c.config.Command
	if cmd == "" {
		cmd = "claude"
	}
	if _, err := exec.LookPath(cmd); err != nil {
		return fmt.Errorf(
			"claude CLI not found (looked for %q): install it from https://docs.anthropic.com/en/docs/claude-cli: %w",
			cmd, err,
		)
	}
	return nil
}

// Run executes the given prompt using the Claude CLI and returns the captured
// output, exit code, and duration. The ctx parameter is used for cancellation
// and timeout propagation.
//
// If opts.StreamEvents is non-nil AND opts.OutputFormat is
// OutputFormatStreamJSON, streaming is enabled: stdout is decoded as JSONL in
// real-time and typed StreamEvent values are forwarded to opts.StreamEvents
// using non-blocking sends (slow consumers drop events). The full stdout is
// still captured in RunResult.Stdout for backward compatibility.
//
// If the output contains a rate-limit signal, the returned RunResult will have
// its RateLimit field populated.
func (c *ClaudeAgent) Run(ctx context.Context, opts RunOpts) (*RunResult, error) {
	start := time.Now()

	cmd := c.buildCommand(ctx, opts)

	if c.logger != nil {
		c.logger.Debug("running claude",
			"command", cmd.Path,
			"args", cmd.Args,
			"work_dir", cmd.Dir,
		)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stderr pipe: %w", err)
	}

	var (
		stdoutBuf bytes.Buffer
		stderrBuf bytes.Buffer
		wg        sync.WaitGroup
	)

	// Determine whether streaming mode is active. Both conditions must hold:
	// the caller must supply a channel AND request stream-json output format.
	streaming := opts.StreamEvents != nil && opts.OutputFormat == OutputFormatStreamJSON

	wg.Add(2)
	go func() {
		defer wg.Done()
		if streaming {
			// Use TeeReader so stdoutBuf captures everything while the decoder
			// reads from the same byte stream. The goroutine owns the pipe read.
			teeReader := io.TeeReader(stdoutPipe, &stdoutBuf)
			decoder := NewStreamDecoder(teeReader)
			for {
				event, err := decoder.Next()
				if err != nil {
					// io.EOF or decode error -- stop reading.
					break
				}
				// Non-blocking send: drop the event when the consumer is slow.
				select {
				case opts.StreamEvents <- *event:
				default:
				}
			}
		} else {
			_, _ = stdoutBuf.ReadFrom(stdoutPipe)
		}
	}()
	go func() {
		defer wg.Done()
		_, _ = stderrBuf.ReadFrom(stderrPipe)
	}()

	if err := cmd.Start(); err != nil {
		// Drain goroutines: Go closes the write ends of the pipes on Start
		// failure, so ReadFrom will return EOF and the goroutines will exit.
		wg.Wait()
		return nil, fmt.Errorf("starting claude: %w", err)
	}

	// Wait for all output to be drained before calling Wait.
	wg.Wait()

	waitErr := cmd.Wait()
	duration := time.Since(start)

	exitCode := 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			// Non-exit error (e.g. process was killed by signal without an
			// ExitError). Still return the output collected so far.
			return nil, fmt.Errorf("waiting for claude: %w", waitErr)
		}
	}

	combined := stdoutBuf.String() + stderrBuf.String()
	rateLimit, _ := c.ParseRateLimit(combined)

	return &RunResult{
		Stdout:    stdoutBuf.String(),
		Stderr:    stderrBuf.String(),
		ExitCode:  exitCode,
		Duration:  duration,
		RateLimit: rateLimit,
	}, nil
}

// ParseRateLimit examines agent output for rate-limit signals.
// It returns a populated *RateLimitInfo and true when a rate-limit phrase is
// detected; otherwise it returns nil and false.
func (c *ClaudeAgent) ParseRateLimit(output string) (*RateLimitInfo, bool) {
	if !reClaudeRateLimit.MatchString(output) {
		return nil, false
	}

	var resetAfter time.Duration

	// Try "reset in N unit" first.
	if m := reClaudeResetTime.FindStringSubmatch(output); len(m) == 3 {
		resetAfter = parseResetDuration(m[1], m[2])
	} else if m := reClaudeTryAgain.FindStringSubmatch(output); len(m) == 3 {
		// Fall back to "try again in N unit".
		resetAfter = parseResetDuration(m[1], m[2])
	}

	return &RateLimitInfo{
		IsLimited:  true,
		ResetAfter: resetAfter,
		Message:    output,
	}, true
}

// DryRunCommand returns the command string that would be executed without
// actually running it. Long prompts are truncated in the output.
func (c *ClaudeAgent) DryRunCommand(opts RunOpts) string {
	args := c.buildArgs(opts, true /* dryRun */)
	cmd := c.config.Command
	if cmd == "" {
		cmd = "claude"
	}
	return cmd + " " + strings.Join(args, " ")
}

// buildCommand constructs the *exec.Cmd for the given RunOpts. Prompt data
// longer than maxInlinePromptBytes is written to a temp file automatically.
func (c *ClaudeAgent) buildCommand(ctx context.Context, opts RunOpts) *exec.Cmd {
	command := c.config.Command
	if command == "" {
		command = "claude"
	}

	args := c.buildArgs(opts, false /* dryRun */)
	cmd := exec.CommandContext(ctx, command, args...)

	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}

	// Build environment: inherit current env, then append effort, then caller env.
	env := os.Environ()

	effort := opts.Effort
	if effort == "" {
		effort = c.config.Effort
	}
	if effort != "" {
		env = append(env, "CLAUDE_CODE_EFFORT_LEVEL="+effort)
	}

	env = append(env, opts.Env...)
	cmd.Env = env

	return cmd
}

// buildArgs constructs the argument slice for the Claude CLI. When dryRun is
// true, long prompts are truncated instead of being written to temp files.
func (c *ClaudeAgent) buildArgs(opts RunOpts, dryRun bool) []string {
	var args []string

	args = append(args, "--permission-mode", "accept")
	args = append(args, "--print")

	// Model: RunOpts takes precedence over config.
	model := opts.Model
	if model == "" {
		model = c.config.Model
	}
	if model != "" {
		args = append(args, "--model", model)
	}

	// AllowedTools: RunOpts takes precedence over config.
	allowedTools := opts.AllowedTools
	if allowedTools == "" {
		allowedTools = c.config.AllowedTools
	}
	if allowedTools != "" {
		args = append(args, "--allowedTools", allowedTools)
	}

	// OutputFormat.
	if opts.OutputFormat != "" {
		args = append(args, "--output-format", opts.OutputFormat)
	}

	// Prompt handling.
	switch {
	case opts.PromptFile != "":
		args = append(args, "--prompt-file", opts.PromptFile)

	case opts.Prompt != "" && len(opts.Prompt) > maxInlinePromptBytes:
		if dryRun {
			truncated := opts.Prompt
			if len(truncated) > maxDryRunPromptLen {
				truncated = truncated[:maxDryRunPromptLen] + "..."
			}
			args = append(args, "--prompt", truncated)
		} else {
			// Write prompt to a temp file to avoid arg-length limits.
			f, err := os.CreateTemp("", "raven-claude-prompt-*.md")
			if err == nil {
				if _, werr := f.WriteString(opts.Prompt); werr == nil {
					_ = f.Close()
					args = append(args, "--prompt-file", f.Name())
				} else {
					_ = f.Close()
					// Fall back to inline if write failed.
					args = append(args, "--prompt", opts.Prompt)
				}
			} else {
				// Fall back to inline if temp file creation failed.
				args = append(args, "--prompt", opts.Prompt)
			}
		}

	case opts.Prompt != "":
		args = append(args, "--prompt", opts.Prompt)
	}

	return args
}

// parseResetDuration converts a numeric string and a time unit word into a
// time.Duration. Unrecognised units return 0.
func parseResetDuration(amount string, unit string) time.Duration {
	n, err := strconv.Atoi(amount)
	if err != nil || n <= 0 {
		return 0
	}

	unit = strings.ToLower(unit)
	switch {
	case strings.HasPrefix(unit, "second"):
		return time.Duration(n) * time.Second
	case strings.HasPrefix(unit, "minute"):
		return time.Duration(n) * time.Minute
	case strings.HasPrefix(unit, "hour"):
		return time.Duration(n) * time.Hour
	default:
		return 0
	}
}

