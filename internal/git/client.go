package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Client is the interface for git operations used by the review pipeline.
// It exposes only the diff-related methods required by diff generation.
type Client interface {
	// DiffFiles returns the list of files changed between base and HEAD.
	DiffFiles(ctx context.Context, base string) ([]DiffEntry, error)

	// DiffStat returns aggregate line-change statistics between base and HEAD.
	DiffStat(ctx context.Context, base string) (*DiffStats, error)

	// DiffUnified returns the full unified diff between base and HEAD.
	DiffUnified(ctx context.Context, base string) (string, error)

	// DiffNumStat returns per-file line-change counts between base and HEAD.
	// Each NumStatEntry holds the path, lines added, and lines deleted.
	DiffNumStat(ctx context.Context, base string) ([]NumStatEntry, error)
}

// Compile-time check: *GitClient must satisfy Client.
var _ Client = (*GitClient)(nil)

// GitClient wraps git CLI operations. All methods use os/exec to call
// the git binary, following the same pattern as gh, lazygit, and k9s.
type GitClient struct {
	// WorkDir is the working directory for git commands.
	// If empty, commands run in the current directory.
	WorkDir string

	// GitBin is the path to the git binary. Defaults to "git".
	GitBin string
}

// NewGitClient creates a new GitClient for the given working directory.
// It verifies that git is installed and accessible.
func NewGitClient(workDir string) (*GitClient, error) {
	g := &GitClient{
		WorkDir: workDir,
		GitBin:  "git",
	}
	if err := g.checkPrerequisites(); err != nil {
		return nil, fmt.Errorf("git: prerequisites: %w", err)
	}
	return g, nil
}

// checkPrerequisites verifies that git is installed and the workDir is a git repo.
func (g *GitClient) checkPrerequisites() error {
	_, err := g.run(context.Background(), "rev-parse", "--git-dir")
	if err != nil {
		return fmt.Errorf("not a git repository or git not installed: %w", err)
	}
	return nil
}

// --- Branch Operations ---

// CurrentBranch returns the name of the current branch.
// Returns an error if the repo is in a detached HEAD state.
func (g *GitClient) CurrentBranch(ctx context.Context) (string, error) {
	out, err := g.run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git: current branch: %w", err)
	}
	branch := strings.TrimSpace(out)
	if branch == "HEAD" {
		return "", fmt.Errorf("git: current branch: detached HEAD state")
	}
	return branch, nil
}

// CreateBranch creates a new branch with the given name, optionally branching
// from the given base ref. If base is empty, branches from the current HEAD.
func (g *GitClient) CreateBranch(ctx context.Context, name, base string) error {
	args := []string{"checkout", "-b", name}
	if base != "" {
		args = append(args, base)
	}
	if _, err := g.run(ctx, args...); err != nil {
		return fmt.Errorf("git: create branch %q: %w", name, err)
	}
	return nil
}

// Checkout switches to the given branch.
func (g *GitClient) Checkout(ctx context.Context, branch string) error {
	if _, err := g.run(ctx, "checkout", branch); err != nil {
		return fmt.Errorf("git: checkout %q: %w", branch, err)
	}
	return nil
}

// BranchExists reports whether the named local branch exists.
func (g *GitClient) BranchExists(ctx context.Context, branch string) (bool, error) {
	exitCode, stdout, _, err := g.runSilent(ctx, "rev-parse", "--verify", "refs/heads/"+branch)
	if err != nil && exitCode == -1 {
		// exec itself failed (e.g., git binary not found).
		return false, fmt.Errorf("git: branch exists %q: %w", branch, err)
	}
	// exitCode 0 with non-empty stdout means the ref was found.
	return exitCode == 0 && strings.TrimSpace(stdout) != "", nil
}

// --- Status Operations ---

