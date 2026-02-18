package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
	"github.com/AbdelazizMoustafa10m/Raven/internal/task"
)

// RunConfig configures the implementation loop behavior.
type RunConfig struct {
	AgentName     string
	PhaseID       int
	TaskID        string        // Specific task ID (empty for phase mode).
	MaxIterations int           // default: 50
	MaxLimitWaits int           // default: 5
	SleepBetween  time.Duration // default: 5s
	DryRun        bool
	TemplateName  string
}

// LoopEventType identifies the type of loop event.
type LoopEventType string

const (
	EventLoopStarted     LoopEventType = "loop_started"
	EventTaskSelected    LoopEventType = "task_selected"
	EventPromptGenerated LoopEventType = "prompt_generated"
	EventAgentStarted    LoopEventType = "agent_started"
	EventAgentCompleted  LoopEventType = "agent_completed"
	EventAgentError      LoopEventType = "agent_error"
	EventRateLimitWait   LoopEventType = "rate_limit_wait"
	EventRateLimitResume LoopEventType = "rate_limit_resume"
	EventTaskCompleted   LoopEventType = "task_completed"
	EventTaskBlocked     LoopEventType = "task_blocked"
	EventPhaseComplete   LoopEventType = "phase_complete"
	EventLoopError       LoopEventType = "loop_error"
	EventLoopAborted     LoopEventType = "loop_aborted"
	EventMaxIterations   LoopEventType = "max_iterations"
	EventSleeping        LoopEventType = "sleeping"
	EventDryRun          LoopEventType = "dry_run"

	// Fine-grained stream observability events (emitted when Claude is
	// invoked with stream-json output format).
	EventToolStarted   LoopEventType = "tool_started"
	EventToolCompleted LoopEventType = "tool_completed"
	EventAgentThinking LoopEventType = "agent_thinking"
	EventSessionStats  LoopEventType = "session_stats"
)

// LoopEvent represents a structured event emitted during loop execution.
type LoopEvent struct {
	Type      LoopEventType
	Iteration int
	TaskID    string
	AgentName string
	Message   string
	Timestamp time.Time
	Duration  time.Duration
	WaitTime  time.Duration

	// Stream-level observability fields (populated for tool/thinking/stats events).
	ToolName string  // Name of the tool called (EventToolStarted) or tool_use_id (EventToolCompleted).
	CostUSD  float64 // Session cost in USD (EventSessionStats).
	TokensIn int     // Input token count (EventSessionStats).
	TokensOut int    // Output token count (EventSessionStats).
}

// CompletionSignal represents a signal detected in agent output.
type CompletionSignal string

const (
	SignalPhaseComplete CompletionSignal = "PHASE_COMPLETE"
	SignalTaskBlocked   CompletionSignal = "TASK_BLOCKED"
	SignalRavenError    CompletionSignal = "RAVEN_ERROR"
)

// defaultMaxIterations is the default maximum number of loop iterations.
const defaultMaxIterations = 50

// defaultSleepBetween is the default sleep duration between iterations.
const defaultSleepBetween = 5 * time.Second

// staleTaskThreshold is the number of consecutive identical task selections
// that triggers a stale-task warning event.
const staleTaskThreshold = 3

// Runner orchestrates the implementation loop. It selects the next actionable
// task, generates a prompt, invokes the agent, interprets the output, updates
// task state, and repeats until the phase is complete, limits are reached, or
// the context is cancelled.
type Runner struct {
	selector     *task.TaskSelector
	promptGen    *PromptGenerator
	agent        agent.Agent
	stateManager *task.StateManager
	rateLimiter  *agent.RateLimitCoordinator
	config       *config.Config
	phases       []task.Phase
	events       chan<- LoopEvent
	logger       interface {
		Info(msg string, kv ...interface{})
		Debug(msg string, kv ...interface{})
	}
}

// NewRunner creates an implementation loop runner with all dependencies.
// The events channel receives structured LoopEvent values; pass nil to disable
// event emission. The logger must not be nil.
func NewRunner(
	selector *task.TaskSelector,
	promptGen *PromptGenerator,
	ag agent.Agent,
	stateManager *task.StateManager,
	rateLimiter *agent.RateLimitCoordinator,
	cfg *config.Config,
	phases []task.Phase,
	events chan<- LoopEvent,
	logger interface {
		Info(msg string, kv ...interface{})
		Debug(msg string, kv ...interface{})
	},
) *Runner {
	return &Runner{
		selector:     selector,
		promptGen:    promptGen,
		agent:        ag,
		stateManager: stateManager,
		rateLimiter:  rateLimiter,
		config:       cfg,
		phases:       phases,
		events:       events,
		logger:       logger,
	}
}

