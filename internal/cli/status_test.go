package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/task"
)

// resetStatusFlags resets the status command's local flags for inter-test isolation.
// It resets both the Changed tracking and the actual flag values to their defaults.
func resetStatusFlags(t *testing.T) {
	t.Helper()
	resetRootCmd(t)
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "status" {
			cmd.Flags().VisitAll(func(f *pflag.Flag) {
				f.Changed = false
				// Reset flag values to their defaults via pflag's Set method.
				if err := f.Value.Set(f.DefValue); err != nil {
					t.Logf("resetting flag %q: %v", f.Name, err)
				}
			})
			break
		}
	}
}

// --- renderPhaseProgress tests -----------------------------------------------

func TestRenderPhaseProgress_ZeroPercent(t *testing.T) {
	t.Parallel()

	prog := task.PhaseProgress{
		PhaseID:    1,
		Total:      10,
		NotStarted: 10,
	}

	output := renderPhaseProgress(prog, "Foundation", false)

	assert.Contains(t, output, "Phase 1: Foundation")
	assert.Contains(t, output, "0%")
	assert.Contains(t, output, "0/10")
}

func TestRenderPhaseProgress_FiftyPercent(t *testing.T) {
	t.Parallel()

	prog := task.PhaseProgress{
		PhaseID:    2,
		Total:      20,
		Completed:  10,
		NotStarted: 10,
	}

	output := renderPhaseProgress(prog, "Core Implementation", false)

	assert.Contains(t, output, "Phase 2: Core Implementation")
	assert.Contains(t, output, "50%")
	assert.Contains(t, output, "10/20")
}

func TestRenderPhaseProgress_HundredPercent(t *testing.T) {
	t.Parallel()

	prog := task.PhaseProgress{
		PhaseID:   3,
		Total:     15,
		Completed: 15,
	}

	output := renderPhaseProgress(prog, "Review Pipeline", false)

	assert.Contains(t, output, "Phase 3: Review Pipeline")
	assert.Contains(t, output, "100%")
	assert.Contains(t, output, "15/15")
}

func TestRenderPhaseProgress_WithInProgressTasks(t *testing.T) {
	t.Parallel()

	prog := task.PhaseProgress{
		PhaseID:    1,
		Total:      10,
		Completed:  3,
		InProgress: 2,
		NotStarted: 5,
	}

	output := renderPhaseProgress(prog, "Foundation", false)

	assert.Contains(t, output, "3/10")
	assert.Contains(t, output, "in-progress")
}

func TestRenderPhaseProgress_WithBlockedTasks(t *testing.T) {
	t.Parallel()

	prog := task.PhaseProgress{
		PhaseID:    2,
		Total:      10,
		Completed:  4,
		Blocked:    2,
		NotStarted: 4,
	}

	output := renderPhaseProgress(prog, "Implementation", true)

	assert.Contains(t, output, "blocked")
}

func TestRenderPhaseProgress_ZeroTasks(t *testing.T) {
	t.Parallel()

	prog := task.PhaseProgress{
		PhaseID: 1,
		Total:   0,
	}

	output := renderPhaseProgress(prog, "Empty Phase", false)

	assert.Contains(t, output, "0%")
	assert.Contains(t, output, "0/0")
}

func TestRenderPhaseProgress_UngroupedPhase(t *testing.T) {
	t.Parallel()

	prog := task.PhaseProgress{
		PhaseID:   0,
		Total:     5,
		Completed: 3,
	}

	output := renderPhaseProgress(prog, "", false)

	assert.Contains(t, output, "All Tasks")
	assert.Contains(t, output, "3/5")
}

// --- renderSummary tests ------------------------------------------------------

func TestRenderSummary_MultiplePhasesAtDifferentProgress(t *testing.T) {
	t.Parallel()

	allProgress := []task.PhaseProgress{
		{PhaseID: 1, Total: 10, Completed: 10},
		{PhaseID: 2, Total: 20, Completed: 5, NotStarted: 15},
		{PhaseID: 3, Total: 5, NotStarted: 5},
	}

	output := renderSummary(allProgress, "my-project")

	assert.Contains(t, output, "Raven Status - my-project")
	assert.Contains(t, output, "15/35")
	assert.Contains(t, output, "43%") // 15/35 ≈ 42.86%, rounds to 43%
}

func TestRenderSummary_EmptyProgress(t *testing.T) {
	t.Parallel()

	output := renderSummary([]task.PhaseProgress{}, "empty-project")

	assert.Contains(t, output, "Raven Status - empty-project")
	assert.Contains(t, output, "0/0")
	assert.Contains(t, output, "0%")
}

