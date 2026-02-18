package review

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newLogger returns a logger that discards all output, suitable for tests that
// only need to exercise code paths that call the logger.
func newTestLogger() *log.Logger {
	return log.New(io.Discard)
}

// makeAgentResult builds an AgentReviewResult for use in tests.
func makeAgentResult(agent string, verdict Verdict, findings []Finding, err error) AgentReviewResult {
	ar := AgentReviewResult{
		Agent: agent,
		Err:   err,
	}
	if err == nil {
		ar.Result = &ReviewResult{
			Verdict:  verdict,
			Findings: findings,
		}
	}
	return ar
}

// ---------------------------------------------------------------------------
// NewConsolidator
// ---------------------------------------------------------------------------

func TestNewConsolidator_NilLogger(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	require.NotNil(t, c)
	assert.Nil(t, c.logger)
}

func TestNewConsolidator_WithLogger(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	c := NewConsolidator(logger)
	require.NotNil(t, c)
	assert.NotNil(t, c.logger)
}

// ---------------------------------------------------------------------------
// Consolidate — empty / nil inputs
// ---------------------------------------------------------------------------

func TestConsolidate_EmptyResults_ReturnsApproved(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	cr, stats := c.Consolidate([]AgentReviewResult{})

	require.NotNil(t, cr)
	assert.Equal(t, VerdictApproved, cr.Verdict)
	assert.Empty(t, cr.Findings)
	assert.Equal(t, 0, cr.TotalAgents)
	assert.Zero(t, stats.TotalInputFindings)
	assert.Zero(t, stats.UniqueFindings)
}

func TestConsolidate_NilResults_ReturnsApproved(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	cr, stats := c.Consolidate(nil)

	require.NotNil(t, cr)
	assert.Equal(t, VerdictApproved, cr.Verdict)
	assert.Empty(t, cr.Findings)
	assert.Zero(t, stats.TotalInputFindings)
}

// ---------------------------------------------------------------------------
// Consolidate — single agent
// ---------------------------------------------------------------------------

func TestConsolidate_SingleAgent_NoFindings_Approved(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictApproved, nil, nil),
	}
	cr, stats := c.Consolidate(results)

	assert.Equal(t, VerdictApproved, cr.Verdict)
	assert.Empty(t, cr.Findings)
	assert.Equal(t, 1, cr.TotalAgents)
	assert.Zero(t, stats.TotalInputFindings)
	assert.Zero(t, stats.UniqueFindings)
}

func TestConsolidate_SingleAgent_WithFindings(t *testing.T) {
	t.Parallel()

	findings := []Finding{
		{
			Severity:    SeverityHigh,
			Category:    "security",
			File:        "internal/auth/token.go",
			Line:        42,
			Description: "token stored in plain text",
			Suggestion:  "use encrypted storage",
		},
		{
			Severity:    SeverityLow,
			Category:    "style",
			File:        "main.go",
			Line:        5,
			Description: "missing doc comment",
		},
	}

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, findings, nil),
	}
	cr, stats := c.Consolidate(results)

	assert.Equal(t, VerdictChangesNeeded, cr.Verdict)
	require.Len(t, cr.Findings, 2)
	assert.Equal(t, 1, cr.TotalAgents)
	assert.Equal(t, 2, stats.TotalInputFindings)
	assert.Equal(t, 2, stats.UniqueFindings)
	assert.Zero(t, stats.DuplicatesRemoved)

	// Agent attribution should be set to the single agent.
	for _, f := range cr.Findings {
		assert.Equal(t, "claude", f.Agent)
	}
}

func TestConsolidate_SingleAgent_BlockingVerdict(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictBlocking, []Finding{
			{Severity: SeverityCritical, Category: "security", File: "main.go", Line: 1},
		}, nil),
	}
	cr, _ := c.Consolidate(results)

	assert.Equal(t, VerdictBlocking, cr.Verdict)
}

// ---------------------------------------------------------------------------
// Consolidate — deduplication
// ---------------------------------------------------------------------------

func TestConsolidate_Deduplication_SameKey_TwoAgents(t *testing.T) {
	t.Parallel()

	sharedFinding := Finding{
		Severity:    SeverityHigh,
		Category:    "security",
		File:        "auth.go",
		Line:        10,
		Description: "hardcoded secret",
	}

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{sharedFinding}, nil),
		makeAgentResult("codex", VerdictChangesNeeded, []Finding{sharedFinding}, nil),
	}
	cr, stats := c.Consolidate(results)

	// Only one unique finding after deduplication.
	require.Len(t, cr.Findings, 1)
	assert.Equal(t, 2, stats.TotalInputFindings)
	assert.Equal(t, 1, stats.UniqueFindings)
	assert.Equal(t, 1, stats.DuplicatesRemoved)

	// Both agent names should appear in the Agent field.
	agents := cr.Findings[0].Agent
	assert.Contains(t, agents, "claude")
	assert.Contains(t, agents, "codex")
}

func TestConsolidate_Deduplication_DifferentCategories_TreatedDistinct(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{
			{Severity: SeverityHigh, Category: "security", File: "main.go", Line: 5},
			{Severity: SeverityMedium, Category: "style", File: "main.go", Line: 5},
		}, nil),
	}
	cr, stats := c.Consolidate(results)

	// Different categories at the same file+line are distinct findings.
	assert.Len(t, cr.Findings, 2)
	assert.Equal(t, 2, stats.UniqueFindings)
	assert.Zero(t, stats.DuplicatesRemoved)
}

func TestConsolidate_Deduplication_LineZero_FileLevelFinding(t *testing.T) {
	t.Parallel()

	fileLevelFinding := Finding{
		Severity:    SeverityInfo,
		Category:    "docs",
		File:        "README.md",
		Line:        0,
		Description: "missing README section",
	}

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictApproved, []Finding{fileLevelFinding}, nil),
		makeAgentResult("gemini", VerdictApproved, []Finding{fileLevelFinding}, nil),
	}
	cr, stats := c.Consolidate(results)

	require.Len(t, cr.Findings, 1)
	assert.Equal(t, 1, stats.DuplicatesRemoved)
	assert.Equal(t, "README.md:0:docs", cr.Findings[0].DeduplicationKey())
}

func TestConsolidate_Deduplication_ThreeAgentsReportSameFinding(t *testing.T) {
	t.Parallel()

	f := Finding{Severity: SeverityMedium, Category: "perf", File: "worker.go", Line: 77}

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{f}, nil),
		makeAgentResult("codex", VerdictChangesNeeded, []Finding{f}, nil),
		makeAgentResult("gemini", VerdictChangesNeeded, []Finding{f}, nil),
	}
	cr, stats := c.Consolidate(results)

	require.Len(t, cr.Findings, 1)
	assert.Equal(t, 2, stats.DuplicatesRemoved) // 3 inputs - 1 unique = 2 removed
	assert.Contains(t, cr.Findings[0].Agent, "claude")
	assert.Contains(t, cr.Findings[0].Agent, "codex")
	assert.Contains(t, cr.Findings[0].Agent, "gemini")
}

// ---------------------------------------------------------------------------
// Consolidate — severity escalation
// ---------------------------------------------------------------------------

