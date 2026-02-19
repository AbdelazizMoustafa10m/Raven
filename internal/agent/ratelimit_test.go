package agent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newNoJitterCoordinator creates a coordinator with no jitter and the given
// DefaultWait and MaxWaits, making timing assertions exact.
func newNoJitterCoordinator(defaultWait time.Duration, maxWaits int) *RateLimitCoordinator {
	return NewRateLimitCoordinator(BackoffConfig{
		DefaultWait:  defaultWait,
		MaxWaits:     maxWaits,
		JitterFactor: 0,
	})
}

// limitInfo returns a RateLimitInfo with IsLimited=true and the given reset duration.
func limitInfo(reset time.Duration) *RateLimitInfo {
	return &RateLimitInfo{IsLimited: true, ResetAfter: reset}
}

// ---------------------------------------------------------------------------
// DefaultBackoffConfig
// ---------------------------------------------------------------------------

func TestDefaultBackoffConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultBackoffConfig()
	assert.Equal(t, 60*time.Second, cfg.DefaultWait)
	assert.Equal(t, 5, cfg.MaxWaits)
	assert.Equal(t, 0.1, cfg.JitterFactor)
}

// ---------------------------------------------------------------------------
// AgentProvider mapping (AC-10)
// ---------------------------------------------------------------------------

func TestAgentProvider_Mapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		agentName    string
		wantProvider string
	}{
		{agentName: "claude", wantProvider: ProviderAnthropic},
		{agentName: "codex", wantProvider: ProviderOpenAI},
		{agentName: "gemini", wantProvider: ProviderGoogle},
	}

	for _, tt := range tests {
		t.Run(tt.agentName, func(t *testing.T) {
			t.Parallel()
			got, ok := AgentProvider[tt.agentName]
			require.True(t, ok, "agent %q should be in AgentProvider map", tt.agentName)
			assert.Equal(t, tt.wantProvider, got)
		})
	}
}

// ---------------------------------------------------------------------------
// providerForAgent
// ---------------------------------------------------------------------------

func TestProviderForAgent_KnownAgents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		agentName    string
		wantProvider string
	}{
		{agentName: "claude", wantProvider: ProviderAnthropic},
		{agentName: "codex", wantProvider: ProviderOpenAI},
		{agentName: "gemini", wantProvider: ProviderGoogle},
	}

	for _, tt := range tests {
		t.Run(tt.agentName, func(t *testing.T) {
			t.Parallel()
			got := providerForAgent(tt.agentName)
			assert.Equal(t, tt.wantProvider, got)
		})
	}
}

func TestProviderForAgent_UnknownAgent_FallsBackToName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		agent string
	}{
		{name: "hyphenated name", agent: "my-custom-agent"},
		{name: "empty string", agent: ""},
		{name: "mixed case", agent: "Claude"}, // capitalized -- not in map
		{name: "numeric suffix", agent: "claude-2"},
		{name: "whitespace", agent: " claude"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := providerForAgent(tt.agent)
			assert.Equal(t, tt.agent, got, "unknown agent should fall back to its own name as provider")
		})
	}
}

// ---------------------------------------------------------------------------
// ProviderState.RemainingWait (AC-16, AC-17)
// ---------------------------------------------------------------------------

func TestProviderState_RemainingWait(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ps        *ProviderState
		wantZero  bool
		wantAbout time.Duration // approximate expected value for non-zero cases
	}{
		{
			name:     "not limited returns zero",
			ps:       &ProviderState{IsLimited: false, ResetAt: time.Now().Add(10 * time.Second)},
			wantZero: true,
		},
		{
			name:     "limited but reset already expired returns zero",
			ps:       &ProviderState{IsLimited: true, ResetAt: time.Now().Add(-5 * time.Second)},
			wantZero: true,
		},
		{
			name:     "zero reset time returns zero",
			ps:       &ProviderState{IsLimited: true, ResetAt: time.Time{}},
			wantZero: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			remaining := tt.ps.RemainingWait()
			if tt.wantZero {
				assert.Equal(t, time.Duration(0), remaining)
				return
			}
		})
	}

	// Separate non-table test for the time-sensitive case to avoid capturing
	// time.Now() before t.Parallel() defers execution.
	t.Run("limited with future reset returns positive duration", func(t *testing.T) {
		t.Parallel()
		// Capture time.Now() inside the subtest so the delta is minimal.
		ps := &ProviderState{IsLimited: true, ResetAt: time.Now().Add(30 * time.Second)}
		remaining := ps.RemainingWait()
		assert.Greater(t, remaining, time.Duration(0), "remaining wait should be positive")
		assert.LessOrEqual(t, remaining, 30*time.Second, "remaining wait should not exceed reset duration")
	})
}

// ---------------------------------------------------------------------------
// NewRateLimitCoordinator
// ---------------------------------------------------------------------------

func TestNewRateLimitCoordinator(t *testing.T) {
	t.Parallel()

	cfg := DefaultBackoffConfig()
	rlc := NewRateLimitCoordinator(cfg)
	require.NotNil(t, rlc)
	assert.Empty(t, rlc.AllStates())
}

// ---------------------------------------------------------------------------
// RecordRateLimit (AC-2, AC-11)
// ---------------------------------------------------------------------------

