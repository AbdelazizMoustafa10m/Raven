# T-015: Git Client Wrapper -- internal/git/client.go

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-001 |
| Blocked By | T-001 |
| Blocks | T-032, T-040, T-050, T-051 |

## Goal
Implement the `GitClient` struct that wraps all git CLI operations used by Raven: branch management, dirty-tree detection, stash/pop, diff generation, and commit log queries. This is the single abstraction through which all Raven subsystems interact with git, using `os/exec` to call the `git` CLI (same pattern as `gh`, `lazygit`, and `k9s`).

## Background
Per PRD Section 5.13, "All git operations via `os/exec` calling the `git` CLI." The `GitClient` struct provides methods: `CreateBranch`, `CurrentBranch`, `HasUncommittedChanges`, `Stash`, `StashPop`, `DiffFiles`, and `Log`. Per PRD Section 6.3, all long-running operations accept `context.Context` for cancellation.

The git client is consumed by:
- T-032 (diff generation for review): `DiffFiles`, `DiffStat`
- T-040 (PR creation): `CurrentBranch`, `HasUncommittedChanges`, `Push`
- T-050 (pipeline orchestrator): `CreateBranch`, `Checkout`
- T-051 (pipeline branch management): `CreateBranch`, `CurrentBranch`, `Checkout`, `MergeBase`

Per PRD Section 6 (Technical Decisions): "os/exec for all external tools -- Shell out to claude, codex, gemini, git, gh CLIs."

## Technical Specifications
### Implementation Approach
Create `internal/git/client.go` with a `GitClient` struct that holds the working directory path and provides methods for each git operation. Each method constructs a `exec.CommandContext`, captures stdout/stderr, and returns parsed results. Create `internal/git/recovery.go` with stash and dirty-tree recovery helpers. Errors are wrapped with context following the project convention (`fmt.Errorf("git: operation: %w", err)`).

### Key Components
- **GitClient**: Main struct with working directory and methods for all git operations
- **NewGitClient()**: Constructor that validates git is available
- **Branch operations**: CreateBranch, CurrentBranch, Checkout, BranchExists
- **Status operations**: HasUncommittedChanges, IsClean
- **Stash operations**: Stash, StashPop
- **Diff operations**: DiffFiles, DiffStat, DiffUnified
- **Log operations**: Log, HeadCommit
- **Push operations**: Push
- **Recovery helpers**: EnsureClean, StashAndRecover (in recovery.go)

