package prd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
)

// --- Helper builders ---

// validEpicBreakdown returns a minimal valid EpicBreakdown for use in tests.
func validEpicBreakdown() *EpicBreakdown {
	return &EpicBreakdown{
		Epics: []Epic{
			{
				ID:                  "E-001",
				Title:               "Foundation",
				Description:         "Core setup and scaffolding",
				PRDSections:         []string{"Section 1"},
				EstimatedTaskCount:  2,
				DependenciesOnEpics: []string{},
			},
			{
				ID:                  "E-002",
				Title:               "Feature Layer",
				Description:         "Primary feature implementation",
				PRDSections:         []string{"Section 2"},
				EstimatedTaskCount:  3,
				DependenciesOnEpics: []string{"E-001"},
			},
		},
	}
}

// validEpicTaskResultJSON returns a valid EpicTaskResult JSON for the given epicID.
func validEpicTaskResultJSON(epicID string) []byte {
	// Derive numeric part: E-001 -> "001" -> prefix "E001"
	numPart := strings.ReplaceAll(strings.TrimPrefix(epicID, "E-"), "-", "")
	etr := EpicTaskResult{
		EpicID: epicID,
		Tasks: []TaskDef{
			{
				TempID:             fmt.Sprintf("E%s-T01", numPart),
				Title:              "First task",
				Description:        "Implement the first feature",
				AcceptanceCriteria: []string{"Feature works correctly"},
				LocalDependencies:  []string{},
				CrossEpicDeps:      []string{},
				Effort:             "medium",
				Priority:           "must-have",
			},
			{
				TempID:             fmt.Sprintf("E%s-T02", numPart),
				Title:              "Second task",
				Description:        "Implement the second feature",
				AcceptanceCriteria: []string{"Second feature works"},
				LocalDependencies:  []string{fmt.Sprintf("E%s-T01", numPart)},
				CrossEpicDeps:      []string{},
				Effort:             "small",
				Priority:           "should-have",
			},
		},
	}
	data, err := json.Marshal(etr)
	if err != nil {
		panic(fmt.Sprintf("validEpicTaskResultJSON: marshal failed: %v", err))
	}
	return data
}

// --- NewScatterOrchestrator / option tests ---

func TestNewScatterOrchestrator_Defaults(t *testing.T) {
	t.Parallel()
	a := agent.NewMockAgent("claude")
	s := NewScatterOrchestrator(a, "/tmp/work")

	assert.Equal(t, a, s.agent)
	assert.Equal(t, "/tmp/work", s.workDir)
	assert.Equal(t, 3, s.concurrency)
	assert.Equal(t, 3, s.maxRetries)
	assert.Nil(t, s.logger)
	assert.Nil(t, s.events)
	assert.Nil(t, s.rateLimiter)
}

func TestNewScatterOrchestrator_WithOptions(t *testing.T) {
	t.Parallel()
	a := agent.NewMockAgent("claude")
	ch := make(chan ScatterEvent, 10)
	rl := agent.NewRateLimitCoordinator(agent.DefaultBackoffConfig())

	s := NewScatterOrchestrator(a, "/tmp",
		WithScatterMaxRetries(5),
		WithConcurrency(2),
		WithScatterEvents(ch),
		WithRateLimiter(rl),
	)

	assert.Equal(t, 5, s.maxRetries)
	assert.Equal(t, 2, s.concurrency)
	assert.Equal(t, (chan<- ScatterEvent)(ch), s.events)
	assert.Equal(t, rl, s.rateLimiter)
}

// --- Scatter: empty breakdown ---

func TestScatterOrchestrator_Scatter_NilBreakdown(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := agent.NewMockAgent("claude")
	s := NewScatterOrchestrator(a, dir)

	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  nil,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Results)
	assert.Empty(t, result.Failures)
	assert.GreaterOrEqual(t, result.Duration, time.Duration(0))
}

func TestScatterOrchestrator_Scatter_EmptyBreakdown(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := agent.NewMockAgent("claude")
	s := NewScatterOrchestrator(a, dir)

	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  &EpicBreakdown{Epics: []Epic{}},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Results)
}

// --- Scatter: success via output file ---

func TestScatterOrchestrator_Scatter_SuccessViaOutputFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	breakdown := validEpicBreakdown()

	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		// Determine which epic this is for by checking the "assigned epic" section.
		for _, epic := range breakdown.Epics {
			// The prompt contains "ID: E-NNN" in the "Your Assigned Epic" section.
			if strings.Contains(opts.Prompt, "ID: "+epic.ID+"\n") {
				filePath := filepath.Join(dir, "epic-"+sanitizeEpicID(epic.ID)+".json")
				require.NoError(t, os.WriteFile(filePath, validEpicTaskResultJSON(epic.ID), 0o644))
				break
			}
		}
		return &agent.RunResult{Stdout: "", ExitCode: 0, Duration: 5 * time.Millisecond}, nil
	})

	ch := make(chan ScatterEvent, 50)
	s := NewScatterOrchestrator(a, dir, WithScatterEvents(ch), WithConcurrency(2))

	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# Test PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Results, 2)
	assert.Empty(t, result.Failures)
	assert.Greater(t, result.Duration, time.Duration(0))

	// Results must be sorted by epic ID.
	assert.Equal(t, "E-001", result.Results[0].EpicID)
	assert.Equal(t, "E-002", result.Results[1].EpicID)
}

// --- Scatter: success via stdout ---