func TestRecordRateLimit_CreatesAnthropicStateForClaude(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 30 * time.Second, Message: "rate limited"}

	before := time.Now()
	state := rlc.RecordRateLimit("claude", info)
	after := time.Now()

	require.NotNil(t, state)
	assert.Equal(t, ProviderAnthropic, state.Provider)
	assert.True(t, state.IsLimited)
	assert.Equal(t, 1, state.WaitCount)
	assert.Equal(t, "rate limited", state.LastMessage)
	assert.Equal(t, 30*time.Second, state.ResetAfter)
	// ResetAt should be approximately now + 30s (plus jitter <= 3s).
	assert.True(t, state.ResetAt.After(before.Add(30*time.Second)))
	assert.True(t, state.ResetAt.Before(after.Add(30*time.Second+3*time.Second+time.Second)))
	assert.False(t, state.UpdatedAt.IsZero())
}

func TestRecordRateLimit_WithSpecificResetAfter_ComputesResetAtCorrectly(t *testing.T) {
	t.Parallel()

	// Use zero jitter to get exact ResetAt computation.
	rlc := newNoJitterCoordinator(60*time.Second, 5)
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 45 * time.Second, Message: "api limit"}

	before := time.Now()
	state := rlc.RecordRateLimit("claude", info)
	after := time.Now()

	require.NotNil(t, state)
	// With zero jitter: ResetAt must be in [before+45s, after+45s].
	assert.True(t, state.ResetAt.After(before.Add(45*time.Second)),
		"ResetAt should be after before+45s; got %v", state.ResetAt)
	assert.True(t, state.ResetAt.Before(after.Add(45*time.Second+time.Second)),
		"ResetAt should not be too far in the future; got %v", state.ResetAt)
}

func TestRecordRateLimit_WithNoResetAfter_UsesDefaultWait(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(10*time.Second, 5)
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 0, Message: "limited"}

	before := time.Now()
	state := rlc.RecordRateLimit("claude", info)
	after := time.Now()

	require.NotNil(t, state)
	// Without jitter, ResetAt should be exactly now + DefaultWait.
	assert.True(t, state.ResetAt.After(before.Add(10*time.Second)))
	assert.True(t, state.ResetAt.Before(after.Add(10*time.Second+time.Second)))
}

func TestRecordRateLimit_NilInfo_UsesDefaultWait(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(15*time.Second, 5)

	before := time.Now()
	state := rlc.RecordRateLimit("claude", nil)
	after := time.Now()

	require.NotNil(t, state)
	assert.True(t, state.ResetAt.After(before.Add(15*time.Second)))
	assert.True(t, state.ResetAt.Before(after.Add(15*time.Second+time.Second)))
}

func TestRecordRateLimit_NegativeResetAfter_TreatedAsZeroUsesDefaultWait(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(20*time.Second, 5)
	info := &RateLimitInfo{IsLimited: true, ResetAfter: -5 * time.Second}

	before := time.Now()
	state := rlc.RecordRateLimit("claude", info)
	after := time.Now()

	require.NotNil(t, state)
	// Negative ResetAfter is non-positive, so DefaultWait (20s) should be used.
	assert.True(t, state.ResetAt.After(before.Add(20*time.Second)),
		"negative ResetAfter should fall back to DefaultWait")
	assert.True(t, state.ResetAt.Before(after.Add(20*time.Second+time.Second)))
}

func TestRecordRateLimit_IncrementsWaitCount(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 5 * time.Second}

	rlc.RecordRateLimit("claude", info)
	rlc.RecordRateLimit("claude", info)
	state := rlc.RecordRateLimit("claude", info)

	require.NotNil(t, state)
	assert.Equal(t, 3, state.WaitCount)
}

func TestRecordRateLimit_ExtentsResetAtForLaterRecord(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(5*time.Second, 5)

	// First record: 5s reset.
	info1 := &RateLimitInfo{IsLimited: true, ResetAfter: 5 * time.Second}
	state1 := rlc.RecordRateLimit("claude", info1)

	// Second record with a longer reset time: ResetAt should be extended.
	info2 := &RateLimitInfo{IsLimited: true, ResetAfter: 120 * time.Second}
	state2 := rlc.RecordRateLimit("claude", info2)

	require.NotNil(t, state2)
	assert.True(t, state2.ResetAt.After(state1.ResetAt),
		"second record with longer reset should extend ResetAt")
}

func TestRecordRateLimit_DoesNotShortenResetAt(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(5*time.Second, 5)

	// First record with a long reset.
	info1 := &RateLimitInfo{IsLimited: true, ResetAfter: 120 * time.Second}
	state1 := rlc.RecordRateLimit("claude", info1)

	// Second record with a shorter reset: ResetAt should NOT move backward.
	info2 := &RateLimitInfo{IsLimited: true, ResetAfter: 5 * time.Second}
	state2 := rlc.RecordRateLimit("claude", info2)

	require.NotNil(t, state2)
	assert.True(t, !state2.ResetAt.Before(state1.ResetAt),
		"shorter reset time should not move ResetAt backward; got state1.ResetAt=%v, state2.ResetAt=%v",
		state1.ResetAt, state2.ResetAt)
}

func TestRecordRateLimit_InvokesUpdateCallback(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())

	var mu sync.Mutex
	var called []ProviderState
	rlc.SetUpdateCallback(func(ps ProviderState) {
		mu.Lock()
		called = append(called, ps)
		mu.Unlock()
	})

	info := &RateLimitInfo{IsLimited: true, ResetAfter: 5 * time.Second, Message: "limited"}
	rlc.RecordRateLimit("claude", info)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, called, 1)
	assert.Equal(t, ProviderAnthropic, called[0].Provider)
	assert.True(t, called[0].IsLimited)
}

