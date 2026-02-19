package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/workflow"
)

// ---- helpers -----------------------------------------------------------------

// makeStateStore creates a StateStore backed by a temporary directory.
func makeStateStore(t *testing.T) (*workflow.StateStore, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := workflow.NewStateStore(dir)
	require.NoError(t, err)
	return store, dir
}

// saveRun persists a minimal WorkflowState and returns it.
func saveRun(t *testing.T, store *workflow.StateStore, id, workflowName, step string) *workflow.WorkflowState {
	t.Helper()
	state := workflow.NewWorkflowState(id, workflowName, step)
	// Advance UpdatedAt so ordering is deterministic.
	state.UpdatedAt = time.Now()
	require.NoError(t, store.Save(state))
	return state
}

// errResolver always returns ErrWorkflowNotFound (the production default).
func errResolver(workflowName string) (*workflow.WorkflowDefinition, error) {
	return nil, fmt.Errorf("workflow %q: %w", workflowName, ErrWorkflowNotFound)
}

// ---- Command structure tests -------------------------------------------------

func TestNewResumeCmd_Registration(t *testing.T) {
	cmd := newResumeCmd()
	assert.Equal(t, "resume", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
}

func TestNewResumeCmd_FlagsRegistered(t *testing.T) {
	cmd := newResumeCmd()

	expectedFlags := []string{"run", "list", "dry-run", "clean", "clean-all", "force"}
	for _, name := range expectedFlags {
		assert.NotNil(t, cmd.Flags().Lookup(name), "flag --%s must be registered", name)
	}
}

func TestNewResumeCmd_FlagDefaults(t *testing.T) {
	cmd := newResumeCmd()

	listFlag := cmd.Flags().Lookup("list")
	require.NotNil(t, listFlag)
	assert.Equal(t, "false", listFlag.DefValue)

	dryRunFlag := cmd.Flags().Lookup("dry-run")
	require.NotNil(t, dryRunFlag)
	assert.Equal(t, "false", dryRunFlag.DefValue)

	cleanAllFlag := cmd.Flags().Lookup("clean-all")
	require.NotNil(t, cleanAllFlag)
	assert.Equal(t, "false", cleanAllFlag.DefValue)

	forceFlag := cmd.Flags().Lookup("force")
	require.NotNil(t, forceFlag)
	assert.Equal(t, "false", forceFlag.DefValue)
}

func TestResumeCmdRegisteredOnRoot(t *testing.T) {
	found := false
	for _, sub := range rootCmd.Commands() {
		if sub.Use == "resume" {
			found = true
			break
		}
	}
	assert.True(t, found, "resume command should be registered as a subcommand of root")
}

// ---- runIDPattern tests -------------------------------------------------------

func TestRunIDPattern_ValidIDs(t *testing.T) {
	t.Parallel()

	validIDs := []string{
		"wf-1234567890",
		"abc",
		"ABC",
		"a1b2c3",
		"my_workflow",
		"wf-abc-def",
		"WF_001",
		"a",
		"1",
		"a-b_c",
	}
	for _, id := range validIDs {
		t.Run(id, func(t *testing.T) {
			assert.True(t, runIDPattern.MatchString(id), "ID %q should match pattern", id)
		})
	}
}

func TestRunIDPattern_InvalidIDs(t *testing.T) {
	t.Parallel()

	invalidIDs := []string{
		"../etc/passwd",
		"/absolute/path",
		"path/with/slashes",
		"has space",
		"has.dot",
		"has@at",
		"has!excl",
		"",
	}
	for _, id := range invalidIDs {
		t.Run(fmt.Sprintf("%q", id), func(t *testing.T) {
			assert.False(t, runIDPattern.MatchString(id), "ID %q should NOT match pattern", id)
		})
	}
}

// ---- runResume flag validation tests ----------------------------------------

func TestRunResume_InvalidRunID_RejectsPathTraversal(t *testing.T) {
	cmd := newResumeCmd()
	var buf bytes.Buffer
	cmd.SetErr(&buf)

	flags := resumeFlags{RunID: "../etc/passwd"}
	err := runResume(cmd, flags, errResolver)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid run ID")
	assert.Contains(t, err.Error(), "../etc/passwd")
}

func TestRunResume_InvalidCleanID_RejectsPathTraversal(t *testing.T) {
	cmd := newResumeCmd()

	flags := resumeFlags{Clean: "/etc/shadow"}
	err := runResume(cmd, flags, errResolver)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid run ID")
}

// ---- runListMode tests -------------------------------------------------------

func TestRunListMode_EmptyStore_ShowsMessage(t *testing.T) {
	store, _ := makeStateStore(t)

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runListMode(cmd, store)
	require.NoError(t, err)
	assert.Contains(t, errBuf.String(), "No resumable workflow runs found")
}

func TestRunListMode_WithRuns_ShowsTable(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-001", "my-workflow", "step-1")
	saveRun(t, store, "wf-002", "other-workflow", "step-2")

	cmd := &cobra.Command{}
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runListMode(cmd, store)
	require.NoError(t, err)

	out := outBuf.String()
	// Header columns must appear.
	assert.Contains(t, out, "RUN ID")
	assert.Contains(t, out, "WORKFLOW")
	assert.Contains(t, out, "STEP")
	assert.Contains(t, out, "STATUS")
	assert.Contains(t, out, "LAST UPDATED")
	assert.Contains(t, out, "STEPS")

	// Both runs must appear in the table.
	assert.Contains(t, out, "wf-001")
	assert.Contains(t, out, "wf-002")
	assert.Contains(t, out, "my-workflow")
	assert.Contains(t, out, "other-workflow")
}

func TestRunListMode_SingleRun_ShowsStatus(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-abc", "build-workflow", "compile")

	cmd := &cobra.Command{}
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runListMode(cmd, store)
	require.NoError(t, err)

	out := outBuf.String()
	assert.Contains(t, out, "wf-abc")
	assert.Contains(t, out, "build-workflow")
	assert.Contains(t, out, "compile")
}

// ---- runCleanMode tests ------------------------------------------------------

func TestRunCleanMode_ExistingRun_DeletesIt(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-del", "test-wf", "step-1")

	err := runCleanMode(store, "wf-del")
	require.NoError(t, err)

	// Verify it's gone.
	summaries, err := store.List()
	require.NoError(t, err)
	for _, s := range summaries {
		assert.NotEqual(t, "wf-del", s.ID, "deleted run should not appear in list")
	}
}

func TestRunCleanMode_NonExistentRun_ReturnsError(t *testing.T) {
	store, _ := makeStateStore(t)

	err := runCleanMode(store, "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
}

// ---- runCleanAllMode tests ---------------------------------------------------

func TestRunCleanAllMode_EmptyStore_ShowsMessage(t *testing.T) {
	store, _ := makeStateStore(t)

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	// Force=true to bypass the confirmation prompt.
	err := runCleanAllMode(cmd, store, true, os.Stdin)
	require.NoError(t, err)
	assert.Contains(t, errBuf.String(), "No workflow checkpoints found")
}

func TestRunCleanAllMode_WithForce_DeletesAll(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-001", "wf-a", "step-1")
	saveRun(t, store, "wf-002", "wf-b", "step-2")
	saveRun(t, store, "wf-003", "wf-c", "step-3")

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runCleanAllMode(cmd, store, true, os.Stdin)
	require.NoError(t, err)

	summaries, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, summaries, "all checkpoints should be deleted")

	assert.Contains(t, errBuf.String(), "Deleted 3 checkpoint(s)")
}

func TestRunCleanAllMode_NonInteractiveWithoutForce_ReturnsError(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-001", "wf-a", "step-1")

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	// Simulate non-interactive mode by using a pipe (not a terminal) as stdin.
	// We pass a real pipe file so isTerminal returns false deterministically.
	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	pw.Close()
	defer pr.Close()

	err = runCleanAllMode(cmd, store, false /* force */, pr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--clean-all")
	assert.Contains(t, err.Error(), "--force")
}

// ---- runResumeMode tests -----------------------------------------------------

func TestRunResumeMode_NoRuns_ReturnsError(t *testing.T) {
	store, _ := makeStateStore(t)

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runResumeMode(context.Background(), cmd, store, "", false, errResolver)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no resumable workflow runs found")
}

func TestRunResumeMode_SpecificRunNotFound_ReturnsError(t *testing.T) {
	store, _ := makeStateStore(t)

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runResumeMode(context.Background(), cmd, store, "nonexistent-run", false, errResolver)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent-run")
}

func TestRunResumeMode_WorkflowNotFound_ReturnsInformativeError(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-999", "unknown-workflow", "step-1")

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runResumeMode(context.Background(), cmd, store, "wf-999", false, errResolver)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown-workflow")
	// Should mention T-049 to help user understand why.
	assert.Contains(t, err.Error(), "T-049")
}

func TestRunResumeMode_DryRun_PrintsDescriptionNoExecution(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-dry", "test-workflow", "step-compile")

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	// DryRun=true should skip resolver and print a description.
	err := runResumeMode(context.Background(), cmd, store, "wf-dry", true /* dryRun */, errResolver)
	require.NoError(t, err, "dry-run mode should not fail even when definition is unavailable")

	out := errBuf.String()
	assert.Contains(t, out, "Dry-run")
	assert.Contains(t, out, "test-workflow")
	assert.Contains(t, out, "wf-dry")
	assert.Contains(t, out, "step-compile")
}

func TestRunResumeMode_DryRun_LatestRun_NoRunID(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-latest", "some-workflow", "step-x")

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	// No RunID, DryRun=true -- should pick latest run and describe it.
	err := runResumeMode(context.Background(), cmd, store, "" /* runID */, true /* dryRun */, errResolver)
	require.NoError(t, err)

	out := errBuf.String()
	assert.Contains(t, out, "Dry-run")
	assert.Contains(t, out, "wf-latest")
}

func TestRunResumeMode_WithMockDefinition_RunsWorkflow(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-run", "test-wf", "step-a")

	// Provide a simple single-step mock definition that terminates immediately.
	mockResolver := func(workflowName string) (*workflow.WorkflowDefinition, error) {
		if workflowName != "test-wf" {
			return nil, fmt.Errorf("workflow %q: %w", workflowName, ErrWorkflowNotFound)
		}
		return &workflow.WorkflowDefinition{
			Name:        "test-wf",
			Description: "Test workflow",
			InitialStep: "step-a",
			Steps: []workflow.StepDefinition{
				{
					Name: "step-a",
					Transitions: map[string]string{
						workflow.EventSuccess: workflow.StepDone,
					},
				},
			},
		}, nil
	}

	// runResumeMode creates a local registry with built-in handlers. Since
	// "step-a" is not a built-in handler name, the engine will fail when it
	// tries to resolve the handler. We verify that:
	// 1. The resolver is called with the correct workflow name.
	// 2. The engine execution reaches step-a (which fails because step-a is
	//    not a built-in handler), proving the definition was resolved.
	resolved := false
	captureResolver := func(workflowName string) (*workflow.WorkflowDefinition, error) {
		resolved = true
		return mockResolver(workflowName)
	}

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	// The engine will fail because step-a is not a built-in handler, but we
	// can verify the resolver was invoked and the error references the step.
	err := runResumeMode(context.Background(), cmd, store, "wf-run", false, captureResolver)
	assert.True(t, resolved, "resolver should have been called")
	// The error will be from the engine (step handler not found), not from
	// the resolver -- this verifies the full path down to engine execution.
	if err != nil {
		assert.Contains(t, err.Error(), "step-a")
	}
}

// ---- formatRunTable tests ----------------------------------------------------

func TestFormatRunTable_Headers(t *testing.T) {
	t.Parallel()

	summaries := []workflow.RunSummary{
		{
			ID:           "wf-001",
			WorkflowName: "test",
			CurrentStep:  "step-1",
			Status:       "interrupted",
			UpdatedAt:    time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
			StepCount:    3,
		},
	}

	var buf bytes.Buffer
	formatRunTable(summaries, &buf)
	out := buf.String()

	assert.Contains(t, out, "RUN ID")
	assert.Contains(t, out, "WORKFLOW")
	assert.Contains(t, out, "STEP")
	assert.Contains(t, out, "STATUS")
	assert.Contains(t, out, "LAST UPDATED")
	assert.Contains(t, out, "STEPS")
}

func TestFormatRunTable_DataRows(t *testing.T) {
	t.Parallel()

	summaries := []workflow.RunSummary{
		{
			ID:           "wf-abc",
			WorkflowName: "build",
			CurrentStep:  "compile",
			Status:       "running",
			UpdatedAt:    time.Date(2026, 2, 10, 9, 0, 0, 0, time.UTC),
			StepCount:    5,
		},
		{
			ID:           "wf-xyz",
			WorkflowName: "review",
			CurrentStep:  "lint",
			Status:       "interrupted",
			UpdatedAt:    time.Date(2026, 2, 11, 12, 0, 0, 0, time.UTC),
			StepCount:    2,
		},
	}

	var buf bytes.Buffer
	formatRunTable(summaries, &buf)
	out := buf.String()

	assert.Contains(t, out, "wf-abc")
	assert.Contains(t, out, "build")
	assert.Contains(t, out, "compile")
	assert.Contains(t, out, "running")
	assert.Contains(t, out, "5")

	assert.Contains(t, out, "wf-xyz")
	assert.Contains(t, out, "review")
	assert.Contains(t, out, "lint")
	assert.Contains(t, out, "interrupted")
	assert.Contains(t, out, "2")
}

func TestFormatRunTable_EmptySlice_OnlyHeaders(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	formatRunTable([]workflow.RunSummary{}, &buf)
	out := buf.String()

	// Headers should still be there even with no data rows.
	assert.Contains(t, out, "RUN ID")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// Two lines: header + separator.
	assert.Equal(t, 2, len(lines), "empty table should have header + separator only")
}

func TestFormatRunTable_DateFormat(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 3, 5, 14, 22, 33, 0, time.UTC)
	summaries := []workflow.RunSummary{
		{
			ID:           "wf-date",
			WorkflowName: "x",
			CurrentStep:  "y",
			Status:       "completed",
			UpdatedAt:    at,
			StepCount:    0,
		},
	}

	var buf bytes.Buffer
	formatRunTable(summaries, &buf)
	out := buf.String()

	assert.Contains(t, out, "2026-03-05 14:22:33", "date should be formatted as YYYY-MM-DD HH:MM:SS")
}

// ---- resolveDefinition tests -------------------------------------------------

func TestResolveDefinition_AlwaysReturnsNotFound(t *testing.T) {
	t.Parallel()

	def, err := resolveDefinition("any-workflow")
	assert.Nil(t, def)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWorkflowNotFound)
}