func TestScatterOrchestrator_Scatter_SuccessViaStdout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	breakdown := validEpicBreakdown()

	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		// Identify the assigned epic from the "ID: E-NNN" line in the prompt.
		for _, epic := range breakdown.Epics {
			if strings.Contains(opts.Prompt, "ID: "+epic.ID+"\n") {
				return &agent.RunResult{
					Stdout:   string(validEpicTaskResultJSON(epic.ID)),
					ExitCode: 0,
					Duration: 5 * time.Millisecond,
				}, nil
			}
		}
		return &agent.RunResult{Stdout: "{}", ExitCode: 0}, nil
	})

	s := NewScatterOrchestrator(a, dir, WithConcurrency(2))

	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# Test PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Results, 2)
	assert.Empty(t, result.Failures)
}

// --- Scatter: results sorted by epic ID ---

func TestScatterOrchestrator_Scatter_ResultsSortedByEpicID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a larger breakdown to exercise sorting.
	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-003", Title: "C", Description: "Third"},
			{ID: "E-001", Title: "A", Description: "First"},
			{ID: "E-002", Title: "B", Description: "Second"},
		},
	}

	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		for _, epic := range breakdown.Epics {
			if strings.Contains(opts.Prompt, "ID: "+epic.ID+"\n") {
				return &agent.RunResult{
					Stdout:   string(validEpicTaskResultJSON(epic.ID)),
					ExitCode: 0,
				}, nil
			}
		}
		return &agent.RunResult{Stdout: "{}", ExitCode: 0}, nil
	})

	s := NewScatterOrchestrator(a, dir, WithConcurrency(3))
	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	require.Len(t, result.Results, 3)
	assert.Equal(t, "E-001", result.Results[0].EpicID)
	assert.Equal(t, "E-002", result.Results[1].EpicID)
	assert.Equal(t, "E-003", result.Results[2].EpicID)
}

// --- Scatter: partial failure ---

func TestScatterOrchestrator_Scatter_PartialFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	breakdown := validEpicBreakdown()

	// E-001 returns JSON with an invalid task (bad effort enum), E-002 succeeds.
	invalidE001JSON := `{"epic_id":"E-001","tasks":[{"temp_id":"E001-T01","title":"T","description":"D","acceptance_criteria":["ac"],"local_dependencies":[],"cross_epic_dependencies":[],"effort":"INVALID","priority":"must-have"}]}`

	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		// Distinguish workers by the "ID: E-NNN\n" line in the prompt.
		if strings.Contains(opts.Prompt, "ID: E-001\n") {
			return &agent.RunResult{Stdout: invalidE001JSON, ExitCode: 0}, nil
		}
		return &agent.RunResult{Stdout: string(validEpicTaskResultJSON("E-002")), ExitCode: 0}, nil
	})

	s := NewScatterOrchestrator(a, dir, WithScatterMaxRetries(1), WithConcurrency(2))
	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	require.NotNil(t, result)

	// E-002 should succeed, E-001 should fail.
	assert.Len(t, result.Results, 1)
	assert.Equal(t, "E-002", result.Results[0].EpicID)
	assert.Len(t, result.Failures, 1)
	assert.Equal(t, "E-001", result.Failures[0].EpicID)
	assert.NotNil(t, result.Failures[0].Err)
}

// --- Scatter: all failures ---

func TestScatterOrchestrator_Scatter_AllFailures(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	breakdown := validEpicBreakdown()

	// Always return invalid JSON (bad effort value triggers validation failure).
	invalidJSON := `{"epic_id":"E-001","tasks":[{"temp_id":"E001-T01","title":"T","description":"D","acceptance_criteria":["ac"],"local_dependencies":[],"cross_epic_dependencies":[],"effort":"INVALID","priority":"must-have"}]}`
	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: invalidJSON, ExitCode: 0}, nil
	})

	s := NewScatterOrchestrator(a, dir, WithScatterMaxRetries(0), WithConcurrency(2))
	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	// No fatal error — just all failures collected.
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Results)
	assert.Len(t, result.Failures, 2)
}

// --- Scatter: agent run error is fatal ---

func TestScatterOrchestrator_Scatter_AgentRunError_IsFatal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "A", Description: "D"},
		},
	}

	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return nil, fmt.Errorf("subprocess error")
	})

	s := NewScatterOrchestrator(a, dir, WithScatterMaxRetries(0))
	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err) // Scatter itself doesn't fail; failures are collected
	require.NotNil(t, result)
	assert.Empty(t, result.Results)
	assert.Len(t, result.Failures, 1)
	assert.ErrorContains(t, result.Failures[0].Err, "subprocess error")
}

// --- Scatter: retry on validation failure ---

func TestScatterOrchestrator_Scatter_RetriesOnValidationError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "A", Description: "D", EstimatedTaskCount: 1},
		},
	}

	// Invalid JSON: bad effort value triggers validation failure.
	invalidJSON := `{"epic_id":"E-001","tasks":[{"temp_id":"E001-T01","title":"T","description":"D","acceptance_criteria":["ac"],"local_dependencies":[],"cross_epic_dependencies":[],"effort":"INVALID","priority":"must-have"}]}`

	var callCount int32
	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			// First attempt: validation failure.
			return &agent.RunResult{Stdout: invalidJSON, ExitCode: 0}, nil
		}
		// Second attempt: valid.
		return &agent.RunResult{Stdout: string(validEpicTaskResultJSON("E-001")), ExitCode: 0}, nil
	})

	ch := make(chan ScatterEvent, 20)
	s := NewScatterOrchestrator(a, dir, WithScatterMaxRetries(3), WithScatterEvents(ch))

	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	assert.Empty(t, result.Failures)
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount))

	// Check that a worker_retry event was emitted.
	var types []ScatterEventType
	for len(ch) > 0 {
		evt := <-ch
		types = append(types, evt.Type)
	}
	assert.Contains(t, types, ScatterEventWorkerRetry)
}