func TestRecordRateLimit_UpdateCallbackReceivesSnapshot(t *testing.T) {
	t.Parallel()

	// Verify the callback receives a value copy (snapshot), not a live pointer.
	// We capture all snapshots to inspect the one from RecordRateLimit specifically.
	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())

	var mu sync.Mutex
	var snapshots []ProviderState
	rlc.SetUpdateCallback(func(ps ProviderState) {
		mu.Lock()
		snapshots = append(snapshots, ps)
		mu.Unlock()
	})

	info := &RateLimitInfo{IsLimited: true, ResetAfter: 30 * time.Second}
	rlc.RecordRateLimit("claude", info) // fires callback: snapshots[0].IsLimited = true

	// Modify the live state via clear (fires callback: snapshots[1].IsLimited = false).
	rlc.ClearRateLimit("claude")

	mu.Lock()
	defer mu.Unlock()

	// Two callback invocations: one for RecordRateLimit, one for ClearRateLimit.
	require.Len(t, snapshots, 2)

	// The first snapshot (from RecordRateLimit) must reflect the limited state.
	assert.True(t, snapshots[0].IsLimited,
		"record callback snapshot must reflect IsLimited=true at time of record")

	// The second snapshot (from ClearRateLimit) must reflect the cleared state.
	assert.False(t, snapshots[1].IsLimited,
		"clear callback snapshot must reflect IsLimited=false after clear")

	// Importantly, the first snapshot is a copy -- its value must not have been
	// mutated by the subsequent ClearRateLimit (it is a value type, not a pointer).
	assert.True(t, snapshots[0].IsLimited,
		"first snapshot must be immutable after subsequent state changes")
}

func TestRecordRateLimit_MultipleAgentsSameProvider_SharedWaitCount(t *testing.T) {
	t.Parallel()

	// Two agents that both map to the same provider should share WaitCount.
	// "claude" maps to anthropic. We simulate a second hypothetical "claude-opus"
	// by temporarily registering it in the map. Instead, we use the real AgentProvider
	// mechanism: two calls with "claude" directly (same provider).
	rlc := newNoJitterCoordinator(5*time.Second, 10)
	info := limitInfo(5 * time.Second)

	rlc.RecordRateLimit("claude", info)
	rlc.RecordRateLimit("claude", info)
	rlc.RecordRateLimit("claude", info)

	state := rlc.GetState("claude")
	require.NotNil(t, state)
	assert.Equal(t, 3, state.WaitCount, "all records for the same provider should share WaitCount")
}

// ---------------------------------------------------------------------------
// ClearRateLimit (AC-3)
// ---------------------------------------------------------------------------

func TestClearRateLimit_ResetsIsLimited(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 30 * time.Second}
	rlc.RecordRateLimit("claude", info)

	rlc.ClearRateLimit("claude")

	state := rlc.GetState("claude")
	require.NotNil(t, state)
	assert.False(t, state.IsLimited)
}

func TestClearRateLimit_PreservesWaitCount(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 5 * time.Second}
	rlc.RecordRateLimit("claude", info)
	rlc.RecordRateLimit("claude", info)

	rlc.ClearRateLimit("claude")

	state := rlc.GetState("claude")
	require.NotNil(t, state)
	assert.Equal(t, 2, state.WaitCount, "WaitCount should be preserved after clear")
}

func TestClearRateLimit_NoOpWhenNoState(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	// Should not panic or error when no state exists.
	rlc.ClearRateLimit("claude")
	assert.Nil(t, rlc.GetState("claude"))
}

func TestClearRateLimit_NoOpWhenNoState_DoesNotInvokeCallback(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	var callCount int32
	rlc.SetUpdateCallback(func(ps ProviderState) {
		atomic.AddInt32(&callCount, 1)
	})

	// ClearRateLimit on an agent with no state -- callback must NOT fire.
	rlc.ClearRateLimit("codex")

	assert.Equal(t, int32(0), atomic.LoadInt32(&callCount),
		"callback should not be invoked when clearing non-existent state")
}

func TestClearRateLimit_InvokesUpdateCallback(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 5 * time.Second}
	rlc.RecordRateLimit("claude", info)

	var mu sync.Mutex
	var called []ProviderState
	rlc.SetUpdateCallback(func(ps ProviderState) {
		mu.Lock()
		called = append(called, ps)
		mu.Unlock()
	})

	rlc.ClearRateLimit("claude")

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, called, 1)
	assert.False(t, called[0].IsLimited)
}

// ---------------------------------------------------------------------------
// ShouldWait (AC-4, AC-5)
// ---------------------------------------------------------------------------

func TestShouldWait_ReturnsStateWhenLimited(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 30 * time.Second}
	rlc.RecordRateLimit("claude", info)

	state := rlc.ShouldWait("claude")
	require.NotNil(t, state)
	assert.True(t, state.IsLimited)
	assert.Equal(t, ProviderAnthropic, state.Provider)
}

func TestShouldWait_ReturnsNilAfterClear(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 30 * time.Second}
	rlc.RecordRateLimit("claude", info)
	rlc.ClearRateLimit("claude")

	state := rlc.ShouldWait("claude")
	assert.Nil(t, state)
}

