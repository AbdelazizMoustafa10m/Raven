package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Compile-time check that CodexAgent implements Agent.
var _ Agent = (*CodexAgent)(nil)

// codexLogger is the minimal logging interface required by CodexAgent.
// It accepts a message and structured key-value pairs.
type codexLogger interface {
	Debug(msg string, keyvals ...interface{})
}

var (
	// reCodexTryAgain matches short decimal-seconds rate-limit messages.
	// Examples: "Please try again in 5.448s" or "try again in 2.482s"
	reCodexTryAgain = regexp.MustCompile(`(?i)try\s+again\s+in\s+(\d+(?:\.\d+)?)s\b`)

	// reCodexTryAgainLong matches the long format with days/hours/minutes/seconds.
	// Each component is optional. Examples:
	//   "try again in 1 days 2 hours 30 minutes 15 seconds"
	//   "try again in 2 hours"
	//   "try again in 45 minutes 30 seconds"
	reCodexTryAgainLong = regexp.MustCompile(
		`(?i)try\s+again\s+in\s+` +
			`(?:(\d+)\s+days?\s*)?` +
			`(?:(\d+)\s+hours?\s*)?` +
			`(?:(\d+)\s+minutes?\s*)?` +
			`(?:(\d+(?:\.\d+)?)\s+seconds?)?`,
	)

	// reCodexRateLimit is a fallback that matches "Rate limit reached" phrases.
	reCodexRateLimit = regexp.MustCompile(`(?i)rate\s*limit(?:\s+reached)?`)
)

// CodexAgent is an Agent adapter that executes prompts via the Codex CLI.
// It wraps the codex command-line tool and handles argument construction,
// subprocess execution, output capture, and rate-limit detection.
type CodexAgent struct {
	config AgentConfig
	logger codexLogger
}

// NewCodexAgent creates a new CodexAgent with the given configuration and
// logger. The logger may be nil, in which case debug messages are silently
// discarded.
func NewCodexAgent(config AgentConfig, logger codexLogger) *CodexAgent {
	return &CodexAgent{
		config: config,
		logger: logger,
	}
}

// Name returns the agent identifier "codex".
func (c *CodexAgent) Name() string { return "codex" }

// CheckPrerequisites verifies that the Codex CLI executable can be found on
// the system PATH. It returns a descriptive error with installation hints when
// the binary is missing.
func (c *CodexAgent) CheckPrerequisites() error {
	cmd := c.config.Command
	if cmd == "" {
		cmd = "codex"
	}
	if _, err := exec.LookPath(cmd); err != nil {
		return fmt.Errorf(
			"codex CLI not found (looked for %q): install it from https://github.com/openai/codex: %w",
			cmd, err,
		)
	}
	return nil
}