func TestRenderSummary_AllComplete(t *testing.T) {
	t.Parallel()

	allProgress := []task.PhaseProgress{
		{PhaseID: 1, Total: 5, Completed: 5},
		{PhaseID: 2, Total: 5, Completed: 5},
	}

	output := renderSummary(allProgress, "done-project")

	assert.Contains(t, output, "10/10")
	assert.Contains(t, output, "100%")
}

// --- JSON output tests --------------------------------------------------------

func TestStatusJSON_ValidSchema(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a minimal task spec.
	specContent := "# T-001: Setup\n\n## Metadata\n| Field | Value |\n|-------|-------|\n| Priority | Must Have |\n| Estimated Effort | Small |\n| Dependencies | None |\n| Blocked By | None |\n| Blocks | None |\n"
	specPath := filepath.Join(tmpDir, "T-001-setup.md")
	require.NoError(t, os.WriteFile(specPath, []byte(specContent), 0o644))

	// Write a state file.
	statePath := filepath.Join(tmpDir, "task-state.conf")
	require.NoError(t, os.WriteFile(statePath, []byte("T-001|completed|claude||\n"), 0o644))

	// Write a phases.conf.
	phasesPath := filepath.Join(tmpDir, "phases.conf")
	require.NoError(t, os.WriteFile(phasesPath, []byte("1|Foundation|T-001|T-001\n"), 0o644))

	// Write a minimal raven.toml.
	tomlContent := fmt.Sprintf("[project]\nname = \"test-project\"\ntasks_dir = %q\ntask_state_file = %q\nphases_conf = %q\n",
		tmpDir, statePath, phasesPath)
	tomlPath := filepath.Join(tmpDir, "raven.toml")
	require.NoError(t, os.WriteFile(tomlPath, []byte(tomlContent), 0o644))

	resetStatusFlags(t)

	// Capture output via rootCmd's writer (JSON goes to cmd.OutOrStdout()).
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)

	rootCmd.SetArgs([]string{"--config", tomlPath, "status", "--json"})
	code := Execute()

	assert.Equal(t, 0, code, "exit code should be 0")

	// Verify valid JSON.
	var out statusOutput
	err := json.Unmarshal(buf.Bytes(), &out)
	require.NoError(t, err, "output must be valid JSON")

	assert.Equal(t, "test-project", out.ProjectName)
	assert.Equal(t, 1, out.TotalTasks)
	assert.Equal(t, 1, out.TotalDone)
	assert.InDelta(t, 100.0, out.OverallPct, 0.01)
	require.Len(t, out.Phases, 1)
	assert.Equal(t, 1, out.Phases[0].PhaseID)
	assert.Equal(t, "Foundation", out.Phases[0].PhaseName)
}

// --- No tasks / no phases edge cases ------------------------------------------

func TestStatusCmd_NoTasks_ShowsMessage(t *testing.T) {
	tmpDir := t.TempDir()

	// Empty tasks dir -- no task specs.
	statePath := filepath.Join(tmpDir, "task-state.conf")
	require.NoError(t, os.WriteFile(statePath, []byte(""), 0o644))

	phasesPath := filepath.Join(tmpDir, "phases.conf")
	require.NoError(t, os.WriteFile(phasesPath, []byte("1|Foundation|T-001|T-005\n"), 0o644))

	tomlContent := fmt.Sprintf("[project]\nname = \"empty-project\"\ntasks_dir = %q\ntask_state_file = %q\nphases_conf = %q\n",
		tmpDir, statePath, phasesPath)
	tomlPath := filepath.Join(tmpDir, "raven.toml")
	require.NoError(t, os.WriteFile(tomlPath, []byte(tomlContent), 0o644))

	resetStatusFlags(t)

	// Capture stderr.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldStderr
	})

	rootCmd.SetArgs([]string{"--config", tomlPath, "status"})
	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stderr = oldStderr

	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "No tasks found")
}

func TestStatusCmd_NoPhases_ShowsUngrouped(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a task spec.
	specContent := "# T-001: Setup\n\n## Metadata\n| Field | Value |\n|-------|-------|\n| Priority | Must Have |\n| Estimated Effort | Small |\n| Dependencies | None |\n| Blocked By | None |\n| Blocks | None |\n"
	specPath := filepath.Join(tmpDir, "T-001-setup.md")
	require.NoError(t, os.WriteFile(specPath, []byte(specContent), 0o644))

	statePath := filepath.Join(tmpDir, "task-state.conf")
	require.NoError(t, os.WriteFile(statePath, []byte(""), 0o644))

	// Point phases_conf to a non-existent file -- should degrade gracefully.
	tomlContent := fmt.Sprintf("[project]\nname = \"no-phases-project\"\ntasks_dir = %q\ntask_state_file = %q\nphases_conf = %q\n",
		tmpDir, statePath, filepath.Join(tmpDir, "nonexistent.conf"))
	tomlPath := filepath.Join(tmpDir, "raven.toml")
	require.NoError(t, os.WriteFile(tomlPath, []byte(tomlContent), 0o644))

	resetStatusFlags(t)

	// Capture stderr.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldStderr
	})

	rootCmd.SetArgs([]string{"--config", tomlPath, "status"})
	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stderr = oldStderr

	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "All Tasks")
}

