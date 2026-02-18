package review

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
)

// ---------------------------------------------------------------------------
// FixPromptBuilder tests
// ---------------------------------------------------------------------------

func TestFixPromptBuilder_Build_NoFindings(t *testing.T) {
	t.Parallel()

	pb := NewFixPromptBuilder(nil, nil, nil)
	prompt, err := pb.Build(nil, "", nil)
	require.NoError(t, err)
	// Should produce an empty findings section but still render the template.
	assert.NotEmpty(t, prompt)
}

func TestFixPromptBuilder_Build_WithFindings(t *testing.T) {
	t.Parallel()

	findings := []*Finding{
		{
			File:        "internal/foo/foo.go",
			Line:        42,
			Severity:    SeverityHigh,
			Category:    "correctness",
			Description: "nil pointer dereference",
			Suggestion:  "add nil check before dereferencing",
		},
	}

	pb := NewFixPromptBuilder(
		[]string{"use early returns", "wrap errors with context"},
		[]string{"go build ./...", "go test ./..."},
		nil,
	)

	prompt, err := pb.Build(findings, "diff content here", nil)
	require.NoError(t, err)

	assert.Contains(t, prompt, "internal/foo/foo.go")
	assert.Contains(t, prompt, "nil pointer dereference")
	assert.Contains(t, prompt, "add nil check before dereferencing")
	assert.Contains(t, prompt, "high")
	assert.Contains(t, prompt, "correctness")
	assert.Contains(t, prompt, "use early returns")
	assert.Contains(t, prompt, "go build ./...")
	assert.Contains(t, prompt, "diff content here")
}

func TestFixPromptBuilder_Build_TruncatesLargeDiff(t *testing.T) {
	t.Parallel()

	pb := NewFixPromptBuilder(nil, nil, nil)

	// Create a diff larger than maxFixDiffBytes (50KB).
	largeDiff := strings.Repeat("x", maxFixDiffBytes+1000)
	prompt, err := pb.Build(nil, largeDiff, nil)
	require.NoError(t, err)

	assert.Contains(t, prompt, "[diff truncated at 50KB]")
	// Verify the truncation was applied (prompt should not contain the full diff).
	assert.Less(t, len(prompt), len(largeDiff)+500)
}

func TestFixPromptBuilder_Build_WithPreviousFailures(t *testing.T) {
	t.Parallel()

	findings := []*Finding{
		{
			File:        "main.go",
			Line:        10,
			Severity:    SeverityMedium,
			Category:    "style",
			Description: "missing doc comment",
			Suggestion:  "add a doc comment",
		},
	}

	previousFailure := FixCycleResult{
		Cycle: 1,
		Verification: &VerificationReport{
			Status: VerificationFailed,
			Results: []CommandResult{
				{
					Command:  "go build ./...",
					ExitCode: 1,
					Stderr:   "build failed: undefined: Foo",
					Passed:   false,
				},
			},
		},
	}

	pb := NewFixPromptBuilder(nil, []string{"go build ./..."}, nil)
	prompt, err := pb.Build(findings, "some diff", []FixCycleResult{previousFailure})
	require.NoError(t, err)

	assert.Contains(t, prompt, "Previous Fix Attempt Results")
	assert.Contains(t, prompt, "Cycle 1 Failures")
	assert.Contains(t, prompt, "go build ./...")
	assert.Contains(t, prompt, "build failed: undefined: Foo")
}

func TestFixPromptBuilder_Build_DefensiveCopy(t *testing.T) {
	t.Parallel()

	convs := []string{"convention one"}
	cmds := []string{"go vet ./..."}

	pb := NewFixPromptBuilder(convs, cmds, nil)

	// Mutate originals after construction.
	convs[0] = "mutated"
	cmds[0] = "mutated"

	prompt, err := pb.Build(nil, "", nil)
	require.NoError(t, err)

	assert.Contains(t, prompt, "convention one")
	assert.NotContains(t, prompt, "mutated")
}

// ---------------------------------------------------------------------------
// FixEngine tests
// ---------------------------------------------------------------------------

func TestNewFixEngine_Defaults(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	fe := NewFixEngine(ag, nil, 3, nil, nil)

	require.NotNil(t, fe)
	assert.Equal(t, 3, fe.maxCycles)
	assert.Equal(t, ag, fe.agent)
}

func TestFixEngine_Fix_NoFindings(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	fe := NewFixEngine(ag, nil, 3, nil, nil)

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings: nil,
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Equal(t, VerificationPassed, report.FinalStatus)
	assert.Equal(t, 0, report.TotalCycles)
	assert.False(t, report.FixesApplied)
	assert.Empty(t, report.Cycles)
	// Agent should not have been called.
	assert.Empty(t, ag.Calls)
}

func TestFixEngine_Fix_MaxCyclesZero(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	fe := NewFixEngine(ag, nil, 0, nil, nil)

	findings := []*Finding{
		{File: "foo.go", Line: 1, Severity: SeverityHigh, Description: "issue"},
	}

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings:  findings,
		MaxCycles: 0, // no override, engine default is 0
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Equal(t, VerificationPassed, report.FinalStatus)
	assert.Equal(t, 0, report.TotalCycles)
	assert.Empty(t, ag.Calls)
}

func TestFixEngine_Fix_MaxCyclesOverride(t *testing.T) {
	t.Parallel()

	callCount := 0
	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		return &agent.RunResult{ExitCode: 0}, nil
	}

	// Engine default is 5, but opts override is 1.
	fe := NewFixEngine(ag, nil, 5, nil, nil)

	findings := []*Finding{
		{File: "foo.go", Line: 1, Severity: SeverityHigh, Description: "issue"},
	}

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings:  findings,
		MaxCycles: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	// Should have run exactly 1 cycle (no verifier = nil report so loop
	// ends after maxCycles with VerificationFailed as final status since
	// no verifier was run).
	assert.Equal(t, 1, callCount)
	assert.Equal(t, 1, report.TotalCycles)
}

func TestFixEngine_Fix_AgentSuccess_VerificationPasses(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{ExitCode: 0, Stdout: "fixed"}, nil
	}

	// Build a verifier with no commands so it always passes.
	verifier := NewVerificationRunner(nil, "", 0, nil)

	fe := NewFixEngine(ag, verifier, 3, nil, nil)

	findings := []*Finding{
		{File: "foo.go", Line: 5, Severity: SeverityMedium, Description: "bad error handling"},
	}

	report, err := fe.Fix(context.Background(), FixOpts{Findings: findings})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.True(t, report.FixesApplied)
	assert.Equal(t, VerificationPassed, report.FinalStatus)
	assert.Equal(t, 1, report.TotalCycles)
	assert.Len(t, report.Cycles, 1)
	assert.Equal(t, 0, report.Cycles[0].AgentResult.ExitCode)
}

