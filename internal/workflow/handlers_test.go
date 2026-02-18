package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------
// metaString tests
// -----------------------------------------------------------------------

func TestMetaString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		state      *WorkflowState
		key        string
		defaultVal string
		want       string
	}{
		{
			name:       "nil state returns default",
			state:      nil,
			key:        "foo",
			defaultVal: "default",
			want:       "default",
		},
		{
			name:       "nil metadata returns default",
			state:      &WorkflowState{},
			key:        "foo",
			defaultVal: "default",
			want:       "default",
		},
		{
			name: "missing key returns default",
			state: &WorkflowState{
				Metadata: map[string]any{"other": "val"},
			},
			key:        "foo",
			defaultVal: "default",
			want:       "default",
		},
		{
			name: "non-string value returns default",
			state: &WorkflowState{
				Metadata: map[string]any{"foo": 42},
			},
			key:        "foo",
			defaultVal: "default",
			want:       "default",
		},
		{
			name: "string value returned",
			state: &WorkflowState{
				Metadata: map[string]any{"foo": "bar"},
			},
			key:        "foo",
			defaultVal: "default",
			want:       "bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := metaString(tt.state, tt.key, tt.defaultVal)
			assert.Equal(t, tt.want, got)
		})
	}
}

// -----------------------------------------------------------------------
// metaInt tests
// -----------------------------------------------------------------------

func TestMetaInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		state      *WorkflowState
		key        string
		defaultVal int
		want       int
	}{
		{
			name:       "nil state returns default",
			state:      nil,
			key:        "n",
			defaultVal: 99,
			want:       99,
		},
		{
			name:       "missing key returns default",
			state:      &WorkflowState{Metadata: map[string]any{}},
			key:        "n",
			defaultVal: 99,
			want:       99,
		},
		{
			name:       "int value returned",
			state:      &WorkflowState{Metadata: map[string]any{"n": 7}},
			key:        "n",
			defaultVal: 0,
			want:       7,
		},
		{
			name:       "float64 value coerced to int (JSON round-trip)",
			state:      &WorkflowState{Metadata: map[string]any{"n": float64(3)}},
			key:        "n",
			defaultVal: 0,
			want:       3,
		},
		{
			name:       "int64 value coerced to int",
			state:      &WorkflowState{Metadata: map[string]any{"n": int64(5)}},
			key:        "n",
			defaultVal: 0,
			want:       5,
		},
		{
			name:       "non-numeric type returns default",
			state:      &WorkflowState{Metadata: map[string]any{"n": "oops"}},
			key:        "n",
			defaultVal: 42,
			want:       42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := metaInt(tt.state, tt.key, tt.defaultVal)
			assert.Equal(t, tt.want, got)
		})
	}
}

// -----------------------------------------------------------------------
// metaBool tests
// -----------------------------------------------------------------------

func TestMetaBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		state      *WorkflowState
		key        string
		defaultVal bool
		want       bool
	}{
		{
			name:       "nil state returns default",
			state:      nil,
			key:        "b",
			defaultVal: true,
			want:       true,
		},
		{
			name:       "missing key returns default",
			state:      &WorkflowState{Metadata: map[string]any{}},
			key:        "b",
			defaultVal: false,
			want:       false,
		},
		{
			name:       "true value returned",
			state:      &WorkflowState{Metadata: map[string]any{"b": true}},
			key:        "b",
			defaultVal: false,
			want:       true,
		},
		{
			name:       "false value returned",
			state:      &WorkflowState{Metadata: map[string]any{"b": false}},
			key:        "b",
			defaultVal: true,
			want:       false,
		},
		{
			name:       "non-bool type returns default",
			state:      &WorkflowState{Metadata: map[string]any{"b": "yes"}},
			key:        "b",
			defaultVal: true,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := metaBool(tt.state, tt.key, tt.defaultVal)
			assert.Equal(t, tt.want, got)
		})
	}
}

// -----------------------------------------------------------------------
// resolveAgents tests
// -----------------------------------------------------------------------

