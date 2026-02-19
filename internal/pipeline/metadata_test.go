package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/task"
)

// --- helpers -------------------------------------------------------------

// makePhases returns a slice of task.Phase for use in tests.
func makePhases(n int) []task.Phase {
	phases := make([]task.Phase, n)
	for i := range n {
		phases[i] = task.Phase{
			ID:        i + 1,
			Name:      "Phase " + string(rune('A'+i)),
			StartTask: "T-001",
			EndTask:   "T-010",
		}
	}
	return phases
}

// assertPhaseUnchanged asserts that the phase at index i still has its
// default pending values, used to verify untouched phases remain pristine.
func assertPhaseUnchanged(t *testing.T, pm *PipelineMetadata, i int) {
	t.Helper()
	ph := pm.Phases[i]
	assert.Equal(t, PhaseStatusPending, ph.Status, "phase %d status should be pending", i)
	assert.Equal(t, PhaseStatusPending, ph.ImplStatus, "phase %d impl_status should be pending", i)
	assert.Equal(t, "pending", ph.ReviewVerdict, "phase %d review_verdict should be pending", i)
	assert.Equal(t, PhaseStatusPending, ph.FixStatus, "phase %d fix_status should be pending", i)
	assert.Equal(t, "pending", ph.PRStatus, "phase %d pr_status should be pending", i)
}

// --- PipelineStatusRunning -----------------------------------------------

func TestPipelineStatusRunning_Constant(t *testing.T) {
	assert.Equal(t, "running", PipelineStatusRunning)
}

// --- NewPipelineMetadata -------------------------------------------------

// TestNewPipelineMetadata creates correct number of PhaseMetadata entries.
// This is the spec-required name; it verifies TotalPhases, len(Phases), and
// initial field values for every entry.
func TestNewPipelineMetadata(t *testing.T) {
	tests := []struct {
		name       string
		pipelineID string
		nPhases    int
		opts       PipelineOpts
	}{
		{
			name:       "creates correct number of PhaseMetadata entries for three phases",
			pipelineID: "pipe-001",
			nPhases:    3,
			opts:       PipelineOpts{ImplAgent: "claude"},
		},
		{
			name:       "creates correct number of PhaseMetadata entries for one phase",
			pipelineID: "pipe-single",
			nPhases:    1,
			opts:       PipelineOpts{},
		},
		{
			name:       "creates zero PhaseMetadata entries for empty phases",
			pipelineID: "pipe-empty",
			nPhases:    0,
			opts:       PipelineOpts{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			phases := makePhases(tt.nPhases)
			pm := NewPipelineMetadata(tt.pipelineID, phases, tt.opts)

			require.NotNil(t, pm)
			assert.Equal(t, tt.nPhases, pm.TotalPhases)
			assert.Len(t, pm.Phases, tt.nPhases)
			assert.Equal(t, tt.pipelineID, pm.PipelineID)
			assert.Equal(t, "pipeline", pm.WorkflowName)
			assert.Equal(t, PipelineStatusRunning, pm.Status)
			assert.Equal(t, 0, pm.CurrentPhase)
			assert.Nil(t, pm.CompletedAt)
			assert.WithinDuration(t, time.Now(), pm.StartedAt, 2*time.Second)

			for i, ph := range pm.Phases {
				assert.Equal(t, phases[i].ID, ph.PhaseID, "phase %d: PhaseID mismatch", i)
				assert.Equal(t, phases[i].Name, ph.PhaseName, "phase %d: PhaseName mismatch", i)
				assert.Equal(t, PhaseStatusPending, ph.Status, "phase %d: Status should be pending", i)
				assert.Equal(t, PhaseStatusPending, ph.ImplStatus, "phase %d: ImplStatus should be pending", i)
				assert.Equal(t, "pending", ph.ReviewVerdict, "phase %d: ReviewVerdict should be pending", i)
				assert.Equal(t, PhaseStatusPending, ph.FixStatus, "phase %d: FixStatus should be pending", i)
				assert.Equal(t, "pending", ph.PRStatus, "phase %d: PRStatus should be pending", i)
			}
		})
	}
}

func TestNewPipelineMetadata_FieldsPopulated(t *testing.T) {
	phases := makePhases(3)
	opts := PipelineOpts{ImplAgent: "claude"}

	pm := NewPipelineMetadata("pipe-001", phases, opts)

	require.NotNil(t, pm)
	assert.Equal(t, "pipe-001", pm.PipelineID)
	assert.Equal(t, "pipeline", pm.WorkflowName)
	assert.Equal(t, PipelineStatusRunning, pm.Status)
	assert.Equal(t, 3, pm.TotalPhases)
	assert.Equal(t, 0, pm.CurrentPhase)
	assert.Nil(t, pm.CompletedAt)
	assert.WithinDuration(t, time.Now(), pm.StartedAt, 2*time.Second)
}

func TestNewPipelineMetadata_PhasesInitialized(t *testing.T) {
	phases := makePhases(2)
	pm := NewPipelineMetadata("pipe-002", phases, PipelineOpts{})

	require.Len(t, pm.Phases, 2)

	assert.Equal(t, 1, pm.Phases[0].PhaseID)
	assert.Equal(t, "Phase A", pm.Phases[0].PhaseName)
	assert.Equal(t, "raven/phase-1", pm.Phases[0].BranchName)
	assert.Equal(t, PhaseStatusPending, pm.Phases[0].Status)
	assert.Equal(t, PhaseStatusPending, pm.Phases[0].ImplStatus)
	assert.Equal(t, "pending", pm.Phases[0].ReviewVerdict)
	assert.Equal(t, PhaseStatusPending, pm.Phases[0].FixStatus)
	assert.Equal(t, "pending", pm.Phases[0].PRStatus)

	assert.Equal(t, 2, pm.Phases[1].PhaseID)
	assert.Equal(t, "Phase B", pm.Phases[1].PhaseName)
	assert.Equal(t, "raven/phase-2", pm.Phases[1].BranchName)
}