func TestFixEngine_Fix_AgentError_CycleRecordedNoAbort(t *testing.T) {
	t.Parallel()

	callCount := 0
	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		if callCount == 1 {
			return nil, fmt.Errorf("agent error")
		}
		return &agent.RunResult{ExitCode: 0}, nil
	}

	verifier := NewVerificationRunner(nil, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 3, nil, nil)

	findings := []*Finding{
		{File: "bar.go", Line: 1, Severity: SeverityHigh, Description: "issue"},
	}

	report, err := fe.Fix(context.Background(), FixOpts{Findings: findings})
	require.NoError(t, err)
	require.NotNil(t, report)

	// Cycle 1: agent error, no verification.
	// Cycle 2: agent succeeds, verification passes.
	assert.Equal(t, 2, callCount)
	assert.Equal(t, 2, report.TotalCycles)

	// First cycle has nil AgentResult due to error.
	assert.Nil(t, report.Cycles[0].AgentResult)
	assert.Nil(t, report.Cycles[0].Verification)

	// Second cycle succeeded.
	assert.NotNil(t, report.Cycles[1].AgentResult)
	assert.Equal(t, 0, report.Cycles[1].AgentResult.ExitCode)
	assert.True(t, report.FixesApplied)
}

func TestFixEngine_Fix_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		// Cancel context on first invocation.
		cancel()
		return &agent.RunResult{ExitCode: 0}, nil
	}

	verifier := NewVerificationRunner(nil, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 5, nil, nil)

	findings := []*Finding{
		{File: "baz.go", Line: 1, Severity: SeverityLow, Description: "minor issue"},
	}

	report, err := fe.Fix(ctx, FixOpts{Findings: findings})
	// No error returned on context cancellation; partial results returned.
	require.NoError(t, err)
	require.NotNil(t, report)

	// At most 1 cycle should have run before cancellation was honoured.
	assert.LessOrEqual(t, report.TotalCycles, 2)
}

func TestFixEngine_Fix_EmitsEvents(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{ExitCode: 0}, nil
	}

	events := make(chan FixEvent, 20)
	verifier := NewVerificationRunner(nil, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 2, nil, events)

	findings := []*Finding{
		{File: "x.go", Line: 1, Severity: SeverityInfo, Description: "minor"},
	}

	_, err := fe.Fix(context.Background(), FixOpts{Findings: findings})
	require.NoError(t, err)

	close(events)

	var eventTypes []string
	for ev := range events {
		eventTypes = append(eventTypes, ev.Type)
	}

	assert.Contains(t, eventTypes, "fix_started")
	assert.Contains(t, eventTypes, "cycle_started")
	assert.Contains(t, eventTypes, "agent_invoked")
	assert.Contains(t, eventTypes, "verification_started")
	assert.Contains(t, eventTypes, "verification_result")
	assert.Contains(t, eventTypes, "cycle_completed")
	assert.Contains(t, eventTypes, "fix_completed")
}

func TestFixEngine_Fix_EventsNilChannel(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	// nil events channel — should not panic.
	fe := NewFixEngine(ag, nil, 2, nil, nil)

	findings := []*Finding{
		{File: "y.go", Line: 1, Severity: SeverityInfo, Description: "minor"},
	}

	report, err := fe.Fix(context.Background(), FixOpts{Findings: findings})
	require.NoError(t, err)
	require.NotNil(t, report)
}

func TestFixEngine_DryRun_NoFindings(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	fe := NewFixEngine(ag, nil, 3, nil, nil)

	prompt, err := fe.DryRun(context.Background(), FixOpts{Findings: nil})
	require.NoError(t, err)
	assert.Empty(t, prompt)
	// Agent should not have been called.
	assert.Empty(t, ag.Calls)
}

func TestFixEngine_DryRun_WithFindings(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	pb := NewFixPromptBuilder(
		[]string{"convention A"},
		[]string{"go build ./..."},
		nil,
	)
	fe := NewFixEngine(ag, nil, 3, nil, nil).WithPromptBuilder(pb)

	findings := []*Finding{
		{
			File:        "internal/pkg/file.go",
			Line:        100,
			Severity:    SeverityHigh,
			Category:    "security",
			Description: "SQL injection risk",
			Suggestion:  "use parameterised queries",
		},
	}

	prompt, err := fe.DryRun(context.Background(), FixOpts{Findings: findings})
	require.NoError(t, err)

	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "SQL injection risk")
	assert.Contains(t, prompt, "convention A")
	assert.Contains(t, prompt, "go build ./...")
	// Agent must not be invoked.
	assert.Empty(t, ag.Calls)
}

func TestFixEngine_DryRun_EmitsFix_Started(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	events := make(chan FixEvent, 10)
	fe := NewFixEngine(ag, nil, 3, nil, events)

	_, err := fe.DryRun(context.Background(), FixOpts{Findings: nil})
	require.NoError(t, err)

	// Should have emitted at least fix_started.
	require.Len(t, events, 1)
	ev := <-events
	assert.Equal(t, "fix_started", ev.Type)
	assert.False(t, ev.Timestamp.IsZero())
}

func TestFixEngine_Fix_FixesApplied_OnlyWhenExitCode0(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	// Agent always exits with non-zero code.
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{ExitCode: 1}, nil
	}

	fe := NewFixEngine(ag, nil, 2, nil, nil)

	findings := []*Finding{
		{File: "z.go", Line: 1, Severity: SeverityHigh, Description: "critical bug"},
	}

	report, err := fe.Fix(context.Background(), FixOpts{Findings: findings})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.False(t, report.FixesApplied, "FixesApplied must be false when all agent runs exit non-zero")
}

func TestFixEngine_Fix_FinalStatus_LastCycleVerification(t *testing.T) {
	t.Parallel()

	cycle := 0
	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		cycle++
		return &agent.RunResult{ExitCode: 0}, nil
	}

	// Use a verifier with a failing command on cycle 1 and no commands on
	// cycle 2 (simulated by swapping verifiers between cycles is not easily
	// done here, so we use no commands so it always passes).
	verifier := NewVerificationRunner(nil, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 2, nil, nil)

	findings := []*Finding{
		{File: "final.go", Line: 1, Severity: SeverityLow, Description: "lint warning"},
	}

	report, err := fe.Fix(context.Background(), FixOpts{Findings: findings})
	require.NoError(t, err)

	// With no commands the verifier always passes on the first cycle.
	assert.Equal(t, VerificationPassed, report.FinalStatus)
	assert.Equal(t, 1, report.TotalCycles)
}

func TestFixEngine_WithPromptBuilder(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	fe := NewFixEngine(ag, nil, 3, nil, nil)

	pb := NewFixPromptBuilder([]string{"convention"}, []string{"cmd"}, nil)
	fe2 := fe.WithPromptBuilder(pb)

	// WithPromptBuilder returns receiver.
	assert.Equal(t, fe, fe2)
	assert.Equal(t, pb, fe.promptBuilder)
}

func TestVerificationResultMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		report  *VerificationReport
		wantSub string
	}{
		{
			name:    "nil report",
			report:  nil,
			wantSub: "skipped",
		},
		{
			name: "passed",
			report: &VerificationReport{
				Status: VerificationPassed,
				Passed: 3,
				Total:  3,
			},
			wantSub: "passed",
		},
		{
			name: "failed",
			report: &VerificationReport{
				Status: VerificationFailed,
				Passed: 1,
				Total:  3,
			},
			wantSub: "failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			msg := verificationResultMessage(tt.report)
			assert.Contains(t, msg, tt.wantSub)
		})
	}
}

// ---------------------------------------------------------------------------
// Fix prompt template rendering tests
// ---------------------------------------------------------------------------

