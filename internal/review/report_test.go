package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// makeConsolidatedReview builds a ConsolidatedReview for use in report tests.
func makeConsolidatedReview(verdict Verdict, findings []*Finding, agentResults []AgentReviewResult) *ConsolidatedReview {
	return &ConsolidatedReview{
		Findings:     findings,
		Verdict:      verdict,
		AgentResults: agentResults,
		TotalAgents:  len(agentResults),
		Duration:     2 * time.Second,
	}
}

// makeStats builds a ConsolidationStats for use in report tests.
func makeStats(total, unique, dupes, escalations int, overlapRate float64) *ConsolidationStats {
	return &ConsolidationStats{
		TotalInputFindings:  total,
		UniqueFindings:      unique,
		DuplicatesRemoved:   dupes,
		SeverityEscalations: escalations,
		OverlapRate:         overlapRate,
		FindingsPerAgent:    make(map[string]int),
		FindingsPerSeverity: make(map[Severity]int),
	}
}

// makeDiffResult builds a minimal DiffResult for use in report tests.
func makeDiffResult(filesChanged, linesAdded, linesDeleted int) *DiffResult {
	return &DiffResult{
		Files:      []ChangedFile{},
		FullDiff:   "",
		BaseBranch: "main",
		Stats: DiffStats{
			TotalFiles:        filesChanged,
			TotalLinesAdded:   linesAdded,
			TotalLinesDeleted: linesDeleted,
		},
	}
}

// ---------------------------------------------------------------------------
// NewReportGenerator
// ---------------------------------------------------------------------------

func TestNewReportGenerator_NilLogger(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	require.NotNil(t, rg)
	assert.Nil(t, rg.logger)
	assert.NotNil(t, rg.tmpl)
}

func TestNewReportGenerator_WithLogger(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	rg := NewReportGenerator(logger)
	require.NotNil(t, rg)
	assert.NotNil(t, rg.logger)
}

// ---------------------------------------------------------------------------
// Generate — input validation
// ---------------------------------------------------------------------------

func TestGenerate_NilConsolidated_ReturnsError(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	_, err := rg.Generate(nil, makeStats(0, 0, 0, 0, 0), makeDiffResult(0, 0, 0))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "consolidated review is required")
}

func TestGenerate_NilStats_ReturnsError(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictApproved, nil, nil)
	_, err := rg.Generate(cr, nil, makeDiffResult(0, 0, 0))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "consolidation stats are required")
}

func TestGenerate_NilDiffResult_ReturnsError(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictApproved, nil, nil)
	stats := makeStats(0, 0, 0, 0, 0)
	_, err := rg.Generate(cr, stats, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "diff result is required")
}

// ---------------------------------------------------------------------------
// Generate — approved with zero findings
// ---------------------------------------------------------------------------

func TestGenerate_ApprovedZeroFindings_ContainsNoIssuesText(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictApproved, nil, []AgentReviewResult{
		makeAgentResult("claude", VerdictApproved, nil, nil),
	})
	stats := makeStats(0, 0, 0, 0, 0)
	diff := makeDiffResult(3, 150, 20)

	report, err := rg.Generate(cr, stats, diff)
	require.NoError(t, err)
	assert.NotEmpty(t, report)
	assert.Contains(t, report, "No issues found")
	assert.Contains(t, report, "[PASS]")
	assert.Contains(t, report, "APPROVED")
}

// ---------------------------------------------------------------------------
// Generate — verdict indicators
// ---------------------------------------------------------------------------

func TestGenerate_VerdictIndicators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		verdict      Verdict
		wantIndicator string
	}{
		{"approved", VerdictApproved, "[PASS]"},
		{"changes_needed", VerdictChangesNeeded, "[FAIL]"},
		{"blocking", VerdictBlocking, "[BLOCK]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rg := NewReportGenerator(nil)
			cr := makeConsolidatedReview(tt.verdict, nil, nil)
			stats := makeStats(0, 0, 0, 0, 0)

			report, err := rg.Generate(cr, stats, makeDiffResult(0, 0, 0))
			require.NoError(t, err)
			assert.Contains(t, report, tt.wantIndicator)
			assert.Contains(t, report, string(tt.verdict))
		})
	}
}