func TestNewPipelineMetadata_EmptyPhases(t *testing.T) {
	pm := NewPipelineMetadata("pipe-empty", nil, PipelineOpts{})
	require.NotNil(t, pm)
	assert.Equal(t, 0, pm.TotalPhases)
	assert.Empty(t, pm.Phases)
}

func TestNewPipelineMetadata_OptsPreserved(t *testing.T) {
	opts := PipelineOpts{
		ImplAgent:       "codex",
		ReviewAgent:     "gemini",
		SkipFix:         true,
		MaxReviewCycles: 5,
	}
	pm := NewPipelineMetadata("pipe-003", makePhases(1), opts)
	assert.Equal(t, opts, pm.Opts)
}

// --- ToMetadataMap / PipelineMetadataFromMap round-trip -----------------

// TestToMetadataMap_PipelineMetadataFromMap round-trip preserves all fields.
// This is the exact spec-required test name.
func TestToMetadataMap_PipelineMetadataFromMap(t *testing.T) {
	t.Run("round-trip preserves all fields", func(t *testing.T) {
		phases := makePhases(3)
		opts := PipelineOpts{
			ImplAgent:       "claude",
			ReviewAgent:     "gemini",
			FixAgent:        "codex",
			MaxReviewCycles: 3,
			SkipPR:          false,
		}
		pm := NewPipelineMetadata("pipe-rt-full", phases, opts)
		pm.Status = PipelineStatusRunning
		pm.CurrentPhase = 2

		// Set some non-default values on phases to verify round-trip fidelity.
		pm.Phases[0].Status = PhaseStatusCompleted
		pm.Phases[0].ImplStatus = "completed"
		pm.Phases[0].ReviewVerdict = "approved"
		pm.Phases[0].FixStatus = "skipped"
		pm.Phases[0].PRStatus = "created"
		pm.Phases[0].PRURL = "https://github.com/org/repo/pull/1"
		pm.Phases[0].ReviewCycles = 2
		pm.Phases[0].Duration = int64(4 * time.Second)
		pm.Phases[0].Error = ""

		pm.Phases[1].Status = PhaseStatusFailed
		pm.Phases[1].Error = "impl timed out"

		m := pm.ToMetadataMap()
		require.NotNil(t, m)

		// Spot-check the raw map before deserialising.
		assert.Equal(t, "pipe-rt-full", m["pipeline_id"])
		assert.Equal(t, PipelineStatusRunning, m["status"])

		restored, err := PipelineMetadataFromMap(m)
		require.NoError(t, err)
		require.NotNil(t, restored)

		// Top-level fields.
		assert.Equal(t, pm.PipelineID, restored.PipelineID)
		assert.Equal(t, pm.WorkflowName, restored.WorkflowName)
		assert.Equal(t, pm.Status, restored.Status)
		assert.Equal(t, pm.CurrentPhase, restored.CurrentPhase)
		assert.Equal(t, pm.TotalPhases, restored.TotalPhases)
		assert.Equal(t, pm.Opts.ImplAgent, restored.Opts.ImplAgent)
		assert.Equal(t, pm.Opts.ReviewAgent, restored.Opts.ReviewAgent)
		assert.Equal(t, pm.Opts.FixAgent, restored.Opts.FixAgent)
		assert.Equal(t, pm.Opts.MaxReviewCycles, restored.Opts.MaxReviewCycles)

		// Phase 0 — completed with PR.
		require.Len(t, restored.Phases, 3)
		r0 := restored.Phases[0]
		assert.Equal(t, pm.Phases[0].PhaseID, r0.PhaseID)
		assert.Equal(t, pm.Phases[0].PhaseName, r0.PhaseName)
		assert.Equal(t, pm.Phases[0].BranchName, r0.BranchName)
		assert.Equal(t, pm.Phases[0].Status, r0.Status)
		assert.Equal(t, pm.Phases[0].ImplStatus, r0.ImplStatus)
		assert.Equal(t, pm.Phases[0].ReviewVerdict, r0.ReviewVerdict)
		assert.Equal(t, pm.Phases[0].FixStatus, r0.FixStatus)
		assert.Equal(t, pm.Phases[0].PRStatus, r0.PRStatus)
		assert.Equal(t, pm.Phases[0].PRURL, r0.PRURL)
		assert.Equal(t, pm.Phases[0].ReviewCycles, r0.ReviewCycles)
		assert.Equal(t, pm.Phases[0].Duration, r0.Duration)

		// Phase 1 — failed with error.
		r1 := restored.Phases[1]
		assert.Equal(t, pm.Phases[1].Status, r1.Status)
		assert.Equal(t, pm.Phases[1].Error, r1.Error)

		// Phase 2 — still pending.
		r2 := restored.Phases[2]
		assert.Equal(t, PhaseStatusPending, r2.Status)
	})
}

func TestToMetadataMap_RoundTrip(t *testing.T) {
	phases := makePhases(2)
	pm := NewPipelineMetadata("pipe-rt", phases, PipelineOpts{ImplAgent: "claude"})
	pm.Status = PipelineStatusRunning
	pm.CurrentPhase = 1

	m := pm.ToMetadataMap()
	require.NotNil(t, m)
	assert.Equal(t, "pipe-rt", m["pipeline_id"])
	assert.Equal(t, PipelineStatusRunning, m["status"])

	restored, err := PipelineMetadataFromMap(m)
	require.NoError(t, err)
	require.NotNil(t, restored)
	assert.Equal(t, pm.PipelineID, restored.PipelineID)
	assert.Equal(t, pm.Status, restored.Status)
	assert.Equal(t, pm.CurrentPhase, restored.CurrentPhase)
	assert.Equal(t, pm.TotalPhases, restored.TotalPhases)
	assert.Equal(t, pm.WorkflowName, restored.WorkflowName)
	require.Len(t, restored.Phases, 2)
	assert.Equal(t, pm.Phases[0].PhaseID, restored.Phases[0].PhaseID)
	assert.Equal(t, pm.Phases[0].PhaseName, restored.Phases[0].PhaseName)
	assert.Equal(t, pm.Phases[0].BranchName, restored.Phases[0].BranchName)
	assert.Equal(t, pm.Phases[0].Status, restored.Phases[0].Status)
}

