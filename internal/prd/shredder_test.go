package prd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
)

// --- Helper builders ---

// validBreakdownJSON returns JSON for a minimal valid EpicBreakdown.
func validBreakdownJSON() []byte {
	eb := EpicBreakdown{
		Epics: []Epic{
			{
				ID:                  "E-001",
				Title:               "Foundation",
				Description:         "Core scaffolding and project setup",
				PRDSections:         []string{"Section 1"},
				EstimatedTaskCount:  5,
				DependenciesOnEpics: []string{},
			},
			{
				ID:                  "E-002",
				Title:               "Feature Layer",
				Description:         "Primary feature implementation",
				PRDSections:         []string{"Section 2"},
				EstimatedTaskCount:  8,
				DependenciesOnEpics: []string{"E-001"},
			},
		},
	}
	data, err := json.Marshal(eb)
	if err != nil {
		panic(fmt.Sprintf("validBreakdownJSON: marshal failed: %v", err))
	}
	return data
}

// writePRDFile writes a minimal PRD markdown file to dir and returns its path.
func writePRDFile(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "PRD.md")
	content := "# Test PRD\n\n## Section 1\nFoundation work.\n\n## Section 2\nFeature development.\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// --- NewShredder / option tests ---

func TestNewShredder_Defaults(t *testing.T) {
	t.Parallel()
	a := agent.NewMockAgent("claude")
	s := NewShredder(a, "/tmp/work")

	assert.Equal(t, a, s.agent)
	assert.Equal(t, "/tmp/work", s.workDir)
	assert.Equal(t, 3, s.maxRetries)
	assert.Nil(t, s.logger)
	assert.Nil(t, s.events)
}

func TestNewShredder_WithMaxRetries(t *testing.T) {
	t.Parallel()
	a := agent.NewMockAgent("claude")
	s := NewShredder(a, "/tmp", WithMaxRetries(5))
	assert.Equal(t, 5, s.maxRetries)
}

func TestNewShredder_WithEvents(t *testing.T) {
	t.Parallel()
	a := agent.NewMockAgent("claude")
	ch := make(chan ShredEvent, 10)
	s := NewShredder(a, "/tmp", WithEvents(ch))
	assert.Equal(t, (chan<- ShredEvent)(ch), s.events)
}

// --- Shred success path ---

func TestShredder_Shred_SuccessViaOutputFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	// Agent writes valid JSON to the output file.
	outputFile := filepath.Join(dir, "epic-breakdown.json")
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		require.NoError(t, os.WriteFile(outputFile, validBreakdownJSON(), 0o644))
		return &agent.RunResult{Stdout: "", ExitCode: 0, Duration: 10 * time.Millisecond}, nil
	})

	ch := make(chan ShredEvent, 10)
	s := NewShredder(mockAgent, dir, WithEvents(ch))

	result, err := s.Shred(context.Background(), ShredOpts{
		PRDPath:    prdPath,
		OutputFile: outputFile,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, len(result.Breakdown.Epics))
	assert.Equal(t, 0, result.Retries)
	assert.Equal(t, outputFile, result.OutputFile)
	assert.Greater(t, result.Duration, time.Duration(0))

	// Verify started + completed events were emitted.
	var types []ShredEventType
	for len(ch) > 0 {
		evt := <-ch
		types = append(types, evt.Type)
	}
	assert.Contains(t, types, ShredEventStarted)
	assert.Contains(t, types, ShredEventCompleted)
}

func TestShredder_Shred_SuccessViaStdout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	// Agent returns JSON in stdout, no output file.
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{
			Stdout:   string(validBreakdownJSON()),
			ExitCode: 0,
			Duration: 10 * time.Millisecond,
		}, nil
	})

	s := NewShredder(mockAgent, dir)
	result, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, len(result.Breakdown.Epics))
	assert.Equal(t, 0, result.Retries)
}

func TestShredder_Shred_DefaultOutputFilePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	defaultOutput := filepath.Join(dir, "epic-breakdown.json")
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		require.NoError(t, os.WriteFile(defaultOutput, validBreakdownJSON(), 0o644))
		return &agent.RunResult{ExitCode: 0, Duration: 5 * time.Millisecond}, nil
	})

	s := NewShredder(mockAgent, dir)
	// ShredOpts.OutputFile is intentionally empty.
	result, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

	require.NoError(t, err)
	assert.Equal(t, defaultOutput, result.OutputFile)
}

// --- Retry path ---

func TestShredder_Shred_RetriesOnValidationError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	callCount := 0
	// First call returns invalid JSON (empty epics), second returns valid.
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		if callCount == 1 {
			// Return broken breakdown: no epics.
			return &agent.RunResult{
				Stdout:   `{"epics":[]}`,
				ExitCode: 0,
				Duration: 5 * time.Millisecond,
			}, nil
		}
		// Second attempt succeeds.
		return &agent.RunResult{
			Stdout:   string(validBreakdownJSON()),
			ExitCode: 0,
			Duration: 5 * time.Millisecond,
		}, nil
	})

	ch := make(chan ShredEvent, 20)
	s := NewShredder(mockAgent, dir, WithMaxRetries(3), WithEvents(ch))
	result, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Retries)
	assert.Equal(t, 2, callCount)

	// A retry event must have been emitted.
	var types []ShredEventType
	for len(ch) > 0 {
		evt := <-ch
		types = append(types, evt.Type)
	}
	assert.Contains(t, types, ShredEventRetry)
}

