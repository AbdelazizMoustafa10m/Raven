# T-040: PR Creation via gh CLI

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-015, T-039 |
| Blocked By | T-015, T-039 |
| Blocks | T-042 |

## Goal
Implement the PR creation component that invokes `gh pr create` via `os/exec` to create GitHub pull requests with AI-generated bodies. Supports configurable base branch, draft PRs, label assignment, and dry-run mode. This is the final step of the review pipeline and the pipeline orchestrator.

## Background
Per PRD Section 5.7, PR creation uses the `gh` CLI (well-established pattern, avoids GitHub API authentication complexity). The PR body comes from the PRBodyGenerator (T-039). The component supports `--draft` for draft PRs, configurable base branch (default: `main`), and dry-run mode that shows the PR body without creating the PR. The `gh` CLI must be installed and authenticated -- the component checks prerequisites before attempting creation.

## Technical Specifications
### Implementation Approach
Create `internal/review/pr.go` containing a `PRCreator` struct that wraps `gh pr create` invocation via `os/exec`. It takes a title, body, base branch, and options (draft, labels, assignees). Before creation, it checks that `gh` is installed and authenticated, that the current branch is not the base branch, and that the branch has been pushed to the remote. Supports dry-run mode by returning the planned command and body without execution.

### Key Components
- **PRCreator**: Wraps `gh pr create` subprocess execution
- **PRCreateOpts**: Options for PR creation (title, body, base, draft, labels, etc.)
- **PRCreateResult**: Result of PR creation (PR URL, PR number, success/failure)
- **PrerequisiteChecker**: Validates gh CLI availability and authentication

### API/Interface Contracts
```go
// internal/review/pr.go

type PRCreator struct {
    workDir string
    logger  *log.Logger
}

type PRCreateOpts struct {
    Title      string
    Body       string
    BaseBranch string   // default: "main"
    Draft      bool
    Labels     []string
    Assignees  []string
    DryRun     bool
}

type PRCreateResult struct {
    URL       string
    Number    int
    Draft     bool
    Created   bool // false in dry-run mode
    Command   string // the gh command that was/would be executed
}

func NewPRCreator(workDir string, logger *log.Logger) *PRCreator

// CheckPrerequisites verifies that gh CLI is installed, authenticated,
// and the current branch is suitable for PR creation.
func (pc *PRCreator) CheckPrerequisites(ctx context.Context, baseBranch string) error

// Create creates a GitHub PR using gh pr create.
// In dry-run mode, returns the planned command and body without executing.
func (pc *PRCreator) Create(ctx context.Context, opts PRCreateOpts) (*PRCreateResult, error)

// EnsureBranchPushed checks if the current branch exists on the remote
// and pushes if necessary.
func (pc *PRCreator) EnsureBranchPushed(ctx context.Context) error
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| os/exec | stdlib | Subprocess execution for gh CLI |
| context | stdlib | Timeout and cancellation |
| strings | stdlib | Parsing gh output for PR URL |
| regexp | stdlib | Extracting PR number from URL |
| internal/git (T-015) | - | Git operations (current branch, push) |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] CheckPrerequisites verifies `gh` is installed (`gh --version`)
- [ ] CheckPrerequisites verifies `gh` is authenticated (`gh auth status`)
- [ ] CheckPrerequisites verifies current branch is not the base branch
- [ ] Creates PR using `gh pr create --title <title> --body <body> --base <base>`
- [ ] Supports `--draft` flag via `gh pr create --draft`
- [ ] Supports labels via `gh pr create --label <label>` (repeated for each)
- [ ] Supports assignees via `gh pr create --assignee <user>` (repeated for each)
- [ ] Extracts PR URL from `gh pr create` stdout output
- [ ] DryRun returns the planned command and body without executing `gh`
- [ ] EnsureBranchPushed pushes the current branch to origin if not already pushed
- [ ] Returns clear error if `gh` is not installed or not authenticated
- [ ] Unit tests achieve 85% coverage

## Testing Requirements
### Unit Tests
- CheckPrerequisites with gh available and authenticated: no error
- CheckPrerequisites with gh not installed: clear error message
- CheckPrerequisites with gh not authenticated: clear error message
- CheckPrerequisites on base branch: error "cannot create PR from base branch"
- Create with all options: correct gh command constructed
- Create with --draft: command includes --draft flag
- Create with labels: command includes --label flags
- DryRun returns command string and body without executing
- PR URL extracted from gh output
- EnsureBranchPushed when branch not on remote: pushes
- EnsureBranchPushed when branch already on remote: no-op

### Integration Tests
- Full PR creation flow in a test repository (requires gh CLI and GitHub access)
- DryRun produces expected output in a real git repository

### Edge Cases to Handle
- PR already exists for this branch: gh returns error -- provide helpful message
- Body exceeds GitHub's 65536 character limit: truncate with note
- Title with special characters (quotes, backticks): properly escaped in command
- Network error during gh pr create: captured and reported
- gh CLI version incompatibility: check minimum version
- Branch has no commits ahead of base: gh may warn or error
- Remote not configured: error before attempting push

## Implementation Notes
### Recommended Approach
1. Implement CheckPrerequisites:
   a. Run `gh --version` -- if error, gh not installed
   b. Run `gh auth status` -- if non-zero exit, not authenticated
   c. Get current branch via git client -- compare with baseBranch
2. Implement EnsureBranchPushed:
   a. Run `git rev-parse --verify origin/<branch>` to check remote tracking
   b. If not found, run `git push -u origin <branch>`
3. Implement Create:
   a. If DryRun, build command string and return without executing
   b. Write body to a temp file to avoid shell escaping issues
   c. Build command: `gh pr create --title <title> --body-file <tempfile> --base <base>`
   d. Add `--draft` if opts.Draft is true
   e. Add `--label` for each label, `--assignee` for each assignee
   f. Execute command, capture stdout
   g. Parse PR URL from stdout (first line typically)
   h. Extract PR number from URL using regex

### Potential Pitfalls
- Shell escaping: PR body may contain markdown, quotes, backticks, dollar signs -- use `--body-file` with a temp file instead of `--body` inline to avoid escaping issues
- gh CLI output format may vary between versions -- parse defensively
- `gh pr create` may open an interactive editor if title/body are not provided -- always provide both
- The temp file for `--body-file` must be cleaned up after creation (use `defer os.Remove`)
- On some systems, `gh` may be a snap or homebrew package with different PATH behavior

### Security Considerations
- PR body may contain code snippets from the review -- this is intentional and acceptable
- Do not pass GitHub tokens via command line -- `gh` handles authentication internally
- Temp file for body should be created with restricted permissions (0600)
- Validate base branch name to prevent command injection

## References
- [PRD Section 5.7 - PR Creation](docs/prd/PRD-Raven.md)
- [gh pr create documentation](https://cli.github.com/manual/gh_pr_create)
- [Go os/exec documentation](https://pkg.go.dev/os/exec)
- [gh CLI Go library](https://pkg.go.dev/github.com/cli/go-gh/v2)
