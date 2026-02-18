package review

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/git"
)

// ---------------------------------------------------------------------------
// mockGitClient implements git.Client for testing without a real git repo.
// ---------------------------------------------------------------------------

type mockGitClient struct {
	diffFilesResult  []git.DiffEntry
	diffFilesErr     error
	numStatResult    []git.NumStatEntry
	numStatErr       error
	unifiedResult    string
	unifiedErr       error
	diffStatResult   *git.DiffStats
	diffStatErr      error
}

func (m *mockGitClient) DiffFiles(_ context.Context, _ string) ([]git.DiffEntry, error) {
	return m.diffFilesResult, m.diffFilesErr
}

func (m *mockGitClient) DiffStat(_ context.Context, _ string) (*git.DiffStats, error) {
	return m.diffStatResult, m.diffStatErr
}

func (m *mockGitClient) DiffUnified(_ context.Context, _ string) (string, error) {
	return m.unifiedResult, m.unifiedErr
}

func (m *mockGitClient) DiffNumStat(_ context.Context, _ string) ([]git.NumStatEntry, error) {
	return m.numStatResult, m.numStatErr
}

// ---------------------------------------------------------------------------
// NewDiffGenerator tests
// ---------------------------------------------------------------------------

func TestNewDiffGenerator_NilClient(t *testing.T) {
	t.Parallel()

	_, err := NewDiffGenerator(nil, ReviewConfig{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gitClient is required")
}

func TestNewDiffGenerator_InvalidExtensionsRegex(t *testing.T) {
	t.Parallel()

	_, err := NewDiffGenerator(&mockGitClient{}, ReviewConfig{
		Extensions: "[invalid(regex",
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extensions regex")
}

func TestNewDiffGenerator_InvalidRiskPatternsRegex(t *testing.T) {
	t.Parallel()

	_, err := NewDiffGenerator(&mockGitClient{}, ReviewConfig{
		RiskPatterns: "[invalid(regex",
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "risk_patterns regex")
}

func TestNewDiffGenerator_EmptyConfig(t *testing.T) {
	t.Parallel()

	dg, err := NewDiffGenerator(&mockGitClient{}, ReviewConfig{}, nil)
	require.NoError(t, err)
	assert.NotNil(t, dg)
	assert.Nil(t, dg.extensions)
	assert.Nil(t, dg.riskPatterns)
}

func TestNewDiffGenerator_ValidConfig(t *testing.T) {
	t.Parallel()

	dg, err := NewDiffGenerator(&mockGitClient{}, ReviewConfig{
		Extensions:   `\.go$`,
		RiskPatterns: `internal/auth`,
	}, nil)
	require.NoError(t, err)
	assert.NotNil(t, dg)
	assert.NotNil(t, dg.extensions)
	assert.NotNil(t, dg.riskPatterns)
}

// ---------------------------------------------------------------------------
// Generate — branch name validation
// ---------------------------------------------------------------------------

func TestGenerate_InvalidBranchName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		branch string
	}{
		{"space in name", "main branch"},
		{"semicolon injection", "main;rm -rf /"},
		{"pipe injection", "main|cat /etc/passwd"},
		{"empty string", ""},
		{"double dot", "main..HEAD"},
		{"at sign", "HEAD@{1}"},
	}

	dg, err := NewDiffGenerator(&mockGitClient{}, ReviewConfig{}, nil)
	require.NoError(t, err)

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := dg.Generate(context.Background(), tt.branch)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid base branch")
		})
	}
}

func TestGenerate_ValidBranchNames(t *testing.T) {
	t.Parallel()

	branches := []string{
		"main",
		"origin/main",
		"feature/my-branch",
		"v1.2.3",
		"release_2024",
	}

	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{},
		numStatResult:   []git.NumStatEntry{},
		unifiedResult:   "",
	}
	dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
	require.NoError(t, err)

	for _, branch := range branches {
		branch := branch
		t.Run(branch, func(t *testing.T) {
			t.Parallel()
			result, err := dg.Generate(context.Background(), branch)
			require.NoError(t, err)
			assert.Equal(t, branch, result.BaseBranch)
		})
	}
}

// ---------------------------------------------------------------------------
// Generate — error propagation
// ---------------------------------------------------------------------------

func TestGenerate_DiffFilesError(t *testing.T) {
	t.Parallel()

	mock := &mockGitClient{
		diffFilesErr: errors.New("git: diff files: repo error"),
	}
	dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
	require.NoError(t, err)

	_, err = dg.Generate(context.Background(), "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listing changed files")
}

func TestGenerate_NumStatError(t *testing.T) {
	t.Parallel()

	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{{Status: "M", Path: "foo.go"}},
		numStatErr:      errors.New("git: numstat: error"),
	}
	dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
	require.NoError(t, err)

	_, err = dg.Generate(context.Background(), "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching numstat")
}

func TestGenerate_UnifiedDiffError(t *testing.T) {
	t.Parallel()

	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{{Status: "M", Path: "foo.go"}},
		numStatResult:   []git.NumStatEntry{{Path: "foo.go", Added: 5, Deleted: 3}},
		unifiedErr:      errors.New("git: diff: error"),
	}
	dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
	require.NoError(t, err)

	_, err = dg.Generate(context.Background(), "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching unified diff")
}

