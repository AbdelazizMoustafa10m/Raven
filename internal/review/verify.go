package review

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/log"
)

// maxOutputBytes is the threshold above which command output is truncated.
// Outputs larger than 1 MiB are reduced to the first and last 512 lines.
const maxOutputBytes = 1024 * 1024

// truncationLines is the number of lines to keep from the head and tail of
// oversized output.
const truncationLines = 512

// VerificationStatus represents the overall pass/fail outcome of a
// verification run.
type VerificationStatus string

const (
	// VerificationPassed indicates all commands exited with code 0.
	VerificationPassed VerificationStatus = "passed"

	// VerificationFailed indicates at least one command exited with non-zero
	// code or timed out.
	VerificationFailed VerificationStatus = "failed"
)

// CommandResult holds the outcome of a single verification command execution.
type CommandResult struct {
	// Command is the original command string that was executed.
	Command string

	// ExitCode is the process exit code. It is -1 when the process could not
	// be started or when the command timed out before it exited.
	ExitCode int

	// Stdout is the captured standard output of the command.
	Stdout string

	// Stderr is the captured standard error of the command.
	Stderr string

	// Duration is the wall-clock time elapsed from start to finish, including
	// timeout enforcement.
	Duration time.Duration

	// Passed is true when ExitCode == 0 and TimedOut is false.
	Passed bool

	// TimedOut is true when the per-command deadline was exceeded.
	TimedOut bool
}

// VerificationReport summarises the outcomes of all verification commands run
// during a single VerificationRunner.Run call.
type VerificationReport struct {
	// Status is VerificationPassed only when every command passed.
	Status VerificationStatus

	// Results holds one CommandResult per executed command in order.
	Results []CommandResult

	// Duration is the total wall-clock time for the entire run.
	Duration time.Duration

	// Passed is the count of commands that exited with code 0.
	Passed int

	// Failed is the count of commands that exited with a non-zero code or
	// timed out.
	Failed int

	// Total is the total number of commands that were attempted.
	Total int
}

// VerificationRunner executes a configured sequence of shell commands and
// collects pass/fail results with per-command timeouts.
type VerificationRunner struct {
	commands []string
	workDir  string
	timeout  time.Duration // per-command timeout
	logger   *log.Logger
}

// NewVerificationRunner creates a VerificationRunner.
//
//   - commands is the ordered list of shell commands to execute. Empty strings
//     are skipped at run time with a warning.
//   - workDir is the working directory for every command. An empty string uses
//     the process working directory.
//   - timeout is the per-command deadline. A zero or negative value disables
//     the per-command timeout.
//   - logger may be nil; when non-nil it receives structured log lines for
//     each command start, pass, and failure.
func NewVerificationRunner(
	commands []string,
	workDir string,
	timeout time.Duration,
	logger *log.Logger,
) *VerificationRunner {
	// Defensive copy so the caller cannot mutate the slice after construction.
	cmds := make([]string, len(commands))
	copy(cmds, commands)

	return &VerificationRunner{
		commands: cmds,
		workDir:  workDir,
		timeout:  timeout,
		logger:   logger,
	}
}

// Run executes all configured verification commands sequentially and returns a
// consolidated VerificationReport.
//
// When stopOnFailure is true Run stops at the first command that does not pass
// and returns partial results for the commands that were attempted.
//
// The returned error is non-nil only for unexpected infrastructure failures
// (e.g. the parent context was cancelled before any command started). A
// command failure does NOT cause Run to return an error; it is represented in
// the VerificationReport instead.
func (vr *VerificationRunner) Run(ctx context.Context, stopOnFailure bool) (*VerificationReport, error) {
	start := time.Now()

	results := make([]CommandResult, 0, len(vr.commands))
	passed := 0
	failed := 0

	for _, cmd := range vr.commands {
		// Honour parent-context cancellation between commands.
		if err := ctx.Err(); err != nil {
			return vr.buildReport(results, passed, failed, time.Since(start)), nil
		}

		if strings.TrimSpace(cmd) == "" {
			if vr.logger != nil {
				vr.logger.Warn("verification: skipping empty command")
			}
			continue
		}

		result, err := vr.RunSingle(ctx, cmd)
		if err != nil {
			// RunSingle only propagates context-level errors.
			return vr.buildReport(results, passed, failed, time.Since(start)), fmt.Errorf("verification: running %q: %w", cmd, err)
		}

		results = append(results, *result)

		if result.Passed {
			passed++
		} else {
			failed++
			if stopOnFailure {
				break
			}
		}
	}

	return vr.buildReport(results, passed, failed, time.Since(start)), nil
}