// ---------------------------------------------------------------------------
// Generate — findings present
// ---------------------------------------------------------------------------

func TestGenerate_WithFindings_ContainsTable(t *testing.T) {
	t.Parallel()

	findings := []*Finding{
		{
			Severity:    SeverityCritical,
			Category:    "security",
			File:        "internal/auth/token.go",
			Line:        42,
			Description: "token stored in plain text",
			Suggestion:  "use encrypted storage",
			Agent:       "claude",
		},
		{
			Severity:    SeverityHigh,
			Category:    "logic",
			File:        "workflow.go",
			Line:        10,
			Description: "nil dereference possible",
			Agent:       "codex",
		},
		{
			Severity:    SeverityLow,
			Category:    "style",
			File:        "main.go",
			Line:        1,
			Description: "missing doc comment",
			Agent:       "claude",
		},
	}

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictBlocking, findings, []AgentReviewResult{
		makeAgentResult("claude", VerdictBlocking, nil, nil),
		makeAgentResult("codex", VerdictChangesNeeded, nil, nil),
	})
	stats := makeStats(4, 3, 1, 1, 50.0)
	stats.FindingsPerAgent["claude"] = 3
	stats.FindingsPerAgent["codex"] = 1
	stats.FindingsPerSeverity[SeverityCritical] = 1
	stats.FindingsPerSeverity[SeverityHigh] = 1
	stats.FindingsPerSeverity[SeverityLow] = 1

	report, err := rg.Generate(cr, stats, makeDiffResult(5, 200, 30))
	require.NoError(t, err)

	// Verdict
	assert.Contains(t, report, "[BLOCK]")
	assert.Contains(t, report, "BLOCKING")

	// Summary counts
	assert.Contains(t, report, "3") // total findings
	assert.Contains(t, report, "1") // critical count

	// Findings table headers
	assert.Contains(t, report, "| Severity |")
	assert.Contains(t, report, "| Category |")

	// Finding content
	assert.Contains(t, report, "security")
	assert.Contains(t, report, "internal/auth/token.go")
	assert.Contains(t, report, "token stored in plain text")

	// Agent breakdown
	assert.Contains(t, report, "Agent Breakdown")
	assert.Contains(t, report, "claude")
	assert.Contains(t, report, "codex")

	// Consolidation stats
	assert.Contains(t, report, "Consolidation Statistics")
	assert.Contains(t, report, "50.0%")

	// Diff stats
	assert.Contains(t, report, "Diff Statistics")
	assert.Contains(t, report, "200") // lines added
}

func TestGenerate_FindingsByFile_Section(t *testing.T) {
	t.Parallel()

	findings := []*Finding{
		{
			Severity:    SeverityHigh,
			Category:    "security",
			File:        "auth.go",
			Line:        10,
			Description: "hardcoded secret",
			Agent:       "claude",
		},
		{
			Severity:    SeverityMedium,
			Category:    "perf",
			File:        "auth.go",
			Line:        25,
			Description: "inefficient query",
			Agent:       "codex",
		},
		{
			Severity:    SeverityLow,
			Category:    "style",
			File:        "main.go",
			Line:        1,
			Description: "missing doc",
			Agent:       "claude",
		},
	}

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictChangesNeeded, findings, nil)
	stats := makeStats(3, 3, 0, 0, 0)

	report, err := rg.Generate(cr, stats, makeDiffResult(2, 100, 10))
	require.NoError(t, err)

	assert.Contains(t, report, "Findings by File")
	assert.Contains(t, report, "auth.go")
	assert.Contains(t, report, "main.go")
	assert.Contains(t, report, "hardcoded secret")
	assert.Contains(t, report, "inefficient query")
	assert.Contains(t, report, "missing doc")
}