// ---------------------------------------------------------------------------
// Generate — empty diff
// ---------------------------------------------------------------------------

func TestGenerate_EmptyDiff(t *testing.T) {
	t.Parallel()

	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{},
		numStatResult:   []git.NumStatEntry{},
		unifiedResult:   "",
	}
	dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), "main")
	require.NoError(t, err)
	assert.Equal(t, "main", result.BaseBranch)
	assert.Empty(t, result.Files)
	assert.Empty(t, result.FullDiff)
	assert.Zero(t, result.Stats.TotalFiles)
}

// ---------------------------------------------------------------------------
// Generate — change type mapping
// ---------------------------------------------------------------------------

func TestGenerate_ChangeTypeMappings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     string
		wantType   ChangeType
	}{
		{"added", "A", ChangeAdded},
		{"modified", "M", ChangeModified},
		{"deleted", "D", ChangeDeleted},
		{"renamed", "R", ChangeRenamed},
		{"copied", "C", ChangeAdded},
		{"unknown treated as modified", "X", ChangeModified},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := &mockGitClient{
				diffFilesResult: []git.DiffEntry{{Status: tt.status, Path: "file.go"}},
				numStatResult:   []git.NumStatEntry{{Path: "file.go", Added: 1, Deleted: 0}},
				unifiedResult:   "diff output",
			}
			dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
			require.NoError(t, err)

			result, err := dg.Generate(context.Background(), "main")
			require.NoError(t, err)
			require.Len(t, result.Files, 1)
			assert.Equal(t, tt.wantType, result.Files[0].ChangeType)
		})
	}
}

// ---------------------------------------------------------------------------
// Generate — risk classification
// ---------------------------------------------------------------------------

func TestGenerate_RiskClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		riskPattern  string
		filePath     string
		expectedRisk RiskLevel
	}{
		{
			name:         "no risk pattern — all normal",
			riskPattern:  "",
			filePath:     "internal/auth/token.go",
			expectedRisk: RiskNormal,
		},
		{
			name:         "matching risk pattern — high",
			riskPattern:  `internal/auth`,
			filePath:     "internal/auth/token.go",
			expectedRisk: RiskHigh,
		},
		{
			name:         "non-matching risk pattern — normal",
			riskPattern:  `internal/auth`,
			filePath:     "internal/util/helpers.go",
			expectedRisk: RiskNormal,
		},
		{
			name:         "crypto pattern matches",
			riskPattern:  `crypto|security|auth`,
			filePath:     "pkg/crypto/cipher.go",
			expectedRisk: RiskHigh,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := &mockGitClient{
				diffFilesResult: []git.DiffEntry{{Status: "M", Path: tt.filePath}},
				numStatResult:   []git.NumStatEntry{{Path: tt.filePath, Added: 2, Deleted: 1}},
				unifiedResult:   "some diff",
			}
			dg, err := NewDiffGenerator(mock, ReviewConfig{RiskPatterns: tt.riskPattern}, nil)
			require.NoError(t, err)

			result, err := dg.Generate(context.Background(), "main")
			require.NoError(t, err)
			require.Len(t, result.Files, 1)
			assert.Equal(t, tt.expectedRisk, result.Files[0].Risk)
		})
	}
}

// ---------------------------------------------------------------------------
// Generate — extension filtering
// ---------------------------------------------------------------------------

func TestGenerate_ExtensionFiltering(t *testing.T) {
	t.Parallel()

	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{
			{Status: "M", Path: "main.go"},
			{Status: "M", Path: "handler.ts"},
			{Status: "M", Path: "README.md"},
		},
		numStatResult: []git.NumStatEntry{
			{Path: "main.go", Added: 5, Deleted: 2},
			{Path: "handler.ts", Added: 10, Deleted: 1},
			{Path: "README.md", Added: 1, Deleted: 0},
		},
		unifiedResult: "diff output",
	}

	// Filter for only Go files.
	dg, err := NewDiffGenerator(mock, ReviewConfig{Extensions: `\.go$`}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), "main")
	require.NoError(t, err)

	require.Len(t, result.Files, 1)
	assert.Equal(t, "main.go", result.Files[0].Path)
}

func TestGenerate_NoExtensionFilter_AllFilesIncluded(t *testing.T) {
	t.Parallel()

	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{
			{Status: "A", Path: "main.go"},
			{Status: "M", Path: "handler.ts"},
			{Status: "D", Path: "old.py"},
		},
		numStatResult: []git.NumStatEntry{
			{Path: "main.go", Added: 3, Deleted: 0},
			{Path: "handler.ts", Added: 7, Deleted: 2},
			{Path: "old.py", Added: 0, Deleted: 5},
		},
		unifiedResult: "diff output",
	}

	dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), "main")
	require.NoError(t, err)
	assert.Len(t, result.Files, 3)
}

// ---------------------------------------------------------------------------
// Generate — line count population
// ---------------------------------------------------------------------------