// Run executes the implementation loop in phase mode. It iterates over all
// not-started tasks in runCfg.PhaseID, running the agent on each, until the
// phase is complete, max iterations are reached, or ctx is cancelled.
func (r *Runner) Run(ctx context.Context, runCfg RunConfig) error {
	applyDefaults(&runCfg)

	r.logger.Info("starting implementation loop",
		"agent", runCfg.AgentName,
		"phase", runCfg.PhaseID,
		"maxIterations", runCfg.MaxIterations,
		"dryRun", runCfg.DryRun,
	)
	r.emit(LoopEvent{
		Type:      EventLoopStarted,
		AgentName: runCfg.AgentName,
		Message:   fmt.Sprintf("phase %d", runCfg.PhaseID),
		Timestamp: time.Now(),
	})

	// recentTaskIDs holds the last staleTaskThreshold task IDs selected,
	// used for stale-task detection.
	recentTaskIDs := make([]string, 0, staleTaskThreshold)

	for iteration := 1; iteration <= runCfg.MaxIterations; iteration++ {
		// Check for context cancellation before each iteration.
		if err := ctx.Err(); err != nil {
			r.emit(LoopEvent{
				Type:      EventLoopAborted,
				Iteration: iteration,
				AgentName: runCfg.AgentName,
				Message:   "context cancelled",
				Timestamp: time.Now(),
			})
			return fmt.Errorf("implementation loop cancelled: %w", err)
		}

		// Select the next task.
		spec, err := r.selectTask(runCfg)
		if err != nil {
			r.emit(loopErrorEvent(iteration, runCfg.AgentName, err.Error()))
			return fmt.Errorf("implementation loop iteration %d: %w", iteration, err)
		}
		if spec == nil {
			// Phase complete: no more actionable tasks.
			r.logger.Info("phase complete", "phase", runCfg.PhaseID, "iteration", iteration)
			r.emit(LoopEvent{
				Type:      EventPhaseComplete,
				Iteration: iteration,
				AgentName: runCfg.AgentName,
				Message:   fmt.Sprintf("phase %d complete after %d iterations", runCfg.PhaseID, iteration-1),
				Timestamp: time.Now(),
			})
			return nil
		}

		r.logger.Info("selected task",
			"task", spec.ID,
			"title", spec.Title,
			"iteration", iteration,
		)
		r.emit(LoopEvent{
			Type:      EventTaskSelected,
			Iteration: iteration,
			TaskID:    spec.ID,
			AgentName: runCfg.AgentName,
			Message:   spec.Title,
			Timestamp: time.Now(),
		})

		// Stale task detection: track recent selections.
		recentTaskIDs = appendRecent(recentTaskIDs, spec.ID)
		if isStale(recentTaskIDs) {
			r.logger.Info("stale task detected: same task selected multiple times in a row",
				"task", spec.ID,
				"count", staleTaskThreshold,
			)
			r.emit(LoopEvent{
				Type:      EventLoopError,
				Iteration: iteration,
				TaskID:    spec.ID,
				AgentName: runCfg.AgentName,
				Message:   fmt.Sprintf("stale task warning: %s selected %d times in a row", spec.ID, staleTaskThreshold),
				Timestamp: time.Now(),
			})
		}

		// Mark task in_progress.
		if err := r.stateManager.UpdateStatus(spec.ID, task.StatusInProgress, runCfg.AgentName); err != nil {
			return fmt.Errorf("updating task %s to in_progress: %w", spec.ID, err)
		}

		if runCfg.DryRun {
			if err := r.handleDryRun(ctx, spec, runCfg, iteration); err != nil {
				return err
			}
			continue
		}

		// Generate prompt.
		prompt, err := r.generatePrompt(spec, runCfg)
		if err != nil {
			r.emit(loopErrorEvent(iteration, runCfg.AgentName, err.Error()))
			return fmt.Errorf("generating prompt for task %s: %w", spec.ID, err)
		}
		r.emit(LoopEvent{
			Type:      EventPromptGenerated,
			Iteration: iteration,
			TaskID:    spec.ID,
			AgentName: runCfg.AgentName,
			Message:   fmt.Sprintf("prompt generated (%d bytes)", len(prompt)),
			Timestamp: time.Now(),
		})

		// Invoke agent (with rate-limit retry).
		result, err := r.invokeAgentWithRetry(ctx, prompt, runCfg, iteration, spec.ID)
		if err != nil {
			// Check if it's a max-waits-exceeded abort.
			if errors.Is(err, agent.ErrMaxWaitsExceeded) {
				r.emit(LoopEvent{
					Type:      EventLoopAborted,
					Iteration: iteration,
					TaskID:    spec.ID,
					AgentName: runCfg.AgentName,
					Message:   "rate-limit max waits exceeded",
					Timestamp: time.Now(),
				})
				return fmt.Errorf("implementation loop aborted: %w", err)
			}
			// Context cancelled.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				r.emit(LoopEvent{
					Type:      EventLoopAborted,
					Iteration: iteration,
					TaskID:    spec.ID,
					AgentName: runCfg.AgentName,
					Message:   "context cancelled during agent invocation",
					Timestamp: time.Now(),
				})
				return fmt.Errorf("implementation loop cancelled during agent run: %w", err)
			}
			r.emit(LoopEvent{
				Type:      EventAgentError,
				Iteration: iteration,
				TaskID:    spec.ID,
				AgentName: runCfg.AgentName,
				Message:   err.Error(),
				Timestamp: time.Now(),
			})
			return fmt.Errorf("agent error on task %s: %w", spec.ID, err)
		}

		r.emit(LoopEvent{
			Type:      EventAgentCompleted,
			Iteration: iteration,
			TaskID:    spec.ID,
			AgentName: runCfg.AgentName,
			Message:   fmt.Sprintf("exit code %d", result.ExitCode),
			Timestamp: time.Now(),
			Duration:  result.Duration,
		})

		// Detect completion signals.
		signal, detail := r.detectSignals(result.Stdout)
		if err := r.handleCompletion(signal, detail, spec.ID, runCfg.AgentName); err != nil {
			return err
		}

		// If PHASE_COMPLETE detected, stop the loop.
		if signal == SignalPhaseComplete {
			r.emit(LoopEvent{
				Type:      EventPhaseComplete,
				Iteration: iteration,
				TaskID:    spec.ID,
				AgentName: runCfg.AgentName,
				Message:   "PHASE_COMPLETE signal detected in output",
				Timestamp: time.Now(),
			})
			return nil
		}

		// If RAVEN_ERROR detected, stop the loop with error.
		if signal == SignalRavenError {
			return fmt.Errorf("agent reported RAVEN_ERROR for task %s: %s", spec.ID, detail)
		}

		// TASK_BLOCKED: already handled in handleCompletion; continue to next.

		// Early phase-completion check: if no more tasks are actionable after this
		// iteration, return nil rather than waiting for the next iteration's
		// selectTask call (which would require an extra loop cycle that may not be
		// available when MaxIterations is tight).
		if signal != SignalTaskBlocked {
			nextSpec, checkErr := r.selectTask(runCfg)
			if checkErr == nil && nextSpec == nil {
				r.logger.Info("phase complete", "phase", runCfg.PhaseID, "iteration", iteration)
				r.emit(LoopEvent{
					Type:      EventPhaseComplete,
					Iteration: iteration,
					AgentName: runCfg.AgentName,
					Message:   fmt.Sprintf("phase %d complete after %d iterations", runCfg.PhaseID, iteration),
					Timestamp: time.Now(),
				})
				return nil
			}
		}

		// Sleep between iterations (cancellable).
		if runCfg.SleepBetween > 0 {
			r.emit(LoopEvent{
				Type:      EventSleeping,
				Iteration: iteration,
				AgentName: runCfg.AgentName,
				Message:   fmt.Sprintf("sleeping %s between iterations", runCfg.SleepBetween),
				Timestamp: time.Now(),
				WaitTime:  runCfg.SleepBetween,
			})
			if err := sleepWithContext(ctx, runCfg.SleepBetween); err != nil {
				r.emit(LoopEvent{
					Type:      EventLoopAborted,
					Iteration: iteration,
					AgentName: runCfg.AgentName,
					Message:   "context cancelled during sleep",
					Timestamp: time.Now(),
				})
				return fmt.Errorf("implementation loop cancelled during sleep: %w", err)
			}
		}
	}

	// Max iterations reached.
	r.logger.Info("max iterations reached", "maxIterations", runCfg.MaxIterations)
	r.emit(LoopEvent{
		Type:      EventMaxIterations,
		Iteration: runCfg.MaxIterations,
		AgentName: runCfg.AgentName,
		Message:   fmt.Sprintf("max iterations (%d) reached", runCfg.MaxIterations),
		Timestamp: time.Now(),
	})
	return fmt.Errorf("implementation loop stopped: max iterations (%d) reached", runCfg.MaxIterations)
}