func TestGenerate_FindingsBySeverity_Section(t *testing.T) {
	t.Parallel()

	findings := []*Finding{
		{
			Severity:    SeverityCritical,
			Category:    "security",
			File:        "auth.go",
			Line:        10,
			Description: "critical issue",
			Agent:       "claude",
		},
		{
			Severity:    SeverityInfo,
			Category:    "docs",
			File:        "README.md",
			Line:        0,
			Description: "missing section",
			Agent:       "codex",
		},
	}

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictBlocking, findings, nil)
	stats := makeStats(2, 2, 0, 0, 0)
	stats.FindingsPerSeverity[SeverityCritical] = 1
	stats.FindingsPerSeverity[SeverityInfo] = 1

	report, err := rg.Generate(cr, stats, makeDiffResult(1, 50, 5))
	require.NoError(t, err)

	assert.Contains(t, report, "Findings by Severity")
	assert.Contains(t, report, "CRITICAL")
	assert.Contains(t, report, "INFO")
}

// ---------------------------------------------------------------------------
// Generate — pipe character escaping
// ---------------------------------------------------------------------------

func TestGenerate_EscapesCellPipes(t *testing.T) {
	t.Parallel()

	findings := []*Finding{
		{
			Severity:    SeverityHigh,
			Category:    "logic",
			File:        "main.go",
			Line:        1,
			Description: "use A | B not C",
			Agent:       "claude",
		},
	}

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictChangesNeeded, findings, nil)
	stats := makeStats(1, 1, 0, 0, 0)

	report, err := rg.Generate(cr, stats, makeDiffResult(1, 10, 0))
	require.NoError(t, err)

	// The pipe in the description should be escaped
	assert.Contains(t, report, `use A \| B not C`)
}

// ---------------------------------------------------------------------------
// Generate — timestamp present
// ---------------------------------------------------------------------------

func TestGenerate_ContainsTimestamp(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictApproved, nil, nil)
	stats := makeStats(0, 0, 0, 0, 0)

	report, err := rg.Generate(cr, stats, makeDiffResult(0, 0, 0))
	require.NoError(t, err)

	// Should contain a date in YYYY-MM-DD format
	currentYear := time.Now().UTC().Format("2006")
	assert.Contains(t, report, currentYear)
}

// ---------------------------------------------------------------------------
// Generate — consolidation and diff statistics
// ---------------------------------------------------------------------------

func TestGenerate_ConsolidationStats_AllFields(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictChangesNeeded, nil, nil)
	stats := &ConsolidationStats{
		TotalInputFindings:  10,
		UniqueFindings:      7,
		DuplicatesRemoved:   3,
		SeverityEscalations: 2,
		OverlapRate:         42.86,
		FindingsPerAgent:    map[string]int{"claude": 6, "codex": 4},
		FindingsPerSeverity: map[Severity]int{SeverityHigh: 4, SeverityLow: 3},
	}

	report, err := rg.Generate(cr, stats, makeDiffResult(0, 0, 0))
	require.NoError(t, err)

	assert.Contains(t, report, "10") // TotalInputFindings
	assert.Contains(t, report, "7")  // UniqueFindings
	assert.Contains(t, report, "3")  // DuplicatesRemoved
	assert.Contains(t, report, "2")  // SeverityEscalations
	assert.Contains(t, report, "42.9") // OverlapRate formatted to 1 decimal
	assert.Contains(t, report, "claude")
	assert.Contains(t, report, "codex")
}

func TestGenerate_DiffStats_AllFields(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictApproved, nil, nil)
	stats := makeStats(0, 0, 0, 0, 0)
	diff := &DiffResult{
		Files:      []ChangedFile{},
		FullDiff:   "",
		BaseBranch: "main",
		Stats: DiffStats{
			TotalFiles:        8,
			FilesAdded:        3,
			FilesModified:     4,
			FilesDeleted:      1,
			FilesRenamed:      0,
			TotalLinesAdded:   350,
			TotalLinesDeleted: 75,
			HighRiskFiles:     2,
		},
	}

	report, err := rg.Generate(cr, stats, diff)
	require.NoError(t, err)

	assert.Contains(t, report, "8")   // TotalFiles
	assert.Contains(t, report, "350") // TotalLinesAdded
	assert.Contains(t, report, "75")  // TotalLinesDeleted
	assert.Contains(t, report, "2")   // HighRiskFiles
}