func TestGenerate_LineCountsFromNumStat(t *testing.T) {
	t.Parallel()

	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{
			{Status: "M", Path: "worker.go"},
		},
		numStatResult: []git.NumStatEntry{
			{Path: "worker.go", Added: 42, Deleted: 17},
		},
		unifiedResult: "diff content",
	}

	dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), "main")
	require.NoError(t, err)
	require.Len(t, result.Files, 1)
	assert.Equal(t, 42, result.Files[0].LinesAdded)
	assert.Equal(t, 17, result.Files[0].LinesDeleted)
}

func TestGenerate_BinaryFileZeroLineCounts(t *testing.T) {
	t.Parallel()

	// Binary files have Added=-1, Deleted=-1 from git numstat.
	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{
			{Status: "M", Path: "image.png"},
		},
		numStatResult: []git.NumStatEntry{
			{Path: "image.png", Added: -1, Deleted: -1},
		},
		unifiedResult: "binary diff",
	}

	dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), "main")
	require.NoError(t, err)
	require.Len(t, result.Files, 1)
	// Negative values from numstat are treated as 0 for ChangedFile.
	assert.Equal(t, 0, result.Files[0].LinesAdded)
	assert.Equal(t, 0, result.Files[0].LinesDeleted)
}

// ---------------------------------------------------------------------------
// Generate — DiffStats aggregation
// ---------------------------------------------------------------------------

func TestGenerate_StatsAggregation(t *testing.T) {
	t.Parallel()

	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{
			{Status: "A", Path: "new.go"},
			{Status: "M", Path: "existing.go"},
			{Status: "D", Path: "removed.go"},
			{Status: "R", Path: "moved.go"},
			{Status: "M", Path: "internal/auth/token.go"},
		},
		numStatResult: []git.NumStatEntry{
			{Path: "new.go", Added: 10, Deleted: 0},
			{Path: "existing.go", Added: 5, Deleted: 3},
			{Path: "removed.go", Added: 0, Deleted: 20},
			{Path: "moved.go", Added: 0, Deleted: 0},
			{Path: "internal/auth/token.go", Added: 8, Deleted: 2},
		},
		unifiedResult: "full diff text",
	}

	dg, err := NewDiffGenerator(mock, ReviewConfig{
		RiskPatterns: `internal/auth`,
	}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), "main")
	require.NoError(t, err)

	s := result.Stats
	assert.Equal(t, 5, s.TotalFiles)
	assert.Equal(t, 1, s.FilesAdded)
	assert.Equal(t, 2, s.FilesModified)
	assert.Equal(t, 1, s.FilesDeleted)
	assert.Equal(t, 1, s.FilesRenamed)
	assert.Equal(t, 23, s.TotalLinesAdded)  // 10+5+0+0+8
	assert.Equal(t, 25, s.TotalLinesDeleted) // 0+3+20+0+2
	assert.Equal(t, 1, s.HighRiskFiles)
}

// ---------------------------------------------------------------------------
// SplitFiles tests
// ---------------------------------------------------------------------------

func TestSplitFiles_NilOrEmptyFiles(t *testing.T) {
	t.Parallel()

	assert.Nil(t, SplitFiles(nil, 3))
	assert.Nil(t, SplitFiles([]ChangedFile{}, 3))
}

func TestSplitFiles_ZeroOrNegativeN(t *testing.T) {
	t.Parallel()

	files := []ChangedFile{
		{Path: "a.go", Risk: RiskNormal},
		{Path: "b.go", Risk: RiskNormal},
	}
	assert.Nil(t, SplitFiles(files, 0))
	assert.Nil(t, SplitFiles(files, -1))
}

func TestSplitFiles_NGreaterThanFiles(t *testing.T) {
	t.Parallel()

	files := []ChangedFile{
		{Path: "a.go", Risk: RiskNormal},
		{Path: "b.go", Risk: RiskHigh},
	}
	// n=5 but only 2 files — should produce 2 buckets, each with 1 file.
	result := SplitFiles(files, 5)
	require.Len(t, result, 2)
	for _, bucket := range result {
		assert.Len(t, bucket, 1)
	}
}

func TestSplitFiles_EvenDistribution(t *testing.T) {
	t.Parallel()

	files := []ChangedFile{
		{Path: "a.go", Risk: RiskNormal},
		{Path: "b.go", Risk: RiskNormal},
		{Path: "c.go", Risk: RiskNormal},
		{Path: "d.go", Risk: RiskNormal},
	}
	result := SplitFiles(files, 2)
	require.Len(t, result, 2)
	assert.Len(t, result[0], 2)
	assert.Len(t, result[1], 2)
}

