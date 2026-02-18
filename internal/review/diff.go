package review

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/log"

	"github.com/AbdelazizMoustafa10m/Raven/internal/git"
)

// ChangeType describes how a file changed in the diff.
type ChangeType string

const (
	// ChangeAdded indicates the file was newly created.
	ChangeAdded ChangeType = "added"

	// ChangeModified indicates the file was modified in place.
	ChangeModified ChangeType = "modified"

	// ChangeDeleted indicates the file was removed.
	ChangeDeleted ChangeType = "deleted"

	// ChangeRenamed indicates the file was moved or renamed.
	ChangeRenamed ChangeType = "renamed"
)

// RiskLevel classifies a file's review risk.
type RiskLevel string

const (
	// RiskHigh marks files that match a configured risk pattern and warrant
	// closer review scrutiny.
	RiskHigh RiskLevel = "high"

	// RiskNormal is the default classification for files without a risk pattern
	// match.
	RiskNormal RiskLevel = "normal"

	// RiskLow marks files that are unlikely to contain meaningful logic changes
	// (e.g. generated files, lock files).
	RiskLow RiskLevel = "low"
)

// ChangedFile represents a single file from the diff along with its
// classification metadata.
type ChangedFile struct {
	// Path is the file path relative to the repository root. For renamed files
	// this is the destination path.
	Path string

	// ChangeType categorises the change: added, modified, deleted, or renamed.
	ChangeType ChangeType

	// Risk classifies review risk: high, normal, or low.
	Risk RiskLevel

	// LinesAdded is the number of lines added. 0 for binary files or deletes.
	LinesAdded int

	// LinesDeleted is the number of lines deleted. 0 for binary files or adds.
	LinesDeleted int

	// OldPath is the original path before a rename. Empty for non-renames.
	OldPath string
}

// DiffStats summarises the overall diff at a high level.
type DiffStats struct {
	// TotalFiles is the total number of files touched.
	TotalFiles int

	// FilesAdded is the count of new files.
	FilesAdded int

	// FilesModified is the count of modified files.
	FilesModified int

	// FilesDeleted is the count of deleted files.
	FilesDeleted int

	// FilesRenamed is the count of renamed files.
	FilesRenamed int

	// TotalLinesAdded is the aggregate number of inserted lines.
	TotalLinesAdded int

	// TotalLinesDeleted is the aggregate number of deleted lines.
	TotalLinesDeleted int

	// HighRiskFiles is the number of files classified as RiskHigh.
	HighRiskFiles int
}

// DiffResult holds the complete output of a diff generation run.
type DiffResult struct {
	// Files is the filtered, classified list of changed files.
	Files []ChangedFile

	// FullDiff is the unified diff text, ready for inclusion in review prompts.
	FullDiff string

	// BaseBranch is the git ref that was diffed against HEAD.
	BaseBranch string

	// Stats is the aggregate summary of the diff.
	Stats DiffStats
}

// validBranchName is compiled once and used to guard against branch names that
// could be interpreted as additional git flags or shell injection vectors.
// It rejects names with consecutive dots (e.g. "main..HEAD") which are git
// range operators that could alter command semantics.
var validBranchName = regexp.MustCompile(`^[a-zA-Z0-9_./-]+$`)

// DiffGenerator orchestrates diff generation, file filtering, and risk
// classification for the review pipeline.
type DiffGenerator struct {
	gitClient    git.Client
	extensions   *regexp.Regexp // nil means "accept all"
	riskPatterns *regexp.Regexp // nil means "no high-risk classification"
	logger       *log.Logger
}

// NewDiffGenerator creates a DiffGenerator configured from the supplied
// ReviewConfig. It compiles the Extensions and RiskPatterns regular expressions
// eagerly so that configuration errors are surfaced at construction time rather
// than during a review run.
func NewDiffGenerator(gitClient git.Client, cfg ReviewConfig, logger *log.Logger) (*DiffGenerator, error) {
	if gitClient == nil {
		return nil, fmt.Errorf("review: NewDiffGenerator: gitClient is required")
	}

	dg := &DiffGenerator{
		gitClient: gitClient,
		logger:    logger,
	}

	if cfg.Extensions != "" {
		re, err := regexp.Compile(cfg.Extensions)
		if err != nil {
			return nil, fmt.Errorf("review: NewDiffGenerator: invalid extensions regex %q: %w", cfg.Extensions, err)
		}
		dg.extensions = re
	}

	if cfg.RiskPatterns != "" {
		re, err := regexp.Compile(cfg.RiskPatterns)
		if err != nil {
			return nil, fmt.Errorf("review: NewDiffGenerator: invalid risk_patterns regex %q: %w", cfg.RiskPatterns, err)
		}
		dg.riskPatterns = re
	}

	return dg, nil
}

