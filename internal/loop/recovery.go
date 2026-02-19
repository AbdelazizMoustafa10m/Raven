package loop

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
	"github.com/AbdelazizMoustafa10m/Raven/internal/git"
)

// RecoveryEventType identifies the type of recovery event.
type RecoveryEventType string

const (
	// EventRateLimitCountdown is emitted when a rate-limit countdown begins.
	EventRateLimitCountdown RecoveryEventType = "rate_limit_countdown"
	// EventRateLimitResuming is emitted when the countdown ends and the agent may retry.
	EventRateLimitResuming RecoveryEventType = "rate_limit_resuming"
	// EventDirtyTreeDetected is emitted when uncommitted changes are found.
	EventDirtyTreeDetected RecoveryEventType = "dirty_tree_detected"
	// EventStashCreated is emitted when changes are successfully stashed.
	EventStashCreated RecoveryEventType = "stash_created"
	// EventStashRestored is emitted when the stash is successfully popped.
	EventStashRestored RecoveryEventType = "stash_restored"
	// EventStashFailed is emitted when a stash push or pop operation fails.
	EventStashFailed RecoveryEventType = "stash_failed"
	// EventRecoveryError is emitted when a recovery step encounters an error.
	EventRecoveryError RecoveryEventType = "recovery_error"
)

// RecoveryEvent is a structured event emitted during recovery operations.
type RecoveryEvent struct {
	// Type identifies what happened.
	Type RecoveryEventType
	// Message is a human-readable description of the event.
	Message string
	// Remaining is the time remaining in a rate-limit countdown (zero otherwise).
	Remaining time.Duration
	// Timestamp is the wall-clock time at which the event was created.
	Timestamp time.Time
}

// emitRecovery sends a RecoveryEvent to ch in a non-blocking fashion.
// If ch is nil or full, the event is silently dropped.
func emitRecovery(ch chan<- RecoveryEvent, event RecoveryEvent) {
	if ch == nil {
		return
	}
	select {
	case ch <- event:
	default:
		// Channel full; drop the event.
	}
}

// ---------------------------------------------------------------------------
// RateLimitWaiter
// ---------------------------------------------------------------------------

// RateLimitWaiter blocks the loop until the rate limit for a given agent
// has reset, displaying a live countdown to the output writer and emitting
// structured recovery events.
type RateLimitWaiter struct {
	coordinator *agent.RateLimitCoordinator
	output      io.Writer
	events      chan<- RecoveryEvent
	logger      interface {
		Info(msg string, kv ...interface{})
	}
}

// NewRateLimitWaiter creates a RateLimitWaiter. output may be nil to suppress
// the countdown display. events may be nil to disable event emission.
func NewRateLimitWaiter(
	coordinator *agent.RateLimitCoordinator,
	output io.Writer,
	events chan<- RecoveryEvent,
	logger interface {
		Info(msg string, kv ...interface{})
	},
) *RateLimitWaiter {
	return &RateLimitWaiter{
		coordinator: coordinator,
		output:      output,
		events:      events,
		logger:      logger,
	}
}

// Wait blocks until the rate limit for agentName has reset.
// If no rate limit is active for the agent, Wait returns immediately.
// If ctx is cancelled before the reset, Wait returns ctx.Err().
func (w *RateLimitWaiter) Wait(ctx context.Context, agentName string) error {
	state := w.coordinator.ShouldWait(agentName)
	if state == nil {
		return nil
	}

	remaining := state.RemainingWait()
	if remaining <= 0 {
		return nil
	}

	if w.logger != nil {
		w.logger.Info("rate limit active, waiting for reset",
			"agent", agentName,
			"remaining", remaining.Round(time.Second),
		)
	}

	emitRecovery(w.events, RecoveryEvent{
		Type:      EventRateLimitCountdown,
		Message:   fmt.Sprintf("rate limited for %s, waiting %s", agentName, remaining.Round(time.Second)),
		Remaining: remaining,
		Timestamp: time.Now(),
	})

	if err := w.displayCountdown(ctx, remaining); err != nil {
		return err
	}

	emitRecovery(w.events, RecoveryEvent{
		Type:      EventRateLimitResuming,
		Message:   fmt.Sprintf("rate limit reset for %s, resuming", agentName),
		Timestamp: time.Now(),
	})

	return nil
}