// --- Command registration tests -----------------------------------------------

func TestStatusCmd_RegisteredInRoot(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "status" {
			found = true
			break
		}
	}
	assert.True(t, found, "status command must be registered in rootCmd")
}

func TestStatusCmd_FlagsRegistered(t *testing.T) {
	var statusCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "status" {
			statusCmd = cmd
			break
		}
	}
	require.NotNil(t, statusCmd, "status command must exist")

	assert.NotNil(t, statusCmd.Flags().Lookup("phase"), "--phase flag must be registered")
	assert.NotNil(t, statusCmd.Flags().Lookup("json"), "--json flag must be registered")
	assert.NotNil(t, statusCmd.Flags().Lookup("verbose"), "--verbose flag must be registered")
}

// --- phaseNameFor tests -------------------------------------------------------

func TestPhaseNameFor_Found(t *testing.T) {
	t.Parallel()

	phases := []task.Phase{
		{ID: 1, Name: "Foundation", StartTask: "T-001", EndTask: "T-010"},
		{ID: 2, Name: "Implementation", StartTask: "T-011", EndTask: "T-020"},
	}

	assert.Equal(t, "Foundation", phaseNameFor(phases, 1))
	assert.Equal(t, "Implementation", phaseNameFor(phases, 2))
}

func TestPhaseNameFor_NotFound(t *testing.T) {
	t.Parallel()

	phases := []task.Phase{
		{ID: 1, Name: "Foundation", StartTask: "T-001", EndTask: "T-010"},
	}

	result := phaseNameFor(phases, 99)
	assert.Equal(t, "Phase 99", result)
}

func TestPhaseNameFor_ZeroID(t *testing.T) {
	t.Parallel()

	phases := []task.Phase{{ID: 1, Name: "Foundation", StartTask: "T-001", EndTask: "T-010"}}
	assert.Equal(t, "", phaseNameFor(phases, 0))
}

// --- currentPhaseID tests -----------------------------------------------------

func TestCurrentPhaseID_FirstIncompletePhase(t *testing.T) {
	t.Parallel()

	allProgress := []task.PhaseProgress{
		{PhaseID: 1, Total: 10, Completed: 10},               // complete
		{PhaseID: 2, Total: 10, Completed: 5, NotStarted: 5}, // in progress
		{PhaseID: 3, Total: 10, NotStarted: 10},              // not started
	}

	assert.Equal(t, 2, currentPhaseID(allProgress))
}

func TestCurrentPhaseID_AllComplete(t *testing.T) {
	t.Parallel()

	allProgress := []task.PhaseProgress{
		{PhaseID: 1, Total: 5, Completed: 5},
		{PhaseID: 2, Total: 5, Completed: 5},
	}

	assert.Equal(t, 0, currentPhaseID(allProgress))
}

func TestCurrentPhaseID_Empty(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, currentPhaseID(nil))
}

// --- buildUngroupedProgress tests ---------------------------------------------

func TestBuildUngroupedProgress_MixedStatuses(t *testing.T) {
	t.Parallel()

	specs := []*task.ParsedTaskSpec{
		{ID: "T-001"},
		{ID: "T-002"},
		{ID: "T-003"},
		{ID: "T-004"},
		{ID: "T-005"},
	}

	stateMap := map[string]*task.TaskState{
		"T-001": {TaskID: "T-001", Status: task.StatusCompleted},
		"T-002": {TaskID: "T-002", Status: task.StatusInProgress},
		"T-003": {TaskID: "T-003", Status: task.StatusBlocked},
		"T-004": {TaskID: "T-004", Status: task.StatusSkipped},
		// T-005 has no entry -> not_started
	}

	prog := buildUngroupedProgress(specs, stateMap)

	assert.Equal(t, 0, prog.PhaseID)
	assert.Equal(t, 5, prog.Total)
	assert.Equal(t, 1, prog.Completed)
	assert.Equal(t, 1, prog.InProgress)
	assert.Equal(t, 1, prog.Blocked)
	assert.Equal(t, 1, prog.Skipped)
	assert.Equal(t, 1, prog.NotStarted)
}

func TestBuildUngroupedProgress_Empty(t *testing.T) {
	t.Parallel()

	prog := buildUngroupedProgress(nil, nil)
	assert.Equal(t, 0, prog.Total)
	assert.Equal(t, 0, prog.Completed)
}
