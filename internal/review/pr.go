package review

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/log"
)

// validBranchNameRe is the allowlist for safe base-branch names.
// Only alphanumeric characters, dots, underscores, forward-slashes, and hyphens
// are permitted to prevent command injection.
var validBranchNameRe = regexp.MustCompile(`^[a-zA-Z0-9_./-]+$`)

// prNumberRe extracts a PR number from a GitHub PR URL.
// Example URL: "https://github.com/owner/repo/pull/42"
var prNumberRe = regexp.MustCompile(`/pull/(\d+)`)

// PRCreator wraps `gh pr create` subprocess execution.
// It manages PR creation lifecycle including prerequisite checks, branch
// pushing, and dry-run support.
type PRCreator struct {
	workDir string
	logger  *log.Logger
}

// PRCreateOpts specifies the options for creating a GitHub pull request.
type PRCreateOpts struct {
	// Title is the PR title. Required.
	Title string

	// Body is the PR body in Markdown. Written to a temp file to avoid shell
	// escaping issues.
	Body string

	// BaseBranch is the branch the PR targets. Defaults to "main".
	BaseBranch string

	// Draft creates the PR in draft state when true.
	Draft bool

	// Labels is a list of label names to apply to the PR.
	Labels []string

	// Assignees is a list of GitHub usernames to assign to the PR.
	Assignees []string

	// DryRun returns the planned command without executing gh.
	DryRun bool
}

// PRCreateResult is the result of a PR creation attempt.
type PRCreateResult struct {
	// URL is the HTML URL of the created PR (e.g. https://github.com/owner/repo/pull/42).
	URL string

	// Number is the PR number extracted from the URL. Zero when unavailable.
	Number int

	// Draft is true when the PR was created as a draft.
	Draft bool

	// Created is false in dry-run mode (no PR was actually created).
	Created bool

	// Command is the gh command that was or would be executed.
	Command string
}

// NewPRCreator creates a new PRCreator for the given working directory.
// logger may be nil.
func NewPRCreator(workDir string, logger *log.Logger) *PRCreator {
	return &PRCreator{
		workDir: workDir,
		logger:  logger,
	}
}