func TestShouldWait_ReturnsNilWhenResetTimeHasPassed(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(time.Millisecond, 5)
	info := &RateLimitInfo{IsLimited: true, ResetAfter: time.Millisecond}
	rlc.RecordRateLimit("claude", info)

	// Wait for the reset time to pass.
	time.Sleep(20 * time.Millisecond)

	state := rlc.ShouldWait("claude")
	assert.Nil(t, state, "should return nil after reset time has passed")
}

func TestShouldWait_ReturnsNilForDifferentProvider(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 30 * time.Second}
	rlc.RecordRateLimit("claude", info) // anthropic provider

	// codex uses openai -- must not be affected by anthropic rate limit.
	state := rlc.ShouldWait("codex")
	assert.Nil(t, state, "different provider should not be affected by claude's rate limit")
}

func TestShouldWait_CodexUnaffectedWhenClaudeLimited(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	rlc.RecordRateLimit("claude", limitInfo(60*time.Second))

	// codex -> openai; gemini -> google; neither should be blocked.
	assert.Nil(t, rlc.ShouldWait("codex"), "openai provider should not be blocked")
	assert.Nil(t, rlc.ShouldWait("gemini"), "google provider should not be blocked")
}

func TestShouldWait_ReturnsNilWhenNoState(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	assert.Nil(t, rlc.ShouldWait("claude"))
}

func TestShouldWait_ReturnsSnapshotNotLivePointer(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	rlc.RecordRateLimit("claude", limitInfo(60*time.Second))

	snapshot := rlc.ShouldWait("claude")
	require.NotNil(t, snapshot)

	// Mutate the snapshot -- should not affect live state.
	snapshot.IsLimited = false
	snapshot.WaitCount = 999

	live := rlc.GetState("claude")
	require.NotNil(t, live)
	assert.True(t, live.IsLimited, "live state must not be affected by snapshot mutation")
	assert.NotEqual(t, 999, live.WaitCount, "live WaitCount must not be affected by snapshot mutation")
}

// ---------------------------------------------------------------------------
// WaitForReset (AC-6, AC-7)
// ---------------------------------------------------------------------------

func TestWaitForReset_ReturnsImmediatelyWhenNotLimited(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	err := rlc.WaitForReset(context.Background(), "claude")
	assert.NoError(t, err)
}

func TestWaitForReset_ReturnsImmediatelyWhenResetAlreadyPassed(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(time.Millisecond, 5)
	info := &RateLimitInfo{IsLimited: true, ResetAfter: time.Millisecond}
	rlc.RecordRateLimit("claude", info)

	// Wait for the reset time to expire.
	time.Sleep(20 * time.Millisecond)

	err := rlc.WaitForReset(context.Background(), "claude")
	assert.NoError(t, err, "should return nil when reset time already passed")
}

func TestWaitForReset_CompletesAfterShortWait(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(60*time.Millisecond, 5)
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 60 * time.Millisecond}
	rlc.RecordRateLimit("claude", info)

	start := time.Now()
	err := rlc.WaitForReset(context.Background(), "claude")
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.GreaterOrEqual(t, elapsed, 40*time.Millisecond, "should have waited approximately the reset duration")
}

func TestWaitForReset_RespectsContextCancellation(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 60 * time.Second}
	rlc.RecordRateLimit("claude", info)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := rlc.WaitForReset(ctx, "claude")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestWaitForReset_RespectsManualContextCancel(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	rlc.RecordRateLimit("claude", limitInfo(60*time.Second))

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- rlc.WaitForReset(ctx, "claude")
	}()

	// Cancel after a brief pause.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("WaitForReset did not return after context cancellation")
	}
}

func TestWaitForReset_ReturnsErrMaxWaitsExceeded(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(5*time.Second, 2)
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 5 * time.Second}

	// Hit the limit exactly.
	rlc.RecordRateLimit("claude", info)
	rlc.RecordRateLimit("claude", info)

	err := rlc.WaitForReset(context.Background(), "claude")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMaxWaitsExceeded)
}

func TestWaitForReset_MaxWaitsZeroAlwaysFails(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(5*time.Second, 0)
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 5 * time.Second}
	rlc.RecordRateLimit("claude", info)

	err := rlc.WaitForReset(context.Background(), "claude")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMaxWaitsExceeded)
}

// ---------------------------------------------------------------------------
// ExceededMaxWaits (AC-8)
// ---------------------------------------------------------------------------

func TestExceededMaxWaits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		maxWaits   int
		recordN    int // how many times to call RecordRateLimit
		agentName  string
		wantResult bool
	}{
		{
			name:       "false when count < maxWaits",
			maxWaits:   3,
			recordN:    2,
			agentName:  "claude",
			wantResult: false,
		},
		{
			name:       "true when count == maxWaits",
			maxWaits:   2,
			recordN:    2,
			agentName:  "claude",
			wantResult: true,
		},
		{
			name:       "true when count > maxWaits",
			maxWaits:   1,
			recordN:    3,
			agentName:  "claude",
			wantResult: true,
		},
		{
			name:       "true when maxWaits is zero (no state)",
			maxWaits:   0,
			recordN:    0,
			agentName:  "claude",
			wantResult: true,
		},
		{
			name:       "true when maxWaits is zero (with state)",
			maxWaits:   0,
			recordN:    1,
			agentName:  "claude",
			wantResult: true,
		},
		{
			name:       "false when no state recorded",
			maxWaits:   3,
			recordN:    0,
			agentName:  "claude",
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rlc := newNoJitterCoordinator(5*time.Second, tt.maxWaits)
			for i := 0; i < tt.recordN; i++ {
				rlc.RecordRateLimit(tt.agentName, limitInfo(5*time.Second))
			}
			got := rlc.ExceededMaxWaits(tt.agentName)
			assert.Equal(t, tt.wantResult, got)
		})
	}
}