// ---------------------------------------------------------------------------
// Generate — agent breakdown
// ---------------------------------------------------------------------------

func TestGenerate_AgentBreakdown_ErroredAgent(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictChangesNeeded, nil, []AgentReviewResult{
		makeAgentResult("claude", VerdictApproved, nil, nil),
		makeAgentResult("codex", "", nil, assert.AnError),
	})
	stats := makeStats(0, 0, 0, 0, 0)

	report, err := rg.Generate(cr, stats, makeDiffResult(0, 0, 0))
	require.NoError(t, err)

	assert.Contains(t, report, "claude")
	assert.Contains(t, report, "codex")
	assert.Contains(t, report, "[FAIL]") // errored agent shows [FAIL]
	assert.Contains(t, report, "ERROR")  // errored agent shows ERROR verdict
}

func TestGenerate_AgentBreakdown_PassStatus(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictApproved, nil, []AgentReviewResult{
		makeAgentResult("claude", VerdictApproved, nil, nil),
	})
	stats := makeStats(0, 0, 0, 0, 0)

	report, err := rg.Generate(cr, stats, makeDiffResult(0, 0, 0))
	require.NoError(t, err)

	assert.Contains(t, report, "[PASS]")
}

// ---------------------------------------------------------------------------
// Generate — deterministic output
// ---------------------------------------------------------------------------

func TestGenerate_Deterministic_SameInputSameOutput(t *testing.T) {
	t.Parallel()

	// Findings across multiple files and severities to exercise map sorting.
	findings := []*Finding{
		{Severity: SeverityCritical, Category: "security", File: "z.go", Line: 1, Description: "critical z", Agent: "claude"},
		{Severity: SeverityHigh, Category: "logic", File: "a.go", Line: 5, Description: "high a", Agent: "codex"},
		{Severity: SeverityMedium, Category: "perf", File: "m.go", Line: 10, Description: "medium m", Agent: "claude"},
		{Severity: SeverityLow, Category: "style", File: "a.go", Line: 20, Description: "low a2", Agent: "codex"},
	}

	stats := &ConsolidationStats{
		TotalInputFindings:  6,
		UniqueFindings:      4,
		DuplicatesRemoved:   2,
		SeverityEscalations: 1,
		OverlapRate:         25.0,
		FindingsPerAgent:    map[string]int{"claude": 3, "codex": 3},
		FindingsPerSeverity: map[Severity]int{
			SeverityCritical: 1,
			SeverityHigh:     1,
			SeverityMedium:   1,
			SeverityLow:      1,
		},
	}

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictBlocking, findings, []AgentReviewResult{
		makeAgentResult("claude", VerdictBlocking, nil, nil),
		makeAgentResult("codex", VerdictChangesNeeded, nil, nil),
	})
	diff := makeDiffResult(4, 300, 50)

	// Generate the report twice; structure should be the same (timestamps will differ).
	report1, err1 := rg.Generate(cr, stats, diff)
	report2, err2 := rg.Generate(cr, stats, diff)

	require.NoError(t, err1)
	require.NoError(t, err2)

	// Strip timestamp lines for comparison.
	stripTimestamp := func(s string) string {
		lines := strings.Split(s, "\n")
		var filtered []string
		for _, l := range lines {
			if strings.Contains(l, "Generated:") || strings.Contains(l, "generated by Raven") {
				continue
			}
			filtered = append(filtered, l)
		}
		return strings.Join(filtered, "\n")
	}

	assert.Equal(t, stripTimestamp(report1), stripTimestamp(report2))
}

// ---------------------------------------------------------------------------
// WriteToFile
// ---------------------------------------------------------------------------

