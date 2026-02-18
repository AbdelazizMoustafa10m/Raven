package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/log"
)

const defaultMaxIterations = 1000

// Engine executes workflow definitions using registered step handlers.
// It drives the state machine forward step by step, emitting WorkflowEvents
// at each lifecycle milestone.
type Engine struct {
	registry      *Registry
	events        chan<- WorkflowEvent
	dryRun        bool
	singleStep    string // if non-empty, run only this step
	maxIterations int
	logger        *log.Logger
	postStepHook  func(*WorkflowState) error // called after each step; nil if not set
}

// EngineOption configures the Engine.
type EngineOption func(*Engine)

// WithDryRun enables or disables dry-run mode. In dry-run mode the engine
// calls DryRun() on each handler instead of Execute(), so no side effects occur.
func WithDryRun(dryRun bool) EngineOption {
	return func(e *Engine) { e.dryRun = dryRun }
}

// WithSingleStep restricts execution to a single named step. After that step
// completes the engine stops regardless of its outgoing transition.
func WithSingleStep(stepName string) EngineOption {
	return func(e *Engine) { e.singleStep = stepName }
}

// WithEventChannel sets the channel on which the engine broadcasts
// WorkflowEvents. The engine uses a non-blocking send so a slow consumer
// never stalls execution.
func WithEventChannel(ch chan<- WorkflowEvent) EngineOption {
	return func(e *Engine) { e.events = ch }
}

// WithLogger attaches a charmbracelet/log Logger to the engine. When nil
// the engine operates silently.
func WithLogger(logger *log.Logger) EngineOption {
	return func(e *Engine) { e.logger = logger }
}

// WithMaxIterations overrides the maximum number of steps the engine will
// execute in a single Run call (default 1000). Useful in tests.
func WithMaxIterations(n int) EngineOption {
	return func(e *Engine) { e.maxIterations = n }
}