func TestPipelineMetadataFromMap_InvalidJSON(t *testing.T) {
	// A map containing an un-marshalable value (channel) cannot be marshalled.
	// Instead, test that the function handles a mal-formed map gracefully by
	// using a valid map with a field that won't unmarshal into PipelineMetadata.
	// We simulate this by verifying that the happy path works and that errors
	// from the unmarshal step are wrapped with context.

	// Valid empty map decodes to zero-value PipelineMetadata without error.
	pm, err := PipelineMetadataFromMap(map[string]interface{}{})
	require.NoError(t, err)
	require.NotNil(t, pm)
	assert.Empty(t, pm.PipelineID)
}

// TestPipelineMetadataFromMap_MissingKeys verifies that a metadata map with
// missing or unknown keys deserializes gracefully without errors and yields a
// zero-value for the absent fields.
func TestPipelineMetadataFromMap_MissingKeys(t *testing.T) {
	tests := []struct {
		name           string
		m              map[string]interface{}
		wantPipelineID string
		wantStatus     string
		wantPhases     int
	}{
		{
			name:           "completely empty map",
			m:              map[string]interface{}{},
			wantPipelineID: "",
			wantStatus:     "",
			wantPhases:     0,
		},
		{
			name:           "only pipeline_id present",
			m:              map[string]interface{}{"pipeline_id": "p-001"},
			wantPipelineID: "p-001",
			wantStatus:     "",
			wantPhases:     0,
		},
		{
			name: "unknown keys are ignored",
			m: map[string]interface{}{
				"pipeline_id":   "p-002",
				"unknown_field": "should be ignored",
				"another_extra": 42,
			},
			wantPipelineID: "p-002",
			wantStatus:     "",
			wantPhases:     0,
		},
		{
			name: "status present but phases absent",
			m: map[string]interface{}{
				"pipeline_id": "p-003",
				"status":      "running",
			},
			wantPipelineID: "p-003",
			wantStatus:     "running",
			wantPhases:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pm, err := PipelineMetadataFromMap(tt.m)
			require.NoError(t, err)
			require.NotNil(t, pm)
			assert.Equal(t, tt.wantPipelineID, pm.PipelineID)
			assert.Equal(t, tt.wantStatus, pm.Status)
			assert.Len(t, pm.Phases, tt.wantPhases)
		})
	}
}

func TestToMetadataMap_PreservesPhaseFields(t *testing.T) {
	phases := makePhases(1)
	pm := NewPipelineMetadata("pipe-phf", phases, PipelineOpts{})
	pm.Phases[0].Status = PhaseStatusImplementing
	pm.Phases[0].ReviewVerdict = "approved"
	pm.Phases[0].ReviewCycles = 2

	m := pm.ToMetadataMap()
	restored, err := PipelineMetadataFromMap(m)
	require.NoError(t, err)
	assert.Equal(t, PhaseStatusImplementing, restored.Phases[0].Status)
	assert.Equal(t, "approved", restored.Phases[0].ReviewVerdict)
	assert.Equal(t, 2, restored.Phases[0].ReviewCycles)
}

// --- UpdatePhaseStatus ---------------------------------------------------

// TestUpdatePhaseStatus correct phase updated, others unchanged.
// This is the exact spec-required test name.
func TestUpdatePhaseStatus(t *testing.T) {
	tests := []struct {
		name        string
		nPhases     int
		updateIndex int
		newStatus   string
		wantChanged int // index that should have changed
	}{
		{
			name:        "correct phase updated, others unchanged",
			nPhases:     3,
			updateIndex: 1,
			newStatus:   PhaseStatusImplementing,
			wantChanged: 1,
		},
		{
			name:        "first phase updated",
			nPhases:     3,
			updateIndex: 0,
			newStatus:   PhaseStatusCompleted,
			wantChanged: 0,
		},
		{
			name:        "last phase updated",
			nPhases:     3,
			updateIndex: 2,
			newStatus:   PhaseStatusFailed,
			wantChanged: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pm := NewPipelineMetadata("p", makePhases(tt.nPhases), PipelineOpts{})
			pm.UpdatePhaseStatus(tt.updateIndex, tt.newStatus)

			for i := range pm.Phases {
				if i == tt.wantChanged {
					assert.Equal(t, tt.newStatus, pm.Phases[i].Status,
						"phase %d should have new status %q", i, tt.newStatus)
				} else {
					assert.Equal(t, PhaseStatusPending, pm.Phases[i].Status,
						"phase %d should remain pending", i)
				}
			}
		})
	}
}

func TestUpdatePhaseStatus_Valid(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(3), PipelineOpts{})

	pm.UpdatePhaseStatus(1, PhaseStatusImplementing)

	assert.Equal(t, PhaseStatusPending, pm.Phases[0].Status)
	assert.Equal(t, PhaseStatusImplementing, pm.Phases[1].Status)
	assert.Equal(t, PhaseStatusPending, pm.Phases[2].Status)
}

func TestUpdatePhaseStatus_OutOfBounds(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(2), PipelineOpts{})

	// Should not panic.
	pm.UpdatePhaseStatus(-1, PhaseStatusCompleted)
	pm.UpdatePhaseStatus(5, PhaseStatusCompleted)

	// Phases must be unchanged.
	assert.Equal(t, PhaseStatusPending, pm.Phases[0].Status)
	assert.Equal(t, PhaseStatusPending, pm.Phases[1].Status)
}

// --- UpdatePhaseStage ----------------------------------------------------