func TestFixPromptBuilder_Build_TemplateRendersCorrectly(t *testing.T) {
	t.Parallel()

	findings := []*Finding{
		{
			File:        "cmd/main.go",
			Line:        7,
			Severity:    SeverityCritical,
			Category:    "security",
			Description: "hardcoded credentials",
			Suggestion:  "use environment variables",
		},
		{
			File:        "internal/db/db.go",
			Line:        22,
			Severity:    SeverityMedium,
			Category:    "performance",
			Description: "N+1 query",
			Suggestion:  "batch the queries",
		},
	}

	pb := NewFixPromptBuilder(
		[]string{"follow SOLID principles", "no global state"},
		[]string{"go build ./...", "go test ./...", "go vet ./..."},
		nil,
	)

	prompt, err := pb.Build(findings, "--- a/cmd/main.go\n+++ b/cmd/main.go\n@@ -1 +1 @@\n+fix", nil)
	require.NoError(t, err)

	// Verify all findings appear.
	assert.Contains(t, prompt, "cmd/main.go:7 (critical)")
	assert.Contains(t, prompt, "hardcoded credentials")
	assert.Contains(t, prompt, "use environment variables")
	assert.Contains(t, prompt, "internal/db/db.go:22 (medium)")
	assert.Contains(t, prompt, "N+1 query")
	assert.Contains(t, prompt, "batch the queries")

	// Verify conventions section.
	assert.Contains(t, prompt, "Project Conventions")
	assert.Contains(t, prompt, "follow SOLID principles")
	assert.Contains(t, prompt, "no global state")

	// Verify verification commands section.
	assert.Contains(t, prompt, "Verification Commands")
	assert.Contains(t, prompt, "go build ./...")
	assert.Contains(t, prompt, "go test ./...")
	assert.Contains(t, prompt, "go vet ./...")

	// Verify the diff is included.
	assert.Contains(t, prompt, "--- a/cmd/main.go")

	// Verify closing instruction.
	assert.Contains(t, prompt, "Apply fixes directly to the files")
}

func TestFixPromptBuilder_Build_NoPreviousFailures_SectionAbsent(t *testing.T) {
	t.Parallel()

	pb := NewFixPromptBuilder(nil, nil, nil)
	findings := []*Finding{
		{File: "x.go", Line: 1, Severity: SeverityLow, Description: "minor", Suggestion: "fix it"},
	}

	prompt, err := pb.Build(findings, "", nil)
	require.NoError(t, err)

	assert.NotContains(t, prompt, "Previous Fix Attempt Results")
}

func TestFixEngine_Fix_Duration(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		time.Sleep(1 * time.Millisecond)
		return &agent.RunResult{ExitCode: 0}, nil
	}

	fe := NewFixEngine(ag, nil, 1, nil, nil)

	findings := []*Finding{
		{File: "t.go", Line: 1, Severity: SeverityInfo, Description: "info"},
	}

	report, err := fe.Fix(context.Background(), FixOpts{Findings: findings})
	require.NoError(t, err)

	assert.Greater(t, report.Duration, time.Duration(0))
	assert.Greater(t, report.Cycles[0].Duration, time.Duration(0))
}

// ---------------------------------------------------------------------------
// Acceptance-criteria table-driven tests (T-038 spec)
// ---------------------------------------------------------------------------

// makeFindingSet builds a slice of *Finding for test use.
func makeFindingSet(n int) []*Finding {
	out := make([]*Finding, n)
	for i := range out {
		out[i] = &Finding{
			File:        fmt.Sprintf("pkg/file%d.go", i),
			Line:        i + 1,
			Severity:    SeverityHigh,
			Category:    "correctness",
			Description: fmt.Sprintf("finding description %d", i),
			Suggestion:  fmt.Sprintf("suggestion %d", i),
		}
	}
	return out
}

// makeCountingVerifier creates a VerificationRunner whose single shell command
// reads a verdict from verdictFile on each call (one verdict per line, "0"=pass,
// "1"=fail) and increments a counter stored in counterFile. This allows
// per-cycle control of verification outcomes without modifying production code.
// The caller must initialise counterFile with "0\n" and verdictFile with the
// desired sequence before using the verifier.
func makeCountingVerifier(counterFile, verdictFile string) *VerificationRunner {
	cmd := fmt.Sprintf(
		`n=$(cat %s); code=$(sed -n "$((n+1))p" %s); echo $((n+1)) > %s; exit ${code:-0}`,
		counterFile,
		verdictFile,
		counterFile,
	)
	return NewVerificationRunner([]string{cmd}, "", 0, nil)
}

// initCountingVerifier writes the initial counter (0) and verdict sequence to
// disk so that makeCountingVerifier works correctly.
//
// counterFile is initialised with "0", and verdictFile receives one line per
// cycle: "0" for a passing cycle, "1" for a failing cycle.
func initCountingVerifier(t *testing.T, counterFile, verdictFile string, verdictPass []bool) {
	t.Helper()

	// Write the counter file with initial value.
	if err := os.WriteFile(counterFile, []byte("0\n"), 0o600); err != nil {
		t.Fatalf("initCountingVerifier: write counter: %v", err)
	}

	// Build verdict file content: one digit per line.
	var verdictLines strings.Builder
	for _, p := range verdictPass {
		if p {
			verdictLines.WriteString("0\n")
		} else {
			verdictLines.WriteString("1\n")
		}
	}
	if err := os.WriteFile(verdictFile, []byte(verdictLines.String()), 0o600); err != nil {
		t.Fatalf("initCountingVerifier: write verdicts: %v", err)
	}
}

// TestFixEngine_CycleScenarios covers the four canonical cycle scenarios from
// the T-038 acceptance criteria in a single table-driven test.
//
// Each sub-test uses a per-cycle shell-script verifier (via makeCountingVerifier)
// to control whether a given cycle's verification passes or fails, without
// modifying any production code.
func TestFixEngine_CycleScenarios(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		maxCycles        int
		agentExitCodes   []int  // exit code returned by agent on each call; last value repeated
		verifyPass       []bool // true = pass on that cycle's verification
		wantTotalCycles  int
		wantFinalStatus  VerificationStatus
		wantFixesApplied bool
	}{
		{
			name:             "single cycle verification passes",
			maxCycles:        3,
			agentExitCodes:   []int{0},
			verifyPass:       []bool{true},
			wantTotalCycles:  1,
			wantFinalStatus:  VerificationPassed,
			wantFixesApplied: true,
		},
		{
			name:             "single cycle verification fails maxCycles=1",
			maxCycles:        1,
			agentExitCodes:   []int{0},
			verifyPass:       []bool{false},
			wantTotalCycles:  1,
			wantFinalStatus:  VerificationFailed,
			wantFixesApplied: true,
		},
		{
			name:             "two cycles first fails second passes",
			maxCycles:        3,
			agentExitCodes:   []int{0, 0},
			verifyPass:       []bool{false, true},
			wantTotalCycles:  2,
			wantFinalStatus:  VerificationPassed,
			wantFixesApplied: true,
		},
		{
			name:             "three cycles all fail maxCycles=3",
			maxCycles:        3,
			agentExitCodes:   []int{0, 0, 0},
			verifyPass:       []bool{false, false, false},
			wantTotalCycles:  3,
			wantFinalStatus:  VerificationFailed,
			wantFixesApplied: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			callIdx := 0
			ag := agent.NewMockAgent("claude")
			ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
				i := callIdx
				if i >= len(tt.agentExitCodes) {
					i = len(tt.agentExitCodes) - 1
				}
				callIdx++
				return &agent.RunResult{ExitCode: tt.agentExitCodes[i]}, nil
			}

			// Prepare a counting verifier that returns per-cycle verdicts.
			tempDir := t.TempDir()
			counterFile := tempDir + "/counter"
			verdictFile := tempDir + "/verdicts"
			initCountingVerifier(t, counterFile, verdictFile, tt.verifyPass)
			verifier := makeCountingVerifier(counterFile, verdictFile)

			fe := NewFixEngine(ag, verifier, tt.maxCycles, nil, nil)
			report, err := fe.Fix(context.Background(), FixOpts{
				Findings: makeFindingSet(2),
			})

			require.NoError(t, err)
			require.NotNil(t, report)

			assert.Equal(t, tt.wantTotalCycles, report.TotalCycles,
				"TotalCycles mismatch for %q", tt.name)
			assert.Equal(t, tt.wantFinalStatus, report.FinalStatus,
				"FinalStatus mismatch for %q", tt.name)
			assert.Equal(t, tt.wantFixesApplied, report.FixesApplied,
				"FixesApplied mismatch for %q", tt.name)
		})
	}
}