func TestShredder_Shred_RetryPromptContainsValidationErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	var secondPrompt string
	callCount := 0
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		if callCount == 1 {
			return &agent.RunResult{Stdout: `{"epics":[]}`, ExitCode: 0}, nil
		}
		secondPrompt = opts.Prompt
		return &agent.RunResult{Stdout: string(validBreakdownJSON()), ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir, WithMaxRetries(3))
	_, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

	require.NoError(t, err)
	assert.Equal(t, 2, callCount, "should invoke agent twice")
	assert.Contains(t, secondPrompt, "Validation Errors from Previous Attempt",
		"retry prompt must include validation error section")
	assert.Contains(t, secondPrompt, "must not be empty",
		"retry prompt must include the specific validation error message")
}

func TestShredder_Shred_ExceedsMaxRetries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	// Always return invalid JSON.
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: `{"epics":[]}`, ExitCode: 0}, nil
	})

	ch := make(chan ShredEvent, 20)
	s := NewShredder(mockAgent, dir, WithMaxRetries(2), WithEvents(ch))
	result, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "exceeded")

	// Failed event must be emitted.
	var types []ShredEventType
	for len(ch) > 0 {
		evt := <-ch
		types = append(types, evt.Type)
	}
	assert.Contains(t, types, ShredEventFailed)
}

func TestShredder_Shred_ZeroRetries_FailsOnFirstBadOutput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: `{"epics":[]}`, ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir, WithMaxRetries(0))
	result, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

	require.Error(t, err)
	assert.Nil(t, result)
}

// --- Error paths ---

func TestShredder_Shred_MissingPRDFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mockAgent := agent.NewMockAgent("claude")
	s := NewShredder(mockAgent, dir)

	_, err := s.Shred(context.Background(), ShredOpts{PRDPath: filepath.Join(dir, "nonexistent.md")})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading PRD file")
}

func TestShredder_Shred_PRDFileTooLarge(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Write a file that exceeds the 1 MB limit.
	bigFile := filepath.Join(dir, "big.md")
	require.NoError(t, os.WriteFile(bigFile, make([]byte, maxPRDSize+1), 0o644))

	mockAgent := agent.NewMockAgent("claude")
	s := NewShredder(mockAgent, dir)

	_, err := s.Shred(context.Background(), ShredOpts{PRDPath: bigFile})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 MB")
}

func TestShredder_Shred_AgentRunError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return nil, errors.New("agent subprocess failed")
	})

	s := NewShredder(mockAgent, dir)
	_, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent run")
}

func TestShredder_Shred_NoJSONInOutput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: "Sorry, I cannot help with that.", ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir, WithMaxRetries(0))
	_, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

	require.Error(t, err)
}

func TestShredder_Shred_ContextCancelled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	mockAgent := agent.NewMockAgent("claude")
	s := NewShredder(mockAgent, dir)
	_, err := s.Shred(ctx, ShredOpts{PRDPath: prdPath})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "context")
}

// --- Agent pass-through tests ---

func TestShredder_Shred_PassesModelAndEffortToAgent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	var capturedOpts agent.RunOpts
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		capturedOpts = opts
		return &agent.RunResult{Stdout: string(validBreakdownJSON()), ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir)
	_, err := s.Shred(context.Background(), ShredOpts{
		PRDPath: prdPath,
		Model:   "claude-opus-4-6",
		Effort:  "high",
	})

	require.NoError(t, err)
	assert.Equal(t, "claude-opus-4-6", capturedOpts.Model)
	assert.Equal(t, "high", capturedOpts.Effort)
	assert.Equal(t, dir, capturedOpts.WorkDir)
}

// --- Prompt content tests ---

func TestShredder_Shred_PromptContainsPRDContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := filepath.Join(dir, "prd.md")
	prdContent := "# My Special PRD\n\nThis is unique content: XYZZY-MARKER."
	require.NoError(t, os.WriteFile(prdPath, []byte(prdContent), 0o644))

	var capturedPrompt string
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		capturedPrompt = opts.Prompt
		return &agent.RunResult{Stdout: string(validBreakdownJSON()), ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir)
	_, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

	require.NoError(t, err)
	assert.Contains(t, capturedPrompt, "XYZZY-MARKER", "prompt must embed the PRD content")
}

func TestShredder_Shred_PromptContainsOutputFilePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)
	outputFile := filepath.Join(dir, "my-output.json")

	var capturedPrompt string
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		capturedPrompt = opts.Prompt
		require.NoError(t, os.WriteFile(outputFile, validBreakdownJSON(), 0o644))
		return &agent.RunResult{ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir)
	_, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath, OutputFile: outputFile})

	require.NoError(t, err)
	assert.Contains(t, capturedPrompt, outputFile, "prompt must include the output file path")
}

