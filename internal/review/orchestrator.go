package review

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"golang.org/x/sync/errgroup"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
	"github.com/AbdelazizMoustafa10m/Raven/internal/jsonutil"
)

// ReviewEvent is a structured event emitted during the review pipeline for
// TUI consumption. All fields are populated for every event; Agent may be
// empty for pipeline-level events (e.g. review_started, consolidated).
type ReviewEvent struct {
	// Type is one of: review_started, agent_started, agent_completed,
	// agent_error, rate_limited, consolidated.
	Type      string
	Agent     string
	Message   string
	Timestamp time.Time
}

// AgentError records a per-agent failure that did not abort the overall review.
type AgentError struct {
	Agent   string
	Err     error
	Message string
}

// OrchestratorResult is the final output of a review pipeline run.
type OrchestratorResult struct {
	Consolidated *ConsolidatedReview
	Stats        *ConsolidationStats
	DiffResult   *DiffResult
	Duration     time.Duration
	AgentErrors  []AgentError
}

// ReviewOrchestrator coordinates multi-agent parallel code review. It fans out
// review requests to agents concurrently, extracts structured JSON from each
// agent's output, and consolidates the results into a single deduplicated
// report.
type ReviewOrchestrator struct {
	agentRegistry *agent.Registry
	diffGen       *DiffGenerator
	promptBuilder *PromptBuilder
	consolidator  *Consolidator
	concurrency   int
	logger        *log.Logger
	events        chan<- ReviewEvent
}

// NewReviewOrchestrator creates a ReviewOrchestrator with the given
// dependencies. concurrency must be >= 1; a value <= 0 is clamped to 1.
// events may be nil — events are dropped when the channel is nil or full.
func NewReviewOrchestrator(
	registry *agent.Registry,
	diffGen *DiffGenerator,
	promptBuilder *PromptBuilder,
	consolidator *Consolidator,
	concurrency int,
	logger *log.Logger,
	events chan<- ReviewEvent,
) *ReviewOrchestrator {
	if concurrency <= 0 {
		concurrency = 1
	}
	return &ReviewOrchestrator{
		agentRegistry: registry,
		diffGen:       diffGen,
		promptBuilder: promptBuilder,
		consolidator:  consolidator,
		concurrency:   concurrency,
		logger:        logger,
		events:        events,
	}
}