// RunSingleTask runs the loop for a specific task ID (--task T-007 mode).
// It selects the task by ID, generates a prompt, invokes the agent, and
// returns after one successful invocation (or error).
func (r *Runner) RunSingleTask(ctx context.Context, runCfg RunConfig) error {
	applyDefaults(&runCfg)

	r.logger.Info("starting single-task implementation",
		"agent", runCfg.AgentName,
		"task", runCfg.TaskID,
		"dryRun", runCfg.DryRun,
	)
	r.emit(LoopEvent{
		Type:      EventLoopStarted,
		AgentName: runCfg.AgentName,
		TaskID:    runCfg.TaskID,
		Message:   fmt.Sprintf("single task %s", runCfg.TaskID),
		Timestamp: time.Now(),
	})

	for iteration := 1; iteration <= runCfg.MaxIterations; iteration++ {
		if err := ctx.Err(); err != nil {
			r.emit(LoopEvent{
				Type:      EventLoopAborted,
				Iteration: iteration,
				TaskID:    runCfg.TaskID,
				AgentName: runCfg.AgentName,
				Message:   "context cancelled",
				Timestamp: time.Now(),
			})
			return fmt.Errorf("single-task loop cancelled: %w", err)
		}

		// Select the specific task.
		spec, err := r.selectTask(runCfg)
		if err != nil {
			r.emit(loopErrorEvent(iteration, runCfg.AgentName, err.Error()))
			return fmt.Errorf("single-task loop: selecting task %s: %w", runCfg.TaskID, err)
		}
		if spec == nil {
			// Task is nil -- this would be unusual in single-task mode.
			return fmt.Errorf("task %s not found or not actionable", runCfg.TaskID)
		}

		r.emit(LoopEvent{
			Type:      EventTaskSelected,
			Iteration: iteration,
			TaskID:    spec.ID,
			AgentName: runCfg.AgentName,
			Message:   spec.Title,
			Timestamp: time.Now(),
		})

		// Mark in_progress.
		if err := r.stateManager.UpdateStatus(spec.ID, task.StatusInProgress, runCfg.AgentName); err != nil {
			return fmt.Errorf("updating task %s to in_progress: %w", spec.ID, err)
		}

		if runCfg.DryRun {
			return r.handleDryRun(ctx, spec, runCfg, iteration)
		}

		// Generate prompt.
		prompt, err := r.generatePrompt(spec, runCfg)
		if err != nil {
			r.emit(loopErrorEvent(iteration, runCfg.AgentName, err.Error()))
			return fmt.Errorf("generating prompt for task %s: %w", spec.ID, err)
		}
		r.emit(LoopEvent{
			Type:      EventPromptGenerated,
			Iteration: iteration,
			TaskID:    spec.ID,
			AgentName: runCfg.AgentName,
			Message:   fmt.Sprintf("prompt generated (%d bytes)", len(prompt)),
			Timestamp: time.Now(),
		})

		// Invoke agent with rate-limit retry.
		result, err := r.invokeAgentWithRetry(ctx, prompt, runCfg, iteration, spec.ID)
		if err != nil {
			if errors.Is(err, agent.ErrMaxWaitsExceeded) {
				r.emit(LoopEvent{
					Type:      EventLoopAborted,
					Iteration: iteration,
					TaskID:    spec.ID,
					AgentName: runCfg.AgentName,
					Message:   "rate-limit max waits exceeded",
					Timestamp: time.Now(),
				})
				return fmt.Errorf("single-task loop aborted: %w", err)
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				r.emit(LoopEvent{
					Type:      EventLoopAborted,
					Iteration: iteration,
					TaskID:    spec.ID,
					AgentName: runCfg.AgentName,
					Message:   "context cancelled during agent invocation",
					Timestamp: time.Now(),
				})
				return fmt.Errorf("single-task loop cancelled during agent run: %w", err)
			}
			r.emit(LoopEvent{
				Type:      EventAgentError,
				Iteration: iteration,
				TaskID:    spec.ID,
				AgentName: runCfg.AgentName,
				Message:   err.Error(),
				Timestamp: time.Now(),
			})
			return fmt.Errorf("agent error on task %s: %w", spec.ID, err)
		}

		r.emit(LoopEvent{
			Type:      EventAgentCompleted,
			Iteration: iteration,
			TaskID:    spec.ID,
			AgentName: runCfg.AgentName,
			Message:   fmt.Sprintf("exit code %d", result.ExitCode),
			Timestamp: time.Now(),
			Duration:  result.Duration,
		})

		// Detect completion signals.
		signal, detail := r.detectSignals(result.Stdout)
		if err := r.handleCompletion(signal, detail, spec.ID, runCfg.AgentName); err != nil {
			return err
		}

		// After a successful PHASE_COMPLETE or task completion, stop.
		if signal == SignalPhaseComplete || signal == "" {
			r.emit(LoopEvent{
				Type:      EventPhaseComplete,
				Iteration: iteration,
				TaskID:    spec.ID,
				AgentName: runCfg.AgentName,
				Message:   fmt.Sprintf("single task %s complete", spec.ID),
				Timestamp: time.Now(),
			})
			return nil
		}

		if signal == SignalRavenError {
			return fmt.Errorf("agent reported RAVEN_ERROR for task %s: %s", spec.ID, detail)
		}

		// TASK_BLOCKED: stop since there's only one task.
		if signal == SignalTaskBlocked {
			return nil
		}

		// Sleep between iterations if re-running.
		if runCfg.SleepBetween > 0 {
			if err := sleepWithContext(ctx, runCfg.SleepBetween); err != nil {
				return fmt.Errorf("single-task loop cancelled during sleep: %w", err)
			}
		}
	}

	r.emit(LoopEvent{
		Type:      EventMaxIterations,
		Iteration: runCfg.MaxIterations,
		TaskID:    runCfg.TaskID,
		AgentName: runCfg.AgentName,
		Message:   fmt.Sprintf("max iterations (%d) reached for task %s", runCfg.MaxIterations, runCfg.TaskID),
		Timestamp: time.Now(),
	})
	return fmt.Errorf("single-task loop stopped: max iterations (%d) reached for task %s", runCfg.MaxIterations, runCfg.TaskID)
}

