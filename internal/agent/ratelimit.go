package agent

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// ErrMaxWaitsExceeded is returned by WaitForReset when the provider has
// exceeded the configured maximum number of rate-limit waits.
var ErrMaxWaitsExceeded = errors.New("max rate-limit waits exceeded")

// Provider names for rate-limit grouping.
const (
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
	ProviderGoogle    = "google"
)

// AgentProvider maps agent names to their API provider.
// Agents sharing a provider share rate-limit state.
var AgentProvider = map[string]string{
	"claude": ProviderAnthropic,
	"codex":  ProviderOpenAI,
	"gemini": ProviderGoogle,
}

// BackoffConfig configures rate-limit backoff behavior.
type BackoffConfig struct {
	// DefaultWait is the wait duration used when the agent does not report a
	// specific reset time (default: 60s).
	DefaultWait time.Duration

	// MaxWaits is the maximum number of rate-limit waits before the
	// coordinator returns ErrMaxWaitsExceeded (default: 5).
	// A value of 0 means no waits are allowed.
	MaxWaits int

	// JitterFactor is a multiplier in [0.0, 1.0] applied to the computed wait
	// duration to introduce randomness and avoid thundering-herd effects
	// (default: 0.1).
	JitterFactor float64
}

// DefaultBackoffConfig returns sensible default backoff configuration.
func DefaultBackoffConfig() BackoffConfig {
	return BackoffConfig{
		DefaultWait:  60 * time.Second,
		MaxWaits:     5,
		JitterFactor: 0.1,
	}
}

// ProviderState tracks rate-limit state for a single API provider.
type ProviderState struct {
	// Provider is the canonical provider name (e.g., "anthropic").
	Provider string

	// IsLimited is true when the provider is currently rate-limited.
	IsLimited bool

	// ResetAt is the wall-clock time at which the rate limit is expected to
	// reset. It is computed as time.Now().Add(computeWaitDuration(info)) at
	// the moment RecordRateLimit is called.
	ResetAt time.Time

	// ResetAfter is the original duration reported by the agent. A zero value
	// means the agent did not report a specific reset time.
	ResetAfter time.Duration

	// WaitCount is the total number of times the coordinator has recorded a
	// rate limit for this provider. It is not reset by ClearRateLimit so that
	// ExceededMaxWaits continues to work correctly after a clear.
	WaitCount int

	// LastMessage is the last rate-limit message received from the agent.
	// It is useful for display in the TUI status panel.
	LastMessage string

	// UpdatedAt is the time at which this state was last modified.
	UpdatedAt time.Time
}

