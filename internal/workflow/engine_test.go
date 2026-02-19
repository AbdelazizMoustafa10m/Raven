package workflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Compile-time interface checks
// ---------------------------------------------------------------------------

var _ StepHandler = (*recordingHandler)(nil)
var _ StepHandler = (*panicHandler)(nil)
var _ StepHandler = (*blockingHandler)(nil)
var _ StepHandler = (*metadataHandler)(nil)
var _ StepHandler = (*conditionalHandler)(nil)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// recordingHandler records every Execute call and returns a configurable event.
type recordingHandler struct {
	mu        sync.Mutex
	name      string
	execEvent string
	execErr   error
	calls     []string // ordered list of step names that called Execute
	dryRunMsg string
}

func (h *recordingHandler) Execute(_ context.Context, state *WorkflowState) (string, error) {
	h.mu.Lock()
	h.calls = append(h.calls, state.CurrentStep)
	h.mu.Unlock()
	return h.execEvent, h.execErr
}

func (h *recordingHandler) DryRun(_ *WorkflowState) string { return h.dryRunMsg }
func (h *recordingHandler) Name() string                   { return h.name }

// callCount returns the number of times Execute was called (safe for concurrent use).
func (h *recordingHandler) callCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.calls)
}

// newRecorder returns a recordingHandler that returns EventSuccess on Execute.
func newRecorder(name string) *recordingHandler {
	return &recordingHandler{
		name:      name,
		execEvent: EventSuccess,
		dryRunMsg: "dry-run: " + name,
	}
}

// panicHandler is a StepHandler whose Execute panics.
type panicHandler struct{ name string }

func (p *panicHandler) Execute(_ context.Context, _ *WorkflowState) (string, error) {
	panic("deliberate panic from " + p.name)
}
func (p *panicHandler) DryRun(_ *WorkflowState) string { return "dry-run: " + p.name }
func (p *panicHandler) Name() string                   { return p.name }

// blockingHandler blocks until the context is cancelled.
type blockingHandler struct{ name string }

func (b *blockingHandler) Execute(ctx context.Context, _ *WorkflowState) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}
func (b *blockingHandler) DryRun(_ *WorkflowState) string { return "dry-run: " + b.name }
func (b *blockingHandler) Name() string                   { return b.name }

// metadataHandler reads and writes a key in WorkflowState.Metadata.
// It stores the counter value it observed under "seen-<name>" so downstream
// steps can verify state propagation.
type metadataHandler struct {
	name    string
	setKey  string
	setVal  any
	readKey string
}

func (m *metadataHandler) Execute(_ context.Context, state *WorkflowState) (string, error) {
	if m.readKey != "" {
		// Record what we read from the shared metadata into our own slot.
		state.Metadata["seen-"+m.name] = state.Metadata[m.readKey]
	}
	if m.setKey != "" {
		state.Metadata[m.setKey] = m.setVal
	}
	return EventSuccess, nil
}
func (m *metadataHandler) DryRun(_ *WorkflowState) string { return "dry-run: " + m.name }
func (m *metadataHandler) Name() string                   { return m.name }

// conditionalHandler returns different events based on a Metadata key value.
type conditionalHandler struct {
	name        string
	metadataKey string
	trueEvent   string
	falseEvent  string
}

func (c *conditionalHandler) Execute(_ context.Context, state *WorkflowState) (string, error) {
	v, ok := state.Metadata[c.metadataKey]
	if ok && v == true {
		return c.trueEvent, nil
	}
	return c.falseEvent, nil
}
func (c *conditionalHandler) DryRun(_ *WorkflowState) string { return "dry-run: " + c.name }
func (c *conditionalHandler) Name() string                   { return c.name }

// dryRunTrackingHandler tracks whether DryRun or Execute was called.
type dryRunTrackingHandler struct {
	name         string
	executeCount int
	dryRunCount  int
	mu           sync.Mutex
}

func (d *dryRunTrackingHandler) Execute(_ context.Context, _ *WorkflowState) (string, error) {
	d.mu.Lock()
	d.executeCount++
	d.mu.Unlock()
	return EventSuccess, nil
}
func (d *dryRunTrackingHandler) DryRun(_ *WorkflowState) string {
	d.mu.Lock()
	d.dryRunCount++
	d.mu.Unlock()
	return "dry-run description for " + d.name
}
func (d *dryRunTrackingHandler) Name() string { return d.name }

// linearDef returns a WorkflowDefinition with chained steps:
//
//	names[0] -> names[1] -> ... -> names[n-1] -> __done__
func linearDef(names ...string) *WorkflowDefinition {
	steps := make([]StepDefinition, len(names))
	for i, n := range names {
		next := StepDone
		if i+1 < len(names) {
			next = names[i+1]
		}
		steps[i] = StepDefinition{
			Name:        n,
			Transitions: map[string]string{EventSuccess: next},
		}
	}
	return &WorkflowDefinition{
		Name:        "test-workflow",
		InitialStep: names[0],
		Steps:       steps,
	}
}

// registerAll registers each handler into a fresh registry.
func registerAll(handlers ...StepHandler) *Registry {
	r := NewRegistry()
	for _, h := range handlers {
		r.Register(h)
	}
	return r
}

// collectEvents drains the events channel into a slice.
// It must be called after the channel is closed.
func collectEvents(t *testing.T, ch <-chan WorkflowEvent) []WorkflowEvent {
	t.Helper()
	var evs []WorkflowEvent
	for ev := range ch {
		evs = append(evs, ev)
	}
	return evs
}

// eventTypes extracts the Type field from a slice of WorkflowEvents.
func eventTypes(evs []WorkflowEvent) []string {
	types := make([]string, 0, len(evs))
	for _, ev := range evs {
		types = append(types, ev.Type)
	}
	return types
}

// buildEventSet returns a set (map) of event type strings.
func buildEventSet(evs []WorkflowEvent) map[string]bool {
	m := make(map[string]bool, len(evs))
	for _, ev := range evs {
		m[ev.Type] = true
	}
	return m
}

// ---------------------------------------------------------------------------
// Engine.Run -- linear workflow
// ---------------------------------------------------------------------------

// TestEngine_Run_LinearWorkflow verifies that a three-step linear workflow
// executes every step in order and terminates at StepDone.
func TestEngine_Run_LinearWorkflow(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	b := newRecorder("step-b")
	c := newRecorder("step-c")

	reg := registerAll(a, b, c)
	def := linearDef("step-a", "step-b", "step-c")
	eng := NewEngine(reg)

	state, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err)
	require.NotNil(t, state)

	// All three handlers must have been invoked once.
	assert.Equal(t, 1, a.callCount(), "step-a should execute once")
	assert.Equal(t, 1, b.callCount(), "step-b should execute once")
	assert.Equal(t, 1, c.callCount(), "step-c should execute once")

	// StepHistory must reflect execution order.
	require.Len(t, state.StepHistory, 3)
	assert.Equal(t, "step-a", state.StepHistory[0].Step)
	assert.Equal(t, "step-b", state.StepHistory[1].Step)
	assert.Equal(t, "step-c", state.StepHistory[2].Step)

	// Final current step is the terminal step.
	assert.Equal(t, StepDone, state.CurrentStep)
}

// TestEngine_Run_SingleStepWorkflow verifies that a workflow with a single step
// that transitions directly to StepDone completes successfully.
func TestEngine_Run_SingleStepWorkflow(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	reg := registerAll(a)
	def := linearDef("step-a") // step-a -> __done__

	eng := NewEngine(reg)
	state, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.Equal(t, 1, a.callCount(), "step-a should execute once")
	require.Len(t, state.StepHistory, 1)
	assert.Equal(t, "step-a", state.StepHistory[0].Step)
	assert.Equal(t, EventSuccess, state.StepHistory[0].Event)
	assert.Equal(t, StepDone, state.CurrentStep)
}