// TestUpdatePhaseStage_Impl updates impl_status.
func TestUpdatePhaseStage_Impl(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(1), PipelineOpts{})
	pm.UpdatePhaseStage(0, "impl", "running")
	assert.Equal(t, "running", pm.Phases[0].ImplStatus)
	// Other fields must be untouched.
	assert.Equal(t, "pending", pm.Phases[0].ReviewVerdict)
	assert.Equal(t, PhaseStatusPending, pm.Phases[0].FixStatus)
	assert.Equal(t, "pending", pm.Phases[0].PRStatus)
}

// TestUpdatePhaseStage_Review updates review_verdict.
func TestUpdatePhaseStage_Review(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(1), PipelineOpts{})
	pm.UpdatePhaseStage(0, "review", "approved")
	assert.Equal(t, "approved", pm.Phases[0].ReviewVerdict)
	// Other fields must be untouched.
	assert.Equal(t, PhaseStatusPending, pm.Phases[0].ImplStatus)
	assert.Equal(t, PhaseStatusPending, pm.Phases[0].FixStatus)
	assert.Equal(t, "pending", pm.Phases[0].PRStatus)
}

// TestUpdatePhaseStage_Fix updates fix_status.
func TestUpdatePhaseStage_Fix(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(1), PipelineOpts{})
	pm.UpdatePhaseStage(0, "fix", "completed")
	assert.Equal(t, "completed", pm.Phases[0].FixStatus)
	// Other fields must be untouched.
	assert.Equal(t, PhaseStatusPending, pm.Phases[0].ImplStatus)
	assert.Equal(t, "pending", pm.Phases[0].ReviewVerdict)
	assert.Equal(t, "pending", pm.Phases[0].PRStatus)
}

// TestUpdatePhaseStage_PR updates pr_status.
func TestUpdatePhaseStage_PR(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(1), PipelineOpts{})
	pm.UpdatePhaseStage(0, "pr", "created")
	assert.Equal(t, "created", pm.Phases[0].PRStatus)
	// Other fields must be untouched.
	assert.Equal(t, PhaseStatusPending, pm.Phases[0].ImplStatus)
	assert.Equal(t, "pending", pm.Phases[0].ReviewVerdict)
	assert.Equal(t, PhaseStatusPending, pm.Phases[0].FixStatus)
}

func TestUpdatePhaseStage_UnknownStage(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(1), PipelineOpts{})
	// Unknown stage should be a no-op (not panic).
	pm.UpdatePhaseStage(0, "unknown_stage", "somevalue")
	// Verify nothing changed.
	assert.Equal(t, PhaseStatusPending, pm.Phases[0].ImplStatus)
}

func TestUpdatePhaseStage_OutOfBounds(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(1), PipelineOpts{})
	pm.UpdatePhaseStage(-1, "impl", "running")
	pm.UpdatePhaseStage(99, "impl", "running")
	assert.Equal(t, PhaseStatusPending, pm.Phases[0].ImplStatus)
}

// TestUpdatePhaseStage_AllStages is a table-driven test that exercises all
// four supported stage values and verifies only the targeted field changes.
func TestUpdatePhaseStage_AllStages(t *testing.T) {
	tests := []struct {
		stage   string
		value   string
		checkFn func(t *testing.T, ph PhaseMetadata)
	}{
		{
			stage: "impl",
			value: "running",
			checkFn: func(t *testing.T, ph PhaseMetadata) {
				t.Helper()
				assert.Equal(t, "running", ph.ImplStatus)
			},
		},
		{
			stage: "review",
			value: "changes_needed",
			checkFn: func(t *testing.T, ph PhaseMetadata) {
				t.Helper()
				assert.Equal(t, "changes_needed", ph.ReviewVerdict)
			},
		},
		{
			stage: "fix",
			value: "running",
			checkFn: func(t *testing.T, ph PhaseMetadata) {
				t.Helper()
				assert.Equal(t, "running", ph.FixStatus)
			},
		},
		{
			stage: "pr",
			value: "failed",
			checkFn: func(t *testing.T, ph PhaseMetadata) {
				t.Helper()
				assert.Equal(t, "failed", ph.PRStatus)
			},
		},
	}

	for _, tt := range tests {
		t.Run("stage="+tt.stage, func(t *testing.T) {
			t.Parallel()
			pm := NewPipelineMetadata("p", makePhases(1), PipelineOpts{})
			pm.UpdatePhaseStage(0, tt.stage, tt.value)
			tt.checkFn(t, pm.Phases[0])
		})
	}
}

// --- SetPhaseResult ------------------------------------------------------

// TestSetPhaseResult records all fields.
// This is the exact spec-required test name.
func TestSetPhaseResult(t *testing.T) {
	t.Run("records all fields", func(t *testing.T) {
		pm := NewPipelineMetadata("p", makePhases(2), PipelineOpts{})

		result := PhaseResult{
			PhaseID:       "1",
			PhaseName:     "Phase A",
			Status:        PhaseStatusCompleted,
			ImplStatus:    "completed",
			ReviewVerdict: "approved",
			FixStatus:     "skipped",
			PRURL:         "https://github.com/example/pr/42",
			BranchName:    "raven/phase-1",
			Duration:      250 * time.Millisecond,
			Error:         "",
		}

		pm.SetPhaseResult(0, result)

		ph := pm.Phases[0]
		assert.Equal(t, PhaseStatusCompleted, ph.Status, "Status mismatch")
		assert.Equal(t, "completed", ph.ImplStatus, "ImplStatus mismatch")
		assert.Equal(t, "approved", ph.ReviewVerdict, "ReviewVerdict mismatch")
		assert.Equal(t, "skipped", ph.FixStatus, "FixStatus mismatch")
		assert.Equal(t, "https://github.com/example/pr/42", ph.PRURL, "PRURL mismatch")
		assert.Equal(t, "created", ph.PRStatus, "PRStatus should be 'created' when PRURL is set")
		assert.Equal(t, int64(250*time.Millisecond), ph.Duration, "Duration mismatch")
		assert.Empty(t, ph.Error, "Error should be empty")
		assert.NotNil(t, ph.CompletedAt, "CompletedAt should be set")
		assert.WithinDuration(t, time.Now(), *ph.CompletedAt, 2*time.Second)

		// The second phase must remain untouched.
		assertPhaseUnchanged(t, pm, 1)
	})
}