// NewEngine creates a workflow engine with the given registry and options.
// The registry must not be nil.
func NewEngine(registry *Registry, opts ...EngineOption) *Engine {
	e := &Engine{
		registry:      registry,
		maxIterations: defaultMaxIterations,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Run executes a workflow definition from start (or from a resumed state).
// If state is nil a new WorkflowState is created using def.InitialStep.
// The function returns the final WorkflowState and any execution error.
//
// Run emits WEWorkflowStarted when beginning a fresh workflow and
// WEWorkflowResumed when state.StepHistory is non-empty.
func (e *Engine) Run(ctx context.Context, def *WorkflowDefinition, state *WorkflowState) (*WorkflowState, error) {
	// Initialise state when not provided.
	if state == nil {
		id := fmt.Sprintf("wf-%d", time.Now().UnixNano())
		state = NewWorkflowState(id, def.Name, def.InitialStep)
	}

	// Assign an ID if somehow still empty.
	if state.ID == "" {
		state.ID = fmt.Sprintf("wf-%d", time.Now().UnixNano())
	}

	// Emit the appropriate lifecycle start event.
	if len(state.StepHistory) > 0 {
		e.emit(WorkflowEvent{
			Type:       WEWorkflowResumed,
			WorkflowID: state.ID,
			Step:       state.CurrentStep,
			Message:    fmt.Sprintf("workflow %q resumed at step %q", def.Name, state.CurrentStep),
			Timestamp:  time.Now(),
		})
		e.log("workflow resumed", "workflow", def.Name, "step", state.CurrentStep)
	} else {
		e.emit(WorkflowEvent{
			Type:       WEWorkflowStarted,
			WorkflowID: state.ID,
			Step:       state.CurrentStep,
			Message:    fmt.Sprintf("workflow %q started", def.Name),
			Timestamp:  time.Now(),
		})
		e.log("workflow started", "workflow", def.Name, "step", state.CurrentStep)
	}

	// Build a step lookup map for O(1) access.
	stepDefs := make(map[string]*StepDefinition, len(def.Steps))
	for i := range def.Steps {
		sd := &def.Steps[i]
		stepDefs[sd.Name] = sd
	}

	// If running in singleStep mode, fast-forward to the requested step.
	if e.singleStep != "" {
		state.CurrentStep = e.singleStep
	}

	for iteration := 0; iteration < e.maxIterations; iteration++ {
		// Honour context cancellation before each step.
		if err := ctx.Err(); err != nil {
			return state, fmt.Errorf("engine: context cancelled before step %q: %w", state.CurrentStep, err)
		}

		currentStep := state.CurrentStep

		// Resolve the step definition.
		stepDef, ok := stepDefs[currentStep]
		if !ok {
			return state, fmt.Errorf("engine: step %q not found in workflow definition", currentStep)
		}

		// Resolve the handler from the registry.
		handler, err := e.registry.Get(currentStep)
		if err != nil {
			return state, fmt.Errorf("engine: step %q: %w", currentStep, err)
		}

		// Emit step started.
		e.emit(WorkflowEvent{
			Type:       WEStepStarted,
			WorkflowID: state.ID,
			Step:       currentStep,
			Message:    fmt.Sprintf("step %q started", currentStep),
			Timestamp:  time.Now(),
		})
		e.log("step started", "step", currentStep)

		startedAt := time.Now()
		var event string
		var stepErr error

		if e.dryRun {
			// In dry-run mode describe what would happen, then pretend success.
			description := handler.DryRun(state)
			e.log("step dry-run", "step", currentStep, "description", description)
			event = EventSuccess

			e.emit(WorkflowEvent{
				Type:       WEStepSkipped,
				WorkflowID: state.ID,
				Step:       currentStep,
				Event:      event,
				Message:    fmt.Sprintf("step %q skipped (dry-run): %s", currentStep, description),
				Timestamp:  time.Now(),
			})
		} else {
			// Execute the handler, catching any panics.
			event, stepErr = e.safeExecute(ctx, handler, state, currentStep)
		}

		duration := time.Since(startedAt)

		// Build and record the step history entry.
		record := StepRecord{
			Step:      currentStep,
			Event:     event,
			StartedAt: startedAt,
			Duration:  duration,
		}
		if stepErr != nil {
			record.Error = stepErr.Error()
		}
		state.AddStepRecord(record)

		if stepErr != nil {
			// Emit step failed event.
			e.emit(WorkflowEvent{
				Type:       WEStepFailed,
				WorkflowID: state.ID,
				Step:       currentStep,
				Event:      EventFailure,
				Message:    fmt.Sprintf("step %q failed: %v", currentStep, stepErr),
				Error:      stepErr.Error(),
				Timestamp:  time.Now(),
			})
			e.log("step failed", "step", currentStep, "error", stepErr)

			// Look up the failure transition; if absent bubble up the error.
			nextStep, hasFailureTrans := stepDef.Transitions[EventFailure]
			if !hasFailureTrans {
				return state, fmt.Errorf("engine: step %q: %w", currentStep, stepErr)
			}
			state.CurrentStep = nextStep
		} else {
			// Emit step completed event.
			e.emit(WorkflowEvent{
				Type:       WEStepCompleted,
				WorkflowID: state.ID,
				Step:       currentStep,
				Event:      event,
				Message:    fmt.Sprintf("step %q completed with event %q", currentStep, event),
				Timestamp:  time.Now(),
			})
			e.log("step completed", "step", currentStep, "event", event)

			// Resolve the next step from the transition map.
			nextStep, hasTrans := stepDef.Transitions[event]
			if !hasTrans {
				return state, fmt.Errorf("engine: step %q: no transition for event %q", currentStep, event)
			}
			state.CurrentStep = nextStep
		}

		// Call post-step hook (e.g., checkpointing) after CurrentStep has been
		// advanced so the checkpoint reflects the next step to execute.
		if e.postStepHook != nil {
			if hookErr := e.postStepHook(state); hookErr != nil {
				e.log("post-step hook error", "error", hookErr)
			}
		}

		// Check for terminal pseudo-steps.
		if state.CurrentStep == StepDone {
			e.emit(WorkflowEvent{
				Type:       WEWorkflowCompleted,
				WorkflowID: state.ID,
				Step:       StepDone,
				Message:    fmt.Sprintf("workflow %q completed successfully", def.Name),
				Timestamp:  time.Now(),
			})
			e.log("workflow completed", "workflow", def.Name)
			break
		}

		if state.CurrentStep == StepFailed {
			e.emit(WorkflowEvent{
				Type:       WEWorkflowFailed,
				WorkflowID: state.ID,
				Step:       StepFailed,
				Message:    fmt.Sprintf("workflow %q failed", def.Name),
				Timestamp:  time.Now(),
			})
			e.log("workflow failed", "workflow", def.Name)
			return state, fmt.Errorf("engine: workflow %q reached terminal failure step", def.Name)
		}

		// In single-step mode stop after executing one step.
		if e.singleStep != "" {
			break
		}
	}

	// Guard against infinite loops.
	if e.singleStep == "" && state.CurrentStep != StepDone {
		return state, fmt.Errorf("engine: workflow %q exceeded maximum iterations (%d)", def.Name, e.maxIterations)
	}

	return state, nil
}

// RunStep executes a single named step in isolation. It creates an ephemeral
// Engine with WithSingleStep applied and delegates to Run.
func (e *Engine) RunStep(ctx context.Context, def *WorkflowDefinition, stepName string, state *WorkflowState) (*WorkflowState, error) {
	opts := []EngineOption{
		WithSingleStep(stepName),
		WithDryRun(e.dryRun),
		WithEventChannel(e.events),
		WithLogger(e.logger),
		WithMaxIterations(e.maxIterations),
	}
	sub := NewEngine(e.registry, opts...)
	return sub.Run(ctx, def, state)
}

// Validate checks a workflow definition for structural errors:
//   - InitialStep exists in the step list
//   - All steps have registered handlers in the registry
//   - All transition targets are either valid step names or terminal pseudo-steps
//
// It returns all detected errors so callers receive a complete picture.
func (e *Engine) Validate(def *WorkflowDefinition) []error {
	var errs []error

	// Build valid step name set.
	validSteps := make(map[string]struct{}, len(def.Steps))
	for _, sd := range def.Steps {
		validSteps[sd.Name] = struct{}{}
	}

	// Terminal pseudo-steps are always valid transition targets.
	validSteps[StepDone] = struct{}{}
	validSteps[StepFailed] = struct{}{}

	// Check InitialStep.
	if _, ok := validSteps[def.InitialStep]; !ok {
		errs = append(errs, fmt.Errorf("engine: initial step %q not found in workflow definition", def.InitialStep))
	}

	// Walk every step.
	for _, sd := range def.Steps {
		// Check handler registration.
		if !e.registry.Has(sd.Name) {
			errs = append(errs, fmt.Errorf("engine: step %q has no registered handler", sd.Name))
		}

		// Check transition targets.
		for event, target := range sd.Transitions {
			if _, ok := validSteps[target]; !ok {
				errs = append(errs, fmt.Errorf("engine: step %q transition %q references unknown step %q", sd.Name, event, target))
			}
		}
	}

	return errs
}

// safeExecute calls handler.Execute wrapped in a recover() block so that
// panicking handlers are converted to errors rather than crashing the process.
func (e *Engine) safeExecute(ctx context.Context, handler StepHandler, state *WorkflowState, stepName string) (event string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("engine: step %q panicked: %v", stepName, r)
		}
	}()
	return handler.Execute(ctx, state)
}

// emit sends ev to the event channel using a non-blocking select so that a
// slow consumer never stalls workflow execution. It is a no-op when no channel
// has been configured.
func (e *Engine) emit(ev WorkflowEvent) {
	if e.events == nil {
		return
	}
	select {
	case e.events <- ev:
	default:
	}
}

// log writes a structured log message when a logger is attached.
func (e *Engine) log(msg string, kvs ...any) {
	if e.logger == nil {
		return
	}
	e.logger.Info(msg, kvs...)
}