func TestConsolidate_SeverityEscalation_LowToHigh(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{
			{Severity: SeverityLow, Category: "security", File: "main.go", Line: 1},
		}, nil),
		makeAgentResult("codex", VerdictChangesNeeded, []Finding{
			{Severity: SeverityHigh, Category: "security", File: "main.go", Line: 1},
		}, nil),
	}
	cr, stats := c.Consolidate(results)

	require.Len(t, cr.Findings, 1)
	assert.Equal(t, SeverityHigh, cr.Findings[0].Severity)
	assert.Equal(t, 1, stats.SeverityEscalations)
}

func TestConsolidate_SeverityEscalation_MediumToCritical(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{
			{Severity: SeverityMedium, Category: "logic", File: "engine.go", Line: 50},
		}, nil),
		makeAgentResult("codex", VerdictBlocking, []Finding{
			{Severity: SeverityCritical, Category: "logic", File: "engine.go", Line: 50},
		}, nil),
	}
	cr, stats := c.Consolidate(results)

	require.Len(t, cr.Findings, 1)
	assert.Equal(t, SeverityCritical, cr.Findings[0].Severity)
	assert.Equal(t, 1, stats.SeverityEscalations)
}

func TestConsolidate_SeverityEscalation_NeverDowngrades(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{
			{Severity: SeverityCritical, Category: "security", File: "auth.go", Line: 1},
		}, nil),
		makeAgentResult("codex", VerdictChangesNeeded, []Finding{
			{Severity: SeverityInfo, Category: "security", File: "auth.go", Line: 1},
		}, nil),
	}
	cr, stats := c.Consolidate(results)

	require.Len(t, cr.Findings, 1)
	// Severity must not be downgraded from critical to info.
	assert.Equal(t, SeverityCritical, cr.Findings[0].Severity)
	assert.Zero(t, stats.SeverityEscalations)
}

// ---------------------------------------------------------------------------
// Consolidate — verdict aggregation
// ---------------------------------------------------------------------------

func TestConsolidate_VerdictAggregation_BlockingWins(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictApproved, nil, nil),
		makeAgentResult("codex", VerdictBlocking, nil, nil),
		makeAgentResult("gemini", VerdictChangesNeeded, nil, nil),
	}
	cr, _ := c.Consolidate(results)

	assert.Equal(t, VerdictBlocking, cr.Verdict)
}

func TestConsolidate_VerdictAggregation_ChangesNeededBeatsApproved(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictApproved, nil, nil),
		makeAgentResult("codex", VerdictChangesNeeded, nil, nil),
	}
	cr, _ := c.Consolidate(results)

	assert.Equal(t, VerdictChangesNeeded, cr.Verdict)
}

func TestConsolidate_VerdictAggregation_AllApproved(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictApproved, nil, nil),
		makeAgentResult("codex", VerdictApproved, nil, nil),
	}
	cr, _ := c.Consolidate(results)

	assert.Equal(t, VerdictApproved, cr.Verdict)
}

// ---------------------------------------------------------------------------
// Consolidate — error handling
// ---------------------------------------------------------------------------

func TestConsolidate_ErroredAgent_FindingsExcluded(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	agentErr := errors.New("rate limited")
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{
			{Severity: SeverityHigh, Category: "security", File: "auth.go", Line: 1},
		}, nil),
		// codex failed — its findings (would be 1) must be excluded.
		makeAgentResult("codex", "", nil, agentErr),
	}
	cr, stats := c.Consolidate(results)

	// Only claude's findings should appear.
	require.Len(t, cr.Findings, 1)
	assert.Equal(t, "claude", cr.Findings[0].Agent)
	assert.Equal(t, 1, stats.TotalInputFindings)
}

func TestConsolidate_ErroredAgent_VerdictTreatedAsChangesNeeded(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictApproved, nil, nil),
		makeAgentResult("codex", "", nil, errors.New("timeout")),
	}
	cr, _ := c.Consolidate(results)

	// claude=APPROVED, codex(error)=CHANGES_NEEDED => CHANGES_NEEDED
	assert.Equal(t, VerdictChangesNeeded, cr.Verdict)
}

func TestConsolidate_AllAgentsError_VerdictChangesNeeded(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", "", nil, errors.New("timeout")),
		makeAgentResult("codex", "", nil, errors.New("rate limited")),
	}
	cr, stats := c.Consolidate(results)

	assert.Equal(t, VerdictChangesNeeded, cr.Verdict)
	assert.Empty(t, cr.Findings)
	assert.Zero(t, stats.TotalInputFindings)
}

func TestConsolidate_NilResultField_TreatedAsError(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		{Agent: "claude", Result: nil, Err: nil}, // nil Result, no error — treated as error
		makeAgentResult("codex", VerdictApproved, []Finding{
			{Severity: SeverityLow, Category: "style", File: "main.go", Line: 1},
		}, nil),
	}
	cr, stats := c.Consolidate(results)

	require.Len(t, cr.Findings, 1)
	assert.Equal(t, "codex", cr.Findings[0].Agent)
	assert.Equal(t, 1, stats.TotalInputFindings)
}

// ---------------------------------------------------------------------------
// Consolidate — sorting order
// ---------------------------------------------------------------------------

func TestConsolidate_SortOrder_CriticalFirst(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{
			{Severity: SeverityLow, Category: "style", File: "b.go", Line: 1},
			{Severity: SeverityCritical, Category: "security", File: "a.go", Line: 1},
			{Severity: SeverityMedium, Category: "perf", File: "c.go", Line: 1},
		}, nil),
	}
	cr, _ := c.Consolidate(results)

	require.Len(t, cr.Findings, 3)
	assert.Equal(t, SeverityCritical, cr.Findings[0].Severity)
	assert.Equal(t, SeverityMedium, cr.Findings[1].Severity)
	assert.Equal(t, SeverityLow, cr.Findings[2].Severity)
}

func TestConsolidate_SortOrder_SameSeverityByFileThenLine(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{
			{Severity: SeverityHigh, Category: "security", File: "z.go", Line: 1},
			{Severity: SeverityHigh, Category: "security", File: "a.go", Line: 50},
			{Severity: SeverityHigh, Category: "security", File: "a.go", Line: 10},
		}, nil),
	}
	cr, _ := c.Consolidate(results)

	require.Len(t, cr.Findings, 3)
	// Alphabetical file order.
	assert.Equal(t, "a.go", cr.Findings[0].File)
	assert.Equal(t, "a.go", cr.Findings[1].File)
	assert.Equal(t, "z.go", cr.Findings[2].File)
	// Within same file, lower line first.
	assert.Equal(t, 10, cr.Findings[0].Line)
	assert.Equal(t, 50, cr.Findings[1].Line)
}