// HasUncommittedChanges reports whether the working tree has uncommitted changes.
func (g *GitClient) HasUncommittedChanges(ctx context.Context) (bool, error) {
	out, err := g.run(ctx, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git: status: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}

// IsClean reports whether the working tree is clean (no uncommitted changes).
func (g *GitClient) IsClean(ctx context.Context) (bool, error) {
	dirty, err := g.HasUncommittedChanges(ctx)
	if err != nil {
		return false, err
	}
	return !dirty, nil
}

// --- Stash Operations ---

// Stash stashes current changes with the given message.
// Returns true if changes were stashed, false if the working tree was already clean
// or if there were only untracked files (which git stash does not stash by default).
func (g *GitClient) Stash(ctx context.Context, message string) (bool, error) {
	dirty, err := g.HasUncommittedChanges(ctx)
	if err != nil {
		return false, fmt.Errorf("git: stash: checking status: %w", err)
	}
	if !dirty {
		return false, nil
	}
	out, err := g.run(ctx, "stash", "push", "-m", message)
	if err != nil {
		return false, fmt.Errorf("git: stash push: %w", err)
	}
	// git stash outputs "No local changes to save" when there is nothing to stash
	// (e.g., only untracked files and -u was not passed). In that case no stash
	// entry was created, so we must return false to prevent a spurious StashPop.
	if strings.Contains(out, "No local changes to save") {
		return false, nil
	}
	return true, nil
}

// StashPop pops the most recent stash entry.
func (g *GitClient) StashPop(ctx context.Context) error {
	if _, err := g.run(ctx, "stash", "pop"); err != nil {
		return fmt.Errorf("git: stash pop: %w", err)
	}
	return nil
}

// --- Diff Operations ---

// DiffEntry represents a single file in a diff.
type DiffEntry struct {
	// Status is the single-character status code from git:
	// "A" (added), "M" (modified), "D" (deleted), "R" (renamed).
	Status string
	// Path is the file path relative to the repository root.
	Path string
}

// DiffFiles returns a list of files changed between base and HEAD.
func (g *GitClient) DiffFiles(ctx context.Context, base string) ([]DiffEntry, error) {
	out, err := g.run(ctx, "diff", "--name-status", base+"...HEAD")
	if err != nil {
		return nil, fmt.Errorf("git: diff files from %q: %w", base, err)
	}
	return parseDiffNameStatus(out), nil
}

// parseDiffNameStatus parses the output of `git diff --name-status`.
func parseDiffNameStatus(output string) []DiffEntry {
	var entries []DiffEntry
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 {
			continue
		}
		status := strings.TrimSpace(parts[0])
		// Rename entries look like "R100\told\tnew" — take first char and last field.
		if strings.HasPrefix(status, "R") {
			status = "R"
			// For renames the path is the destination (second tab-field after status).
			subparts := strings.SplitN(parts[1], "\t", 2)
			path := subparts[len(subparts)-1]
			entries = append(entries, DiffEntry{Status: status, Path: strings.TrimSpace(path)})
		} else {
			entries = append(entries, DiffEntry{Status: status, Path: strings.TrimSpace(parts[1])})
		}
	}
	return entries
}

// DiffStats summarises the number of changed files and line counts.
type DiffStats struct {
	FilesChanged int
	Insertions   int
	Deletions    int
}

// DiffStat returns aggregate change statistics between base and HEAD.
func (g *GitClient) DiffStat(ctx context.Context, base string) (*DiffStats, error) {
	out, err := g.run(ctx, "diff", "--stat", base+"...HEAD")
	if err != nil {
		return nil, fmt.Errorf("git: diff stat from %q: %w", base, err)
	}
	stats, err := parseDiffStat(out)
	if err != nil {
		return nil, fmt.Errorf("git: diff stat parse: %w", err)
	}
	return stats, nil
}

// parseDiffStat parses the summary line produced by `git diff --stat`.
// Example summary lines:
//
//	"3 files changed, 45 insertions(+), 12 deletions(-)"
//	"1 file changed, 5 insertions(+)"
//	"1 file changed, 3 deletions(-)"
func parseDiffStat(output string) (*DiffStats, error) {
	stats := &DiffStats{}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return stats, nil
	}
	// The summary line is always the last non-empty line.
	var summary string
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			summary = strings.TrimSpace(lines[i])
			break
		}
	}
	if summary == "" {
		return stats, nil
	}

	// Split on ", " to get individual segments.
	segments := strings.Split(summary, ", ")
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		switch {
		case strings.Contains(seg, "file"):
			// "3 files changed" or "1 file changed"
			n, err := parseLeadingInt(seg)
			if err != nil {
				return nil, fmt.Errorf("parsing files changed %q: %w", seg, err)
			}
			stats.FilesChanged = n
		case strings.Contains(seg, "insertion"):
			n, err := parseLeadingInt(seg)
			if err != nil {
				return nil, fmt.Errorf("parsing insertions %q: %w", seg, err)
			}
			stats.Insertions = n
		case strings.Contains(seg, "deletion"):
			n, err := parseLeadingInt(seg)
			if err != nil {
				return nil, fmt.Errorf("parsing deletions %q: %w", seg, err)
			}
			stats.Deletions = n
		}
	}
	return stats, nil
}