// CheckPrerequisites verifies that the gh CLI is installed, authenticated,
// and that the current branch is not the base branch (which would make a PR
// nonsensical).
func (pc *PRCreator) CheckPrerequisites(ctx context.Context, baseBranch string) error {
	if baseBranch == "" {
		baseBranch = "main"
	}

	// 1. Verify gh is installed.
	if _, _, _, err := pc.runGH(ctx, "--version"); err != nil {
		return fmt.Errorf("pr: prerequisites: gh CLI not installed or not in PATH: %w", err)
	}

	// 2. Verify gh is authenticated.
	exitCode, _, stderr, err := pc.runGH(ctx, "auth", "status")
	if exitCode == -1 {
		// The gh binary could not be started (should not happen after step 1).
		return fmt.Errorf("pr: prerequisites: checking gh auth status: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("pr: prerequisites: gh is not authenticated (run `gh auth login`): %s", strings.TrimSpace(stderr))
	}

	// 3. Verify current branch is not the base branch.
	currentBranch, err := pc.currentBranch(ctx)
	if err != nil {
		return fmt.Errorf("pr: prerequisites: %w", err)
	}
	if currentBranch == baseBranch {
		return fmt.Errorf("pr: prerequisites: current branch %q is the same as the base branch %q; switch to a feature branch first", currentBranch, baseBranch)
	}

	if pc.logger != nil {
		pc.logger.Info("pr: prerequisites satisfied",
			"branch", currentBranch,
			"base", baseBranch,
		)
	}

	return nil
}

// EnsureBranchPushed checks whether the current branch has a remote tracking
// reference on origin. If not, it pushes the branch with `-u origin <branch>`.
func (pc *PRCreator) EnsureBranchPushed(ctx context.Context) error {
	branch, err := pc.currentBranch(ctx)
	if err != nil {
		return fmt.Errorf("pr: ensure branch pushed: %w", err)
	}

	// Check whether origin/<branch> exists.
	exitCode, _, _, err := pc.runGit(ctx, "rev-parse", "--verify", "origin/"+branch)
	if err != nil && exitCode == -1 {
		// The git binary itself could not be run.
		return fmt.Errorf("pr: ensure branch pushed: checking remote ref: %w", err)
	}

	if exitCode == 0 {
		// Remote tracking branch already exists -- nothing to do.
		if pc.logger != nil {
			pc.logger.Debug("pr: branch already pushed to origin", "branch", branch)
		}
		return nil
	}

	// Push with upstream tracking.
	if pc.logger != nil {
		pc.logger.Info("pr: pushing branch to origin", "branch", branch)
	}

	_, _, pushStderr, pushErr := pc.runGit(ctx, "push", "-u", "origin", branch)
	if pushErr != nil {
		return fmt.Errorf("pr: ensure branch pushed: git push: %w -- stderr: %s", pushErr, strings.TrimSpace(pushStderr))
	}

	return nil
}

// Create creates a GitHub pull request using `gh pr create`.
//
// In dry-run mode, the function builds and returns the command string without
// executing it. The body is written to a temporary file (0600 permissions) to
// avoid shell escaping problems with arbitrary Markdown content.
func (pc *PRCreator) Create(ctx context.Context, opts PRCreateOpts) (*PRCreateResult, error) {
	if opts.BaseBranch == "" {
		opts.BaseBranch = "main"
	}

	// Validate base branch name to prevent command injection.
	if !validBranchNameRe.MatchString(opts.BaseBranch) {
		return nil, fmt.Errorf("pr: create: invalid base branch name %q: only [a-zA-Z0-9_./-] are allowed", opts.BaseBranch)
	}

	if opts.DryRun {
		return pc.dryRun(opts), nil
	}

	// Write body to a temp file with restricted permissions.
	bodyFile, err := os.CreateTemp("", "raven-pr-body-*.md")
	if err != nil {
		return nil, fmt.Errorf("pr: create: creating body temp file: %w", err)
	}
	defer os.Remove(bodyFile.Name())

	if err := bodyFile.Chmod(0600); err != nil {
		bodyFile.Close()
		return nil, fmt.Errorf("pr: create: setting body temp file permissions: %w", err)
	}

	if _, err := bodyFile.WriteString(opts.Body); err != nil {
		bodyFile.Close()
		return nil, fmt.Errorf("pr: create: writing body temp file: %w", err)
	}
	if err := bodyFile.Close(); err != nil {
		return nil, fmt.Errorf("pr: create: closing body temp file: %w", err)
	}

	// Build gh pr create arguments.
	args := []string{
		"pr", "create",
		"--title", opts.Title,
		"--body-file", bodyFile.Name(),
		"--base", opts.BaseBranch,
	}

	if opts.Draft {
		args = append(args, "--draft")
	}

	for _, label := range opts.Labels {
		args = append(args, "--label", label)
	}

	for _, assignee := range opts.Assignees {
		args = append(args, "--assignee", assignee)
	}

	cmdStr := buildCommandString("gh", args)

	if pc.logger != nil {
		pc.logger.Info("pr: creating pull request",
			"title", opts.Title,
			"base", opts.BaseBranch,
			"draft", opts.Draft,
			"labels", opts.Labels,
			"assignees", opts.Assignees,
		)
	}

	exitCode, stdout, stderr, err := pc.runGH(ctx, args...)
	if err != nil {
		// Provide a clearer message when a PR already exists for the branch.
		combined := strings.ToLower(stdout + stderr)
		if strings.Contains(combined, "already exists") || strings.Contains(combined, "pull request already") {
			return nil, fmt.Errorf("pr: create: a pull request already exists for this branch: %s", strings.TrimSpace(stderr))
		}
		return nil, fmt.Errorf("pr: create: gh pr create exited %d: %s", exitCode, strings.TrimSpace(stderr))
	}

	// Parse PR URL from stdout. gh outputs the URL as the last non-empty line.
	url := extractPRURL(stdout)

	// Extract PR number from URL.
	prNumber := extractPRNumber(url)

	if pc.logger != nil {
		pc.logger.Info("pr: pull request created",
			"url", url,
			"number", prNumber,
			"draft", opts.Draft,
		)
	}

	return &PRCreateResult{
		URL:     url,
		Number:  prNumber,
		Draft:   opts.Draft,
		Created: true,
		Command: cmdStr,
	}, nil
}

// --- private helpers --------------------------------------------------------

// dryRun builds and returns a PRCreateResult without executing any command.
func (pc *PRCreator) dryRun(opts PRCreateOpts) *PRCreateResult {
	args := []string{
		"pr", "create",
		"--title", opts.Title,
		"--body-file", "<body-tempfile>",
		"--base", opts.BaseBranch,
	}

	if opts.Draft {
		args = append(args, "--draft")
	}

	for _, label := range opts.Labels {
		args = append(args, "--label", label)
	}

	for _, assignee := range opts.Assignees {
		args = append(args, "--assignee", assignee)
	}

	cmdStr := buildCommandString("gh", args)

	if pc.logger != nil {
		pc.logger.Info("pr: dry run",
			"command", cmdStr,
			"title", opts.Title,
			"base", opts.BaseBranch,
			"draft", opts.Draft,
		)
	}

	return &PRCreateResult{
		Draft:   opts.Draft,
		Created: false,
		Command: cmdStr,
	}
}

// currentBranch returns the name of the currently checked-out branch.
func (pc *PRCreator) currentBranch(ctx context.Context) (string, error) {
	_, stdout, _, err := pc.runGit(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("getting current branch: %w", err)
	}
	branch := strings.TrimSpace(stdout)
	if branch == "HEAD" {
		return "", fmt.Errorf("repository is in detached HEAD state")
	}
	if branch == "" {
		return "", fmt.Errorf("could not determine current branch")
	}
	return branch, nil
}

// runGH executes a gh command and returns (exitCode, stdout, stderr, error).
// exitCode is -1 when the binary could not be started.
func (pc *PRCreator) runGH(ctx context.Context, args ...string) (int, string, string, error) {
	return pc.runBin(ctx, "gh", args...)
}

// runGit executes a git command and returns (exitCode, stdout, stderr, error).
// exitCode is -1 when the binary could not be started.
func (pc *PRCreator) runGit(ctx context.Context, args ...string) (int, string, string, error) {
	return pc.runBin(ctx, "git", args...)
}

// runBin executes an arbitrary binary and returns (exitCode, stdout, stderr, error).
// A non-zero exit code is returned as an error. exitCode is -1 when the binary
// itself could not be started (e.g. not in PATH).
func (pc *PRCreator) runBin(ctx context.Context, bin string, args ...string) (int, string, string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	if pc.workDir != "" {
		cmd.Dir = pc.workDir
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()

	if runErr == nil {
		return 0, stdoutBuf.String(), stderrBuf.String(), nil
	}

	if exitErr, ok := runErr.(*exec.ExitError); ok {
		code := exitErr.ExitCode()
		stdout := stdoutBuf.String()
		stderr := strings.TrimSpace(stderrBuf.String())
		return code, stdout, stderr, fmt.Errorf("exit status %d: %s", code, stderr)
	}

	// Binary could not be started.
	return -1, "", "", runErr
}

// extractPRURL returns the last non-empty line from gh output, which is
// conventionally the HTML URL of the created PR.
func extractPRURL(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

// extractPRNumber parses the PR number from a GitHub PR URL.
// Returns 0 when no number can be found.
func extractPRNumber(url string) int {
	m := prNumberRe.FindStringSubmatch(url)
	if len(m) < 2 {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

// buildCommandString assembles a human-readable command string for display or
// logging. Arguments containing spaces are single-quoted.
func buildCommandString(bin string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, bin)
	for _, a := range args {
		if strings.ContainsAny(a, " \t\n") {
			parts = append(parts, "'"+a+"'")
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}