// ---------------------------------------------------------------------------
// Engine.Run -- step history and state updates
// ---------------------------------------------------------------------------

// TestEngine_Run_StepHistoryAccumulation verifies that StepRecord entries are
// appended after each step and that all fields are populated correctly.
func TestEngine_Run_StepHistoryAccumulation(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	b := newRecorder("step-b")
	reg := registerAll(a, b)
	def := linearDef("step-a", "step-b")

	eng := NewEngine(reg)
	before := time.Now()
	state, err := eng.Run(context.Background(), def, nil)
	after := time.Now()
	require.NoError(t, err)
	require.NotNil(t, state)

	require.Len(t, state.StepHistory, 2, "exactly two step records must be accumulated")

	for _, rec := range state.StepHistory {
		assert.NotEmpty(t, rec.Step, "step name must be set")
		assert.Equal(t, EventSuccess, rec.Event, "event must be EventSuccess")
		assert.Empty(t, rec.Error, "no error should be recorded")
		assert.True(t, !rec.StartedAt.Before(before) && !rec.StartedAt.After(after),
			"StartedAt must be within the test execution window")
		assert.GreaterOrEqual(t, rec.Duration.Nanoseconds(), int64(0),
			"Duration must be non-negative")
	}
}

// TestEngine_Run_CurrentStepUpdatedAfterTransition verifies that
// WorkflowState.CurrentStep reflects the next step after each transition.
func TestEngine_Run_CurrentStepUpdatedAfterTransition(t *testing.T) {
	t.Parallel()

	// Build step sequence: a -> b -> c -> done.
	// After step-a runs, CurrentStep becomes step-b, and so on.
	// We verify through StepHistory that steps were recorded in the correct order,
	// confirming that state.CurrentStep was advanced after each transition.
	a := newRecorder("step-a")
	b := newRecorder("step-b")
	c := newRecorder("step-c")
	reg := registerAll(a, b, c)
	def := linearDef("step-a", "step-b", "step-c")

	eng := NewEngine(reg)
	state, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err)

	// After the complete run, CurrentStep must be StepDone.
	assert.Equal(t, StepDone, state.CurrentStep)

	// Verify history records show steps executed in order (confirming state was advanced).
	require.Len(t, state.StepHistory, 3)
	assert.Equal(t, "step-a", state.StepHistory[0].Step)
	assert.Equal(t, "step-b", state.StepHistory[1].Step)
	assert.Equal(t, "step-c", state.StepHistory[2].Step)
}

// TestEngine_Run_UpdatedAtAdvancesAfterEachStep verifies that
// WorkflowState.UpdatedAt is updated after each step execution.
func TestEngine_Run_UpdatedAtAdvancesAfterEachStep(t *testing.T) {
	t.Parallel()

	initialState := NewWorkflowState("wf-updatedAt", "test-workflow", "step-a")
	initialUpdatedAt := initialState.UpdatedAt

	// Introduce a small delay to ensure time advances before Run is called.
	time.Sleep(time.Millisecond)

	a := newRecorder("step-a")
	b := newRecorder("step-b")
	reg := registerAll(a, b)
	def := linearDef("step-a", "step-b")

	eng := NewEngine(reg)
	finalState, err := eng.Run(context.Background(), def, initialState)
	require.NoError(t, err)

	assert.True(t, finalState.UpdatedAt.After(initialUpdatedAt),
		"UpdatedAt must be advanced after step execution")
}

// ---------------------------------------------------------------------------
// Engine.Run -- dry-run mode
// ---------------------------------------------------------------------------

// TestEngine_Run_DryRunMode verifies that in dry-run mode DryRun() is called
// on each handler instead of Execute(), and WEStepSkipped events are emitted.
func TestEngine_Run_DryRunMode(t *testing.T) {
	t.Parallel()

	a := &dryRunTrackingHandler{name: "step-a"}
	b := &dryRunTrackingHandler{name: "step-b"}

	reg := registerAll(a, b)
	def := linearDef("step-a", "step-b")

	events := make(chan WorkflowEvent, 64)
	eng := NewEngine(reg, WithDryRun(true), WithEventChannel(events))

	state, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err)
	require.NotNil(t, state)

	// Execute must NOT have been called.
	a.mu.Lock()
	aExec := a.executeCount
	aDry := a.dryRunCount
	a.mu.Unlock()
	b.mu.Lock()
	bExec := b.executeCount
	bDry := b.dryRunCount
	b.mu.Unlock()

	assert.Equal(t, 0, aExec, "dry-run must not call Execute on step-a")
	assert.Equal(t, 0, bExec, "dry-run must not call Execute on step-b")
	assert.Equal(t, 1, aDry, "dry-run must call DryRun on step-a once")
	assert.Equal(t, 1, bDry, "dry-run must call DryRun on step-b once")

	// WEStepSkipped events must be present.
	close(events)
	collected := collectEvents(t, events)
	types := eventTypes(collected)
	assert.Contains(t, types, WEStepSkipped, "dry-run must emit WEStepSkipped events")

	// StepHistory must still be accumulated even in dry-run.
	require.Len(t, state.StepHistory, 2, "dry-run must still record step history")
}

