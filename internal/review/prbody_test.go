package review

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
)

// --- GenerateTitle tests ----------------------------------------------------

func TestPRBodyGenerator_GenerateTitle(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	tests := []struct {
		name string
		data PRBodyData
		want string
	}{
		{
			name: "single task with title",
			data: PRBodyData{
				TasksCompleted: []TaskSummary{{ID: "T-007", Title: "Add logging"}},
			},
			want: "T-007: Add logging",
		},
		{
			name: "single task without title",
			data: PRBodyData{
				TasksCompleted: []TaskSummary{{ID: "T-007"}},
			},
			want: "T-007",
		},
		{
			name: "two tasks no phase",
			data: PRBodyData{
				TasksCompleted: []TaskSummary{
					{ID: "T-007", Title: "Foo"},
					{ID: "T-008", Title: "Bar"},
				},
			},
			want: "Tasks T-007, T-008",
		},
		{
			name: "four tasks no phase -- truncates at 3 with 'and N more'",
			data: PRBodyData{
				TasksCompleted: []TaskSummary{
					{ID: "T-001"},
					{ID: "T-002"},
					{ID: "T-003"},
					{ID: "T-004"},
				},
			},
			want: "Tasks T-001, T-002, T-003 and 1 more",
		},
		{
			name: "phase branch with tasks and PhaseName set",
			data: PRBodyData{
				BranchName: "phase/2-core-impl",
				PhaseName:  "Core Implementation",
				TasksCompleted: []TaskSummary{
					{ID: "T-011"},
					{ID: "T-020"},
				},
			},
			want: "Phase 2: Core Implementation (T-011 - T-020)",
		},
		{
			name: "phase branch without PhaseName",
			data: PRBodyData{
				BranchName: "phase/3-review-pipeline",
				TasksCompleted: []TaskSummary{
					{ID: "T-035"},
					{ID: "T-039"},
				},
			},
			want: "Phase 3: T-035 - T-039",
		},
		{
			name: "PhaseName set but no branch phase number",
			data: PRBodyData{
				PhaseName: "Review Pipeline",
				TasksCompleted: []TaskSummary{
					{ID: "T-035"},
					{ID: "T-039"},
				},
			},
			want: "Review Pipeline (T-035 - T-039)",
		},
		{
			name: "no tasks",
			data: PRBodyData{},
			want: "Implementation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pg.GenerateTitle(tt.data)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- GenerateSummary tests --------------------------------------------------

func TestPRBodyGenerator_GenerateSummary_withAgent(t *testing.T) {
	mock := agent.NewMockAgent("claude").WithRunFunc(func(_ context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{
			Stdout:   "This PR adds structured logging via charmbracelet/log.",
			ExitCode: 0,
		}, nil
	})

	pg := NewPRBodyGenerator(mock, "", nil)
	tasks := []TaskSummary{{ID: "T-001", Title: "Add logging"}}

	summary, err := pg.GenerateSummary(context.Background(), "", tasks)
	require.NoError(t, err)
	assert.Equal(t, "This PR adds structured logging via charmbracelet/log.", summary)
	// Agent must have been called exactly once.
	assert.Len(t, mock.Calls, 1)
}

func TestPRBodyGenerator_GenerateSummary_agentError_fallback(t *testing.T) {
	mock := agent.NewMockAgent("claude").WithRunFunc(func(_ context.Context, _ agent.RunOpts) (*agent.RunResult, error) {
		return nil, errors.New("rate limited")
	})

	pg := NewPRBodyGenerator(mock, "", nil)
	tasks := []TaskSummary{
		{ID: "T-001", Title: "Logging"},
		{ID: "T-002", Title: "Config"},
	}

	summary, err := pg.GenerateSummary(context.Background(), "", tasks)
	require.NoError(t, err)
	// Should fall back to structured summary.
	assert.Contains(t, summary, "T-001")
	assert.Contains(t, summary, "T-002")
}

func TestPRBodyGenerator_GenerateSummary_nilAgent_fallback(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)
	tasks := []TaskSummary{{ID: "T-007", Title: "CLI flag parsing"}}

	summary, err := pg.GenerateSummary(context.Background(), "", tasks)
	require.NoError(t, err)
	assert.Contains(t, summary, "T-007")
}

func TestPRBodyGenerator_GenerateSummary_noTasks(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	summary, err := pg.GenerateSummary(context.Background(), "", nil)
	require.NoError(t, err)
	assert.NotEmpty(t, summary)
}

// --- Generate tests ---------------------------------------------------------

func TestPRBodyGenerator_Generate_basicBody(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	data := PRBodyData{
		Summary: "Implements the review pipeline.",
		TasksCompleted: []TaskSummary{
			{ID: "T-035", Title: "Orchestrator"},
			{ID: "T-036", Title: "Report"},
		},
		DiffStats:           DiffStats{TotalFiles: 5, TotalLinesAdded: 200, TotalLinesDeleted: 30},
		ReviewVerdict:       VerdictApproved,
		ReviewFindingsCount: 0,
		BranchName:          "phase/3-review-pipeline",
		BaseBranch:          "main",
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, body, "## Summary")
	assert.Contains(t, body, "Implements the review pipeline.")
	assert.Contains(t, body, "## Tasks Completed")
	assert.Contains(t, body, "T-035")
	assert.Contains(t, body, "T-036")
}

func TestPRBodyGenerator_Generate_reviewReportSection(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	data := PRBodyData{
		Summary:             "Some changes.",
		ReviewVerdict:       VerdictChangesNeeded,
		ReviewFindingsCount: 3,
		ReviewReport:        "# Review\nSome findings here.",
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, body, "## Review Results")
	assert.Contains(t, body, "[FAIL]")
	assert.Contains(t, body, "3")
	assert.Contains(t, body, "<details>")
	assert.Contains(t, body, "Some findings here.")
}

func TestPRBodyGenerator_Generate_omitsReviewSectionWhenEmpty(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	data := PRBodyData{
		Summary:       "No review run.",
		ReviewReport:  "", // empty
		ReviewVerdict: VerdictApproved,
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)
	assert.NotContains(t, body, "## Review Results")
}

func TestPRBodyGenerator_Generate_fixCyclesSection(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	data := PRBodyData{
		Summary: "With fixes.",
		FixReport: &FixReport{
			TotalCycles:  2,
			FinalStatus:  VerificationPassed,
			FixesApplied: true,
		},
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, body, "## Fix Cycles")
	assert.Contains(t, body, "2")
	assert.Contains(t, body, "passed")
	assert.Contains(t, body, "true")
}

func TestPRBodyGenerator_Generate_omitsFixSectionWhenNil(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	data := PRBodyData{
		Summary:   "No fix cycles.",
		FixReport: nil,
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)
	assert.NotContains(t, body, "## Fix Cycles")
}

func TestPRBodyGenerator_Generate_verificationSection(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	vr := &VerificationReport{
		Status: VerificationPassed,
		Results: []CommandResult{
			{Command: "go build ./...", Passed: true},
		},
		Passed: 1,
		Failed: 0,
		Total:  1,
	}

	data := PRBodyData{
		Summary:            "With verification.",
		VerificationReport: vr,
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, body, "Verification Results")
	assert.Contains(t, body, "go build ./...")
}

func TestPRBodyGenerator_Generate_omitsVerificationWhenNil(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	data := PRBodyData{
		Summary:            "No verification.",
		VerificationReport: nil,
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)
	assert.NotContains(t, body, "Verification Results")
}

func TestPRBodyGenerator_Generate_truncatesReviewReport(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	// Create a report longer than maxReviewReportBytes (10,000 chars).
	longReport := strings.Repeat("x", maxReviewReportBytes+500)

	data := PRBodyData{
		Summary:      "Long report.",
		ReviewReport: longReport,
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, body, "truncated")
	assert.Contains(t, body, "see full report")
}

func TestPRBodyGenerator_Generate_truncatesBodyToGitHubLimit(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	// Create a summary that pushes the body well over 65,536 bytes.
	hugeSummary := strings.Repeat("A", maxPRBodyBytes)

	data := PRBodyData{
		Summary: hugeSummary,
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(body), maxPRBodyBytes)
	assert.Contains(t, body, "truncated")
}

func TestPRBodyGenerator_Generate_noTasksMessage(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	data := PRBodyData{
		Summary:        "Empty PR.",
		TasksCompleted: nil,
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, body, "No tasks")
}

// --- adjustSummaryHeadings tests --------------------------------------------

func TestAdjustSummaryHeadings(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "h1 becomes h3",
			input: "# Title\nsome text",
			want:  "### Title\nsome text",
		},
		{
			name:  "h2 becomes h4",
			input: "## Section\ncontent",
			want:  "#### Section\ncontent",
		},
		{
			name:  "h4 becomes h6 (capped)",
			input: "#### Deep\ncontent",
			want:  "###### Deep\ncontent",
		},
		{
			name:  "no headings unchanged",
			input: "plain text without headings",
			want:  "plain text without headings",
		},
		{
			name:  "inline hash not affected",
			input: "use #tag or color #fff",
			want:  "use #tag or color #fff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustSummaryHeadings(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- extractPhaseNumber tests -----------------------------------------------

func TestExtractPhaseNumber(t *testing.T) {
	tests := []struct {
		branch string
		want   string
	}{
		{"phase/3-review-pipeline", "3"},
		{"phase/12-something", "12"},
		{"phase-2-core", "2"},
		{"phase_5", "5"},
		{"main", ""},
		{"feature/my-feature", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			got := extractPhaseNumber(tt.branch)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Generate: all sections populated simultaneously -----------------------

// TestPRBodyGenerator_Generate_allSectionsPopulated verifies acceptance
// criterion: "Generates markdown PR body with all required sections" when
// every optional section is present.
func TestPRBodyGenerator_Generate_allSectionsPopulated(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	vr := &VerificationReport{
		Status: VerificationPassed,
		Results: []CommandResult{
			{Command: "go build ./...", Passed: true},
			{Command: "go test ./...", Passed: true},
		},
		Passed: 2,
		Failed: 0,
		Total:  2,
	}

	data := PRBodyData{
		Summary: "Implements the full review pipeline including orchestrator, report, fix engine, and PR body generation.",
		TasksCompleted: []TaskSummary{
			{ID: "T-035", Title: "Multi-Agent Parallel Review"},
			{ID: "T-036", Title: "Review Report Generation"},
			{ID: "T-037", Title: "Verification Command Runner"},
			{ID: "T-038", Title: "Review Fix Engine"},
			{ID: "T-039", Title: "PR Body Generation"},
		},
		DiffStats:           DiffStats{TotalFiles: 12, TotalLinesAdded: 950, TotalLinesDeleted: 120},
		ReviewVerdict:       VerdictApproved,
		ReviewFindingsCount: 2,
		ReviewReport:        "# Review\n\nAll changes look good. Minor style suggestion.",
		FixReport: &FixReport{
			TotalCycles:  1,
			FinalStatus:  VerificationPassed,
			FixesApplied: true,
		},
		VerificationReport: vr,
		BranchName:         "phase/3-review-pipeline",
		BaseBranch:         "main",
		PhaseName:          "Review Pipeline",
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)

	// Summary section.
	assert.Contains(t, body, "## Summary")
	assert.Contains(t, body, "full review pipeline")

	// Tasks section.
	assert.Contains(t, body, "## Tasks Completed")
	assert.Contains(t, body, "T-035")
	assert.Contains(t, body, "T-039")
	assert.Contains(t, body, "Multi-Agent Parallel Review")
	assert.Contains(t, body, "PR Body Generation")

	// Review section.
	assert.Contains(t, body, "## Review Results")
	assert.Contains(t, body, "[PASS]")
	assert.Contains(t, body, "APPROVED")

	// Fix cycles section.
	assert.Contains(t, body, "## Fix Cycles")
	assert.Contains(t, body, "passed")

	// Verification section.
	assert.Contains(t, body, "Verification Results")
	assert.Contains(t, body, "go build ./...")
	assert.Contains(t, body, "go test ./...")

	// Body must be valid markdown (no raw template delimiters leaked).
	assert.NotContains(t, body, "[[")
	assert.NotContains(t, body, "]]")
}

// --- Generate: review skipped (no review report) ---

// TestPRBodyGenerator_Generate_noReviewReport verifies that when the review
// pipeline was not run (ReviewReport is empty) the Review Results section is
// entirely omitted from the PR body.
func TestPRBodyGenerator_Generate_noReviewReport(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	data := PRBodyData{
		Summary: "Quick hotfix, no formal review.",
		TasksCompleted: []TaskSummary{
			{ID: "T-042", Title: "Hotfix"},
		},
		ReviewReport:  "", // review skipped
		ReviewVerdict: "", // zero value
		FixReport:     nil,
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)

	assert.Contains(t, body, "## Summary")
	assert.Contains(t, body, "## Tasks Completed")
	assert.NotContains(t, body, "## Review Results")
	assert.NotContains(t, body, "## Fix Cycles")
	assert.NotContains(t, body, "Verification Results")
}

// --- Generate: verification failures shown with output excerpts ------------

// TestPRBodyGenerator_Generate_verificationFailures verifies acceptance
// criterion: "Verification section shows pass/fail for each verification
// command" with failed commands showing output.
func TestPRBodyGenerator_Generate_verificationFailures(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	vr := &VerificationReport{
		Status: VerificationFailed,
		Results: []CommandResult{
			{Command: "go build ./...", Passed: true, ExitCode: 0},
			{
				Command:  "go test ./...",
				Passed:   false,
				ExitCode: 1,
				Stderr:   "FAIL\tgithub.com/example/raven [build failed]\nFAIL\t(exit status 1)",
			},
		},
		Passed: 1,
		Failed: 1,
		Total:  2,
	}

	data := PRBodyData{
		Summary:            "Feature with test failure.",
		VerificationReport: vr,
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)

	assert.Contains(t, body, "Verification Results")
	assert.Contains(t, body, "go build ./...")
	assert.Contains(t, body, "go test ./...")
	// Failed command output must appear (in collapsible details block).
	assert.Contains(t, body, "go test ./...")
	assert.Contains(t, body, "❌ Failed")
	assert.Contains(t, body, "✅ Passed")
	// Output excerpt from the failed command.
	assert.Contains(t, body, "build failed")
	// Overall summary line.
	assert.Contains(t, body, "1/2 passed")
}

// --- Generate: empty PRBodyData --------------------------------------------

// TestPRBodyGenerator_Generate_emptyData verifies that Generate does not
// return an error when given a zero-value PRBodyData.
func TestPRBodyGenerator_Generate_emptyData(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	body, err := pg.Generate(context.Background(), PRBodyData{})
	require.NoError(t, err)
	assert.NotEmpty(t, body)

	// Template delimiters must never appear in the output.
	assert.NotContains(t, body, "[[")
	assert.NotContains(t, body, "]]")

	// The "No tasks" fallback should be present.
	assert.Contains(t, body, "No tasks")
}

// --- Generate: fix section with failed final status -----------------------

// TestPRBodyGenerator_Generate_fixCyclesFailed verifies that when the fix
// engine's final status is VerificationFailed the label "failed" appears in
// the Fix Cycles section.
func TestPRBodyGenerator_Generate_fixCyclesFailed(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	data := PRBodyData{
		Summary: "Fix attempts exhausted.",
		FixReport: &FixReport{
			TotalCycles:  3,
			FinalStatus:  VerificationFailed,
			FixesApplied: true,
		},
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, body, "## Fix Cycles")
	assert.Contains(t, body, "3")
	assert.Contains(t, body, "failed")
}

// --- Generate: BLOCKING verdict --------------------------------------------

// TestPRBodyGenerator_Generate_blockingVerdict verifies the [BLOCK] indicator
// appears when the review verdict is VerdictBlocking.
func TestPRBodyGenerator_Generate_blockingVerdict(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	data := PRBodyData{
		Summary:             "Critical security issue found.",
		ReviewVerdict:       VerdictBlocking,
		ReviewFindingsCount: 5,
		ReviewReport:        "# Security Review\n\nCritical SQL injection vulnerability detected.",
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, body, "[BLOCK]")
	assert.Contains(t, body, "BLOCKING")
}

// --- GenerateSummary: agent returns non-zero exit code --------------------

// TestPRBodyGenerator_GenerateSummary_agentNonZeroExit verifies that when
// the agent returns a non-zero exit code (but no error), GenerateSummary falls
// back to a structured summary rather than returning the empty/bad output.
func TestPRBodyGenerator_GenerateSummary_agentNonZeroExit(t *testing.T) {
	mock := agent.NewMockAgent("claude").WithRunFunc(func(_ context.Context, _ agent.RunOpts) (*agent.RunResult, error) {
		// Simulate agent failing with non-zero exit.
		return &agent.RunResult{Stdout: "", ExitCode: 1}, nil
	})

	pg := NewPRBodyGenerator(mock, "", nil)
	tasks := []TaskSummary{
		{ID: "T-010", Title: "Some feature"},
		{ID: "T-011", Title: "Another feature"},
	}

	summary, err := pg.GenerateSummary(context.Background(), "", tasks)
	require.NoError(t, err)
	// Must fall back to structured summary.
	assert.Contains(t, summary, "T-010")
	assert.Contains(t, summary, "T-011")
}

// TestPRBodyGenerator_GenerateSummary_agentEmptyStdout verifies that an agent
// returning an empty stdout string (exit 0) triggers the fallback summary.
func TestPRBodyGenerator_GenerateSummary_agentEmptyStdout(t *testing.T) {
	mock := agent.NewMockAgent("claude").WithRunFunc(func(_ context.Context, _ agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: "   ", ExitCode: 0}, nil // whitespace-only
	})

	pg := NewPRBodyGenerator(mock, "", nil)
	tasks := []TaskSummary{{ID: "T-007", Title: "CLI flags"}}

	summary, err := pg.GenerateSummary(context.Background(), "", tasks)
	require.NoError(t, err)
	assert.Contains(t, summary, "T-007")
}

// --- GenerateSummary: single task structured fallback ----------------------

// TestPRBodyGenerator_GenerateSummary_singleTaskFallback verifies the exact
// shape of the structured fallback for a single task with and without a title.
func TestPRBodyGenerator_GenerateSummary_singleTaskFallback(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	t.Run("with title", func(t *testing.T) {
		summary, err := pg.GenerateSummary(context.Background(), "", []TaskSummary{
			{ID: "T-007", Title: "CLI flag parsing"},
		})
		require.NoError(t, err)
		assert.Contains(t, summary, "T-007")
		assert.Contains(t, summary, "CLI flag parsing")
	})

	t.Run("without title", func(t *testing.T) {
		summary, err := pg.GenerateSummary(context.Background(), "", []TaskSummary{
			{ID: "T-007"},
		})
		require.NoError(t, err)
		assert.Contains(t, summary, "T-007")
	})
}

// --- GenerateSummary: prompt content verification -------------------------

// TestPRBodyGenerator_GenerateSummary_promptContainsTasks verifies that the
// prompt sent to the agent includes each task ID and title.
func TestPRBodyGenerator_GenerateSummary_promptContainsTasks(t *testing.T) {
	var capturedPrompt string
	mock := agent.NewMockAgent("claude").WithRunFunc(func(_ context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		capturedPrompt = opts.Prompt
		return &agent.RunResult{Stdout: "AI summary", ExitCode: 0}, nil
	})

	pg := NewPRBodyGenerator(mock, "", nil)
	tasks := []TaskSummary{
		{ID: "T-035", Title: "Orchestrator"},
		{ID: "T-036", Title: "Report Generation"},
	}

	_, err := pg.GenerateSummary(context.Background(), "some diff", tasks)
	require.NoError(t, err)

	assert.Contains(t, capturedPrompt, "T-035")
	assert.Contains(t, capturedPrompt, "Orchestrator")
	assert.Contains(t, capturedPrompt, "T-036")
	assert.Contains(t, capturedPrompt, "Report Generation")
}

// --- GenerateTitle: explicit spec scenarios --------------------------------

// TestPRBodyGenerator_GenerateTitle_phaseWithTasks verifies the "Phase N:
// T-011 - T-020" title format specified in T-039.
func TestPRBodyGenerator_GenerateTitle_phaseWithTasks(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	data := PRBodyData{
		BranchName: "phase/2-core-implementation",
		TasksCompleted: []TaskSummary{
			{ID: "T-011"},
			{ID: "T-020"},
		},
	}

	got := pg.GenerateTitle(data)
	assert.Equal(t, "Phase 2: T-011 - T-020", got)
}

// TestPRBodyGenerator_GenerateTitle_withoutPhase verifies the "Tasks T-007,
// T-008, T-009" format specified in T-039 when no phase info is present.
func TestPRBodyGenerator_GenerateTitle_withoutPhase(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	data := PRBodyData{
		TasksCompleted: []TaskSummary{
			{ID: "T-007"},
			{ID: "T-008"},
			{ID: "T-009"},
		},
	}

	got := pg.GenerateTitle(data)
	assert.Equal(t, "Tasks T-007, T-008, T-009", got)
}

// TestPRBodyGenerator_GenerateTitle_fiveTasksAndMore verifies "and N more"
// truncation beyond 3 tasks without phase context.
func TestPRBodyGenerator_GenerateTitle_fiveTasksAndMore(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	data := PRBodyData{
		TasksCompleted: []TaskSummary{
			{ID: "T-001"}, {ID: "T-002"}, {ID: "T-003"},
			{ID: "T-004"}, {ID: "T-005"},
		},
	}

	got := pg.GenerateTitle(data)
	assert.Equal(t, "Tasks T-001, T-002, T-003 and 2 more", got)
}

// TestPRBodyGenerator_GenerateTitle_phaseWithSingleTask verifies the phase
// title for a phase PR containing only a single task.
func TestPRBodyGenerator_GenerateTitle_phaseWithSingleTask(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	data := PRBodyData{
		BranchName: "phase/1-bootstrap",
		TasksCompleted: []TaskSummary{
			{ID: "T-001", Title: "Bootstrap project"},
		},
	}

	got := pg.GenerateTitle(data)
	// Single task: falls through to the single-task path before the phase check.
	assert.Equal(t, "T-001: Bootstrap project", got)
}

// --- PR template integration -----------------------------------------------

// TestPRBodyGenerator_Generate_withPRTemplate verifies acceptance criterion:
// "Integrates with .github/PULL_REQUEST_TEMPLATE.md when present". The
// generator must NOT error and must produce a body even when a PR template
// file is present.
func TestPRBodyGenerator_Generate_withPRTemplate(t *testing.T) {
	dir := t.TempDir()
	templateDir := filepath.Join(dir, ".github")
	require.NoError(t, os.MkdirAll(templateDir, 0755))

	templatePath := filepath.Join(templateDir, "PULL_REQUEST_TEMPLATE.md")
	templateContent := "## Summary\n\n## Checklist\n- [ ] Tests pass\n- [ ] Lint passes\n"
	require.NoError(t, os.WriteFile(templatePath, []byte(templateContent), 0644))

	pg := NewPRBodyGenerator(nil, templatePath, nil)

	data := PRBodyData{
		Summary: "With PR template.",
		TasksCompleted: []TaskSummary{
			{ID: "T-039", Title: "PR Body Generation"},
		},
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)
	assert.NotEmpty(t, body)

	// Generated body must contain required sections from the default template.
	assert.Contains(t, body, "## Summary")
	assert.Contains(t, body, "T-039")
	assert.NotContains(t, body, "[[")
	assert.NotContains(t, body, "]]")
}

// TestPRBodyGenerator_Generate_prTemplateNotFound verifies that when the
// configured PR template path does not exist, Generate falls back to the
// default structure without error.
func TestPRBodyGenerator_Generate_prTemplateNotFound(t *testing.T) {
	nonExistent := filepath.Join(t.TempDir(), ".github", "PULL_REQUEST_TEMPLATE.md")
	pg := NewPRBodyGenerator(nil, nonExistent, nil)

	data := PRBodyData{
		Summary: "No PR template found.",
		TasksCompleted: []TaskSummary{
			{ID: "T-001", Title: "Bootstrap"},
		},
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)
	assert.Contains(t, body, "## Summary")
	assert.Contains(t, body, "T-001")
}

// --- hasPRTemplate ---------------------------------------------------------

func TestPRBodyGenerator_hasPRTemplate(t *testing.T) {
	t.Run("empty path returns false", func(t *testing.T) {
		pg := NewPRBodyGenerator(nil, "", nil)
		assert.False(t, pg.hasPRTemplate())
	})

	t.Run("nonexistent path returns false", func(t *testing.T) {
		pg := NewPRBodyGenerator(nil, "/nonexistent/path/PULL_REQUEST_TEMPLATE.md", nil)
		assert.False(t, pg.hasPRTemplate())
	})

	t.Run("existing file returns true", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "PULL_REQUEST_TEMPLATE.md")
		require.NoError(t, os.WriteFile(p, []byte("# PR Template\n"), 0644))
		pg := NewPRBodyGenerator(nil, p, nil)
		assert.True(t, pg.hasPRTemplate())
	})
}

// --- verdictIndicator (via Generate) ---------------------------------------

// TestVerdictIndicator_allValues verifies that verdictIndicator returns the
// correct string for all known verdict values and an unknown fallback.
func TestVerdictIndicator_allValues(t *testing.T) {
	tests := []struct {
		verdict Verdict
		want    string
	}{
		{VerdictApproved, "[PASS]"},
		{VerdictChangesNeeded, "[FAIL]"},
		{VerdictBlocking, "[BLOCK]"},
		{Verdict("UNKNOWN_VERDICT"), "[UNKNOWN]"},
		{Verdict(""), "[UNKNOWN]"},
	}

	for _, tt := range tests {
		t.Run(string(tt.verdict), func(t *testing.T) {
			got := verdictIndicator(tt.verdict)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- exitCodeOf ------------------------------------------------------------

// TestExitCodeOf verifies the nil-safe helper.
func TestExitCodeOf(t *testing.T) {
	assert.Equal(t, -1, exitCodeOf(nil))
	assert.Equal(t, 0, exitCodeOf(&agent.RunResult{ExitCode: 0}))
	assert.Equal(t, 42, exitCodeOf(&agent.RunResult{ExitCode: 42}))
}

// --- buildTemplateData field derivation ------------------------------------

// TestPRBodyGenerator_buildTemplateData_truncatesReviewReport verifies that
// the TruncatedReviewReport field is set correctly and the truncation flag is
// raised.
func TestPRBodyGenerator_buildTemplateData_truncatesReviewReport(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	longReport := strings.Repeat("A", maxReviewReportBytes+1)
	data := PRBodyData{ReviewReport: longReport}

	td := pg.buildTemplateData(data)
	assert.True(t, td.ReviewReportTruncated)
	assert.LessOrEqual(t, len(td.TruncatedReviewReport), maxReviewReportBytes)
	assert.Contains(t, td.TruncatedReviewReport, "see full report")
}

// TestPRBodyGenerator_buildTemplateData_shortReviewReport verifies that a
// report within the limit is not truncated.
func TestPRBodyGenerator_buildTemplateData_shortReviewReport(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	shortReport := "# Review\n\nLooks good."
	data := PRBodyData{ReviewReport: shortReport}

	td := pg.buildTemplateData(data)
	assert.False(t, td.ReviewReportTruncated)
	assert.Equal(t, shortReport, td.TruncatedReviewReport)
}

// TestPRBodyGenerator_buildTemplateData_adjustsHeadings verifies that h1
// headings in the Summary field are demoted to avoid conflicts with the PR
// body's own section headings.
func TestPRBodyGenerator_buildTemplateData_adjustsHeadings(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	data := PRBodyData{Summary: "# AI-written summary\n\nSome prose."}
	td := pg.buildTemplateData(data)

	// Original h1 heading must be demoted — should not start with a single "#" followed by space.
	assert.NotContains(t, td.PRBodyData.Summary, "\n# AI-written")
	assert.False(t, strings.HasPrefix(td.PRBodyData.Summary, "# AI-written"), "h1 heading should be demoted")
	assert.Contains(t, td.PRBodyData.Summary, "### AI-written")
}

// TestPRBodyGenerator_buildTemplateData_fixFinalStatusLabel verifies the
// human-readable fix status label for both outcomes.
func TestPRBodyGenerator_buildTemplateData_fixFinalStatusLabel(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	t.Run("passed", func(t *testing.T) {
		data := PRBodyData{FixReport: &FixReport{FinalStatus: VerificationPassed}}
		td := pg.buildTemplateData(data)
		assert.Equal(t, "passed", td.FixFinalStatusLabel)
	})

	t.Run("failed", func(t *testing.T) {
		data := PRBodyData{FixReport: &FixReport{FinalStatus: VerificationFailed}}
		td := pg.buildTemplateData(data)
		assert.Equal(t, "failed", td.FixFinalStatusLabel)
	})
}

// --- adjustSummaryHeadings: h5 and h6 cap --------------------------------

// TestAdjustSummaryHeadings_capAtH6 verifies that headings at level 5 and 6
// are capped at h6 (not demoted beyond 6 hashes).
func TestAdjustSummaryHeadings_capAtH6(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "h5 becomes h6 (capped)",
			input: "##### Five\ncontent",
			want:  "###### Five\ncontent",
		},
		{
			name:  "h6 stays h6 (already capped)",
			input: "###### Six\ncontent",
			want:  "###### Six\ncontent",
		},
		{
			name:  "h3 becomes h5",
			input: "### Three\ncontent",
			want:  "##### Three\ncontent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustSummaryHeadings(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Generate: body size boundary ------------------------------------------

// TestPRBodyGenerator_Generate_bodyExactlyAtLimit verifies that a body that
// exactly fits within maxPRBodyBytes is not truncated or modified.
func TestPRBodyGenerator_Generate_bodyExactlyAtLimit(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	// A minimal body should always be well below the limit.
	data := PRBodyData{Summary: "Small change."}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(body), maxPRBodyBytes)
	assert.NotContains(t, body, "truncated to fit GitHub")
}

// --- Generate: PR body does not exceed GitHub limit -----------------------

// TestPRBodyGenerator_Generate_veryLargeAllSections verifies the combined
// truncation path when review report, summary, and tasks together push the
// body beyond the GitHub limit.
func TestPRBodyGenerator_Generate_veryLargeAllSections(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)

	// Build a large review report that triggers review truncation.
	hugeReport := strings.Repeat("R", maxReviewReportBytes+1000)

	// Build tasks to add bulk.
	tasks := make([]TaskSummary, 50)
	for i := range tasks {
		tasks[i] = TaskSummary{
			ID:    fmt.Sprintf("T-%03d", i+1),
			Title: strings.Repeat("Task title content ", 10),
		}
	}

	data := PRBodyData{
		Summary:             strings.Repeat("S", 30000),
		TasksCompleted:      tasks,
		ReviewReport:        hugeReport,
		ReviewFindingsCount: 99,
		ReviewVerdict:       VerdictChangesNeeded,
	}

	body, err := pg.Generate(context.Background(), data)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(body), maxPRBodyBytes,
		"body must not exceed GitHub's 65,536 byte limit")
}

// --- buildMultiTaskTitle edge cases ----------------------------------------

// TestPRBodyGenerator_buildMultiTaskTitle_emptySlice verifies the fallback
// "Implementation" title for an empty tasks slice.
func TestPRBodyGenerator_buildMultiTaskTitle_emptySlice(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)
	got := pg.buildMultiTaskTitle(nil)
	assert.Equal(t, "Implementation", got)
}

// TestPRBodyGenerator_buildPhaseTitle_noTasksNoPhase verifies the fallback
// "Phase Implementation" title when neither tasks nor phase number are
// available.
func TestPRBodyGenerator_buildPhaseTitle_noTasksNoPhase(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)
	data := PRBodyData{PhaseName: "My Phase"} // no tasks, no branch
	got := pg.buildPhaseTitle(data)
	assert.Equal(t, "My Phase", got)
}

// TestPRBodyGenerator_buildPhaseTitle_emptyEverything verifies the ultimate
// "Phase Implementation" fallback.
func TestPRBodyGenerator_buildPhaseTitle_emptyEverything(t *testing.T) {
	pg := NewPRBodyGenerator(nil, "", nil)
	data := PRBodyData{} // no tasks, no branch, no phase name
	got := pg.buildPhaseTitle(data)
	assert.Equal(t, "Phase Implementation", got)
}