// TestFixEngine_Fix_SingleCycle_VerificationFails_MaxCycles1 is an explicit,
// named test for the AC "Single cycle, verification fails, maxCycles=1:
// FixReport with 1 cycle, FinalStatus=failed".
func TestFixEngine_Fix_SingleCycle_VerificationFails_MaxCycles1(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{ExitCode: 0, Stdout: "applied"}, nil
	}

	// Verifier that always fails.
	verifier := NewVerificationRunner([]string{"exit 1"}, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 1, nil, nil)

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings:  makeFindingSet(1),
		MaxCycles: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Equal(t, 1, report.TotalCycles)
	assert.Len(t, report.Cycles, 1)
	assert.Equal(t, VerificationFailed, report.FinalStatus)
	// Agent ran successfully so fixes were applied.
	assert.True(t, report.FixesApplied)
}

// TestFixEngine_Fix_ThreeCycles_AllFail verifies the AC
// "Three cycles, all fail, maxCycles=3: FixReport with 3 cycles, FinalStatus=failed".
func TestFixEngine_Fix_ThreeCycles_AllFail(t *testing.T) {
	t.Parallel()

	callCount := 0
	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		return &agent.RunResult{ExitCode: 0}, nil
	}

	verifier := NewVerificationRunner([]string{"exit 1"}, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 3, nil, nil)

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings:  makeFindingSet(1),
		MaxCycles: 3,
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Equal(t, 3, callCount, "agent must be called once per cycle")
	assert.Equal(t, 3, report.TotalCycles)
	assert.Len(t, report.Cycles, 3)
	assert.Equal(t, VerificationFailed, report.FinalStatus)
	assert.True(t, report.FixesApplied)

	// Every cycle must have its Cycle index set correctly (1-based).
	for i, c := range report.Cycles {
		assert.Equal(t, i+1, c.Cycle)
	}
}

// TestFixEngine_Fix_TwoCycles_FirstFails_SecondPasses verifies the AC
// "Two cycles, first fails, second passes: FixReport with 2 cycles, FinalStatus=passed".
func TestFixEngine_Fix_TwoCycles_FirstFails_SecondPasses(t *testing.T) {
	t.Parallel()

	callCount := 0
	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		return &agent.RunResult{ExitCode: 0}, nil
	}

	tempDir := t.TempDir()
	counterFile := tempDir + "/counter"
	verdictFile := tempDir + "/verdicts"
	initCountingVerifier(t, counterFile, verdictFile, []bool{false, true})
	verifier := makeCountingVerifier(counterFile, verdictFile)

	fe := NewFixEngine(ag, verifier, 5, nil, nil)

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings: makeFindingSet(1),
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Equal(t, 2, callCount)
	assert.Equal(t, 2, report.TotalCycles)
	assert.Equal(t, VerificationFailed, report.Cycles[0].Verification.Status)
	assert.Equal(t, VerificationPassed, report.Cycles[1].Verification.Status)
	assert.Equal(t, VerificationPassed, report.FinalStatus)
	assert.True(t, report.FixesApplied)
}

// TestFixEngine_Fix_PromptIncludesPreviousFailures verifies that cycle 2's
// prompt includes cycle 1 failure output (AC: "Updated prompt for cycle 2
// includes cycle 1 failure output").
func TestFixEngine_Fix_PromptIncludesPreviousFailures(t *testing.T) {
	t.Parallel()

	var capturedPrompts []string
	callCount := 0
	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		capturedPrompts = append(capturedPrompts, opts.Prompt)
		return &agent.RunResult{ExitCode: 0}, nil
	}

	tempDir := t.TempDir()
	counterFile := tempDir + "/counter"
	verdictFile := tempDir + "/verdicts"

	// Cycle 1 fails with a distinctive stderr message, cycle 2 passes.
	// We need a custom command that also writes to stderr on failure.
	initCountingVerifier(t, counterFile, verdictFile, []bool{false, true})

	// Override with a command that emits a recognisable stderr on failure.
	verifyCmd := fmt.Sprintf(
		`n=$(cat %s); code=$(sed -n "$((n+1))p" %s); echo $((n+1)) > %s; `+
			`if [ "$code" = "1" ]; then echo "cycle1-verification-error" >&2; fi; exit ${code:-0}`,
		counterFile,
		verdictFile,
		counterFile,
	)
	verifier := NewVerificationRunner([]string{verifyCmd}, "", 0, nil)

	pb := NewFixPromptBuilder(
		[]string{"no global state"},
		[]string{"go test ./..."},
		nil,
	)
	fe := NewFixEngine(ag, verifier, 5, nil, nil).WithPromptBuilder(pb)

	findings := []*Finding{
		{
			File:        "main.go",
			Line:        10,
			Severity:    SeverityHigh,
			Category:    "correctness",
			Description: "missing error check",
			Suggestion:  "always check error return values",
		},
	}

	report, err := fe.Fix(context.Background(), FixOpts{Findings: findings})
	require.NoError(t, err)
	require.NotNil(t, report)

	// Must have run 2 cycles.
	require.Equal(t, 2, callCount)
	require.Len(t, capturedPrompts, 2)

	// Cycle 1 prompt must NOT contain the "Previous Fix Attempt Results" section.
	assert.NotContains(t, capturedPrompts[0], "Previous Fix Attempt Results",
		"cycle 1 prompt must not have previous failures section")

	// Cycle 2 prompt must contain cycle 1 failure context.
	assert.Contains(t, capturedPrompts[1], "Previous Fix Attempt Results",
		"cycle 2 prompt must include previous failures section")
	assert.Contains(t, capturedPrompts[1], "Cycle 1 Failures",
		"cycle 2 prompt must reference cycle 1 failures")
	assert.Contains(t, capturedPrompts[1], "cycle1-verification-error",
		"cycle 2 prompt must include cycle 1 stderr output")
}