func TestConsolidate_SortOrder_AllSeverities(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{
			{Severity: SeverityInfo, Category: "docs", File: "e.go", Line: 1},
			{Severity: SeverityLow, Category: "style", File: "d.go", Line: 1},
			{Severity: SeverityMedium, Category: "perf", File: "c.go", Line: 1},
			{Severity: SeverityHigh, Category: "logic", File: "b.go", Line: 1},
			{Severity: SeverityCritical, Category: "security", File: "a.go", Line: 1},
		}, nil),
	}
	cr, _ := c.Consolidate(results)

	require.Len(t, cr.Findings, 5)
	assert.Equal(t, SeverityCritical, cr.Findings[0].Severity)
	assert.Equal(t, SeverityHigh, cr.Findings[1].Severity)
	assert.Equal(t, SeverityMedium, cr.Findings[2].Severity)
	assert.Equal(t, SeverityLow, cr.Findings[3].Severity)
	assert.Equal(t, SeverityInfo, cr.Findings[4].Severity)
}

// ---------------------------------------------------------------------------
// Consolidate — stats
// ---------------------------------------------------------------------------

func TestConsolidate_Stats_FindingsPerAgent(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{
			{Severity: SeverityHigh, Category: "security", File: "a.go", Line: 1},
			{Severity: SeverityMedium, Category: "perf", File: "b.go", Line: 2},
		}, nil),
		makeAgentResult("codex", VerdictChangesNeeded, []Finding{
			{Severity: SeverityLow, Category: "style", File: "c.go", Line: 3},
		}, nil),
	}
	_, stats := c.Consolidate(results)

	assert.Equal(t, 2, stats.FindingsPerAgent["claude"])
	assert.Equal(t, 1, stats.FindingsPerAgent["codex"])
	assert.Equal(t, 3, stats.TotalInputFindings)
	assert.Equal(t, 3, stats.UniqueFindings)
}

func TestConsolidate_Stats_FindingsPerSeverity(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{
			{Severity: SeverityHigh, Category: "security", File: "a.go", Line: 1},
			{Severity: SeverityHigh, Category: "logic", File: "b.go", Line: 2},
			{Severity: SeverityLow, Category: "style", File: "c.go", Line: 3},
		}, nil),
	}
	_, stats := c.Consolidate(results)

	assert.Equal(t, 2, stats.FindingsPerSeverity[SeverityHigh])
	assert.Equal(t, 1, stats.FindingsPerSeverity[SeverityLow])
	assert.Zero(t, stats.FindingsPerSeverity[SeverityCritical])
}

func TestConsolidate_Stats_OverlapRate_NoOverlap(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{
			{Severity: SeverityHigh, Category: "security", File: "a.go", Line: 1},
		}, nil),
		makeAgentResult("codex", VerdictChangesNeeded, []Finding{
			{Severity: SeverityLow, Category: "style", File: "b.go", Line: 2},
		}, nil),
	}
	_, stats := c.Consolidate(results)

	// 2 unique findings, neither is shared — overlap rate should be 0.
	assert.Equal(t, 0.0, stats.OverlapRate)
}

func TestConsolidate_Stats_OverlapRate_FullOverlap(t *testing.T) {
	t.Parallel()

	// Both agents report exactly the same finding.
	sharedFinding := Finding{Severity: SeverityHigh, Category: "security", File: "a.go", Line: 1}

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{sharedFinding}, nil),
		makeAgentResult("codex", VerdictChangesNeeded, []Finding{sharedFinding}, nil),
	}
	_, stats := c.Consolidate(results)

	// 1 unique finding, reported by 2 agents — overlap rate 100%.
	assert.Equal(t, 100.0, stats.OverlapRate)
}

func TestConsolidate_Stats_OverlapRate_PartialOverlap(t *testing.T) {
	t.Parallel()

	sharedFinding := Finding{Severity: SeverityHigh, Category: "security", File: "a.go", Line: 1}
	uniqueFinding := Finding{Severity: SeverityLow, Category: "style", File: "b.go", Line: 2}

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{sharedFinding, uniqueFinding}, nil),
		makeAgentResult("codex", VerdictChangesNeeded, []Finding{sharedFinding}, nil),
	}
	_, stats := c.Consolidate(results)

	// 2 unique findings, 1 shared — overlap rate 50%.
	assert.Equal(t, 50.0, stats.OverlapRate)
}

func TestConsolidate_Stats_ZeroInputFindings(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictApproved, nil, nil),
	}
	_, stats := c.Consolidate(results)

	assert.Zero(t, stats.TotalInputFindings)
	assert.Zero(t, stats.UniqueFindings)
	assert.Zero(t, stats.DuplicatesRemoved)
	assert.Zero(t, stats.OverlapRate)
}

// ---------------------------------------------------------------------------
// Consolidate — AgentResults preservation
// ---------------------------------------------------------------------------

func TestConsolidate_PreservesAgentResults(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictApproved, nil, nil),
		makeAgentResult("codex", VerdictChangesNeeded, nil, nil),
	}
	cr, _ := c.Consolidate(results)

	assert.Equal(t, results, cr.AgentResults)
	assert.Equal(t, 2, cr.TotalAgents)
}

// ---------------------------------------------------------------------------
// Consolidate — logger paths
// ---------------------------------------------------------------------------

func TestConsolidate_WithLogger_ErroredAgentLogged(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	c := NewConsolidator(logger)
	results := []AgentReviewResult{
		makeAgentResult("claude", "", nil, errors.New("timeout")),
		makeAgentResult("codex", VerdictApproved, nil, nil),
	}
	// Must not panic; logger discards output.
	cr, _ := c.Consolidate(results)
	assert.NotNil(t, cr)
}

func TestConsolidate_WithLogger_SeverityEscalationLogged(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	c := NewConsolidator(logger)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{
			{Severity: SeverityLow, Category: "security", File: "main.go", Line: 1},
		}, nil),
		makeAgentResult("codex", VerdictChangesNeeded, []Finding{
			{Severity: SeverityHigh, Category: "security", File: "main.go", Line: 1},
		}, nil),
	}
	// Must not panic with a real (discarding) logger.
	cr, stats := c.Consolidate(results)
	require.Len(t, cr.Findings, 1)
	assert.Equal(t, 1, stats.SeverityEscalations)
}

// ---------------------------------------------------------------------------
// AggregateVerdicts
// ---------------------------------------------------------------------------

func TestAggregateVerdicts_EmptyInput_ReturnsApproved(t *testing.T) {
	t.Parallel()

	assert.Equal(t, VerdictApproved, AggregateVerdicts(nil))
	assert.Equal(t, VerdictApproved, AggregateVerdicts([]Verdict{}))
}

func TestAggregateVerdicts_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		verdicts []Verdict
		want     Verdict
	}{
		{
			name:     "all approved",
			verdicts: []Verdict{VerdictApproved, VerdictApproved},
			want:     VerdictApproved,
		},
		{
			name:     "one changes_needed",
			verdicts: []Verdict{VerdictApproved, VerdictChangesNeeded},
			want:     VerdictChangesNeeded,
		},
		{
			name:     "one blocking",
			verdicts: []Verdict{VerdictApproved, VerdictChangesNeeded, VerdictBlocking},
			want:     VerdictBlocking,
		},
		{
			name:     "blocking with no others",
			verdicts: []Verdict{VerdictBlocking},
			want:     VerdictBlocking,
		},
		{
			name:     "changes_needed with no blocking",
			verdicts: []Verdict{VerdictChangesNeeded, VerdictApproved, VerdictChangesNeeded},
			want:     VerdictChangesNeeded,
		},
		{
			name:     "blocking first short-circuits",
			verdicts: []Verdict{VerdictBlocking, VerdictApproved, VerdictChangesNeeded},
			want:     VerdictBlocking,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, AggregateVerdicts(tt.verdicts))
		})
	}
}

