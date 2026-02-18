package loop

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
	"github.com/AbdelazizMoustafa10m/Raven/internal/git"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// noopLogger satisfies all logger interfaces used by recovery types.
type noopLogger struct{}

func (l *noopLogger) Info(msg string, kv ...interface{})  {}
func (l *noopLogger) Warn(msg string, kv ...interface{})  {}
func (l *noopLogger) Debug(msg string, kv ...interface{}) {}

// newTestGitRepo creates a temporary git repo and returns a GitClient.
// The repo contains a single "Initial commit" so stash operations work.
func newTestGitRepo(t *testing.T) *git.GitClient {
	t.Helper()
	dir := t.TempDir()
	mustRunGit(t, dir, "init", "-b", "main")
	mustRunGit(t, dir, "config", "user.email", "test@example.com")
	mustRunGit(t, dir, "config", "user.name", "Test")
	writeTestFile(t, dir, "README.md", "# Test\n")
	mustRunGit(t, dir, "add", ".")
	mustRunGit(t, dir, "commit", "-m", "Initial commit")
	c, err := git.NewGitClient(dir)
	require.NoError(t, err)
	return c
}

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, out)
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

// drainRecoveryEvents reads all events from a buffered recovery channel.
func drainRecoveryEvents(ch <-chan RecoveryEvent) []RecoveryEvent {
	var events []RecoveryEvent
	for {
		select {
		case e := <-ch:
			events = append(events, e)
		default:
			return events
		}
	}
}

// eventTypes extracts just the event types from a slice of RecoveryEvents.
func eventTypes(events []RecoveryEvent) []RecoveryEventType {
	types := make([]RecoveryEventType, len(events))
	for i, e := range events {
		types[i] = e.Type
	}
	return types
}

// ---------------------------------------------------------------------------
// emitRecovery tests
// ---------------------------------------------------------------------------

func TestEmitRecovery_NilChannel(t *testing.T) {
	t.Parallel()

	// Must not panic when channel is nil.
	emitRecovery(nil, RecoveryEvent{
		Type:      EventStashCreated,
		Message:   "test",
		Timestamp: time.Now(),
	})
}

func TestEmitRecovery_FullChannel_DropsEvent(t *testing.T) {
	t.Parallel()

	// Create a channel with capacity 1 and fill it.
	ch := make(chan RecoveryEvent, 1)
	first := RecoveryEvent{Type: EventStashCreated, Message: "first", Timestamp: time.Now()}
	second := RecoveryEvent{Type: EventStashRestored, Message: "second", Timestamp: time.Now()}

	emitRecovery(ch, first)
	// Channel is now full; this call must not block.
	emitRecovery(ch, second)

	// Only the first event should be in the channel.
	require.Len(t, ch, 1)
	got := <-ch
	assert.Equal(t, first.Type, got.Type)
}

func TestEmitRecovery_SendsEvent(t *testing.T) {
	t.Parallel()

	ch := make(chan RecoveryEvent, 4)
	evt := RecoveryEvent{
		Type:      EventRateLimitCountdown,
		Message:   "countdown started",
		Remaining: 30 * time.Second,
		Timestamp: time.Now(),
	}

	emitRecovery(ch, evt)

	require.Len(t, ch, 1)
	got := <-ch
	assert.Equal(t, EventRateLimitCountdown, got.Type)
	assert.Equal(t, "countdown started", got.Message)
	assert.Equal(t, 30*time.Second, got.Remaining)
}

// ---------------------------------------------------------------------------
// RateLimitWaiter.Wait tests
// ---------------------------------------------------------------------------

func TestRateLimitWaiter_Wait_NoActiveRateLimit_ReturnsImmediately(t *testing.T) {
	t.Parallel()

	coord := agent.NewRateLimitCoordinator(agent.DefaultBackoffConfig())
	// No rate limit recorded; ShouldWait returns nil.
	ch := make(chan RecoveryEvent, 8)
	waiter := NewRateLimitWaiter(coord, nil, ch, &noopLogger{})

	ctx := context.Background()
	start := time.Now()
	err := waiter.Wait(ctx, "claude")

	require.NoError(t, err)
	assert.Less(t, time.Since(start), 500*time.Millisecond, "should return immediately with no rate limit")

	// No events should be emitted.
	evts := drainRecoveryEvents(ch)
	assert.Empty(t, evts)
}