// parseLeadingInt extracts the leading integer from a string like "3 files changed".
func parseLeadingInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	spaceIdx := strings.IndexByte(s, ' ')
	if spaceIdx < 0 {
		return 0, fmt.Errorf("no space found in %q", s)
	}
	return strconv.Atoi(s[:spaceIdx])
}

// DiffUnified returns the full unified diff between base and HEAD.
func (g *GitClient) DiffUnified(ctx context.Context, base string) (string, error) {
	out, err := g.run(ctx, "diff", base+"...HEAD")
	if err != nil {
		return "", fmt.Errorf("git: diff unified from %q: %w", base, err)
	}
	return out, nil
}

// NumStatEntry holds per-file line-change counts from git diff --numstat.
type NumStatEntry struct {
	// Path is the file path relative to the repository root.
	// For renames, Path is the destination path.
	Path string

	// OldPath is non-empty for renamed files and holds the source path.
	OldPath string

	// Added is the number of lines added. -1 if the file is binary.
	Added int

	// Deleted is the number of lines deleted. -1 if the file is binary.
	Deleted int
}

// DiffNumStat returns per-file line-change counts between base and HEAD using
// git diff --numstat. Binary files are returned with Added and Deleted set to -1.
func (g *GitClient) DiffNumStat(ctx context.Context, base string) ([]NumStatEntry, error) {
	out, err := g.run(ctx, "diff", "--numstat", base+"...HEAD")
	if err != nil {
		return nil, fmt.Errorf("git: diff numstat from %q: %w", base, err)
	}
	return parseNumStat(out), nil
}

// parseNumStat parses the output of `git diff --numstat`.
// Each normal line is: "<added>\t<deleted>\t<path>".
// Binary files produce:   "-\t-\t<path>".
// Renamed files produce:  "<added>\t<deleted>\t{old => new}" or two path variants.
func parseNumStat(output string) []NumStatEntry {
	var entries []NumStatEntry
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		addedStr := strings.TrimSpace(parts[0])
		deletedStr := strings.TrimSpace(parts[1])
		rawPath := strings.TrimSpace(parts[2])

		entry := NumStatEntry{}

		// Parse lines added/deleted; "-" means binary file.
		if addedStr == "-" {
			entry.Added = -1
		} else {
			n, err := strconv.Atoi(addedStr)
			if err == nil {
				entry.Added = n
			}
		}
		if deletedStr == "-" {
			entry.Deleted = -1
		} else {
			n, err := strconv.Atoi(deletedStr)
			if err == nil {
				entry.Deleted = n
			}
		}

		// Handle rename notation "{old => new}" embedded in path.
		// Example: "src/{foo => bar}/file.go" or "old.go => new.go"
		if strings.Contains(rawPath, " => ") {
			oldPath, newPath := parseRenamePath(rawPath)
			entry.OldPath = oldPath
			entry.Path = newPath
		} else {
			entry.Path = rawPath
		}

		entries = append(entries, entry)
	}
	return entries
}