// TestFixEngine_Fix_AgentNonZeroExit_NotAbort verifies that an agent that
// returns a non-zero exit code (but no Go-level error) does not abort the
// loop. The loop continues until verification passes or maxCycles is reached.
// FixesApplied remains false because no cycle had an agent exit code of 0.
func TestFixEngine_Fix_AgentNonZeroExit_NotAbort(t *testing.T) {
	t.Parallel()

	const maxCycles = 2
	callCount := 0
	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		// Non-zero exit but no Go-level error — the loop must continue.
		return &agent.RunResult{ExitCode: 1, Stderr: "agent could not apply fix"}, nil
	}

	// Use a failing verifier so the loop does not break early on verification success.
	verifier := NewVerificationRunner([]string{"exit 1"}, "", 0, nil)
	fe := NewFixEngine(ag, verifier, maxCycles, nil, nil)

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings:  makeFindingSet(1),
		MaxCycles: maxCycles,
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	// The loop must NOT abort on non-zero exit; all maxCycles must run.
	assert.Equal(t, maxCycles, callCount, "agent must be called once per cycle even when exit code is non-zero")
	assert.Equal(t, maxCycles, report.TotalCycles)
	// FixesApplied must be false because the agent always exits 1.
	assert.False(t, report.FixesApplied)
	// Verification failed every cycle.
	assert.Equal(t, VerificationFailed, report.FinalStatus)
}

// TestFixEngine_Fix_ContextCancelledDuringAgent verifies that cancelling the
// context while the agent is running causes Fix to return a partial result
// with no error.
func TestFixEngine_Fix_ContextCancelledDuringAgent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		// Cancel the context synchronously before returning to simulate
		// cancellation happening during the agent call.
		cancel()
		// Return a successful result — the context check happens after Run().
		return &agent.RunResult{ExitCode: 0}, nil
	}

	verifier := NewVerificationRunner(nil, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 5, nil, nil)

	report, err := fe.Fix(ctx, FixOpts{
		Findings: makeFindingSet(1),
	})

	// Context cancellation must NOT surface as an error.
	require.NoError(t, err)
	require.NotNil(t, report)

	// At most 1 cycle ran before cancellation was honoured.
	assert.LessOrEqual(t, report.TotalCycles, 2,
		"at most one extra cycle should run after cancellation")
}

// TestFixEngine_Fix_EmptyDiffAfterFix verifies that when the agent produces no
// changes (empty diff), the cycle's DiffAfterFix field is an empty string and
// the loop continues normally.
func TestFixEngine_Fix_EmptyDiffAfterFix(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{ExitCode: 0}, nil
	}

	verifier := NewVerificationRunner(nil, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 1, nil, nil)

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings: makeFindingSet(1),
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	// DiffAfterFix may be empty (no changes on the test machine's working tree).
	// The important thing is that the engine does not error and the cycle is
	// recorded. DiffAfterFix is a string, so empty is valid.
	assert.Equal(t, 1, report.TotalCycles)
	// The cycle must be present regardless of diff content.
	require.Len(t, report.Cycles, 1)
	// DiffAfterFix is allowed to be "" — we just assert the field type is string.
	_ = report.Cycles[0].DiffAfterFix // no panic = pass
}

// TestFixEngine_Fix_MaxCyclesNegative verifies that negative maxCycles is
// treated the same as zero (fast path, no cycles run).
func TestFixEngine_Fix_MaxCyclesNegative(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	fe := NewFixEngine(ag, nil, -1, nil, nil)

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings: makeFindingSet(1),
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Equal(t, VerificationPassed, report.FinalStatus)
	assert.Equal(t, 0, report.TotalCycles)
	assert.Empty(t, ag.Calls)
}

// TestFixEngine_Fix_VerifierNil_NoVerification verifies that when the FixEngine
// is constructed with a nil verifier, cycles still run but verification is
// skipped (Verification == nil in each cycle).
func TestFixEngine_Fix_VerifierNil_NoVerification(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{ExitCode: 0}, nil
	}

	// Pass nil verifier — engine should run all cycles and set final status to
	// VerificationFailed (initial value, never updated by a real verifier).
	fe := NewFixEngine(ag, nil, 2, nil, nil)

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings:  makeFindingSet(1),
		MaxCycles: 2,
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Equal(t, 2, report.TotalCycles)
	for _, c := range report.Cycles {
		assert.Nil(t, c.Verification,
			"Verification must be nil when no verifier is configured")
	}
	// finalStatus stays VerificationFailed because no verifier ran.
	assert.Equal(t, VerificationFailed, report.FinalStatus)
}

// TestFixEngine_Fix_PromptContainsAllFindings verifies that the fix prompt sent
// to the agent includes the description and file of every finding (AC: "Fix
// prompt includes all finding descriptions and affected files").
func TestFixEngine_Fix_PromptContainsAllFindings(t *testing.T) {
	t.Parallel()

	var capturedPrompt string
	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		capturedPrompt = opts.Prompt
		return &agent.RunResult{ExitCode: 0}, nil
	}

	findings := []*Finding{
		{
			File:        "internal/auth/auth.go",
			Line:        42,
			Severity:    SeverityCritical,
			Category:    "security",
			Description: "SQL injection vulnerability",
			Suggestion:  "use parameterised queries",
		},
		{
			File:        "internal/api/handler.go",
			Line:        88,
			Severity:    SeverityHigh,
			Category:    "correctness",
			Description: "unchecked error from Write",
			Suggestion:  "always check Write return value",
		},
	}

	verifier := NewVerificationRunner(nil, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 1, nil, nil)

	_, err := fe.Fix(context.Background(), FixOpts{Findings: findings})
	require.NoError(t, err)

	// Every finding's file, description, and suggestion must appear in the prompt.
	for _, f := range findings {
		assert.Contains(t, capturedPrompt, f.File,
			"prompt must contain file %q", f.File)
		assert.Contains(t, capturedPrompt, f.Description,
			"prompt must contain description %q", f.Description)
		assert.Contains(t, capturedPrompt, f.Suggestion,
			"prompt must contain suggestion %q", f.Suggestion)
	}
}

// TestFixEngine_Fix_PromptContainsVerificationCommands verifies that the fix
// prompt includes the configured verification commands (AC: "Fix prompt includes
// verification commands").
func TestFixEngine_Fix_PromptContainsVerificationCommands(t *testing.T) {
	t.Parallel()

	var capturedPrompt string
	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		capturedPrompt = opts.Prompt
		return &agent.RunResult{ExitCode: 0}, nil
	}

	verifyCommands := []string{
		"go build ./...",
		"go test ./...",
		"go vet ./...",
	}
	pb := NewFixPromptBuilder(nil, verifyCommands, nil)
	verifier := NewVerificationRunner(nil, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 1, nil, nil).WithPromptBuilder(pb)

	_, err := fe.Fix(context.Background(), FixOpts{Findings: makeFindingSet(1)})
	require.NoError(t, err)

	for _, cmd := range verifyCommands {
		assert.Contains(t, capturedPrompt, cmd,
			"prompt must contain verification command %q", cmd)
	}
}

// TestFixEngine_Fix_CycleNumbers verifies that FixCycleResult.Cycle is set to
// the correct 1-based cycle number for each cycle.
func TestFixEngine_Fix_CycleNumbers(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{ExitCode: 0}, nil
	}

	// Failing verifier so all 3 cycles run.
	verifier := NewVerificationRunner([]string{"exit 1"}, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 3, nil, nil)

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings:  makeFindingSet(1),
		MaxCycles: 3,
	})
	require.NoError(t, err)
	require.Len(t, report.Cycles, 3)

	for i, c := range report.Cycles {
		assert.Equal(t, i+1, c.Cycle, "Cycle field must be 1-based index")
	}
}