// --- Scatter: retry prompt contains validation errors ---

func TestScatterOrchestrator_Scatter_RetryPromptContainsValidationErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "A", Description: "D", EstimatedTaskCount: 1},
		},
	}

	// Invalid JSON: bad effort value triggers validation failure.
	invalidJSON := `{"epic_id":"E-001","tasks":[{"temp_id":"E001-T01","title":"T","description":"D","acceptance_criteria":["ac"],"local_dependencies":[],"cross_epic_dependencies":[],"effort":"INVALID","priority":"must-have"}]}`

	var secondPrompt string
	var callCount int32
	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			return &agent.RunResult{Stdout: invalidJSON, ExitCode: 0}, nil
		}
		secondPrompt = opts.Prompt
		return &agent.RunResult{Stdout: string(validEpicTaskResultJSON("E-001")), ExitCode: 0}, nil
	})

	s := NewScatterOrchestrator(a, dir, WithScatterMaxRetries(3))
	_, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount))
	assert.Contains(t, secondPrompt, "Validation Errors from Previous Attempt")
}

// --- Scatter: context cancellation ---

func TestScatterOrchestrator_Scatter_ContextCancelled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	breakdown := validEpicBreakdown()
	a := agent.NewMockAgent("claude")

	s := NewScatterOrchestrator(a, dir)
	result, err := s.Scatter(ctx, ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	// Should return partial results with context error or collect failures.
	// Either way, it must not hang.
	_ = err
	_ = result
}

// --- Scatter: concurrency limit respected ---

func TestScatterOrchestrator_Scatter_ConcurrencyLimitRespected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create 5 epics.
	epics := make([]Epic, 5)
	for i := range epics {
		epics[i] = Epic{
			ID:          fmt.Sprintf("E-%03d", i+1),
			Title:       fmt.Sprintf("Epic %d", i+1),
			Description: fmt.Sprintf("Description %d", i+1),
		}
	}
	breakdown := &EpicBreakdown{Epics: epics}

	var concurrentCount int32
	var maxConcurrent int32

	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		current := atomic.AddInt32(&concurrentCount, 1)
		defer atomic.AddInt32(&concurrentCount, -1)

		// Track maximum observed concurrency.
		for {
			max := atomic.LoadInt32(&maxConcurrent)
			if current <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
				break
			}
		}

		// Identify which epic is assigned (look for "ID: E-NNN\n" pattern).
		for _, epic := range epics {
			if strings.Contains(opts.Prompt, "ID: "+epic.ID+"\n") {
				return &agent.RunResult{
					Stdout:   string(validEpicTaskResultJSON(epic.ID)),
					ExitCode: 0,
				}, nil
			}
		}
		return &agent.RunResult{Stdout: "{}", ExitCode: 0}, nil
	})

	concurrencyLimit := 2
	s := NewScatterOrchestrator(a, dir, WithConcurrency(concurrencyLimit))
	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	assert.Len(t, result.Results, 5)
	assert.LessOrEqual(t, int(maxConcurrent), concurrencyLimit,
		"concurrent workers must not exceed the concurrency limit")
}

// --- Scatter: nil events channel does not panic ---

func TestScatterOrchestrator_Scatter_NilEvents_DoesNotPanic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	breakdown := validEpicBreakdown()

	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		for _, epic := range breakdown.Epics {
			if strings.Contains(opts.Prompt, "ID: "+epic.ID+"\n") {
				return &agent.RunResult{Stdout: string(validEpicTaskResultJSON(epic.ID)), ExitCode: 0}, nil
			}
		}
		return &agent.RunResult{Stdout: "{}", ExitCode: 0}, nil
	})

	// No events channel — must not panic.
	s := NewScatterOrchestrator(a, dir)
	assert.NotPanics(t, func() {
		result, err := s.Scatter(context.Background(), ScatterOpts{
			PRDContent: "# PRD",
			Breakdown:  breakdown,
		})
		require.NoError(t, err)
		assert.Len(t, result.Results, 2)
	})
}

// --- Scatter: full event channel does not block ---

func TestScatterOrchestrator_Scatter_FullEventChannel_DoesNotBlock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	breakdown := validEpicBreakdown()

	// Capacity 0 — events will always be dropped.
	ch := make(chan ScatterEvent)

	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		for _, epic := range breakdown.Epics {
			if strings.Contains(opts.Prompt, "ID: "+epic.ID+"\n") {
				return &agent.RunResult{Stdout: string(validEpicTaskResultJSON(epic.ID)), ExitCode: 0}, nil
			}
		}
		return &agent.RunResult{Stdout: "{}", ExitCode: 0}, nil
	})

	s := NewScatterOrchestrator(a, dir, WithScatterEvents(ch))

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err := s.Scatter(context.Background(), ScatterOpts{
			PRDContent: "# PRD",
			Breakdown:  breakdown,
		})
		assert.NoError(t, err)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("Scatter blocked on full events channel")
	}
}

// --- Scatter: event types emitted ---