// --- Event emission tests ---

func TestShredder_Shred_NilEvents_DoesNotPanic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: string(validBreakdownJSON()), ExitCode: 0}, nil
	})

	// No WithEvents option -- events channel is nil.
	s := NewShredder(mockAgent, dir)
	assert.NotPanics(t, func() {
		_, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})
		require.NoError(t, err)
	})
}

func TestShredder_Shred_FullChannelDoesNotBlock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	// Channel with capacity 0 -- events will always be dropped.
	ch := make(chan ShredEvent)
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: string(validBreakdownJSON()), ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir, WithEvents(ch))
	// Must complete without blocking, even though channel capacity is zero.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})
		assert.NoError(t, err)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(3 * time.Second):
		t.Fatal("Shred blocked on full events channel")
	}
}

// --- Table-driven tests for buildPrompt ---

func TestShredder_BuildPrompt_ContainsExpectedSections(t *testing.T) {
	t.Parallel()
	s := NewShredder(agent.NewMockAgent("claude"), "/tmp")

	tests := []struct {
		name    string
		data    shredPromptData
		wantIn  []string
		wantOut []string
	}{
		{
			name: "first attempt -- no validation errors section",
			data: shredPromptData{
				PRDContent:       "some PRD content",
				OutputFile:       "/tmp/out.json",
				ValidationErrors: "",
			},
			wantIn:  []string{"some PRD content", "/tmp/out.json", "E-NNN"},
			wantOut: []string{"Validation Errors from Previous Attempt"},
		},
		{
			name: "retry -- validation errors section present",
			data: shredPromptData{
				PRDContent:       "prd",
				OutputFile:       "/tmp/out.json",
				ValidationErrors: "1. [epics] must not be empty\n",
			},
			wantIn: []string{
				"prd",
				"/tmp/out.json",
				"Validation Errors from Previous Attempt",
				"1. [epics] must not be empty",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prompt, err := s.buildPrompt(tt.data)
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

// --- readPRD tests ---

func TestShredder_ReadPRD_ExactlySizeCap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "prd.md")
	require.NoError(t, os.WriteFile(path, make([]byte, maxPRDSize), 0o644))

	s := NewShredder(agent.NewMockAgent("claude"), dir)
	content, err := s.readPRD(path)

	require.NoError(t, err)
	assert.Equal(t, maxPRDSize, len(content))
}

func TestShredder_ReadPRD_OneByteOver(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "prd.md")
	require.NoError(t, os.WriteFile(path, make([]byte, maxPRDSize+1), 0o644))

	s := NewShredder(agent.NewMockAgent("claude"), dir)
	_, err := s.readPRD(path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 MB")
}

// --- extractBreakdown tests ---

func TestShredder_ExtractBreakdown_FromOutputFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	outputFile := filepath.Join(dir, "out.json")
	require.NoError(t, os.WriteFile(outputFile, validBreakdownJSON(), 0o644))

	s := NewShredder(agent.NewMockAgent("claude"), dir)
	breakdown, validErrs, err := s.extractBreakdown(outputFile, "no json here")

	require.NoError(t, err)
	assert.Nil(t, validErrs)
	require.NotNil(t, breakdown)
	assert.Len(t, breakdown.Epics, 2)
}

func TestShredder_ExtractBreakdown_FallsBackToStdout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	nonExistentFile := filepath.Join(dir, "missing.json")

	s := NewShredder(agent.NewMockAgent("claude"), dir)
	breakdown, validErrs, err := s.extractBreakdown(nonExistentFile, string(validBreakdownJSON()))

	require.NoError(t, err)
	assert.Nil(t, validErrs)
	require.NotNil(t, breakdown)
	assert.Len(t, breakdown.Epics, 2)
}

func TestShredder_ExtractBreakdown_NoJSONAnywhere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	nonExistentFile := filepath.Join(dir, "missing.json")

	s := NewShredder(agent.NewMockAgent("claude"), dir)
	_, _, err := s.extractBreakdown(nonExistentFile, "plain text no json")

	require.Error(t, err)
}

func TestShredder_ExtractBreakdown_InvalidJSONInFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	outputFile := filepath.Join(dir, "out.json")
	// Valid JSON but not an EpicBreakdown-shaped object.
	require.NoError(t, os.WriteFile(outputFile, []byte(`{"not":"breakdown"}`), 0o644))

	// Stdout contains valid breakdown.
	s := NewShredder(agent.NewMockAgent("claude"), dir)
	breakdown, validErrs, err := s.extractBreakdown(outputFile, string(validBreakdownJSON()))

	// The file parses successfully (it is valid JSON) but will fail EpicBreakdown
	// validation (no epics). We then fall through to stdout for actual validation;
	// but since ParseEpicBreakdown succeeds on the file (with validation errors),
	// the file result is returned with those errors.
	// This test just ensures no panic and we get some result.
	assert.NoError(t, err)
	if len(validErrs) > 0 {
		// File result had validation errors -- that's fine.
		_ = breakdown
	}
}

// --- ShredResult field tests ---