// TestEngine_Run_DryRunMode_TableDriven verifies dry-run behaviour across
// different workflow sizes.
func TestEngine_Run_DryRunMode_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		stepNames []string
	}{
		{name: "single step", stepNames: []string{"step-a"}},
		{name: "two steps", stepNames: []string{"step-a", "step-b"}},
		{name: "five steps", stepNames: []string{"step-a", "step-b", "step-c", "step-d", "step-e"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handlers := make([]*dryRunTrackingHandler, len(tt.stepNames))
			stepHandlers := make([]StepHandler, len(tt.stepNames))
			for i, name := range tt.stepNames {
				h := &dryRunTrackingHandler{name: name}
				handlers[i] = h
				stepHandlers[i] = h
			}

			reg := registerAll(stepHandlers...)
			def := linearDef(tt.stepNames...)
			eng := NewEngine(reg, WithDryRun(true))

			_, err := eng.Run(context.Background(), def, nil)
			require.NoError(t, err)

			for _, h := range handlers {
				h.mu.Lock()
				exec := h.executeCount
				dry := h.dryRunCount
				h.mu.Unlock()
				assert.Equal(t, 0, exec, "Execute must not be called in dry-run for %s", h.name)
				assert.Equal(t, 1, dry, "DryRun must be called exactly once for %s", h.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Engine.Run -- missing handler
// ---------------------------------------------------------------------------

// TestEngine_Run_MissingHandler verifies that the engine returns an error when
// a step's handler is not registered in the registry.
func TestEngine_Run_MissingHandler(t *testing.T) {
	t.Parallel()

	// step-b has no handler registered.
	a := newRecorder("step-a")
	reg := registerAll(a)

	def := linearDef("step-a", "step-b")
	eng := NewEngine(reg)

	state, err := eng.Run(context.Background(), def, nil)
	require.Error(t, err, "engine must return error when handler is missing")
	assert.True(t, errors.Is(err, ErrStepNotFound),
		"error must wrap ErrStepNotFound, got: %v", err)
	require.NotNil(t, state)
	// step-a completed before the error, so it must be in history.
	require.Len(t, state.StepHistory, 1)
	assert.Equal(t, "step-a", state.StepHistory[0].Step)
}

// TestEngine_Run_MissingHandlerAtStart verifies a descriptive error is returned
// when the very first step has no registered handler.
func TestEngine_Run_MissingHandlerAtStart(t *testing.T) {
	t.Parallel()

	reg := NewRegistry() // empty registry

	def := linearDef("step-a")
	eng := NewEngine(reg)

	_, err := eng.Run(context.Background(), def, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrStepNotFound),
		"error must wrap ErrStepNotFound, got: %v", err)
	assert.Contains(t, err.Error(), "step-a", "error must mention the step name")
}

// ---------------------------------------------------------------------------
// Engine.Run -- context cancellation
// ---------------------------------------------------------------------------

// TestEngine_Run_ContextCancelledDuringStep verifies that the engine respects
// context cancellation while a step is executing. The handler receives the
// cancelled context and returns a context error.
func TestEngine_Run_ContextCancelledDuringStep(t *testing.T) {
	t.Parallel()

	// Use a handler that blocks until context is cancelled.
	blocker := &blockingHandler{name: "step-a"}
	reg := registerAll(blocker)

	def := &WorkflowDefinition{
		Name:        "cancel-workflow",
		InitialStep: "step-a",
		Steps: []StepDefinition{
			{
				Name:        "step-a",
				Transitions: map[string]string{EventSuccess: StepDone},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	eng := NewEngine(reg)
	_, err := eng.Run(ctx, def, nil)
	require.Error(t, err)
	assert.True(t,
		errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled),
		"expected context error, got: %v", err)
}

// TestEngine_Run_ContextCancelledBetweenSteps verifies that a pre-cancelled
// context causes the engine to stop before executing any step.
func TestEngine_Run_ContextCancelledBetweenSteps(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	b := newRecorder("step-b")
	reg := registerAll(a, b)
	def := linearDef("step-a", "step-b")

	// Cancel context immediately before running.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	eng := NewEngine(reg)
	_, err := eng.Run(ctx, def, nil)
	require.Error(t, err)
	assert.True(t,
		errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
	// No steps should have been executed because context was already cancelled.
	assert.Equal(t, 0, a.callCount(), "step-a must not execute with pre-cancelled context")
	assert.Equal(t, 0, b.callCount(), "step-b must not execute with pre-cancelled context")
}

// ---------------------------------------------------------------------------
// Engine.Run -- panic recovery
// ---------------------------------------------------------------------------

// TestEngine_Run_PanicRecovery verifies that a panicking handler is converted
// to an error instead of crashing the process. The panic message must appear
// in the step history record so it is not silently discarded.
func TestEngine_Run_PanicRecovery(t *testing.T) {
	t.Parallel()

	pnk := &panicHandler{name: "step-a"}
	reg := registerAll(pnk)

	// No failure transition: engine must return the panic error directly.
	def := &WorkflowDefinition{
		Name:        "panic-workflow",
		InitialStep: "step-a",
		Steps: []StepDefinition{
			{
				Name:        "step-a",
				Transitions: map[string]string{EventSuccess: StepDone},
			},
		},
	}

	eng := NewEngine(reg)
	state, err := eng.Run(context.Background(), def, nil)
	require.Error(t, err, "engine must return an error when a handler panics")
	assert.Contains(t, err.Error(), "panicked", "error must mention panic")

	// Panic must be captured in step history.
	require.NotEmpty(t, state.StepHistory)
	assert.Contains(t, state.StepHistory[0].Error, "panicked",
		"panic message must appear in step history error field")
}

// TestEngine_Run_PanicWithFailureTransition verifies that a panicking handler
// can be handled by a failure transition when one is configured.
func TestEngine_Run_PanicWithFailureTransition(t *testing.T) {
	t.Parallel()

	pnk := &panicHandler{name: "step-a"}
	recovery := newRecorder("step-recovery")
	reg := registerAll(pnk, recovery)

	def := &WorkflowDefinition{
		Name:        "panic-recovery-workflow",
		InitialStep: "step-a",
		Steps: []StepDefinition{
			{
				Name: "step-a",
				Transitions: map[string]string{
					EventSuccess: StepDone,
					EventFailure: "step-recovery",
				},
			},
			{
				Name:        "step-recovery",
				Transitions: map[string]string{EventSuccess: StepDone},
			},
		},
	}

	eng := NewEngine(reg)
	state, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err, "panic with failure transition should not bubble up the error")
	require.NotNil(t, state)
	assert.Equal(t, 1, recovery.callCount(), "recovery step must execute after panic")
}

// ---------------------------------------------------------------------------
// Engine.Run -- workflow ID generation
// ---------------------------------------------------------------------------

// TestEngine_Run_GeneratesWorkflowID verifies that a new workflow state is
// assigned a non-empty ID with the expected prefix when Run creates it.
func TestEngine_Run_GeneratesWorkflowID(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	reg := registerAll(a)
	def := linearDef("step-a")

	eng := NewEngine(reg)
	state, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, state.ID, "engine must generate a workflow ID")
	assert.Contains(t, state.ID, "wf-", "generated ID must have the wf- prefix")
}

// TestEngine_Run_PreservesExistingWorkflowID verifies that when a non-nil state
// is provided, the engine preserves the existing workflow ID.
func TestEngine_Run_PreservesExistingWorkflowID(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	reg := registerAll(a)
	def := linearDef("step-a")

	existing := NewWorkflowState("my-custom-id", "test-workflow", "step-a")
	eng := NewEngine(reg)
	state, err := eng.Run(context.Background(), def, existing)
	require.NoError(t, err)
	assert.Equal(t, "my-custom-id", state.ID, "existing workflow ID must be preserved")
}

// ---------------------------------------------------------------------------
// Engine.Run -- resume from checkpoint
// ---------------------------------------------------------------------------

// TestEngine_Run_Resumption verifies that WEWorkflowResumed is emitted when
// the provided state already has StepHistory entries.
func TestEngine_Run_Resumption(t *testing.T) {
	t.Parallel()

	b := newRecorder("step-b")
	reg := registerAll(b)

	def := &WorkflowDefinition{
		Name:        "resume-workflow",
		InitialStep: "step-a",
		Steps: []StepDefinition{
			{Name: "step-b", Transitions: map[string]string{EventSuccess: StepDone}},
		},
	}

	// Simulate a pre-existing state with one completed step.
	existing := NewWorkflowState("wf-existing", "resume-workflow", "step-b")
	existing.StepHistory = append(existing.StepHistory, StepRecord{
		Step:      "step-a",
		Event:     EventSuccess,
		StartedAt: time.Now(),
	})

	events := make(chan WorkflowEvent, 64)
	eng := NewEngine(reg, WithEventChannel(events))

	_, err := eng.Run(context.Background(), def, existing)
	require.NoError(t, err)

	close(events)
	collected := collectEvents(t, events)
	typeSet := buildEventSet(collected)

	assert.True(t, typeSet[WEWorkflowResumed], "engine must emit WEWorkflowResumed when resuming")
	assert.False(t, typeSet[WEWorkflowStarted],
		"engine must NOT emit WEWorkflowStarted when resuming an existing workflow")
}

// TestEngine_Run_ResumeFromCheckpoint verifies that the engine starts execution
// at state.CurrentStep rather than def.InitialStep when resuming.
func TestEngine_Run_ResumeFromCheckpoint(t *testing.T) {
	t.Parallel()

	// Set up a three-step workflow: step-a -> step-b -> step-c -> done.
	// Simulate a resume from step-b (step-a was already completed).
	stepA := newRecorder("step-a")
	stepB := newRecorder("step-b")
	stepC := newRecorder("step-c")
	reg := registerAll(stepA, stepB, stepC)

	def := &WorkflowDefinition{
		Name:        "checkpoint-workflow",
		InitialStep: "step-a",
		Steps: []StepDefinition{
			{Name: "step-a", Transitions: map[string]string{EventSuccess: "step-b"}},
			{Name: "step-b", Transitions: map[string]string{EventSuccess: "step-c"}},
			{Name: "step-c", Transitions: map[string]string{EventSuccess: StepDone}},
		},
	}

	// Resume from step-b with a pre-existing history entry for step-a.
	existing := NewWorkflowState("wf-resume", "checkpoint-workflow", "step-b")
	existing.StepHistory = append(existing.StepHistory, StepRecord{
		Step:  "step-a",
		Event: EventSuccess,
	})

	eng := NewEngine(reg)
	state, err := eng.Run(context.Background(), def, existing)
	require.NoError(t, err)

	// step-a must NOT have been called again.
	assert.Equal(t, 0, stepA.callCount(), "step-a must not re-execute on resume")
	// step-b and step-c must have executed.
	assert.Equal(t, 1, stepB.callCount(), "step-b must execute on resume")
	assert.Equal(t, 1, stepC.callCount(), "step-c must execute on resume")

	// Final history must include all three records (1 from checkpoint + 2 new).
	require.Len(t, state.StepHistory, 3)
	assert.Equal(t, "step-a", state.StepHistory[0].Step, "pre-existing step-a must be first")
	assert.Equal(t, "step-b", state.StepHistory[1].Step)
	assert.Equal(t, "step-c", state.StepHistory[2].Step)
}

// ---------------------------------------------------------------------------
// Engine.Run -- branching workflow
// ---------------------------------------------------------------------------

// TestEngine_Run_BranchingWorkflow_SuccessPath verifies that a workflow with
// branching transitions follows the success path when the handler returns
// EventSuccess.
func TestEngine_Run_BranchingWorkflow_SuccessPath(t *testing.T) {
	t.Parallel()

	// step-a branches: success -> step-b, failure -> step-c.
	// Handler returns EventSuccess so step-b is the expected path.
	stepA := &recordingHandler{name: "step-a", execEvent: EventSuccess, dryRunMsg: "dry"}
	stepB := newRecorder("step-b") // success branch
	stepC := newRecorder("step-c") // failure branch
	reg := registerAll(stepA, stepB, stepC)

	def := &WorkflowDefinition{
		Name:        "branch-workflow",
		InitialStep: "step-a",
		Steps: []StepDefinition{
			{
				Name: "step-a",
				Transitions: map[string]string{
					EventSuccess: "step-b",
					EventFailure: "step-c",
				},
			},
			{Name: "step-b", Transitions: map[string]string{EventSuccess: StepDone}},
			{Name: "step-c", Transitions: map[string]string{EventSuccess: StepDone}},
		},
	}

	eng := NewEngine(reg)
	state, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, stepA.callCount(), "step-a must execute once")
	assert.Equal(t, 1, stepB.callCount(), "step-b (success path) must execute")
	assert.Equal(t, 0, stepC.callCount(), "step-c (failure path) must NOT execute")
	assert.Equal(t, StepDone, state.CurrentStep)
	require.Len(t, state.StepHistory, 2)
	assert.Equal(t, "step-a", state.StepHistory[0].Step)
	assert.Equal(t, "step-b", state.StepHistory[1].Step)
}

// TestEngine_Run_BranchingWorkflow_FailurePath verifies that a workflow with
// branching transitions follows the failure path when the handler returns
// EventFailure (no error, just the event).
func TestEngine_Run_BranchingWorkflow_FailurePath(t *testing.T) {
	t.Parallel()

	// step-a returns EventFailure (without an error) so step-c is expected.
	stepA := &recordingHandler{name: "step-a", execEvent: EventFailure, dryRunMsg: "dry"}
	stepB := newRecorder("step-b") // success branch
	stepC := newRecorder("step-c") // failure branch
	reg := registerAll(stepA, stepB, stepC)

	def := &WorkflowDefinition{
		Name:        "branch-failure-workflow",
		InitialStep: "step-a",
		Steps: []StepDefinition{
			{
				Name: "step-a",
				Transitions: map[string]string{
					EventSuccess: "step-b",
					EventFailure: "step-c",
				},
			},
			{Name: "step-b", Transitions: map[string]string{EventSuccess: StepDone}},
			{Name: "step-c", Transitions: map[string]string{EventSuccess: StepDone}},
		},
	}

	eng := NewEngine(reg)
	state, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err, "EventFailure event should follow transition, not error out")

	assert.Equal(t, 1, stepA.callCount(), "step-a must execute once")
	assert.Equal(t, 0, stepB.callCount(), "step-b (success path) must NOT execute")
	assert.Equal(t, 1, stepC.callCount(), "step-c (failure path) must execute")
	assert.Equal(t, StepDone, state.CurrentStep)
}

// ---------------------------------------------------------------------------
// Engine.Run -- failure transition
// ---------------------------------------------------------------------------

// TestEngine_Run_FailureTransition verifies that when Execute returns an error
// and a failure transition exists, the engine follows it instead of halting.
func TestEngine_Run_FailureTransition(t *testing.T) {
	t.Parallel()

	failing := &recordingHandler{
		name:      "step-a",
		execEvent: EventFailure,
		execErr:   errors.New("deliberate error"),
		dryRunMsg: "dry",
	}
	cleanup := newRecorder("step-cleanup")
	reg := registerAll(failing, cleanup)

	def := &WorkflowDefinition{
		Name:        "failure-workflow",
		InitialStep: "step-a",
		Steps: []StepDefinition{
			{
				Name: "step-a",
				Transitions: map[string]string{
					EventSuccess: StepDone,
					EventFailure: "step-cleanup",
				},
			},
			{
				Name:        "step-cleanup",
				Transitions: map[string]string{EventSuccess: StepDone},
			},
		},
	}

	eng := NewEngine(reg)
	state, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err, "engine should follow failure transition, not return error")
	require.NotNil(t, state)
	assert.Equal(t, 1, cleanup.callCount(), "cleanup step should be executed after failure")

	// Step history must record the error for step-a.
	require.Len(t, state.StepHistory, 2)
	assert.NotEmpty(t, state.StepHistory[0].Error, "step-a error must be recorded in history")
	assert.Empty(t, state.StepHistory[1].Error, "cleanup step must have no error")
}

// TestEngine_Run_FailureTransitionToStepFailed verifies that a step error
// with a failure transition to StepFailed causes the workflow to fail.
func TestEngine_Run_FailureTransitionToStepFailed(t *testing.T) {
	t.Parallel()

	failing := &recordingHandler{
		name:      "step-a",
		execErr:   errors.New("critical error"),
		dryRunMsg: "dry",
	}
	reg := registerAll(failing)

	def := &WorkflowDefinition{
		Name:        "critical-failure-workflow",
		InitialStep: "step-a",
		Steps: []StepDefinition{
			{
				Name: "step-a",
				Transitions: map[string]string{
					EventSuccess: StepDone,
					EventFailure: StepFailed,
				},
			},
		},
	}

	events := make(chan WorkflowEvent, 64)
	eng := NewEngine(reg, WithEventChannel(events))
	_, err := eng.Run(context.Background(), def, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terminal failure")

	close(events)
	collected := collectEvents(t, events)
	typeSet := buildEventSet(collected)
	assert.True(t, typeSet[WEWorkflowFailed], "WEWorkflowFailed must be emitted")
}

// ---------------------------------------------------------------------------
// Engine.Run -- missing transition for event
// ---------------------------------------------------------------------------

// TestEngine_Run_MissingTransition verifies that the engine returns an error
// when a handler returns an event for which no transition is defined.
func TestEngine_Run_MissingTransition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		returnedEvent string
		transitions   map[string]string
		wantErrMsg    string
	}{
		{
			name:          "blocked event with no transition",
			returnedEvent: EventBlocked,
			transitions:   map[string]string{EventSuccess: StepDone},
			wantErrMsg:    "no transition for event",
		},
		{
			name:          "rate_limited event with no transition",
			returnedEvent: EventRateLimited,
			transitions:   map[string]string{EventSuccess: StepDone},
			wantErrMsg:    "no transition for event",
		},
		{
			name:          "partial event with no transition",
			returnedEvent: EventPartial,
			transitions:   map[string]string{EventSuccess: StepDone},
			wantErrMsg:    "no transition for event",
		},
		{
			name:          "custom event with no transition",
			returnedEvent: "unexpected_custom_event",
			transitions:   map[string]string{EventSuccess: StepDone},
			wantErrMsg:    "no transition for event",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := &recordingHandler{
				name:      "step-a",
				execEvent: tt.returnedEvent,
				dryRunMsg: "dry",
			}
			reg := registerAll(h)

			def := &WorkflowDefinition{
				Name:        "no-trans-workflow",
				InitialStep: "step-a",
				Steps: []StepDefinition{
					{Name: "step-a", Transitions: tt.transitions},
				},
			}

			eng := NewEngine(reg)
			_, err := eng.Run(context.Background(), def, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrMsg,
				"error must mention missing transition")
			assert.Contains(t, err.Error(), "step-a",
				"error must mention the step name")
			assert.Contains(t, err.Error(), tt.returnedEvent,
				"error must mention the event name")
		})
	}
}