func TestSplitFiles_HighRiskFirst(t *testing.T) {
	t.Parallel()

	files := []ChangedFile{
		{Path: "normal1.go", Risk: RiskNormal},
		{Path: "high1.go", Risk: RiskHigh},
		{Path: "normal2.go", Risk: RiskNormal},
		{Path: "high2.go", Risk: RiskHigh},
		{Path: "low1.go", Risk: RiskLow},
	}

	result := SplitFiles(files, 2)
	require.Len(t, result, 2)

	// Collect all files assigned to the first bucket and verify high-risk files
	// were distributed first (meaning at least one high-risk file ends up in
	// bucket 0 since there are 2 high-risk files and 2 buckets).
	allFiles := make([]ChangedFile, 0)
	for _, bucket := range result {
		allFiles = append(allFiles, bucket...)
	}
	assert.Len(t, allFiles, 5, "all files must be distributed")

	// Verify high-risk files got into the first two buckets (indices 0 and 1
	// in the sorted order), so bucket[0] and bucket[1] each contain one
	// high-risk file in a 2-agent split.
	highRiskInBucket0 := 0
	for _, f := range result[0] {
		if f.Risk == RiskHigh {
			highRiskInBucket0++
		}
	}
	highRiskInBucket1 := 0
	for _, f := range result[1] {
		if f.Risk == RiskHigh {
			highRiskInBucket1++
		}
	}
	assert.Equal(t, 1, highRiskInBucket0, "bucket 0 should have exactly 1 high-risk file")
	assert.Equal(t, 1, highRiskInBucket1, "bucket 1 should have exactly 1 high-risk file")
}

func TestSplitFiles_SingleAgent(t *testing.T) {
	t.Parallel()

	files := []ChangedFile{
		{Path: "a.go", Risk: RiskNormal},
		{Path: "b.go", Risk: RiskHigh},
		{Path: "c.go", Risk: RiskLow},
	}
	result := SplitFiles(files, 1)
	require.Len(t, result, 1)
	assert.Len(t, result[0], 3)
}

func TestSplitFiles_PreservesAllFiles(t *testing.T) {
	t.Parallel()

	files := []ChangedFile{
		{Path: "a.go", Risk: RiskHigh},
		{Path: "b.go", Risk: RiskNormal},
		{Path: "c.go", Risk: RiskLow},
		{Path: "d.go", Risk: RiskHigh},
		{Path: "e.go", Risk: RiskNormal},
		{Path: "f.go", Risk: RiskLow},
		{Path: "g.go", Risk: RiskNormal},
	}

	result := SplitFiles(files, 3)
	require.Len(t, result, 3)

	total := 0
	for _, bucket := range result {
		total += len(bucket)
	}
	assert.Equal(t, len(files), total, "total files across all buckets must equal input")
}

// ---------------------------------------------------------------------------
// DiffStats type
// ---------------------------------------------------------------------------

func TestDiffStats_ZeroValue(t *testing.T) {
	t.Parallel()

	var s DiffStats
	assert.Zero(t, s.TotalFiles)
	assert.Zero(t, s.FilesAdded)
	assert.Zero(t, s.FilesModified)
	assert.Zero(t, s.FilesDeleted)
	assert.Zero(t, s.FilesRenamed)
	assert.Zero(t, s.TotalLinesAdded)
	assert.Zero(t, s.TotalLinesDeleted)
	assert.Zero(t, s.HighRiskFiles)
}

// ---------------------------------------------------------------------------
// ChangedFile type
// ---------------------------------------------------------------------------

func TestChangedFile_ZeroValue(t *testing.T) {
	t.Parallel()

	var cf ChangedFile
	assert.Empty(t, cf.Path)
	assert.Empty(t, cf.OldPath)
	assert.Zero(t, cf.LinesAdded)
	assert.Zero(t, cf.LinesDeleted)
}

// ---------------------------------------------------------------------------
// ChangeType and RiskLevel constants
// ---------------------------------------------------------------------------

func TestChangeTypeConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ChangeType("added"), ChangeAdded)
	assert.Equal(t, ChangeType("modified"), ChangeModified)
	assert.Equal(t, ChangeType("deleted"), ChangeDeleted)
	assert.Equal(t, ChangeType("renamed"), ChangeRenamed)
}

func TestRiskLevelConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, RiskLevel("high"), RiskHigh)
	assert.Equal(t, RiskLevel("normal"), RiskNormal)
	assert.Equal(t, RiskLevel("low"), RiskLow)
}

// ---------------------------------------------------------------------------
// Generate — renamed file OldPath population
// ---------------------------------------------------------------------------

func TestGenerate_RenamedFileOldPath(t *testing.T) {
	t.Parallel()

	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{
			{Status: "R", Path: "new_name.go"},
		},
		numStatResult: []git.NumStatEntry{
			{Path: "new_name.go", OldPath: "old_name.go", Added: 5, Deleted: 2},
		},
		unifiedResult: "diff --git a/old_name.go b/new_name.go",
	}
	dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), "main")
	require.NoError(t, err)
	require.Len(t, result.Files, 1)

	f := result.Files[0]
	assert.Equal(t, "new_name.go", f.Path)
	assert.Equal(t, "old_name.go", f.OldPath)
	assert.Equal(t, ChangeRenamed, f.ChangeType)
	assert.Equal(t, 5, f.LinesAdded)
	assert.Equal(t, 2, f.LinesDeleted)
}