### API/Interface Contracts
```go
// internal/git/client.go

// Package git provides a Git CLI wrapper for all Raven git operations.
package git

import (
    "context"
    "fmt"
    "os/exec"
    "strings"
)

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
    client := &GitClient{
        WorkDir: workDir,
        GitBin:  "git",
    }
    // Verify git is available
    if err := client.checkPrerequisites(); err != nil {
        return nil, err
    }
    return client, nil
}

// checkPrerequisites verifies that git is installed and the workDir is a git repo.
func (g *GitClient) checkPrerequisites() error

// --- Branch Operations ---

// CurrentBranch returns the name of the current branch.
func (g *GitClient) CurrentBranch(ctx context.Context) (string, error)

// CreateBranch creates a new branch from the given base and checks it out.
// If base is empty, branches from the current HEAD.
func (g *GitClient) CreateBranch(ctx context.Context, name, base string) error

// Checkout switches to the given branch.
func (g *GitClient) Checkout(ctx context.Context, branch string) error

// BranchExists checks if a branch exists locally.
func (g *GitClient) BranchExists(ctx context.Context, branch string) (bool, error)

// --- Status Operations ---

// HasUncommittedChanges returns true if the working tree has uncommitted changes
// (staged or unstaged).
func (g *GitClient) HasUncommittedChanges(ctx context.Context) (bool, error)

// IsClean returns true if the working tree and index are clean.
func (g *GitClient) IsClean(ctx context.Context) (bool, error)

// --- Stash Operations ---

// Stash stashes all uncommitted changes with the given message.
// Returns true if changes were stashed, false if working tree was clean.
func (g *GitClient) Stash(ctx context.Context, message string) (bool, error)

// StashPop pops the most recent stash entry.
func (g *GitClient) StashPop(ctx context.Context) error

// --- Diff Operations ---

// DiffFiles returns the list of changed files between the current branch and base.
// Uses `git diff --name-status <base>...HEAD`.
type DiffEntry struct {
    Status string // "A" (added), "M" (modified), "D" (deleted), "R" (renamed)
    Path   string
}

func (g *GitClient) DiffFiles(ctx context.Context, base string) ([]DiffEntry, error)

// DiffStat returns the diff stat (files changed, insertions, deletions)
// between the current branch and base.
type DiffStats struct {
    FilesChanged int
    Insertions   int
    Deletions    int
}

func (g *GitClient) DiffStat(ctx context.Context, base string) (*DiffStats, error)

// DiffUnified returns the full unified diff text between current branch and base.
func (g *GitClient) DiffUnified(ctx context.Context, base string) (string, error)

// --- Log Operations ---

// HeadCommit returns the short SHA of HEAD.
func (g *GitClient) HeadCommit(ctx context.Context) (string, error)

// Log returns the last N commit messages.
type LogEntry struct {
    SHA     string
    Message string
}

func (g *GitClient) Log(ctx context.Context, n int) ([]LogEntry, error)

// --- Push Operations ---

// Push pushes the current branch to the remote.
// If setUpstream is true, sets the upstream tracking reference.
func (g *GitClient) Push(ctx context.Context, remote string, setUpstream bool) error

// --- Internal helpers ---

// run executes a git command and returns stdout.
func (g *GitClient) run(ctx context.Context, args ...string) (string, error)

// runSilent executes a git command and returns the exit code without error wrapping.
func (g *GitClient) runSilent(ctx context.Context, args ...string) (int, string, string, error)
```