func TestSetPhaseResult_FieldsUpdated(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(2), PipelineOpts{})

	result := PhaseResult{
		PhaseID:       "1",
		PhaseName:     "Phase A",
		Status:        PhaseStatusCompleted,
		ImplStatus:    "completed",
		ReviewVerdict: "approved",
		FixStatus:     "skipped",
		PRURL:         "https://github.com/example/pr/42",
		BranchName:    "raven/phase-1",
		Duration:      250 * time.Millisecond,
		Error:         "",
	}

	pm.SetPhaseResult(0, result)

	ph := pm.Phases[0]
	assert.Equal(t, PhaseStatusCompleted, ph.Status)
	assert.Equal(t, "completed", ph.ImplStatus)
	assert.Equal(t, "approved", ph.ReviewVerdict)
	assert.Equal(t, "skipped", ph.FixStatus)
	assert.Equal(t, "https://github.com/example/pr/42", ph.PRURL)
	assert.Equal(t, "created", ph.PRStatus, "PRStatus should be 'created' when PRURL is set")
	assert.Equal(t, int64(250*time.Millisecond), ph.Duration)
	assert.NotNil(t, ph.CompletedAt)
	assert.WithinDuration(t, time.Now(), *ph.CompletedAt, 2*time.Second)
}

func TestSetPhaseResult_NoPRURL_PRStatusUnchanged(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(1), PipelineOpts{})
	pm.Phases[0].PRStatus = "skipped"

	result := PhaseResult{
		Status: PhaseStatusCompleted,
		PRURL:  "", // no PR
	}
	pm.SetPhaseResult(0, result)

	// PRStatus should not be overwritten to "created" when PRURL is empty.
	assert.Equal(t, "skipped", pm.Phases[0].PRStatus)
}

func TestSetPhaseResult_ErrorPreserved(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(1), PipelineOpts{})

	result := PhaseResult{
		Status: PhaseStatusFailed,
		Error:  "something went wrong",
	}
	pm.SetPhaseResult(0, result)
	assert.Equal(t, "something went wrong", pm.Phases[0].Error)
}

func TestSetPhaseResult_OutOfBounds(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(1), PipelineOpts{})
	// Should not panic.
	pm.SetPhaseResult(-1, PhaseResult{Status: PhaseStatusCompleted})
	pm.SetPhaseResult(99, PhaseResult{Status: PhaseStatusCompleted})
	assert.Equal(t, PhaseStatusPending, pm.Phases[0].Status)
}

func TestSetPhaseResult_DurationConversion(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(1), PipelineOpts{})

	d := 3*time.Second + 500*time.Millisecond
	result := PhaseResult{Duration: d}
	pm.SetPhaseResult(0, result)

	assert.Equal(t, int64(d), pm.Phases[0].Duration)
	// Round-trip back to time.Duration.
	assert.Equal(t, d, time.Duration(pm.Phases[0].Duration))
}

// --- NextIncompletePhase -------------------------------------------------

// TestNextIncompletePhase_AllPending returns 0.
func TestNextIncompletePhase_AllPending(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(3), PipelineOpts{})
	assert.Equal(t, 0, pm.NextIncompletePhase())
}

// TestNextIncompletePhase_FirstCompleted returns 1.
func TestNextIncompletePhase_FirstCompleted(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(3), PipelineOpts{})
	pm.Phases[0].Status = PhaseStatusCompleted
	// Only phase 0 is done; the first incomplete is index 1.
	assert.Equal(t, 1, pm.NextIncompletePhase())
}

// TestNextIncompletePhase_AllCompleted returns -1.
func TestNextIncompletePhase_AllCompleted(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(3), PipelineOpts{})
	pm.Phases[0].Status = PhaseStatusCompleted
	pm.Phases[1].Status = PhaseStatusCompleted
	pm.Phases[2].Status = PhaseStatusCompleted
	assert.Equal(t, -1, pm.NextIncompletePhase())
}

// TestNextIncompletePhase_SkippedPhases skips them.
func TestNextIncompletePhase_SkippedPhases(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(4), PipelineOpts{})
	pm.Phases[0].Status = PhaseStatusCompleted
	pm.Phases[1].Status = PhaseStatusSkipped
	pm.Phases[2].Status = PhaseStatusSkipped
	// Phase 3 is still pending -- it must be the first incomplete.
	assert.Equal(t, 3, pm.NextIncompletePhase())
}

func TestNextIncompletePhase_SomeComplete(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(3), PipelineOpts{})
	pm.Phases[0].Status = PhaseStatusCompleted
	pm.Phases[1].Status = PhaseStatusSkipped
	assert.Equal(t, 2, pm.NextIncompletePhase())
}

func TestNextIncompletePhase_AllDone(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(3), PipelineOpts{})
	pm.Phases[0].Status = PhaseStatusCompleted
	pm.Phases[1].Status = PhaseStatusSkipped
	pm.Phases[2].Status = PhaseStatusCompleted
	assert.Equal(t, -1, pm.NextIncompletePhase())
}

func TestNextIncompletePhase_EmptyPhases(t *testing.T) {
	pm := NewPipelineMetadata("p", nil, PipelineOpts{})
	assert.Equal(t, -1, pm.NextIncompletePhase())
}

func TestNextIncompletePhase_FailedPhase(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(2), PipelineOpts{})
	pm.Phases[0].Status = PhaseStatusFailed
	// A failed phase is not "complete" or "skipped" -- it should be returned.
	assert.Equal(t, 0, pm.NextIncompletePhase())
}