// DetectSignals scans output for completion signal strings. It returns the
// first CompletionSignal found and any trailing detail text (e.g., reason
// following TASK_BLOCKED or RAVEN_ERROR). Returns an empty signal if none found.
//
// This function is exported for use in tests.
func DetectSignals(output string) (CompletionSignal, string) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, string(SignalPhaseComplete)) {
			detail := strings.TrimSpace(strings.TrimPrefix(trimmed, string(SignalPhaseComplete)))
			return SignalPhaseComplete, detail
		}
		if strings.HasPrefix(trimmed, string(SignalTaskBlocked)) {
			detail := strings.TrimSpace(strings.TrimPrefix(trimmed, string(SignalTaskBlocked)))
			return SignalTaskBlocked, detail
		}
		if strings.HasPrefix(trimmed, string(SignalRavenError)) {
			detail := strings.TrimSpace(strings.TrimPrefix(trimmed, string(SignalRavenError)))
			return SignalRavenError, detail
		}
	}
	return "", ""
}

// ------- internal methods -------

// selectTask picks the next task based on run mode. In single-task mode
// (runCfg.TaskID non-empty) it uses SelectByID; otherwise it uses
// SelectNext for phase mode.
func (r *Runner) selectTask(runCfg RunConfig) (*task.ParsedTaskSpec, error) {
	if runCfg.TaskID != "" {
		spec, err := r.selector.SelectByID(runCfg.TaskID)
		if err != nil {
			return nil, fmt.Errorf("selecting task by ID %s: %w", runCfg.TaskID, err)
		}
		return spec, nil
	}
	spec, err := r.selector.SelectNext(runCfg.PhaseID)
	if err != nil {
		return nil, fmt.Errorf("selecting next task in phase %d: %w", runCfg.PhaseID, err)
	}
	return spec, nil
}