// ---------------------------------------------------------------------------
// Engine.Run -- workflow reaches StepFailed
// ---------------------------------------------------------------------------

// TestEngine_Run_WorkflowFailed verifies that the engine returns an error when
// a workflow reaches the StepFailed terminal pseudo-step.
func TestEngine_Run_WorkflowFailed(t *testing.T) {
	t.Parallel()

	h := &recordingHandler{name: "step-a", execEvent: EventFailure, dryRunMsg: "dry"}
	reg := registerAll(h)

	def := &WorkflowDefinition{
		Name:        "failing-workflow",
		InitialStep: "step-a",
		Steps: []StepDefinition{
			{
				Name:        "step-a",
				Transitions: map[string]string{EventFailure: StepFailed},
			},
		},
	}

	eng := NewEngine(reg)
	_, err := eng.Run(context.Background(), def, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terminal failure", "error must mention terminal failure step")
}

// ---------------------------------------------------------------------------
// Engine.Run -- state metadata propagation
// ---------------------------------------------------------------------------

// TestEngine_Run_MetadataPropagatedBetweenSteps verifies that metadata set by
// one step handler is visible to subsequent step handlers.
func TestEngine_Run_MetadataPropagatedBetweenSteps(t *testing.T) {
	t.Parallel()

	// step-writer sets Metadata["result"] = "computed-value".
	// step-reader reads Metadata["result"] and copies it to Metadata["seen-step-reader"].
	writer := &metadataHandler{
		name:   "step-writer",
		setKey: "result",
		setVal: "computed-value",
	}
	reader := &metadataHandler{
		name:    "step-reader",
		readKey: "result",
	}
	reg := registerAll(writer, reader)

	def := &WorkflowDefinition{
		Name:        "metadata-workflow",
		InitialStep: "step-writer",
		Steps: []StepDefinition{
			{Name: "step-writer", Transitions: map[string]string{EventSuccess: "step-reader"}},
			{Name: "step-reader", Transitions: map[string]string{EventSuccess: StepDone}},
		},
	}

	eng := NewEngine(reg)
	state, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err)

	assert.Equal(t, "computed-value", state.Metadata["result"],
		"metadata written by step-writer must be in final state")
	assert.Equal(t, "computed-value", state.Metadata["seen-step-reader"],
		"step-reader must have observed the metadata set by step-writer")
}