// Run executes the full review pipeline.
//
// Steps:
//  1. Validate inputs and resolve agents from the registry.
//  2. Generate a unified diff against opts.BaseBranch.
//  3. Assign files to agents according to opts.Mode.
//  4. Fan out review requests to agents concurrently via errgroup.
//  5. Extract ReviewResult JSON from each agent's stdout.
//  6. Consolidate all results.
//
// Per-agent errors are captured in OrchestratorResult.AgentErrors and do NOT
// abort the pipeline — review continues with the remaining agents.
func (ro *ReviewOrchestrator) Run(ctx context.Context, opts ReviewOpts) (*OrchestratorResult, error) {
	start := time.Now()

	// --- Input validation ---
	if len(opts.Agents) == 0 {
		return nil, fmt.Errorf("review: orchestrator: at least one agent is required")
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	// Resolve all agents up-front so we fail fast on unknown names.
	agents := make([]agent.Agent, 0, len(opts.Agents))
	for _, name := range opts.Agents {
		ag, err := ro.agentRegistry.Get(name)
		if err != nil {
			return nil, fmt.Errorf("review: orchestrator: resolving agent %q: %w", name, err)
		}
		agents = append(agents, ag)
	}

	ro.emit(ReviewEvent{
		Type:      "review_started",
		Message:   fmt.Sprintf("starting review with %d agent(s)", len(agents)),
		Timestamp: time.Now(),
	})

	if ro.logger != nil {
		ro.logger.Info("review started",
			"agents", opts.Agents,
			"mode", opts.Mode,
			"base_branch", opts.BaseBranch,
			"concurrency", concurrency,
		)
	}

	// --- Diff generation ---
	diffResult, err := ro.diffGen.Generate(ctx, opts.BaseBranch)
	if err != nil {
		return nil, fmt.Errorf("review: orchestrator: generating diff: %w", err)
	}

	// --- File assignment ---
	// fileBuckets[i] is the slice of files assigned to agents[i].
	fileBuckets := ro.assignFiles(diffResult, opts.Mode, len(agents))

	// --- Parallel agent fan-out ---
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	var mu sync.Mutex
	var agentResults []AgentReviewResult
	var agentErrors []AgentError

	for i, ag := range agents {
		i, ag := i, ag // capture loop variables
		files := fileBuckets[i]

		g.Go(func() error {
			result, agErr := ro.runAgent(gctx, ag, diffResult, files, opts.Mode)

			mu.Lock()
			agentResults = append(agentResults, result)
			if agErr != nil {
				agentErrors = append(agentErrors, *agErr)
			}
			mu.Unlock()

			// ALWAYS return nil — per-agent errors must not abort the errgroup.
			return nil
		})
	}

	// Wait for all workers. The only non-nil error from g.Wait() would be a
	// context cancellation, which we surface to the caller.
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("review: orchestrator: agent workers: %w", err)
	}

	// --- Consolidation ---
	consolidated, stats := ro.consolidator.Consolidate(agentResults)
	consolidated.Duration = time.Since(start)

	ro.emit(ReviewEvent{
		Type:      "consolidated",
		Message:   fmt.Sprintf("consolidated %d finding(s), verdict: %s", len(consolidated.Findings), consolidated.Verdict),
		Timestamp: time.Now(),
	})

	if ro.logger != nil {
		ro.logger.Info("review consolidated",
			"findings", len(consolidated.Findings),
			"verdict", consolidated.Verdict,
			"agent_errors", len(agentErrors),
			"duration", time.Since(start),
		)
	}

	return &OrchestratorResult{
		Consolidated: consolidated,
		Stats:        stats,
		DiffResult:   diffResult,
		Duration:     time.Since(start),
		AgentErrors:  agentErrors,
	}, nil
}

// DryRun returns a human-readable description of the planned review actions
// without invoking any agent. It performs diff generation and file assignment
// so the output accurately reflects what Run() would do.
func (ro *ReviewOrchestrator) DryRun(ctx context.Context, opts ReviewOpts) (string, error) {
	if len(opts.Agents) == 0 {
		return "", fmt.Errorf("review: orchestrator: at least one agent is required")
	}

	// Resolve agents to get DryRunCommand output.
	agents := make([]agent.Agent, 0, len(opts.Agents))
	for _, name := range opts.Agents {
		ag, err := ro.agentRegistry.Get(name)
		if err != nil {
			return "", fmt.Errorf("review: orchestrator: resolving agent %q: %w", name, err)
		}
		agents = append(agents, ag)
	}

	// Generate diff for accurate file counts.
	diffResult, err := ro.diffGen.Generate(ctx, opts.BaseBranch)
	if err != nil {
		return "", fmt.Errorf("review: orchestrator: generating diff for dry run: %w", err)
	}

	fileBuckets := ro.assignFiles(diffResult, opts.Mode, len(agents))

	var sb strings.Builder
	sb.WriteString("Review Plan (dry run)\n")
	fmt.Fprintf(&sb, "Base branch: %s\n", opts.BaseBranch)
	fmt.Fprintf(&sb, "Mode: %s\n", opts.Mode)
	fmt.Fprintf(&sb, "Agents: %d\n", len(agents))

	for i, ag := range agents {
		files := fileBuckets[i]
		// Build a representative RunOpts so DryRunCommand can produce a useful string.
		runOpts := agent.RunOpts{
			Prompt:  "(review prompt)",
			WorkDir: ".",
		}
		cmd := ag.DryRunCommand(runOpts)
		fmt.Fprintf(&sb, "  %s: %d files, command: %s\n", ag.Name(), len(files), cmd)
	}

	return sb.String(), nil
}