```go
// internal/git/recovery.go

// EnsureClean checks if the working tree is clean. If dirty, it stashes changes
// and returns a cleanup function that pops the stash.
func (g *GitClient) EnsureClean(ctx context.Context) (cleanup func() error, err error)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| os/exec | stdlib | Git CLI subprocess execution |
| context | stdlib | Command cancellation |
| strings | stdlib | Output parsing |
| bytes | stdlib | Output capture |
| fmt | stdlib | Error wrapping |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] `NewGitClient("")` validates git is installed
- [ ] `NewGitClient("nonexistent")` returns error
- [ ] `CurrentBranch` returns the correct branch name
- [ ] `CreateBranch` creates and checks out a new branch
- [ ] `CreateBranch` with a base branch creates from that base
- [ ] `Checkout` switches to an existing branch
- [ ] `BranchExists` returns true for existing branches, false otherwise
- [ ] `HasUncommittedChanges` returns true when there are staged or unstaged changes
- [ ] `HasUncommittedChanges` returns false when working tree is clean
- [ ] `Stash` stashes changes and returns true; returns false when clean
- [ ] `StashPop` restores stashed changes
- [ ] `DiffFiles` returns correct list of changed files with status codes
- [ ] `DiffStat` returns correct file count, insertions, and deletions
- [ ] `DiffUnified` returns the full diff text
- [ ] `HeadCommit` returns a short SHA string
- [ ] `Log(5)` returns up to 5 log entries
- [ ] `Push` executes git push with correct arguments
- [ ] `EnsureClean` returns cleanup function that pops stash
- [ ] All methods accept `context.Context` and respect cancellation
- [ ] All errors are wrapped with "git: operation:" context
- [ ] Unit tests achieve 85% coverage (using git repos in t.TempDir())
- [ ] `go vet ./...` passes

## Testing Requirements
### Unit Tests
- NewGitClient with a valid git repo: succeeds
- CurrentBranch in a repo on "main": returns "main"
- CreateBranch creates a branch visible in `git branch`
- Checkout to an existing branch changes HEAD
- BranchExists for "main" returns true, for "nonexistent" returns false
- HasUncommittedChanges: false in clean repo, true after adding a file
- Stash with changes: returns true, working tree is clean after
- StashPop: restores the stashed file
- Stash on clean repo: returns false
- DiffFiles between two branches with a new file: returns entry with "A" status
- DiffStat returns non-zero insertions when files added
- HeadCommit returns a non-empty string of expected length (7-40 chars)
- Log returns entries with SHA and message
- Context cancellation: command returns context error

### Integration Tests
- Full branch lifecycle: create branch -> make changes -> stash -> pop -> checkout original
- EnsureClean with dirty tree: cleanup function restores changes

### Edge Cases to Handle
- Git not installed: NewGitClient returns descriptive error
- Not a git repository: checkPrerequisites returns error
- Detached HEAD state: CurrentBranch returns commit SHA or error
- Branch name with slashes: "feature/my-branch" should work
- Branch name with special characters: test with common patterns
- Empty repository (no commits): some operations may fail, handle gracefully
- Very large diffs: DiffUnified with many files should not run out of memory
- Concurrent git operations: git has its own locking, but document that callers should serialize

## Implementation Notes
### Recommended Approach
1. Create `internal/git/client.go` with `GitClient` struct and constructor
2. Implement the `run()` helper that executes `git` with args, captures stdout/stderr
3. Implement each method using `run()`:
   - `CurrentBranch`: `git rev-parse --abbrev-ref HEAD`
   - `CreateBranch`: `git checkout -b <name> [<base>]`
   - `Checkout`: `git checkout <branch>`
   - `BranchExists`: `git rev-parse --verify refs/heads/<branch>`
   - `HasUncommittedChanges`: `git status --porcelain` (non-empty = dirty)
   - `Stash`: `git stash push -m <message>`
   - `StashPop`: `git stash pop`
   - `DiffFiles`: `git diff --name-status <base>...HEAD`
   - `DiffStat`: `git diff --stat <base>...HEAD`, parse last line
   - `DiffUnified`: `git diff <base>...HEAD`
   - `HeadCommit`: `git rev-parse --short HEAD`
   - `Log`: `git log --oneline -<n>`
   - `Push`: `git push [-u] <remote> <branch>`
4. Create `internal/git/recovery.go` with `EnsureClean()` using Stash/StashPop
5. For tests, create temporary git repos using `git init` in `t.TempDir()`
6. Verify: `go build ./... && go vet ./... && go test ./internal/git/...`

### Potential Pitfalls
- `git diff <base>...HEAD` (three dots) shows changes since the merge base of `base` and `HEAD`. This is what review pipelines typically need. Two dots (`base..HEAD`) shows all commits between, which may include merge commits.
- `git rev-parse --abbrev-ref HEAD` returns "HEAD" in detached state. Handle this case.
- `git status --porcelain` output includes both staged and unstaged changes. An empty output means completely clean.
- `git stash push` (not `git stash save` which is deprecated) with `-m` for a message.
- `exec.CommandContext` kills the process on context cancellation, but the git index lock may be left behind. Document this.
- Test setup must create complete git repos: `git init`, `git add`, `git commit` to have a valid history.
- Git commands may fail with a non-zero exit code and useful stderr. Always capture and include stderr in error messages.

### Security Considerations
- Git commands run with the user's credentials and permissions. No escalation needed.
- Branch names are used in `git checkout -b` -- validate they do not contain shell injection characters. Go's `exec.Command` does not invoke a shell, so this is safe by default.
- DiffUnified output may contain sensitive code -- handle appropriately downstream.

## References
- [PRD Section 5.13 - Git Integration](docs/prd/PRD-Raven.md)
- [PRD Section 6 - os/exec for all external tools](docs/prd/PRD-Raven.md)
- [Git Documentation](https://git-scm.com/docs)
- [os/exec Package](https://pkg.go.dev/os/exec)
- [lazygit git package (reference implementation)](https://github.com/jesseduffield/lazygit/tree/master/pkg/commands/git_commands)