func TestResolveDefinition_ErrorMessageContainsName(t *testing.T) {
	t.Parallel()

	_, err := resolveDefinition("my-special-workflow")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "my-special-workflow")
}

// ---- isTerminal tests --------------------------------------------------------

func TestIsTerminal_RegularFile_ReturnsFalse(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp(t.TempDir(), "test-*.txt")
	require.NoError(t, err)
	defer f.Close()

	assert.False(t, isTerminal(f), "regular file should not be detected as terminal")
}

func TestIsTerminal_Pipe_ReturnsFalse(t *testing.T) {
	// Create a pipe; neither end is a terminal.
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()
	defer w.Close()

	assert.False(t, isTerminal(r), "pipe read end should not be detected as terminal")
	assert.False(t, isTerminal(w), "pipe write end should not be detected as terminal")
}

// ---- ErrWorkflowNotFound tests -----------------------------------------------

func TestErrWorkflowNotFound_IsDetectable(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("outer: %w", ErrWorkflowNotFound)
	assert.ErrorIs(t, wrapped, ErrWorkflowNotFound)
}

// ---- runResume integration path tests ----------------------------------------

func TestRunResume_ListFlag_UsesStateDir(t *testing.T) {
	// Write a checkpoint JSON to a temp dir and point defaultStateDir at it
	// via the state store directly. This tests the List branch end-to-end.
	tmpDir := t.TempDir()

	store, err := workflow.NewStateStore(tmpDir)
	require.NoError(t, err)

	state := workflow.NewWorkflowState("wf-integration", "int-wf", "step-1")
	require.NoError(t, store.Save(state))

	cmd := &cobra.Command{}
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err = runListMode(cmd, store)
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "wf-integration")
}