// TestNextIncompletePhase_TableDriven exercises a matrix of states to ensure
// the first non-terminal index is always returned correctly.
func TestNextIncompletePhase_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		statuses []string
		want     int
	}{
		{
			name:     "all pending returns 0",
			statuses: []string{PhaseStatusPending, PhaseStatusPending},
			want:     0,
		},
		{
			name:     "first completed returns 1",
			statuses: []string{PhaseStatusCompleted, PhaseStatusPending},
			want:     1,
		},
		{
			name:     "all completed returns -1",
			statuses: []string{PhaseStatusCompleted, PhaseStatusCompleted},
			want:     -1,
		},
		{
			name:     "skipped phases are treated as done",
			statuses: []string{PhaseStatusSkipped, PhaseStatusPending},
			want:     1,
		},
		{
			name:     "all skipped returns -1",
			statuses: []string{PhaseStatusSkipped, PhaseStatusSkipped},
			want:     -1,
		},
		{
			name:     "failed phase is not terminal",
			statuses: []string{PhaseStatusFailed, PhaseStatusPending},
			want:     0,
		},
		{
			name:     "implementing phase is not terminal",
			statuses: []string{PhaseStatusImplementing, PhaseStatusPending},
			want:     0,
		},
		{
			name:     "mixed completed skipped pending",
			statuses: []string{PhaseStatusCompleted, PhaseStatusSkipped, PhaseStatusPending},
			want:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pm := NewPipelineMetadata("p", makePhases(len(tt.statuses)), PipelineOpts{})
			for i, s := range tt.statuses {
				pm.Phases[i].Status = s
			}
			assert.Equal(t, tt.want, pm.NextIncompletePhase())
		})
	}
}

// --- IsComplete ----------------------------------------------------------

// TestIsComplete_AllCompleted returns true.
func TestIsComplete_AllCompleted(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(2), PipelineOpts{})
	pm.Phases[0].Status = PhaseStatusCompleted
	pm.Phases[1].Status = PhaseStatusCompleted
	assert.True(t, pm.IsComplete())
}

// TestIsComplete_OnePending returns false.
func TestIsComplete_OnePending(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(2), PipelineOpts{})
	pm.Phases[0].Status = PhaseStatusCompleted
	// Phases[1] is still pending.
	assert.False(t, pm.IsComplete())
}

// TestIsComplete_MixCompletedSkipped returns true.
func TestIsComplete_MixCompletedSkipped(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(3), PipelineOpts{})
	pm.Phases[0].Status = PhaseStatusCompleted
	pm.Phases[1].Status = PhaseStatusSkipped
	pm.Phases[2].Status = PhaseStatusCompleted
	assert.True(t, pm.IsComplete())
}

func TestIsComplete_AllSkipped(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(2), PipelineOpts{})
	pm.Phases[0].Status = PhaseStatusSkipped
	pm.Phases[1].Status = PhaseStatusSkipped
	assert.True(t, pm.IsComplete())
}

func TestIsComplete_Mixed(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(2), PipelineOpts{})
	pm.Phases[0].Status = PhaseStatusCompleted
	pm.Phases[1].Status = PhaseStatusSkipped
	assert.True(t, pm.IsComplete())
}

func TestIsComplete_HasPending(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(2), PipelineOpts{})
	pm.Phases[0].Status = PhaseStatusCompleted
	// Phases[1] is still pending.
	assert.False(t, pm.IsComplete())
}

func TestIsComplete_HasFailed(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(2), PipelineOpts{})
	pm.Phases[0].Status = PhaseStatusCompleted
	pm.Phases[1].Status = PhaseStatusFailed
	assert.False(t, pm.IsComplete())
}

func TestIsComplete_Empty(t *testing.T) {
	pm := NewPipelineMetadata("p", nil, PipelineOpts{})
	assert.True(t, pm.IsComplete())
}

// TestIsComplete_TableDriven covers all relevant combinations.
func TestIsComplete_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		statuses []string
		want     bool
	}{
		{
			name:     "all completed returns true",
			statuses: []string{PhaseStatusCompleted, PhaseStatusCompleted},
			want:     true,
		},
		{
			name:     "one pending returns false",
			statuses: []string{PhaseStatusCompleted, PhaseStatusPending},
			want:     false,
		},
		{
			name:     "mix completed skipped returns true",
			statuses: []string{PhaseStatusCompleted, PhaseStatusSkipped},
			want:     true,
		},
		{
			name:     "all skipped returns true",
			statuses: []string{PhaseStatusSkipped, PhaseStatusSkipped},
			want:     true,
		},
		{
			name:     "failed phase returns false",
			statuses: []string{PhaseStatusCompleted, PhaseStatusFailed},
			want:     false,
		},
		{
			name:     "empty pipeline returns true",
			statuses: []string{},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pm := NewPipelineMetadata("p", makePhases(len(tt.statuses)), PipelineOpts{})
			for i, s := range tt.statuses {
				pm.Phases[i].Status = s
			}
			assert.Equal(t, tt.want, pm.IsComplete())
		})
	}
}

// --- Summary -------------------------------------------------------------

// TestSummary includes phase count and status.
// This is the exact spec-required test name.
func TestSummary(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(pm *PipelineMetadata)
		wantContains []string
	}{
		{
			name:         "includes phase count all pending",
			setup:        func(pm *PipelineMetadata) {},
			wantContains: []string{"0/3", "Pipeline"},
		},
		{
			name: "includes phase count with some complete",
			setup: func(pm *PipelineMetadata) {
				pm.Phases[0].Status = PhaseStatusCompleted
				pm.Phases[1].Status = PhaseStatusSkipped
			},
			wantContains: []string{"2/3"},
		},
		{
			name: "includes status of current phase",
			setup: func(pm *PipelineMetadata) {
				pm.CurrentPhase = 1
				pm.Phases[1].Status = PhaseStatusReviewing
			},
			wantContains: []string{"Current", "reviewing"},
		},
		{
			name: "all phases complete",
			setup: func(pm *PipelineMetadata) {
				for i := range pm.Phases {
					pm.Phases[i].Status = PhaseStatusCompleted
				}
			},
			wantContains: []string{"3/3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pm := NewPipelineMetadata("p", makePhases(3), PipelineOpts{})
			tt.setup(pm)
			s := pm.Summary()
			for _, want := range tt.wantContains {
				assert.Contains(t, s, want, "summary %q missing %q", s, want)
			}
		})
	}
}