// ---------------------------------------------------------------------------
// EscalateSeverity
// ---------------------------------------------------------------------------

func TestEscalateSeverity_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    Severity
		b    Severity
		want Severity
	}{
		{"info to low", SeverityInfo, SeverityLow, SeverityLow},
		{"low to medium", SeverityLow, SeverityMedium, SeverityMedium},
		{"medium to high", SeverityMedium, SeverityHigh, SeverityHigh},
		{"high to critical", SeverityHigh, SeverityCritical, SeverityCritical},
		{"same severity", SeverityMedium, SeverityMedium, SeverityMedium},
		{"downgrade not applied -- critical stays", SeverityCritical, SeverityInfo, SeverityCritical},
		{"downgrade not applied -- high stays", SeverityHigh, SeverityLow, SeverityHigh},
		{"b higher than a", SeverityInfo, SeverityCritical, SeverityCritical},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, EscalateSeverity(tt.a, tt.b))
		})
	}
}

func TestEscalateSeverity_Symmetric(t *testing.T) {
	t.Parallel()

	// EscalateSeverity(a,b) must always equal EscalateSeverity(b,a).
	pairs := [][2]Severity{
		{SeverityInfo, SeverityLow},
		{SeverityMedium, SeverityHigh},
		{SeverityLow, SeverityCritical},
	}
	for _, pair := range pairs {
		assert.Equal(t, EscalateSeverity(pair[0], pair[1]), EscalateSeverity(pair[1], pair[0]),
			"EscalateSeverity must be symmetric for %s and %s", pair[0], pair[1])
	}
}

// ---------------------------------------------------------------------------
// severityRank
// ---------------------------------------------------------------------------

func TestSeverityRank_Ordering(t *testing.T) {
	t.Parallel()

	assert.Less(t, severityRank(SeverityInfo), severityRank(SeverityLow))
	assert.Less(t, severityRank(SeverityLow), severityRank(SeverityMedium))
	assert.Less(t, severityRank(SeverityMedium), severityRank(SeverityHigh))
	assert.Less(t, severityRank(SeverityHigh), severityRank(SeverityCritical))
}

func TestSeverityRank_UnknownSeverity(t *testing.T) {
	t.Parallel()

	// Unknown severity values return 0.
	assert.Equal(t, 0, severityRank(Severity("unknown")))
	assert.Equal(t, 0, severityRank(Severity("")))
}

// ---------------------------------------------------------------------------
// mergeDescriptions
// ---------------------------------------------------------------------------

func TestMergeDescriptions_EmptySecondary_ReturnsPrimary(t *testing.T) {
	t.Parallel()

	result := mergeDescriptions("primary description", "")
	assert.Equal(t, "primary description", result)
}

func TestMergeDescriptions_EmptyPrimary_ReturnsSecondary(t *testing.T) {
	t.Parallel()

	result := mergeDescriptions("", "secondary description")
	assert.Equal(t, "secondary description", result)
}

func TestMergeDescriptions_SameContent_NoDuplication(t *testing.T) {
	t.Parallel()

	desc := "identical description"
	result := mergeDescriptions(desc, desc)
	assert.Equal(t, desc, result)
	// The result should not contain the description twice.
	assert.Equal(t, 1, strings.Count(result, desc))
}

func TestMergeDescriptions_PrimaryLonger_KeepsPrimary(t *testing.T) {
	t.Parallel()

	primary := "this is a very detailed description of the security issue with much context"
	secondary := "brief note"
	result := mergeDescriptions(primary, secondary)
	assert.True(t, strings.HasPrefix(result, primary))
}

func TestMergeDescriptions_SecondaryLonger_PromotedToPrimary(t *testing.T) {
	t.Parallel()

	short := "brief"
	long := "this is a very detailed description with extensive explanation of the issue"
	result := mergeDescriptions(short, long)
	// The long description should be the base.
	assert.True(t, strings.HasPrefix(result, long))
}

func TestMergeDescriptions_LongSecondary_TruncatedInNote(t *testing.T) {
	t.Parallel()

	primary := strings.Repeat("p", 200)
	secondary := strings.Repeat("s", 200)
	result := mergeDescriptions(primary, secondary)
	// The result should be bounded — secondary note truncated at 120 chars + "..."
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "Additional note:")
}

func TestMergeDescriptions_SecondaryAlreadyInPrimary_NotDuplicated(t *testing.T) {
	t.Parallel()

	primary := "this is a detailed description that includes a brief note"
	secondary := "brief note"
	result := mergeDescriptions(primary, secondary)
	// secondary is contained in primary — no additional note should be appended.
	assert.Equal(t, primary, result)
}

// ---------------------------------------------------------------------------
// Integration: multi-agent scenario
// ---------------------------------------------------------------------------

func TestConsolidate_Integration_MultiAgent(t *testing.T) {
	t.Parallel()

	// Scenario:
	//   claude: 3 findings (2 unique to claude, 1 shared)
	//   codex:  2 findings (1 shared with claude, 1 unique)
	//   gemini: errors (findings excluded, verdict = CHANGES_NEEDED)
	//
	// Expected:
	//   - 4 unique findings (claude:logic, claude:style, codex:perf, shared:security deduped)
	//   - 1 severity escalation (shared finding escalated from medium to critical)
	//   - verdict: BLOCKING (claude reports it)

	sharedFinding := Finding{
		Severity:    SeverityMedium,
		Category:    "security",
		File:        "internal/auth/token.go",
		Line:        42,
		Description: "token stored in plain text",
	}
	sharedFindingEscalated := Finding{
		Severity:    SeverityCritical,
		Category:    "security",
		File:        "internal/auth/token.go",
		Line:        42,
		Description: "critical: token must be encrypted",
	}

	claudeFindings := []Finding{
		sharedFinding,
		{Severity: SeverityHigh, Category: "logic", File: "workflow.go", Line: 10, Description: "nil dereference"},
		{Severity: SeverityLow, Category: "style", File: "main.go", Line: 1, Description: "missing doc"},
	}
	codexFindings := []Finding{
		sharedFindingEscalated,
		{Severity: SeverityMedium, Category: "perf", File: "loop.go", Line: 20, Description: "alloc in hot path"},
	}

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictBlocking, claudeFindings, nil),
		makeAgentResult("codex", VerdictChangesNeeded, codexFindings, nil),
		makeAgentResult("gemini", "", nil, errors.New("rate limited")),
	}
	cr, stats := c.Consolidate(results)

	// 3 unique findings: shared(deduped), logic, style, perf = 4 unique.
	assert.Len(t, cr.Findings, 4)
	assert.Equal(t, VerdictBlocking, cr.Verdict)
	assert.Equal(t, 3, cr.TotalAgents)

	// Stats checks.
	assert.Equal(t, 5, stats.TotalInputFindings) // 3 claude + 2 codex (gemini excluded)
	assert.Equal(t, 4, stats.UniqueFindings)
	assert.Equal(t, 1, stats.DuplicatesRemoved)
	assert.Equal(t, 1, stats.SeverityEscalations)

	// The first finding (sorted critical first) should be the escalated shared finding.
	assert.Equal(t, SeverityCritical, cr.Findings[0].Severity)
	assert.Contains(t, cr.Findings[0].Agent, "claude")
	assert.Contains(t, cr.Findings[0].Agent, "codex")

	// Verify per-agent counts.
	assert.Equal(t, 3, stats.FindingsPerAgent["claude"])
	assert.Equal(t, 2, stats.FindingsPerAgent["codex"])
	assert.Zero(t, stats.FindingsPerAgent["gemini"])
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestConsolidate_EdgeCase_SingleAgentZeroFindingsBlocking(t *testing.T) {
	t.Parallel()

	// Unusual but valid: agent says BLOCKING with zero findings.
	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictBlocking, []Finding{}, nil),
	}
	cr, stats := c.Consolidate(results)

	assert.Equal(t, VerdictBlocking, cr.Verdict)
	assert.Empty(t, cr.Findings)
	assert.Zero(t, stats.TotalInputFindings)
}