// runAgent executes a single agent review pass: builds the prompt, calls
// agent.Run(), extracts JSON, and validates the result. It returns both the
// AgentReviewResult (always populated) and an optional *AgentError (non-nil
// when the agent produced an error or unusable output).
func (ro *ReviewOrchestrator) runAgent(
	ctx context.Context,
	ag agent.Agent,
	diff *DiffResult,
	files []ChangedFile,
	mode ReviewMode,
) (AgentReviewResult, *AgentError) {
	agentStart := time.Now()
	agentName := ag.Name()

	ro.emit(ReviewEvent{
		Type:      "agent_started",
		Agent:     agentName,
		Message:   fmt.Sprintf("agent %s starting review of %d file(s)", agentName, len(files)),
		Timestamp: time.Now(),
	})

	if ro.logger != nil {
		ro.logger.Info("agent review started",
			"agent", agentName,
			"files", len(files),
			"mode", mode,
		)
	}

	// Build the review prompt.
	prompt, err := ro.promptBuilder.BuildForAgent(ctx, agentName, diff, files, mode)
	if err != nil {
		agErr := &AgentError{
			Agent:   agentName,
			Err:     err,
			Message: fmt.Sprintf("building prompt: %v", err),
		}
		ro.emit(ReviewEvent{
			Type:      "agent_error",
			Agent:     agentName,
			Message:   agErr.Message,
			Timestamp: time.Now(),
		})
		return AgentReviewResult{
			Agent:    agentName,
			Duration: time.Since(agentStart),
			Err:      fmt.Errorf("review: orchestrator: agent %s: building prompt: %w", agentName, err),
		}, agErr
	}

	// Execute the agent.
	runOpts := agent.RunOpts{
		Prompt: prompt,
	}
	result, err := ag.Run(ctx, runOpts)
	duration := time.Since(agentStart)

	if err != nil {
		agErr := &AgentError{
			Agent:   agentName,
			Err:     err,
			Message: fmt.Sprintf("agent.Run failed: %v", err),
		}
		ro.emit(ReviewEvent{
			Type:      "agent_error",
			Agent:     agentName,
			Message:   agErr.Message,
			Timestamp: time.Now(),
		})
		if ro.logger != nil {
			ro.logger.Error("agent run failed", "agent", agentName, "error", err)
		}
		return AgentReviewResult{
			Agent:    agentName,
			Duration: duration,
			Err:      fmt.Errorf("review: orchestrator: agent %s: run failed: %w", agentName, err),
		}, agErr
	}

	// Check for rate limiting.
	if result.WasRateLimited() {
		msg := fmt.Sprintf("agent %s rate limited", agentName)
		if result.RateLimit != nil && result.RateLimit.Message != "" {
			msg = result.RateLimit.Message
		}
		ro.emit(ReviewEvent{
			Type:      "rate_limited",
			Agent:     agentName,
			Message:   msg,
			Timestamp: time.Now(),
		})
		if ro.logger != nil {
			ro.logger.Warn("agent rate limited", "agent", agentName)
		}
		agErr := &AgentError{
			Agent:   agentName,
			Err:     fmt.Errorf("rate limited"),
			Message: msg,
		}
		return AgentReviewResult{
			Agent:     agentName,
			Duration:  duration,
			Err:       fmt.Errorf("review: orchestrator: agent %s: rate limited", agentName),
			RawOutput: result.Stdout,
		}, agErr
	}

	// Non-zero exit code without rate limiting is an error.
	if result.ExitCode != 0 {
		msg := fmt.Sprintf("agent %s exited with code %d", agentName, result.ExitCode)
		ro.emit(ReviewEvent{
			Type:      "agent_error",
			Agent:     agentName,
			Message:   msg,
			Timestamp: time.Now(),
		})
		if ro.logger != nil {
			ro.logger.Error("agent non-zero exit", "agent", agentName, "exit_code", result.ExitCode)
		}
		agErr := &AgentError{
			Agent:   agentName,
			Err:     fmt.Errorf("exit code %d", result.ExitCode),
			Message: msg,
		}
		return AgentReviewResult{
			Agent:     agentName,
			Duration:  duration,
			Err:       fmt.Errorf("review: orchestrator: agent %s: non-zero exit code %d", agentName, result.ExitCode),
			RawOutput: result.Stdout,
		}, agErr
	}

	// Extract JSON from agent output.
	var reviewResult ReviewResult
	if extractErr := jsonutil.ExtractInto(result.Stdout, &reviewResult); extractErr != nil {
		msg := fmt.Sprintf("agent %s: failed to extract JSON from output: %v", agentName, extractErr)
		ro.emit(ReviewEvent{
			Type:      "agent_error",
			Agent:     agentName,
			Message:   msg,
			Timestamp: time.Now(),
		})
		if ro.logger != nil {
			ro.logger.Warn("JSON extraction failed",
				"agent", agentName,
				"error", extractErr,
				"stdout_len", len(result.Stdout),
			)
		}
		agErr := &AgentError{
			Agent:   agentName,
			Err:     extractErr,
			Message: msg,
		}
		return AgentReviewResult{
			Agent:     agentName,
			Duration:  duration,
			Err:       fmt.Errorf("review: orchestrator: agent %s: extracting JSON: %w", agentName, extractErr),
			RawOutput: result.Stdout,
		}, agErr
	}

	// Validate the extracted result.
	if validateErr := reviewResult.Validate(); validateErr != nil {
		msg := fmt.Sprintf("agent %s: invalid review result: %v", agentName, validateErr)
		ro.emit(ReviewEvent{
			Type:      "agent_error",
			Agent:     agentName,
			Message:   msg,
			Timestamp: time.Now(),
		})
		if ro.logger != nil {
			ro.logger.Warn("review result validation failed",
				"agent", agentName,
				"error", validateErr,
			)
		}
		agErr := &AgentError{
			Agent:   agentName,
			Err:     validateErr,
			Message: msg,
		}
		return AgentReviewResult{
			Agent:     agentName,
			Duration:  duration,
			Err:       fmt.Errorf("review: orchestrator: agent %s: invalid result: %w", agentName, validateErr),
			RawOutput: result.Stdout,
		}, agErr
	}

	ro.emit(ReviewEvent{
		Type:      "agent_completed",
		Agent:     agentName,
		Message:   fmt.Sprintf("agent %s completed: %d finding(s), verdict %s", agentName, len(reviewResult.Findings), reviewResult.Verdict),
		Timestamp: time.Now(),
	})

	if ro.logger != nil {
		ro.logger.Info("agent review completed",
			"agent", agentName,
			"findings", len(reviewResult.Findings),
			"verdict", reviewResult.Verdict,
			"duration", duration,
		)
	}

	return AgentReviewResult{
		Agent:     agentName,
		Result:    &reviewResult,
		Duration:  duration,
		RawOutput: result.Stdout,
	}, nil
}

// assignFiles builds per-agent file buckets according to the review mode.
// In "all" mode every agent receives all files. In "split" mode files are
// partitioned across agents via SplitFiles. The returned slice always has
// exactly n entries (one per agent); empty slices are used when an agent
// receives no files.
func (ro *ReviewOrchestrator) assignFiles(diff *DiffResult, mode ReviewMode, n int) [][]ChangedFile {
	buckets := make([][]ChangedFile, n)

	switch mode {
	case ReviewModeSplit:
		splits := SplitFiles(diff.Files, n)
		for i := 0; i < n; i++ {
			if i < len(splits) {
				buckets[i] = splits[i]
			}
			// else: bucket[i] stays nil (empty slice for this agent)
		}
	default: // ReviewModeAll and any unrecognised mode
		for i := range buckets {
			buckets[i] = diff.Files
		}
	}

	return buckets
}

// emit sends a ReviewEvent to the events channel using a non-blocking send.
// If the channel is nil or full the event is silently dropped.
func (ro *ReviewOrchestrator) emit(ev ReviewEvent) {
	if ro.events == nil {
		return
	}
	select {
	case ro.events <- ev:
	default:
		// Drop the event rather than blocking the worker goroutine.
	}
}