// ---------------------------------------------------------------------------
// Engine.Run -- empty workflow definition
// ---------------------------------------------------------------------------

// TestEngine_Run_EmptyWorkflowDefinition verifies that a workflow definition
// with no steps fails validation and/or returns a descriptive error.
func TestEngine_Run_EmptyWorkflowDefinition(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	def := &WorkflowDefinition{
		Name:        "empty-workflow",
		InitialStep: "step-a",
		Steps:       []StepDefinition{},
	}

	eng := NewEngine(reg)
	_, err := eng.Run(context.Background(), def, nil)
	require.Error(t, err, "empty workflow with non-empty InitialStep must return an error")
}

// ---------------------------------------------------------------------------
// Engine.Validate
// ---------------------------------------------------------------------------

// TestEngine_Validate_ValidDefinition verifies Validate returns no errors for a
// well-formed workflow definition.
func TestEngine_Validate_ValidDefinition(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	b := newRecorder("step-b")
	reg := registerAll(a, b)

	def := linearDef("step-a", "step-b")
	eng := NewEngine(reg)

	errs := eng.Validate(def)
	assert.Empty(t, errs, "valid definition must produce no validation errors")
}

// TestEngine_Validate_ValidDefinition_WithTerminalTransitions verifies that
// transitions to StepDone and StepFailed are considered valid targets.
func TestEngine_Validate_ValidDefinition_WithTerminalTransitions(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	reg := registerAll(a)

	def := &WorkflowDefinition{
		Name:        "terminal-trans-workflow",
		InitialStep: "step-a",
		Steps: []StepDefinition{
			{
				Name: "step-a",
				Transitions: map[string]string{
					EventSuccess: StepDone,
					EventFailure: StepFailed,
				},
			},
		},
	}
	eng := NewEngine(reg)
	errs := eng.Validate(def)
	assert.Empty(t, errs, "transitions to terminal pseudo-steps must be valid")
}

// TestEngine_Validate_MissingHandler verifies Validate returns an error when a
// step has no registered handler.
func TestEngine_Validate_MissingHandler(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	reg := registerAll(a)

	// step-b has no handler.
	def := linearDef("step-a", "step-b")
	eng := NewEngine(reg)

	errs := eng.Validate(def)
	require.NotEmpty(t, errs)
	combined := fmt.Sprintf("%v", errs)
	assert.Contains(t, combined, "step-b", "error must name the unregistered step")
}

// TestEngine_Validate_UnknownInitialStep verifies Validate returns an error
// when InitialStep does not match any defined step.
func TestEngine_Validate_UnknownInitialStep(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	reg := registerAll(a)

	def := &WorkflowDefinition{
		Name:        "bad-init",
		InitialStep: "does-not-exist",
		Steps: []StepDefinition{
			{Name: "step-a", Transitions: map[string]string{EventSuccess: StepDone}},
		},
	}
	eng := NewEngine(reg)
	errs := eng.Validate(def)
	require.NotEmpty(t, errs)
	combined := fmt.Sprintf("%v", errs)
	assert.Contains(t, combined, "does-not-exist")
}