// RunSingle executes a single command string in the configured working
// directory and returns a CommandResult. It applies the per-command timeout
// when one is configured.
//
// RunSingle returns a non-nil error only when the parent context is cancelled
// before the command finishes. Command failures (non-zero exit codes, timeouts)
// are represented in the returned CommandResult with Passed == false.
func (vr *VerificationRunner) RunSingle(ctx context.Context, command string) (*CommandResult, error) {
	cmdStart := time.Now()

	if vr.logger != nil {
		vr.logger.Info("verification: running command", "command", command)
	}

	// Build the execution context: wrap with per-command timeout if configured.
	execCtx := ctx
	var cancel context.CancelFunc
	if vr.timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, vr.timeout)
		defer cancel()
	}

	// Choose the shell wrapper based on the OS.
	var shellCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		shellCmd = exec.CommandContext(execCtx, "cmd", "/c", command)
	} else {
		shellCmd = exec.CommandContext(execCtx, "sh", "-c", command)
	}

	if vr.workDir != "" {
		shellCmd.Dir = vr.workDir
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	shellCmd.Stdout = &stdoutBuf
	shellCmd.Stderr = &stderrBuf

	runErr := shellCmd.Run()
	duration := time.Since(cmdStart)

	exitCode := 0
	timedOut := false

	if runErr != nil {
		// Determine whether the failure was a timeout (per-command deadline).
		if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			timedOut = true
			exitCode = -1
			// Kill the process if it is still running after timeout.
			if shellCmd.Process != nil {
				_ = shellCmd.Process.Kill()
			}
		} else if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			// The parent context was cancelled; propagate to the caller so
			// that Run() can stop the loop cleanly.
			return nil, fmt.Errorf("verification: context cancelled while running %q: %w", command, ctx.Err())
		} else {
			// Extract the exit code from the process error.
			var exitErr *exec.ExitError
			if errors.As(runErr, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else {
				// Unknown process-start failure.
				exitCode = -1
			}
		}
	}

	stdout := truncateOutput(stdoutBuf.String())
	stderr := truncateOutput(stderrBuf.String())

	passed := exitCode == 0 && !timedOut

	result := &CommandResult{
		Command:  command,
		ExitCode: exitCode,
		Stdout:   stdout,
		Stderr:   stderr,
		Duration: duration,
		Passed:   passed,
		TimedOut: timedOut,
	}

	if vr.logger != nil {
		if passed {
			vr.logger.Info("verification: command passed",
				"command", command,
				"duration", duration,
			)
		} else {
			vr.logger.Warn("verification: command failed",
				"command", command,
				"exit_code", exitCode,
				"timed_out", timedOut,
				"duration", duration,
			)
		}
	}

	return result, nil
}

// FormatReport produces a human-readable terminal summary of the verification
// results, suitable for direct output to a terminal or log file.
//
// Example output:
//
//	Verification Results
//	--------------------
//	✓ go build ./cmd/raven/ (1.23s)
//	✗ go test ./...        (5.67s)
//	  FAIL    github.com/example/raven [build failed]
//
//	Summary: 1 passed, 1 failed, 2 total (6.90s)
func (vr *VerificationReport) FormatReport() string {
	var sb strings.Builder

	sb.WriteString("Verification Results\n")
	sb.WriteString("--------------------\n")

	for _, r := range vr.Results {
		indicator := "✓"
		if !r.Passed {
			indicator = "✗"
		}
		fmt.Fprintf(&sb, "%s %s (%s)\n", indicator, r.Command, formatDuration(r.Duration))

		if !r.Passed {
			// Indent failure output for readability. Prefer Stderr when
			// available; otherwise fall back to Stdout.
			output := r.Stderr
			if strings.TrimSpace(output) == "" {
				output = r.Stdout
			}
			if r.TimedOut {
				output = fmt.Sprintf("command timed out after %s", formatDuration(r.Duration))
			}
			for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
				fmt.Fprintf(&sb, "  %s\n", line)
			}
		}
	}

	sb.WriteString("\n")
	fmt.Fprintf(&sb, "Summary: %d passed, %d failed, %d total (%s)\n",
		vr.Passed, vr.Failed, vr.Total, formatDuration(vr.Duration))

	return sb.String()
}