// parseRenamePath splits a git rename path like "{old => new}" or "old => new"
// into its source and destination components.
func parseRenamePath(rawPath string) (oldPath, newPath string) {
	// Handle inline brace notation: "prefix/{old => new}/suffix"
	openBrace := strings.Index(rawPath, "{")
	closeBrace := strings.Index(rawPath, "}")
	if openBrace >= 0 && closeBrace > openBrace {
		prefix := rawPath[:openBrace]
		suffix := rawPath[closeBrace+1:]
		inner := rawPath[openBrace+1 : closeBrace]
		halves := strings.SplitN(inner, " => ", 2)
		if len(halves) == 2 {
			oldPath = strings.TrimSuffix(prefix+halves[0]+suffix, "/")
			newPath = strings.TrimSuffix(prefix+halves[1]+suffix, "/")
			return oldPath, newPath
		}
	}
	// Simple "old => new" without braces.
	halves := strings.SplitN(rawPath, " => ", 2)
	if len(halves) == 2 {
		return strings.TrimSpace(halves[0]), strings.TrimSpace(halves[1])
	}
	// Fallback: treat as non-rename.
	return "", rawPath
}

// --- Log Operations ---

// HeadCommit returns the short SHA of the current HEAD commit.
func (g *GitClient) HeadCommit(ctx context.Context) (string, error) {
	out, err := g.run(ctx, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git: head commit: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// LogEntry represents a single commit in the log.
type LogEntry struct {
	SHA     string
	Message string
}

// Log returns the n most recent log entries in short format.
func (g *GitClient) Log(ctx context.Context, n int) ([]LogEntry, error) {
	out, err := g.run(ctx, "log", "--oneline", fmt.Sprintf("-%d", n))
	if err != nil {
		return nil, fmt.Errorf("git: log: %w", err)
	}
	return parseOneline(out), nil
}

// parseOneline parses the output of `git log --oneline`.
// Each line is: "<short-sha> <message>".
func parseOneline(output string) []LogEntry {
	var entries []LogEntry
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		entry := LogEntry{SHA: parts[0]}
		if len(parts) == 2 {
			entry.Message = parts[1]
		}
		entries = append(entries, entry)
	}
	return entries
}

// --- Fetch Operations ---

// Fetch fetches from the named remote. If remote is empty, fetches from "origin".
func (g *GitClient) Fetch(ctx context.Context, remote string) error {
	if remote == "" {
		remote = "origin"
	}
	if _, err := g.run(ctx, "fetch", remote); err != nil {
		return fmt.Errorf("git: fetch %s: %w", remote, err)
	}
	return nil
}

// --- Push Operations ---

// Push pushes the current branch to the named remote.
// If setUpstream is true, sets the upstream tracking reference (-u).
func (g *GitClient) Push(ctx context.Context, remote string, setUpstream bool) error {
	branch, err := g.CurrentBranch(ctx)
	if err != nil {
		return fmt.Errorf("git: push: %w", err)
	}
	args := []string{"push"}
	if setUpstream {
		args = append(args, "-u")
	}
	args = append(args, remote, branch)
	if _, err := g.run(ctx, args...); err != nil {
		return fmt.Errorf("git: push %s %s: %w", remote, branch, err)
	}
	return nil
}

// --- Internal helpers ---

// run executes a git command and returns stdout.
// stderr is included in the error message when the command fails.
func (g *GitClient) run(ctx context.Context, args ...string) (string, error) {
	_, stdout, stderr, err := g.runSilent(ctx, args...)
	if err != nil {
		return "", err
	}
	if stdout == "" && stderr != "" {
		// Some git commands (e.g., checkout) write to stderr on success.
		return stderr, nil
	}
	return stdout, nil
}

// runSilent executes a git command and returns the exit code, stdout, stderr,
// and an error. The error is non-nil for both exec failures (exitCode=-1, e.g.
// git binary not found) and non-zero git exits (exitCode>0). Callers that need
// to distinguish the two cases check whether exitCode == -1.
func (g *GitClient) runSilent(ctx context.Context, args ...string) (int, string, string, error) {
	bin := g.GitBin
	if bin == "" {
		bin = "git"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = g.WorkDir

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
			// Non-zero exit is not an exec error — return it as a wrapped error
			// so callers that need it can detect the exit code.
			stderr := strings.TrimSpace(stderrBuf.String())
			stdout := strings.TrimSpace(stdoutBuf.String())
			return exitCode, stdout, stderr, fmt.Errorf("exit status %d: %s", exitCode, stderr)
		}
		// The process could not be started at all.
		return -1, "", "", runErr
	}

	return exitCode, stdoutBuf.String(), stderrBuf.String(), nil
}