// TestEngine_Validate_InvalidTransitionTarget verifies Validate returns an error
// when a step's transition references an unknown step name.
func TestEngine_Validate_InvalidTransitionTarget(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	reg := registerAll(a)

	def := &WorkflowDefinition{
		Name:        "bad-trans",
		InitialStep: "step-a",
		Steps: []StepDefinition{
			{
				Name:        "step-a",
				Transitions: map[string]string{EventSuccess: "ghost-step"},
			},
		},
	}
	eng := NewEngine(reg)
	errs := eng.Validate(def)
	require.NotEmpty(t, errs)
	combined := fmt.Sprintf("%v", errs)
	assert.Contains(t, combined, "ghost-step")
}

// TestEngine_Validate_MultipleErrors verifies Validate collects all errors
// rather than returning on the first failure.
func TestEngine_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()

	// Empty registry: all steps have missing handlers.
	reg := NewRegistry()

	def := &WorkflowDefinition{
		Name:        "multi-error-workflow",
		InitialStep: "missing-initial",
		Steps: []StepDefinition{
			// step-a: no handler, transition to ghost-step (invalid target).
			{
				Name:        "step-a",
				Transitions: map[string]string{EventSuccess: "ghost-step"},
			},
			// step-b: no handler, valid terminal transition.
			{
				Name:        "step-b",
				Transitions: map[string]string{EventSuccess: StepDone},
			},
		},
	}
	eng := NewEngine(reg)
	errs := eng.Validate(def)

	// We expect at least 4 errors:
	// 1. missing-initial not found
	// 2. step-a missing handler
	// 3. step-a transition to ghost-step is invalid
	// 4. step-b missing handler
	assert.GreaterOrEqual(t, len(errs), 4,
		"Validate must collect all errors, got: %v", errs)
}

// TestEngine_Validate_TableDriven covers additional Validate edge cases.
func TestEngine_Validate_TableDriven(t *testing.T) {
	t.Parallel()

	aHandler := newRecorder("step-a")
	bHandler := newRecorder("step-b")
	regAB := registerAll(aHandler, bHandler)

	tests := []struct {
		name      string
		registry  *Registry
		def       *WorkflowDefinition
		wantEmpty bool
		wantInErr string
	}{
		{
			name:     "valid linear workflow",
			registry: regAB,
			def: &WorkflowDefinition{
				Name:        "valid",
				InitialStep: "step-a",
				Steps: []StepDefinition{
					{Name: "step-a", Transitions: map[string]string{EventSuccess: "step-b"}},
					{Name: "step-b", Transitions: map[string]string{EventSuccess: StepDone}},
				},
			},
			wantEmpty: true,
		},
		{
			name:     "initial step missing from steps list",
			registry: regAB,
			def: &WorkflowDefinition{
				Name:        "bad-init",
				InitialStep: "nonexistent",
				Steps: []StepDefinition{
					{Name: "step-a", Transitions: map[string]string{EventSuccess: StepDone}},
				},
			},
			wantEmpty: false,
			wantInErr: "nonexistent",
		},
		{
			name:     "transition to valid terminal pseudo-step",
			registry: registerAll(newRecorder("step-only")),
			def: &WorkflowDefinition{
				Name:        "terminal-ok",
				InitialStep: "step-only",
				Steps: []StepDefinition{
					{Name: "step-only", Transitions: map[string]string{
						EventSuccess: StepDone,
						EventFailure: StepFailed,
					}},
				},
			},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := NewEngine(tt.registry)
			errs := eng.Validate(tt.def)
			if tt.wantEmpty {
				assert.Empty(t, errs, "expected no validation errors but got: %v", errs)
			} else {
				require.NotEmpty(t, errs)
				if tt.wantInErr != "" {
					combined := fmt.Sprintf("%v", errs)
					assert.Contains(t, combined, tt.wantInErr)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Engine.RunStep
// ---------------------------------------------------------------------------

// TestEngine_RunStep_ExecutesSingleStep verifies RunStep executes only the
// requested step and does not advance to any subsequent step.
func TestEngine_RunStep_ExecutesSingleStep(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	b := newRecorder("step-b")
	reg := registerAll(a, b)
	def := linearDef("step-a", "step-b")

	eng := NewEngine(reg)
	state, err := eng.RunStep(context.Background(), def, "step-a", nil)
	require.NoError(t, err)

	assert.Equal(t, 1, a.callCount(), "step-a should execute once")
	assert.Equal(t, 0, b.callCount(), "step-b must not be executed by RunStep(step-a)")
	require.Len(t, state.StepHistory, 1)
	assert.Equal(t, "step-a", state.StepHistory[0].Step)
}

// TestEngine_RunStep_MiddleStep verifies RunStep can target a non-initial step.
func TestEngine_RunStep_MiddleStep(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	b := newRecorder("step-b")
	c := newRecorder("step-c")
	reg := registerAll(a, b, c)
	def := linearDef("step-a", "step-b", "step-c")

	eng := NewEngine(reg)
	state, err := eng.RunStep(context.Background(), def, "step-b", nil)
	require.NoError(t, err)

	assert.Equal(t, 0, a.callCount(), "step-a must not execute when targeting step-b")
	assert.Equal(t, 1, b.callCount(), "step-b must execute")
	assert.Equal(t, 0, c.callCount(), "step-c must not execute when targeting step-b")
	require.Len(t, state.StepHistory, 1)
	assert.Equal(t, "step-b", state.StepHistory[0].Step)
}

// TestEngine_RunStep_ReturnsAfterSingleStepEvenWithSuccessfulResult verifies
// that RunStep stops after one step regardless of the returned event.
func TestEngine_RunStep_ReturnsAfterSingleStepEvenWithSuccessfulResult(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	b := newRecorder("step-b")
	reg := registerAll(a, b)
	def := linearDef("step-a", "step-b")

	eng := NewEngine(reg)
	// Run step-a which normally transitions to step-b.
	state, err := eng.RunStep(context.Background(), def, "step-a", nil)
	require.NoError(t, err)

	// Even though step-a's transition would lead to step-b, RunStep stops after step-a.
	assert.Equal(t, 1, a.callCount())
	assert.Equal(t, 0, b.callCount(), "RunStep must not continue to step-b")
	_ = state
}

// TestEngine_RunStep_WithSingleStepMode verifies that WithSingleStep option
// applied directly to an Engine causes it to run only the named step.
func TestEngine_RunStep_WithSingleStepMode(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	b := newRecorder("step-b")
	c := newRecorder("step-c")
	reg := registerAll(a, b, c)
	def := linearDef("step-a", "step-b", "step-c")

	eng := NewEngine(reg, WithSingleStep("step-b"))
	state, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err)

	assert.Equal(t, 0, a.callCount(), "step-a must not execute in single-step mode for step-b")
	assert.Equal(t, 1, b.callCount(), "step-b must execute")
	assert.Equal(t, 0, c.callCount(), "step-c must not execute in single-step mode for step-b")
	require.Len(t, state.StepHistory, 1)
	assert.Equal(t, "step-b", state.StepHistory[0].Step)
}

// ---------------------------------------------------------------------------
// Engine options: WithMaxIterations
// ---------------------------------------------------------------------------

// TestEngine_Run_MaxIterationsGuard verifies the engine returns an error when
// a workflow exceeds the configured maximum number of iterations (e.g. a loop).
func TestEngine_Run_MaxIterationsGuard(t *testing.T) {
	t.Parallel()

	// Handler always returns EventSuccess, transitions back to itself -- infinite loop.
	loop := newRecorder("step-loop")
	reg := registerAll(loop)

	def := &WorkflowDefinition{
		Name:        "loop-workflow",
		InitialStep: "step-loop",
		Steps: []StepDefinition{
			{
				Name:        "step-loop",
				Transitions: map[string]string{EventSuccess: "step-loop"},
			},
		},
	}

	const maxIter = 5
	eng := NewEngine(reg, WithMaxIterations(maxIter))
	_, err := eng.Run(context.Background(), def, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maximum iterations", "error must mention iteration limit")
	// The loop handler should have been called exactly maxIter times.
	assert.Equal(t, maxIter, loop.callCount(),
		"loop handler must execute exactly maxIter times before being stopped")
}

// ---------------------------------------------------------------------------
// Event emission completeness
// ---------------------------------------------------------------------------

// TestEngine_Run_EmitsLifecycleEvents verifies that a complete Run produces
// the expected set of lifecycle event types.
func TestEngine_Run_EmitsLifecycleEvents(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	reg := registerAll(a)
	def := linearDef("step-a")

	events := make(chan WorkflowEvent, 64)
	eng := NewEngine(reg, WithEventChannel(events))

	_, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err)

	close(events)
	collected := collectEvents(t, events)
	typeSet := buildEventSet(collected)

	assert.True(t, typeSet[WEWorkflowStarted], "WEWorkflowStarted must be emitted")
	assert.True(t, typeSet[WEStepStarted], "WEStepStarted must be emitted")
	assert.True(t, typeSet[WEStepCompleted], "WEStepCompleted must be emitted")
	assert.True(t, typeSet[WEWorkflowCompleted], "WEWorkflowCompleted must be emitted")
}

// TestEngine_Run_EmitsStepFailedEvent verifies that WEStepFailed is emitted
// when a step handler returns an error.
func TestEngine_Run_EmitsStepFailedEvent(t *testing.T) {
	t.Parallel()

	failing := &recordingHandler{
		name:      "step-a",
		execErr:   errors.New("step-a failed"),
		dryRunMsg: "dry",
	}
	reg := registerAll(failing)

	def := &WorkflowDefinition{
		Name:        "step-failed-event-workflow",
		InitialStep: "step-a",
		Steps: []StepDefinition{
			{
				Name:        "step-a",
				Transitions: map[string]string{EventSuccess: StepDone},
				// No failure transition: error will bubble up.
			},
		},
	}

	events := make(chan WorkflowEvent, 64)
	eng := NewEngine(reg, WithEventChannel(events))

	_, err := eng.Run(context.Background(), def, nil)
	require.Error(t, err)

	close(events)
	collected := collectEvents(t, events)
	typeSet := buildEventSet(collected)
	assert.True(t, typeSet[WEStepFailed], "WEStepFailed must be emitted when step errors")
}

// TestEngine_Run_EventsCarryWorkflowID verifies that every WorkflowEvent has
// the correct WorkflowID populated.
func TestEngine_Run_EventsCarryWorkflowID(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	reg := registerAll(a)
	def := linearDef("step-a")

	existing := NewWorkflowState("my-wf-id-123", "test-workflow", "step-a")
	events := make(chan WorkflowEvent, 64)
	eng := NewEngine(reg, WithEventChannel(events))

	_, err := eng.Run(context.Background(), def, existing)
	require.NoError(t, err)

	close(events)
	collected := collectEvents(t, events)
	require.NotEmpty(t, collected, "at least one event must be emitted")
	for _, ev := range collected {
		assert.Equal(t, "my-wf-id-123", ev.WorkflowID,
			"every event must carry the workflow ID, got event type %q with ID %q",
			ev.Type, ev.WorkflowID)
	}
}

// TestEngine_Run_EventsCarryStepName verifies that step lifecycle events
// include the correct step name.
func TestEngine_Run_EventsCarryStepName(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-alpha")
	reg := registerAll(a)
	def := linearDef("step-alpha")

	events := make(chan WorkflowEvent, 64)
	eng := NewEngine(reg, WithEventChannel(events))

	_, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err)

	close(events)
	collected := collectEvents(t, events)

	var stepStarted, stepCompleted bool
	for _, ev := range collected {
		switch ev.Type {
		case WEStepStarted:
			assert.Equal(t, "step-alpha", ev.Step)
			stepStarted = true
		case WEStepCompleted:
			assert.Equal(t, "step-alpha", ev.Step)
			assert.Equal(t, EventSuccess, ev.Event)
			stepCompleted = true
		}
	}
	assert.True(t, stepStarted, "WEStepStarted must be emitted for step-alpha")
	assert.True(t, stepCompleted, "WEStepCompleted must be emitted for step-alpha")
}

// TestEngine_Run_EventsHaveTimestamps verifies that every emitted event has a
// non-zero Timestamp.
func TestEngine_Run_EventsHaveTimestamps(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	reg := registerAll(a)
	def := linearDef("step-a")

	events := make(chan WorkflowEvent, 64)
	eng := NewEngine(reg, WithEventChannel(events))

	_, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err)

	close(events)
	collected := collectEvents(t, events)
	require.NotEmpty(t, collected)
	for _, ev := range collected {
		assert.False(t, ev.Timestamp.IsZero(),
			"event of type %q must have a non-zero Timestamp", ev.Type)
	}
}

// TestEngine_Run_NilEventChannel verifies the engine operates silently when
// no event channel is configured (nil channel is safe to use).
func TestEngine_Run_NilEventChannel(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	reg := registerAll(a)
	def := linearDef("step-a")

	// No event channel configured.
	eng := NewEngine(reg)
	state, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, StepDone, state.CurrentStep)
}

// ---------------------------------------------------------------------------
// Engine options
// ---------------------------------------------------------------------------

// TestEngine_Options_WithLogger verifies that attaching a logger does not
// change observable engine behaviour.
func TestEngine_Options_WithLogger(t *testing.T) {
	t.Parallel()

	a := newRecorder("step-a")
	reg := registerAll(a)
	def := linearDef("step-a")

	// A nil logger must not crash the engine.
	eng := NewEngine(reg, WithLogger(nil))
	state, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err)
	assert.Equal(t, StepDone, state.CurrentStep)
}