func TestShredder_Shred_ResultDurationIsPositive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: string(validBreakdownJSON()), ExitCode: 0, Duration: 5 * time.Millisecond}, nil
	})

	s := NewShredder(mockAgent, dir)
	result, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

	require.NoError(t, err)
	assert.Greater(t, result.Duration, time.Duration(0))
}

func TestShredder_Shred_RetriesCountCorrectly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	callCount := 0
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		if callCount < 3 {
			return &agent.RunResult{Stdout: `{"epics":[]}`, ExitCode: 0}, nil
		}
		return &agent.RunResult{Stdout: string(validBreakdownJSON()), ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir, WithMaxRetries(5))
	result, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

	require.NoError(t, err)
	assert.Equal(t, 2, result.Retries, "two failures before success means 2 retries")
	assert.Equal(t, 3, callCount)
}

// --- Event ordering tests ---

func TestShredder_Shred_EventOrdering(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	ch := make(chan ShredEvent, 20)
	callCount := 0
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		if callCount == 1 {
			return &agent.RunResult{Stdout: `{"epics":[]}`, ExitCode: 0}, nil
		}
		return &agent.RunResult{Stdout: string(validBreakdownJSON()), ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir, WithMaxRetries(3), WithEvents(ch))
	_, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})
	require.NoError(t, err)

	// Collect all events.
	close(ch) // safe since Shred has returned
	var evts []ShredEvent
	for evt := range ch {
		evts = append(evts, evt)
	}

	require.NotEmpty(t, evts)
	assert.Equal(t, ShredEventStarted, evts[0].Type, "first event must be started")
	assert.Equal(t, ShredEventCompleted, evts[len(evts)-1].Type, "last event must be completed")

	// There should be a retry event somewhere in the middle.
	found := false
	for _, evt := range evts[1 : len(evts)-1] {
		if evt.Type == ShredEventRetry {
			found = true
			break
		}
	}
	assert.True(t, found, "retry event must appear between started and completed")
}

// --- Ensure Shredder prompt template has no Go template conflicts ---

func TestShredPromptTemplate_ParsesCleanly(t *testing.T) {
	t.Parallel()
	// The package-level parsedShredTemplate already panics on failure, but
	// we also verify it can be executed with zero-value data without error.
	s := NewShredder(agent.NewMockAgent("claude"), "/tmp")
	prompt, err := s.buildPrompt(shredPromptData{
		PRDContent:       "some content",
		OutputFile:       "/tmp/out.json",
		ValidationErrors: "",
	})
	require.NoError(t, err)
	// The prompt should NOT contain [[ or ]] -- those are delimiters that were
	// consumed during rendering.
	assert.NotContains(t, prompt, "[[")
	assert.NotContains(t, prompt, "]]")
}

// TestShredder_Shred_EpicBreakdownInStdoutSurroundedByText verifies that JSON
// embedded in prose is correctly extracted from stdout.
func TestShredder_Shred_EpicBreakdownInStdoutSurroundedByText(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	wrappedOutput := "Here is the epic breakdown:\n\n" + string(validBreakdownJSON()) + "\n\nDone!"
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: wrappedOutput, ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir)
	result, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Breakdown.Epics, 2)
}

// TestShredder_Shred_OutputFilePreferredOverStdout verifies the extraction
// priority: output file takes precedence over stdout when both contain JSON.
func TestShredder_Shred_OutputFilePreferredOverStdout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)
	outputFile := filepath.Join(dir, "epic-breakdown.json")

	// Output file has 3 epics, stdout has 2 epics.
	threeEpicsBreakdown := EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "T1", Description: "D1"},
			{ID: "E-002", Title: "T2", Description: "D2"},
			{ID: "E-003", Title: "T3", Description: "D3"},
		},
	}
	threeEpicsJSON, err := json.Marshal(threeEpicsBreakdown)
	require.NoError(t, err)

	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		require.NoError(t, os.WriteFile(outputFile, threeEpicsJSON, 0o644))
		return &agent.RunResult{
			Stdout:   string(validBreakdownJSON()), // 2 epics
			ExitCode: 0,
		}, nil
	})

	s := NewShredder(mockAgent, dir)
	result, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath, OutputFile: outputFile})

	require.NoError(t, err)
	// Should use file (3 epics), not stdout (2 epics).
	assert.Len(t, result.Breakdown.Epics, 3, "output file should take priority over stdout")
}

// TestShredder_Shred_StaleOutputFileRemovedOnRetry verifies that a stale output
// file from a previous attempt is removed before the next attempt.
func TestShredder_Shred_StaleOutputFileRemovedOnRetry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)
	outputFile := filepath.Join(dir, "epic-breakdown.json")

	// First attempt: write invalid breakdown to file.
	// Second attempt: write valid breakdown to file.
	callCount := 0
	invalidJSON, err := json.Marshal(EpicBreakdown{Epics: []Epic{}})
	require.NoError(t, err)

	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		if callCount == 1 {
			require.NoError(t, os.WriteFile(outputFile, invalidJSON, 0o644))
			return &agent.RunResult{ExitCode: 0}, nil
		}
		// On second call: file was removed, write valid content.
		_, statErr := os.Stat(outputFile)
		assert.True(t, os.IsNotExist(statErr), "stale output file should have been removed before retry")
		require.NoError(t, os.WriteFile(outputFile, validBreakdownJSON(), 0o644))
		return &agent.RunResult{ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir, WithMaxRetries(3))
	result, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath, OutputFile: outputFile})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Retries)
	assert.Equal(t, 2, callCount)
}