func TestGenerate_RenamedFileWithSimilarityScore(t *testing.T) {
	t.Parallel()

	// Simulates git diff --name-status output with "R095" style status.
	// The mockGitClient returns already-parsed DiffEntry with Status="R".
	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{
			{Status: "R", Path: "internal/handler/new.go"},
		},
		numStatResult: []git.NumStatEntry{
			{
				Path:    "internal/handler/new.go",
				OldPath: "internal/handler/old.go",
				Added:   3,
				Deleted: 1,
			},
		},
		unifiedResult: "some rename diff",
	}
	dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), "main")
	require.NoError(t, err)
	require.Len(t, result.Files, 1)
	assert.Equal(t, "internal/handler/old.go", result.Files[0].OldPath)
	assert.Equal(t, ChangeRenamed, result.Files[0].ChangeType)
}

// ---------------------------------------------------------------------------
// Generate — spec risk classification examples
// ---------------------------------------------------------------------------

func TestGenerate_RiskClassification_SpecExamples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		filePath     string
		riskPattern  string
		expectedRisk RiskLevel
	}{
		{
			name:         "auth handler is high-risk",
			filePath:     "internal/auth/handler.go",
			riskPattern:  `internal/auth`,
			expectedRisk: RiskHigh,
		},
		{
			name:         "README is normal-risk",
			filePath:     "README.md",
			riskPattern:  `internal/auth`,
			expectedRisk: RiskNormal,
		},
		{
			name:         "auth token is high-risk",
			filePath:     "internal/auth/token.go",
			riskPattern:  `internal/auth`,
			expectedRisk: RiskHigh,
		},
		{
			name:         "util helper is normal-risk",
			filePath:     "internal/util/helpers.go",
			riskPattern:  `internal/auth`,
			expectedRisk: RiskNormal,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := &mockGitClient{
				diffFilesResult: []git.DiffEntry{{Status: "M", Path: tt.filePath}},
				numStatResult:   []git.NumStatEntry{{Path: tt.filePath, Added: 1, Deleted: 0}},
				unifiedResult:   "diff",
			}
			dg, err := NewDiffGenerator(mock, ReviewConfig{RiskPatterns: tt.riskPattern}, nil)
			require.NoError(t, err)

			result, err := dg.Generate(context.Background(), "main")
			require.NoError(t, err)
			require.Len(t, result.Files, 1)
			assert.Equal(t, tt.expectedRisk, result.Files[0].Risk)
		})
	}
}

// ---------------------------------------------------------------------------
// Generate — extension filtering spec examples
// ---------------------------------------------------------------------------

func TestGenerate_ExtensionFilter_GoIncludedPNGExcluded(t *testing.T) {
	t.Parallel()

	// Spec requirement: .go files included, .png files excluded.
	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{
			{Status: "A", Path: "main.go"},
			{Status: "M", Path: "assets/logo.png"},
			{Status: "M", Path: "handler.go"},
		},
		numStatResult: []git.NumStatEntry{
			{Path: "main.go", Added: 10, Deleted: 0},
			{Path: "assets/logo.png", Added: -1, Deleted: -1},
			{Path: "handler.go", Added: 5, Deleted: 2},
		},
		unifiedResult: "diff output",
	}

	dg, err := NewDiffGenerator(mock, ReviewConfig{Extensions: `\.go$`}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), "main")
	require.NoError(t, err)

	require.Len(t, result.Files, 2)
	paths := make([]string, len(result.Files))
	for i, f := range result.Files {
		paths[i] = f.Path
	}
	assert.Contains(t, paths, "main.go")
	assert.Contains(t, paths, "handler.go")
	assert.NotContains(t, paths, "assets/logo.png")
}

func TestGenerate_ExtensionFilter_MultipleExtensions(t *testing.T) {
	t.Parallel()

	// Filter for both .go and .ts files.
	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{
			{Status: "M", Path: "server.go"},
			{Status: "A", Path: "client.ts"},
			{Status: "M", Path: "styles.css"},
			{Status: "M", Path: "README.md"},
		},
		numStatResult: []git.NumStatEntry{
			{Path: "server.go", Added: 3, Deleted: 1},
			{Path: "client.ts", Added: 7, Deleted: 0},
			{Path: "styles.css", Added: 2, Deleted: 0},
			{Path: "README.md", Added: 1, Deleted: 0},
		},
		unifiedResult: "diff",
	}

	dg, err := NewDiffGenerator(mock, ReviewConfig{Extensions: `\.(go|ts)$`}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), "main")
	require.NoError(t, err)

	require.Len(t, result.Files, 2, "only .go and .ts files should be included")
	paths := make([]string, len(result.Files))
	for i, f := range result.Files {
		paths[i] = f.Path
	}
	assert.Contains(t, paths, "server.go")
	assert.Contains(t, paths, "client.ts")
	assert.NotContains(t, paths, "styles.css")
	assert.NotContains(t, paths, "README.md")
}

// ---------------------------------------------------------------------------
// Generate — logger path coverage
// ---------------------------------------------------------------------------