func TestResolveAgents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state *WorkflowState
		want  []string
	}{
		{
			name:  "nil state returns nil",
			state: nil,
			want:  nil,
		},
		{
			name:  "missing key returns nil",
			state: &WorkflowState{Metadata: map[string]any{}},
			want:  nil,
		},
		{
			name: "[]string value returned as-is",
			state: &WorkflowState{Metadata: map[string]any{
				"review_agents": []string{"claude", "codex"},
			}},
			want: []string{"claude", "codex"},
		},
		{
			name: "comma-separated string split correctly",
			state: &WorkflowState{Metadata: map[string]any{
				"review_agents": "claude, codex, gemini",
			}},
			want: []string{"claude", "codex", "gemini"},
		},
		{
			name: "comma-separated string with extra spaces trimmed",
			state: &WorkflowState{Metadata: map[string]any{
				"review_agents": "  claude  ,  codex  ",
			}},
			want: []string{"claude", "codex"},
		},
		{
			name: "unrecognised type returns nil",
			state: &WorkflowState{Metadata: map[string]any{
				"review_agents": 42,
			}},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resolveAgents(tt.state)
			assert.Equal(t, tt.want, got)
		})
	}
}

// -----------------------------------------------------------------------
// ImplementHandler tests
// -----------------------------------------------------------------------

func TestImplementHandler_NilRunner(t *testing.T) {
	t.Parallel()
	h := &ImplementHandler{}
	state := NewWorkflowState("run-1", "implement", "run_implement")
	event, err := h.Execute(context.Background(), state)
	assert.Equal(t, EventFailure, event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runner not configured")
}

func TestImplementHandler_DryRun(t *testing.T) {
	t.Parallel()
	h := &ImplementHandler{}
	state := NewWorkflowState("run-1", "implement", "run_implement")
	state.Metadata["phase_id"] = 2
	state.Metadata["task_id"] = "T-007"
	state.Metadata["agent_name"] = "claude"
	got := h.DryRun(state)
	assert.Contains(t, got, "phase 2")
	assert.Contains(t, got, "T-007")
	assert.Contains(t, got, "claude")
}

// -----------------------------------------------------------------------
// ReviewHandler tests
// -----------------------------------------------------------------------

func TestReviewHandler_NilOrchestrator(t *testing.T) {
	t.Parallel()
	h := &ReviewHandler{}
	state := NewWorkflowState("run-1", "implement-review-pr", "run_review")
	event, err := h.Execute(context.Background(), state)
	assert.Equal(t, EventFailure, event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "orchestrator not configured")
}

func TestReviewHandler_DryRun(t *testing.T) {
	t.Parallel()
	h := &ReviewHandler{}
	state := NewWorkflowState("run-1", "implement-review-pr", "run_review")
	state.Metadata["base_branch"] = "develop"
	got := h.DryRun(state)
	assert.Contains(t, got, "develop")
}

// -----------------------------------------------------------------------
// CheckReviewHandler tests
// -----------------------------------------------------------------------

func TestCheckReviewHandler_Execute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		verdict     string
		wantEvent   string
		wantErr     bool
	}{
		{
			name:      "approved verdict returns success",
			verdict:   "APPROVED",
			wantEvent: EventSuccess,
		},
		{
			name:      "changes needed returns needs_human",
			verdict:   "CHANGES_NEEDED",
			wantEvent: EventNeedsHuman,
		},
		{
			name:      "blocking returns needs_human",
			verdict:   "BLOCKING",
			wantEvent: EventNeedsHuman,
		},
		{
			name:      "empty verdict treated as approved",
			verdict:   "",
			wantEvent: EventSuccess,
		},
		{
			name:      "unknown verdict treated as approved",
			verdict:   "UNKNOWN",
			wantEvent: EventSuccess,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := &CheckReviewHandler{}
			state := NewWorkflowState("run-1", "implement-review-pr", "check_review")
			if tt.verdict != "" {
				state.Metadata["review_verdict"] = tt.verdict
			}
			event, err := h.Execute(context.Background(), state)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantEvent, event)
		})
	}
}