// generatePrompt builds the prompt for the given task spec by looking up the
// Phase (from r.phases using runCfg.PhaseID), calling BuildContext, and
// rendering via r.promptGen.
func (r *Runner) generatePrompt(spec *task.ParsedTaskSpec, runCfg RunConfig) (string, error) {
	phase := task.PhaseByID(r.phases, runCfg.PhaseID)
	if phase == nil {
		// Fall back to a synthetic phase when PhaseID is 0 or unknown.
		phase = &task.Phase{
			ID:        runCfg.PhaseID,
			Name:      "Unknown Phase",
			StartTask: spec.ID,
			EndTask:   spec.ID,
		}
	}

	pctx, err := BuildContext(spec, phase, r.config, r.selector, runCfg.AgentName)
	if err != nil {
		return "", fmt.Errorf("building prompt context for task %s: %w", spec.ID, err)
	}

	prompt, err := r.promptGen.Generate(runCfg.TemplateName, *pctx)
	if err != nil {
		return "", fmt.Errorf("generating prompt for task %s: %w", spec.ID, err)
	}
	return prompt, nil
}

// invokeAgent runs the agent with the generated prompt and returns the result.
// It always creates a streaming channel and launches consumeStreamEvents so
// that fine-grained LoopEvents are emitted when the agent supports stream-json.
// Agents that do not support streaming (e.g. CodexAgent) simply ignore
// opts.StreamEvents, and the consumer goroutine exits cleanly when the channel
// is closed after Run returns.
func (r *Runner) invokeAgent(ctx context.Context, prompt string, runCfg RunConfig, iteration int, taskID string) (*agent.RunResult, error) {
	agentCfg, _ := r.config.Agents[runCfg.AgentName]

	// Buffered channel owned by the caller (this method). It is closed after
	// Run returns so the consumer goroutine can drain and exit.
	streamCh := make(chan agent.StreamEvent, 256)

	opts := agent.RunOpts{
		Prompt:       prompt,
		Model:        agentCfg.Model,
		Effort:       agentCfg.Effort,
		AllowedTools: agentCfg.AllowedTools,
		OutputFormat: agent.OutputFormatStreamJSON,
		StreamEvents: streamCh,
	}

	r.logger.Debug("invoking agent",
		"agent", runCfg.AgentName,
		"model", opts.Model,
		"promptBytes", len(prompt),
	)

	// Launch consumer goroutine. It drains streamCh until it is closed.
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		r.consumeStreamEvents(ctx, streamCh, iteration, taskID, runCfg.AgentName)
	}()

	result, err := r.agent.Run(ctx, opts)

	// Close the channel now that Run has returned; the consumer will drain any
	// remaining buffered events then exit.
	close(streamCh)
	<-consumerDone

	if err != nil {
		return nil, fmt.Errorf("invoking agent %s: %w", runCfg.AgentName, err)
	}
	return result, nil
}