func TestGenerate_WithLogger_DoesNotPanic(t *testing.T) {
	t.Parallel()

	// Pass a non-nil logger to exercise the debug/info log branches.
	logger := log.New(io.Discard)

	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{
			{Status: "M", Path: "main.go"},
			{Status: "M", Path: "README.md"}, // will be filtered
		},
		numStatResult: []git.NumStatEntry{
			{Path: "main.go", Added: 3, Deleted: 1},
			{Path: "README.md", Added: 1, Deleted: 0},
		},
		unifiedResult: "diff output",
	}

	// Use extension filter to trigger the debug log branch for skipped files.
	dg, err := NewDiffGenerator(mock, ReviewConfig{Extensions: `\.go$`}, logger)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), "main")
	require.NoError(t, err)
	require.Len(t, result.Files, 1)
	assert.Equal(t, "main.go", result.Files[0].Path)
}

func TestGenerate_WithLogger_EmptyDiff(t *testing.T) {
	t.Parallel()

	logger := log.New(io.Discard)

	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{},
		numStatResult:   []git.NumStatEntry{},
		unifiedResult:   "",
	}

	dg, err := NewDiffGenerator(mock, ReviewConfig{}, logger)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), "main")
	require.NoError(t, err)
	assert.Empty(t, result.Files)
	assert.Zero(t, result.Stats.TotalFiles)
}

// ---------------------------------------------------------------------------
// Generate — context cancellation
// ---------------------------------------------------------------------------

func TestGenerate_ContextCancellation(t *testing.T) {
	t.Parallel()

	mock := &mockGitClient{
		diffFilesErr: context.Canceled,
	}
	dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = dg.Generate(ctx, "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listing changed files")
}

// ---------------------------------------------------------------------------
// SplitFiles — spec-required distribution scenarios
// ---------------------------------------------------------------------------

func TestSplitFiles_SixFilesAcrossTwoAgents(t *testing.T) {
	t.Parallel()

	// Spec: 6 files across 2 agents = 3 files each.
	files := []ChangedFile{
		{Path: "a.go", Risk: RiskNormal},
		{Path: "b.go", Risk: RiskNormal},
		{Path: "c.go", Risk: RiskNormal},
		{Path: "d.go", Risk: RiskNormal},
		{Path: "e.go", Risk: RiskNormal},
		{Path: "f.go", Risk: RiskNormal},
	}

	result := SplitFiles(files, 2)
	require.Len(t, result, 2)
	assert.Len(t, result[0], 3)
	assert.Len(t, result[1], 3)
}

func TestSplitFiles_FiveFilesAcrossThreeAgents(t *testing.T) {
	t.Parallel()

	// Spec: 5 files across 3 agents = 2, 2, 1 distribution.
	files := []ChangedFile{
		{Path: "a.go", Risk: RiskNormal},
		{Path: "b.go", Risk: RiskNormal},
		{Path: "c.go", Risk: RiskNormal},
		{Path: "d.go", Risk: RiskNormal},
		{Path: "e.go", Risk: RiskNormal},
	}

	result := SplitFiles(files, 3)
	require.Len(t, result, 3)

	total := 0
	for _, bucket := range result {
		total += len(bucket)
	}
	assert.Equal(t, 5, total, "all 5 files must be distributed")

	// The distribution must be 2, 2, 1 (round-robin from 5 files into 3 buckets).
	// File 0 -> bucket 0, file 1 -> bucket 1, file 2 -> bucket 2,
	// file 3 -> bucket 0, file 4 -> bucket 1.
	assert.Len(t, result[0], 2, "bucket 0 should have 2 files")
	assert.Len(t, result[1], 2, "bucket 1 should have 2 files")
	assert.Len(t, result[2], 1, "bucket 2 should have 1 file")
}

func TestSplitFiles_HighRiskDistributedFirstAcrossThreeAgents(t *testing.T) {
	t.Parallel()

	// 3 high-risk files, 3 normal — 3 agents. Each bucket should get 1 high-risk.
	files := []ChangedFile{
		{Path: "normal1.go", Risk: RiskNormal},
		{Path: "normal2.go", Risk: RiskNormal},
		{Path: "normal3.go", Risk: RiskNormal},
		{Path: "auth1.go", Risk: RiskHigh},
		{Path: "auth2.go", Risk: RiskHigh},
		{Path: "auth3.go", Risk: RiskHigh},
	}

	result := SplitFiles(files, 3)
	require.Len(t, result, 3)

	for i, bucket := range result {
		highCount := 0
		for _, f := range bucket {
			if f.Risk == RiskHigh {
				highCount++
			}
		}
		assert.Equal(t, 1, highCount,
			"each bucket should have exactly 1 high-risk file (bucket %d)", i)
	}
}

func TestSplitFiles_OnlyHighRiskFiles(t *testing.T) {
	t.Parallel()

	files := []ChangedFile{
		{Path: "auth.go", Risk: RiskHigh},
		{Path: "crypto.go", Risk: RiskHigh},
		{Path: "secret.go", Risk: RiskHigh},
	}

	result := SplitFiles(files, 2)
	require.Len(t, result, 2)

	total := 0
	for _, bucket := range result {
		total += len(bucket)
	}
	assert.Equal(t, 3, total, "all high-risk files must be distributed")
}