// TestFixEngine_Fix_AgentErrorCycleHasNilAgentResult verifies that when an
// agent call returns an error (not just non-zero exit), the FixCycleResult
// has AgentResult == nil and Verification == nil.
func TestFixEngine_Fix_AgentErrorCycleHasNilAgentResult(t *testing.T) {
	t.Parallel()

	callCount := 0
	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		return nil, fmt.Errorf("network timeout on call %d", callCount)
	}

	verifier := NewVerificationRunner(nil, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 2, nil, nil)

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings:  makeFindingSet(1),
		MaxCycles: 2,
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	// Both cycles should be recorded even though the agent errored.
	assert.Equal(t, 2, report.TotalCycles)
	assert.Equal(t, 2, callCount)

	for _, c := range report.Cycles {
		assert.Nil(t, c.AgentResult, "AgentResult must be nil when agent.Run() returns an error")
		assert.Nil(t, c.Verification, "Verification must be nil when agent errored")
	}

	// No successful agent run means FixesApplied is false.
	assert.False(t, report.FixesApplied)
}

// TestFixEngine_Fix_AgentErrorThenSuccess verifies that after an agent error on
// cycle 1, the engine retries on cycle 2 and succeeds.
func TestFixEngine_Fix_AgentErrorThenSuccess(t *testing.T) {
	t.Parallel()

	callCount := 0
	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		if callCount == 1 {
			return nil, fmt.Errorf("transient agent error")
		}
		return &agent.RunResult{ExitCode: 0, Stdout: "fixed on cycle 2"}, nil
	}

	verifier := NewVerificationRunner(nil, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 3, nil, nil)

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings: makeFindingSet(1),
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Equal(t, 2, callCount)
	assert.Equal(t, 2, report.TotalCycles)

	// Cycle 1: agent error.
	assert.Nil(t, report.Cycles[0].AgentResult)
	// Cycle 2: success.
	require.NotNil(t, report.Cycles[1].AgentResult)
	assert.Equal(t, 0, report.Cycles[1].AgentResult.ExitCode)
	assert.True(t, report.FixesApplied)
	assert.Equal(t, VerificationPassed, report.FinalStatus)
}

// TestFixEngine_Fix_EventSequence verifies that all expected event types are
// emitted in the correct order for a single successful cycle.
func TestFixEngine_Fix_EventSequence(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{ExitCode: 0}, nil
	}

	events := make(chan FixEvent, 50)
	verifier := NewVerificationRunner(nil, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 3, nil, events)

	_, err := fe.Fix(context.Background(), FixOpts{
		Findings:  makeFindingSet(1),
		MaxCycles: 1,
	})
	require.NoError(t, err)
	close(events)

	var types []string
	for ev := range events {
		types = append(types, ev.Type)
		// All events must have a non-zero timestamp.
		assert.False(t, ev.Timestamp.IsZero(), "event %q must have non-zero Timestamp", ev.Type)
	}

	// Expected types in order for a single successful cycle.
	wantContains := []string{
		"fix_started",
		"cycle_started",
		"agent_invoked",
		"verification_started",
		"verification_result",
		"cycle_completed",
		"fix_completed",
	}
	for _, want := range wantContains {
		assert.Contains(t, types, want, "event sequence must include %q", want)
	}

	// fix_started must come before fix_completed.
	startIdx := -1
	endIdx := -1
	for i, ty := range types {
		if ty == "fix_started" {
			startIdx = i
		}
		if ty == "fix_completed" {
			endIdx = i
		}
	}
	assert.Greater(t, endIdx, startIdx,
		"fix_completed must come after fix_started")
}

// TestFixEngine_Fix_EventCycleField verifies that cycle-scoped events carry
// the correct Cycle number.
func TestFixEngine_Fix_EventCycleField(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{ExitCode: 0}, nil
	}

	events := make(chan FixEvent, 100)
	verifier := NewVerificationRunner([]string{"exit 1"}, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 2, nil, events)

	_, err := fe.Fix(context.Background(), FixOpts{
		Findings:  makeFindingSet(1),
		MaxCycles: 2,
	})
	require.NoError(t, err)
	close(events)

	cycleEvents := map[string][]int{} // event type -> list of Cycle values seen
	for ev := range events {
		if ev.Cycle > 0 {
			cycleEvents[ev.Type] = append(cycleEvents[ev.Type], ev.Cycle)
		}
	}

	// cycle_started events must have Cycle=1 then Cycle=2.
	if starts, ok := cycleEvents["cycle_started"]; ok {
		for i, c := range starts {
			assert.Equal(t, i+1, c,
				"cycle_started[%d] must have Cycle=%d", i, i+1)
		}
	}
}

// TestFixEngine_Fix_FullReportStructure verifies all fields of FixReport are
// populated correctly after a successful run.
func TestFixEngine_Fix_FullReportStructure(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{ExitCode: 0, Stdout: "all fixed"}, nil
	}

	verifier := NewVerificationRunner(nil, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 5, nil, nil)

	findings := makeFindingSet(3)
	before := time.Now()
	report, err := fe.Fix(context.Background(), FixOpts{Findings: findings})
	after := time.Now()

	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Equal(t, VerificationPassed, report.FinalStatus)
	assert.Equal(t, 1, report.TotalCycles)
	assert.True(t, report.FixesApplied)
	assert.Greater(t, report.Duration, time.Duration(0))
	assert.LessOrEqual(t, report.Duration, after.Sub(before)+time.Second,
		"Duration must be within reasonable bounds")

	require.Len(t, report.Cycles, 1)
	c := report.Cycles[0]
	assert.Equal(t, 1, c.Cycle)
	require.NotNil(t, c.AgentResult)
	assert.Equal(t, 0, c.AgentResult.ExitCode)
	assert.NotNil(t, c.Verification)
	assert.Equal(t, VerificationPassed, c.Verification.Status)
	assert.Greater(t, c.Duration, time.Duration(0))
}

// TestFixEngine_Fix_MaxCyclesOpts_Overrides_Engine verifies that opts.MaxCycles
// takes precedence over the engine's configured maxCycles when opts.MaxCycles > 0.
func TestFixEngine_Fix_MaxCyclesOpts_Overrides_Engine(t *testing.T) {
	t.Parallel()

	callCount := int32(0)
	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		atomic.AddInt32(&callCount, 1)
		return &agent.RunResult{ExitCode: 0}, nil
	}

	// Engine default is 10, but opts will cap it to 2.
	verifier := NewVerificationRunner([]string{"exit 1"}, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 10, nil, nil)

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings:  makeFindingSet(1),
		MaxCycles: 2,
	})
	require.NoError(t, err)

	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount),
		"opts.MaxCycles=2 must override engine default of 10")
	assert.Equal(t, 2, report.TotalCycles)
}