func TestWriteToFile_CreatesFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictApproved, nil, []AgentReviewResult{
		makeAgentResult("claude", VerdictApproved, nil, nil),
	})
	stats := makeStats(0, 0, 0, 0, 0)
	diff := makeDiffResult(2, 50, 10)

	err := rg.WriteToFile(path, cr, stats, diff)
	require.NoError(t, err)

	// File must exist and be non-empty.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
	assert.Contains(t, string(data), "Code Review Report")
}

func TestWriteToFile_CreatesParentDirectories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deeply", "report.md")

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictApproved, nil, nil)
	stats := makeStats(0, 0, 0, 0, 0)

	err := rg.WriteToFile(path, cr, stats, makeDiffResult(0, 0, 0))
	require.NoError(t, err)

	_, err = os.Stat(path)
	assert.NoError(t, err, "file should exist at nested path")
}

func TestWriteToFile_FilePermissions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictApproved, nil, nil)
	stats := makeStats(0, 0, 0, 0, 0)

	err := rg.WriteToFile(path, cr, stats, makeDiffResult(0, 0, 0))
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	// Mask to just permission bits and check for owner read+write.
	perm := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0644), perm)
}

func TestWriteToFile_NilConsolidated_ReturnsError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")

	rg := NewReportGenerator(nil)
	err := rg.WriteToFile(path, nil, makeStats(0, 0, 0, 0, 0), makeDiffResult(0, 0, 0))
	require.Error(t, err)

	// File must not have been created.
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

func TestWriteToFile_WithLogger(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")

	logger := newTestLogger()
	rg := NewReportGenerator(logger)
	cr := makeConsolidatedReview(VerdictApproved, nil, nil)
	stats := makeStats(0, 0, 0, 0, 0)

	err := rg.WriteToFile(path, cr, stats, makeDiffResult(0, 0, 0))
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// verdictIndicator
// ---------------------------------------------------------------------------

func TestVerdictIndicator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		verdict Verdict
		want    string
	}{
		{VerdictApproved, "[PASS]"},
		{VerdictChangesNeeded, "[FAIL]"},
		{VerdictBlocking, "[BLOCK]"},
		{Verdict("UNKNOWN"), "[UNKNOWN]"},
		{Verdict(""), "[UNKNOWN]"},
	}

	for _, tt := range tests {
		t.Run(string(tt.verdict), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, verdictIndicator(tt.verdict))
		})
	}
}

// ---------------------------------------------------------------------------
// escapeCellContent
// ---------------------------------------------------------------------------

func TestEscapeCellContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no special chars",
			input: "normal text",
			want:  "normal text",
		},
		{
			name:  "pipe escaped",
			input: "a | b",
			want:  `a \| b`,
		},
		{
			name:  "multiple pipes",
			input: "a | b | c",
			want:  `a \| b \| c`,
		},
		{
			name:  "newline replaced",
			input: "line one\nline two",
			want:  "line one line two",
		},
		{
			name:  "carriage return removed",
			input: "line one\r\nline two",
			want:  "line one line two",
		},
		{
			name:  "pipe and newline combined",
			input: "a | b\nc | d",
			want:  `a \| b c \| d`,
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, escapeCellContent(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// buildReportData
// ---------------------------------------------------------------------------

func TestBuildReportData_SeverityCounts(t *testing.T) {
	t.Parallel()

	findings := []*Finding{
		{Severity: SeverityCritical, Category: "sec", File: "a.go", Line: 1, Agent: "claude"},
		{Severity: SeverityCritical, Category: "sec", File: "b.go", Line: 2, Agent: "claude"},
		{Severity: SeverityHigh, Category: "logic", File: "c.go", Line: 3, Agent: "codex"},
		{Severity: SeverityMedium, Category: "perf", File: "d.go", Line: 4, Agent: "claude"},
		{Severity: SeverityLow, Category: "style", File: "e.go", Line: 5, Agent: "codex"},
		{Severity: SeverityInfo, Category: "docs", File: "f.go", Line: 6, Agent: "claude"},
	}

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictBlocking, findings, nil)
	stats := makeStats(6, 6, 0, 0, 0)
	diff := makeDiffResult(6, 100, 20)

	data := rg.buildReportData(cr, stats, diff)

	assert.Equal(t, 6, data.TotalFindings)
	assert.Equal(t, 2, data.CriticalCount)
	assert.Equal(t, 1, data.HighCount)
	assert.Equal(t, 1, data.MediumCount)
	assert.Equal(t, 1, data.LowCount)
	assert.Equal(t, 1, data.InfoCount)
}

func TestBuildReportData_FindingsByFileKeys_Sorted(t *testing.T) {
	t.Parallel()

	findings := []*Finding{
		{Severity: SeverityHigh, Category: "sec", File: "z.go", Line: 1, Agent: "claude"},
		{Severity: SeverityHigh, Category: "sec", File: "a.go", Line: 2, Agent: "claude"},
		{Severity: SeverityHigh, Category: "sec", File: "m.go", Line: 3, Agent: "codex"},
	}

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictChangesNeeded, findings, nil)
	data := rg.buildReportData(cr, makeStats(3, 3, 0, 0, 0), makeDiffResult(3, 100, 0))

	require.Len(t, data.FindingsByFileKeys, 3)
	assert.Equal(t, "a.go", data.FindingsByFileKeys[0])
	assert.Equal(t, "m.go", data.FindingsByFileKeys[1])
	assert.Equal(t, "z.go", data.FindingsByFileKeys[2])
}