// TestShredder_Shred_EventAttemptNumbers verifies that attempt numbers in events
// are 1-based and increment correctly across retries.
func TestShredder_Shred_EventAttemptNumbers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)
	ch := make(chan ShredEvent, 20)

	callCount := 0
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		if callCount < 3 {
			return &agent.RunResult{Stdout: `{"epics":[]}`, ExitCode: 0}, nil
		}
		return &agent.RunResult{Stdout: string(validBreakdownJSON()), ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir, WithMaxRetries(5), WithEvents(ch))
	_, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})
	require.NoError(t, err)

	close(ch)
	var evts []ShredEvent
	for evt := range ch {
		evts = append(evts, evt)
	}

	// started: attempt 1; retry: attempt 2; retry: attempt 3; completed: attempt 3
	require.NotEmpty(t, evts)
	assert.Equal(t, 1, evts[0].Attempt, "started event should have attempt=1")

	// Find retry events and verify they have increasing attempt numbers.
	var retryAttempts []int
	for _, evt := range evts {
		if evt.Type == ShredEventRetry {
			retryAttempts = append(retryAttempts, evt.Attempt)
		}
	}
	require.Len(t, retryAttempts, 2, "expected 2 retry events")
	assert.Equal(t, 2, retryAttempts[0])
	assert.Equal(t, 3, retryAttempts[1])
}

// TestShredder_Shred_FailedEventCarriesLastErrors verifies that the failed event
// includes the last set of validation errors.
func TestShredder_Shred_FailedEventCarriesLastErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)
	ch := make(chan ShredEvent, 20)

	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: `{"epics":[]}`, ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir, WithMaxRetries(1), WithEvents(ch))
	_, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})
	require.Error(t, err)

	close(ch)
	var failedEvt *ShredEvent
	for evt := range ch {
		evt := evt
		if evt.Type == ShredEventFailed {
			failedEvt = &evt
		}
	}
	require.NotNil(t, failedEvt, "ShredEventFailed must be emitted")
	assert.NotEmpty(t, failedEvt.Errors, "failed event must carry validation errors")
	// The error should be about empty epics.
	found := false
	for _, ve := range failedEvt.Errors {
		if strings.Contains(ve.Field, "epics") {
			found = true
			break
		}
	}
	assert.True(t, found, "failed event errors should reference the epics field")
}

// ---------------------------------------------------------------------------
// AC-9: Markdown-fenced JSON extraction from stdout
// ---------------------------------------------------------------------------

// TestShredder_Shred_StdoutWithMarkdownFence verifies that JSON embedded inside
// a markdown code fence (```json ... ```) is successfully extracted via the
// stdout fallback path.
func TestShredder_Shred_StdoutWithMarkdownFence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	fenced := "Here is the breakdown:\n\n```json\n" + string(validBreakdownJSON()) + "\n```\n\nDone."
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: fenced, ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir)
	result, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Breakdown.Epics, 2)
	assert.Equal(t, 0, result.Retries)
}

// ---------------------------------------------------------------------------
// AC-10: Empty PRD file
// ---------------------------------------------------------------------------

// TestShredder_Shred_EmptyPRDFile verifies that an empty PRD file is accepted
// (no size error) and the prompt is built with an empty PRD content section.
// The agent still returns valid JSON so Shred succeeds.
func TestShredder_Shred_EmptyPRDFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	emptyPRD := filepath.Join(dir, "empty.md")
	require.NoError(t, os.WriteFile(emptyPRD, []byte(""), 0o644))

	var capturedPrompt string
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		capturedPrompt = opts.Prompt
		return &agent.RunResult{Stdout: string(validBreakdownJSON()), ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir)
	result, err := s.Shred(context.Background(), ShredOpts{PRDPath: emptyPRD})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.Retries)
	// Prompt should be built even with no PRD content — the template section
	// will be present but the injected content will be empty.
	assert.NotEmpty(t, capturedPrompt)
}

// ---------------------------------------------------------------------------
// AC-5 extended: Rate-limit error propagation
// ---------------------------------------------------------------------------

// TestShredder_Shred_AgentRateLimitError verifies that when the agent's Run
// method returns a rate-limit-flavored error it propagates up from Shred
// without retry (agent errors are not retried — only validation errors are).
func TestShredder_Shred_AgentRateLimitError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	rateLimitErr := fmt.Errorf("rate limit exceeded: try again in 5 minutes")
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return nil, rateLimitErr
	})

	s := NewShredder(mockAgent, dir, WithMaxRetries(3))
	result, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

	require.Error(t, err)
	assert.Nil(t, result)
	// Error must wrap the original agent error.
	assert.Contains(t, err.Error(), "rate limit exceeded")
	// The mock should only have been called once — agent errors are not retried.
	assert.Len(t, mockAgent.Calls, 1, "agent error must not trigger retry")
}