func TestCheckReviewHandler_ContextCancelled(t *testing.T) {
	t.Parallel()
	h := &CheckReviewHandler{}
	state := NewWorkflowState("run-1", "implement-review-pr", "check_review")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	event, err := h.Execute(ctx, state)
	assert.Equal(t, EventFailure, event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context cancelled")
}

func TestCheckReviewHandler_DryRun(t *testing.T) {
	t.Parallel()
	h := &CheckReviewHandler{}
	state := NewWorkflowState("run-1", "implement-review-pr", "check_review")
	got := h.DryRun(state)
	assert.Contains(t, got, "verdict")
}

// -----------------------------------------------------------------------
// FixHandler tests
// -----------------------------------------------------------------------

func TestFixHandler_NilEngine(t *testing.T) {
	t.Parallel()
	h := &FixHandler{}
	state := NewWorkflowState("run-1", "implement-review-pr", "run_fix")
	event, err := h.Execute(context.Background(), state)
	assert.Equal(t, EventFailure, event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "engine not configured")
}

func TestFixHandler_DryRun(t *testing.T) {
	t.Parallel()
	h := &FixHandler{}
	state := NewWorkflowState("run-1", "implement-review-pr", "run_fix")
	got := h.DryRun(state)
	assert.Contains(t, got, "fix engine")
}

// -----------------------------------------------------------------------
// PRHandler tests
// -----------------------------------------------------------------------

func TestPRHandler_NilCreator(t *testing.T) {
	t.Parallel()
	h := &PRHandler{}
	state := NewWorkflowState("run-1", "implement-review-pr", "create_pr")
	event, err := h.Execute(context.Background(), state)
	assert.Equal(t, EventFailure, event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creator not configured")
}

func TestPRHandler_DryRun(t *testing.T) {
	t.Parallel()
	h := &PRHandler{}
	state := NewWorkflowState("run-1", "implement-review-pr", "create_pr")
	got := h.DryRun(state)
	assert.Contains(t, got, "pull request")
}

// -----------------------------------------------------------------------
// InitPhaseHandler tests
// -----------------------------------------------------------------------

func TestInitPhaseHandler_Execute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		currentPhase  any // stored in metadata
		wantPhaseID   int
	}{
		{
			name:        "sets phase_id from current_phase",
			currentPhase: 3,
			wantPhaseID: 3,
		},
		{
			name:        "defaults to 1 when current_phase absent",
			currentPhase: nil,
			wantPhaseID: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := &InitPhaseHandler{}
			state := NewWorkflowState("run-1", "pipeline", "init_phase")
			if tt.currentPhase != nil {
				state.Metadata["current_phase"] = tt.currentPhase
			}
			event, err := h.Execute(context.Background(), state)
			require.NoError(t, err)
			assert.Equal(t, EventSuccess, event)
			assert.Equal(t, tt.wantPhaseID, state.Metadata["phase_id"])
		})
	}
}

func TestInitPhaseHandler_DryRun(t *testing.T) {
	t.Parallel()
	h := &InitPhaseHandler{}
	state := NewWorkflowState("run-1", "pipeline", "init_phase")
	got := h.DryRun(state)
	assert.Contains(t, got, "phase")
}

// -----------------------------------------------------------------------
// RunPhaseWorkflowHandler tests
// -----------------------------------------------------------------------

func TestRunPhaseWorkflowHandler_NilRunner(t *testing.T) {
	t.Parallel()
	h := &RunPhaseWorkflowHandler{}
	state := NewWorkflowState("run-1", "pipeline", "run_phase_workflow")
	event, err := h.Execute(context.Background(), state)
	assert.Equal(t, EventFailure, event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runner not configured")
}

func TestRunPhaseWorkflowHandler_DryRun(t *testing.T) {
	t.Parallel()
	h := &RunPhaseWorkflowHandler{}
	state := NewWorkflowState("run-1", "pipeline", "run_phase_workflow")
	got := h.DryRun(state)
	assert.Contains(t, got, "phase")
}

// -----------------------------------------------------------------------
// AdvancePhaseHandler tests
// -----------------------------------------------------------------------

func TestAdvancePhaseHandler_Execute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		currentPhase  int
		totalPhases   int
		wantEvent     string
		wantNextPhase int // only checked when EventPartial
	}{
		{
			name:          "last phase returns success",
			currentPhase:  3,
			totalPhases:   3,
			wantEvent:     EventSuccess,
		},
		{
			name:          "more phases returns partial",
			currentPhase:  1,
			totalPhases:   3,
			wantEvent:     EventPartial,
			wantNextPhase: 2,
		},
		{
			name:          "single phase defaults returns success when advanced",
			currentPhase:  1,
			totalPhases:   1,
			wantEvent:     EventSuccess,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := &AdvancePhaseHandler{}
			state := NewWorkflowState("run-1", "pipeline", "advance_phase")
			state.Metadata["current_phase"] = tt.currentPhase
			state.Metadata["total_phases"] = tt.totalPhases

			event, err := h.Execute(context.Background(), state)
			require.NoError(t, err)
			assert.Equal(t, tt.wantEvent, event)

			if tt.wantEvent == EventPartial {
				assert.Equal(t, tt.wantNextPhase, state.Metadata["current_phase"])
			}
		})
	}
}