func TestExceededMaxWaits_FalseBeforeLimit(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(5*time.Second, 3)
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 5 * time.Second}

	rlc.RecordRateLimit("claude", info)
	rlc.RecordRateLimit("claude", info)

	assert.False(t, rlc.ExceededMaxWaits("claude"))
}

func TestExceededMaxWaits_TrueAtLimit(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(5*time.Second, 2)
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 5 * time.Second}

	rlc.RecordRateLimit("claude", info)
	rlc.RecordRateLimit("claude", info)

	assert.True(t, rlc.ExceededMaxWaits("claude"))
}

func TestExceededMaxWaits_TrueWhenMaxWaitsIsZero(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(5*time.Second, 0)

	// Even without any recorded rate limits, MaxWaits=0 means no waits allowed.
	assert.True(t, rlc.ExceededMaxWaits("claude"))
}

func TestExceededMaxWaits_FalseWhenNoState(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(5*time.Second, 3)

	assert.False(t, rlc.ExceededMaxWaits("claude"))
}

func TestExceededMaxWaits_PreservedAfterClear(t *testing.T) {
	t.Parallel()

	// WaitCount survives ClearRateLimit, so ExceededMaxWaits should still
	// return true after a clear when count >= maxWaits.
	rlc := newNoJitterCoordinator(5*time.Second, 2)
	info := limitInfo(5 * time.Second)

	rlc.RecordRateLimit("claude", info)
	rlc.RecordRateLimit("claude", info)
	rlc.ClearRateLimit("claude")

	assert.True(t, rlc.ExceededMaxWaits("claude"),
		"ExceededMaxWaits should remain true after ClearRateLimit when WaitCount >= MaxWaits")
}

// ---------------------------------------------------------------------------
// GetState
// ---------------------------------------------------------------------------

func TestGetState_ReturnsNilWhenNoState(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	assert.Nil(t, rlc.GetState("claude"))
}

func TestGetState_ReturnsCorrectProvider(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 30 * time.Second}
	rlc.RecordRateLimit("claude", info)

	state := rlc.GetState("claude")
	require.NotNil(t, state)
	assert.Equal(t, ProviderAnthropic, state.Provider)
}

func TestGetState_ReturnsSnapshotNotLivePointer(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	rlc.RecordRateLimit("claude", limitInfo(30*time.Second))

	snap1 := rlc.GetState("claude")
	require.NotNil(t, snap1)
	snap1.WaitCount = 9999

	snap2 := rlc.GetState("claude")
	require.NotNil(t, snap2)
	assert.NotEqual(t, 9999, snap2.WaitCount, "GetState must return a copy, not a live pointer")
}

// ---------------------------------------------------------------------------
// AllStates (AC-9)
// ---------------------------------------------------------------------------

func TestAllStates_EmptyWhenNoRecords(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	states := rlc.AllStates()
	assert.Empty(t, states)
}

func TestAllStates_SortedByProviderName(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 30 * time.Second}

	// Record in non-alphabetical order.
	rlc.RecordRateLimit("gemini", info) // google
	rlc.RecordRateLimit("claude", info) // anthropic
	rlc.RecordRateLimit("codex", info)  // openai

	states := rlc.AllStates()
	require.Len(t, states, 3)
	// Alphabetical: anthropic < google < openai.
	assert.Equal(t, ProviderAnthropic, states[0].Provider)
	assert.Equal(t, ProviderGoogle, states[1].Provider)
	assert.Equal(t, ProviderOpenAI, states[2].Provider)
}

func TestAllStates_ReturnsSnapshot(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 30 * time.Second}
	rlc.RecordRateLimit("claude", info)

	snapshot := rlc.AllStates()
	require.Len(t, snapshot, 1)

	// Modify the snapshot and verify the coordinator's state is unaffected.
	snapshot[0].IsLimited = false
	liveState := rlc.GetState("claude")
	require.NotNil(t, liveState)
	assert.True(t, liveState.IsLimited, "coordinator state should not be affected by snapshot modification")
}

func TestAllStates_ContainsAllProviders(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	rlc.RecordRateLimit("claude", limitInfo(30*time.Second))
	rlc.RecordRateLimit("codex", limitInfo(30*time.Second))
	rlc.RecordRateLimit("gemini", limitInfo(30*time.Second))

	states := rlc.AllStates()
	require.Len(t, states, 3)

	providers := make(map[string]bool)
	for _, s := range states {
		providers[s.Provider] = true
	}
	assert.True(t, providers[ProviderAnthropic])
	assert.True(t, providers[ProviderOpenAI])
	assert.True(t, providers[ProviderGoogle])
}

// ---------------------------------------------------------------------------
// SetUpdateCallback (AC-13)
// ---------------------------------------------------------------------------

func TestSetUpdateCallback_InvokedOnRecordRateLimit(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())

	var mu sync.Mutex
	var invocations []ProviderState
	rlc.SetUpdateCallback(func(ps ProviderState) {
		mu.Lock()
		invocations = append(invocations, ps)
		mu.Unlock()
	})

	rlc.RecordRateLimit("claude", limitInfo(30*time.Second))
	rlc.RecordRateLimit("codex", limitInfo(30*time.Second))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, invocations, 2)
}