// FormatMarkdown produces a GitHub-compatible markdown section summarising the
// verification results. Failed command output is wrapped in a collapsible
// <details> block to keep PR bodies concise.
//
// Example output:
//
//	## Verification Results
//
//	| Command | Status | Duration |
//	|---------|--------|----------|
//	| `go build ./cmd/raven/` | ✅ Passed | 1.23s |
//	| `go test ./...` | ❌ Failed | 5.67s |
//
//	<details>
//	<summary>go test ./... output</summary>
//
//	```
//	FAIL    github.com/example/raven [build failed]
//	```
//
//	</details>
//
//	**Overall: 1/2 passed**
func (vr *VerificationReport) FormatMarkdown() string {
	var sb strings.Builder

	sb.WriteString("## Verification Results\n\n")

	sb.WriteString("| Command | Status | Duration |\n")
	sb.WriteString("|---------|--------|----------|\n")

	for _, r := range vr.Results {
		status := "✅ Passed"
		if !r.Passed {
			if r.TimedOut {
				status = "⏱️ Timed Out"
			} else {
				status = "❌ Failed"
			}
		}
		fmt.Fprintf(&sb, "| `%s` | %s | %s |\n",
			escapeMarkdownCode(r.Command),
			status,
			formatDuration(r.Duration),
		)
	}

	// Emit collapsible details blocks for commands that produced output on
	// failure.
	for _, r := range vr.Results {
		if r.Passed {
			continue
		}

		output := strings.TrimSpace(r.Stderr)
		if output == "" {
			output = strings.TrimSpace(r.Stdout)
		}
		if r.TimedOut {
			output = fmt.Sprintf("command timed out after %s", formatDuration(r.Duration))
		}
		if output == "" {
			continue
		}

		fmt.Fprintf(&sb, "\n<details>\n<summary>%s output</summary>\n\n", r.Command)
		sb.WriteString("```\n")
		sb.WriteString(output)
		sb.WriteString("\n```\n\n</details>\n")
	}

	sb.WriteString("\n")
	fmt.Fprintf(&sb, "**Overall: %d/%d passed**\n", vr.Passed, vr.Total)

	return sb.String()
}

// buildReport constructs a VerificationReport from accumulated results.
func (vr *VerificationRunner) buildReport(
	results []CommandResult,
	passed, failed int,
	duration time.Duration,
) *VerificationReport {
	status := VerificationPassed
	if failed > 0 {
		status = VerificationFailed
	}

	return &VerificationReport{
		Status:   status,
		Results:  results,
		Duration: duration,
		Passed:   passed,
		Failed:   failed,
		Total:    passed + failed,
	}
}

// truncateOutput returns the output unchanged when it is within maxOutputBytes.
// When it exceeds the limit, it keeps the first truncationLines lines and the
// last truncationLines lines with a truncation notice in between.
func truncateOutput(output string) string {
	if len(output) <= maxOutputBytes {
		return output
	}

	lines := strings.Split(output, "\n")
	if len(lines) <= truncationLines*2 {
		// The line count is small but the byte count is large (e.g. very long
		// lines). Truncate by byte, keeping room for the notice so the total
		// output length is strictly less than the original input.
		const notice = "\n... (output truncated)"
		cutoff := maxOutputBytes - len(notice)
		if cutoff < 0 {
			cutoff = 0
		}
		if cutoff > len(output) {
			cutoff = len(output)
		}
		return output[:cutoff] + notice
	}

	head := lines[:truncationLines]
	tail := lines[len(lines)-truncationLines:]
	omitted := len(lines) - truncationLines*2

	var sb strings.Builder
	sb.WriteString(strings.Join(head, "\n"))
	fmt.Fprintf(&sb, "\n\n... (%d lines omitted) ...\n\n", omitted)
	sb.WriteString(strings.Join(tail, "\n"))

	return sb.String()
}

// formatDuration returns a human-readable duration string with two decimal
// places of seconds (e.g. "1.23s"). It does not use time.Duration.String()
// because that format is less readable for humans scanning terminal output.
func formatDuration(d time.Duration) string {
	return fmt.Sprintf("%.2fs", d.Seconds())
}

// escapeMarkdownCode escapes backtick characters inside inline code spans to
// prevent them from breaking GitHub Flavored Markdown rendering.
func escapeMarkdownCode(s string) string {
	return strings.ReplaceAll(s, "`", "\\`")
}