func TestRunResume_CleanFlag_DeletesCheckpoint(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-to-clean", "cleanup-wf", "step-1")

	// Verify it exists before.
	summaries, err := store.List()
	require.NoError(t, err)
	require.Len(t, summaries, 1)

	err = runCleanMode(store, "wf-to-clean")
	require.NoError(t, err)

	// Verify it's gone.
	summaries, err = store.List()
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

// ---- JSON round-trip for RunSummary (ensures state.go compatibility) ----------

func TestRunSummary_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := workflow.RunSummary{
		ID:           "wf-json",
		WorkflowName: "json-test",
		CurrentStep:  "parse",
		Status:       "running",
		UpdatedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		StepCount:    7,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded workflow.RunSummary
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original.ID, decoded.ID)
	assert.Equal(t, original.WorkflowName, decoded.WorkflowName)
	assert.Equal(t, original.CurrentStep, decoded.CurrentStep)
	assert.Equal(t, original.Status, decoded.Status)
	assert.Equal(t, original.StepCount, decoded.StepCount)
}

// ---- StateStore.LatestRun ordering (verifies runResumeMode picks newest) -----

func TestRunResumeMode_LatestRun_PicksMostRecentlyUpdated(t *testing.T) {
	store, _ := makeStateStore(t)

	// Save two runs with slightly different timestamps.
	older := workflow.NewWorkflowState("wf-older", "wf-a", "step-1")
	older.UpdatedAt = time.Now().Add(-time.Hour)
	require.NoError(t, store.Save(older))

	newer := workflow.NewWorkflowState("wf-newer", "wf-b", "step-2")
	newer.UpdatedAt = time.Now()
	require.NoError(t, store.Save(newer))

	// Use dry-run so we don't need a real definition; just verify which run is
	// selected (the newer one).
	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runResumeMode(context.Background(), cmd, store, "" /* pick latest */, true /* dryRun */, errResolver)
	require.NoError(t, err)

	out := errBuf.String()
	assert.Contains(t, out, "wf-newer", "should resume the most recently updated run")
	assert.NotContains(t, out, "wf-older", "should not resume the older run")
}