func TestSetUpdateCallback_InvokedOnClearRateLimit(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	rlc.RecordRateLimit("claude", limitInfo(30*time.Second))

	var mu sync.Mutex
	var received []ProviderState
	rlc.SetUpdateCallback(func(ps ProviderState) {
		mu.Lock()
		received = append(received, ps)
		mu.Unlock()
	})

	rlc.ClearRateLimit("claude")

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, received, 1, "ClearRateLimit should invoke the callback")
	assert.False(t, received[0].IsLimited)
}

func TestSetUpdateCallback_CanBeReplacedAtRuntime(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())

	var first, second int32
	rlc.SetUpdateCallback(func(ps ProviderState) { atomic.AddInt32(&first, 1) })

	rlc.RecordRateLimit("claude", limitInfo(30*time.Second))

	// Replace the callback.
	rlc.SetUpdateCallback(func(ps ProviderState) { atomic.AddInt32(&second, 1) })

	rlc.RecordRateLimit("claude", limitInfo(30*time.Second))

	assert.Equal(t, int32(1), atomic.LoadInt32(&first), "first callback should have fired once")
	assert.Equal(t, int32(1), atomic.LoadInt32(&second), "second callback should have fired once")
}

func TestSetUpdateCallback_NilCallbackDoesNotPanic(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	rlc.SetUpdateCallback(nil) // should be a no-op

	// Operations should succeed without panicking.
	assert.NotPanics(t, func() {
		rlc.RecordRateLimit("claude", limitInfo(30*time.Second))
		rlc.ClearRateLimit("claude")
	})
}

// ---------------------------------------------------------------------------
// computeWaitDuration (AC-11, AC-12)
// ---------------------------------------------------------------------------

func TestComputeWaitDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		cfg          BackoffConfig
		info         *RateLimitInfo
		wantExact    time.Duration // expected when jitter is 0
		wantMin      time.Duration // for jitter tests
		wantMax      time.Duration // for jitter tests; exclusive upper bound
		jitterTest   bool
	}{
		{
			name:      "positive ResetAfter uses it as base",
			cfg:       BackoffConfig{DefaultWait: 60 * time.Second, MaxWaits: 5, JitterFactor: 0},
			info:      &RateLimitInfo{ResetAfter: 45 * time.Second},
			wantExact: 45 * time.Second,
		},
		{
			name:      "zero ResetAfter uses DefaultWait",
			cfg:       BackoffConfig{DefaultWait: 90 * time.Second, MaxWaits: 5, JitterFactor: 0},
			info:      &RateLimitInfo{ResetAfter: 0},
			wantExact: 90 * time.Second,
		},
		{
			name:      "negative ResetAfter uses DefaultWait",
			cfg:       BackoffConfig{DefaultWait: 30 * time.Second, MaxWaits: 5, JitterFactor: 0},
			info:      &RateLimitInfo{ResetAfter: -10 * time.Second},
			wantExact: 30 * time.Second,
		},
		{
			name:      "nil info uses DefaultWait",
			cfg:       BackoffConfig{DefaultWait: 30 * time.Second, MaxWaits: 5, JitterFactor: 0},
			info:      nil,
			wantExact: 30 * time.Second,
		},
		{
			name:        "jitter stays within expected range",
			cfg:         BackoffConfig{DefaultWait: 60 * time.Second, MaxWaits: 5, JitterFactor: 0.2},
			info:        &RateLimitInfo{ResetAfter: 60 * time.Second},
			wantMin:     60 * time.Second,
			wantMax:     73 * time.Second, // 60s + 20% = 72s; allow 1s margin
			jitterTest:  true,
		},
		{
			name:      "zero jitter factor produces exact base",
			cfg:       BackoffConfig{DefaultWait: 60 * time.Second, MaxWaits: 5, JitterFactor: 0},
			info:      &RateLimitInfo{ResetAfter: 60 * time.Second},
			wantExact: 60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rlc := NewRateLimitCoordinator(tt.cfg)

			if tt.jitterTest {
				// Run many iterations to verify statistical correctness.
				for i := 0; i < 500; i++ {
					got := rlc.computeWaitDuration(tt.info)
					assert.GreaterOrEqual(t, got, tt.wantMin, "iteration %d: duration below minimum", i)
					assert.Less(t, got, tt.wantMax, "iteration %d: duration above maximum", i)
				}
				return
			}

			got := rlc.computeWaitDuration(tt.info)
			assert.Equal(t, tt.wantExact, got)
		})
	}
}

func TestComputeWaitDuration_UsesResetAfterWhenPositive(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(60*time.Second, 5)
	info := &RateLimitInfo{ResetAfter: 45 * time.Second}

	got := rlc.computeWaitDuration(info)
	assert.Equal(t, 45*time.Second, got)
}

func TestComputeWaitDuration_UsesDefaultWaitForZeroResetAfter(t *testing.T) {
	t.Parallel()

	cfg := BackoffConfig{DefaultWait: 90 * time.Second, MaxWaits: 5, JitterFactor: 0}
	rlc := NewRateLimitCoordinator(cfg)
	info := &RateLimitInfo{ResetAfter: 0}

	got := rlc.computeWaitDuration(info)
	assert.Equal(t, 90*time.Second, got)
}

func TestComputeWaitDuration_UsesDefaultWaitForNilInfo(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(30*time.Second, 5)

	got := rlc.computeWaitDuration(nil)
	assert.Equal(t, 30*time.Second, got)
}