// TestEngine_Options_DefaultMaxIterations verifies the default maxIterations
// is 1000.
func TestEngine_Options_DefaultMaxIterations(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	eng := NewEngine(reg)
	assert.Equal(t, defaultMaxIterations, eng.maxIterations,
		"default max iterations must be %d", defaultMaxIterations)
}

// TestEngine_Options_WithMaxIterations verifies the option overrides the default.
func TestEngine_Options_WithMaxIterations(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	eng := NewEngine(reg, WithMaxIterations(42))
	assert.Equal(t, 42, eng.maxIterations)
}

// ---------------------------------------------------------------------------
// Integration tests
// ---------------------------------------------------------------------------

// TestEngine_Integration_MetadataManipulation verifies a multi-step workflow
// where step handlers manipulate shared state metadata and downstream steps
// observe the accumulated values.
func TestEngine_Integration_MetadataManipulation(t *testing.T) {
	t.Parallel()

	// step-1 writes Metadata["counter"] = 10.
	// step-2 reads "counter" (stores in "seen-step-2") and writes Metadata["doubled"] = 20.
	// step-3 reads "doubled" (stores in "seen-step-3").
	step1 := &metadataHandler{name: "step-1", setKey: "counter", setVal: 10}
	step2 := &metadataHandler{name: "step-2", readKey: "counter", setKey: "doubled", setVal: 20}
	step3 := &metadataHandler{name: "step-3", readKey: "doubled"}

	reg := registerAll(step1, step2, step3)
	def := &WorkflowDefinition{
		Name:        "metadata-integration",
		InitialStep: "step-1",
		Steps: []StepDefinition{
			{Name: "step-1", Transitions: map[string]string{EventSuccess: "step-2"}},
			{Name: "step-2", Transitions: map[string]string{EventSuccess: "step-3"}},
			{Name: "step-3", Transitions: map[string]string{EventSuccess: StepDone}},
		},
	}

	eng := NewEngine(reg)
	state, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err)

	// step-3 should have observed the "doubled" key set by step-2.
	assert.Equal(t, 20, state.Metadata["doubled"],
		"step-2 must write 'doubled' metadata visible to step-3")
	assert.Equal(t, 20, state.Metadata["seen-step-3"],
		"step-3 must have observed 'doubled' value from step-2")
	assert.Equal(t, 10, state.Metadata["counter"],
		"initial counter from step-1 must be preserved")
}