// Run executes the given prompt using the Codex CLI and returns the captured
// output, exit code, and duration. The ctx parameter is used for cancellation
// and timeout propagation.
//
// If the output contains a rate-limit signal, the returned RunResult will have
// its RateLimit field populated.
//
// Note: RunOpts.StreamEvents is intentionally ignored by CodexAgent; the Codex CLI
// does not support stream-json output format.
func (c *CodexAgent) Run(ctx context.Context, opts RunOpts) (*RunResult, error) {
	start := time.Now()

	cmd := c.buildCommand(ctx, opts)

	if c.logger != nil {
		c.logger.Debug("running codex",
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

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = stdoutBuf.ReadFrom(stdoutPipe)
	}()
	go func() {
		defer wg.Done()
		_, _ = stderrBuf.ReadFrom(stderrPipe)
	}()

	if err := cmd.Start(); err != nil {
		// Drain goroutines: Go closes the write ends of the pipes on Start
		// failure, so ReadFrom will return EOF and the goroutines will exit.
		wg.Wait()
		return nil, fmt.Errorf("starting codex: %w", err)
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
			return nil, fmt.Errorf("waiting for codex: %w", waitErr)
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
//
// Detection order:
//  1. Short decimal-seconds format: "Please try again in 5.448s"
//  2. Long format: "try again in 1 days 2 hours 30 minutes 15 seconds"
//  3. Fallback keyword: "Rate limit reached"
func (c *CodexAgent) ParseRateLimit(output string) (*RateLimitInfo, bool) {
	// 1. Short decimal-seconds format.
	if m := reCodexTryAgain.FindStringSubmatch(output); len(m) == 2 {
		secs, err := strconv.ParseFloat(m[1], 64)
		if err == nil && secs > 0 {
			d := time.Duration(secs * float64(time.Second))
			return &RateLimitInfo{
				IsLimited:  true,
				ResetAfter: d,
				Message:    output,
			}, true
		}
	}

	// 2. Long format: "try again in X days Y hours Z minutes W seconds".
	if m := reCodexTryAgainLong.FindStringSubmatch(output); len(m) == 5 {
		d := parseCodexDuration(m)
		// Only treat as a match if the overall pattern actually matched
		// something meaningful (at least one component must be non-empty).
		if m[1] != "" || m[2] != "" || m[3] != "" || m[4] != "" {
			return &RateLimitInfo{
				IsLimited:  true,
				ResetAfter: d,
				Message:    output,
			}, true
		}
	}

	// 3. Fallback keyword match.
	if reCodexRateLimit.MatchString(output) {
		return &RateLimitInfo{
			IsLimited:  true,
			ResetAfter: 0,
			Message:    output,
		}, true
	}

	return nil, false
}

// DryRunCommand returns the command string that would be executed without
// actually running it. Long prompts are truncated in the output.
func (c *CodexAgent) DryRunCommand(opts RunOpts) string {
	cmd := c.config.Command
	if cmd == "" {
		cmd = "codex"
	}

	args := []string{"exec", "--sandbox", "--ephemeral", "-a", "never"}

	// Model: RunOpts takes precedence over config.
	model := opts.Model
	if model == "" {
		model = c.config.Model
	}
	if model != "" {
		args = append(args, "--model", model)
	}

	// Prompt handling (dry-run: truncate long prompts).
	switch {
	case opts.PromptFile != "":
		args = append(args, "--prompt-file", opts.PromptFile)
	case opts.Prompt != "":
		prompt := opts.Prompt
		if len([]rune(prompt)) > maxDryRunPromptLen {
			prompt = string([]rune(prompt)[:maxDryRunPromptLen]) + "..."
		}
		args = append(args, "--prompt", prompt)
	}

	return cmd + " " + strings.Join(args, " ")
}

// buildCommand constructs the *exec.Cmd for the given RunOpts.
func (c *CodexAgent) buildCommand(ctx context.Context, opts RunOpts) *exec.Cmd {
	command := c.config.Command
	if command == "" {
		command = "codex"
	}

	args := []string{"exec", "--sandbox", "--ephemeral", "-a", "never"}

	// Model: RunOpts takes precedence over config.
	model := opts.Model
	if model == "" {
		model = c.config.Model
	}
	if model != "" {
		args = append(args, "--model", model)
	}

	// Prompt handling.
	switch {
	case opts.PromptFile != "":
		args = append(args, "--prompt-file", opts.PromptFile)
	case opts.Prompt != "":
		args = append(args, "--prompt", opts.Prompt)
	}

	cmd := exec.CommandContext(ctx, command, args...)
	setProcGroup(cmd)

	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}

	// Build environment: inherit current env, then append caller env.
	env := os.Environ()
	env = append(env, opts.Env...)
	cmd.Env = env

	return cmd
}

// parseCodexDuration converts the submatches from reCodexTryAgainLong into a
// time.Duration. The match slice has the form:
//
//	[full_match, days, hours, minutes, seconds]
//
// Each component is optional (may be an empty string). The seconds component
// may contain a decimal fraction.
func parseCodexDuration(match []string) time.Duration {
	// match = [full, days, hours, minutes, seconds]
	var total time.Duration

	if len(match) < 5 {
		return 0
	}

	if match[1] != "" {
		if n, err := strconv.Atoi(match[1]); err == nil && n > 0 {
			total += time.Duration(n) * 24 * time.Hour
		}
	}
	if match[2] != "" {
		if n, err := strconv.Atoi(match[2]); err == nil && n > 0 {
			total += time.Duration(n) * time.Hour
		}
	}
	if match[3] != "" {
		if n, err := strconv.Atoi(match[3]); err == nil && n > 0 {
			total += time.Duration(n) * time.Minute
		}
	}
	if match[4] != "" {
		if secs, err := strconv.ParseFloat(match[4], 64); err == nil && secs > 0 {
			total += time.Duration(secs * float64(time.Second))
		}
	}

	return total
}