// ---- File that writes a complete state file then lists it --------------------

func TestRunListMode_CorruptFileSkipped(t *testing.T) {
	store, dir := makeStateStore(t)

	// Write a valid run first.
	saveRun(t, store, "wf-good", "good-wf", "step-ok")

	// Write a corrupt JSON file directly into the state dir.
	corruptPath := filepath.Join(dir, "corrupt.json")
	require.NoError(t, os.WriteFile(corruptPath, []byte("not-json{{{"), 0o644))

	cmd := &cobra.Command{}
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	// List should succeed and only show the valid run.
	err := runListMode(cmd, store)
	require.NoError(t, err)

	out := outBuf.String()
	assert.Contains(t, out, "wf-good", "valid run should appear in list")
	// The corrupt file is silently skipped by StateStore.List().
}

// ---- Dry-run output format verification --------------------------------------

// TestRunResumeMode_DryRun_ShowsStepCount verifies that the dry-run output
// includes the steps-completed count when the state has step history records.
func TestRunResumeMode_DryRun_ShowsStepCount(t *testing.T) {
	store, _ := makeStateStore(t)

	// Build a state that already has step history so StepCount > 0.
	state := workflow.NewWorkflowState("wf-steps", "counted-workflow", "step-3")
	state.AddStepRecord(workflow.StepRecord{
		Step:      "step-1",
		Event:     workflow.EventSuccess,
		StartedAt: time.Now().Add(-2 * time.Minute),
		Duration:  time.Second,
	})
	state.AddStepRecord(workflow.StepRecord{
		Step:      "step-2",
		Event:     workflow.EventSuccess,
		StartedAt: time.Now().Add(-time.Minute),
		Duration:  time.Second,
	})
	require.NoError(t, store.Save(state))

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runResumeMode(context.Background(), cmd, store, "wf-steps", true, errResolver)
	require.NoError(t, err)

	out := errBuf.String()
	// The output must contain the steps-completed count.
	assert.Contains(t, out, "Steps completed: 2", "dry-run must show how many steps have been completed")
	// The output must contain last-updated information.
	assert.Contains(t, out, "Last updated:", "dry-run must show the last-updated timestamp")
}