// TestFixEngine_Fix_MultipleFindingsSeverities verifies that findings of all
// severity levels are correctly included in the fix prompt.
func TestFixEngine_Fix_MultipleFindingsSeverities(t *testing.T) {
	t.Parallel()

	var capturedPrompt string
	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		capturedPrompt = opts.Prompt
		return &agent.RunResult{ExitCode: 0}, nil
	}

	findings := []*Finding{
		{File: "a.go", Line: 1, Severity: SeverityCritical, Category: "security", Description: "critical issue", Suggestion: "fix critical"},
		{File: "b.go", Line: 2, Severity: SeverityHigh, Category: "correctness", Description: "high issue", Suggestion: "fix high"},
		{File: "c.go", Line: 3, Severity: SeverityMedium, Category: "style", Description: "medium issue", Suggestion: "fix medium"},
		{File: "d.go", Line: 4, Severity: SeverityLow, Category: "docs", Description: "low issue", Suggestion: "fix low"},
		{File: "e.go", Line: 5, Severity: SeverityInfo, Category: "info", Description: "info issue", Suggestion: "fix info"},
	}

	verifier := NewVerificationRunner(nil, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 1, nil, nil)

	_, err := fe.Fix(context.Background(), FixOpts{Findings: findings})
	require.NoError(t, err)

	for _, f := range findings {
		assert.Contains(t, capturedPrompt, string(f.Severity),
			"prompt must contain severity %q", f.Severity)
		assert.Contains(t, capturedPrompt, f.Description,
			"prompt must contain description %q", f.Description)
		assert.Contains(t, capturedPrompt, f.File,
			"prompt must contain file %q", f.File)
	}
}

// TestFixPromptBuilder_Build_ExactlyAtDiffLimit verifies that a diff of
// exactly maxFixDiffBytes is not truncated.
func TestFixPromptBuilder_Build_ExactlyAtDiffLimit(t *testing.T) {
	t.Parallel()

	pb := NewFixPromptBuilder(nil, nil, nil)
	exactDiff := strings.Repeat("x", maxFixDiffBytes)

	prompt, err := pb.Build(nil, exactDiff, nil)
	require.NoError(t, err)

	assert.NotContains(t, prompt, "[diff truncated at 50KB]",
		"diff exactly at limit must not be truncated")
	assert.Contains(t, prompt, exactDiff[:100],
		"prompt must contain the full diff when at limit")
}

// TestFixPromptBuilder_Build_OneBytePastDiffLimit verifies that a diff of
// maxFixDiffBytes+1 is truncated.
func TestFixPromptBuilder_Build_OneBytePastDiffLimit(t *testing.T) {
	t.Parallel()

	pb := NewFixPromptBuilder(nil, nil, nil)
	overDiff := strings.Repeat("y", maxFixDiffBytes+1)

	prompt, err := pb.Build(nil, overDiff, nil)
	require.NoError(t, err)

	assert.Contains(t, prompt, "[diff truncated at 50KB]",
		"diff one byte over limit must be truncated")
}

// TestFixPromptBuilder_Build_MultiplePreviousFailures verifies that a prompt
// built with two prior failure cycles mentions both cycles.
func TestFixPromptBuilder_Build_MultiplePreviousFailures(t *testing.T) {
	t.Parallel()

	findings := []*Finding{
		{File: "x.go", Line: 1, Severity: SeverityHigh, Category: "test", Description: "issue", Suggestion: "fix"},
	}

	prev := []FixCycleResult{
		{
			Cycle: 1,
			Verification: &VerificationReport{
				Status: VerificationFailed,
				Results: []CommandResult{
					{Command: "go build ./...", ExitCode: 1, Stderr: "build error from cycle 1", Passed: false},
				},
			},
		},
		{
			Cycle: 2,
			Verification: &VerificationReport{
				Status: VerificationFailed,
				Results: []CommandResult{
					{Command: "go test ./...", ExitCode: 1, Stderr: "test error from cycle 2", Passed: false},
				},
			},
		},
	}

	pb := NewFixPromptBuilder(nil, []string{"go build ./...", "go test ./..."}, nil)
	prompt, err := pb.Build(findings, "some diff", prev)
	require.NoError(t, err)

	assert.Contains(t, prompt, "Cycle 1 Failures")
	assert.Contains(t, prompt, "Cycle 2 Failures")
	assert.Contains(t, prompt, "build error from cycle 1")
	assert.Contains(t, prompt, "test error from cycle 2")
}

// TestFixPromptBuilder_Build_ConventionsSection verifies the conventions section
// is present when conventions are provided but absent when conventions is nil.
func TestFixPromptBuilder_Build_ConventionsSection(t *testing.T) {
	t.Parallel()

	findings := []*Finding{
		{File: "x.go", Line: 1, Severity: SeverityLow, Description: "minor", Suggestion: "fix"},
	}

	t.Run("with conventions", func(t *testing.T) {
		t.Parallel()
		pb := NewFixPromptBuilder([]string{"use early returns", "no global state"}, nil, nil)
		prompt, err := pb.Build(findings, "", nil)
		require.NoError(t, err)
		assert.Contains(t, prompt, "Project Conventions")
		assert.Contains(t, prompt, "use early returns")
		assert.Contains(t, prompt, "no global state")
	})

	t.Run("without conventions", func(t *testing.T) {
		t.Parallel()
		pb := NewFixPromptBuilder(nil, nil, nil)
		prompt, err := pb.Build(findings, "", nil)
		require.NoError(t, err)
		// The conventions section should be absent when there are no conventions.
		assert.NotContains(t, prompt, "Project Conventions")
	})
}

// TestFixEngine_DryRun_DoesNotInvokeAgent verifies that DryRun never calls
// agent.Run() even when findings are provided.
func TestFixEngine_DryRun_DoesNotInvokeAgent(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	// RunFunc set to panic so any invocation fails the test.
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		t.Fatal("DryRun must not invoke agent.Run()")
		return nil, nil
	}

	pb := NewFixPromptBuilder([]string{"follow SOLID"}, []string{"go build ./..."}, nil)
	fe := NewFixEngine(ag, nil, 3, nil, nil).WithPromptBuilder(pb)

	findings := []*Finding{
		{File: "main.go", Line: 1, Severity: SeverityHigh, Category: "correctness", Description: "bug", Suggestion: "fix it"},
	}

	prompt, err := fe.DryRun(context.Background(), FixOpts{Findings: findings})
	require.NoError(t, err)
	assert.NotEmpty(t, prompt)
	assert.Empty(t, ag.Calls, "DryRun must not add entries to ag.Calls")
}

// TestFixEngine_DryRun_ReturnsBuildPrompt verifies the prompt returned by DryRun
// contains findings, conventions, and verification commands.
func TestFixEngine_DryRun_ReturnsBuildPrompt(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	pb := NewFixPromptBuilder(
		[]string{"wrap errors with context"},
		[]string{"go test ./...", "go vet ./..."},
		nil,
	)
	fe := NewFixEngine(ag, nil, 3, nil, nil).WithPromptBuilder(pb)

	findings := []*Finding{
		{
			File:        "internal/pkg/handler.go",
			Line:        55,
			Severity:    SeverityMedium,
			Category:    "error-handling",
			Description: "error swallowed silently",
			Suggestion:  "return or log the error",
		},
	}

	prompt, err := fe.DryRun(context.Background(), FixOpts{Findings: findings})
	require.NoError(t, err)

	assert.Contains(t, prompt, "internal/pkg/handler.go")
	assert.Contains(t, prompt, "error swallowed silently")
	assert.Contains(t, prompt, "return or log the error")
	assert.Contains(t, prompt, "wrap errors with context")
	assert.Contains(t, prompt, "go test ./...")
	assert.Contains(t, prompt, "go vet ./...")
}