func TestBuildReportData_FindingsBySeverityKeys_SortedByRankDesc(t *testing.T) {
	t.Parallel()

	findings := []*Finding{
		{Severity: SeverityInfo, Category: "docs", File: "a.go", Line: 1, Agent: "claude"},
		{Severity: SeverityCritical, Category: "sec", File: "b.go", Line: 2, Agent: "claude"},
		{Severity: SeverityMedium, Category: "perf", File: "c.go", Line: 3, Agent: "codex"},
	}

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictBlocking, findings, nil)
	data := rg.buildReportData(cr, makeStats(3, 3, 0, 0, 0), makeDiffResult(3, 50, 10))

	require.Len(t, data.FindingsBySeverityKeys, 3)
	// Critical (rank 5) should be first, Info (rank 1) should be last.
	assert.Equal(t, SeverityCritical, data.FindingsBySeverityKeys[0])
	assert.Equal(t, SeverityMedium, data.FindingsBySeverityKeys[1])
	assert.Equal(t, SeverityInfo, data.FindingsBySeverityKeys[2])
}

func TestBuildReportData_StatsAgentKeys_Sorted(t *testing.T) {
	t.Parallel()

	stats := &ConsolidationStats{
		FindingsPerAgent:    map[string]int{"zz": 5, "aa": 3, "mm": 2},
		FindingsPerSeverity: make(map[Severity]int),
	}

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictApproved, nil, nil)
	data := rg.buildReportData(cr, stats, makeDiffResult(0, 0, 0))

	require.Len(t, data.Stats.FindingsPerAgentKeys, 3)
	assert.Equal(t, "aa", data.Stats.FindingsPerAgentKeys[0])
	assert.Equal(t, "mm", data.Stats.FindingsPerAgentKeys[1])
	assert.Equal(t, "zz", data.Stats.FindingsPerAgentKeys[2])
}

func TestBuildReportData_StatsSeverityKeys_SortedByRankDesc(t *testing.T) {
	t.Parallel()

	stats := &ConsolidationStats{
		FindingsPerAgent: make(map[string]int),
		FindingsPerSeverity: map[Severity]int{
			SeverityLow:      2,
			SeverityCritical: 1,
			SeverityMedium:   3,
		},
	}

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictChangesNeeded, nil, nil)
	data := rg.buildReportData(cr, stats, makeDiffResult(0, 0, 0))

	require.Len(t, data.Stats.FindingsPerSeverityKeys, 3)
	assert.Equal(t, SeverityCritical, data.Stats.FindingsPerSeverityKeys[0])
	assert.Equal(t, SeverityMedium, data.Stats.FindingsPerSeverityKeys[1])
	assert.Equal(t, SeverityLow, data.Stats.FindingsPerSeverityKeys[2])
}