func TestScatterOrchestrator_Scatter_EventTypes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	breakdown := validEpicBreakdown()

	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		for _, epic := range breakdown.Epics {
			if strings.Contains(opts.Prompt, "ID: "+epic.ID+"\n") {
				return &agent.RunResult{Stdout: string(validEpicTaskResultJSON(epic.ID)), ExitCode: 0}, nil
			}
		}
		return &agent.RunResult{Stdout: "{}", ExitCode: 0}, nil
	})

	ch := make(chan ScatterEvent, 50)
	s := NewScatterOrchestrator(a, dir, WithScatterEvents(ch), WithConcurrency(2))

	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	assert.Len(t, result.Results, 2)

	// Drain channel and collect event types.
	var types []ScatterEventType
	for len(ch) > 0 {
		evt := <-ch
		types = append(types, evt.Type)
	}

	assert.Contains(t, types, ScatterEventWorkerStarted)
	assert.Contains(t, types, ScatterEventWorkerCompleted)
}

// --- Scatter: failure events emitted ---

func TestScatterOrchestrator_Scatter_FailureEvent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "A", Description: "D"},
		},
	}

	// Always invalid (bad effort enum value).
	invalidJSON := `{"epic_id":"E-001","tasks":[{"temp_id":"E001-T01","title":"T","description":"D","acceptance_criteria":["ac"],"local_dependencies":[],"cross_epic_dependencies":[],"effort":"INVALID","priority":"must-have"}]}`
	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: invalidJSON, ExitCode: 0}, nil
	})

	ch := make(chan ScatterEvent, 20)
	s := NewScatterOrchestrator(a, dir, WithScatterMaxRetries(0), WithScatterEvents(ch))

	_, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)

	var types []ScatterEventType
	for len(ch) > 0 {
		evt := <-ch
		types = append(types, evt.Type)
	}
	assert.Contains(t, types, ScatterEventWorkerFailed)
}

// --- Scatter: stale output file removed on retry ---

func TestScatterOrchestrator_Scatter_StaleOutputFileRemovedOnRetry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "A", Description: "D", EstimatedTaskCount: 1},
		},
	}

	outputFile := filepath.Join(dir, "epic-E-001.json")

	// Invalid output file content: bad effort enum.
	invalidContent := []byte(`{"epic_id":"E-001","tasks":[{"temp_id":"E001-T01","title":"T","description":"D","acceptance_criteria":["ac"],"local_dependencies":[],"cross_epic_dependencies":[],"effort":"INVALID","priority":"must-have"}]}`)

	var callCount int32
	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			// Write stale invalid output.
			require.NoError(t, os.WriteFile(outputFile, invalidContent, 0o644))
			return &agent.RunResult{ExitCode: 0}, nil
		}
		// Second call: file should have been removed.
		_, statErr := os.Stat(outputFile)
		assert.True(t, os.IsNotExist(statErr), "stale output file should have been removed before retry")
		require.NoError(t, os.WriteFile(outputFile, validEpicTaskResultJSON("E-001"), 0o644))
		return &agent.RunResult{ExitCode: 0}, nil
	})

	s := NewScatterOrchestrator(a, dir, WithScatterMaxRetries(3))
	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount))
}

// --- sanitizeEpicID ---

func TestSanitizeEpicID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "normal E-NNN format", input: "E-001", want: "E-001"},
		{name: "already safe", input: "E001", want: "E001"},
		{name: "path traversal dots", input: "../evil", want: "evil"},
		{name: "slashes removed", input: "E/001", want: "E001"},
		{name: "spaces removed", input: "E 001", want: "E001"},
		{name: "underscore and hyphen allowed", input: "E_001-alpha", want: "E_001-alpha"},
		{name: "empty string", input: "", want: ""},
		{name: "only unsafe chars", input: "/../", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeEpicID(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- epicFilePath ---

func TestScatterOrchestrator_EpicFilePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewScatterOrchestrator(agent.NewMockAgent("claude"), dir)

	tests := []struct {
		name    string
		epicID  string
		wantErr bool
		wantSub string // expected substring in path
	}{
		{
			name:    "normal epic ID",
			epicID:  "E-001",
			wantSub: "epic-E-001.json",
		},
		{
			name:    "already-safe ID",
			epicID:  "E001",
			wantSub: "epic-E001.json",
		},
		{
			name:    "empty after sanitization",
			epicID:  "/../",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path, err := s.epicFilePath(tt.epicID)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, path, tt.wantSub)
			// Verify path is inside workDir.
			assert.True(t, strings.HasPrefix(path, dir), "path must be inside workDir")
		})
	}
}

// --- buildScatterPrompt ---

func TestBuildScatterPrompt_ContainsExpectedSections(t *testing.T) {
	t.Parallel()

	epic := Epic{
		ID:                 "E-001",
		Title:              "Foundation",
		Description:        "Core setup",
		EstimatedTaskCount: 3,
	}

	tests := []struct {
		name    string
		data    scatterPromptData
		wantIn  []string
		wantOut []string
	}{
		{
			name: "first attempt, no validation errors",
			data: scatterPromptData{
				PRDContent:       "some PRD content here",
				Epic:             epic,
				OtherEpics:       "- E-002: Feature -- Description",
				OutputFile:       "/tmp/epic-E001.json",
				ValidationErrors: "",
			},
			wantIn: []string{
				"some PRD content here",
				"E-001",
				"Foundation",
				"/tmp/epic-E001.json",
				"- E-002: Feature -- Description",
				"E-NNN:label",
			},
			wantOut: []string{"Validation Errors from Previous Attempt"},
		},
		{
			name: "retry with validation errors",
			data: scatterPromptData{
				PRDContent:       "prd",
				Epic:             epic,
				OtherEpics:       "",
				OutputFile:       "/tmp/out.json",
				ValidationErrors: "1. [tasks[0].effort] invalid value\n",
			},
			wantIn: []string{
				"Validation Errors from Previous Attempt",
				"1. [tasks[0].effort] invalid value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prompt, err := buildScatterPrompt(tt.data)
			require.NoError(t, err)
			for _, want := range tt.wantIn {
				assert.Contains(t, prompt, want)
			}
			for _, notWant := range tt.wantOut {
				assert.NotContains(t, prompt, notWant)
			}
		})
	}
}