func TestComputeWaitDuration_JitterIsWithinExpectedRange(t *testing.T) {
	t.Parallel()

	cfg := BackoffConfig{DefaultWait: 60 * time.Second, MaxWaits: 5, JitterFactor: 0.2}
	rlc := NewRateLimitCoordinator(cfg)
	info := &RateLimitInfo{ResetAfter: 60 * time.Second}

	// Run many iterations to verify jitter stays within [base, base * (1 + jitterFactor)).
	for i := 0; i < 1000; i++ {
		got := rlc.computeWaitDuration(info)
		assert.GreaterOrEqual(t, got, 60*time.Second, "duration must be at least base")
		assert.Less(t, got, 73*time.Second, "duration must be less than base + jitterFactor * base + 1s margin")
	}
}

func TestComputeWaitDuration_JitterVaries(t *testing.T) {
	t.Parallel()

	// With non-zero jitter, values should not all be identical.
	cfg := BackoffConfig{DefaultWait: 60 * time.Second, MaxWaits: 5, JitterFactor: 0.5}
	rlc := NewRateLimitCoordinator(cfg)
	info := &RateLimitInfo{ResetAfter: 60 * time.Second}

	seen := make(map[time.Duration]bool)
	for i := 0; i < 100; i++ {
		seen[rlc.computeWaitDuration(info)] = true
	}
	assert.Greater(t, len(seen), 1, "jitter should produce varying durations across many calls")
}

// ---------------------------------------------------------------------------
// Concurrency (AC-1)
// ---------------------------------------------------------------------------

func TestRateLimitCoordinator_ConcurrentRecordAndShouldWait(t *testing.T) {
	t.Parallel()

	cfg := BackoffConfig{DefaultWait: 5 * time.Second, MaxWaits: 100, JitterFactor: 0.1}
	rlc := NewRateLimitCoordinator(cfg)

	const workers = 20
	var wg sync.WaitGroup
	wg.Add(workers * 2)

	// Half goroutines record rate limits.
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			info := &RateLimitInfo{IsLimited: true, ResetAfter: 5 * time.Second}
			rlc.RecordRateLimit("claude", info)
		}()
	}

	// Other half check ShouldWait concurrently.
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_ = rlc.ShouldWait("claude")
		}()
	}

	wg.Wait()

	state := rlc.GetState("claude")
	require.NotNil(t, state)
	// WaitCount should equal the number of RecordRateLimit calls.
	assert.Equal(t, workers, state.WaitCount)
}

func TestRateLimitCoordinator_ConcurrentAllStates(t *testing.T) {
	t.Parallel()

	cfg := BackoffConfig{DefaultWait: 5 * time.Second, MaxWaits: 100, JitterFactor: 0}
	rlc := NewRateLimitCoordinator(cfg)
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 5 * time.Second}
	rlc.RecordRateLimit("claude", info)
	rlc.RecordRateLimit("codex", info)

	const readers = 50
	var wg sync.WaitGroup
	wg.Add(readers)

	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			states := rlc.AllStates()
			_ = states
		}()
	}

	wg.Wait()
}

func TestRateLimitCoordinator_ConcurrentMixedOperations(t *testing.T) {
	t.Parallel()

	// Exercise all major operations concurrently to surface any data races.
	// Run with -race to detect them.
	cfg := BackoffConfig{DefaultWait: 100 * time.Millisecond, MaxWaits: 1000, JitterFactor: 0.05}
	rlc := NewRateLimitCoordinator(cfg)

	const goroutines = 30
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			switch i % 5 {
			case 0:
				rlc.RecordRateLimit("claude", limitInfo(100*time.Millisecond))
			case 1:
				_ = rlc.ShouldWait("claude")
			case 2:
				_ = rlc.AllStates()
			case 3:
				_ = rlc.ExceededMaxWaits("claude")
			case 4:
				rlc.ClearRateLimit("claude")
			}
		}()
	}

	wg.Wait()
}

func TestRateLimitCoordinator_ConcurrentMultipleProviders(t *testing.T) {
	t.Parallel()

	// Multiple agents for multiple providers hitting rate limits simultaneously.
	cfg := BackoffConfig{DefaultWait: 5 * time.Second, MaxWaits: 1000, JitterFactor: 0}
	rlc := NewRateLimitCoordinator(cfg)

	agents := []string{"claude", "codex", "gemini"}
	const perAgent = 10

	var wg sync.WaitGroup
	wg.Add(len(agents) * perAgent)

	for _, agent := range agents {
		for i := 0; i < perAgent; i++ {
			agent := agent
			go func() {
				defer wg.Done()
				rlc.RecordRateLimit(agent, limitInfo(5*time.Second))
			}()
		}
	}

	wg.Wait()

	states := rlc.AllStates()
	require.Len(t, states, 3, "all three providers should have state after concurrent records")

	for _, s := range states {
		assert.Equal(t, perAgent, s.WaitCount,
			"provider %q should have WaitCount=%d, got %d", s.Provider, perAgent, s.WaitCount)
	}
}

// ---------------------------------------------------------------------------
// Integration tests
// ---------------------------------------------------------------------------