// ---------------------------------------------------------------------------
// AC-6 extended: Context cancellation during retry loop
// ---------------------------------------------------------------------------

// TestShredder_Shred_ContextCancelledDuringRetry verifies that cancelling the
// context between retry attempts stops the loop and returns a context error,
// even when attempts up to that point were yielding validation failures.
func TestShredder_Shred_ContextCancelledDuringRetry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		// Cancel after the first attempt returns invalid JSON so the retry
		// loop will hit ctx.Err() at the top of the second iteration.
		if callCount == 1 {
			cancel()
			return &agent.RunResult{Stdout: `{"epics":[]}`, ExitCode: 0}, nil
		}
		return &agent.RunResult{Stdout: string(validBreakdownJSON()), ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir, WithMaxRetries(3))
	result, err := s.Shred(ctx, ShredOpts{PRDPath: prdPath})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "context")
	// The agent should have been invoked exactly once before cancellation.
	assert.Equal(t, 1, callCount)
}

// ---------------------------------------------------------------------------
// WithMaxRetries: exact call-count assertion
// ---------------------------------------------------------------------------

// TestShredder_WithMaxRetries_ExactCallCount verifies that setting
// WithMaxRetries(n) allows exactly n+1 total agent invocations before failure.
func TestShredder_WithMaxRetries_ExactCallCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		maxRetries int
		wantCalls  int
	}{
		{name: "zero retries — one call only", maxRetries: 0, wantCalls: 1},
		{name: "one retry — two calls", maxRetries: 1, wantCalls: 2},
		{name: "two retries — three calls", maxRetries: 2, wantCalls: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			prdPath := writePRDFile(t, dir)

			// Always return invalid JSON so all allowed attempts are consumed.
			mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
				return &agent.RunResult{Stdout: `{"epics":[]}`, ExitCode: 0}, nil
			})

			s := NewShredder(mockAgent, dir, WithMaxRetries(tt.maxRetries))
			_, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

			require.Error(t, err)
			assert.Len(t, mockAgent.Calls, tt.wantCalls,
				"expected exactly %d agent calls for maxRetries=%d", tt.wantCalls, tt.maxRetries)
		})
	}
}

// ---------------------------------------------------------------------------
// Table-driven: various invalid EpicBreakdown patterns
// ---------------------------------------------------------------------------