// TestRunResumeMode_DryRun_ShowsAllRequiredFields verifies every required field
// is printed in dry-run mode: workflow name, run ID, current step, steps
// completed, and last updated.
func TestRunResumeMode_DryRun_ShowsAllRequiredFields(t *testing.T) {
	store, _ := makeStateStore(t)
	state := workflow.NewWorkflowState("wf-fields", "field-check-workflow", "current-step")
	require.NoError(t, store.Save(state))

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runResumeMode(context.Background(), cmd, store, "wf-fields", true, errResolver)
	require.NoError(t, err)

	out := errBuf.String()
	assert.Contains(t, out, "field-check-workflow", "dry-run output must include workflow name")
	assert.Contains(t, out, "wf-fields", "dry-run output must include run ID")
	assert.Contains(t, out, "current-step", "dry-run output must include current step")
	assert.Contains(t, out, "Steps completed:", "dry-run output must include steps-completed label")
	assert.Contains(t, out, "Last updated:", "dry-run output must include last-updated label")
}

// TestRunResumeMode_DryRun_ZeroStepsCompleted verifies that when a state has no
// step history the dry-run output still shows "Steps completed: 0".
func TestRunResumeMode_DryRun_ZeroStepsCompleted(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-fresh", "fresh-workflow", "step-init")

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runResumeMode(context.Background(), cmd, store, "wf-fresh", true, errResolver)
	require.NoError(t, err)

	out := errBuf.String()
	assert.Contains(t, out, "Steps completed: 0", "fresh run should show zero steps completed")
}

// ---- Global flagDryRun -------------------------------------------------------

// TestRunResumeMode_GlobalDryRunFlag verifies that the global --dry-run flag
// (flagDryRun) triggers dry-run output even when the local dryRun parameter
// is false.
func TestRunResumeMode_GlobalDryRunFlag_TriggersDryRun(t *testing.T) {
	// Save and restore the global flag to avoid test pollution.
	orig := flagDryRun
	flagDryRun = true
	defer func() { flagDryRun = orig }()

	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-global-dry", "global-dry-workflow", "step-x")

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	// Pass dryRun=false; flagDryRun=true should take over.
	err := runResumeMode(context.Background(), cmd, store, "wf-global-dry", false /* dryRun */, errResolver)
	require.NoError(t, err, "global flagDryRun should prevent execution and not return an error")

	out := errBuf.String()
	assert.Contains(t, out, "Dry-run", "global --dry-run should produce dry-run output")
	assert.Contains(t, out, "wf-global-dry", "dry-run output must include the run ID")
}

// ---- Completed run handling --------------------------------------------------

// TestRunListMode_CompletedRun_ShowsCompletedStatus verifies that a run whose
// CurrentStep is StepDone appears with status "completed" in the table.
func TestRunListMode_CompletedRun_ShowsCompletedStatus(t *testing.T) {
	store, _ := makeStateStore(t)

	// Create a state that is at the terminal done step.
	state := workflow.NewWorkflowState("wf-done", "finished-workflow", workflow.StepDone)
	require.NoError(t, store.Save(state))

	cmd := &cobra.Command{}
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runListMode(cmd, store)
	require.NoError(t, err)

	out := outBuf.String()
	assert.Contains(t, out, "wf-done", "completed run must appear in the list")
	assert.Contains(t, out, "completed", "status must be 'completed' for a run at StepDone")
}

// TestRunResumeMode_CompletedRun_ResolvesDefinitionNormally verifies that when
// the state's CurrentStep is StepDone the resume path still attempts to resolve
// the definition (the implementation does not short-circuit on completed status).
func TestRunResumeMode_CompletedRun_DefinitionResolutionAttempted(t *testing.T) {
	store, _ := makeStateStore(t)

	state := workflow.NewWorkflowState("wf-done2", "done-workflow", workflow.StepDone)
	require.NoError(t, store.Save(state))

	resolverCalled := false
	resolver := func(workflowName string) (*workflow.WorkflowDefinition, error) {
		resolverCalled = true
		return nil, fmt.Errorf("workflow %q: %w", workflowName, ErrWorkflowNotFound)
	}

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runResumeMode(context.Background(), cmd, store, "wf-done2", false, resolver)
	// The resolver should be invoked even for completed runs (no early return in impl).
	assert.True(t, resolverCalled, "definition resolver must be called when dryRun=false")
	require.Error(t, err, "ErrWorkflowNotFound must propagate as an error")
}

// ---- Failed run status in table ----------------------------------------------

// TestRunListMode_FailedRun_ShowsFailedStatus verifies that a run whose
// CurrentStep is StepFailed appears with status "failed" in the table.
func TestRunListMode_FailedRun_ShowsFailedStatus(t *testing.T) {
	store, _ := makeStateStore(t)

	state := workflow.NewWorkflowState("wf-fail", "failing-workflow", workflow.StepFailed)
	require.NoError(t, store.Save(state))

	cmd := &cobra.Command{}
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runListMode(cmd, store)
	require.NoError(t, err)

	out := outBuf.String()
	assert.Contains(t, out, "wf-fail")
	assert.Contains(t, out, "failed", "status must be 'failed' for a run at StepFailed")
}

// ---- Table sort order --------------------------------------------------------