// consumeStreamEvents reads StreamEvent values from streamCh and translates
// them into fine-grained LoopEvents that are forwarded to the events channel.
// It blocks until streamCh is closed or ctx is cancelled.
func (r *Runner) consumeStreamEvents(ctx context.Context, streamCh <-chan agent.StreamEvent, iteration int, taskID string, agentName string) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-streamCh:
			if !ok {
				return
			}
			switch event.Type {
			case agent.StreamEventAssistant:
				// Tool_use blocks → EventToolStarted.
				for _, block := range event.ToolUseBlocks() {
					r.emit(LoopEvent{
						Type:      EventToolStarted,
						Iteration: iteration,
						TaskID:    taskID,
						AgentName: agentName,
						ToolName:  block.Name,
						Message:   fmt.Sprintf("tool call: %s", block.Name),
						Timestamp: time.Now(),
					})
				}
				// Text content → EventAgentThinking.
				if text := event.TextContent(); text != "" {
					r.emit(LoopEvent{
						Type:      EventAgentThinking,
						Iteration: iteration,
						TaskID:    taskID,
						AgentName: agentName,
						Message:   text,
						Timestamp: time.Now(),
					})
				}
			case agent.StreamEventUser:
				// Tool_result blocks → EventToolCompleted.
				for _, block := range event.ToolResultBlocks() {
					r.emit(LoopEvent{
						Type:      EventToolCompleted,
						Iteration: iteration,
						TaskID:    taskID,
						AgentName: agentName,
						ToolName:  block.ToolUseID,
						Message:   fmt.Sprintf("tool result: %s", block.ToolUseID),
						Timestamp: time.Now(),
					})
				}
			case agent.StreamEventResult:
				r.emit(LoopEvent{
					Type:      EventSessionStats,
					Iteration: iteration,
					TaskID:    taskID,
					AgentName: agentName,
					CostUSD:   event.CostUSD,
					Message:   fmt.Sprintf("session cost: $%.4f", event.CostUSD),
					Timestamp: time.Now(),
				})
			}
		}
	}
}