// Generate produces a DiffResult by diffing HEAD against baseBranch.
// An empty diff (no changed files) returns a valid DiffResult with an empty
// Files slice, empty FullDiff, and zero Stats.
func (d *DiffGenerator) Generate(ctx context.Context, baseBranch string) (*DiffResult, error) {
	if !validBranchName.MatchString(baseBranch) || strings.Contains(baseBranch, "..") {
		return nil, fmt.Errorf("review: generate: invalid base branch %q: must match ^[a-zA-Z0-9_./-]+$ with no consecutive dots", baseBranch)
	}

	// Fetch all three data sources concurrently would be nice, but the git
	// client is a sequential CLI wrapper. Run them sequentially to keep the
	// implementation simple and avoid interleaved stderr.
	entries, err := d.gitClient.DiffFiles(ctx, baseBranch)
	if err != nil {
		return nil, fmt.Errorf("review: generate: listing changed files: %w", err)
	}

	numStats, err := d.gitClient.DiffNumStat(ctx, baseBranch)
	if err != nil {
		return nil, fmt.Errorf("review: generate: fetching numstat: %w", err)
	}

	fullDiff, err := d.gitClient.DiffUnified(ctx, baseBranch)
	if err != nil {
		return nil, fmt.Errorf("review: generate: fetching unified diff: %w", err)
	}

	// Build a lookup from path -> numstat entry for O(1) line-count access.
	numStatByPath := make(map[string]git.NumStatEntry, len(numStats))
	for _, ns := range numStats {
		numStatByPath[ns.Path] = ns
	}

	// Convert git.DiffEntry list into []ChangedFile, applying filtering and
	// risk classification.
	files := make([]ChangedFile, 0, len(entries))
	for _, entry := range entries {
		cf := d.buildChangedFile(entry, numStatByPath)

		// Apply extension filter if configured.
		if d.extensions != nil && !d.extensions.MatchString(cf.Path) {
			if d.logger != nil {
				d.logger.Debug("skipping file: extension filter",
					"path", cf.Path,
					"filter", d.extensions.String(),
				)
			}
			continue
		}

		files = append(files, cf)
	}

	result := &DiffResult{
		Files:      files,
		FullDiff:   fullDiff,
		BaseBranch: baseBranch,
		Stats:      computeStats(files),
	}

	if d.logger != nil {
		d.logger.Info("diff generated",
			"base", baseBranch,
			"files", result.Stats.TotalFiles,
			"high_risk", result.Stats.HighRiskFiles,
			"lines_added", result.Stats.TotalLinesAdded,
			"lines_deleted", result.Stats.TotalLinesDeleted,
		)
	}

	return result, nil
}

// buildChangedFile converts a single git.DiffEntry and its associated numstat
// data into a ChangedFile with risk classification applied.
func (d *DiffGenerator) buildChangedFile(entry git.DiffEntry, numStatByPath map[string]git.NumStatEntry) ChangedFile {
	cf := ChangedFile{
		Path:       entry.Path,
		ChangeType: statusToChangeType(entry.Status),
		Risk:       RiskNormal,
	}

	// Populate line counts from numstat.
	if ns, ok := numStatByPath[entry.Path]; ok {
		if ns.Added > 0 {
			cf.LinesAdded = ns.Added
		}
		if ns.Deleted > 0 {
			cf.LinesDeleted = ns.Deleted
		}
		// Carry over OldPath for renames detected by numstat.
		if ns.OldPath != "" {
			cf.OldPath = ns.OldPath
		}
	}

	// Apply risk classification.
	if d.riskPatterns != nil && d.riskPatterns.MatchString(cf.Path) {
		cf.Risk = RiskHigh
	}

	return cf
}

// statusToChangeType maps a git status letter to a ChangeType constant.
func statusToChangeType(status string) ChangeType {
	switch {
	case status == "A" || status == "C": // Added or Copied
		return ChangeAdded
	case status == "D":
		return ChangeDeleted
	case status == "R":
		return ChangeRenamed
	default:
		return ChangeModified
	}
}

// computeStats aggregates a slice of ChangedFile into a DiffStats summary.
func computeStats(files []ChangedFile) DiffStats {
	s := DiffStats{TotalFiles: len(files)}
	for _, f := range files {
		switch f.ChangeType {
		case ChangeAdded:
			s.FilesAdded++
		case ChangeModified:
			s.FilesModified++
		case ChangeDeleted:
			s.FilesDeleted++
		case ChangeRenamed:
			s.FilesRenamed++
		}
		s.TotalLinesAdded += f.LinesAdded
		s.TotalLinesDeleted += f.LinesDeleted
		if f.Risk == RiskHigh {
			s.HighRiskFiles++
		}
	}
	return s
}

// SplitFiles divides changed files across n agents for split review mode.
// High-risk files are distributed first to ensure even coverage across agents.
// Files are distributed in round-robin order after sorting by risk (high first).
//
// Special cases:
//   - When n <= 0, an empty slice is returned.
//   - When n >= len(files), each non-empty bucket gets at most one file.
func SplitFiles(files []ChangedFile, n int) [][]ChangedFile {
	if n <= 0 || len(files) == 0 {
		return nil
	}

	// Sort so that high-risk files are distributed first.
	sorted := make([]ChangedFile, len(files))
	copy(sorted, files)
	sort.SliceStable(sorted, func(i, j int) bool {
		return riskOrder(sorted[i].Risk) < riskOrder(sorted[j].Risk)
	})

	buckets := n
	if len(files) < n {
		buckets = len(files)
	}
	result := make([][]ChangedFile, buckets)
	for idx, f := range sorted {
		bucket := idx % buckets
		result[bucket] = append(result[bucket], f)
	}
	return result
}

// riskOrder returns a sort key for RiskLevel: high=0, normal=1, low=2.
func riskOrder(r RiskLevel) int {
	switch r {
	case RiskHigh:
		return 0
	case RiskNormal:
		return 1
	default:
		return 2
	}
}