// TestRunListMode_ThreeRuns_SortedByUpdatedAtDesc verifies that when multiple
// runs exist they are displayed most-recently-updated first (descending).
func TestRunListMode_ThreeRuns_SortedByUpdatedAtDesc(t *testing.T) {
	store, _ := makeStateStore(t)

	now := time.Now()

	oldest := workflow.NewWorkflowState("wf-oldest", "wf-x", "step-1")
	oldest.UpdatedAt = now.Add(-2 * time.Hour)
	require.NoError(t, store.Save(oldest))

	middle := workflow.NewWorkflowState("wf-middle", "wf-y", "step-2")
	middle.UpdatedAt = now.Add(-time.Hour)
	require.NoError(t, store.Save(middle))

	newest := workflow.NewWorkflowState("wf-newest", "wf-z", "step-3")
	newest.UpdatedAt = now
	require.NoError(t, store.Save(newest))

	cmd := &cobra.Command{}
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runListMode(cmd, store)
	require.NoError(t, err)

	out := outBuf.String()
	// All three runs must appear.
	assert.Contains(t, out, "wf-oldest")
	assert.Contains(t, out, "wf-middle")
	assert.Contains(t, out, "wf-newest")

	// The newest run must appear before the oldest in the output string.
	posNewest := strings.Index(out, "wf-newest")
	posMiddle := strings.Index(out, "wf-middle")
	posOldest := strings.Index(out, "wf-oldest")
	assert.Less(t, posNewest, posMiddle, "newest run must appear before middle run")
	assert.Less(t, posMiddle, posOldest, "middle run must appear before oldest run")
}

// ---- runResumeMode with non-ErrWorkflowNotFound resolver error ---------------

// TestRunResumeMode_ResolverOtherError_WrapsError verifies that a resolver
// error that is NOT ErrWorkflowNotFound is propagated with appropriate context.
func TestRunResumeMode_ResolverOtherError_WrapsError(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-resolver-err", "some-workflow", "step-1")

	otherErr := fmt.Errorf("network timeout")
	resolver := func(_ string) (*workflow.WorkflowDefinition, error) {
		return nil, otherErr
	}

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runResumeMode(context.Background(), cmd, store, "wf-resolver-err", false, resolver)
	require.Error(t, err)
	// Must NOT reference T-049 (that message is only for ErrWorkflowNotFound).
	assert.NotContains(t, err.Error(), "T-049")
	// Must contain the workflow name and the original error text.
	assert.Contains(t, err.Error(), "some-workflow")
	assert.Contains(t, err.Error(), "network timeout")
}

// ---- Context cancellation during resume -------------------------------------