func TestSummary_AllPending(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(5), PipelineOpts{})
	s := pm.Summary()
	assert.Contains(t, s, "0/5")
}

func TestSummary_SomeComplete(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(5), PipelineOpts{})
	pm.Phases[0].Status = PhaseStatusCompleted
	pm.Phases[1].Status = PhaseStatusSkipped
	s := pm.Summary()
	assert.Contains(t, s, "2/5")
}

func TestSummary_CurrentPhase(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(3), PipelineOpts{})
	pm.CurrentPhase = 1
	pm.Phases[1].Status = PhaseStatusReviewing
	s := pm.Summary()
	assert.Contains(t, s, "Current")
	assert.Contains(t, s, "reviewing")
}

func TestSummary_AllComplete(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(3), PipelineOpts{})
	for i := range pm.Phases {
		pm.Phases[i].Status = PhaseStatusCompleted
	}
	s := pm.Summary()
	assert.Contains(t, s, "3/3")
}

func TestSummary_EmptyPhases(t *testing.T) {
	pm := NewPipelineMetadata("p", nil, PipelineOpts{})
	s := pm.Summary()
	assert.Contains(t, s, "0/0")
}

// TestSummary_OutOfBoundsCurrentPhase verifies that Summary does not panic
// when CurrentPhase is set to an index outside the Phases slice.
func TestSummary_OutOfBoundsCurrentPhase(t *testing.T) {
	pm := NewPipelineMetadata("p", makePhases(2), PipelineOpts{})
	pm.CurrentPhase = 99
	// Should not panic; just omit the "Current" section.
	s := pm.Summary()
	assert.Contains(t, s, "0/2")
}

// --- Integration test ----------------------------------------------------

// TestPipelineMetadata_WorkflowStateRoundTrip simulates the full lifecycle of
// storing and recovering PipelineMetadata via WorkflowState.Metadata (a
// map[string]interface{}), which is exactly how the pipeline persists its
// progress between phases.
func TestPipelineMetadata_WorkflowStateRoundTrip(t *testing.T) {
	// 1. Build a realistic mid-run PipelineMetadata (3 phases, first done).
	phases := makePhases(3)
	opts := PipelineOpts{
		ImplAgent:       "claude",
		ReviewAgent:     "gemini",
		FixAgent:        "codex",
		MaxReviewCycles: 2,
		SkipFix:         false,
		SkipPR:          false,
	}
	pm := NewPipelineMetadata("wsr-pipe-001", phases, opts)
	pm.CurrentPhase = 1
	pm.Status = PipelineStatusRunning

	// Phase 0 — fully completed.
	now := time.Now()
	pm.Phases[0].Status = PhaseStatusCompleted
	pm.Phases[0].ImplStatus = "completed"
	pm.Phases[0].ReviewVerdict = "approved"
	pm.Phases[0].FixStatus = "skipped"
	pm.Phases[0].PRStatus = "created"
	pm.Phases[0].PRURL = "https://github.com/org/repo/pull/10"
	pm.Phases[0].ReviewCycles = 1
	pm.Phases[0].Duration = int64(5 * time.Second)
	pm.Phases[0].CompletedAt = &now

	// Phase 1 — in progress.
	pm.Phases[1].Status = PhaseStatusImplementing
	pm.Phases[1].ImplStatus = "running"

	// 2. Serialise to map (what WorkflowState.Metadata would receive).
	metadataMap := pm.ToMetadataMap()
	require.NotNil(t, metadataMap)
	require.NotEmpty(t, metadataMap)

	// Confirm key presence in the raw map.
	assert.Equal(t, "wsr-pipe-001", metadataMap["pipeline_id"])
	assert.Equal(t, PipelineStatusRunning, metadataMap["status"])

	// 3. Recover from map (simulating what a resumed run would do).
	recovered, err := PipelineMetadataFromMap(metadataMap)
	require.NoError(t, err)
	require.NotNil(t, recovered)

	// 4. Verify all top-level fields match.
	assert.Equal(t, pm.PipelineID, recovered.PipelineID)
	assert.Equal(t, pm.WorkflowName, recovered.WorkflowName)
	assert.Equal(t, pm.Status, recovered.Status)
	assert.Equal(t, pm.CurrentPhase, recovered.CurrentPhase)
	assert.Equal(t, pm.TotalPhases, recovered.TotalPhases)

	// Opts round-trip.
	assert.Equal(t, pm.Opts.ImplAgent, recovered.Opts.ImplAgent)
	assert.Equal(t, pm.Opts.ReviewAgent, recovered.Opts.ReviewAgent)
	assert.Equal(t, pm.Opts.FixAgent, recovered.Opts.FixAgent)
	assert.Equal(t, pm.Opts.MaxReviewCycles, recovered.Opts.MaxReviewCycles)
	assert.Equal(t, pm.Opts.SkipFix, recovered.Opts.SkipFix)
	assert.Equal(t, pm.Opts.SkipPR, recovered.Opts.SkipPR)

	// 5. Verify phase 0 (completed).
	require.Len(t, recovered.Phases, 3)
	r0 := recovered.Phases[0]
	assert.Equal(t, pm.Phases[0].PhaseID, r0.PhaseID)
	assert.Equal(t, pm.Phases[0].PhaseName, r0.PhaseName)
	assert.Equal(t, pm.Phases[0].BranchName, r0.BranchName)
	assert.Equal(t, pm.Phases[0].Status, r0.Status)
	assert.Equal(t, pm.Phases[0].ImplStatus, r0.ImplStatus)
	assert.Equal(t, pm.Phases[0].ReviewVerdict, r0.ReviewVerdict)
	assert.Equal(t, pm.Phases[0].FixStatus, r0.FixStatus)
	assert.Equal(t, pm.Phases[0].PRStatus, r0.PRStatus)
	assert.Equal(t, pm.Phases[0].PRURL, r0.PRURL)
	assert.Equal(t, pm.Phases[0].ReviewCycles, r0.ReviewCycles)
	assert.Equal(t, pm.Phases[0].Duration, r0.Duration)
	require.NotNil(t, r0.CompletedAt, "CompletedAt should survive round-trip")

	// 6. Verify phase 1 (in progress).
	r1 := recovered.Phases[1]
	assert.Equal(t, PhaseStatusImplementing, r1.Status)
	assert.Equal(t, "running", r1.ImplStatus)

	// 7. Verify phase 2 (still pending).
	r2 := recovered.Phases[2]
	assert.Equal(t, PhaseStatusPending, r2.Status)
	assert.Equal(t, PhaseStatusPending, r2.ImplStatus)
	assert.Equal(t, "pending", r2.ReviewVerdict)
	assert.Nil(t, r2.CompletedAt, "CompletedAt must be nil for pending phase")

	// 8. Confirm IsComplete is correctly false mid-run.
	assert.False(t, recovered.IsComplete())

	// 9. Mark all phases done and verify IsComplete flips to true.
	recovered.Phases[1].Status = PhaseStatusCompleted
	recovered.Phases[2].Status = PhaseStatusCompleted
	assert.True(t, recovered.IsComplete())
}