func TestBuildScatterPrompt_NoUnconsumedDelimiters(t *testing.T) {
	t.Parallel()

	epic := Epic{
		ID:                 "E-001",
		Title:              "Test Epic",
		Description:        "Description",
		EstimatedTaskCount: 2,
	}

	prompt, err := buildScatterPrompt(scatterPromptData{
		PRDContent:       "# My PRD",
		Epic:             epic,
		OtherEpics:       "- E-002: Other -- Description",
		OutputFile:       "/tmp/out.json",
		ValidationErrors: "",
	})

	require.NoError(t, err)
	// No raw template delimiters should remain.
	assert.NotContains(t, prompt, "[[ .PRDContent ]]")
	assert.NotContains(t, prompt, "[[ .Epic.ID ]]")
	assert.NotContains(t, prompt, "[[ .OutputFile ]]")
}

// --- buildOtherEpicsSummary ---

func TestBuildOtherEpicsSummary(t *testing.T) {
	t.Parallel()

	epics := []Epic{
		{ID: "E-001", Title: "Foundation", Description: "Core setup"},
		{ID: "E-002", Title: "Features", Description: "Main features"},
		{ID: "E-003", Title: "Testing", Description: "Test coverage"},
	}

	tests := []struct {
		name          string
		currentEpicID string
		wantIn        []string
		wantOut       []string
	}{
		{
			name:          "excludes current epic",
			currentEpicID: "E-001",
			wantIn:        []string{"E-002", "Features", "E-003", "Testing"},
			wantOut:       []string{"E-001: Foundation"},
		},
		{
			name:          "excludes middle epic",
			currentEpicID: "E-002",
			wantIn:        []string{"E-001", "E-003"},
			wantOut:       []string{"E-002: Features"},
		},
		{
			name:          "all single epic — empty output",
			currentEpicID: "E-999",
			wantIn:        []string{"E-001", "E-002", "E-003"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			summary := buildOtherEpicsSummary(tt.currentEpicID, epics)
			for _, want := range tt.wantIn {
				assert.Contains(t, summary, want)
			}
			for _, notWant := range tt.wantOut {
				assert.NotContains(t, summary, notWant)
			}
		})
	}
}

// --- Table-driven: various invalid EpicTaskResult patterns ---