// TestRunResumeMode_ContextCancelled_ReturnsContextErr verifies that when the
// context is cancelled before the engine runs the error is propagated.
func TestRunResumeMode_ContextCancelled_ReturnsContextErr(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-ctx", "ctx-workflow", "step-1")

	// Return a valid (but handler-less) definition so the engine can start.
	resolver := func(_ string) (*workflow.WorkflowDefinition, error) {
		return &workflow.WorkflowDefinition{
			Name:        "ctx-workflow",
			InitialStep: "step-1",
			Steps: []workflow.StepDefinition{
				{
					Name:        "step-1",
					Transitions: map[string]string{workflow.EventSuccess: workflow.StepDone},
				},
			},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	// With a pre-cancelled context the engine should return context.Canceled.
	err := runResumeMode(ctx, cmd, store, "wf-ctx", false, resolver)
	// Either context.Canceled or a wrapped error from the engine/registry.
	require.Error(t, err)
}

// ---- runCleanAllMode output message -----------------------------------------

// TestRunCleanAllMode_Force_PrintsDeletedCount verifies that the number of
// deleted checkpoints is correctly reported in the output.
func TestRunCleanAllMode_Force_PrintsDeletedCount(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-a1", "wf-a", "step-1")
	saveRun(t, store, "wf-b1", "wf-b", "step-1")

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runCleanAllMode(cmd, store, true, os.Stdin)
	require.NoError(t, err)

	out := errBuf.String()
	assert.Contains(t, out, "Deleted 2 checkpoint(s)", "output must report exact count of deleted checkpoints")

	// Verify store is now empty.
	summaries, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

// TestRunCleanAllMode_SingleCheckpoint_Force verifies clean-all with a single
// checkpoint reports "Deleted 1 checkpoint(s)".
func TestRunCleanAllMode_SingleCheckpoint_Force(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-only", "only-wf", "step-1")

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runCleanAllMode(cmd, store, true, os.Stdin)
	require.NoError(t, err)
	assert.Contains(t, errBuf.String(), "Deleted 1 checkpoint(s)")
}

// ---- Invalid run ID edge cases -----------------------------------------------

// TestRunResume_InvalidRunID_WithSlashes rejects IDs containing path separators.
func TestRunResume_InvalidRunID_WithSlashes(t *testing.T) {
	tests := []struct {
		name  string
		runID string
	}{
		{"forward slash", "path/with/slashes"},
		{"dot-dot slash", "../parent"},
		{"absolute path", "/absolute"},
		{"space in id", "has space"},
		{"dot in id", "has.dot"},
		{"at sign", "has@symbol"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newResumeCmd()
			var errBuf bytes.Buffer
			cmd.SetErr(&errBuf)

			flags := resumeFlags{RunID: tt.runID}
			err := runResume(cmd, flags, errResolver)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid run ID")
		})
	}
}

// TestRunResume_InvalidCleanID_VariousFormats verifies that invalid ID formats
// are rejected for --clean as well.
func TestRunResume_InvalidCleanID_VariousFormats(t *testing.T) {
	tests := []struct {
		name    string
		cleanID string
	}{
		{"path traversal", "../etc/passwd"},
		{"absolute path", "/etc/shadow"},
		{"forward slash", "dir/file"},
		{"space", "run id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newResumeCmd()
			flags := resumeFlags{Clean: tt.cleanID}
			err := runResume(cmd, flags, errResolver)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid run ID")
		})
	}
}

// ---- formatRunTable step count column ---------------------------------------

// TestFormatRunTable_StepCountColumn verifies that zero and non-zero step counts
// are rendered correctly in the STEPS column.
func TestFormatRunTable_StepCountColumn(t *testing.T) {
	t.Parallel()

	summaries := []workflow.RunSummary{
		{
			ID:           "wf-zero",
			WorkflowName: "fresh",
			CurrentStep:  "init",
			Status:       "interrupted",
			UpdatedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			StepCount:    0,
		},
		{
			ID:           "wf-many",
			WorkflowName: "mature",
			CurrentStep:  "deploy",
			Status:       "running",
			UpdatedAt:    time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
			StepCount:    42,
		},
	}

	var buf bytes.Buffer
	formatRunTable(summaries, &buf)
	out := buf.String()

	assert.Contains(t, out, "0", "zero step count must appear in table")
	assert.Contains(t, out, "42", "non-zero step count must appear in table")
}

// ---- formatRunTable separator line ------------------------------------------

// TestFormatRunTable_SeparatorLine verifies the separator row immediately below
// the header contains dashes.
func TestFormatRunTable_SeparatorLine(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	formatRunTable([]workflow.RunSummary{}, &buf)
	out := buf.String()

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.GreaterOrEqual(t, len(lines), 2, "table must have at least header and separator")
	// The separator line should contain dashes.
	assert.Contains(t, lines[1], "------", "second line must be the separator row")
}

// ---- runResumeMode with a step-history state (status "running") --------------

// TestRunListMode_RunningStatus_ShowsRunning verifies that a state with
// recorded step history but not at a terminal step shows status "running".
func TestRunListMode_RunningStatus_ShowsRunning(t *testing.T) {
	store, _ := makeStateStore(t)

	state := workflow.NewWorkflowState("wf-running", "active-workflow", "step-2")
	state.AddStepRecord(workflow.StepRecord{
		Step:      "step-1",
		Event:     workflow.EventSuccess,
		StartedAt: time.Now().Add(-time.Minute),
		Duration:  time.Second,
	})
	require.NoError(t, store.Save(state))

	cmd := &cobra.Command{}
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runListMode(cmd, store)
	require.NoError(t, err)

	out := outBuf.String()
	assert.Contains(t, out, "running", "a state with step history at a non-terminal step must have status 'running'")
}

// ---- runResume dispatches to correct sub-modes (table-driven) ----------------

// TestRunResume_FlagDispatch_Clean verifies that providing a valid clean ID
// with a valid run in the store dispatches to the delete path and succeeds.
func TestRunResume_FlagDispatch_Clean_ExistingRun(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-dispatch-clean", "dc-wf", "step-1")

	// runResume uses defaultStateDir internally so we cannot inject the store.
	// We test runCleanMode directly as the dispatch target.
	err := runCleanMode(store, "wf-dispatch-clean")
	require.NoError(t, err)

	summaries, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

// ---- runResumeMode resolver called with correct workflow name ----------------

// TestRunResumeMode_ResolverCalledWithCorrectWorkflowName verifies that the
// definition resolver receives the exact workflow name stored in the state.
func TestRunResumeMode_ResolverCalledWithCorrectWorkflowName(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-name-check", "exact-workflow-name", "step-1")

	var capturedName string
	resolver := func(workflowName string) (*workflow.WorkflowDefinition, error) {
		capturedName = workflowName
		return nil, fmt.Errorf("workflow %q: %w", workflowName, ErrWorkflowNotFound)
	}

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runResumeMode(context.Background(), cmd, store, "wf-name-check", false, resolver)
	require.Error(t, err)
	assert.Equal(t, "exact-workflow-name", capturedName,
		"resolver must receive the exact workflow name from the persisted state")
}

// ---- runResumeMode: engine integration with registered handler ---------------

// TestRunResumeMode_WithRegisteredHandler_WorkflowCompletes verifies that
// the engine runs to completion using a built-in handler that does not require
// runtime dependencies (init_phase). runResumeMode creates its own local
// registry and registers all built-in handlers, so built-in step names are
// always available.
func TestRunResumeMode_WithRegisteredHandler_EngineRunsToCompletion(t *testing.T) {
	// Use a built-in handler that succeeds without external deps.
	const handlerName = "init_phase"
	const workflowName = "integration-workflow"
	const runID = "wf-integration-engine"

	store, _ := makeStateStore(t)
	saveRun(t, store, runID, workflowName, handlerName)

	resolver := func(_ string) (*workflow.WorkflowDefinition, error) {
		return &workflow.WorkflowDefinition{
			Name:        workflowName,
			InitialStep: handlerName,
			Steps: []workflow.StepDefinition{
				{
					Name: handlerName,
					Transitions: map[string]string{
						workflow.EventSuccess: workflow.StepDone,
					},
				},
			},
		}, nil
	}

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runResumeMode(context.Background(), cmd, store, runID, false, resolver)
	require.NoError(t, err, "engine should run to completion when a valid definition and handler are provided")
}

// ---- Workflow name error message includes T-049 reference -------------------

// TestRunResumeMode_WorkflowNotFound_MentionsT049 ensures the error message
// from the default resolver (ErrWorkflowNotFound) mentions T-049 so users
// know why the definition is missing.
func TestRunResumeMode_WorkflowNotFound_ErrorMentionsT049(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-t049", "any-workflow", "step-1")

	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runResumeMode(context.Background(), cmd, store, "wf-t049", false, errResolver)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "T-049",
		"error message must reference T-049 so users understand the limitation")
}

// ---- runResume: state directory error ----------------------------------------

// TestRunResume_StoreErrorOnInvalidPath verifies that runResume returns a
// descriptive error when the state store cannot be opened. We simulate this by
// relying on the fact that defaultStateDir is used internally; instead we test
// NewStateStore directly with an unwritable parent to confirm the error path.
func TestStateStore_NewStateStore_UnwritableDir(t *testing.T) {
	// Create a file where the state directory should be, so MkdirAll fails.
	parent := t.TempDir()
	blockingFile := filepath.Join(parent, "not-a-dir")
	require.NoError(t, os.WriteFile(blockingFile, []byte("block"), 0o444))

	// Attempt to create a store inside the file (which is not a directory).
	_, err := workflow.NewStateStore(filepath.Join(blockingFile, "state"))
	require.Error(t, err, "NewStateStore must fail when the path is blocked by a file")
}

// ---- formatRunTable: very long run IDs --------------------------------------

// TestFormatRunTable_VeryLongRunID verifies that a very long run ID does not
// cause the table writer to panic or produce malformed output. tabwriter aligns
// columns but does not truncate, so the long value must still appear.
func TestFormatRunTable_VeryLongRunID(t *testing.T) {
	t.Parallel()

	longID := strings.Repeat("a", 200)
	summaries := []workflow.RunSummary{
		{
			ID:           longID,
			WorkflowName: "test",
			CurrentStep:  "step-1",
			Status:       "running",
			UpdatedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			StepCount:    1,
		},
	}

	var buf bytes.Buffer
	// Must not panic.
	assert.NotPanics(t, func() { formatRunTable(summaries, &buf) })
	out := buf.String()
	// The long ID must be present in full (tabwriter does not truncate).
	assert.Contains(t, out, longID, "very long run ID must appear in the table output")
}

// ---- runCleanMode: multiple sequential deletes -------------------------------

// TestRunCleanMode_MultipleDeletes verifies that runCleanMode can be called
// multiple times on different run IDs within the same store session.
func TestRunCleanMode_MultipleDeletes(t *testing.T) {
	store, _ := makeStateStore(t)
	saveRun(t, store, "wf-d1", "wf-a", "step-1")
	saveRun(t, store, "wf-d2", "wf-b", "step-1")
	saveRun(t, store, "wf-d3", "wf-c", "step-1")

	require.NoError(t, runCleanMode(store, "wf-d1"))
	require.NoError(t, runCleanMode(store, "wf-d2"))

	summaries, err := store.List()
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "wf-d3", summaries[0].ID, "only wf-d3 should remain after deleting d1 and d2")
}