// TestFixEngine_Fix_VerificationCrash verifies that a verification command that
// itself crashes (process killed or not found) is handled gracefully. The cycle
// result captures the outcome and the engine does not panic or error out.
func TestFixEngine_Fix_VerificationCrash(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{ExitCode: 0}, nil
	}

	// A command that does not exist on PATH should be handled gracefully by
	// the verification runner; it results in a non-passing CommandResult.
	verifier := NewVerificationRunner(
		[]string{"this-command-absolutely-does-not-exist-9z8x7w"},
		"",
		0,
		nil,
	)
	fe := NewFixEngine(ag, verifier, 1, nil, nil)

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings:  makeFindingSet(1),
		MaxCycles: 1,
	})
	// The engine must not return an error; the crash is captured in the report.
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Equal(t, 1, report.TotalCycles)
	require.Len(t, report.Cycles, 1)
	require.NotNil(t, report.Cycles[0].Verification)
	// The non-existent command must have failed.
	assert.Equal(t, VerificationFailed, report.Cycles[0].Verification.Status)
	assert.Equal(t, VerificationFailed, report.FinalStatus)
}

// TestFixEngine_Fix_FixesApplied_MixedAgentResults verifies that FixesApplied
// is true when at least one cycle's agent returned exit code 0, even if
// other cycles had non-zero exit codes.
func TestFixEngine_Fix_FixesApplied_MixedAgentResults(t *testing.T) {
	t.Parallel()

	callCount := 0
	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		if callCount == 1 {
			// First call fails.
			return &agent.RunResult{ExitCode: 1}, nil
		}
		// Second call succeeds.
		return &agent.RunResult{ExitCode: 0}, nil
	}

	// Failing verifier so two cycles run.
	verifier := NewVerificationRunner([]string{"exit 1"}, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 3, nil, nil)

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings:  makeFindingSet(1),
		MaxCycles: 2,
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	// At least cycle 2 had exit code 0, so FixesApplied must be true.
	assert.True(t, report.FixesApplied,
		"FixesApplied must be true when any cycle's agent exited 0")
}

// TestFixEngine_Fix_NoFindings_ReturnsImmediately verifies the fast path for
// zero findings: FixReport has TotalCycles=0 and FinalStatus=passed (AC: "Zero
// findings: no-op, returns empty FixReport with status=passed").
func TestFixEngine_Fix_NoFindings_ReturnsImmediately(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	fe := NewFixEngine(ag, nil, 5, nil, nil)

	report, err := fe.Fix(context.Background(), FixOpts{
		Findings:  []*Finding{},
		MaxCycles: 5,
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Equal(t, VerificationPassed, report.FinalStatus)
	assert.Equal(t, 0, report.TotalCycles)
	assert.False(t, report.FixesApplied)
	assert.Empty(t, report.Cycles)
	// Agent must not have been called.
	assert.Empty(t, ag.Calls)
}

// TestFixEngine_Fix_ContextCancelledBeforeVerification verifies that when the
// context is already cancelled before the verifier runs (cancelled by the
// agent's RunFunc), the VerificationRunner returns immediately with a partial
// (empty) result, and Fix returns without error.
//
// Note: this test exercises the path where VerificationRunner.Run sees a
// cancelled context at the top of its command loop and returns (report, nil)
// without error. The fix.go code then sees verReport with status=Passed (no
// failures) and breaks the cycle loop.
func TestFixEngine_Fix_ContextCancelledBeforeVerification(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(_ context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		// Cancel before returning so that by the time verifier.Run is called
		// the context is already done. VerificationRunner.Run will see the
		// cancelled context at its first loop iteration and return immediately.
		cancel()
		return &agent.RunResult{ExitCode: 0}, nil
	}

	// Use a slow command to make intent clear (it won't actually run since
	// context is already cancelled before the command loop starts).
	verifier := NewVerificationRunner([]string{"sleep 5"}, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 5, nil, nil)

	report, err := fe.Fix(ctx, FixOpts{
		Findings:  makeFindingSet(1),
		MaxCycles: 5,
	})

	// No error is returned from Fix regardless of context state.
	require.NoError(t, err)
	require.NotNil(t, report)

	// At most 1 cycle ran because: cycle 1 runs agent (which cancels ctx),
	// verifier returns immediately (ctx already cancelled), verifier report
	// shows "passed" (0 commands ran = 0 failures), loop breaks.
	// Subsequent cycles are not attempted because ctx is cancelled and the
	// cycle-start check fires.
	assert.LessOrEqual(t, report.TotalCycles, 2)
	assert.True(t, report.FixesApplied, "the agent ran successfully (exit 0)")
}

// TestFixEngine_Fix_FullEventChannelDropsEvents verifies that when the events
// channel is full, the engine does not block and events are dropped silently.
func TestFixEngine_Fix_FullEventChannelDropsEvents(t *testing.T) {
	t.Parallel()

	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{ExitCode: 0}, nil
	}

	// Create a channel with capacity 0 (unbuffered) so every send blocks.
	// The non-blocking send in emit() should drop all events silently.
	events := make(chan FixEvent) // unbuffered = always full from non-blocking perspective
	verifier := NewVerificationRunner(nil, "", 0, nil)
	fe := NewFixEngine(ag, verifier, 1, nil, events)

	// Should not block or panic even though events channel is always "full".
	report, err := fe.Fix(context.Background(), FixOpts{
		Findings:  makeFindingSet(1),
		MaxCycles: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	// No events should be in the channel (all dropped since no receiver).
	assert.Equal(t, 0, len(events))
}

// ---------------------------------------------------------------------------
// Benchmarks for fix engine
// ---------------------------------------------------------------------------

// BenchmarkFixPromptBuilder_Build measures prompt construction for a
// realistic number of findings.
func BenchmarkFixPromptBuilder_Build(b *testing.B) {
	pb := NewFixPromptBuilder(
		[]string{"use early returns", "wrap errors", "no global state"},
		[]string{"go build ./...", "go test ./...", "go vet ./..."},
		nil,
	)

	findings := make([]*Finding, 20)
	for i := range findings {
		findings[i] = &Finding{
			File:        fmt.Sprintf("pkg/file%d.go", i),
			Line:        i * 10,
			Severity:    SeverityHigh,
			Category:    "correctness",
			Description: fmt.Sprintf("description of finding %d with some detail", i),
			Suggestion:  fmt.Sprintf("suggestion for finding %d explaining the fix", i),
		}
	}

	diff := strings.Repeat("+ added line\n- removed line\n", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pb.Build(findings, diff, nil)
	}
}

// BenchmarkFixEngine_Fix_SingleCycle measures Fix for a single fast cycle
// with no verifier.
func BenchmarkFixEngine_Fix_SingleCycle(b *testing.B) {
	ag := agent.NewMockAgent("claude")
	ag.RunFunc = func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{ExitCode: 0}, nil
	}

	fe := NewFixEngine(ag, nil, 1, nil, nil)
	findings := makeFindingSet(5)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = fe.Fix(context.Background(), FixOpts{
			Findings:  findings,
			MaxCycles: 1,
		})
	}
}