// invokeAgentWithRetry invokes the agent, handling rate limits with the
// coordinator. On rate-limit detection it records the limit, emits a wait
// event, waits for reset, then retries once. If max waits are exceeded it
// returns ErrMaxWaitsExceeded.
func (r *Runner) invokeAgentWithRetry(
	ctx context.Context,
	prompt string,
	runCfg RunConfig,
	iteration int,
	taskID string,
) (*agent.RunResult, error) {
	r.emit(LoopEvent{
		Type:      EventAgentStarted,
		Iteration: iteration,
		TaskID:    taskID,
		AgentName: runCfg.AgentName,
		Message:   fmt.Sprintf("invoking %s", runCfg.AgentName),
		Timestamp: time.Now(),
	})

	result, err := r.invokeAgent(ctx, prompt, runCfg, iteration, taskID)
	if err != nil {
		return nil, err
	}

	// Check for rate limit in the result or by parsing stdout/stderr.
	rlInfo, limited := r.agent.ParseRateLimit(result.Stdout + result.Stderr)
	if result.WasRateLimited() {
		rlInfo = result.RateLimit
		limited = true
	}

	if !limited {
		return result, nil
	}

	// Rate limit detected.
	r.logger.Info("rate limit detected, waiting for reset",
		"agent", runCfg.AgentName,
		"task", taskID,
	)
	ps := r.rateLimiter.RecordRateLimit(runCfg.AgentName, rlInfo)
	waitDuration := ps.RemainingWait()

	r.emit(LoopEvent{
		Type:      EventRateLimitWait,
		Iteration: iteration,
		TaskID:    taskID,
		AgentName: runCfg.AgentName,
		Message:   fmt.Sprintf("rate limited, waiting %s", waitDuration.Round(time.Second)),
		Timestamp: time.Now(),
		WaitTime:  waitDuration,
	})

	if err := r.rateLimiter.WaitForReset(ctx, runCfg.AgentName); err != nil {
		return nil, err
	}

	r.emit(LoopEvent{
		Type:      EventRateLimitResume,
		Iteration: iteration,
		TaskID:    taskID,
		AgentName: runCfg.AgentName,
		Message:   "rate limit reset, retrying",
		Timestamp: time.Now(),
	})

	// Retry the agent after waiting.
	result, err = r.invokeAgent(ctx, prompt, runCfg, iteration, taskID)
	if err != nil {
		return nil, err
	}
	r.rateLimiter.ClearRateLimit(runCfg.AgentName)
	return result, nil
}

// detectSignals scans the output for completion signals. It first attempts a
// plain-text scan (backward compatible), then falls back to scanning JSONL
// text content for signals embedded in stream-json output.
func (r *Runner) detectSignals(output string) (CompletionSignal, string) {
	if sig, detail := DetectSignals(output); sig != "" {
		return sig, detail
	}
	return DetectSignalsFromJSONL(output)
}

// DetectSignalsFromJSONL scans JSONL output (stream-json format) for completion
// signals embedded within assistant text content blocks. Each line is parsed as
// a StreamEvent; text blocks within assistant messages are scanned for signals.
// Returns an empty signal if none is found.
//
// This function is exported for use in tests.
func DetectSignalsFromJSONL(output string) (CompletionSignal, string) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var event agent.StreamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if text := event.TextContent(); text != "" {
			if sig, detail := DetectSignals(text); sig != "" {
				return sig, detail
			}
		}
	}
	return "", ""
}

// handleCompletion processes a detected signal and updates task state
// accordingly. Returns a non-nil error only for RAVEN_ERROR (which the caller
// should propagate to stop the loop).
func (r *Runner) handleCompletion(signal CompletionSignal, detail string, taskID string, agentName string) error {
	switch signal {
	case SignalPhaseComplete:
		if err := r.stateManager.UpdateStatus(taskID, task.StatusCompleted, agentName); err != nil {
			return fmt.Errorf("marking task %s completed: %w", taskID, err)
		}
		r.emit(LoopEvent{
			Type:      EventTaskCompleted,
			TaskID:    taskID,
			AgentName: agentName,
			Message:   "task completed (PHASE_COMPLETE signal)",
			Timestamp: time.Now(),
		})

	case SignalTaskBlocked:
		if err := r.stateManager.UpdateStatus(taskID, task.StatusBlocked, agentName); err != nil {
			return fmt.Errorf("marking task %s blocked: %w", taskID, err)
		}
		r.emit(LoopEvent{
			Type:      EventTaskBlocked,
			TaskID:    taskID,
			AgentName: agentName,
			Message:   detail,
			Timestamp: time.Now(),
		})

	case SignalRavenError:
		r.emit(loopErrorEvent(0, agentName, fmt.Sprintf("RAVEN_ERROR for task %s: %s", taskID, detail)))
		// Caller is responsible for returning the error.

	default:
		// No signal -- treat as task completed successfully.
		if err := r.stateManager.UpdateStatus(taskID, task.StatusCompleted, agentName); err != nil {
			return fmt.Errorf("marking task %s completed: %w", taskID, err)
		}
		r.emit(LoopEvent{
			Type:      EventTaskCompleted,
			TaskID:    taskID,
			AgentName: agentName,
			Message:   "task completed",
			Timestamp: time.Now(),
		})
	}
	return nil
}