// displayCountdown renders a live countdown to w.output, ticking once per
// second. Returns nil when the duration has elapsed, or ctx.Err() if the
// context is cancelled first. If w.output is nil the function sleeps for the
// full duration using sleepWithContext (which is cancellable via ctx).
func (w *RateLimitWaiter) displayCountdown(ctx context.Context, duration time.Duration) error {
	if w.output == nil {
		return sleepWithContext(ctx, duration)
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	totalSeconds := int(duration.Seconds())
	if totalSeconds < 0 {
		totalSeconds = 0
	}

	elapsed := 0

	for {
		remaining := totalSeconds - elapsed
		if remaining <= 0 {
			// Countdown complete; move to a new line.
			fmt.Fprint(w.output, "\n")
			return nil
		}

		fmt.Fprintf(w.output, "\rRate limited. Retrying in %ds...  ", remaining)

		select {
		case <-ticker.C:
			elapsed++
		case <-ctx.Done():
			fmt.Fprint(w.output, "\n")
			return ctx.Err()
		}
	}
}

// ---------------------------------------------------------------------------
// DirtyTreeRecovery
// ---------------------------------------------------------------------------

// DirtyTreeRecovery detects and handles uncommitted changes in the working
// tree before an agent run, ensuring the agent starts from a clean state.
type DirtyTreeRecovery struct {
	gitClient *git.GitClient
	events    chan<- RecoveryEvent
	logger    interface {
		Info(msg string, kv ...interface{})
		Warn(msg string, kv ...interface{})
	}
}

// NewDirtyTreeRecovery creates a DirtyTreeRecovery. events may be nil.
func NewDirtyTreeRecovery(
	gitClient *git.GitClient,
	events chan<- RecoveryEvent,
	logger interface {
		Info(msg string, kv ...interface{})
		Warn(msg string, kv ...interface{})
	},
) *DirtyTreeRecovery {
	return &DirtyTreeRecovery{
		gitClient: gitClient,
		events:    events,
		logger:    logger,
	}
}

// CheckAndStash checks for uncommitted changes and stashes them if present.
// Returns true if changes were stashed, false if the working tree was already
// clean (or if git stash reported nothing to save).
func (dtr *DirtyTreeRecovery) CheckAndStash(ctx context.Context, taskID string) (bool, error) {
	dirty, err := dtr.gitClient.HasUncommittedChanges(ctx)
	if err != nil {
		return false, fmt.Errorf("dirty tree: checking status: %w", err)
	}
	if !dirty {
		return false, nil
	}

	emitRecovery(dtr.events, RecoveryEvent{
		Type:      EventDirtyTreeDetected,
		Message:   fmt.Sprintf("dirty working tree detected for task %s", taskID),
		Timestamp: time.Now(),
	})

	if dtr.logger != nil {
		dtr.logger.Info("dirty working tree detected, stashing",
			"task", taskID,
		)
	}

	stashed, err := dtr.gitClient.Stash(ctx, "raven-autostash: "+taskID)
	if err != nil {
		emitRecovery(dtr.events, RecoveryEvent{
			Type:      EventStashFailed,
			Message:   fmt.Sprintf("stash failed for task %s: %s", taskID, err),
			Timestamp: time.Now(),
		})
		return false, fmt.Errorf("dirty tree: stashing: %w", err)
	}
	if !stashed {
		// Nothing was stashed (e.g., only untracked files).
		return false, nil
	}

	emitRecovery(dtr.events, RecoveryEvent{
		Type:      EventStashCreated,
		Message:   fmt.Sprintf("stashed changes for task %s", taskID),
		Timestamp: time.Now(),
	})

	return true, nil
}

// RestoreStash pops the most recent stash entry created by CheckAndStash.
func (dtr *DirtyTreeRecovery) RestoreStash(ctx context.Context) error {
	if err := dtr.gitClient.StashPop(ctx); err != nil {
		if dtr.logger != nil {
			dtr.logger.Warn("stash pop failed", "err", err)
		}
		emitRecovery(dtr.events, RecoveryEvent{
			Type:      EventStashFailed,
			Message:   fmt.Sprintf("stash pop failed: %s", err),
			Timestamp: time.Now(),
		})
		return fmt.Errorf("restoring stash: %w", err)
	}

	emitRecovery(dtr.events, RecoveryEvent{
		Type:      EventStashRestored,
		Message:   "stash restored successfully",
		Timestamp: time.Now(),
	})

	return nil
}

// EnsureCleanTree verifies the working tree is clean. If dirty, it stashes
// the changes. Returns an error if the status check or stash operation fails.
func (dtr *DirtyTreeRecovery) EnsureCleanTree(ctx context.Context, taskID string) error {
	_, err := dtr.CheckAndStash(ctx, taskID)
	if err != nil {
		emitRecovery(dtr.events, RecoveryEvent{
			Type:      EventRecoveryError,
			Message:   fmt.Sprintf("ensure clean tree failed for task %s: %s", taskID, err),
			Timestamp: time.Now(),
		})
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// AgentErrorRecovery
// ---------------------------------------------------------------------------

// AgentErrorRecovery tracks consecutive agent errors and decides whether the
// implementation loop should continue or abort.
type AgentErrorRecovery struct {
	maxConsecutiveErrors int
	consecutiveErrors    int
	logger               interface {
		Warn(msg string, kv ...interface{})
	}
}

// NewAgentErrorRecovery creates an AgentErrorRecovery. Set
// maxConsecutiveErrors to 0 or negative to disable the limit (the loop never
// aborts due to consecutive errors). logger may be nil.
func NewAgentErrorRecovery(maxConsecutiveErrors int, logger interface {
	Warn(msg string, kv ...interface{})
}) *AgentErrorRecovery {
	return &AgentErrorRecovery{
		maxConsecutiveErrors: maxConsecutiveErrors,
		logger:               logger,
	}
}

// RecordError records an agent error and returns whether the loop should
// continue. Returns false when the consecutive error limit has been reached,
// signalling the caller to abort the loop.
func (aer *AgentErrorRecovery) RecordError(err error) bool {
	aer.consecutiveErrors++

	if aer.logger != nil {
		aer.logger.Warn("agent error recorded",
			"consecutiveErrors", aer.consecutiveErrors,
			"max", aer.maxConsecutiveErrors,
			"error", err,
		)
	}

	if aer.ShouldAbort() {
		if aer.logger != nil {
			aer.logger.Warn("consecutive error limit reached",
				"consecutiveErrors", aer.consecutiveErrors,
				"max", aer.maxConsecutiveErrors,
			)
		}
		return false
	}

	return true
}

// RecordSuccess resets the consecutive error counter. Call this after each
// successful agent invocation.
func (aer *AgentErrorRecovery) RecordSuccess() {
	aer.consecutiveErrors = 0
}

// ShouldAbort returns true if the consecutive error count has reached or
// exceeded the configured maximum. Returns false when the limit is disabled
// (maxConsecutiveErrors <= 0).
func (aer *AgentErrorRecovery) ShouldAbort() bool {
	if aer.maxConsecutiveErrors <= 0 {
		return false
	}
	return aer.consecutiveErrors >= aer.maxConsecutiveErrors
}