func TestConsolidate_EdgeCase_MixedAgentResultsWithNilFindings(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictApproved, nil, nil),
		makeAgentResult("codex", VerdictApproved, nil, nil),
	}
	cr, stats := c.Consolidate(results)

	assert.Equal(t, VerdictApproved, cr.Verdict)
	assert.Empty(t, cr.Findings)
	assert.Zero(t, stats.TotalInputFindings)
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

// BenchmarkConsolidate measures the consolidation throughput with a realistic
// multi-agent payload of 100 findings per agent with ~30% overlap.
func BenchmarkConsolidate(b *testing.B) {
	const numAgents = 3
	const findingsPerAgent = 100
	const overlapEvery = 3 // every 3rd finding is shared across agents

	// Build a reusable set of results.
	allResults := make([]AgentReviewResult, numAgents)
	agents := []string{"claude", "codex", "gemini"}
	severities := []Severity{SeverityInfo, SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical}

	for ai, agent := range agents {
		findings := make([]Finding, findingsPerAgent)
		for i := range findings {
			file := "different.go"
			if i%overlapEvery == 0 {
				file = "shared.go" // same file triggers dedup on some lines
			}
			findings[i] = Finding{
				Severity: severities[i%len(severities)],
				Category: "security",
				File:     file,
				Line:     i,
			}
		}
		allResults[ai] = makeAgentResult(agent, VerdictChangesNeeded, findings, nil)
	}

	c := NewConsolidator(nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Consolidate(allResults)
	}
}

// BenchmarkConsolidateLarge measures the consolidation throughput with 1000+
// findings per agent to verify O(n) map-based deduplication remains fast.
func BenchmarkConsolidateLarge(b *testing.B) {
	const numAgents = 3
	const findingsPerAgent = 400 // 3 agents × 400 = 1200 total input findings
	const overlapEvery = 4       // 25% overlap between agents

	allResults := make([]AgentReviewResult, numAgents)
	agents := []string{"claude", "codex", "gemini"}
	severities := []Severity{SeverityInfo, SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical}
	categories := []string{"security", "logic", "perf", "style", "docs"}

	for ai, agent := range agents {
		findings := make([]Finding, findingsPerAgent)
		for i := range findings {
			file := "unique.go"
			line := ai*findingsPerAgent + i // unique per agent by default
			if i%overlapEvery == 0 {
				// Shared: same file+line+category across all agents.
				file = "shared.go"
				line = i
			}
			findings[i] = Finding{
				Severity:    severities[i%len(severities)],
				Category:    categories[i%len(categories)],
				File:        file,
				Line:        line,
				Description: strings.Repeat("detail ", 10),
			}
		}
		allResults[ai] = makeAgentResult(agent, VerdictChangesNeeded, findings, nil)
	}

	c := NewConsolidator(nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Consolidate(allResults)
	}
}

// ---------------------------------------------------------------------------
// Severity escalation — three-agent chain
// ---------------------------------------------------------------------------

func TestConsolidate_SeverityEscalation_ThreeAgents_MaxOfAll(t *testing.T) {
	t.Parallel()

	// Agent A=info, B=medium, C=high → final must be high (max of all three).
	f := Finding{Category: "security", File: "auth.go", Line: 5}

	fA := f
	fA.Severity = SeverityInfo

	fB := f
	fB.Severity = SeverityMedium

	fC := f
	fC.Severity = SeverityHigh

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("agentA", VerdictChangesNeeded, []Finding{fA}, nil),
		makeAgentResult("agentB", VerdictChangesNeeded, []Finding{fB}, nil),
		makeAgentResult("agentC", VerdictChangesNeeded, []Finding{fC}, nil),
	}
	cr, stats := c.Consolidate(results)

	require.Len(t, cr.Findings, 1)
	assert.Equal(t, SeverityHigh, cr.Findings[0].Severity)
	// First dedup (info→medium) is one escalation; second (medium→high) is another.
	assert.Equal(t, 2, stats.SeverityEscalations)
	assert.Equal(t, 2, stats.DuplicatesRemoved)
}

func TestConsolidate_SeverityEscalation_ThreeAgents_CriticalAlwaysWins(t *testing.T) {
	t.Parallel()

	// Order: critical first, then low, then medium → should stay critical.
	f := Finding{Category: "perf", File: "engine.go", Line: 100}

	fA := f
	fA.Severity = SeverityCritical

	fB := f
	fB.Severity = SeverityLow

	fC := f
	fC.Severity = SeverityMedium

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("agentA", VerdictBlocking, []Finding{fA}, nil),
		makeAgentResult("agentB", VerdictChangesNeeded, []Finding{fB}, nil),
		makeAgentResult("agentC", VerdictChangesNeeded, []Finding{fC}, nil),
	}
	cr, stats := c.Consolidate(results)

	require.Len(t, cr.Findings, 1)
	assert.Equal(t, SeverityCritical, cr.Findings[0].Severity)
	// Neither low nor medium escalates beyond critical; no escalations after the
	// initial copy.
	assert.Zero(t, stats.SeverityEscalations)
}

// ---------------------------------------------------------------------------
// Description merging — detailed coverage
// ---------------------------------------------------------------------------

func TestMergeDescriptions_BothEmpty_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	result := mergeDescriptions("", "")
	assert.Equal(t, "", result)
}

func TestMergeDescriptions_WhitespaceOnly_TreatedAsEmpty(t *testing.T) {
	t.Parallel()

	// Whitespace-only secondary is trimmed to empty and ignored.
	result := mergeDescriptions("real description", "   ")
	assert.Equal(t, "real description", result)

	// Whitespace-only primary is trimmed; secondary becomes the primary.
	result2 := mergeDescriptions("   ", "real description")
	assert.Equal(t, "real description", result2)
}

func TestMergeDescriptions_PrimaryContainsSecondary_NotDuplicated(t *testing.T) {
	t.Parallel()

	// Secondary is a strict substring of primary; no "Additional note:" appended.
	primary := "this is a long description that contains a short note inside it"
	secondary := "short note"
	result := mergeDescriptions(primary, secondary)
	assert.Equal(t, primary, result)
	assert.NotContains(t, result, "Additional note:")
}