// RemainingWait returns the time remaining until the rate limit resets.
// Returns 0 if the rate limit has already reset or the state is not limited.
func (ps *ProviderState) RemainingWait() time.Duration {
	if !ps.IsLimited {
		return 0
	}
	remaining := time.Until(ps.ResetAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// RateLimitCoordinator manages rate-limit state across all providers.
// It is safe for concurrent use by multiple goroutines.
type RateLimitCoordinator struct {
	mu       sync.RWMutex
	states   map[string]*ProviderState // provider name -> state
	config   BackoffConfig
	onUpdate func(ProviderState) // optional callback for TUI notifications
}

// NewRateLimitCoordinator creates a coordinator with the given backoff config.
func NewRateLimitCoordinator(config BackoffConfig) *RateLimitCoordinator {
	return &RateLimitCoordinator{
		states: make(map[string]*ProviderState),
		config: config,
	}
}

// SetUpdateCallback sets a function called whenever rate-limit state changes.
// The callback is called outside the coordinator's lock to avoid deadlocks.
// The TUI uses this to receive real-time rate-limit notifications. The
// callback must not block; use a buffered channel internally if needed.
func (rlc *RateLimitCoordinator) SetUpdateCallback(fn func(ProviderState)) {
	rlc.mu.Lock()
	rlc.onUpdate = fn
	rlc.mu.Unlock()
}

// RecordRateLimit records that an agent hit a rate limit.
// It updates the provider's state and increments the wait count.
// Returns the ProviderState after recording.
// The update callback is invoked after the lock is released.
func (rlc *RateLimitCoordinator) RecordRateLimit(agentName string, info *RateLimitInfo) *ProviderState {
	provider := providerForAgent(agentName)
	waitDuration := rlc.computeWaitDuration(info)
	now := time.Now()

	rlc.mu.Lock()
	ps, ok := rlc.states[provider]
	if !ok {
		ps = &ProviderState{Provider: provider}
		rlc.states[provider] = ps
	}

	ps.IsLimited = true
	ps.WaitCount++
	ps.UpdatedAt = now

	// Use the later of the current ResetAt and the new reset time so that
	// concurrent records from multiple agents always extend the window.
	newResetAt := now.Add(waitDuration)
	if newResetAt.After(ps.ResetAt) {
		ps.ResetAt = newResetAt
	}

	if info != nil {
		ps.ResetAfter = info.ResetAfter
		if info.Message != "" {
			ps.LastMessage = info.Message
		}
	}

	// Take a snapshot and capture the callback before releasing the lock.
	snapshot := *ps
	cb := rlc.onUpdate
	rlc.mu.Unlock()

	// Call the update callback outside the lock to avoid deadlocks.
	if cb != nil {
		cb(snapshot)
	}

	return &snapshot
}

// ClearRateLimit clears the rate-limit state for the agent's provider.
// Called after a successful agent run following a rate-limit wait.
// WaitCount is preserved so ExceededMaxWaits continues to work correctly.
func (rlc *RateLimitCoordinator) ClearRateLimit(agentName string) {
	provider := providerForAgent(agentName)
	now := time.Now()

	rlc.mu.Lock()
	ps, ok := rlc.states[provider]
	if !ok {
		rlc.mu.Unlock()
		return
	}
	ps.IsLimited = false
	ps.UpdatedAt = now

	snapshot := *ps
	cb := rlc.onUpdate
	rlc.mu.Unlock()

	if cb != nil {
		cb(snapshot)
	}
}

// ShouldWait checks if an agent should wait before making a request.
// Returns a copy of the ProviderState if waiting is needed, nil if clear to proceed.
// A read lock is used so this is safe to call concurrently with other reads.
func (rlc *RateLimitCoordinator) ShouldWait(agentName string) *ProviderState {
	provider := providerForAgent(agentName)

	rlc.mu.RLock()
	ps, ok := rlc.states[provider]
	if !ok {
		rlc.mu.RUnlock()
		return nil
	}
	if !ps.IsLimited || !ps.ResetAt.After(time.Now()) {
		rlc.mu.RUnlock()
		return nil
	}
	snapshot := *ps
	rlc.mu.RUnlock()

	return &snapshot
}

// WaitForReset blocks until the rate limit for the agent's provider resets
// or the context is cancelled. Returns nil when the wait completes normally,
// context.Err() when the context is cancelled, or an error wrapping
// ErrMaxWaitsExceeded when the provider has exceeded the configured max waits.
func (rlc *RateLimitCoordinator) WaitForReset(ctx context.Context, agentName string) error {
	// Return immediately if already clear.
	state := rlc.ShouldWait(agentName)
	if state == nil {
		return nil
	}

	provider := providerForAgent(agentName)

	// Refuse to wait if max waits have been exceeded.
	if rlc.ExceededMaxWaits(agentName) {
		maxWaits := rlc.config.MaxWaits
		return fmt.Errorf("rate limit: max waits (%d) exceeded for provider %s: %w",
			maxWaits, provider, ErrMaxWaitsExceeded)
	}

	remaining := state.RemainingWait()
	if remaining <= 0 {
		return nil
	}

	timer := time.NewTimer(remaining)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ExceededMaxWaits returns true if the provider has hit the max wait limit.
// If MaxWaits is 0, always returns true (no waits allowed).
func (rlc *RateLimitCoordinator) ExceededMaxWaits(agentName string) bool {
	if rlc.config.MaxWaits == 0 {
		return true
	}

	provider := providerForAgent(agentName)

	rlc.mu.RLock()
	ps, ok := rlc.states[provider]
	if !ok {
		rlc.mu.RUnlock()
		return false
	}
	waitCount := ps.WaitCount
	rlc.mu.RUnlock()

	return waitCount >= rlc.config.MaxWaits
}

// GetState returns a copy of the current rate-limit state for an agent's
// provider. Returns nil if no state exists for the provider.
func (rlc *RateLimitCoordinator) GetState(agentName string) *ProviderState {
	provider := providerForAgent(agentName)

	rlc.mu.RLock()
	ps, ok := rlc.states[provider]
	if !ok {
		rlc.mu.RUnlock()
		return nil
	}
	snapshot := *ps
	rlc.mu.RUnlock()

	return &snapshot
}

// AllStates returns a snapshot of all provider states, sorted by provider name
// for deterministic ordering. Used by the TUI for the rate-limit status panel.
func (rlc *RateLimitCoordinator) AllStates() []ProviderState {
	rlc.mu.RLock()
	providers := make([]string, 0, len(rlc.states))
	for p := range rlc.states {
		providers = append(providers, p)
	}
	sort.Strings(providers)

	result := make([]ProviderState, 0, len(providers))
	for _, p := range providers {
		result = append(result, *rlc.states[p])
	}
	rlc.mu.RUnlock()

	return result
}

// computeWaitDuration determines how long to wait based on rate-limit info
// and the backoff configuration. If info is nil or has a non-positive
// ResetAfter, config.DefaultWait is used. Jitter is added to avoid
// thundering-herd effects when multiple agents resume simultaneously.
func (rlc *RateLimitCoordinator) computeWaitDuration(info *RateLimitInfo) time.Duration {
	var base time.Duration
	if info != nil && info.ResetAfter > 0 {
		base = info.ResetAfter
	} else {
		base = rlc.config.DefaultWait
	}

	// Apply jitter: base + rand[0, jitterFactor * base).
	if rlc.config.JitterFactor > 0 {
		jitter := time.Duration(rand.Float64() * rlc.config.JitterFactor * float64(base))
		base += jitter
	}

	return base
}

// providerForAgent looks up the API provider for an agent name.
// Falls back to the agent name itself for unknown agents (conservative grouping).
func providerForAgent(agentName string) string {
	if p, ok := AgentProvider[agentName]; ok {
		return p
	}
	return agentName
}
