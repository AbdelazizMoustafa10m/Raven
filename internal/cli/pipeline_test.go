package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/pipeline"
	"github.com/AbdelazizMoustafa10m/Raven/internal/task"
)

// ---- newPipelineCmd tests ---------------------------------------------------

func TestNewPipelineCmd_Registration(t *testing.T) {
	cmd := newPipelineCmd()
	assert.Equal(t, "pipeline", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
}

func TestNewPipelineCmd_FlagsExist(t *testing.T) {
	cmd := newPipelineCmd()

	expectedFlags := []string{
		"phase",
		"from-phase",
		"impl-agent",
		"review-agent",
		"fix-agent",
		"review-concurrency",
		"max-review-cycles",
		"skip-implement",
		"skip-review",
		"skip-fix",
		"skip-pr",
		"interactive",
		"dry-run",
		"base",
		"sync-base",
	}
	for _, name := range expectedFlags {
		flag := cmd.Flags().Lookup(name)
		assert.NotNil(t, flag, "expected flag --%s to exist", name)
	}
}

func TestNewPipelineCmd_Defaults(t *testing.T) {
	cmd := newPipelineCmd()

	tests := []struct {
		flag    string
		wantDef string
	}{
		{"review-concurrency", "2"},
		{"max-review-cycles", "3"},
		{"skip-implement", "false"},
		{"skip-review", "false"},
		{"skip-fix", "false"},
		{"skip-pr", "false"},
		{"interactive", "false"},
		{"dry-run", "false"},
		{"base", "main"},
		{"sync-base", "false"},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			f := cmd.Flags().Lookup(tt.flag)
			require.NotNil(t, f, "flag --%s must exist", tt.flag)
			assert.Equal(t, tt.wantDef, f.DefValue, "flag --%s default mismatch", tt.flag)
		})
	}
}

func TestPipelineCmdRegisteredOnRoot(t *testing.T) {
	found := false
	for _, sub := range rootCmd.Commands() {
		if sub.Use == "pipeline" {
			found = true
			break
		}
	}
	assert.True(t, found, "pipeline command should be registered as a subcommand of root")
}

// ---- completePipelineAgent tests --------------------------------------------