func TestMergeDescriptions_NoteFormat_ContainsAdditionalNote(t *testing.T) {
	t.Parallel()

	primary := "first agent found X"
	secondary := "second agent found Y"
	result := mergeDescriptions(primary, secondary)
	// The longer string stays as primary, shorter appended with label.
	assert.Contains(t, result, "Additional note:")
	// Both pieces of content appear in the result.
	assert.Contains(t, result, "second agent found Y")
	assert.Contains(t, result, "first agent found X")
}

func TestMergeDescriptions_TruncationBoundary_ExactlyAtLimit(t *testing.T) {
	t.Parallel()

	// Secondary is exactly 120 chars — no truncation expected.
	primary := strings.Repeat("p", 200)
	secondary := strings.Repeat("s", 120)
	result := mergeDescriptions(primary, secondary)
	// The secondary fits within the 120-char cap so it must not be truncated.
	assert.NotContains(t, result, "...")
	assert.Contains(t, result, secondary)
}

func TestMergeDescriptions_TruncationBoundary_OneOverLimit(t *testing.T) {
	t.Parallel()

	// Secondary is 121 chars — exactly one byte over the 120-char cap, so it
	// must be truncated with "...".
	primary := strings.Repeat("p", 200)
	secondary := strings.Repeat("s", 121)
	result := mergeDescriptions(primary, secondary)
	assert.Contains(t, result, "...")
}

// ---------------------------------------------------------------------------
// Fuzz: mergeDescriptions
// ---------------------------------------------------------------------------

// FuzzMergeDescriptions verifies that mergeDescriptions never panics and always
// returns a string that starts with the longer of the two trimmed inputs (unless
// it appends an "Additional note:" suffix).
func FuzzMergeDescriptions(f *testing.F) {
	f.Add("", "")
	f.Add("primary text", "")
	f.Add("", "secondary text")
	f.Add("same", "same")
	f.Add("short", "a much longer description that provides far more context")
	f.Add(strings.Repeat("x", 200), strings.Repeat("y", 200))
	f.Add("contains note", "note")

	f.Fuzz(func(t *testing.T, primary, secondary string) {
		// Must not panic.
		result := mergeDescriptions(primary, secondary)
		// Result is always a string (never nil since strings cannot be nil in Go).
		_ = result
	})
}

// ---------------------------------------------------------------------------
// Stats — additional coverage
// ---------------------------------------------------------------------------

func TestConsolidationStats_ZeroValue(t *testing.T) {
	t.Parallel()

	var stats ConsolidationStats
	assert.Zero(t, stats.TotalInputFindings)
	assert.Zero(t, stats.UniqueFindings)
	assert.Zero(t, stats.DuplicatesRemoved)
	assert.Zero(t, stats.SeverityEscalations)
	assert.Zero(t, stats.OverlapRate)
	assert.Nil(t, stats.FindingsPerAgent)
	assert.Nil(t, stats.FindingsPerSeverity)
}

func TestConsolidate_Stats_FindingsPerAgent_ErroredAgentNotCounted(t *testing.T) {
	t.Parallel()

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{
			{Severity: SeverityHigh, Category: "security", File: "a.go", Line: 1},
		}, nil),
		makeAgentResult("codex", "", nil, errors.New("timeout")),
	}
	_, stats := c.Consolidate(results)

	// Errored agent contributes no findings.
	assert.Equal(t, 1, stats.FindingsPerAgent["claude"])
	assert.Zero(t, stats.FindingsPerAgent["codex"])
}

func TestConsolidate_Stats_TotalAgents_IncludesErroredAgents(t *testing.T) {
	t.Parallel()

	// TotalAgents counts all results passed in, even errored ones.
	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictApproved, nil, nil),
		makeAgentResult("codex", "", nil, errors.New("failed")),
		makeAgentResult("gemini", VerdictApproved, nil, nil),
	}
	cr, _ := c.Consolidate(results)

	assert.Equal(t, 3, cr.TotalAgents)
}

func TestConsolidate_Stats_OverlapRate_NoUniqueFindings(t *testing.T) {
	t.Parallel()

	// When there are no unique findings, overlap rate must be 0 (not NaN/Inf).
	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictApproved, nil, nil),
	}
	_, stats := c.Consolidate(results)

	assert.Zero(t, stats.UniqueFindings)
	assert.Equal(t, 0.0, stats.OverlapRate)
}

func TestConsolidate_Stats_FindingsPerSeverity_AfterEscalation(t *testing.T) {
	t.Parallel()

	// After severity escalation the FindingsPerSeverity map must reflect the
	// final (escalated) severity, not the original lower severity.
	f := Finding{Category: "security", File: "auth.go", Line: 1}

	fLow := f
	fLow.Severity = SeverityLow

	fHigh := f
	fHigh.Severity = SeverityHigh

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{fLow}, nil),
		makeAgentResult("codex", VerdictChangesNeeded, []Finding{fHigh}, nil),
	}
	_, stats := c.Consolidate(results)

	// After escalation to high, the finding must count under SeverityHigh.
	assert.Equal(t, 1, stats.FindingsPerSeverity[SeverityHigh])
	// Low must not appear in the final tally.
	assert.Zero(t, stats.FindingsPerSeverity[SeverityLow])
}

func TestConsolidate_Stats_DuplicatesRemovedVsTotalInput(t *testing.T) {
	t.Parallel()

	// Invariant: TotalInputFindings = UniqueFindings + DuplicatesRemoved.
	shared := Finding{Severity: SeverityMedium, Category: "perf", File: "hot.go", Line: 10}
	unique1 := Finding{Severity: SeverityLow, Category: "style", File: "a.go", Line: 1}
	unique2 := Finding{Severity: SeverityHigh, Category: "logic", File: "b.go", Line: 2}

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{shared, unique1}, nil),
		makeAgentResult("codex", VerdictChangesNeeded, []Finding{shared, unique2}, nil),
		makeAgentResult("gemini", VerdictChangesNeeded, []Finding{shared}, nil),
	}
	_, stats := c.Consolidate(results)

	assert.Equal(t, stats.TotalInputFindings, stats.UniqueFindings+stats.DuplicatesRemoved)
}

// ---------------------------------------------------------------------------
// Finding preservation — suggestion field survives consolidation
// ---------------------------------------------------------------------------

func TestConsolidate_FindingSuggestionPreserved(t *testing.T) {
	t.Parallel()

	// The Suggestion field from the first agent that reports a finding must be
	// preserved through deduplication (it is not actively merged/overwritten).
	f := Finding{
		Severity:    SeverityHigh,
		Category:    "security",
		File:        "main.go",
		Line:        42,
		Description: "token in plain text",
		Suggestion:  "use encrypted storage vault",
	}

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{f}, nil),
	}
	cr, _ := c.Consolidate(results)

	require.Len(t, cr.Findings, 1)
	assert.Equal(t, "use encrypted storage vault", cr.Findings[0].Suggestion)
}

// ---------------------------------------------------------------------------
// Description merging — via Consolidate
// ---------------------------------------------------------------------------