// TestShredder_Shred_TableDriven_InvalidJSON covers a range of malformed or
// schema-invalid stdout payloads that must all result in an error when
// maxRetries is 0 (no retries allowed).
func TestShredder_Shred_TableDriven_InvalidJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stdout string
	}{
		{
			name:   "completely empty stdout",
			stdout: "",
		},
		{
			name:   "plain text no JSON",
			stdout: "Sorry, I cannot help with that.",
		},
		{
			name:   "malformed JSON",
			stdout: `{"epics": [{"id": "E-001"`,
		},
		{
			name:   "empty epics array",
			stdout: `{"epics":[]}`,
		},
		{
			name:   "missing id field",
			stdout: `{"epics":[{"title":"T","description":"D","prd_sections":[],"estimated_task_count":1,"dependencies_on_epics":[]}]}`,
		},
		{
			name:   "missing title field",
			stdout: `{"epics":[{"id":"E-001","description":"D","prd_sections":[],"estimated_task_count":1,"dependencies_on_epics":[]}]}`,
		},
		{
			name:   "missing description field",
			stdout: `{"epics":[{"id":"E-001","title":"T","prd_sections":[],"estimated_task_count":1,"dependencies_on_epics":[]}]}`,
		},
		{
			name:   "invalid id format",
			stdout: `{"epics":[{"id":"BAD","title":"T","description":"D","prd_sections":[],"estimated_task_count":1,"dependencies_on_epics":[]}]}`,
		},
		{
			name:   "self-dependency",
			stdout: `{"epics":[{"id":"E-001","title":"T","description":"D","prd_sections":[],"estimated_task_count":1,"dependencies_on_epics":["E-001"]}]}`,
		},
		{
			name:   "wrong top-level key",
			stdout: `{"tasks":[]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			prdPath := writePRDFile(t, dir)

			mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
				return &agent.RunResult{Stdout: tt.stdout, ExitCode: 0}, nil
			})

			s := NewShredder(mockAgent, dir, WithMaxRetries(0))
			result, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

			require.Error(t, err, "expected error for invalid stdout: %s", tt.name)
			assert.Nil(t, result)
		})
	}
}

// ---------------------------------------------------------------------------
// Table-driven: stdout extraction formats
// ---------------------------------------------------------------------------

// TestShredder_Shred_TableDriven_StdoutFormats verifies that valid
// EpicBreakdown JSON is correctly extracted from various stdout wrapping
// styles via the jsonutil fallback path.
func TestShredder_Shred_TableDriven_StdoutFormats(t *testing.T) {
	t.Parallel()

	validJSON := string(validBreakdownJSON())

	tests := []struct {
		name       string
		stdout     string
		wantEpics  int
	}{
		{
			name:      "bare JSON",
			stdout:    validJSON,
			wantEpics: 2,
		},
		{
			name:      "JSON preceded by prose",
			stdout:    "Here is the breakdown:\n\n" + validJSON,
			wantEpics: 2,
		},
		{
			name:      "JSON followed by prose",
			stdout:    validJSON + "\n\nThe breakdown is complete.",
			wantEpics: 2,
		},
		{
			name:      "JSON surrounded by prose",
			stdout:    "Starting output.\n" + validJSON + "\nEnd of output.",
			wantEpics: 2,
		},
		{
			name:      "backtick fenced JSON",
			stdout:    "```json\n" + validJSON + "\n```",
			wantEpics: 2,
		},
		{
			name:      "backtick fenced without language tag",
			stdout:    "```\n" + validJSON + "\n```",
			wantEpics: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			prdPath := writePRDFile(t, dir)

			mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
				return &agent.RunResult{Stdout: tt.stdout, ExitCode: 0}, nil
			})

			s := NewShredder(mockAgent, dir)
			result, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

			require.NoError(t, err, "unexpected error for stdout format: %s", tt.name)
			require.NotNil(t, result)
			assert.Len(t, result.Breakdown.Epics, tt.wantEpics)
		})
	}
}

// ---------------------------------------------------------------------------
// AC-12: Comprehensive event emission coverage
// ---------------------------------------------------------------------------

// TestShredder_Events_AllRetriesExhausted verifies that on complete failure
// the event sequence is: shred_started, shred_retry×N, shred_failed.
func TestShredder_Events_AllRetriesExhausted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)
	ch := make(chan ShredEvent, 20)

	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: `{"epics":[]}`, ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir, WithMaxRetries(2), WithEvents(ch))
	_, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})
	require.Error(t, err)

	close(ch)
	var evts []ShredEvent
	for evt := range ch {
		evts = append(evts, evt)
	}

	require.NotEmpty(t, evts)
	assert.Equal(t, ShredEventStarted, evts[0].Type, "first event must be shred_started")
	assert.Equal(t, ShredEventFailed, evts[len(evts)-1].Type, "last event must be shred_failed")

	var retryCount int
	for _, evt := range evts {
		if evt.Type == ShredEventRetry {
			retryCount++
		}
	}
	assert.Equal(t, 2, retryCount, "expected 2 retry events for maxRetries=2")
}

// TestShredder_Events_SuccessNoRetry verifies that on first-attempt success
// the event sequence is exactly: shred_started, shred_completed.
func TestShredder_Events_SuccessNoRetry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)
	ch := make(chan ShredEvent, 10)

	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: string(validBreakdownJSON()), ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir, WithEvents(ch))
	_, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})
	require.NoError(t, err)

	close(ch)
	var evts []ShredEvent
	for evt := range ch {
		evts = append(evts, evt)
	}

	require.Len(t, evts, 2, "success without retry should emit exactly 2 events")
	assert.Equal(t, ShredEventStarted, evts[0].Type)
	assert.Equal(t, ShredEventCompleted, evts[1].Type)

	// No retry events.
	for _, evt := range evts {
		assert.NotEqual(t, ShredEventRetry, evt.Type, "no retry event expected on first-attempt success")
	}
}

// ---------------------------------------------------------------------------
// Malformed output file falls back to valid stdout
// ---------------------------------------------------------------------------

// TestShredder_ExtractBreakdown_MalformedFileFallsBackToStdout verifies that
// when the output file contains unparseable JSON (not even valid JSON syntax)
// the extractor falls back to stdout and succeeds.
func TestShredder_ExtractBreakdown_MalformedFileFallsBackToStdout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	outputFile := filepath.Join(dir, "out.json")
	// Write syntactically invalid JSON to the file.
	require.NoError(t, os.WriteFile(outputFile, []byte("this is not json {{{"), 0o644))

	s := NewShredder(agent.NewMockAgent("claude"), dir)
	breakdown, validErrs, err := s.extractBreakdown(outputFile, string(validBreakdownJSON()))

	require.NoError(t, err)
	assert.Nil(t, validErrs)
	require.NotNil(t, breakdown)
	assert.Len(t, breakdown.Epics, 2)
}

// ---------------------------------------------------------------------------
// PRD file size boundary conditions
// ---------------------------------------------------------------------------

// TestShredder_PRDFileTooLarge_ErrorMessageContainsSizeInfo verifies that the
// error returned for an oversized PRD includes byte count and the "1 MB" cap.
func TestShredder_PRDFileTooLarge_ErrorMessageContainsSizeInfo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bigFile := filepath.Join(dir, "oversized.md")
	oversize := maxPRDSize + 512
	require.NoError(t, os.WriteFile(bigFile, make([]byte, oversize), 0o644))

	s := NewShredder(agent.NewMockAgent("claude"), dir)
	_, err := s.Shred(context.Background(), ShredOpts{PRDPath: bigFile})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 MB", "error must mention the 1 MB cap")
	assert.Contains(t, err.Error(), fmt.Sprintf("%d", oversize), "error must include actual file size")
}

// ---------------------------------------------------------------------------
// Prompt template: duplicate IDs and dependency validation in error message
// ---------------------------------------------------------------------------

// TestShredder_Shred_RetryPromptContainsDuplicateIDError verifies that when
// the agent produces a breakdown with duplicate epic IDs, the retry prompt
// references the duplicate ID validation error.
func TestShredder_Shred_RetryPromptContainsDuplicateIDError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prdPath := writePRDFile(t, dir)

	// Two epics with the same ID — duplicate ID validation error.
	duplicateIDJSON := `{"epics":[` +
		`{"id":"E-001","title":"T1","description":"D1","prd_sections":[],"estimated_task_count":1,"dependencies_on_epics":[]},` +
		`{"id":"E-001","title":"T2","description":"D2","prd_sections":[],"estimated_task_count":1,"dependencies_on_epics":[]}` +
		`]}`

	callCount := 0
	var retryPrompt string
	mockAgent := agent.NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts agent.RunOpts) (*agent.RunResult, error) {
		callCount++
		if callCount == 1 {
			return &agent.RunResult{Stdout: duplicateIDJSON, ExitCode: 0}, nil
		}
		retryPrompt = opts.Prompt
		return &agent.RunResult{Stdout: string(validBreakdownJSON()), ExitCode: 0}, nil
	})

	s := NewShredder(mockAgent, dir, WithMaxRetries(3))
	_, err := s.Shred(context.Background(), ShredOpts{PRDPath: prdPath})

	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
	assert.Contains(t, retryPrompt, "duplicate", "retry prompt must mention duplicate epic ID error")
}

// ---------------------------------------------------------------------------
// Benchmark
// ---------------------------------------------------------------------------

// BenchmarkShredder_BuildPrompt measures the cost of rendering the shred
// prompt template, which is on the hot path for every attempt.
func BenchmarkShredder_BuildPrompt(b *testing.B) {
	s := NewShredder(agent.NewMockAgent("claude"), b.TempDir())
	data := shredPromptData{
		PRDContent:       strings.Repeat("# Section\nContent line.\n", 100),
		OutputFile:       "/tmp/epic-breakdown.json",
		ValidationErrors: "",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.buildPrompt(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkShredder_BuildPrompt_WithErrors measures prompt rendering when
// validation errors are appended (the retry path).
func BenchmarkShredder_BuildPrompt_WithErrors(b *testing.B) {
	s := NewShredder(agent.NewMockAgent("claude"), b.TempDir())
	errs := FormatValidationErrors([]ValidationError{
		{Field: "epics[0].id", Message: "must not be empty"},
		{Field: "epics[0].title", Message: "must not be empty"},
		{Field: "epics[0].description", Message: "must not be empty"},
	})
	data := shredPromptData{
		PRDContent:       strings.Repeat("# Section\nContent line.\n", 100),
		OutputFile:       "/tmp/epic-breakdown.json",
		ValidationErrors: errs,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.buildPrompt(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ---------------------------------------------------------------------------
// Fuzz test
// ---------------------------------------------------------------------------

// FuzzShredder_BuildPrompt verifies that the prompt template renderer never
// panics or returns an error regardless of the PRD content injected.
func FuzzShredder_BuildPrompt(f *testing.F) {
	// Seed corpus with interesting edge cases.
	f.Add("")
	f.Add("# Simple PRD\n\nContent.")
	f.Add("[[malicious delimiter]]")
	f.Add("]] end delimiter attack [[")
	f.Add(strings.Repeat("A", 10000))
	f.Add("PRD with <script>alert(1)</script> content")
	f.Add("{\"json\": \"embedded\"}")
	f.Add("PRD with backticks ``` code ``` here")

	s := NewShredder(agent.NewMockAgent("claude"), "/tmp")

	f.Fuzz(func(t *testing.T, prdContent string) {
		data := shredPromptData{
			PRDContent:       prdContent,
			OutputFile:       "/tmp/out.json",
			ValidationErrors: "",
		}
		// Must not panic and must not return an error for any PRD content.
		prompt, err := s.buildPrompt(data)
		if err != nil {
			t.Errorf("buildPrompt returned unexpected error: %v", err)
		}
		// The rendered prompt must not contain raw template delimiters.
		if strings.Contains(prompt, "[[") || strings.Contains(prompt, "]]") {
			// Only fail if the [[ or ]] appear outside of what was injected by
			// prdContent itself (i.e., they are un-consumed template delimiters).
			// A simple heuristic: if the prompt length is longer than the
			// template constant plus the PRD content, something is wrong.
			// We check for the specific template marker that should have been
			// consumed.
			if strings.Contains(prompt, "[[ .PRDContent ]]") ||
				strings.Contains(prompt, "[[ .OutputFile ]]") ||
				strings.Contains(prompt, "[[ .ValidationErrors ]]") {
				t.Errorf("buildPrompt left unconsumed template delimiters in output")
			}
		}
	})
}