// TestEngine_Integration_ConditionalBranching verifies that conditional
// branching based on metadata works correctly across two separate runs.
func TestEngine_Integration_ConditionalBranching(t *testing.T) {
	t.Parallel()

	buildWorkflow := func() (*WorkflowDefinition, *Registry, *recordingHandler, *recordingHandler) {
		decider := &conditionalHandler{
			name:        "decider",
			metadataKey: "go_success",
			trueEvent:   EventSuccess,
			falseEvent:  EventFailure,
		}
		successStep := newRecorder("success-step")
		failureStep := newRecorder("failure-step")
		reg := registerAll(decider, successStep, failureStep)

		def := &WorkflowDefinition{
			Name:        "conditional-workflow",
			InitialStep: "decider",
			Steps: []StepDefinition{
				{
					Name: "decider",
					Transitions: map[string]string{
						EventSuccess: "success-step",
						EventFailure: "failure-step",
					},
				},
				{Name: "success-step", Transitions: map[string]string{EventSuccess: StepDone}},
				{Name: "failure-step", Transitions: map[string]string{EventSuccess: StepDone}},
			},
		}
		return def, reg, successStep, failureStep
	}

	t.Run("success branch when metadata is true", func(t *testing.T) {
		t.Parallel()

		def, reg, successStep, failureStep := buildWorkflow()
		initialState := NewWorkflowState("wf-cond-success", "conditional-workflow", "decider")
		initialState.Metadata["go_success"] = true

		eng := NewEngine(reg)
		state, err := eng.Run(context.Background(), def, initialState)
		require.NoError(t, err)

		assert.Equal(t, 1, successStep.callCount(), "success-step must execute")
		assert.Equal(t, 0, failureStep.callCount(), "failure-step must NOT execute")
		assert.Equal(t, StepDone, state.CurrentStep)
	})

	t.Run("failure branch when metadata is false", func(t *testing.T) {
		t.Parallel()

		def, reg, successStep, failureStep := buildWorkflow()
		initialState := NewWorkflowState("wf-cond-failure", "conditional-workflow", "decider")
		initialState.Metadata["go_success"] = false

		eng := NewEngine(reg)
		state, err := eng.Run(context.Background(), def, initialState)
		require.NoError(t, err)

		assert.Equal(t, 0, successStep.callCount(), "success-step must NOT execute")
		assert.Equal(t, 1, failureStep.callCount(), "failure-step must execute")
		assert.Equal(t, StepDone, state.CurrentStep)
	})

	t.Run("failure branch when metadata key is absent", func(t *testing.T) {
		t.Parallel()

		def, reg, successStep, failureStep := buildWorkflow()
		// No metadata set; conditionalHandler returns falseEvent.
		initialState := NewWorkflowState("wf-cond-absent", "conditional-workflow", "decider")

		eng := NewEngine(reg)
		state, err := eng.Run(context.Background(), def, initialState)
		require.NoError(t, err)

		assert.Equal(t, 0, successStep.callCount(), "success-step must NOT execute when key absent")
		assert.Equal(t, 1, failureStep.callCount(), "failure-step must execute when key absent")
		assert.Equal(t, StepDone, state.CurrentStep)
	})
}

// TestEngine_Integration_FullLinearPipeline verifies a realistic five-step
// pipeline where each step sets metadata read by the next step.
func TestEngine_Integration_FullLinearPipeline(t *testing.T) {
	t.Parallel()

	steps := []struct {
		name    string
		setKey  string
		readKey string
	}{
		{name: "fetch", setKey: "fetched", readKey: ""},
		{name: "parse", setKey: "parsed", readKey: "fetched"},
		{name: "validate", setKey: "valid", readKey: "parsed"},
		{name: "transform", setKey: "transformed", readKey: "valid"},
		{name: "output", setKey: "", readKey: "transformed"},
	}

	handlers := make([]StepHandler, len(steps))
	for i, s := range steps {
		setVal := any("done-" + s.name)
		handlers[i] = &metadataHandler{
			name:    s.name,
			setKey:  s.setKey,
			setVal:  setVal,
			readKey: s.readKey,
		}
	}

	reg := registerAll(handlers...)

	names := make([]string, len(steps))
	for i, s := range steps {
		names[i] = s.name
	}
	def := linearDef(names...)

	events := make(chan WorkflowEvent, 128)
	eng := NewEngine(reg, WithEventChannel(events))
	state, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err)

	close(events)
	collected := collectEvents(t, events)

	assert.Equal(t, StepDone, state.CurrentStep)
	require.Len(t, state.StepHistory, len(steps))

	// Verify metadata chain.
	assert.Equal(t, "done-fetch", state.Metadata["fetched"])
	assert.Equal(t, "done-parse", state.Metadata["parsed"])
	assert.Equal(t, "done-validate", state.Metadata["valid"])
	assert.Equal(t, "done-transform", state.Metadata["transformed"])
	// "output" step read "transformed" into "seen-output".
	assert.Equal(t, "done-transform", state.Metadata["seen-output"])

	// Verify lifecycle event sequence for a 5-step workflow.
	typeSet := buildEventSet(collected)
	assert.True(t, typeSet[WEWorkflowStarted])
	assert.True(t, typeSet[WEStepStarted])
	assert.True(t, typeSet[WEStepCompleted])
	assert.True(t, typeSet[WEWorkflowCompleted])
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

// BenchmarkEngine_Run_Linear benchmarks execution of a 10-step linear workflow.
func BenchmarkEngine_Run_Linear(b *testing.B) {
	stepNames := make([]string, 10)
	handlers := make([]StepHandler, 10)
	for i := range stepNames {
		name := fmt.Sprintf("step-%02d", i)
		stepNames[i] = name
		handlers[i] = newRecorder(name)
	}
	reg := registerAll(handlers...)
	def := linearDef(stepNames...)
	eng := NewEngine(reg)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eng.Run(ctx, def, nil)
	}
}

// BenchmarkEngine_Run_SingleStep benchmarks RunStep on a 10-step definition.
func BenchmarkEngine_Run_SingleStep(b *testing.B) {
	stepNames := make([]string, 10)
	handlers := make([]StepHandler, 10)
	for i := range stepNames {
		name := fmt.Sprintf("step-%02d", i)
		stepNames[i] = name
		handlers[i] = newRecorder(name)
	}
	reg := registerAll(handlers...)
	def := linearDef(stepNames...)
	eng := NewEngine(reg)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eng.RunStep(ctx, def, "step-05", nil)
	}
}

// BenchmarkEngine_Validate benchmarks Validate on a 20-step workflow.
func BenchmarkEngine_Validate(b *testing.B) {
	stepNames := make([]string, 20)
	handlers := make([]StepHandler, 20)
	for i := range stepNames {
		name := fmt.Sprintf("step-%02d", i)
		stepNames[i] = name
		handlers[i] = newRecorder(name)
	}
	reg := registerAll(handlers...)
	def := linearDef(stepNames...)
	eng := NewEngine(reg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = eng.Validate(def)
	}
}