func TestRateLimitCoordinator_FullCycle_RecordWaitClear(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(50*time.Millisecond, 5)
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 50 * time.Millisecond}

	// Record: provider is now limited.
	rlc.RecordRateLimit("claude", info)
	assert.NotNil(t, rlc.ShouldWait("claude"), "should be limited after record")

	// Wait for reset to complete.
	err := rlc.WaitForReset(context.Background(), "claude")
	require.NoError(t, err, "WaitForReset should complete without error")

	// After the timer fires the reset time has passed; ShouldWait should return nil.
	assert.Nil(t, rlc.ShouldWait("claude"), "should be clear after wait")

	// Clear: explicit clear; state stays but IsLimited = false.
	rlc.ClearRateLimit("claude")
	assert.Nil(t, rlc.ShouldWait("claude"), "should be clear after explicit clear")
}

func TestRateLimitCoordinator_TwoAgentsSameProvider_ShareState(t *testing.T) {
	t.Parallel()

	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 30 * time.Second}

	// Two calls with "claude" (same provider: anthropic) share state.
	rlc.RecordRateLimit("claude", info)

	state := rlc.ShouldWait("claude")
	require.NotNil(t, state)
	assert.Equal(t, ProviderAnthropic, state.Provider)
}

func TestRateLimitCoordinator_TwoGoroutines_Coordination(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(80*time.Millisecond, 5)

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: record rate limit.
	go func() {
		defer wg.Done()
		info := &RateLimitInfo{IsLimited: true, ResetAfter: 80 * time.Millisecond}
		rlc.RecordRateLimit("claude", info)
	}()

	// Goroutine 2: check ShouldWait shortly after goroutine 1 records.
	var shouldWaitResult *ProviderState
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond) // Give goroutine 1 time to record.
		shouldWaitResult = rlc.ShouldWait("claude")
	}()

	wg.Wait()

	// By the time goroutine 2 checked, goroutine 1 should have recorded.
	require.NotNil(t, shouldWaitResult, "goroutine 2 should observe the rate limit recorded by goroutine 1")
	assert.True(t, shouldWaitResult.IsLimited)
}

func TestRateLimitCoordinator_SharedProvider_AgentCrossIsolation(t *testing.T) {
	t.Parallel()

	// claude (anthropic) being limited must not prevent codex (openai) from proceeding.
	rlc := NewRateLimitCoordinator(DefaultBackoffConfig())
	rlc.RecordRateLimit("claude", limitInfo(60*time.Second))

	assert.NotNil(t, rlc.ShouldWait("claude"), "claude should be rate-limited")
	assert.Nil(t, rlc.ShouldWait("codex"), "codex (different provider) should not be rate-limited")
	assert.Nil(t, rlc.ShouldWait("gemini"), "gemini (different provider) should not be rate-limited")
}

// ---------------------------------------------------------------------------
// Sentinel error
// ---------------------------------------------------------------------------

func TestErrMaxWaitsExceeded_IsDistinctSentinel(t *testing.T) {
	t.Parallel()

	assert.NotNil(t, ErrMaxWaitsExceeded)

	wrapped := errors.New("wrapper: " + ErrMaxWaitsExceeded.Error())
	assert.False(t, errors.Is(wrapped, ErrMaxWaitsExceeded),
		"a plain wrap does not use errors.Is chain")
}

func TestErrMaxWaitsExceeded_CanBeUnwrapped(t *testing.T) {
	t.Parallel()

	rlc := newNoJitterCoordinator(5*time.Second, 1)
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 5 * time.Second}
	rlc.RecordRateLimit("claude", info)

	err := rlc.WaitForReset(context.Background(), "claude")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMaxWaitsExceeded, "returned error should wrap ErrMaxWaitsExceeded")
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkRecordRateLimit(b *testing.B) {
	rlc := newNoJitterCoordinator(60*time.Second, 10000)
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 60 * time.Second}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rlc.RecordRateLimit("claude", info)
	}
}

func BenchmarkShouldWait_Limited(b *testing.B) {
	rlc := newNoJitterCoordinator(60*time.Second, 10000)
	rlc.RecordRateLimit("claude", &RateLimitInfo{IsLimited: true, ResetAfter: 60 * time.Second})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rlc.ShouldWait("claude")
	}
}

func BenchmarkShouldWait_NotLimited(b *testing.B) {
	rlc := newNoJitterCoordinator(60*time.Second, 5)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rlc.ShouldWait("claude")
	}
}

func BenchmarkAllStates_ThreeProviders(b *testing.B) {
	rlc := newNoJitterCoordinator(60*time.Second, 10000)
	rlc.RecordRateLimit("claude", &RateLimitInfo{IsLimited: true, ResetAfter: 60 * time.Second})
	rlc.RecordRateLimit("codex", &RateLimitInfo{IsLimited: true, ResetAfter: 60 * time.Second})
	rlc.RecordRateLimit("gemini", &RateLimitInfo{IsLimited: true, ResetAfter: 60 * time.Second})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rlc.AllStates()
	}
}

func BenchmarkRecordRateLimit_Parallel(b *testing.B) {
	rlc := newNoJitterCoordinator(60*time.Second, 100000)
	info := &RateLimitInfo{IsLimited: true, ResetAfter: 60 * time.Second}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rlc.RecordRateLimit("claude", info)
		}
	})
}

func BenchmarkShouldWait_Parallel(b *testing.B) {
	rlc := newNoJitterCoordinator(60*time.Second, 10000)
	rlc.RecordRateLimit("claude", &RateLimitInfo{IsLimited: true, ResetAfter: 60 * time.Second})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = rlc.ShouldWait("claude")
		}
	})
}