func TestCompletePipelineAgent(t *testing.T) {
	completions, directive := completePipelineAgent(nil, nil, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Contains(t, completions, "claude")
	assert.Contains(t, completions, "codex")
	assert.Contains(t, completions, "gemini")
	assert.Len(t, completions, 3)
}

// ---- newPipelineCmd shell completion tests ----------------------------------

func TestNewPipelineCmd_AgentFlagHasShellCompletion(t *testing.T) {
	cmd := newPipelineCmd()

	for _, flagName := range []string{"impl-agent", "review-agent", "fix-agent"} {
		t.Run(flagName, func(t *testing.T) {
			fn, exists := cmd.GetFlagCompletionFunc(flagName)
			require.True(t, exists, "--%s should have a completion function", flagName)
			require.NotNil(t, fn)

			completions, directive := fn(cmd, []string{}, "")
			assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
			assert.Contains(t, completions, "claude")
			assert.Contains(t, completions, "codex")
			assert.Contains(t, completions, "gemini")
		})
	}
}

func TestNewPipelineCmd_PhaseFlagHasShellCompletion(t *testing.T) {
	cmd := newPipelineCmd()

	for _, flagName := range []string{"phase", "from-phase"} {
		t.Run(flagName, func(t *testing.T) {
			fn, exists := cmd.GetFlagCompletionFunc(flagName)
			require.True(t, exists, "--%s should have a completion function", flagName)
			require.NotNil(t, fn)
			// We cannot load real phases in a unit test, but we verify the function
			// at minimum does not panic and returns some completions.
			completions, directive := fn(cmd, []string{}, "")
			assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
			assert.NotEmpty(t, completions, "phase completion should return at least some options")
		})
	}
}

// ---- validatePipelineFlags tests --------------------------------------------

func TestValidatePipelineFlags_ValidConcurrency(t *testing.T) {
	flags := pipelineFlags{
		Phase:             "1",
		ReviewConcurrency: 2,
		MaxReviewCycles:   3,
	}
	err := validatePipelineFlags(flags, nil)
	assert.NoError(t, err)
}

func TestValidatePipelineFlags_ZeroConcurrency(t *testing.T) {
	flags := pipelineFlags{
		Phase:             "1",
		ReviewConcurrency: 0,
		MaxReviewCycles:   3,
	}
	err := validatePipelineFlags(flags, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--review-concurrency must be >= 1")
}

func TestValidatePipelineFlags_ZeroMaxCycles(t *testing.T) {
	flags := pipelineFlags{
		Phase:             "1",
		ReviewConcurrency: 2,
		MaxReviewCycles:   0,
	}
	err := validatePipelineFlags(flags, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--max-review-cycles must be >= 1")
}

func TestValidatePipelineFlags_AllSkipped(t *testing.T) {
	flags := pipelineFlags{
		Phase:             "1",
		ReviewConcurrency: 2,
		MaxReviewCycles:   3,
		SkipImplement:     true,
		SkipReview:        true,
		SkipFix:           true,
		SkipPR:            true,
	}
	err := validatePipelineFlags(flags, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all pipeline stages are skipped")
}

func TestValidatePipelineFlags_InvalidPhaseString(t *testing.T) {
	flags := pipelineFlags{
		Phase:             "xyz",
		ReviewConcurrency: 2,
		MaxReviewCycles:   3,
	}
	err := validatePipelineFlags(flags, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --phase value")
}

func TestValidatePipelineFlags_PhaseNotFoundInPhases(t *testing.T) {
	phases := []task.Phase{
		{ID: 1, Name: "Foundation", StartTask: "T-001", EndTask: "T-010"},
		{ID: 2, Name: "Core", StartTask: "T-011", EndTask: "T-020"},
	}
	flags := pipelineFlags{
		Phase:             "99",
		ReviewConcurrency: 2,
		MaxReviewCycles:   3,
	}
	err := validatePipelineFlags(flags, phases)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	// Error should list available phases.
	assert.Contains(t, err.Error(), "1")
	assert.Contains(t, err.Error(), "2")
}

func TestValidatePipelineFlags_InvalidFromPhase(t *testing.T) {
	tests := []struct {
		name      string
		fromPhase string
		wantErr   bool
	}{
		{"valid from-phase", "2", false},
		{"invalid string", "abc", true},
		{"negative", "-1", true},
		{"zero", "0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := pipelineFlags{
				FromPhase:         tt.fromPhase,
				ReviewConcurrency: 2,
				MaxReviewCycles:   3,
			}
			err := validatePipelineFlags(flags, nil)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "--from-phase")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePipelineFlags_InvalidAgentName(t *testing.T) {
	tests := []struct {
		name   string
		agent  string
		flag   string
		wantOK bool
	}{
		{"valid claude impl", "claude", "impl", true},
		{"valid codex review", "codex", "review", true},
		{"invalid impl agent", "badagent", "impl", false},
		{"invalid review agent", "unknown", "review", false},
		{"invalid fix agent", "notreal", "fix", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := pipelineFlags{
				Phase:             "1",
				ReviewConcurrency: 2,
				MaxReviewCycles:   3,
			}
			switch tt.flag {
			case "impl":
				flags.ImplAgent = tt.agent
			case "review":
				flags.ReviewAgent = tt.agent
			case "fix":
				flags.FixAgent = tt.agent
			}
			err := validatePipelineFlags(flags, nil)
			if tt.wantOK {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unknown agent")
			}
		})
	}
}

func TestValidatePipelineFlags_PhaseAllIsValid(t *testing.T) {
	// "all" must not fail the phase validation even with phases loaded.
	phases := []task.Phase{
		{ID: 1, Name: "Foundation", StartTask: "T-001", EndTask: "T-010"},
	}
	flags := pipelineFlags{
		Phase:             "all",
		ReviewConcurrency: 2,
		MaxReviewCycles:   3,
	}
	err := validatePipelineFlags(flags, phases)
	assert.NoError(t, err)
}

// ---- buildPipelineOpts tests ------------------------------------------------

func TestBuildPipelineOpts_AllFlagsSet(t *testing.T) {
	flags := pipelineFlags{
		Phase:             "2",
		ImplAgent:         "claude",
		ReviewAgent:       "codex",
		FixAgent:          "gemini",
		ReviewConcurrency: 4,
		MaxReviewCycles:   5,
		SkipImplement:     true,
		SkipReview:        false,
		SkipFix:           true,
		SkipPR:            false,
		DryRun:            true,
		Interactive:       false,
	}
	opts := buildPipelineOpts(flags)

	assert.Equal(t, "2", opts.PhaseID)
	assert.Equal(t, "", opts.FromPhase)
	assert.Equal(t, "claude", opts.ImplAgent)
	assert.Equal(t, "codex", opts.ReviewAgent)
	assert.Equal(t, "gemini", opts.FixAgent)
	assert.Equal(t, 4, opts.ReviewConcurrency)
	assert.Equal(t, 5, opts.MaxReviewCycles)
	assert.True(t, opts.SkipImplement)
	assert.False(t, opts.SkipReview)
	assert.True(t, opts.SkipFix)
	assert.False(t, opts.SkipPR)
	assert.True(t, opts.DryRun)
	assert.False(t, opts.Interactive)
}

func TestBuildPipelineOpts_AllNormalisedToEmpty(t *testing.T) {
	// "all" in Phase must be normalised to "" for the orchestrator.
	tests := []struct {
		phaseStr string
	}{
		{"all"},
		{"ALL"},
		{"All"},
	}
	for _, tt := range tests {
		t.Run(tt.phaseStr, func(t *testing.T) {
			flags := pipelineFlags{Phase: tt.phaseStr, ReviewConcurrency: 2, MaxReviewCycles: 3}
			opts := buildPipelineOpts(flags)
			assert.Equal(t, "", opts.PhaseID, `Phase "all" must be normalised to "" in PipelineOpts`)
		})
	}
}

func TestBuildPipelineOpts_FromPhasePreserved(t *testing.T) {
	flags := pipelineFlags{FromPhase: "3", ReviewConcurrency: 2, MaxReviewCycles: 3}
	opts := buildPipelineOpts(flags)
	assert.Equal(t, "3", opts.FromPhase)
	assert.Equal(t, "", opts.PhaseID)
}

// ---- applyWizardOpts tests --------------------------------------------------

func TestApplyWizardOpts_OverridesEmptyFields(t *testing.T) {
	wizardOpts := &pipeline.PipelineOpts{
		PhaseID:           "2",
		ImplAgent:         "codex",
		ReviewAgent:       "gemini",
		FixAgent:          "claude",
		ReviewConcurrency: 5,
		MaxReviewCycles:   4,
		SkipImplement:     true,
		SkipReview:        false,
		SkipFix:           true,
		SkipPR:            false,
		DryRun:            true,
	}
	flags := pipelineFlags{
		ReviewConcurrency: 2,
		MaxReviewCycles:   3,
	}
	applyWizardOpts(wizardOpts, &flags)

	assert.Equal(t, "2", flags.Phase)
	assert.Equal(t, "codex", flags.ImplAgent)
	assert.Equal(t, "gemini", flags.ReviewAgent)
	assert.Equal(t, "claude", flags.FixAgent)
	assert.Equal(t, 5, flags.ReviewConcurrency)
	assert.Equal(t, 4, flags.MaxReviewCycles)
	assert.True(t, flags.SkipImplement)
	assert.False(t, flags.SkipReview)
	assert.True(t, flags.SkipFix)
	assert.False(t, flags.SkipPR)
	assert.True(t, flags.DryRun)
}

func TestApplyWizardOpts_DoesNotOverrideExistingPhase(t *testing.T) {
	// When wizardOpts has empty PhaseID, existing flags.Phase must not be clobbered.
	wizardOpts := &pipeline.PipelineOpts{
		PhaseID:           "",
		ReviewConcurrency: 3,
		MaxReviewCycles:   2,
	}
	flags := pipelineFlags{Phase: "4", ReviewConcurrency: 2, MaxReviewCycles: 3}
	applyWizardOpts(wizardOpts, &flags)

	// Phase must remain 4 since wizard returned empty PhaseID.
	assert.Equal(t, "4", flags.Phase)
}

// ---- buildSkippedList tests -------------------------------------------------

func TestBuildSkippedList_None(t *testing.T) {
	opts := pipeline.PipelineOpts{}
	skipped := buildSkippedList(opts)
	assert.Empty(t, skipped)
}

func TestBuildSkippedList_All(t *testing.T) {
	opts := pipeline.PipelineOpts{
		SkipImplement: true,
		SkipReview:    true,
		SkipFix:       true,
		SkipPR:        true,
	}
	skipped := buildSkippedList(opts)
	assert.Len(t, skipped, 4)
	assert.Contains(t, skipped, "implement")
	assert.Contains(t, skipped, "review")
	assert.Contains(t, skipped, "fix")
	assert.Contains(t, skipped, "pr")
}

func TestBuildSkippedList_OnlyReview(t *testing.T) {
	opts := pipeline.PipelineOpts{SkipReview: true}
	skipped := buildSkippedList(opts)
	assert.Equal(t, []string{"review"}, skipped)
}

// ---- filterPhasesForDryRun tests --------------------------------------------

func TestFilterPhasesForDryRun_All(t *testing.T) {
	phases := []task.Phase{
		{ID: 1, Name: "Phase 1", StartTask: "T-001", EndTask: "T-010"},
		{ID: 2, Name: "Phase 2", StartTask: "T-011", EndTask: "T-020"},
		{ID: 3, Name: "Phase 3", StartTask: "T-021", EndTask: "T-030"},
	}
	opts := pipeline.PipelineOpts{PhaseID: ""}
	filtered, err := filterPhasesForDryRun(phases, opts)
	require.NoError(t, err)
	assert.Len(t, filtered, 3)
}

func TestFilterPhasesForDryRun_Single(t *testing.T) {
	phases := []task.Phase{
		{ID: 1, Name: "Phase 1", StartTask: "T-001", EndTask: "T-010"},
		{ID: 2, Name: "Phase 2", StartTask: "T-011", EndTask: "T-020"},
	}
	opts := pipeline.PipelineOpts{PhaseID: "2"}
	filtered, err := filterPhasesForDryRun(phases, opts)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, 2, filtered[0].ID)
}

func TestFilterPhasesForDryRun_FromPhase(t *testing.T) {
	phases := []task.Phase{
		{ID: 1, Name: "Phase 1", StartTask: "T-001", EndTask: "T-010"},
		{ID: 2, Name: "Phase 2", StartTask: "T-011", EndTask: "T-020"},
		{ID: 3, Name: "Phase 3", StartTask: "T-021", EndTask: "T-030"},
	}
	opts := pipeline.PipelineOpts{FromPhase: "2"}
	filtered, err := filterPhasesForDryRun(phases, opts)
	require.NoError(t, err)
	assert.Len(t, filtered, 2)
	assert.Equal(t, 2, filtered[0].ID)
	assert.Equal(t, 3, filtered[1].ID)
}

func TestFilterPhasesForDryRun_PhaseNotFound(t *testing.T) {
	phases := []task.Phase{
		{ID: 1, Name: "Phase 1", StartTask: "T-001", EndTask: "T-010"},
	}
	opts := pipeline.PipelineOpts{PhaseID: "99"}
	_, err := filterPhasesForDryRun(phases, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "phase 99 not found")
}

func TestFilterPhasesForDryRun_FromPhaseNoMatch(t *testing.T) {
	phases := []task.Phase{
		{ID: 1, Name: "Phase 1", StartTask: "T-001", EndTask: "T-010"},
	}
	opts := pipeline.PipelineOpts{FromPhase: "99"}
	_, err := filterPhasesForDryRun(phases, opts)
	require.Error(t, err)
}

func TestFilterPhasesForDryRun_InvalidPhaseID(t *testing.T) {
	phases := []task.Phase{{ID: 1}}
	opts := pipeline.PipelineOpts{PhaseID: "notanumber"}
	_, err := filterPhasesForDryRun(phases, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --phase")
}

// ---- phaseIDList tests ------------------------------------------------------

func TestPhaseIDList_MultiplePhases(t *testing.T) {
	phases := []task.Phase{
		{ID: 1}, {ID: 2}, {ID: 3},
	}
	result := phaseIDList(phases)
	assert.Equal(t, "1, 2, 3", result)
}

func TestPhaseIDList_SinglePhase(t *testing.T) {
	phases := []task.Phase{{ID: 5}}
	result := phaseIDList(phases)
	assert.Equal(t, "5", result)
}

func TestPhaseIDList_Empty(t *testing.T) {
	result := phaseIDList(nil)
	assert.Equal(t, "", result)
}

// ---- availableAgentNames tests ----------------------------------------------

func TestAvailableAgentNames(t *testing.T) {
	names := availableAgentNames()
	assert.Len(t, names, 3)
	assert.Contains(t, names, "claude")
	assert.Contains(t, names, "codex")
	assert.Contains(t, names, "gemini")
}

// ---- safeBranchRE tests -----------------------------------------------------

func TestSafeBranchRE(t *testing.T) {
	tests := []struct {
		branch string
		valid  bool
	}{
		{"main", true},
		{"feature/my-branch", true},
		{"release-1.0", true},
		{"raven/phase-1", true},
		{"feature_branch", true},
		{"my.branch.123", true},
		{"branch with spaces", false},
		{"branch@symbol", false},
		{"branch!exclaim", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			if tt.valid {
				assert.True(t, safeBranchRE.MatchString(tt.branch), "expected %q to be valid", tt.branch)
			} else {
				assert.False(t, safeBranchRE.MatchString(tt.branch), "expected %q to be invalid", tt.branch)
			}
		})
	}
}

// ---- safeBranchRE injection / security edge cases --------------------------

func TestSafeBranchRE_SecurityEdgeCases(t *testing.T) {
	// These cases come from the acceptance criteria: branch names containing
	// characters used for shell injection or command separation must be rejected.
	dangerousNames := []string{
		"branch;injection",
		"branch&&cmd",
		"branch||cmd",
		"branch`cmd`",
		"branch$(cmd)",
		"branch>redirect",
		"branch<redirect",
		"branch|pipe",
		"branch\ttab",
		"branch\nnewline",
		"branch\"quoted",
		"branch'single",
		"branch*glob",
		"branch?wildcard",
		"branch[bracket",
		"branch{brace",
		"branch#hash",
		"branch~tilde",
		"branch^caret",
		"branch:colon",
	}
	for _, name := range dangerousNames {
		t.Run(fmt.Sprintf("reject %q", name), func(t *testing.T) {
			assert.False(t, safeBranchRE.MatchString(name),
				"dangerous branch name %q should be rejected by safeBranchRE", name)
		})
	}
}

func TestSafeBranchRE_ValidBranchNames(t *testing.T) {
	// Additional valid names beyond those already tested in TestSafeBranchRE.
	validNames := []string{
		"v1.2.3",
		"release/2026-02-18",
		"hotfix/JIRA-1234",
		"user/feature-branch_123",
		"abc",
		"A",
		"Z9",
		"raven/phase-100",
	}
	for _, name := range validNames {
		t.Run(fmt.Sprintf("accept %q", name), func(t *testing.T) {
			assert.True(t, safeBranchRE.MatchString(name),
				"valid branch name %q should be accepted by safeBranchRE", name)
		})
	}
}

// ---- validatePipelineFlags additional edge cases ----------------------------

func TestValidatePipelineFlags_NegativeConcurrency(t *testing.T) {
	flags := pipelineFlags{
		Phase:             "1",
		ReviewConcurrency: -1,
		MaxReviewCycles:   3,
	}
	err := validatePipelineFlags(flags, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--review-concurrency must be >= 1")
	assert.Contains(t, err.Error(), "-1")
}

func TestValidatePipelineFlags_NegativeMaxCycles(t *testing.T) {
	flags := pipelineFlags{
		Phase:             "1",
		ReviewConcurrency: 2,
		MaxReviewCycles:   -1,
	}
	err := validatePipelineFlags(flags, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--max-review-cycles must be >= 1")
	assert.Contains(t, err.Error(), "-1")
}

func TestValidatePipelineFlags_PartialSkipCombinations(t *testing.T) {
	// Any combination of fewer than four skips must not return the "all skipped"
	// error. This table tests every combination that leaves at least one active.
	tests := []struct {
		name          string
		skipImplement bool
		skipReview    bool
		skipFix       bool
		skipPR        bool
	}{
		{"only implement skipped", true, false, false, false},
		{"only review skipped", false, true, false, false},
		{"only fix skipped", false, false, true, false},
		{"only pr skipped", false, false, false, true},
		{"implement+review skipped", true, true, false, false},
		{"implement+fix skipped", true, false, true, false},
		{"implement+pr skipped", true, false, false, true},
		{"review+fix skipped", false, true, true, false},
		{"review+pr skipped", false, true, false, true},
		{"fix+pr skipped", false, false, true, true},
		{"implement+review+fix skipped", true, true, true, false},
		{"implement+review+pr skipped", true, true, false, true},
		{"implement+fix+pr skipped", true, false, true, true},
		{"review+fix+pr skipped", false, true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := pipelineFlags{
				Phase:             "1",
				ReviewConcurrency: 2,
				MaxReviewCycles:   3,
				SkipImplement:     tt.skipImplement,
				SkipReview:        tt.skipReview,
				SkipFix:           tt.skipFix,
				SkipPR:            tt.skipPR,
			}
			err := validatePipelineFlags(flags, nil)
			// The only error we must NOT see is the "all stages skipped" error.
			if err != nil {
				assert.NotContains(t, err.Error(), "all pipeline stages are skipped",
					"partial skip combination %q should not trigger all-skipped error", tt.name)
			}
		})
	}
}

func TestValidatePipelineFlags_NoSkipsIsValid(t *testing.T) {
	flags := pipelineFlags{
		Phase:             "1",
		ReviewConcurrency: 1,
		MaxReviewCycles:   1,
	}
	err := validatePipelineFlags(flags, nil)
	assert.NoError(t, err)
}

func TestValidatePipelineFlags_MinimumValidValues(t *testing.T) {
	// Boundary: both numeric flags at minimum value (1) must be valid.
	flags := pipelineFlags{
		Phase:             "1",
		ReviewConcurrency: 1,
		MaxReviewCycles:   1,
	}
	err := validatePipelineFlags(flags, nil)
	assert.NoError(t, err)
}

func TestValidatePipelineFlags_FromPhaseNonNumeric(t *testing.T) {
	flags := pipelineFlags{
		FromPhase:         "two",
		ReviewConcurrency: 2,
		MaxReviewCycles:   3,
	}
	err := validatePipelineFlags(flags, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--from-phase")
}

func TestValidatePipelineFlags_FromPhaseZeroInvalid(t *testing.T) {
	flags := pipelineFlags{
		FromPhase:         "0",
		ReviewConcurrency: 2,
		MaxReviewCycles:   3,
	}
	err := validatePipelineFlags(flags, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--from-phase")
}

func TestValidatePipelineFlags_ValidGeminiAgent(t *testing.T) {
	flags := pipelineFlags{
		Phase:             "1",
		ReviewConcurrency: 2,
		MaxReviewCycles:   3,
		ImplAgent:         "gemini",
		ReviewAgent:       "gemini",
		FixAgent:          "gemini",
	}
	err := validatePipelineFlags(flags, nil)
	assert.NoError(t, err)
}

func TestValidatePipelineFlags_PhaseAllCaseVariants(t *testing.T) {
	// "all" in any case must be valid as a phase value.
	tests := []struct {
		name     string
		phaseStr string
	}{
		{"lowercase", "all"},
		{"uppercase", "ALL"},
		{"mixed", "All"},
		{"mixed2", "aLl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := pipelineFlags{
				Phase:             tt.phaseStr,
				ReviewConcurrency: 2,
				MaxReviewCycles:   3,
			}
			err := validatePipelineFlags(flags, nil)
			assert.NoError(t, err, "phase %q should be valid", tt.phaseStr)
		})
	}
}

func TestValidatePipelineFlags_PhaseFoundInPhases(t *testing.T) {
	// When phases list is provided, a phase that exists must succeed.
	phases := []task.Phase{
		{ID: 1, Name: "Foundation", StartTask: "T-001", EndTask: "T-010"},
		{ID: 2, Name: "Core", StartTask: "T-011", EndTask: "T-020"},
		{ID: 3, Name: "Integration", StartTask: "T-021", EndTask: "T-030"},
	}
	for _, ph := range phases {
		flags := pipelineFlags{
			Phase:             fmt.Sprintf("%d", ph.ID),
			ReviewConcurrency: 2,
			MaxReviewCycles:   3,
		}
		err := validatePipelineFlags(flags, phases)
		assert.NoError(t, err, "phase %d should be found in the phases list", ph.ID)
	}
}

func TestValidatePipelineFlags_PhaseNotFoundShowsAvailable(t *testing.T) {
	// The error message for a missing phase must list available phase IDs.
	phases := []task.Phase{
		{ID: 2, Name: "Core", StartTask: "T-011", EndTask: "T-020"},
		{ID: 5, Name: "Polish", StartTask: "T-041", EndTask: "T-050"},
	}
	flags := pipelineFlags{
		Phase:             "99",
		ReviewConcurrency: 2,
		MaxReviewCycles:   3,
	}
	err := validatePipelineFlags(flags, phases)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2", "error should mention available phase 2")
	assert.Contains(t, err.Error(), "5", "error should mention available phase 5")
}

// ---- buildPipelineOpts additional cases ------------------------------------

func TestBuildPipelineOpts_DefaultEmptyFlags(t *testing.T) {
	// A zero-value pipelineFlags must map to a PipelineOpts without panicking.
	flags := pipelineFlags{}
	opts := buildPipelineOpts(flags)

	assert.Equal(t, "", opts.PhaseID)
	assert.Equal(t, "", opts.FromPhase)
	assert.Equal(t, "", opts.ImplAgent)
	assert.Equal(t, "", opts.ReviewAgent)
	assert.Equal(t, "", opts.FixAgent)
	assert.Equal(t, 0, opts.ReviewConcurrency)
	assert.Equal(t, 0, opts.MaxReviewCycles)
	assert.False(t, opts.SkipImplement)
	assert.False(t, opts.SkipReview)
	assert.False(t, opts.SkipFix)
	assert.False(t, opts.SkipPR)
	assert.False(t, opts.DryRun)
	assert.False(t, opts.Interactive)
}

func TestBuildPipelineOpts_SkipFlagsPreserved(t *testing.T) {
	// Each skip flag must be independently mapped.
	tests := []struct {
		name         string
		flags        pipelineFlags
		wantSkipImpl bool
		wantSkipRev  bool
		wantSkipFix  bool
		wantSkipPR   bool
	}{
		{
			name:         "only skip-implement",
			flags:        pipelineFlags{SkipImplement: true},
			wantSkipImpl: true,
		},
		{
			name:        "only skip-review",
			flags:       pipelineFlags{SkipReview: true},
			wantSkipRev: true,
		},
		{
			name:        "only skip-fix",
			flags:       pipelineFlags{SkipFix: true},
			wantSkipFix: true,
		},
		{
			name:       "only skip-pr",
			flags:      pipelineFlags{SkipPR: true},
			wantSkipPR: true,
		},
		{
			name:         "all skip flags true",
			flags:        pipelineFlags{SkipImplement: true, SkipReview: true, SkipFix: true, SkipPR: true},
			wantSkipImpl: true,
			wantSkipRev:  true,
			wantSkipFix:  true,
			wantSkipPR:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := buildPipelineOpts(tt.flags)
			assert.Equal(t, tt.wantSkipImpl, opts.SkipImplement)
			assert.Equal(t, tt.wantSkipRev, opts.SkipReview)
			assert.Equal(t, tt.wantSkipFix, opts.SkipFix)
			assert.Equal(t, tt.wantSkipPR, opts.SkipPR)
		})
	}
}

func TestBuildPipelineOpts_InteractivePreserved(t *testing.T) {
	flags := pipelineFlags{
		Phase:       "1",
		Interactive: true,
	}
	opts := buildPipelineOpts(flags)
	assert.True(t, opts.Interactive)
}

func TestBuildPipelineOpts_DryRunPreserved(t *testing.T) {
	flags := pipelineFlags{
		Phase:  "1",
		DryRun: true,
	}
	opts := buildPipelineOpts(flags)
	assert.True(t, opts.DryRun)
}

func TestBuildPipelineOpts_PhaseNumericPreserved(t *testing.T) {
	// A numeric phase ID other than "all" must be preserved as-is.
	for _, id := range []string{"1", "2", "10", "99"} {
		flags := pipelineFlags{Phase: id}
		opts := buildPipelineOpts(flags)
		assert.Equal(t, id, opts.PhaseID, "numeric phase %q must be preserved", id)
	}
}

func TestBuildPipelineOpts_AllVariantsNormalised(t *testing.T) {
	// All case variants of "all" must normalise to "".
	for _, phaseStr := range []string{"all", "ALL", "All", "aLL", "ALl"} {
		flags := pipelineFlags{Phase: phaseStr}
		opts := buildPipelineOpts(flags)
		assert.Equal(t, "", opts.PhaseID,
			"Phase %q must be normalised to empty string", phaseStr)
	}
}

// ---- applyWizardOpts additional cases ---------------------------------------

func TestApplyWizardOpts_FromPhaseOverride(t *testing.T) {
	// When wizard returns a FromPhase, it must be applied.
	wizardOpts := &pipeline.PipelineOpts{
		FromPhase:         "3",
		ReviewConcurrency: 2,
		MaxReviewCycles:   2,
	}
	flags := pipelineFlags{ReviewConcurrency: 2, MaxReviewCycles: 3}
	applyWizardOpts(wizardOpts, &flags)

	assert.Equal(t, "3", flags.FromPhase)
	// Phase must remain empty since wizard didn't set it.
	assert.Equal(t, "", flags.Phase)
}

func TestApplyWizardOpts_ZeroConcurrencyDoesNotOverride(t *testing.T) {
	// When wizard returns ReviewConcurrency == 0, the existing flag value must
	// not be overwritten (zero is not a valid concurrency value).
	wizardOpts := &pipeline.PipelineOpts{
		ReviewConcurrency: 0,
		MaxReviewCycles:   0,
	}
	flags := pipelineFlags{ReviewConcurrency: 4, MaxReviewCycles: 5}
	applyWizardOpts(wizardOpts, &flags)

	assert.Equal(t, 4, flags.ReviewConcurrency, "zero wizard concurrency must not overwrite existing value")
	assert.Equal(t, 5, flags.MaxReviewCycles, "zero wizard max-cycles must not overwrite existing value")
}

func TestApplyWizardOpts_DryRunFalseDoesNotClearTrue(t *testing.T) {
	// applyWizardOpts only sets DryRun to true; it must not reset it to false.
	wizardOpts := &pipeline.PipelineOpts{DryRun: false}
	flags := pipelineFlags{DryRun: true}
	applyWizardOpts(wizardOpts, &flags)
	assert.True(t, flags.DryRun, "DryRun must not be reset to false by wizard returning false")
}

func TestApplyWizardOpts_DryRunTrueSetsFlag(t *testing.T) {
	wizardOpts := &pipeline.PipelineOpts{DryRun: true}
	flags := pipelineFlags{DryRun: false}
	applyWizardOpts(wizardOpts, &flags)
	assert.True(t, flags.DryRun, "DryRun must be set to true when wizard returns true")
}

func TestApplyWizardOpts_EmptyAgentsDoNotOverride(t *testing.T) {
	// When wizard returns empty agent strings, the existing CLI flag values
	// must be preserved.
	wizardOpts := &pipeline.PipelineOpts{
		ImplAgent:         "",
		ReviewAgent:       "",
		FixAgent:          "",
		ReviewConcurrency: 2,
		MaxReviewCycles:   2,
	}
	flags := pipelineFlags{
		ImplAgent:         "claude",
		ReviewAgent:       "codex",
		FixAgent:          "gemini",
		ReviewConcurrency: 2,
		MaxReviewCycles:   3,
	}
	applyWizardOpts(wizardOpts, &flags)

	assert.Equal(t, "claude", flags.ImplAgent, "empty wizard ImplAgent must not override existing value")
	assert.Equal(t, "codex", flags.ReviewAgent, "empty wizard ReviewAgent must not override existing value")
	assert.Equal(t, "gemini", flags.FixAgent, "empty wizard FixAgent must not override existing value")
}

func TestApplyWizardOpts_SkipFlagsAlwaysOverwritten(t *testing.T) {
	// Skip flags are always applied from wizard output, even when false.
	// If wizard returns false, the flag must be false afterwards regardless of
	// the previous CLI value.
	wizardOpts := &pipeline.PipelineOpts{
		SkipImplement:     false,
		SkipReview:        false,
		SkipFix:           false,
		SkipPR:            false,
		ReviewConcurrency: 2,
		MaxReviewCycles:   2,
	}
	flags := pipelineFlags{
		SkipImplement:     true,
		SkipReview:        true,
		SkipFix:           true,
		SkipPR:            true,
		ReviewConcurrency: 2,
		MaxReviewCycles:   3,
	}
	applyWizardOpts(wizardOpts, &flags)

	assert.False(t, flags.SkipImplement, "wizard false must overwrite CLI true for SkipImplement")
	assert.False(t, flags.SkipReview, "wizard false must overwrite CLI true for SkipReview")
	assert.False(t, flags.SkipFix, "wizard false must overwrite CLI true for SkipFix")
	assert.False(t, flags.SkipPR, "wizard false must overwrite CLI true for SkipPR")
}

// ---- buildSkippedList additional cases -------------------------------------

func TestBuildSkippedList_OrderIsImplementReviewFixPR(t *testing.T) {
	// The canonical order must be: implement, review, fix, pr.
	opts := pipeline.PipelineOpts{
		SkipImplement: true,
		SkipReview:    true,
		SkipFix:       true,
		SkipPR:        true,
	}
	skipped := buildSkippedList(opts)
	require.Len(t, skipped, 4)
	assert.Equal(t, "implement", skipped[0])
	assert.Equal(t, "review", skipped[1])
	assert.Equal(t, "fix", skipped[2])
	assert.Equal(t, "pr", skipped[3])
}

func TestBuildSkippedList_ImplementAndFix(t *testing.T) {
	opts := pipeline.PipelineOpts{SkipImplement: true, SkipFix: true}
	skipped := buildSkippedList(opts)
	require.Len(t, skipped, 2)
	assert.Equal(t, "implement", skipped[0])
	assert.Equal(t, "fix", skipped[1])
}

func TestBuildSkippedList_ReviewAndPR(t *testing.T) {
	opts := pipeline.PipelineOpts{SkipReview: true, SkipPR: true}
	skipped := buildSkippedList(opts)
	require.Len(t, skipped, 2)
	assert.Equal(t, "review", skipped[0])
	assert.Equal(t, "pr", skipped[1])
}

func TestBuildSkippedList_OnlyImplement(t *testing.T) {
	opts := pipeline.PipelineOpts{SkipImplement: true}
	skipped := buildSkippedList(opts)
	assert.Equal(t, []string{"implement"}, skipped)
}

func TestBuildSkippedList_OnlyFix(t *testing.T) {
	opts := pipeline.PipelineOpts{SkipFix: true}
	skipped := buildSkippedList(opts)
	assert.Equal(t, []string{"fix"}, skipped)
}

func TestBuildSkippedList_OnlyPR(t *testing.T) {
	opts := pipeline.PipelineOpts{SkipPR: true}
	skipped := buildSkippedList(opts)
	assert.Equal(t, []string{"pr"}, skipped)
}

// ---- filterPhasesForDryRun additional cases --------------------------------

func TestFilterPhasesForDryRun_PhaseAllString(t *testing.T) {
	// PhaseID "all" (not empty) must also return all phases.
	phases := []task.Phase{
		{ID: 1, Name: "Phase 1", StartTask: "T-001", EndTask: "T-010"},
		{ID: 2, Name: "Phase 2", StartTask: "T-011", EndTask: "T-020"},
	}
	opts := pipeline.PipelineOpts{PhaseID: "all"}
	filtered, err := filterPhasesForDryRun(phases, opts)
	require.NoError(t, err)
	assert.Len(t, filtered, 2)
}

func TestFilterPhasesForDryRun_PhaseAllCaseInsensitive(t *testing.T) {
	phases := []task.Phase{
		{ID: 1, Name: "Phase 1", StartTask: "T-001", EndTask: "T-010"},
	}
	for _, phaseStr := range []string{"ALL", "All", "aLL"} {
		opts := pipeline.PipelineOpts{PhaseID: phaseStr}
		filtered, err := filterPhasesForDryRun(phases, opts)
		require.NoError(t, err, "PhaseID %q should be treated as all", phaseStr)
		assert.Len(t, filtered, 1)
	}
}

func TestFilterPhasesForDryRun_FromPhaseInvalidNonNumeric(t *testing.T) {
	phases := []task.Phase{
		{ID: 1, Name: "Phase 1", StartTask: "T-001", EndTask: "T-010"},
	}
	opts := pipeline.PipelineOpts{FromPhase: "abc"}
	_, err := filterPhasesForDryRun(phases, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --from-phase")
}

func TestFilterPhasesForDryRun_EmptyPhasesList(t *testing.T) {
	// Empty phases with empty PhaseID must return empty slice, no error.
	opts := pipeline.PipelineOpts{PhaseID: ""}
	filtered, err := filterPhasesForDryRun(nil, opts)
	require.NoError(t, err)
	assert.Empty(t, filtered)
}

func TestFilterPhasesForDryRun_FromPhaseAtExactBoundary(t *testing.T) {
	phases := []task.Phase{
		{ID: 1, Name: "Phase 1", StartTask: "T-001", EndTask: "T-010"},
		{ID: 2, Name: "Phase 2", StartTask: "T-011", EndTask: "T-020"},
		{ID: 3, Name: "Phase 3", StartTask: "T-021", EndTask: "T-030"},
	}
	// FromPhase exactly equal to the last phase ID.
	opts := pipeline.PipelineOpts{FromPhase: "3"}
	filtered, err := filterPhasesForDryRun(phases, opts)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, 3, filtered[0].ID)
}

func TestFilterPhasesForDryRun_SinglePhaseFromList(t *testing.T) {
	phases := []task.Phase{
		{ID: 1, Name: "Phase 1", StartTask: "T-001", EndTask: "T-010"},
		{ID: 2, Name: "Phase 2", StartTask: "T-011", EndTask: "T-020"},
		{ID: 3, Name: "Phase 3", StartTask: "T-021", EndTask: "T-030"},
	}
	for _, ph := range phases {
		opts := pipeline.PipelineOpts{PhaseID: fmt.Sprintf("%d", ph.ID)}
		filtered, err := filterPhasesForDryRun(phases, opts)
		require.NoError(t, err)
		require.Len(t, filtered, 1)
		assert.Equal(t, ph.ID, filtered[0].ID)
	}
}

// ---- printPipelineSummary tests ---------------------------------------------

// newTestCmd returns a minimal cobra.Command with its output redirected to buf.
// t.Helper is called so failures point to the actual test line.
func newTestCmd(t *testing.T, buf *bytes.Buffer) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.SetOut(buf)
	return cmd
}

func TestPrintPipelineSummary_SinglePhaseCompleted(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newTestCmd(t, buf)

	result := &pipeline.PipelineResult{
		Status:        pipeline.PipelineStatusCompleted,
		TotalDuration: 5 * time.Second,
		Phases: []pipeline.PhaseResult{
			{
				PhaseID:    "1",
				PhaseName:  "Foundation",
				Status:     pipeline.PhaseStatusCompleted,
				BranchName: "raven/phase-1",
			},
		},
	}

	printPipelineSummary(cmd, result)

	output := buf.String()
	assert.Contains(t, output, "completed", "summary should contain pipeline status")
	assert.Contains(t, output, "Phase", "summary should mention Phase")
	assert.Contains(t, output, "1", "summary should contain phase ID")
	assert.Contains(t, output, "raven/phase-1", "summary should contain branch name")
}

func TestPrintPipelineSummary_ShowsPRURL(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newTestCmd(t, buf)

	result := &pipeline.PipelineResult{
		Status:        pipeline.PipelineStatusCompleted,
		TotalDuration: 10 * time.Second,
		Phases: []pipeline.PhaseResult{
			{
				PhaseID:    "1",
				Status:     pipeline.PhaseStatusCompleted,
				BranchName: "raven/phase-1",
				PRURL:      "https://github.com/org/repo/pull/42",
			},
		},
	}

	printPipelineSummary(cmd, result)

	output := buf.String()
	assert.Contains(t, output, "https://github.com/org/repo/pull/42",
		"summary must include the PR URL when set")
}

func TestPrintPipelineSummary_ShowsErrorMessage(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newTestCmd(t, buf)

	result := &pipeline.PipelineResult{
		Status:        pipeline.PipelineStatusFailed,
		TotalDuration: 2 * time.Second,
		Phases: []pipeline.PhaseResult{
			{
				PhaseID:    "1",
				Status:     pipeline.PhaseStatusFailed,
				BranchName: "raven/phase-1",
				Error:      "workflow engine: step run_implement failed",
			},
		},
	}

	printPipelineSummary(cmd, result)

	output := buf.String()
	assert.Contains(t, output, "workflow engine: step run_implement failed",
		"summary must include the error message when set")
}

func TestPrintPipelineSummary_MultiplePhases(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newTestCmd(t, buf)

	result := &pipeline.PipelineResult{
		Status:        pipeline.PipelineStatusPartial,
		TotalDuration: 30 * time.Second,
		Phases: []pipeline.PhaseResult{
			{
				PhaseID:    "1",
				Status:     pipeline.PhaseStatusCompleted,
				BranchName: "raven/phase-1",
				PRURL:      "https://github.com/org/repo/pull/1",
			},
			{
				PhaseID:    "2",
				Status:     pipeline.PhaseStatusFailed,
				BranchName: "raven/phase-2",
				Error:      "git checkout failed",
			},
			{
				PhaseID:    "3",
				Status:     pipeline.PhaseStatusPending,
				BranchName: "raven/phase-3",
			},
		},
	}

	printPipelineSummary(cmd, result)

	output := buf.String()
	assert.Contains(t, output, "partial", "output should include partial status")
	assert.Contains(t, output, "raven/phase-1", "output should list phase 1 branch")
	assert.Contains(t, output, "raven/phase-2", "output should list phase 2 branch")
	assert.Contains(t, output, "raven/phase-3", "output should list phase 3 branch")
	assert.Contains(t, output, "https://github.com/org/repo/pull/1", "output should show phase 1 PR URL")
	assert.Contains(t, output, "git checkout failed", "output should show phase 2 error")
}

func TestPrintPipelineSummary_DurationRounded(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newTestCmd(t, buf)

	result := &pipeline.PipelineResult{
		Status:        pipeline.PipelineStatusCompleted,
		TotalDuration: 5*time.Second + 123*time.Millisecond,
		Phases:        []pipeline.PhaseResult{},
	}

	printPipelineSummary(cmd, result)

	output := buf.String()
	// Should contain the duration in some form (rounded to 1ms).
	assert.Contains(t, output, "duration:", "output should include duration label")
}

func TestPrintPipelineSummary_NoPROrErrorOmitted(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newTestCmd(t, buf)

	result := &pipeline.PipelineResult{
		Status:        pipeline.PipelineStatusCompleted,
		TotalDuration: 1 * time.Second,
		Phases: []pipeline.PhaseResult{
			{
				PhaseID:    "1",
				Status:     pipeline.PhaseStatusCompleted,
				BranchName: "raven/phase-1",
				// No PRURL, no Error.
			},
		},
	}

	printPipelineSummary(cmd, result)

	output := buf.String()
	// "PR:" and "Error:" tokens must not appear when empty.
	assert.NotContains(t, output, "PR:", "summary must omit PR label when PRURL is empty")
	assert.NotContains(t, output, "Error:", "summary must omit Error label when Error is empty")
}

func TestPrintPipelineSummary_SeparatorLinePresent(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newTestCmd(t, buf)

	result := &pipeline.PipelineResult{
		Status:        pipeline.PipelineStatusCompleted,
		TotalDuration: 1 * time.Second,
		Phases:        []pipeline.PhaseResult{},
	}

	printPipelineSummary(cmd, result)

	output := buf.String()
	// The implementation prints a separator line of 60 dashes.
	assert.Contains(t, output, strings.Repeat("-", 60),
		"summary should contain the 60-dash separator line")
}

// ---- phaseIDList additional cases ------------------------------------------

func TestPhaseIDList_NonSequentialIDs(t *testing.T) {
	phases := []task.Phase{
		{ID: 1}, {ID: 5}, {ID: 10},
	}
	result := phaseIDList(phases)
	assert.Equal(t, "1, 5, 10", result)
}

func TestPhaseIDList_SingleLargeID(t *testing.T) {
	phases := []task.Phase{{ID: 100}}
	result := phaseIDList(phases)
	assert.Equal(t, "100", result)
}

// ---- newPipelineCmd structural tests ----------------------------------------

func TestNewPipelineCmd_ArgsNoArgs(t *testing.T) {
	// The command must be configured to accept no positional arguments.
	cmd := newPipelineCmd()
	// cobra.NoArgs: passing any positional arg must error.
	cmd.SetArgs([]string{"unexpected-arg"})
	// We can't run the full RunE (requires config), but we can test the Args
	// validator directly via the exported ValidateArgs method.
	err := cmd.ValidateArgs([]string{"unexpected-arg"})
	assert.Error(t, err, "command should reject positional arguments")
}

func TestNewPipelineCmd_HelpContainsKeyInformation(t *testing.T) {
	cmd := newPipelineCmd()
	combined := cmd.Long + "\n" + cmd.Example
	assert.Contains(t, combined, "--phase", "help must mention --phase flag")
	assert.Contains(t, combined, "--from-phase", "help must mention --from-phase flag")
	assert.Contains(t, combined, "--dry-run", "help must mention --dry-run flag")
	assert.Contains(t, combined, "--interactive", "help must mention --interactive flag")
}

func TestNewPipelineCmd_ExitCodeDocumentation(t *testing.T) {
	// The Long description must document exit codes 0, 1, 2, 3.
	cmd := newPipelineCmd()
	for _, code := range []string{"0", "1", "2", "3"} {
		assert.Contains(t, cmd.Long, code,
			"Long description should document exit code %s", code)
	}
}

// ---- isAllStagesSkipped logic (exercised through validatePipelineFlags) -----

// TestAllStagesSkipped_TableDriven verifies the inline "all stages skipped"
// logic inside validatePipelineFlags across every relevant combination.
func TestAllStagesSkipped_TableDriven(t *testing.T) {
	type skipCombo struct {
		impl, review, fix, pr bool
		wantAllSkipped        bool
	}
	tests := []skipCombo{
		{true, true, true, true, true},
		{false, true, true, true, false},
		{true, false, true, true, false},
		{true, true, false, true, false},
		{true, true, true, false, false},
		{false, false, false, false, false},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("impl=%v review=%v fix=%v pr=%v", tt.impl, tt.review, tt.fix, tt.pr)
		t.Run(name, func(t *testing.T) {
			flags := pipelineFlags{
				Phase:             "1",
				ReviewConcurrency: 2,
				MaxReviewCycles:   3,
				SkipImplement:     tt.impl,
				SkipReview:        tt.review,
				SkipFix:           tt.fix,
				SkipPR:            tt.pr,
			}
			err := validatePipelineFlags(flags, nil)
			if tt.wantAllSkipped {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "all pipeline stages are skipped")
			} else {
				// It is possible other errors exist (e.g. from agent names), but
				// the specific "all stages" error must NOT be present.
				if err != nil {
					assert.NotContains(t, err.Error(), "all pipeline stages are skipped")
				}
			}
		})
	}
}

// ---- pipeline command flag interaction smoke tests --------------------------

// TestNewPipelineCmd_FlagParsing exercises cobra flag parsing (not the full
// RunE which requires a real project) by using cmd.ParseFlags.
func TestNewPipelineCmd_FlagParsing(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		checkFn func(t *testing.T, cmd *cobra.Command)
	}{
		{
			name: "phase flag parsed",
			args: []string{"--phase", "3"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				f := cmd.Flags().Lookup("phase")
				require.NotNil(t, f)
				assert.Equal(t, "3", f.Value.String())
			},
		},
		{
			name: "from-phase flag parsed",
			args: []string{"--from-phase", "2"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				f := cmd.Flags().Lookup("from-phase")
				require.NotNil(t, f)
				assert.Equal(t, "2", f.Value.String())
			},
		},
		{
			name: "review-concurrency flag parsed",
			args: []string{"--review-concurrency", "5"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				f := cmd.Flags().Lookup("review-concurrency")
				require.NotNil(t, f)
				assert.Equal(t, "5", f.Value.String())
			},
		},
		{
			name: "max-review-cycles flag parsed",
			args: []string{"--max-review-cycles", "4"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				f := cmd.Flags().Lookup("max-review-cycles")
				require.NotNil(t, f)
				assert.Equal(t, "4", f.Value.String())
			},
		},
		{
			name: "skip-implement flag parsed",
			args: []string{"--skip-implement"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				f := cmd.Flags().Lookup("skip-implement")
				require.NotNil(t, f)
				assert.Equal(t, "true", f.Value.String())
			},
		},
		{
			name: "skip-review flag parsed",
			args: []string{"--skip-review"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				f := cmd.Flags().Lookup("skip-review")
				require.NotNil(t, f)
				assert.Equal(t, "true", f.Value.String())
			},
		},
		{
			name: "skip-fix flag parsed",
			args: []string{"--skip-fix"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				f := cmd.Flags().Lookup("skip-fix")
				require.NotNil(t, f)
				assert.Equal(t, "true", f.Value.String())
			},
		},
		{
			name: "skip-pr flag parsed",
			args: []string{"--skip-pr"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				f := cmd.Flags().Lookup("skip-pr")
				require.NotNil(t, f)
				assert.Equal(t, "true", f.Value.String())
			},
		},
		{
			name: "base flag parsed",
			args: []string{"--base", "develop"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				f := cmd.Flags().Lookup("base")
				require.NotNil(t, f)
				assert.Equal(t, "develop", f.Value.String())
			},
		},
		{
			name: "sync-base flag parsed",
			args: []string{"--sync-base"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				f := cmd.Flags().Lookup("sync-base")
				require.NotNil(t, f)
				assert.Equal(t, "true", f.Value.String())
			},
		},
		{
			name: "dry-run flag parsed",
			args: []string{"--dry-run"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				f := cmd.Flags().Lookup("dry-run")
				require.NotNil(t, f)
				assert.Equal(t, "true", f.Value.String())
			},
		},
		{
			name: "interactive flag parsed",
			args: []string{"--interactive"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				f := cmd.Flags().Lookup("interactive")
				require.NotNil(t, f)
				assert.Equal(t, "true", f.Value.String())
			},
		},
		{
			name: "impl-agent flag parsed",
			args: []string{"--impl-agent", "codex"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				f := cmd.Flags().Lookup("impl-agent")
				require.NotNil(t, f)
				assert.Equal(t, "codex", f.Value.String())
			},
		},
		{
			name: "review-agent flag parsed",
			args: []string{"--review-agent", "gemini"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				f := cmd.Flags().Lookup("review-agent")
				require.NotNil(t, f)
				assert.Equal(t, "gemini", f.Value.String())
			},
		},
		{
			name: "fix-agent flag parsed",
			args: []string{"--fix-agent", "claude"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				f := cmd.Flags().Lookup("fix-agent")
				require.NotNil(t, f)
				assert.Equal(t, "claude", f.Value.String())
			},
		},
		{
			name: "phase all flag parsed",
			args: []string{"--phase", "all"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				f := cmd.Flags().Lookup("phase")
				require.NotNil(t, f)
				assert.Equal(t, "all", f.Value.String())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newPipelineCmd()
			err := cmd.ParseFlags(tt.args)
			require.NoError(t, err, "ParseFlags must not fail for valid flags")
			tt.checkFn(t, cmd)
		})
	}
}

// ---- buildPipelineOpts: phase "all" to PipelineOpts mapping -----------------

func TestBuildPipelineOpts_PhaseAllInOptsPhaseIDEmpty(t *testing.T) {
	// After buildPipelineOpts, PhaseID "all" must become "" (the orchestrator
	// treats "" as "all phases").
	flags := pipelineFlags{Phase: "all", ReviewConcurrency: 2, MaxReviewCycles: 3}
	opts := buildPipelineOpts(flags)
	assert.Empty(t, opts.PhaseID, `"all" must normalise to empty PhaseID in PipelineOpts`)
}

// ---- Race detector smoke test ----------------------------------------------

func TestValidatePipelineFlags_Concurrency(t *testing.T) {
	// Run validatePipelineFlags in multiple goroutines to expose any races.
	// The function is pure (no shared mutable state), so this should always
	// pass the race detector.
	const goroutines = 20
	phases := []task.Phase{
		{ID: 1, Name: "Phase 1", StartTask: "T-001", EndTask: "T-010"},
	}

	done := make(chan struct{}, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			flags := pipelineFlags{
				Phase:             "1",
				ReviewConcurrency: 2 + i%3,
				MaxReviewCycles:   1 + i%4,
			}
			_ = validatePipelineFlags(flags, phases)
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < goroutines; i++ {
		<-done
	}
}