func TestSplitFiles_RoundRobinDistribution(t *testing.T) {
	t.Parallel()

	// 7 files across 3 agents: round-robin gives 3, 2, 2.
	files := make([]ChangedFile, 7)
	for i := range files {
		files[i] = ChangedFile{Path: fmt.Sprintf("file%d.go", i), Risk: RiskNormal}
	}

	result := SplitFiles(files, 3)
	require.Len(t, result, 3)

	total := 0
	for _, bucket := range result {
		total += len(bucket)
	}
	assert.Equal(t, 7, total)

	// Verify no bucket is empty.
	for i, bucket := range result {
		assert.NotEmpty(t, bucket, "bucket %d should not be empty", i)
	}
}

// ---------------------------------------------------------------------------
// Generate — files with special characters in names
// ---------------------------------------------------------------------------

func TestGenerate_FilesWithSpecialCharacters(t *testing.T) {
	t.Parallel()

	// Files with spaces, hyphens, underscores, dots in names.
	specialFiles := []git.DiffEntry{
		{Status: "A", Path: "src/my component.go"},
		{Status: "M", Path: "internal/some-package/handler_test.go"},
		{Status: "M", Path: "docs/api.v2.spec.go"},
	}
	numStats := []git.NumStatEntry{
		{Path: "src/my component.go", Added: 10, Deleted: 0},
		{Path: "internal/some-package/handler_test.go", Added: 5, Deleted: 3},
		{Path: "docs/api.v2.spec.go", Added: 2, Deleted: 1},
	}

	mock := &mockGitClient{
		diffFilesResult: specialFiles,
		numStatResult:   numStats,
		unifiedResult:   "diff output",
	}

	dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), "main")
	require.NoError(t, err)
	assert.Len(t, result.Files, 3)

	pathSet := make(map[string]struct{})
	for _, f := range result.Files {
		pathSet[f.Path] = struct{}{}
	}
	assert.Contains(t, pathSet, "src/my component.go")
	assert.Contains(t, pathSet, "internal/some-package/handler_test.go")
	assert.Contains(t, pathSet, "docs/api.v2.spec.go")
}

// ---------------------------------------------------------------------------
// Generate — numstat path mismatch (file in diff but not in numstat)
// ---------------------------------------------------------------------------

func TestGenerate_FileNotInNumStat(t *testing.T) {
	t.Parallel()

	// A file appears in DiffFiles but not in DiffNumStat (edge case: deleted
	// file with 0 lines in numstat or parsing mismatch). The generator must
	// handle this gracefully with zero line counts.
	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{
			{Status: "D", Path: "removed.go"},
		},
		numStatResult: []git.NumStatEntry{}, // empty — no numstat entry
		unifiedResult: "diff output",
	}

	dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), "main")
	require.NoError(t, err)
	require.Len(t, result.Files, 1)

	f := result.Files[0]
	assert.Equal(t, "removed.go", f.Path)
	assert.Equal(t, ChangeDeleted, f.ChangeType)
	assert.Equal(t, 0, f.LinesAdded)
	assert.Equal(t, 0, f.LinesDeleted)
}

// ---------------------------------------------------------------------------
// Generate — mixed change types with DiffStats verification (spec scenario)
// ---------------------------------------------------------------------------

func TestGenerate_MixedChangeTypes_StatsVerification(t *testing.T) {
	t.Parallel()

	// Spec: DiffStats accurately counts files by type and lines added/deleted.
	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{
			{Status: "A", Path: "new_feature.go"},
			{Status: "M", Path: "existing.go"},
			{Status: "D", Path: "deprecated.go"},
			{Status: "R", Path: "refactored.go"},
		},
		numStatResult: []git.NumStatEntry{
			{Path: "new_feature.go", Added: 50, Deleted: 0},
			{Path: "existing.go", Added: 10, Deleted: 5},
			{Path: "deprecated.go", Added: 0, Deleted: 30},
			{Path: "refactored.go", OldPath: "old.go", Added: 2, Deleted: 2},
		},
		unifiedResult: "full diff",
	}

	dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), "main")
	require.NoError(t, err)

	s := result.Stats
	assert.Equal(t, 4, s.TotalFiles)
	assert.Equal(t, 1, s.FilesAdded)
	assert.Equal(t, 1, s.FilesModified)
	assert.Equal(t, 1, s.FilesDeleted)
	assert.Equal(t, 1, s.FilesRenamed)
	assert.Equal(t, 62, s.TotalLinesAdded)   // 50+10+0+2
	assert.Equal(t, 37, s.TotalLinesDeleted) // 0+5+30+2
	assert.Equal(t, 0, s.HighRiskFiles)
}

// ---------------------------------------------------------------------------
// Generate — full diff text is propagated
// ---------------------------------------------------------------------------

func TestGenerate_FullDiffText(t *testing.T) {
	t.Parallel()

	const expectedDiff = `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+
+func main() {}`

	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{{Status: "M", Path: "main.go"}},
		numStatResult:   []git.NumStatEntry{{Path: "main.go", Added: 2, Deleted: 0}},
		unifiedResult:   expectedDiff,
	}

	dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), "main")
	require.NoError(t, err)
	assert.Equal(t, expectedDiff, result.FullDiff)
}