func TestAdvancePhaseHandler_ContextCancelled(t *testing.T) {
	t.Parallel()
	h := &AdvancePhaseHandler{}
	state := NewWorkflowState("run-1", "pipeline", "advance_phase")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	event, err := h.Execute(ctx, state)
	assert.Equal(t, EventFailure, event)
	require.Error(t, err)
}

func TestAdvancePhaseHandler_DryRun(t *testing.T) {
	t.Parallel()
	h := &AdvancePhaseHandler{}
	state := NewWorkflowState("run-1", "pipeline", "advance_phase")
	got := h.DryRun(state)
	assert.Contains(t, got, "phase")
}

// -----------------------------------------------------------------------
// ShredHandler tests
// -----------------------------------------------------------------------

func TestShredHandler_Execute(t *testing.T) {
	t.Parallel()
	h := &ShredHandler{}
	state := NewWorkflowState("run-1", "prd-decompose", "shred")
	state.Metadata["prd_path"] = "/path/to/PRD.md"

	event, err := h.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.Equal(t, EventSuccess, event)
	assert.Equal(t, true, state.Metadata["shred_complete"])
}

func TestShredHandler_DryRun(t *testing.T) {
	t.Parallel()
	h := &ShredHandler{}
	state := NewWorkflowState("run-1", "prd-decompose", "shred")
	got := h.DryRun(state)
	assert.Contains(t, got, "PRD")
}

// -----------------------------------------------------------------------
// ScatterHandler tests
// -----------------------------------------------------------------------

func TestScatterHandler_Execute(t *testing.T) {
	t.Parallel()
	h := &ScatterHandler{}
	state := NewWorkflowState("run-1", "prd-decompose", "scatter")
	state.Metadata["shred_complete"] = true

	event, err := h.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.Equal(t, EventSuccess, event)
	assert.Equal(t, true, state.Metadata["scatter_complete"])
}

func TestScatterHandler_DryRun(t *testing.T) {
	t.Parallel()
	h := &ScatterHandler{}
	state := NewWorkflowState("run-1", "prd-decompose", "scatter")
	got := h.DryRun(state)
	assert.Contains(t, got, "scatter")
}

// -----------------------------------------------------------------------
// GatherHandler tests
// -----------------------------------------------------------------------

func TestGatherHandler_Execute(t *testing.T) {
	t.Parallel()
	h := &GatherHandler{}
	state := NewWorkflowState("run-1", "prd-decompose", "gather")
	state.Metadata["scatter_complete"] = true

	event, err := h.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.Equal(t, EventSuccess, event)
	assert.Equal(t, true, state.Metadata["gather_complete"])
}

func TestGatherHandler_DryRun(t *testing.T) {
	t.Parallel()
	h := &GatherHandler{}
	state := NewWorkflowState("run-1", "prd-decompose", "gather")
	got := h.DryRun(state)
	assert.Contains(t, got, "gather")
}

// -----------------------------------------------------------------------
// Interface compliance (compile-time; verified by var block in handlers.go)
// -----------------------------------------------------------------------

func TestHandlerNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		handler StepHandler
		want    string
	}{
		{&ImplementHandler{}, "run_implement"},
		{&ReviewHandler{}, "run_review"},
		{&CheckReviewHandler{}, "check_review"},
		{&FixHandler{}, "run_fix"},
		{&PRHandler{}, "create_pr"},
		{&InitPhaseHandler{}, "init_phase"},
		{&RunPhaseWorkflowHandler{}, "run_phase_workflow"},
		{&AdvancePhaseHandler{}, "advance_phase"},
		{&ShredHandler{}, "shred"},
		{&ScatterHandler{}, "scatter"},
		{&GatherHandler{}, "gather"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.handler.Name())
		})
	}
}