// ---------------------------------------------------------------------------
// Generate — GitHub markdown compatibility
// ---------------------------------------------------------------------------

func TestGenerate_ValidMarkdownStructure(t *testing.T) {
	t.Parallel()

	findings := []*Finding{
		{Severity: SeverityHigh, Category: "security", File: "main.go", Line: 42, Description: "issue", Agent: "claude"},
	}
	findingsVal := []Finding{*findings[0]}

	rg := NewReportGenerator(nil)
	cr := makeConsolidatedReview(VerdictChangesNeeded, findings, []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, findingsVal, nil),
	})
	stats := makeStats(1, 1, 0, 0, 0)
	stats.FindingsPerAgent["claude"] = 1
	stats.FindingsPerSeverity[SeverityHigh] = 1

	report, err := rg.Generate(cr, stats, makeDiffResult(1, 42, 0))
	require.NoError(t, err)

	// Should have top-level H1
	assert.True(t, strings.HasPrefix(report, "# Code Review Report"))

	// Should have H2 sections
	assert.Contains(t, report, "## Summary")
	assert.Contains(t, report, "## Findings")
	assert.Contains(t, report, "## Agent Breakdown")
	assert.Contains(t, report, "## Consolidation Statistics")
	assert.Contains(t, report, "## Diff Statistics")

	// Tables should have proper markdown pipe syntax
	assert.Contains(t, report, "| Metric | Value |")
}

// ---------------------------------------------------------------------------
// Integration test: full pipeline with real consolidation
// ---------------------------------------------------------------------------

func TestGenerate_Integration_FullPipeline(t *testing.T) {
	t.Parallel()

	// Build a realistic consolidated review via the consolidator.
	rawFindings := []Finding{
		{Severity: SeverityCritical, Category: "security", File: "auth.go", Line: 10, Description: "hardcoded API key"},
		{Severity: SeverityHigh, Category: "logic", File: "engine.go", Line: 55, Description: "nil dereference"},
		{Severity: SeverityMedium, Category: "perf", File: "loop.go", Line: 30, Description: "alloc in hot path"},
		{Severity: SeverityLow, Category: "style", File: "main.go", Line: 1, Description: "missing doc"},
	}

	consolidator := NewConsolidator(nil)
	agentResults := []AgentReviewResult{
		makeAgentResult("claude", VerdictBlocking, rawFindings[:2], nil),
		makeAgentResult("codex", VerdictChangesNeeded, rawFindings[1:], nil),
	}
	consolidated, stats := consolidator.Consolidate(agentResults)

	diff := &DiffResult{
		Files:      []ChangedFile{{Path: "auth.go", ChangeType: ChangeModified, Risk: RiskHigh, LinesAdded: 10, LinesDeleted: 2}},
		FullDiff:   "diff --git a/auth.go b/auth.go\n--- a/auth.go\n+++ b/auth.go",
		BaseBranch: "main",
		Stats: DiffStats{
			TotalFiles:        1,
			FilesModified:     1,
			TotalLinesAdded:   10,
			TotalLinesDeleted: 2,
			HighRiskFiles:     1,
		},
	}

	rg := NewReportGenerator(nil)
	report, err := rg.Generate(consolidated, stats, diff)
	require.NoError(t, err)
	require.NotEmpty(t, report)

	// Verify key sections are present.
	assert.Contains(t, report, "# Code Review Report")
	assert.Contains(t, report, "[BLOCK]")
	assert.Contains(t, report, "BLOCKING")
	assert.Contains(t, report, "auth.go")
	assert.Contains(t, report, "hardcoded API key")
	assert.Contains(t, report, "claude")
	assert.Contains(t, report, "codex")
	assert.Contains(t, report, "Consolidation Statistics")
	assert.Contains(t, report, "Diff Statistics")
}
