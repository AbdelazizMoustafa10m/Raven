# T-051: Pipeline Branch Management

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-015, T-009, T-050 |
| Blocked By | T-015 |
| Blocks | T-055 |

## Goal
Implement the branch management subsystem for the pipeline orchestrator: creating phase branches from configurable templates, chaining branches so each phase branches from the previous phase's branch, and handling branch switching during pipeline execution. This ensures each phase's work is cleanly isolated on its own branch with a correct parent lineage.

## Background
Per PRD Section 5.9:
- "Branch management: creates branches from a configurable template (`phase/{phase_id}-{slug}`)"
- "Phase chaining: in multi-phase mode, each phase's branch is based on the previous phase's branch"

Per PRD Section 5.13, the git client wrapper (T-015) provides `CreateBranch`, `CurrentBranch`, and related operations. This task builds the pipeline-specific branch logic on top of that git client.

Branch template variables include `{phase_id}` (phase number), `{slug}` (phase name kebab-cased), and `{project}` (project name from config). The template is configured in `raven.toml` under `project.branch_template` (default: `phase/{phase_id}-{slug}`).

## Technical Specifications
### Implementation Approach
Create `internal/pipeline/branch.go` with a `BranchManager` struct that uses the git client (T-015) and config to create and switch branches according to the pipeline's phase sequence. The manager resolves branch names from templates, ensures the correct base branch for each phase, and handles the create-and-switch operation.

### Key Components
- **BranchManager**: Creates and manages branches for pipeline phases
- **ResolveBranchName**: Applies template variables to branch template string
- **CreatePhaseBranch**: Creates branch from correct base (main for first phase, previous phase's branch for subsequent)
- **SwitchToPhaseBranch**: Checks out existing phase branch (for resume scenarios)
- **BranchExists**: Checks if a phase branch already exists (for resume)

### API/Interface Contracts
```go
// internal/pipeline/branch.go

type BranchManager struct {
    gitClient      *git.Client
    branchTemplate string // e.g., "phase/{phase_id}-{slug}"
    baseBranch     string // e.g., "main"
}

func NewBranchManager(gitClient *git.Client, branchTemplate, baseBranch string) *BranchManager

// ResolveBranchName applies template variables to create a branch name.
// Variables: {phase_id}, {slug}, {project}
func (b *BranchManager) ResolveBranchName(phaseID int, phaseName string, projectName string) string

// CreatePhaseBranch creates a new branch for the given phase.
// For the first phase, branches from baseBranch.
// For subsequent phases, branches from previousPhaseBranch.
// Returns the created branch name.
func (b *BranchManager) CreatePhaseBranch(ctx context.Context, opts PhaseBranchOpts) (string, error)

type PhaseBranchOpts struct {
    PhaseID             int
    PhaseName           string
    ProjectName         string
    PreviousPhaseBranch string // empty for first phase
    SyncBase            bool   // fetch and fast-forward base from origin
}

// SwitchToPhaseBranch checks out an existing phase branch.
// Returns error if the branch does not exist.
func (b *BranchManager) SwitchToPhaseBranch(ctx context.Context, branchName string) error

// BranchExists checks if a branch name exists locally.
func (b *BranchManager) BranchExists(ctx context.Context, branchName string) (bool, error)

// EnsureBranch creates the branch if it does not exist, or switches to it if it does.
// Used by resume logic.
func (b *BranchManager) EnsureBranch(ctx context.Context, opts PhaseBranchOpts) (string, error)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/git (T-015) | - | Git operations (create branch, checkout, fetch) |
| internal/config (T-009) | - | Branch template from config |
| strings | stdlib | Template variable replacement |
| regexp | stdlib | Slug generation from phase name |

## Acceptance Criteria
- [ ] ResolveBranchName correctly substitutes {phase_id}, {slug}, {project} variables
- [ ] Slug generation converts phase name to kebab-case (lowercase, spaces to hyphens, strip special chars)
- [ ] First phase branches from baseBranch (e.g., main)
- [ ] Subsequent phases branch from the previous phase's branch
- [ ] CreatePhaseBranch calls git to create and checkout the new branch
- [ ] SyncBase flag causes fetch + fast-forward of base branch before branching
- [ ] BranchExists correctly detects existing branches
- [ ] EnsureBranch creates if new, switches if existing (idempotent)
- [ ] Error when git operations fail (not a git repo, branch already exists on create)
- [ ] Context cancellation stops long-running git operations
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- ResolveBranchName with "phase/{phase_id}-{slug}": produces "phase/1-foundation-setup"
- ResolveBranchName with "{project}/phase-{phase_id}": produces "raven/phase-1"
- Slug from "Foundation & Setup": produces "foundation-setup"
- Slug from "Phase 1: Init": produces "phase-1-init"
- Slug from name with unicode: removes non-ASCII or transliterates
- CreatePhaseBranch for phase 1 (no previous): branches from main
- CreatePhaseBranch for phase 2 with previous "phase/1-foundation": branches from that
- SyncBase true: fetch is called before branch creation
- BranchExists with mock git: returns true for existing branch
- BranchExists with mock git: returns false for nonexistent branch
- EnsureBranch creates new branch when it does not exist
- EnsureBranch switches to existing branch when it does exist

### Integration Tests
- Create 3 chained branches in a test git repo: verify each has correct parent
- Resume scenario: branches exist, EnsureBranch switches without creating

### Edge Cases to Handle
- Empty branch template (use default "phase/{phase_id}-{slug}")
- Phase name with characters invalid for git branch names (sanitize)
- Branch already exists on CreatePhaseBranch (should this error or switch? -- error, use EnsureBranch for idempotent behavior)
- Git repo in detached HEAD state
- Remote-only branch (not yet checked out locally)
- Empty phase name (use phase ID only)

## Implementation Notes
### Recommended Approach
1. `ResolveBranchName`: use `strings.NewReplacer` for template variable substitution
2. Slug generation: lowercase, replace `[^a-z0-9-]` with hyphen, collapse multiple hyphens, trim leading/trailing hyphens
3. `CreatePhaseBranch`:
   a. Determine base: if PreviousPhaseBranch is empty, use baseBranch; else use PreviousPhaseBranch
   b. If SyncBase, call gitClient.Fetch(ctx) and gitClient.FastForward(ctx, base)
   c. Resolve branch name from template
   d. Call gitClient.CreateBranch(ctx, branchName, base)
   e. Call gitClient.Checkout(ctx, branchName)
4. `EnsureBranch`: call BranchExists first, then create or switch

### Potential Pitfalls
- Git branch names have restrictions: no spaces, no `..`, no `~`, no `^`, no `:`, no `\`, no ASCII control chars. Sanitize the slug accordingly.
- Phase chaining: if the previous phase's branch has new commits (from its implementation), the next phase should branch from the HEAD of that branch, not from where it was created. Use `git checkout -b new-branch previous-branch` (which uses HEAD of previous-branch)
- SyncBase may fail if there is no remote configured (common in local-only repos) -- handle gracefully with warning

### Security Considerations
- Branch names derived from user-provided phase names must be sanitized before use in git commands (prevent shell injection)
- Validate branch template does not contain shell metacharacters

## References
- [PRD Section 5.9 - Branch management](docs/prd/PRD-Raven.md)
- [PRD Section 5.13 - Git integration](docs/prd/PRD-Raven.md)
- [Git branch naming rules](https://git-scm.com/docs/git-check-ref-format)