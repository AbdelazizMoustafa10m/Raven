# T-032: Git Diff Generation and Risk Classification

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-015, T-031, T-009 |
| Blocked By | T-015, T-031 |
| Blocks | T-033, T-035 |

## Goal
Implement diff generation and risk classification for the review pipeline. This component runs `git diff` against a base branch, parses the output into structured changed-file data, filters files by configured extensions, and classifies each changed file by risk level using configurable regex patterns. The output drives both review prompt construction and file assignment in split review mode.

## Background
Per PRD Section 5.5, the review pipeline begins by generating a git diff of changed files with risk classification. PRD Section 5.13 specifies that all git operations use `os/exec` calling the `git` CLI, wrapped in a `GitClient` struct (T-015). The `[review]` config section in `raven.toml` provides `extensions` (regex to filter reviewable files) and `risk_patterns` (regex to classify high-risk paths). Risk classification is used to prioritize findings and to split files across agents in `split` review mode.

## Technical Specifications
### Implementation Approach
Create `internal/review/diff.go` containing a `DiffGenerator` struct that uses the git client (T-015) to run `git diff --name-status` and `git diff --stat` against a base branch. Parse the output into a list of `ChangedFile` structs with file path, change type (added/modified/deleted/renamed), and risk level. Filter files using the configured `extensions` regex and classify risk using the `risk_patterns` regex. Also generate the full unified diff text for embedding in review prompts.

### Key Components
- **DiffGenerator**: Orchestrates diff generation, filtering, and classification
- **ChangedFile**: A single changed file with metadata (path, change type, risk, lines added/deleted)
- **DiffResult**: Complete diff output containing file list, stats, and full diff text
- **RiskLevel**: Classification of file risk (high, normal, low)
- **FileSplitter**: Splits files across agents for `split` review mode

### API/Interface Contracts
```go
// internal/review/diff.go

type ChangeType string

const (
    ChangeAdded    ChangeType = "added"
    ChangeModified ChangeType = "modified"
    ChangeDeleted  ChangeType = "deleted"
    ChangeRenamed  ChangeType = "renamed"
)

type RiskLevel string

const (
    RiskHigh   RiskLevel = "high"
    RiskNormal RiskLevel = "normal"
    RiskLow    RiskLevel = "low"
)

type ChangedFile struct {
    Path       string
    ChangeType ChangeType
    Risk       RiskLevel
    LinesAdded int
    LinesDeleted int
    OldPath    string // for renames
}

type DiffResult struct {
    Files       []ChangedFile
    FullDiff    string // unified diff text for prompt inclusion
    BaseBranch  string
    Stats       DiffStats
}

type DiffStats struct {
    TotalFiles   int
    FilesAdded   int
    FilesModified int
    FilesDeleted int
    FilesRenamed int
    TotalLinesAdded   int
    TotalLinesDeleted int
    HighRiskFiles int
}

type DiffGenerator struct {
    gitClient    git.Client
    extensions   *regexp.Regexp
    riskPatterns *regexp.Regexp
    logger       *log.Logger
}

func NewDiffGenerator(gitClient git.Client, cfg ReviewConfig, logger *log.Logger) (*DiffGenerator, error)

// Generate produces a DiffResult by running git diff against baseBranch.
func (d *DiffGenerator) Generate(ctx context.Context, baseBranch string) (*DiffResult, error)

// SplitFiles divides changed files across N agents for split review mode.
// High-risk files are distributed first to ensure even coverage.
func SplitFiles(files []ChangedFile, n int) [][]ChangedFile
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/git (T-015) | - | Git client for executing git diff commands |
| internal/review (T-031) | - | ReviewConfig type |
| regexp | stdlib | File extension and risk pattern matching |
| strings | stdlib | Parsing git diff output |
| strconv | stdlib | Parsing line counts from diff stat |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] Generates diff against a configurable base branch using `git diff --name-status` for file list
- [ ] Generates full unified diff text using `git diff` for prompt inclusion
- [ ] Filters files by configured `extensions` regex (only reviewable file types included)
- [ ] Classifies files as high-risk when path matches `risk_patterns` regex
- [ ] Parses change types correctly: A (added), M (modified), D (deleted), R (renamed)
- [ ] DiffStats accurately counts files by type and lines added/deleted
- [ ] SplitFiles evenly distributes files across N agents, prioritizing high-risk files
- [ ] Handles empty diff (no changes) gracefully with empty DiffResult
- [ ] Unit tests achieve 90% coverage
- [ ] Works with detached HEAD (compares against base branch by name)

## Testing Requirements
### Unit Tests
- Parse `git diff --name-status` output with A, M, D, R status codes
- Filter files: `.go` files included, `.png` files excluded by default extensions regex
- Risk classification: `internal/auth/handler.go` is high-risk, `README.md` is normal-risk
- DiffStats correctly tallied for mixed change types
- SplitFiles with 6 files across 2 agents: 3 files each
- SplitFiles with 5 files across 3 agents: 2, 2, 1 distribution
- SplitFiles with high-risk files distributed first
- Empty diff output produces empty file list and zero stats
- Renamed files parsed with both old and new paths

### Integration Tests
- Generate diff in a test git repository with known changes (using t.TempDir)
- Full pipeline: generate diff, filter, classify, split

### Edge Cases to Handle
- Binary files in diff output (should be excluded)
- Very large diffs (>10000 files) -- ensure no excessive memory allocation
- Files with spaces or special characters in names
- Merge commits with multiple parents
- No common ancestor between branches (unrelated histories)
- Renamed files with similarity percentage (R095)
- Base branch does not exist locally (need to fetch)

## Implementation Notes
### Recommended Approach
1. Run `git diff --name-status <baseBranch>...HEAD` to get the list of changed files with status codes
2. Run `git diff --numstat <baseBranch>...HEAD` to get lines added/deleted per file
3. Run `git diff <baseBranch>...HEAD` to get the full unified diff text
4. Parse name-status output line by line: status code is first field, path is second (tab-separated)
5. For renames (R\d+), parse both old and new paths
6. Apply extensions filter using `regexp.MatchString` on each file path
7. Apply risk classification using `regexp.MatchString` on each file path
8. Compile results into DiffResult with stats

### Potential Pitfalls
- Use three-dot diff (`baseBranch...HEAD`) not two-dot (`baseBranch..HEAD`) to get changes since the common ancestor, which is what reviewers typically want
- The `extensions` config value is a regex string that needs `regexp.Compile` -- handle compile errors at DiffGenerator construction time, not at diff generation time
- Renamed files have the format `RXXX\told_path\tnew_path` where XXX is the similarity percentage
- `git diff --name-status` output for copies uses `C` status -- treat like additions
- Large diffs may exceed memory if captured as a single string -- consider streaming for the full diff text

### Security Considerations
- Validate base branch name to prevent command injection (alphanumeric, hyphens, slashes, dots only)
- Do not include file contents from outside the repository in the diff

## References
- [PRD Section 5.5 - Generates git diff (changed files, risk classification)](docs/prd/PRD-Raven.md)
- [PRD Section 5.13 - Git Integration](docs/prd/PRD-Raven.md)
- [git diff documentation](https://git-scm.com/docs/git-diff)
- [go-gitdiff library](https://github.com/bluekeyes/go-gitdiff)