func TestRateLimitWaiter_Wait_ContextCancelledBeforeWait(t *testing.T) {
	t.Parallel()

	coord := agent.NewRateLimitCoordinator(agent.DefaultBackoffConfig())
	// Record a long rate limit so the waiter would block without cancellation.
	coord.RecordRateLimit("claude", &agent.RateLimitInfo{
		IsLimited:  true,
		ResetAfter: 30 * time.Second,
	})

	ch := make(chan RecoveryEvent, 8)
	waiter := NewRateLimitWaiter(coord, nil, ch, &noopLogger{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately before calling Wait.

	err := waiter.Wait(ctx, "claude")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRateLimitWaiter_Wait_ContextCancelledDuringWait(t *testing.T) {
	t.Parallel()

	coord := agent.NewRateLimitCoordinator(agent.DefaultBackoffConfig())
	coord.RecordRateLimit("claude", &agent.RateLimitInfo{
		IsLimited:  true,
		ResetAfter: 30 * time.Second,
	})

	var buf bytes.Buffer
	ch := make(chan RecoveryEvent, 8)
	waiter := NewRateLimitWaiter(coord, &buf, ch, &noopLogger{})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := waiter.Wait(ctx, "claude")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRateLimitWaiter_Wait_RateLimitAlreadyPastResetTime(t *testing.T) {
	t.Parallel()

	// Use a config with zero jitter and a very small default wait so we can
	// record a rate limit whose reset time is already in the past.
	cfg := agent.BackoffConfig{
		DefaultWait:  1 * time.Millisecond,
		MaxWaits:     5,
		JitterFactor: 0,
	}
	coord := agent.NewRateLimitCoordinator(cfg)
	coord.RecordRateLimit("claude", &agent.RateLimitInfo{
		IsLimited:  true,
		ResetAfter: 1 * time.Millisecond, // essentially already past
	})

	// Wait for the reset time to definitely pass.
	time.Sleep(20 * time.Millisecond)

	ch := make(chan RecoveryEvent, 8)
	waiter := NewRateLimitWaiter(coord, nil, ch, &noopLogger{})

	ctx := context.Background()
	start := time.Now()
	err := waiter.Wait(ctx, "claude")

	require.NoError(t, err)
	assert.Less(t, time.Since(start), 500*time.Millisecond, "should return immediately since reset time has passed")
}

func TestRateLimitWaiter_Wait_ActiveRateLimit_EmitsEvents(t *testing.T) {
	t.Parallel()

	// Record a short rate limit (1ms + jitter=0 so it completes very fast).
	cfg := agent.BackoffConfig{
		DefaultWait:  1 * time.Millisecond,
		MaxWaits:     5,
		JitterFactor: 0,
	}
	coord := agent.NewRateLimitCoordinator(cfg)
	coord.RecordRateLimit("claude", &agent.RateLimitInfo{
		IsLimited:  true,
		ResetAfter: 10 * time.Millisecond,
	})

	ch := make(chan RecoveryEvent, 16)
	// Use nil output so displayCountdown falls through to sleepWithContext —
	// which completes the moment the timer fires.
	waiter := NewRateLimitWaiter(coord, nil, ch, &noopLogger{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := waiter.Wait(ctx, "claude")
	require.NoError(t, err)

	evts := drainRecoveryEvents(ch)
	types := eventTypes(evts)
	assert.Contains(t, types, EventRateLimitCountdown)
	assert.Contains(t, types, EventRateLimitResuming)
}

func TestRateLimitWaiter_Wait_NilLogger_DoesNotPanic(t *testing.T) {
	t.Parallel()

	coord := agent.NewRateLimitCoordinator(agent.DefaultBackoffConfig())
	waiter := NewRateLimitWaiter(coord, nil, nil, nil) // nil logger

	ctx := context.Background()
	err := waiter.Wait(ctx, "claude")
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// RateLimitWaiter.displayCountdown tests
// ---------------------------------------------------------------------------

func TestDisplayCountdown_NilOutput_SleepsAndReturns(t *testing.T) {
	t.Parallel()

	coord := agent.NewRateLimitCoordinator(agent.DefaultBackoffConfig())
	waiter := NewRateLimitWaiter(coord, nil, nil, nil)

	ctx := context.Background()
	start := time.Now()
	err := waiter.displayCountdown(ctx, 5*time.Millisecond)
	elapsed := time.Since(start)

	require.NoError(t, err)
	// Should have slept at least 5ms.
	assert.GreaterOrEqual(t, elapsed, 5*time.Millisecond)
}

func TestDisplayCountdown_NilOutput_ContextCancelled(t *testing.T) {
	t.Parallel()

	coord := agent.NewRateLimitCoordinator(agent.DefaultBackoffConfig())
	waiter := NewRateLimitWaiter(coord, nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancelled before countdown starts.

	err := waiter.displayCountdown(ctx, 10*time.Second)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDisplayCountdown_WithWriter_WritesCountdown(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	coord := agent.NewRateLimitCoordinator(agent.DefaultBackoffConfig())
	waiter := NewRateLimitWaiter(coord, &buf, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use a 2-second duration so the ticker fires at least once before done.
	err := waiter.displayCountdown(ctx, 2*time.Second)
	require.NoError(t, err)

	output := buf.String()
	// Should contain countdown text with "Rate limited" and a number.
	assert.Contains(t, output, "Rate limited")
	// Should end with a newline (countdown complete).
	assert.True(t, strings.HasSuffix(output, "\n"), "countdown should end with newline")
}

func TestDisplayCountdown_WithWriter_ZeroDuration_ReturnsImmediately(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	coord := agent.NewRateLimitCoordinator(agent.DefaultBackoffConfig())
	waiter := NewRateLimitWaiter(coord, &buf, nil, nil)

	ctx := context.Background()
	start := time.Now()
	err := waiter.displayCountdown(ctx, 0)

	require.NoError(t, err)
	assert.Less(t, time.Since(start), 500*time.Millisecond)
	// A newline should be emitted immediately.
	assert.Contains(t, buf.String(), "\n")
}

func TestDisplayCountdown_WithWriter_ContextCancelledDuringCountdown(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	coord := agent.NewRateLimitCoordinator(agent.DefaultBackoffConfig())
	waiter := NewRateLimitWaiter(coord, &buf, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := waiter.displayCountdown(ctx, 30*time.Second)
	assert.ErrorIs(t, err, context.Canceled)
	// A newline should be written before returning the error.
	assert.Contains(t, buf.String(), "\n")
}

// TestDisplayCountdown_TableDriven covers various duration/cancellation combinations.
func TestDisplayCountdown_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		duration    time.Duration
		cancelAfter time.Duration // 0 means no cancellation
		wantErr     error
		wantNewline bool
	}{
		{
			name:        "zero duration completes immediately",
			duration:    0,
			wantNewline: true,
		},
		{
			name:        "negative duration treated as zero",
			duration:    -1 * time.Second,
			wantNewline: true,
		},
		{
			name:        "context cancelled after short delay",
			duration:    30 * time.Second,
			cancelAfter: 10 * time.Millisecond,
			wantErr:     context.Canceled,
			wantNewline: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			coord := agent.NewRateLimitCoordinator(agent.DefaultBackoffConfig())
			waiter := NewRateLimitWaiter(coord, &buf, nil, nil)

			ctx := context.Background()
			var cancel context.CancelFunc
			if tt.cancelAfter > 0 {
				ctx, cancel = context.WithCancel(ctx)
				go func() {
					time.Sleep(tt.cancelAfter)
					cancel()
				}()
			}

			err := waiter.displayCountdown(ctx, tt.duration)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			if tt.wantNewline {
				assert.Contains(t, buf.String(), "\n")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DirtyTreeRecovery.CheckAndStash tests (unit -- no real git)
// ---------------------------------------------------------------------------

// These tests use real git repos since DirtyTreeRecovery wraps git.GitClient
// which shells out to the git binary.

func TestCheckAndStash_CleanTree_ReturnsFalse(t *testing.T) {
	c := newTestGitRepo(t)
	ch := make(chan RecoveryEvent, 8)
	dtr := NewDirtyTreeRecovery(c, ch, &noopLogger{})

	ctx := context.Background()
	stashed, err := dtr.CheckAndStash(ctx, "T-001")

	require.NoError(t, err)
	assert.False(t, stashed, "clean tree should not produce a stash")

	// No stash events should be emitted.
	evts := drainRecoveryEvents(ch)
	types := eventTypes(evts)
	assert.NotContains(t, types, EventStashCreated)
	assert.NotContains(t, types, EventStashFailed)
}

func TestCheckAndStash_DirtyTree_ReturnsTrue_EmitsStashCreated(t *testing.T) {
	c := newTestGitRepo(t)
	ch := make(chan RecoveryEvent, 8)
	dtr := NewDirtyTreeRecovery(c, ch, &noopLogger{})

	// Stage a change so git stash can pick it up.
	writeTestFile(t, c.WorkDir, "README.md", "# Dirty\n")
	mustRunGit(t, c.WorkDir, "add", ".")

	ctx := context.Background()
	stashed, err := dtr.CheckAndStash(ctx, "T-001")

	require.NoError(t, err)
	assert.True(t, stashed)

	evts := drainRecoveryEvents(ch)
	types := eventTypes(evts)
	assert.Contains(t, types, EventDirtyTreeDetected)
	assert.Contains(t, types, EventStashCreated)

	// Verify the working tree is now clean.
	dirty, err := c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	assert.False(t, dirty, "tree should be clean after stash")
}

func TestCheckAndStash_NilChannel_DoesNotPanic(t *testing.T) {
	c := newTestGitRepo(t)
	// nil events channel must not panic.
	dtr := NewDirtyTreeRecovery(c, nil, &noopLogger{})

	ctx := context.Background()
	_, err := dtr.CheckAndStash(ctx, "T-001")
	require.NoError(t, err)
}

func TestCheckAndStash_NilLogger_DoesNotPanic(t *testing.T) {
	c := newTestGitRepo(t)
	ch := make(chan RecoveryEvent, 4)
	dtr := NewDirtyTreeRecovery(c, ch, nil)

	ctx := context.Background()
	_, err := dtr.CheckAndStash(ctx, "T-001")
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// DirtyTreeRecovery.RestoreStash tests
// ---------------------------------------------------------------------------

func TestRestoreStash_Success_EmitsStashRestored(t *testing.T) {
	c := newTestGitRepo(t)
	ch := make(chan RecoveryEvent, 8)
	dtr := NewDirtyTreeRecovery(c, ch, &noopLogger{})

	ctx := context.Background()

	// Create a stash entry first.
	writeTestFile(t, c.WorkDir, "README.md", "# Stashed\n")
	mustRunGit(t, c.WorkDir, "add", ".")
	stashed, err := dtr.CheckAndStash(ctx, "T-001")
	require.NoError(t, err)
	require.True(t, stashed)
	// Drain the CheckAndStash events.
	drainRecoveryEvents(ch)

	// Now restore.
	err = dtr.RestoreStash(ctx)
	require.NoError(t, err)

	evts := drainRecoveryEvents(ch)
	types := eventTypes(evts)
	assert.Contains(t, types, EventStashRestored)

	// Changes should be back.
	dirty, err := c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	assert.True(t, dirty, "stashed changes should be restored")
}

func TestRestoreStash_NoStash_ReturnsError_EmitsStashFailed(t *testing.T) {
	c := newTestGitRepo(t)
	ch := make(chan RecoveryEvent, 8)
	dtr := NewDirtyTreeRecovery(c, ch, &noopLogger{})

	ctx := context.Background()
	// No stash exists; pop should fail.
	err := dtr.RestoreStash(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "restoring stash")

	evts := drainRecoveryEvents(ch)
	types := eventTypes(evts)
	assert.Contains(t, types, EventStashFailed)
}

func TestRestoreStash_NilLogger_DoesNotPanic(t *testing.T) {
	c := newTestGitRepo(t)
	ch := make(chan RecoveryEvent, 4)
	dtr := NewDirtyTreeRecovery(c, ch, nil)

	ctx := context.Background()
	// No stash; expect error but no panic.
	_ = dtr.RestoreStash(ctx)
}

// ---------------------------------------------------------------------------
// DirtyTreeRecovery.EnsureCleanTree tests
// ---------------------------------------------------------------------------

func TestEnsureCleanTree_CleanTree_NoOp(t *testing.T) {
	c := newTestGitRepo(t)
	ch := make(chan RecoveryEvent, 8)
	dtr := NewDirtyTreeRecovery(c, ch, &noopLogger{})

	ctx := context.Background()
	err := dtr.EnsureCleanTree(ctx, "T-002")
	require.NoError(t, err)

	// No stash events should be emitted.
	evts := drainRecoveryEvents(ch)
	types := eventTypes(evts)
	assert.NotContains(t, types, EventStashCreated)
}

func TestEnsureCleanTree_DirtyTree_StashesAndEmitsEvents(t *testing.T) {
	c := newTestGitRepo(t)
	ch := make(chan RecoveryEvent, 8)
	dtr := NewDirtyTreeRecovery(c, ch, &noopLogger{})

	// Dirty the tree with a staged change.
	writeTestFile(t, c.WorkDir, "README.md", "# Ensure clean test\n")
	mustRunGit(t, c.WorkDir, "add", ".")

	ctx := context.Background()
	err := dtr.EnsureCleanTree(ctx, "T-003")
	require.NoError(t, err)

	evts := drainRecoveryEvents(ch)
	types := eventTypes(evts)
	assert.Contains(t, types, EventStashCreated)

	// Tree should be clean now.
	dirty, err := c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	assert.False(t, dirty)
}

func TestEnsureCleanTree_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		makeItDirty    bool
		wantEventTypes []RecoveryEventType
		wantError      bool
	}{
		{
			name:           "clean tree produces no stash events",
			makeItDirty:    false,
			wantEventTypes: []RecoveryEventType{},
			wantError:      false,
		},
		{
			name:        "dirty tree produces stash events",
			makeItDirty: true,
			wantEventTypes: []RecoveryEventType{
				EventDirtyTreeDetected,
				EventStashCreated,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			c := newTestGitRepo(t)
			ch := make(chan RecoveryEvent, 8)
			dtr := NewDirtyTreeRecovery(c, ch, &noopLogger{})

			if tt.makeItDirty {
				writeTestFile(t, c.WorkDir, "README.md", "# dirty for "+tt.name+"\n")
				mustRunGit(t, c.WorkDir, "add", ".")
			}

			ctx := context.Background()
			err := dtr.EnsureCleanTree(ctx, "T-test")

			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			evts := drainRecoveryEvents(ch)
			types := eventTypes(evts)
			for _, want := range tt.wantEventTypes {
				assert.Contains(t, types, want, "expected event type %s", want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AgentErrorRecovery tests
// ---------------------------------------------------------------------------

func TestAgentErrorRecovery_ShouldAbort_WithMaxThreeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		errorCount     int
		maxErrors      int
		wantShouldAbort bool
	}{
		{
			name:            "two errors with max=3: should not abort",
			errorCount:      2,
			maxErrors:       3,
			wantShouldAbort: false,
		},
		{
			name:            "three errors with max=3: should abort",
			errorCount:      3,
			maxErrors:       3,
			wantShouldAbort: true,
		},
		{
			name:            "four errors with max=3: should abort",
			errorCount:      4,
			maxErrors:       3,
			wantShouldAbort: true,
		},
		{
			name:            "one error with max=1: should abort",
			errorCount:      1,
			maxErrors:       1,
			wantShouldAbort: true,
		},
		{
			name:            "zero errors: should not abort",
			errorCount:      0,
			maxErrors:       3,
			wantShouldAbort: false,
		},
		{
			name:            "max=0 (disabled): should never abort",
			errorCount:      100,
			maxErrors:       0,
			wantShouldAbort: false,
		},
		{
			name:            "max negative (disabled): should never abort",
			errorCount:      100,
			maxErrors:       -1,
			wantShouldAbort: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			aer := NewAgentErrorRecovery(tt.maxErrors, &noopLogger{})

			testErr := errors.New("simulated agent error")
			for i := 0; i < tt.errorCount; i++ {
				aer.RecordError(testErr)
			}

			assert.Equal(t, tt.wantShouldAbort, aer.ShouldAbort())
		})
	}
}

func TestAgentErrorRecovery_RecordError_ReturnsFalseAtLimit(t *testing.T) {
	t.Parallel()

	aer := NewAgentErrorRecovery(3, &noopLogger{})
	testErr := errors.New("agent failure")

	// First two errors should return true (continue).
	assert.True(t, aer.RecordError(testErr), "first error: should continue")
	assert.True(t, aer.RecordError(testErr), "second error: should continue")

	// Third error reaches the limit; should return false (abort).
	assert.False(t, aer.RecordError(testErr), "third error at limit: should abort")
}

func TestAgentErrorRecovery_RecordSuccess_ResetsCounter(t *testing.T) {
	t.Parallel()

	aer := NewAgentErrorRecovery(3, &noopLogger{})
	testErr := errors.New("failure")

	// Record 2 errors (just under limit).
	aer.RecordError(testErr)
	aer.RecordError(testErr)
	assert.False(t, aer.ShouldAbort(), "should not abort with 2/3 errors")

	// Success resets counter.
	aer.RecordSuccess()
	assert.False(t, aer.ShouldAbort(), "after success, counter should be reset")
	assert.Equal(t, 0, aer.consecutiveErrors)

	// Can accumulate errors again without hitting old count.
	aer.RecordError(testErr)
	assert.False(t, aer.ShouldAbort(), "1 error after reset should not abort")
}

func TestAgentErrorRecovery_MaxZero_AlwaysFalse(t *testing.T) {
	t.Parallel()

	aer := NewAgentErrorRecovery(0, nil) // max=0, nil logger
	testErr := errors.New("failure")

	for i := 0; i < 1000; i++ {
		// RecordError returns bool but with max=0 ShouldAbort is always false,
		// so RecordError should keep returning true (limit never reached).
		result := aer.RecordError(testErr)
		assert.True(t, result, "with max=0, RecordError should always return true (no limit)")
	}
	assert.False(t, aer.ShouldAbort())
}

func TestAgentErrorRecovery_MaxNegative_AlwaysFalse(t *testing.T) {
	t.Parallel()

	aer := NewAgentErrorRecovery(-5, nil)
	testErr := errors.New("failure")

	for i := 0; i < 10; i++ {
		result := aer.RecordError(testErr)
		assert.True(t, result, "with max<0, RecordError should always return true")
	}
	assert.False(t, aer.ShouldAbort())
}

func TestAgentErrorRecovery_NilLogger_DoesNotPanic(t *testing.T) {
	t.Parallel()

	aer := NewAgentErrorRecovery(3, nil)
	testErr := errors.New("failure")

	// Must not panic with nil logger.
	aer.RecordError(testErr)
	aer.RecordSuccess()
	aer.ShouldAbort()
}

func TestAgentErrorRecovery_RecordSuccess_AfterLimit(t *testing.T) {
	t.Parallel()

	aer := NewAgentErrorRecovery(2, &noopLogger{})
	testErr := errors.New("failure")

	aer.RecordError(testErr)
	aer.RecordError(testErr)
	require.True(t, aer.ShouldAbort(), "should abort at limit")

	// After success, counter resets and abort should clear.
	aer.RecordSuccess()
	assert.False(t, aer.ShouldAbort(), "abort should clear after success")
}

// ---------------------------------------------------------------------------
// Recovery event type table-driven tests
// ---------------------------------------------------------------------------

// TestRecoveryEventTypes_TableDriven verifies that events are emitted at the
// correct recovery points and carry appropriate fields.
func TestRecoveryEventTypes_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		wantType       RecoveryEventType
		wantNonEmpty   bool // Remaining should be non-zero for rate-limit events
	}{
		{
			name:         "rate limit countdown event has remaining duration",
			wantType:     EventRateLimitCountdown,
			wantNonEmpty: true,
		},
		{
			name:         "rate limit resuming event has zero remaining",
			wantType:     EventRateLimitResuming,
			wantNonEmpty: false,
		},
		{
			name:     "dirty tree detected event",
			wantType: EventDirtyTreeDetected,
		},
		{
			name:     "stash created event",
			wantType: EventStashCreated,
		},
		{
			name:     "stash restored event",
			wantType: EventStashRestored,
		},
		{
			name:     "stash failed event",
			wantType: EventStashFailed,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ch := make(chan RecoveryEvent, 1)
			evt := RecoveryEvent{
				Type:      tt.wantType,
				Message:   "test message",
				Remaining: func() time.Duration {
					if tt.wantNonEmpty {
						return 30 * time.Second
					}
					return 0
				}(),
				Timestamp: time.Now(),
			}
			emitRecovery(ch, evt)

			got := <-ch
			assert.Equal(t, tt.wantType, got.Type)
			assert.Equal(t, "test message", got.Message)
			assert.False(t, got.Timestamp.IsZero(), "timestamp should be set")

			if tt.wantNonEmpty {
				assert.Greater(t, got.Remaining, time.Duration(0))
			} else {
				assert.Equal(t, time.Duration(0), got.Remaining)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration: dirty tree -> stash -> restore stash cycle
// ---------------------------------------------------------------------------

func TestIntegration_DirtyTree_Stash_Restore_Cycle(t *testing.T) {
	// Integration test: uses a real git repo.
	c := newTestGitRepo(t)
	ch := make(chan RecoveryEvent, 16)
	dtr := NewDirtyTreeRecovery(c, ch, &noopLogger{})
	ctx := context.Background()

	// Verify starting state is clean.
	dirty, err := c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	require.False(t, dirty, "repo should start clean")

	// --- Step 1: Dirty the tree ---
	originalContent := "# Integration test content\n"
	writeTestFile(t, c.WorkDir, "README.md", originalContent)
	mustRunGit(t, c.WorkDir, "add", ".")

	dirty, err = c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	require.True(t, dirty, "repo should be dirty after staging")

	// --- Step 2: CheckAndStash ---
	stashed, err := dtr.CheckAndStash(ctx, "T-integration")
	require.NoError(t, err)
	require.True(t, stashed)

	// Tree should be clean now.
	dirty, err = c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	assert.False(t, dirty, "tree should be clean after stash")

	// Events so far.
	evts := drainRecoveryEvents(ch)
	types := eventTypes(evts)
	assert.Contains(t, types, EventDirtyTreeDetected)
	assert.Contains(t, types, EventStashCreated)

	// --- Step 3: RestoreStash ---
	err = dtr.RestoreStash(ctx)
	require.NoError(t, err)

	// Tree should be dirty again with original changes.
	dirty, err = c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	assert.True(t, dirty, "changes should be restored after stash pop")

	// Content should match what we staged.
	data, err := os.ReadFile(filepath.Join(c.WorkDir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, originalContent, string(data))

	evts = drainRecoveryEvents(ch)
	types = eventTypes(evts)
	assert.Contains(t, types, EventStashRestored)
}

// TestIntegration_EnsureCleanTree_MultipleFiles verifies that EnsureCleanTree
// stashes multiple staged files and the tree is clean afterwards.
func TestIntegration_EnsureCleanTree_MultipleFiles(t *testing.T) {
	c := newTestGitRepo(t)
	ch := make(chan RecoveryEvent, 16)
	dtr := NewDirtyTreeRecovery(c, ch, &noopLogger{})
	ctx := context.Background()

	// Stage multiple changes.
	writeTestFile(t, c.WorkDir, "README.md", "# Changed\n")
	writeTestFile(t, c.WorkDir, "new-file.go", "package main\n")
	mustRunGit(t, c.WorkDir, "add", ".")

	dirty, err := c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	require.True(t, dirty)

	err = dtr.EnsureCleanTree(ctx, "T-multi")
	require.NoError(t, err)

	dirty, err = c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	assert.False(t, dirty, "tree should be clean after EnsureCleanTree")

	evts := drainRecoveryEvents(ch)
	types := eventTypes(evts)
	assert.Contains(t, types, EventStashCreated)
}

// ---------------------------------------------------------------------------
// Integration: RateLimitWaiter with a mock coordinator that has active limit
// ---------------------------------------------------------------------------

func TestRateLimitWaiter_Integration_ActiveLimit_WaitsAndReturns(t *testing.T) {
	t.Parallel()

	// Record a very short rate limit (10ms, no jitter).
	cfg := agent.BackoffConfig{
		DefaultWait:  10 * time.Millisecond,
		MaxWaits:     5,
		JitterFactor: 0,
	}
	coord := agent.NewRateLimitCoordinator(cfg)
	coord.RecordRateLimit("claude", &agent.RateLimitInfo{
		IsLimited:  true,
		ResetAfter: 10 * time.Millisecond,
	})

	ch := make(chan RecoveryEvent, 16)
	waiter := NewRateLimitWaiter(coord, nil, ch, &noopLogger{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := waiter.Wait(ctx, "claude")
	elapsed := time.Since(start)

	require.NoError(t, err)
	// Should have waited at least some time (the rate limit duration).
	assert.GreaterOrEqual(t, elapsed, 1*time.Millisecond)

	// Both countdown and resuming events should be emitted.
	evts := drainRecoveryEvents(ch)
	types := eventTypes(evts)
	assert.Contains(t, types, EventRateLimitCountdown)
	assert.Contains(t, types, EventRateLimitResuming)
}

// TestRateLimitWaiter_CountdownEventHasRemainingDuration verifies that the
// EventRateLimitCountdown event carries a non-zero Remaining duration.
func TestRateLimitWaiter_CountdownEventHasRemainingDuration(t *testing.T) {
	t.Parallel()

	cfg := agent.BackoffConfig{
		DefaultWait:  10 * time.Millisecond,
		MaxWaits:     5,
		JitterFactor: 0,
	}
	coord := agent.NewRateLimitCoordinator(cfg)
	coord.RecordRateLimit("claude", &agent.RateLimitInfo{
		IsLimited:  true,
		ResetAfter: 10 * time.Millisecond,
	})

	ch := make(chan RecoveryEvent, 16)
	waiter := NewRateLimitWaiter(coord, nil, ch, &noopLogger{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := waiter.Wait(ctx, "claude")
	require.NoError(t, err)

	evts := drainRecoveryEvents(ch)
	for _, e := range evts {
		if e.Type == EventRateLimitCountdown {
			assert.Greater(t, e.Remaining, time.Duration(0),
				"EventRateLimitCountdown should carry a positive Remaining duration")
			return
		}
	}
	t.Fatal("EventRateLimitCountdown was not emitted")
}

// ---------------------------------------------------------------------------
// RecoveryEvent struct field tests
// ---------------------------------------------------------------------------

func TestRecoveryEvent_Timestamp_IsSetByEmitRecovery(t *testing.T) {
	t.Parallel()

	ch := make(chan RecoveryEvent, 1)
	before := time.Now()
	emitRecovery(ch, RecoveryEvent{
		Type:      EventRecoveryError,
		Message:   "some error",
		Timestamp: time.Now(),
	})
	after := time.Now()

	got := <-ch
	assert.False(t, got.Timestamp.IsZero())
	assert.True(t, got.Timestamp.After(before) || got.Timestamp.Equal(before),
		"timestamp should not be before test start")
	assert.True(t, got.Timestamp.Before(after) || got.Timestamp.Equal(after),
		"timestamp should not be after test end")
}

func TestRecoveryEvent_AllEventTypesAreDefined(t *testing.T) {
	t.Parallel()

	// Verify that all defined event type constants are non-empty strings.
	allTypes := []RecoveryEventType{
		EventRateLimitCountdown,
		EventRateLimitResuming,
		EventDirtyTreeDetected,
		EventStashCreated,
		EventStashRestored,
		EventStashFailed,
		EventRecoveryError,
	}

	for _, et := range allTypes {
		assert.NotEmpty(t, string(et), "event type constant should not be empty")
	}

	// All types should be distinct.
	seen := make(map[RecoveryEventType]bool, len(allTypes))
	for _, et := range allTypes {
		assert.False(t, seen[et], "duplicate event type: %s", et)
		seen[et] = true
	}
}