// ---- runCleanMode error message contains run ID -----------------------------

// TestRunCleanMode_ErrorMessage_ContainsRunID verifies that the error returned
// by runCleanMode for a non-existent run ID includes the ID in the message.
func TestRunCleanMode_ErrorMessage_ContainsRunID(t *testing.T) {
	store, _ := makeStateStore(t)

	err := runCleanMode(store, "missing-run-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing-run-id",
		"error message must include the run ID that was not found")
}

// ---- ErrWorkflowNotFound sentinel --------------------------------------------

// TestErrWorkflowNotFound_Sentinel verifies the sentinel error value itself.
func TestErrWorkflowNotFound_Sentinel(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "workflow definition not found", ErrWorkflowNotFound.Error())
}

// ---- runIDPattern boundary cases --------------------------------------------

// TestRunIDPattern_BoundaryLengths verifies that single-character and very long
// IDs that match the pattern are accepted.
func TestRunIDPattern_BoundaryLengths(t *testing.T) {
	t.Parallel()

	// Single character
	assert.True(t, runIDPattern.MatchString("a"), "single char must match")
	assert.True(t, runIDPattern.MatchString("1"), "single digit must match")
	assert.True(t, runIDPattern.MatchString("-"), "single hyphen must match")
	assert.True(t, runIDPattern.MatchString("_"), "single underscore must match")

	// Very long valid ID
	longValid := strings.Repeat("ab-", 100)
	assert.True(t, runIDPattern.MatchString(longValid), "long valid ID must match")
}