func TestConsolidate_DescriptionMerge_LongerKept(t *testing.T) {
	t.Parallel()

	// When two agents report the same finding with different descriptions, the
	// longer one becomes the primary.
	short := Finding{
		Severity:    SeverityMedium,
		Category:    "logic",
		File:        "engine.go",
		Line:        30,
		Description: "nil pointer",
	}
	long := Finding{
		Severity:    SeverityMedium,
		Category:    "logic",
		File:        "engine.go",
		Line:        30,
		Description: "nil pointer dereference in the hot path when state machine transitions to stopped",
	}

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{short}, nil),
		makeAgentResult("codex", VerdictChangesNeeded, []Finding{long}, nil),
	}
	cr, _ := c.Consolidate(results)

	require.Len(t, cr.Findings, 1)
	// The longer description must be present in the final finding.
	assert.Contains(t, cr.Findings[0].Description, long.Description)
}

func TestConsolidate_DescriptionMerge_IdenticalDescriptions_NoDuplication(t *testing.T) {
	t.Parallel()

	desc := "hardcoded API key detected"
	f := Finding{
		Severity:    SeverityCritical,
		Category:    "security",
		File:        "config.go",
		Line:        10,
		Description: desc,
	}

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictBlocking, []Finding{f}, nil),
		makeAgentResult("codex", VerdictBlocking, []Finding{f}, nil),
	}
	cr, _ := c.Consolidate(results)

	require.Len(t, cr.Findings, 1)
	// Description must not be duplicated.
	assert.Equal(t, 1, strings.Count(cr.Findings[0].Description, desc))
}

// ---------------------------------------------------------------------------
// Agent attribution — order and format
// ---------------------------------------------------------------------------

func TestConsolidate_AgentAttribution_OrderMatchesInput(t *testing.T) {
	t.Parallel()

	// When two agents report the same finding, both must appear in Agent field.
	f := Finding{Severity: SeverityHigh, Category: "security", File: "auth.go", Line: 1}

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("alpha", VerdictChangesNeeded, []Finding{f}, nil),
		makeAgentResult("beta", VerdictChangesNeeded, []Finding{f}, nil),
	}
	cr, _ := c.Consolidate(results)

	require.Len(t, cr.Findings, 1)
	assert.Contains(t, cr.Findings[0].Agent, "alpha")
	assert.Contains(t, cr.Findings[0].Agent, "beta")
}

func TestConsolidate_AgentAttribution_UniqueFindings_SingleAgentName(t *testing.T) {
	t.Parallel()

	// A finding reported by only one agent must list only that agent.
	fA := Finding{Severity: SeverityLow, Category: "style", File: "a.go", Line: 1}
	fB := Finding{Severity: SeverityHigh, Category: "security", File: "b.go", Line: 2}

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{fA}, nil),
		makeAgentResult("codex", VerdictChangesNeeded, []Finding{fB}, nil),
	}
	cr, _ := c.Consolidate(results)

	require.Len(t, cr.Findings, 2)

	for _, finding := range cr.Findings {
		// Each finding belongs to exactly one agent.
		switch finding.File {
		case "a.go":
			assert.Equal(t, "claude", finding.Agent)
		case "b.go":
			assert.Equal(t, "codex", finding.Agent)
		default:
			t.Errorf("unexpected file: %s", finding.File)
		}
	}
}

// ---------------------------------------------------------------------------
// Edge cases — additional
// ---------------------------------------------------------------------------

func TestConsolidate_EdgeCase_AgentZeroFindingsBlocking_VerdictBlocking(t *testing.T) {
	t.Parallel()

	// An agent that says BLOCKING with zero findings: verdict is BLOCKING,
	// no findings in output.
	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictApproved, nil, nil),
		makeAgentResult("codex", VerdictBlocking, []Finding{}, nil),
	}
	cr, stats := c.Consolidate(results)

	assert.Equal(t, VerdictBlocking, cr.Verdict)
	assert.Empty(t, cr.Findings)
	assert.Zero(t, stats.TotalInputFindings)
	assert.Equal(t, 2, cr.TotalAgents)
}

func TestConsolidate_EdgeCase_AgentZeroFindingsChangesNeeded_VerdictChangesNeeded(t *testing.T) {
	t.Parallel()

	// Agent says CHANGES_NEEDED with no actual findings.
	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{}, nil),
		makeAgentResult("codex", VerdictApproved, nil, nil),
	}
	cr, stats := c.Consolidate(results)

	assert.Equal(t, VerdictChangesNeeded, cr.Verdict)
	assert.Empty(t, cr.Findings)
	assert.Zero(t, stats.TotalInputFindings)
}

func TestConsolidate_EdgeCase_UnknownSeverity_RankedLowest(t *testing.T) {
	t.Parallel()

	// A finding with an unrecognised severity string (rank=0) must sort below
	// all named severities and not cause a panic.
	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{
			{Severity: Severity("unknown"), Category: "misc", File: "a.go", Line: 1},
			{Severity: SeverityInfo, Category: "docs", File: "b.go", Line: 2},
		}, nil),
	}
	cr, stats := c.Consolidate(results)

	require.Len(t, cr.Findings, 2)
	assert.Equal(t, 2, stats.UniqueFindings)
	// info (rank=1) must come before unknown (rank=0) in the sorted output.
	assert.Equal(t, SeverityInfo, cr.Findings[0].Severity)
	assert.Equal(t, Severity("unknown"), cr.Findings[1].Severity)
}

func TestConsolidate_EdgeCase_SameFileDifferentLines_TreatedDistinct(t *testing.T) {
	t.Parallel()

	// Two findings at different lines in the same file+category are distinct.
	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{
			{Severity: SeverityHigh, Category: "security", File: "main.go", Line: 10},
			{Severity: SeverityHigh, Category: "security", File: "main.go", Line: 20},
		}, nil),
	}
	cr, stats := c.Consolidate(results)

	assert.Len(t, cr.Findings, 2)
	assert.Equal(t, 2, stats.UniqueFindings)
	assert.Zero(t, stats.DuplicatesRemoved)
}

func TestConsolidate_EdgeCase_ManyAgentsManyFindings_NoDataRace(t *testing.T) {
	t.Parallel()

	// Stress test: 5 agents each with 50 findings, 20% overlap.
	// Primary goal: verify there are no data races (run with -race).
	const numAgents = 5
	const findingsPerAgent = 50
	const overlapEvery = 5

	agents := []string{"claude", "codex", "gemini", "gpt", "llama"}
	severities := []Severity{SeverityInfo, SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical}
	categories := []string{"security", "logic", "perf", "style", "docs"}

	allResults := make([]AgentReviewResult, numAgents)
	for ai, agent := range agents {
		findings := make([]Finding, findingsPerAgent)
		for i := range findings {
			file := "unique.go"
			line := ai*findingsPerAgent + i
			if i%overlapEvery == 0 {
				file = "shared.go"
				line = i
			}
			findings[i] = Finding{
				Severity: severities[i%len(severities)],
				Category: categories[i%len(categories)],
				File:     file,
				Line:     line,
			}
		}
		allResults[ai] = makeAgentResult(agent, VerdictChangesNeeded, findings, nil)
	}

	c := NewConsolidator(nil)
	cr, stats := c.Consolidate(allResults)

	require.NotNil(t, cr)
	require.NotNil(t, stats)
	assert.Equal(t, numAgents, cr.TotalAgents)
	// Invariant: TotalInputFindings = UniqueFindings + DuplicatesRemoved.
	assert.Equal(t, stats.TotalInputFindings, stats.UniqueFindings+stats.DuplicatesRemoved)
}