// handleDryRun generates and prints the prompt to stderr without invoking the
// agent, then emits a dry_run event and returns.
func (r *Runner) handleDryRun(_ context.Context, spec *task.ParsedTaskSpec, runCfg RunConfig, iteration int) error {
	prompt, err := r.generatePrompt(spec, runCfg)
	if err != nil {
		r.emit(loopErrorEvent(iteration, runCfg.AgentName, err.Error()))
		return fmt.Errorf("generating dry-run prompt for task %s: %w", spec.ID, err)
	}

	// Print the command that would be executed.
	agentCfg, _ := r.config.Agents[runCfg.AgentName]
	opts := agent.RunOpts{
		Prompt:       prompt,
		Model:        agentCfg.Model,
		Effort:       agentCfg.Effort,
		AllowedTools: agentCfg.AllowedTools,
	}
	cmd := r.agent.DryRunCommand(opts)

	log.Info("[DRY RUN] would execute", "command", cmd, "task", spec.ID)
	fmt.Fprintf(os.Stderr, "\n--- DRY RUN: task %s ---\n%s\n\n--- PROMPT ---\n%s\n", spec.ID, cmd, prompt)

	r.emit(LoopEvent{
		Type:      EventDryRun,
		Iteration: iteration,
		TaskID:    spec.ID,
		AgentName: runCfg.AgentName,
		Message:   fmt.Sprintf("dry run: %s", cmd),
		Timestamp: time.Now(),
	})

	// In dry-run mode, don't mark the task in_progress permanently -- revert
	// it back to not_started so actual runs can pick it up.
	if err := r.stateManager.UpdateStatus(spec.ID, task.StatusNotStarted, ""); err != nil {
		return fmt.Errorf("reverting dry-run task %s to not_started: %w", spec.ID, err)
	}

	return nil
}

// emit sends a LoopEvent to the events channel in a non-blocking fashion.
// If the channel is full, the event is dropped.
func (r *Runner) emit(event LoopEvent) {
	if r.events == nil {
		return
	}
	select {
	case r.events <- event:
	default:
		// Channel full; drop the event to avoid blocking the loop.
	}
}

// ------- helpers -------

// applyDefaults fills in zero-value fields in RunConfig with sensible defaults.
func applyDefaults(cfg *RunConfig) {
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = defaultMaxIterations
	}
	if cfg.SleepBetween <= 0 {
		cfg.SleepBetween = defaultSleepBetween
	}
}

// sleepWithContext sleeps for d, returning early if ctx is cancelled.
// Returns nil if the sleep completed normally, ctx.Err() otherwise.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// appendRecent appends id to the recent list, keeping at most
// staleTaskThreshold entries.
func appendRecent(recent []string, id string) []string {
	recent = append(recent, id)
	if len(recent) > staleTaskThreshold {
		recent = recent[len(recent)-staleTaskThreshold:]
	}
	return recent
}

// isStale returns true if all entries in recent are identical (and the slice
// is full).
func isStale(recent []string) bool {
	if len(recent) < staleTaskThreshold {
		return false
	}
	first := recent[0]
	for _, id := range recent[1:] {
		if id != first {
			return false
		}
	}
	return true
}

// loopErrorEvent creates a LoopEvent of type EventLoopError with the given
// message.
func loopErrorEvent(iteration int, agentName, message string) LoopEvent {
	return LoopEvent{
		Type:      EventLoopError,
		Iteration: iteration,
		AgentName: agentName,
		Message:   message,
		Timestamp: time.Now(),
	}
}