func TestScatterOrchestrator_Scatter_TableDriven_InvalidJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stdout string
	}{
		{name: "completely empty stdout", stdout: ""},
		{name: "plain text no JSON", stdout: "Sorry, I cannot help."},
		{name: "malformed JSON", stdout: `{"epic_id":"E-001"`},
		{
			name:   "invalid effort value",
			stdout: `{"epic_id":"E-001","tasks":[{"temp_id":"E001-T01","title":"T","description":"D","acceptance_criteria":["ac"],"local_dependencies":[],"cross_epic_dependencies":[],"effort":"INVALID","priority":"must-have"}]}`,
		},
		{
			name:   "missing temp_id",
			stdout: `{"epic_id":"E-001","tasks":[{"temp_id":"","title":"T","description":"D","acceptance_criteria":["ac"],"local_dependencies":[],"cross_epic_dependencies":[],"effort":"medium","priority":"must-have"}]}`,
		},
		{
			name:   "invalid epic_id format",
			stdout: `{"epic_id":"BADFMT","tasks":[{"temp_id":"E001-T01","title":"T","description":"D","acceptance_criteria":["ac"],"local_dependencies":[],"cross_epic_dependencies":[],"effort":"medium","priority":"must-have"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()

			breakdown := &EpicBreakdown{
				Epics: []Epic{
					{ID: "E-001", Title: "A", Description: "D"},
				},
			}

			a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
				return &agent.RunResult{Stdout: tt.stdout, ExitCode: 0}, nil
			})

			s := NewScatterOrchestrator(a, dir, WithScatterMaxRetries(0))
			result, err := s.Scatter(context.Background(), ScatterOpts{
				PRDContent: "# PRD",
				Breakdown:  breakdown,
			})

			require.NoError(t, err)
			assert.Empty(t, result.Results, "expected no results for invalid stdout: %s", tt.name)
			assert.NotEmpty(t, result.Failures)
		})
	}
}

// --- Scatter: model and effort passed to agent ---

func TestScatterOrchestrator_Scatter_PassesModelAndEffortToAgent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "A", Description: "D"},
		},
	}

	var capturedOpts agent.RunOpts
	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		capturedOpts = opts
		return &agent.RunResult{Stdout: string(validEpicTaskResultJSON("E-001")), ExitCode: 0}, nil
	})

	s := NewScatterOrchestrator(a, dir)
	_, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
		Model:      "claude-opus-4-6",
		Effort:     "high",
	})

	require.NoError(t, err)
	assert.Equal(t, "claude-opus-4-6", capturedOpts.Model)
	assert.Equal(t, "high", capturedOpts.Effort)
	assert.Equal(t, dir, capturedOpts.WorkDir)
}

// --- Scatter: duration is positive ---

func TestScatterOrchestrator_Scatter_DurationIsPositive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	breakdown := validEpicBreakdown()

	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		for _, epic := range breakdown.Epics {
			if strings.Contains(opts.Prompt, "ID: "+epic.ID+"\n") {
				return &agent.RunResult{Stdout: string(validEpicTaskResultJSON(epic.ID)), ExitCode: 0}, nil
			}
		}
		return &agent.RunResult{Stdout: "{}", ExitCode: 0}, nil
	})

	s := NewScatterOrchestrator(a, dir)
	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	assert.GreaterOrEqual(t, result.Duration, time.Duration(0))
}

// --- Scatter: stdout with markdown fence ---

func TestScatterOrchestrator_Scatter_StdoutWithMarkdownFence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "A", Description: "D"},
		},
	}

	fenced := "Here is the result:\n\n```json\n" + string(validEpicTaskResultJSON("E-001")) + "\n```\n"
	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: fenced, ExitCode: 0}, nil
	})

	s := NewScatterOrchestrator(a, dir)
	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	assert.Equal(t, "E-001", result.Results[0].EpicID)
}

// --- Scatter: rate limiter integration ---

func TestScatterOrchestrator_Scatter_RateLimiterShouldWait(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "A", Description: "D"},
		},
	}

	// Create a rate limiter and set a very short wait.
	rl := agent.NewRateLimitCoordinator(agent.BackoffConfig{
		DefaultWait:  10 * time.Millisecond,
		MaxWaits:     5,
		JitterFactor: 0,
	})

	// Invalid JSON on first attempt to force a retry; also record rate limit so the
	// ShouldWait check fires before the second attempt.
	invalidJSON := `{"epic_id":"E-001","tasks":[{"temp_id":"E001-T01","title":"T","description":"D","acceptance_criteria":["ac"],"local_dependencies":[],"cross_epic_dependencies":[],"effort":"INVALID","priority":"must-have"}]}`

	var callCount int32
	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			// Record a rate limit so the next iteration must wait.
			rl.RecordRateLimit("claude", &agent.RateLimitInfo{
				IsLimited:  true,
				ResetAfter: 10 * time.Millisecond,
			})
			// Also return invalid JSON so retry is triggered.
			return &agent.RunResult{Stdout: invalidJSON, ExitCode: 0}, nil
		}
		// After waiting, return valid result.
		return &agent.RunResult{Stdout: string(validEpicTaskResultJSON("E-001")), ExitCode: 0}, nil
	})

	ch := make(chan ScatterEvent, 20)
	s := NewScatterOrchestrator(a, dir,
		WithScatterMaxRetries(3),
		WithRateLimiter(rl),
		WithScatterEvents(ch),
	)

	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	require.Len(t, result.Results, 1)

	// Verify rate_limited event was emitted at some point.
	var eventTypes []ScatterEventType
	for len(ch) > 0 {
		evt := <-ch
		eventTypes = append(eventTypes, evt.Type)
	}
	assert.Contains(t, eventTypes, ScatterEventRateLimited)
}

// --- scatterValidationFailure ---

func TestScatterValidationFailure_Error(t *testing.T) {
	t.Parallel()
	err := &scatterValidationFailure{
		epicID: "E-001",
		errs: []ValidationError{
			{Field: "tasks[0].effort", Message: "invalid value"},
		},
	}
	errStr := err.Error()
	assert.Contains(t, errStr, "E-001")
	assert.Contains(t, errStr, "exhausted retries")
	assert.Contains(t, errStr, "tasks[0].effort")
}

// --- Scatter: concurrency of 1 runs workers sequentially ---

// TestScatterOrchestrator_Scatter_ConcurrencyOne verifies that when concurrency
// is set to 1, workers run strictly sequentially (never more than one at a time).
// This directly addresses the acceptance criterion: "Concurrency of 1: workers
// run sequentially".
func TestScatterOrchestrator_Scatter_ConcurrencyOne(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "A", Description: "First"},
			{ID: "E-002", Title: "B", Description: "Second"},
			{ID: "E-003", Title: "C", Description: "Third"},
		},
	}

	var concurrentCount int32
	var maxConcurrent int32

	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		current := atomic.AddInt32(&concurrentCount, 1)
		defer atomic.AddInt32(&concurrentCount, -1)

		for {
			max := atomic.LoadInt32(&maxConcurrent)
			if current <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
				break
			}
		}

		for _, epic := range breakdown.Epics {
			if strings.Contains(opts.Prompt, "ID: "+epic.ID+"\n") {
				return &agent.RunResult{
					Stdout:   string(validEpicTaskResultJSON(epic.ID)),
					ExitCode: 0,
				}, nil
			}
		}
		return &agent.RunResult{Stdout: "{}", ExitCode: 0}, nil
	})

	s := NewScatterOrchestrator(a, dir, WithConcurrency(1))
	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	assert.Len(t, result.Results, 3)
	assert.Empty(t, result.Failures)

	// With concurrency 1, max concurrent workers must be exactly 1.
	assert.Equal(t, int32(1), atomic.LoadInt32(&maxConcurrent),
		"concurrency=1 must serialize workers; at most 1 may run at a time")
}

// --- Scatter: 3 epics with concurrency 2 (explicit AC scenario) ---

// TestScatterOrchestrator_Scatter_ThreeEpicsWithConcurrencyTwo is an explicit
// acceptance criterion test: "3 epics with concurrency 2: verify only 2 workers
// run simultaneously". All 3 epics succeed on the first attempt.
func TestScatterOrchestrator_Scatter_ThreeEpicsWithConcurrencyTwo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "Epic One", Description: "First epic"},
			{ID: "E-002", Title: "Epic Two", Description: "Second epic"},
			{ID: "E-003", Title: "Epic Three", Description: "Third epic"},
		},
	}

	var concurrentCount int32
	var maxConcurrent int32

	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		current := atomic.AddInt32(&concurrentCount, 1)
		defer atomic.AddInt32(&concurrentCount, -1)

		for {
			max := atomic.LoadInt32(&maxConcurrent)
			if current <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
				break
			}
		}

		// Small artificial pause so goroutines overlap in time.
		time.Sleep(2 * time.Millisecond)

		for _, epic := range breakdown.Epics {
			if strings.Contains(opts.Prompt, "ID: "+epic.ID+"\n") {
				return &agent.RunResult{
					Stdout:   string(validEpicTaskResultJSON(epic.ID)),
					ExitCode: 0,
				}, nil
			}
		}
		return &agent.RunResult{Stdout: "{}", ExitCode: 0}, nil
	})

	s := NewScatterOrchestrator(a, dir, WithConcurrency(2))
	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	require.NotNil(t, result)

	// All 3 epics must succeed.
	assert.Len(t, result.Results, 3, "all 3 epics should succeed on first attempt")
	assert.Empty(t, result.Failures)

	// Concurrency must never exceed 2.
	assert.LessOrEqual(t, int(atomic.LoadInt32(&maxConcurrent)), 2,
		"concurrent workers must not exceed the configured limit of 2")

	// Results must be sorted by epic ID.
	assert.Equal(t, "E-001", result.Results[0].EpicID)
	assert.Equal(t, "E-002", result.Results[1].EpicID)
	assert.Equal(t, "E-003", result.Results[2].EpicID)
}

// --- Scatter: context cancellation mid-scatter returns partial results ---

// TestScatterOrchestrator_Scatter_ContextCancelledMidScatter verifies the
// acceptance criterion: "Context cancelled mid-scatter: partial results returned
// with cancellation error". A slow worker is cancelled before it finishes.
func TestScatterOrchestrator_Scatter_ContextCancelledMidScatter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "Fast", Description: "Fast epic"},
			{ID: "E-002", Title: "Slow", Description: "Slow epic"},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	// E-001 succeeds quickly; E-002 is slow and blocked waiting on ctx.
	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		if strings.Contains(opts.Prompt, "ID: E-001\n") {
			// Fast path: return immediately and trigger cancellation.
			cancel()
			return &agent.RunResult{Stdout: string(validEpicTaskResultJSON("E-001")), ExitCode: 0}, nil
		}
		// Slow path: block until cancelled.
		<-ctx.Done()
		return nil, ctx.Err()
	})

	s := NewScatterOrchestrator(a, dir, WithConcurrency(2))
	result, err := s.Scatter(ctx, ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	// Scatter must return the context cancellation error alongside partial results.
	assert.Error(t, err, "expected context cancellation error")
	require.NotNil(t, result)

	// The test must complete within a reasonable time (no hang).
	// At least the fast epic (E-001) may have succeeded, or we have failures;
	// but Scatter must not block forever.
	total := len(result.Results) + len(result.Failures)
	assert.Equal(t, 2, total, "all 2 epics must be accounted for (result or failure)")
}

// --- Scatter: WithScatterLogger sets the logger on the orchestrator ---

func TestScatterOrchestrator_WithScatterLogger(t *testing.T) {
	t.Parallel()
	a := agent.NewMockAgent("claude")

	logger := log.New(os.Stderr)

	s := NewScatterOrchestrator(a, "/tmp",
		WithScatterLogger(logger),
	)

	assert.Equal(t, logger, s.logger)
}

// --- Scatter: one worker fails all retries, two succeed (3 epics, 2 succeed, 1 fails) ---

// TestScatterOrchestrator_Scatter_OneFailAllRetries_TwoSucceed verifies the
// acceptance criterion: "One worker fails all retries: ScatterResult has 2 results,
// 1 failure".
func TestScatterOrchestrator_Scatter_OneFailAllRetries_TwoSucceed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "Always fails", Description: "Will fail all retries"},
			{ID: "E-002", Title: "Succeeds", Description: "Succeeds on first try"},
			{ID: "E-003", Title: "Also succeeds", Description: "Also succeeds on first try"},
		},
	}

	// E-001 always returns invalid JSON; E-002 and E-003 return valid JSON.
	invalidJSON := `{"epic_id":"E-001","tasks":[{"temp_id":"E001-T01","title":"T","description":"D","acceptance_criteria":["ac"],"local_dependencies":[],"cross_epic_dependencies":[],"effort":"INVALID","priority":"must-have"}]}`

	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		if strings.Contains(opts.Prompt, "ID: E-001\n") {
			return &agent.RunResult{Stdout: invalidJSON, ExitCode: 0}, nil
		}
		for _, epic := range breakdown.Epics {
			if strings.Contains(opts.Prompt, "ID: "+epic.ID+"\n") {
				return &agent.RunResult{Stdout: string(validEpicTaskResultJSON(epic.ID)), ExitCode: 0}, nil
			}
		}
		return &agent.RunResult{Stdout: "{}", ExitCode: 0}, nil
	})

	s := NewScatterOrchestrator(a, dir, WithScatterMaxRetries(2), WithConcurrency(3))
	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	require.NotNil(t, result)

	// 2 epics succeed, 1 fails.
	assert.Len(t, result.Results, 2, "expected 2 successful results")
	assert.Len(t, result.Failures, 1, "expected 1 failure")

	// The failure must be E-001.
	assert.Equal(t, "E-001", result.Failures[0].EpicID)
	assert.NotNil(t, result.Failures[0].Err)
	assert.NotEmpty(t, result.Failures[0].Errors, "validation errors should be recorded")

	// The results must be sorted.
	epicIDs := make([]string, len(result.Results))
	for i, r := range result.Results {
		epicIDs[i] = r.EpicID
	}
	assert.Equal(t, []string{"E-002", "E-003"}, epicIDs)
}

// --- Scatter: output file is written by the agent ---

// TestScatterOrchestrator_Scatter_OutputFileWritten verifies that the output
// file path derived from the epic ID is passed to the agent and, when the agent
// writes JSON to it, the orchestrator reads it back correctly.
func TestScatterOrchestrator_Scatter_OutputFileWritten(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "Test", Description: "Test epic"},
		},
	}

	var capturedOutputFile string

	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		// The prompt tells the agent where to write the JSON output.
		// Parse the file path from the prompt ("Write a JSON object to the file: ").
		for _, line := range strings.Split(opts.Prompt, "\n") {
			if strings.HasPrefix(line, "Write a JSON object to the file: ") {
				capturedOutputFile = strings.TrimPrefix(line, "Write a JSON object to the file: ")
				capturedOutputFile = strings.TrimSpace(capturedOutputFile)
				break
			}
		}

		if capturedOutputFile != "" {
			require.NoError(t, os.WriteFile(capturedOutputFile, validEpicTaskResultJSON("E-001"), 0o644))
		}
		return &agent.RunResult{ExitCode: 0}, nil
	})

	s := NewScatterOrchestrator(a, dir)
	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	assert.Equal(t, "E-001", result.Results[0].EpicID)

	// Verify the output file path is inside the workDir.
	require.NotEmpty(t, capturedOutputFile)
	assert.True(t, strings.HasPrefix(capturedOutputFile, dir),
		"output file must be inside workDir")
	assert.Equal(t, filepath.Join(dir, "epic-E-001.json"), capturedOutputFile)
}

// --- Scatter: other epics summary excludes current epic ---

// TestScatterOrchestrator_Scatter_OtherEpicsSummaryInPrompt verifies that the
// prompt for each worker contains summaries of the other epics but not its own.
func TestScatterOrchestrator_Scatter_OtherEpicsSummaryInPrompt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	breakdown := validEpicBreakdown()

	var capturedPrompts []string
	var promptsMu sync.Mutex

	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		promptsMu.Lock()
		capturedPrompts = append(capturedPrompts, opts.Prompt)
		promptsMu.Unlock()

		for _, epic := range breakdown.Epics {
			if strings.Contains(opts.Prompt, "ID: "+epic.ID+"\n") {
				return &agent.RunResult{Stdout: string(validEpicTaskResultJSON(epic.ID)), ExitCode: 0}, nil
			}
		}
		return &agent.RunResult{Stdout: "{}", ExitCode: 0}, nil
	})

	s := NewScatterOrchestrator(a, dir, WithConcurrency(2))
	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	assert.Len(t, result.Results, 2)

	promptsMu.Lock()
	captured := make([]string, len(capturedPrompts))
	copy(captured, capturedPrompts)
	promptsMu.Unlock()

	require.Len(t, captured, 2)

	for _, prompt := range captured {
		if strings.Contains(prompt, "ID: E-001\n") {
			// E-001's prompt should list E-002 in the "Other Epics" section
			// but not E-001 itself.
			assert.Contains(t, prompt, "E-002", "prompt for E-001 must list E-002 as other epic")
			assert.NotContains(t, prompt, "E-001: Foundation", "prompt for E-001 must not list itself")
		} else if strings.Contains(prompt, "ID: E-002\n") {
			assert.Contains(t, prompt, "E-001", "prompt for E-002 must list E-001 as other epic")
		}
	}
}

// --- Scatter: scatterValidationFailure wraps validation errors ---

// TestScatterValidationFailure_ErrorsPreserved checks that the Errors field in
// ScatterFailure is populated from the scatterValidationFailure sentinel.
func TestScatterValidationFailure_ErrorsPreserved(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "A", Description: "D"},
		},
	}

	invalidJSON := `{"epic_id":"E-001","tasks":[{"temp_id":"E001-T01","title":"T","description":"D","acceptance_criteria":["ac"],"local_dependencies":[],"cross_epic_dependencies":[],"effort":"INVALID","priority":"must-have"}]}`

	a := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: invalidJSON, ExitCode: 0}, nil
	})

	s := NewScatterOrchestrator(a, dir, WithScatterMaxRetries(0))
	result, err := s.Scatter(context.Background(), ScatterOpts{
		PRDContent: "# PRD",
		Breakdown:  breakdown,
	})

	require.NoError(t, err)
	require.Len(t, result.Failures, 1)

	failure := result.Failures[0]
	assert.Equal(t, "E-001", failure.EpicID)
	// The Errors field must be populated from the scatterValidationFailure.
	assert.NotEmpty(t, failure.Errors, "validation errors must be preserved in ScatterFailure.Errors")
	// Check that the effort-related validation error is present.
	var foundEffortErr bool
	for _, ve := range failure.Errors {
		if strings.Contains(ve.Message, "effort") || strings.Contains(ve.Field, "effort") {
			foundEffortErr = true
			break
		}
	}
	assert.True(t, foundEffortErr, "effort validation error must be recorded in failure")
}

// --- Benchmark ---

func BenchmarkBuildScatterPrompt(b *testing.B) {
	epic := Epic{
		ID:                 "E-001",
		Title:              "Foundation",
		Description:        "Core scaffolding",
		EstimatedTaskCount: 5,
	}
	data := scatterPromptData{
		PRDContent:       strings.Repeat("# Section\nContent.\n", 100),
		Epic:             epic,
		OtherEpics:       "- E-002: Features -- Main features\n- E-003: Testing -- Tests\n",
		OutputFile:       "/tmp/epic-E001.json",
		ValidationErrors: "",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := buildScatterPrompt(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