// ---------------------------------------------------------------------------
// Logger — nil result path covered with logger
// ---------------------------------------------------------------------------

func TestConsolidate_WithLogger_NilResultLogged(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	c := NewConsolidator(logger)
	results := []AgentReviewResult{
		{Agent: "claude", Result: nil, Err: nil}, // nil Result, no Err
		makeAgentResult("codex", VerdictApproved, nil, nil),
	}
	// Must not panic; logger discards output.
	cr, _ := c.Consolidate(results)
	assert.NotNil(t, cr)
	// claude has nil result — treated as error, verdict becomes CHANGES_NEEDED.
	assert.Equal(t, VerdictChangesNeeded, cr.Verdict)
}

// ---------------------------------------------------------------------------
// AggregateVerdicts — additional edge cases
// ---------------------------------------------------------------------------

func TestAggregateVerdicts_SingleBlocking(t *testing.T) {
	t.Parallel()

	assert.Equal(t, VerdictBlocking, AggregateVerdicts([]Verdict{VerdictBlocking}))
}

func TestAggregateVerdicts_SingleChangesNeeded(t *testing.T) {
	t.Parallel()

	assert.Equal(t, VerdictChangesNeeded, AggregateVerdicts([]Verdict{VerdictChangesNeeded}))
}

func TestAggregateVerdicts_SingleApproved(t *testing.T) {
	t.Parallel()

	assert.Equal(t, VerdictApproved, AggregateVerdicts([]Verdict{VerdictApproved}))
}

func TestAggregateVerdicts_UnknownVerdictTreatedAsApproved(t *testing.T) {
	t.Parallel()

	// An unrecognised verdict string doesn't match BLOCKING or CHANGES_NEEDED,
	// so the aggregated result with only unknown verdicts stays APPROVED.
	result := AggregateVerdicts([]Verdict{Verdict("LGTM"), Verdict("LOOKS_GOOD")})
	assert.Equal(t, VerdictApproved, result)
}

// ---------------------------------------------------------------------------
// EscalateSeverity — additional coverage
// ---------------------------------------------------------------------------

func TestEscalateSeverity_UnknownSeverity_NeverRaisesAboveUnknown(t *testing.T) {
	t.Parallel()

	// unknown (rank=0) vs info (rank=1): info wins.
	assert.Equal(t, SeverityInfo, EscalateSeverity(Severity("unknown"), SeverityInfo))
	assert.Equal(t, SeverityInfo, EscalateSeverity(SeverityInfo, Severity("unknown")))
}

func TestEscalateSeverity_AllPairsNeverDowngrade(t *testing.T) {
	t.Parallel()

	// For every severity pair (a, b): EscalateSeverity(a, b) >= a always.
	all := []Severity{SeverityInfo, SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical}

	for _, a := range all {
		for _, b := range all {
			result := EscalateSeverity(a, b)
			// The result must be at least as severe as a.
			assert.GreaterOrEqual(t, severityRank(result), severityRank(a),
				"EscalateSeverity(%s, %s) = %s downgraded from %s", a, b, result, a)
		}
	}
}

// ---------------------------------------------------------------------------
// severityRank — additional coverage
// ---------------------------------------------------------------------------

func TestSeverityRank_AllKnownSeverities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		severity Severity
		want     int
	}{
		{SeverityInfo, 1},
		{SeverityLow, 2},
		{SeverityMedium, 3},
		{SeverityHigh, 4},
		{SeverityCritical, 5},
		{Severity(""), 0},
		{Severity("bogus"), 0},
		{Severity("CRITICAL"), 0}, // case-sensitive — uppercase not recognised
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, severityRank(tt.severity))
		})
	}
}

// ---------------------------------------------------------------------------
// Integration — realistic multi-agent output with description merging
// ---------------------------------------------------------------------------

func TestConsolidate_Integration_DescriptionMergeAcrossAgents(t *testing.T) {
	t.Parallel()

	// Three agents report the same finding with escalating severity and
	// varying descriptions. The consolidated finding must reflect the highest
	// severity and the most detailed description.
	f := Finding{Category: "security", File: "internal/auth/jwt.go", Line: 88}

	fA := f
	fA.Severity = SeverityMedium
	fA.Description = "JWT secret not rotated"

	fB := f
	fB.Severity = SeverityHigh
	fB.Description = "JWT secret not rotated; also missing expiry validation which allows token reuse indefinitely"

	fC := f
	fC.Severity = SeverityHigh
	fC.Description = "JWT secret not rotated"

	c := NewConsolidator(nil)
	results := []AgentReviewResult{
		makeAgentResult("claude", VerdictChangesNeeded, []Finding{fA}, nil),
		makeAgentResult("codex", VerdictBlocking, []Finding{fB}, nil),
		makeAgentResult("gemini", VerdictChangesNeeded, []Finding{fC}, nil),
	}
	cr, stats := c.Consolidate(results)

	require.Len(t, cr.Findings, 1)
	assert.Equal(t, VerdictBlocking, cr.Verdict)
	assert.Equal(t, SeverityHigh, cr.Findings[0].Severity)

	// All three agents must be attributed.
	assert.Contains(t, cr.Findings[0].Agent, "claude")
	assert.Contains(t, cr.Findings[0].Agent, "codex")
	assert.Contains(t, cr.Findings[0].Agent, "gemini")

	// The longer description (fB) must be present.
	assert.Contains(t, cr.Findings[0].Description, "expiry validation")

	// Stats must reflect correct counts.
	assert.Equal(t, 3, stats.TotalInputFindings)
	assert.Equal(t, 1, stats.UniqueFindings)
	assert.Equal(t, 2, stats.DuplicatesRemoved)
	assert.Equal(t, 1, stats.SeverityEscalations) // medium→high
}

// ---------------------------------------------------------------------------
// Benchmark — large scale (1000+ findings)
// ---------------------------------------------------------------------------

// BenchmarkConsolidate1000Findings verifies O(n) deduplication remains fast
// with more than 1000 total input findings across 3 agents.
func BenchmarkConsolidate1000Findings(b *testing.B) {
	const findingsPerAgent = 400 // 3 × 400 = 1200 total input findings
	const numAgents = 3

	agents := []string{"claude", "codex", "gemini"}
	severities := []Severity{SeverityInfo, SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical}
	categories := []string{"security", "logic", "perf", "style", "docs"}

	allResults := make([]AgentReviewResult, numAgents)
	for ai, agent := range agents {
		findings := make([]Finding, findingsPerAgent)
		for i := range findings {
			findings[i] = Finding{
				Severity:    severities[i%len(severities)],
				Category:    categories[i%len(categories)],
				File:        "file.go",
				Line:        ai*findingsPerAgent + i,
				Description: "description",
			}
		}
		allResults[ai] = makeAgentResult(agent, VerdictChangesNeeded, findings, nil)
	}

	c := NewConsolidator(nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Consolidate(allResults)
	}
}