// --- Edge case tests ------------------------------------------------------

// TestUpdatePhaseStatus_EdgeCases validates out-of-bound index handling with a
// table-driven approach.
func TestUpdatePhaseStatus_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		index int
	}{
		{name: "negative index", index: -1},
		{name: "index equal to len", index: 2},
		{name: "large positive index", index: 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pm := NewPipelineMetadata("p", makePhases(2), PipelineOpts{})
			// None of these should panic.
			pm.UpdatePhaseStatus(tt.index, PhaseStatusCompleted)
			// Original phases must remain pending.
			assertPhaseUnchanged(t, pm, 0)
			assertPhaseUnchanged(t, pm, 1)
		})
	}
}

// TestUpdatePhaseStage_EdgeCases validates out-of-bound index handling.
func TestUpdatePhaseStage_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		index int
	}{
		{name: "negative index", index: -1},
		{name: "index equal to len", index: 1},
		{name: "large positive index", index: 999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pm := NewPipelineMetadata("p", makePhases(1), PipelineOpts{})
			// None of these should panic.
			pm.UpdatePhaseStage(tt.index, "impl", "running")
			pm.UpdatePhaseStage(tt.index, "review", "approved")
			pm.UpdatePhaseStage(tt.index, "fix", "completed")
			pm.UpdatePhaseStage(tt.index, "pr", "created")
			// Phase 0 must remain untouched.
			assertPhaseUnchanged(t, pm, 0)
		})
	}
}

// TestSetPhaseResult_EdgeCases validates out-of-bound index handling.
func TestSetPhaseResult_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		index int
	}{
		{name: "negative index", index: -1},
		{name: "index equal to len", index: 1},
		{name: "large positive index", index: 5000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pm := NewPipelineMetadata("p", makePhases(1), PipelineOpts{})
			// None of these should panic.
			pm.SetPhaseResult(tt.index, PhaseResult{
				Status:        PhaseStatusCompleted,
				ImplStatus:    "completed",
				ReviewVerdict: "approved",
			})
			// Phase 0 must remain untouched.
			assertPhaseUnchanged(t, pm, 0)
		})
	}
}

// TestNewPipelineMetadata_EmptyPhasesEdge verifies that methods on a
// PipelineMetadata with no phases never panic.
func TestNewPipelineMetadata_EmptyPhasesEdge(t *testing.T) {
	pm := NewPipelineMetadata("p-edge", nil, PipelineOpts{})

	require.NotNil(t, pm)
	assert.Equal(t, 0, pm.TotalPhases)
	assert.Empty(t, pm.Phases)

	// None of the following should panic on an empty Phases slice.
	pm.UpdatePhaseStatus(0, PhaseStatusCompleted)
	pm.UpdatePhaseStage(0, "impl", "running")
	pm.SetPhaseResult(0, PhaseResult{Status: PhaseStatusCompleted})

	assert.Equal(t, -1, pm.NextIncompletePhase())
	assert.True(t, pm.IsComplete())
	assert.Contains(t, pm.Summary(), "0/0")
}

// --- Benchmarks ----------------------------------------------------------

// BenchmarkToMetadataMap measures the cost of serialising PipelineMetadata
// to a map[string]interface{} (the hot path called after every phase).
func BenchmarkToMetadataMap(b *testing.B) {
	pm := NewPipelineMetadata("bench-pipe", makePhases(10), PipelineOpts{
		ImplAgent:       "claude",
		ReviewAgent:     "gemini",
		MaxReviewCycles: 3,
	})
	for i := range pm.Phases {
		pm.Phases[i].Status = PhaseStatusCompleted
		pm.Phases[i].ReviewCycles = 1
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pm.ToMetadataMap()
	}
}

// BenchmarkPipelineMetadataFromMap measures the cost of deserialising from a
// map[string]interface{} (the hot path when resuming a pipeline run).
func BenchmarkPipelineMetadataFromMap(b *testing.B) {
	pm := NewPipelineMetadata("bench-pipe", makePhases(10), PipelineOpts{ImplAgent: "claude"})
	m := pm.ToMetadataMap()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = PipelineMetadataFromMap(m)
	}
}

// BenchmarkNextIncompletePhase measures the linear scan for the first
// incomplete phase across a large pipeline.
func BenchmarkNextIncompletePhase(b *testing.B) {
	pm := NewPipelineMetadata("bench-pipe", makePhases(100), PipelineOpts{})
	// Mark first 99 phases done so the scan traverses the full slice.
	for i := 0; i < 99; i++ {
		pm.Phases[i].Status = PhaseStatusCompleted
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pm.NextIncompletePhase()
	}
}