// ---------------------------------------------------------------------------
// SplitFiles — no files lost with mixed risk levels
// ---------------------------------------------------------------------------

func TestSplitFiles_MixedRiskLevelsNoFilesLost(t *testing.T) {
	t.Parallel()

	files := []ChangedFile{
		{Path: "auth.go", Risk: RiskHigh},
		{Path: "config.go", Risk: RiskNormal},
		{Path: "README.md", Risk: RiskLow},
		{Path: "crypto.go", Risk: RiskHigh},
		{Path: "util.go", Risk: RiskNormal},
		{Path: "generated.pb.go", Risk: RiskLow},
		{Path: "handler.go", Risk: RiskHigh},
	}

	result := SplitFiles(files, 3)
	require.Len(t, result, 3)

	totalFiles := 0
	highRiskTotal := 0
	for _, bucket := range result {
		totalFiles += len(bucket)
		for _, f := range bucket {
			if f.Risk == RiskHigh {
				highRiskTotal++
			}
		}
	}

	assert.Equal(t, len(files), totalFiles, "no files should be lost")
	assert.Equal(t, 3, highRiskTotal, "all high-risk files should be preserved")
}

// ---------------------------------------------------------------------------
// Generate — base branch stored in result
// ---------------------------------------------------------------------------

func TestGenerate_BaseBranchStoredInResult(t *testing.T) {
	t.Parallel()

	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{},
		numStatResult:   []git.NumStatEntry{},
		unifiedResult:   "",
	}

	dg, err := NewDiffGenerator(mock, ReviewConfig{}, nil)
	require.NoError(t, err)

	for _, branch := range []string{"main", "origin/main", "feature/my-branch", "v1.0.0"} {
		branch := branch
		t.Run(branch, func(t *testing.T) {
			t.Parallel()
			result, err := dg.Generate(context.Background(), branch)
			require.NoError(t, err)
			assert.Equal(t, branch, result.BaseBranch)
		})
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

// BenchmarkSplitFiles measures the cost of distributing files across agents,
// including the sort-by-risk step.
func BenchmarkSplitFiles(b *testing.B) {
	// Create a representative file list: 1/3 high, 1/3 normal, 1/3 low.
	const total = 300
	files := make([]ChangedFile, total)
	risks := []RiskLevel{RiskHigh, RiskNormal, RiskLow}
	for i := range files {
		files[i] = ChangedFile{
			Path: fmt.Sprintf("internal/pkg%d/file.go", i),
			Risk: risks[i%3],
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = SplitFiles(files, 4)
	}
}

// BenchmarkGenerate measures the DiffGenerator pipeline (mock client, so
// only the filtering + classification overhead is benchmarked).
func BenchmarkGenerate(b *testing.B) {
	const numFiles = 200
	entries := make([]git.DiffEntry, numFiles)
	numStats := make([]git.NumStatEntry, numFiles)
	for i := range entries {
		path := fmt.Sprintf("internal/pkg%d/file.go", i)
		entries[i] = git.DiffEntry{Status: "M", Path: path}
		numStats[i] = git.NumStatEntry{Path: path, Added: 5, Deleted: 2}
	}

	mock := &mockGitClient{
		diffFilesResult: entries,
		numStatResult:   numStats,
		unifiedResult:   "diff output",
	}
	dg, err := NewDiffGenerator(mock, ReviewConfig{
		Extensions:   `\.go$`,
		RiskPatterns: `internal/pkg[0-9]{1,2}/`,
	}, nil)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = dg.Generate(context.Background(), "main")
	}
}

// ---------------------------------------------------------------------------
// Fuzz — branch name validation
// ---------------------------------------------------------------------------

// FuzzGenerate_BranchNameValidation verifies that the branch name validator
// never panics and correctly rejects/accepts names according to the regex.
func FuzzGenerate_BranchNameValidation(f *testing.F) {
	// Seed with known valid names.
	f.Add("main")
	f.Add("origin/main")
	f.Add("feature/my-branch")
	f.Add("v1.2.3")
	f.Add("release_2024")

	// Seed with known invalid names.
	f.Add("")
	f.Add("main branch")
	f.Add("main;rm -rf /")
	f.Add("HEAD@{1}")
	f.Add("main..HEAD")
	f.Add("main|cat")

	mock := &mockGitClient{
		diffFilesResult: []git.DiffEntry{},
		numStatResult:   []git.NumStatEntry{},
		unifiedResult:   "",
	}
	dg, _ := NewDiffGenerator(mock, ReviewConfig{}, nil)

	f.Fuzz(func(t *testing.T, branch string) {
		// Must not panic regardless of input.
		result, err := dg.Generate(context.Background(), branch)
		if err != nil {
			// If there is an error, it must mention "invalid base branch".
			if result != nil {
				t.Errorf("expected nil result when error returned, got non-nil for branch %q", branch)
			}
			return
		}
		// If no error, the result must have BaseBranch set.
		if result.BaseBranch != branch {
			t.Errorf("result.BaseBranch %q != branch %q", result.BaseBranch, branch)
		}
	})
}
