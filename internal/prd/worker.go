package prd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"text/template"
	"time"

	"github.com/charmbracelet/log"
	"golang.org/x/sync/errgroup"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
	"github.com/AbdelazizMoustafa10m/Raven/internal/jsonutil"
)

// ScatterEventType identifies the kind of scatter event emitted during parallel epic decomposition.
type ScatterEventType string

const (
	// ScatterEventWorkerStarted is emitted when a worker begins processing an epic.
	ScatterEventWorkerStarted ScatterEventType = "worker_started"
	// ScatterEventWorkerCompleted is emitted when a worker successfully decomposes an epic.
	ScatterEventWorkerCompleted ScatterEventType = "worker_completed"
	// ScatterEventWorkerRetry is emitted before each retry attempt within a worker.
	ScatterEventWorkerRetry ScatterEventType = "worker_retry"
	// ScatterEventWorkerFailed is emitted when a worker exhausts all retries.
	ScatterEventWorkerFailed ScatterEventType = "worker_failed"
	// ScatterEventRateLimited is emitted when a rate limit is detected and the worker must wait.
	ScatterEventRateLimited ScatterEventType = "rate_limited"
)

// ScatterEvent is emitted during the scatter phase for progress tracking.
type ScatterEvent struct {
	// Type identifies the kind of event.
	Type ScatterEventType
	// EpicID is the epic being processed when this event was emitted.
	EpicID string
	// Message is a human-readable description of the event.
	Message string
	// Attempt is the 1-based attempt number associated with this event.
	Attempt int
}

// ScatterOpts specifies the parameters for a single Scatter call.
type ScatterOpts struct {
	// PRDContent is the full PRD text injected into each worker prompt for context.
	PRDContent string
	// Breakdown is the EpicBreakdown produced by Phase 1 (Shredder).
	Breakdown *EpicBreakdown
	// Model is an optional model override for each agent invocation.
	Model string
	// Effort is an optional effort-level override for each agent invocation.
	Effort string
}

// ScatterResult contains the aggregated output of the scatter phase.
type ScatterResult struct {
	// Results holds one EpicTaskResult per epic that succeeded, ordered by epic ID.
	Results []*EpicTaskResult
	// Failures holds metadata for epics that failed after all retries.
	Failures []ScatterFailure
	// Duration is the total wall-clock time spent in Scatter.
	Duration time.Duration
}

// ScatterFailure records information about an epic that could not be decomposed.
type ScatterFailure struct {
	// EpicID identifies the epic that failed.
	EpicID string
	// Errors holds the last set of validation errors, if any.
	Errors []ValidationError
	// Err is the underlying error (e.g., context cancellation, rate-limit exhaustion).
	Err error
}

// ScatterOrchestrator orchestrates the parallel Phase 2 decomposition of epics into tasks.
// It spawns one worker goroutine per epic, bounded by the configured concurrency limit.
type ScatterOrchestrator struct {
	agent       agent.Agent
	workDir     string
	concurrency int
	maxRetries  int
	rateLimiter *agent.RateLimitCoordinator
	logger      *log.Logger
	events      chan<- ScatterEvent
}

// ScatterOption is a functional option for configuring a ScatterOrchestrator.
type ScatterOption func(*ScatterOrchestrator)

// NewScatterOrchestrator creates a ScatterOrchestrator with the given agent, working
// directory, and options. Defaults: concurrency=3, maxRetries=3.
func NewScatterOrchestrator(a agent.Agent, workDir string, opts ...ScatterOption) *ScatterOrchestrator {
	s := &ScatterOrchestrator{
		agent:       a,
		workDir:     workDir,
		concurrency: 3,
		maxRetries:  3,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// WithScatterMaxRetries sets the maximum number of retry attempts per epic worker.
func WithScatterMaxRetries(n int) ScatterOption {
	return func(s *ScatterOrchestrator) {
		s.maxRetries = n
	}
}

// WithScatterLogger sets the structured logger on the ScatterOrchestrator.
func WithScatterLogger(l *log.Logger) ScatterOption {
	return func(s *ScatterOrchestrator) {
		s.logger = l
	}
}

// WithScatterEvents sets the event channel for progress tracking.
// Events are sent non-blocking; if the channel is full the event is dropped.
func WithScatterEvents(ch chan<- ScatterEvent) ScatterOption {
	return func(s *ScatterOrchestrator) {
		s.events = ch
	}
}

// WithConcurrency sets the maximum number of epic workers running concurrently.
func WithConcurrency(n int) ScatterOption {
	return func(s *ScatterOrchestrator) {
		s.concurrency = n
	}
}

// WithRateLimiter sets the shared rate-limit coordinator used by all workers.
func WithRateLimiter(rl *agent.RateLimitCoordinator) ScatterOption {
	return func(s *ScatterOrchestrator) {
		s.rateLimiter = rl
	}
}

// reUnsafeEpicIDChars matches characters that are not safe for use in file paths.
var reUnsafeEpicIDChars = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// sanitizeEpicID removes any characters from epicID that could be used for
// path traversal or shell injection, leaving only [A-Za-z0-9_-].
func sanitizeEpicID(epicID string) string {
	return reUnsafeEpicIDChars.ReplaceAllString(epicID, "")
}

// epicFilePath returns the absolute path to the output file for the given epic,
// verifying that it is contained within workDir.
func (s *ScatterOrchestrator) epicFilePath(epicID string) (string, error) {
	safe := sanitizeEpicID(epicID)
	if safe == "" {
		return "", fmt.Errorf("epic ID %q sanitizes to empty string; cannot derive safe file path", epicID)
	}
	p := filepath.Join(s.workDir, "epic-"+safe+".json")
	// Verify the path is inside workDir (defence-in-depth).
	rel, err := filepath.Rel(s.workDir, p)
	if err != nil || rel != filepath.Base(rel) {
		return "", fmt.Errorf("derived path %q is outside workDir %q", p, s.workDir)
	}
	return p, nil
}

// Scatter spawns one worker goroutine per epic in opts.Breakdown, with bounded
// concurrency. Workers collect results into shared slices protected by a mutex.
// Worker goroutines only return non-nil errors for fatal conditions (context
// cancellation); validation failures are recorded in ScatterResult.Failures.
//
// Returns (empty ScatterResult, nil) for an empty or nil breakdown.
// Returns partial results alongside the context error on cancellation.
func (s *ScatterOrchestrator) Scatter(ctx context.Context, opts ScatterOpts) (*ScatterResult, error) {
	start := time.Now()

	if opts.Breakdown == nil || len(opts.Breakdown.Epics) == 0 {
		return &ScatterResult{Duration: time.Since(start)}, nil
	}

	epics := opts.Breakdown.Epics

	// Build the list of all known epic IDs for cross-epic dependency validation.
	knownEpicIDs := make([]string, 0, len(epics))
	for _, e := range epics {
		knownEpicIDs = append(knownEpicIDs, e.ID)
	}

	// Mutex-protected result accumulators.
	var mu sync.Mutex
	results := make([]*EpicTaskResult, 0, len(epics))
	failures := make([]ScatterFailure, 0)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(s.concurrency)

	for _, epic := range epics {
		epic := epic // capture loop variable

		g.Go(func() error {
			s.emit(ScatterEvent{
				Type:    ScatterEventWorkerStarted,
				EpicID:  epic.ID,
				Message: fmt.Sprintf("starting worker for epic %s: %s", epic.ID, epic.Title),
				Attempt: 1,
			})
			s.logInfo("worker started", "epic_id", epic.ID, "title", epic.Title)

			etr, err := s.runWithRetry(gctx, epic, opts, knownEpicIDs)
			if err != nil {
				// Distinguish between a validation-exhaustion failure (non-fatal,
				// runWithRetry already emitted worker_failed) and a truly fatal
				// error (context cancelled, rate-limit exhausted, etc.).
				failure := ScatterFailure{EpicID: epic.ID, Err: err}
				if vf, ok := err.(*scatterValidationFailure); ok {
					failure.Errors = vf.errs
					// worker_failed was already emitted inside runWithRetry.
				} else {
					// Fatal error — emit worker_failed now.
					s.emit(ScatterEvent{
						Type:    ScatterEventWorkerFailed,
						EpicID:  epic.ID,
						Message: fmt.Sprintf("worker failed for epic %s: %v", epic.ID, err),
					})
					s.logWarn("worker failed", "epic_id", epic.ID, "error", err)
				}

				mu.Lock()
				failures = append(failures, failure)
				mu.Unlock()

				// Always return nil so sibling workers are not cancelled.
				return nil
			}

			mu.Lock()
			results = append(results, etr)
			mu.Unlock()

			s.emit(ScatterEvent{
				Type:    ScatterEventWorkerCompleted,
				EpicID:  epic.ID,
				Message: fmt.Sprintf("worker completed for epic %s with %d tasks", epic.ID, len(etr.Tasks)),
			})
			s.logInfo("worker completed", "epic_id", epic.ID, "tasks", len(etr.Tasks))

			return nil
		})
	}

	// Wait for all goroutines. Because workers always return nil, g.Wait() only
	// returns non-nil when the errgroup context is cancelled.
	waitErr := g.Wait()

	// Sort results by epic ID for deterministic ordering.
	sort.Slice(results, func(i, j int) bool {
		return results[i].EpicID < results[j].EpicID
	})

	sr := &ScatterResult{
		Results:  results,
		Failures: failures,
		Duration: time.Since(start),
	}

	if waitErr != nil {
		return sr, waitErr
	}

	// If context was cancelled (e.g., workers swallowed the nil but ctx is done),
	// surface the context error alongside partial results.
	if ctx.Err() != nil {
		return sr, ctx.Err()
	}

	return sr, nil
}

// runWithRetry executes the agent for a single epic, retrying on validation
// failure up to maxRetries times. Returns the validated EpicTaskResult or an
// error for fatal conditions (context cancelled, rate-limit exhausted).
func (s *ScatterOrchestrator) runWithRetry(
	ctx context.Context,
	epic Epic,
	opts ScatterOpts,
	knownEpicIDs []string,
) (*EpicTaskResult, error) {
	outputFile, err := s.epicFilePath(epic.ID)
	if err != nil {
		return nil, fmt.Errorf("scatter: deriving output file for epic %s: %w", epic.ID, err)
	}

	var lastValidationErrs []ValidationError

	for attempt := 1; attempt <= s.maxRetries+1; attempt++ {
		// Honor context cancellation before each attempt.
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("scatter: epic %s: context cancelled: %w", epic.ID, err)
		}

		// Check for rate limit before running.
		if s.rateLimiter != nil {
			if state := s.rateLimiter.ShouldWait(s.agent.Name()); state != nil {
				s.emit(ScatterEvent{
					Type:    ScatterEventRateLimited,
					EpicID:  epic.ID,
					Message: fmt.Sprintf("rate limited on epic %s; waiting %s", epic.ID, state.RemainingWait()),
					Attempt: attempt,
				})
				s.logInfo("rate limited; waiting", "epic_id", epic.ID, "remaining", state.RemainingWait())

				if err := s.rateLimiter.WaitForReset(ctx, s.agent.Name()); err != nil {
					return nil, fmt.Errorf("scatter: epic %s: rate limit wait failed: %w", epic.ID, err)
				}
			}
		}

		if attempt > 1 {
			s.emit(ScatterEvent{
				Type:    ScatterEventWorkerRetry,
				EpicID:  epic.ID,
				Message: fmt.Sprintf("retrying epic %s (attempt %d/%d)", epic.ID, attempt, s.maxRetries+1),
				Attempt: attempt,
			})
			s.logInfo("retrying worker", "epic_id", epic.ID, "attempt", attempt, "validation_errors", len(lastValidationErrs))

			// Remove stale output file from previous attempt.
			_ = os.Remove(outputFile)
		}

		// Build prompt.
		validationErrText := ""
		if len(lastValidationErrs) > 0 {
			validationErrText = FormatValidationErrors(lastValidationErrs)
		}

		prompt, err := buildScatterPrompt(scatterPromptData{
			PRDContent:       opts.PRDContent,
			Epic:             epic,
			OtherEpics:       buildOtherEpicsSummary(epic.ID, opts.Breakdown.Epics),
			OutputFile:       outputFile,
			ValidationErrors: validationErrText,
		})
		if err != nil {
			return nil, fmt.Errorf("scatter: epic %s: building prompt (attempt %d): %w", epic.ID, attempt, err)
		}

		// Run agent.
		result, err := s.agent.Run(ctx, agent.RunOpts{
			Prompt:  prompt,
			Model:   opts.Model,
			Effort:  opts.Effort,
			WorkDir: s.workDir,
		})
		if err != nil {
			return nil, fmt.Errorf("scatter: epic %s: agent run (attempt %d): %w", epic.ID, attempt, err)
		}

		s.logDebug("agent run complete",
			"epic_id", epic.ID,
			"attempt", attempt,
			"exit_code", result.ExitCode,
			"stdout_bytes", len(result.Stdout),
			"duration", result.Duration,
		)

		// Check for rate limit in agent output.
		if s.rateLimiter != nil {
			if rlInfo, isLimited := s.agent.ParseRateLimit(result.Stdout); isLimited {
				s.rateLimiter.RecordRateLimit(s.agent.Name(), rlInfo)
				s.emit(ScatterEvent{
					Type:    ScatterEventRateLimited,
					EpicID:  epic.ID,
					Message: fmt.Sprintf("rate limit detected in output for epic %s; will retry", epic.ID),
					Attempt: attempt,
				})
				s.logWarn("rate limit detected in agent output", "epic_id", epic.ID, "attempt", attempt)
				// Rate-limited — retry without counting this as a validation error.
				continue
			}
		}

		// Extract and validate the task result.
		etr, validErrs, extractErr := s.extractTaskResult(outputFile, result.Stdout, knownEpicIDs)
		if extractErr != nil {
			// Cannot parse JSON at all — treat as a validation failure and retry.
			lastValidationErrs = []ValidationError{{
				Field:   "json",
				Message: extractErr.Error(),
			}}
			s.logWarn("failed to extract task result", "epic_id", epic.ID, "attempt", attempt, "error", extractErr)
			continue
		}

		if len(validErrs) == 0 {
			// Success.
			s.logDebug("task result valid",
				"epic_id", epic.ID,
				"attempt", attempt,
				"tasks", len(etr.Tasks),
			)
			return etr, nil
		}

		// Validation failed — prepare for next iteration.
		lastValidationErrs = validErrs
		s.logWarn("task result validation failed",
			"epic_id", epic.ID,
			"attempt", attempt,
			"error_count", len(validErrs),
		)
	}

	// All attempts exhausted — record as a failure (not a fatal error).
	s.emit(ScatterEvent{
		Type:    ScatterEventWorkerFailed,
		EpicID:  epic.ID,
		Message: fmt.Sprintf("epic %s failed after %d attempts; last validation errors: %s", epic.ID, s.maxRetries+1, FormatValidationErrors(lastValidationErrs)),
		Attempt: s.maxRetries + 1,
	})

	// Return a failure by wrapping into the caller's failures slice instead of
	// returning an error (which would cancel sibling workers). We signal this
	// by returning a descriptive non-fatal sentinel wrapped in an error so the
	// caller in Scatter can record it in failures and return nil from the goroutine.
	// To keep runWithRetry's contract clean we return a special ScatterValidationError.
	return nil, &scatterValidationFailure{
		epicID: epic.ID,
		errs:   lastValidationErrs,
	}
}

// scatterValidationFailure is a non-fatal error signalling that all retry
// attempts for an epic were exhausted due to validation failures.
type scatterValidationFailure struct {
	epicID string
	errs   []ValidationError
}

func (e *scatterValidationFailure) Error() string {
	return fmt.Sprintf("scatter: epic %s exhausted retries; last validation errors:\n%s",
		e.epicID, FormatValidationErrors(e.errs))
}

// scatterPromptData holds data injected into the scatter prompt template.
type scatterPromptData struct {
	// PRDContent is the full PRD text for context.
	PRDContent string
	// Epic is the epic being decomposed.
	Epic Epic
	// OtherEpics is a formatted summary of all other epics for cross-referencing.
	OtherEpics string
	// OutputFile is the path where the agent must write the JSON.
	OutputFile string
	// ValidationErrors is a formatted list of validation errors from the previous attempt.
	ValidationErrors string
}

// scatterPromptTemplate is the built-in prompt template for the scatter phase.
// It uses [[ and ]] as delimiters to avoid conflicts with JSON content.
const scatterPromptTemplate = `You are performing Phase 2 of a PRD decomposition workflow.

Your task is to decompose the epic described below into concrete development tasks.

## PRD Context

[[ .PRDContent ]]

## Your Assigned Epic

ID: [[ .Epic.ID ]]
Title: [[ .Epic.Title ]]
Description: [[ .Epic.Description ]]
Estimated Task Count: [[ .Epic.EstimatedTaskCount ]]

## Other Epics (for cross-referencing)

[[ .OtherEpics ]]

## Output Instructions

Write a JSON object to the file: [[ .OutputFile ]]

The JSON must conform exactly to this schema:

{
  "epic_id": "E-001",
  "tasks": [
    {
      "temp_id": "E001-T01",
      "title": "Task Title",
      "description": "What this task implements",
      "acceptance_criteria": ["criterion 1", "criterion 2"],
      "local_dependencies": [],
      "cross_epic_dependencies": [],
      "effort": "medium",
      "priority": "must-have"
    }
  ]
}

## Schema Rules

- epic_id must match the assigned epic ID exactly: [[ .Epic.ID ]]
- temp_id format: ENNN-TNN (no hyphens in numeric part, e.g., E001-T01 for epic E-001)
- effort: must be one of: small, medium, large
- priority: must be one of: must-have, should-have, nice-to-have
- acceptance_criteria must not be empty for each task
- local_dependencies lists temp_ids of other tasks in THIS epic only
- cross_epic_dependencies format: E-NNN:label (e.g., E-002:database-schema)
[[ if .ValidationErrors ]]
## Validation Errors from Previous Attempt

The previous JSON output contained the following errors that you must fix:

[[ .ValidationErrors ]]
[[ end ]]
## Instructions

1. Decompose the epic into [[ .Epic.EstimatedTaskCount ]] or more concrete, implementable tasks
2. Order tasks so dependencies come first (tasks with no local_dependencies first)
3. Write the JSON object to the output file path specified above
4. Output only a JSON object -- do not include any prose before or after it in the file
`

// parsedScatterTemplate is the parsed scatter prompt template, initialized once.
var parsedScatterTemplate = func() *template.Template {
	tmpl, err := template.New("scatter").Delims("[[", "]]").Parse(scatterPromptTemplate)
	if err != nil {
		// The template is a package-level constant; a parse error is a programming bug.
		panic(fmt.Sprintf("prd: failed to parse scatter prompt template: %v", err))
	}
	return tmpl
}()

// buildScatterPrompt renders the scatter prompt template with the provided data.
func buildScatterPrompt(data scatterPromptData) (string, error) {
	var buf bytes.Buffer
	if err := parsedScatterTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing scatter prompt template: %w", err)
	}
	return buf.String(), nil
}

// buildOtherEpicsSummary formats a bulleted list of epics excluding the one
// identified by currentEpicID. Format: "- E-001: Title -- Description".
func buildOtherEpicsSummary(currentEpicID string, epics []Epic) string {
	var buf bytes.Buffer
	for _, e := range epics {
		if e.ID == currentEpicID {
			continue
		}
		fmt.Fprintf(&buf, "- %s: %s -- %s\n", e.ID, e.Title, e.Description)
	}
	return buf.String()
}

// extractTaskResult attempts to parse an EpicTaskResult, first from the output
// file (if it exists and is non-empty), then from stdout via jsonutil.
// Returns the parsed result, any validation errors, and any fatal parse error.
func (s *ScatterOrchestrator) extractTaskResult(
	outputFile, stdout string,
	knownEpicIDs []string,
) (*EpicTaskResult, []ValidationError, error) {
	// Try the output file first.
	if data, err := os.ReadFile(outputFile); err == nil && len(data) > 0 {
		etr, validErrs, parseErr := ParseEpicTaskResult(data, knownEpicIDs)
		if parseErr == nil {
			s.logDebug("extracted task result from output file", "file", outputFile)
			return etr, validErrs, nil
		}
		s.logDebug("output file present but not parseable; falling back to stdout", "parse_err", parseErr)
	}

	// Fall back to extracting from stdout.
	var etr EpicTaskResult
	if err := jsonutil.ExtractInto(stdout, &etr); err != nil {
		return nil, nil, fmt.Errorf("no valid EpicTaskResult JSON found in agent output: %w", err)
	}

	// Re-marshal and parse through ParseEpicTaskResult for uniform validation.
	data, err := json.Marshal(&etr)
	if err != nil {
		return nil, nil, fmt.Errorf("re-marshalling extracted task result: %w", err)
	}
	result, validErrs, parseErr := ParseEpicTaskResult(data, knownEpicIDs)
	if parseErr != nil {
		return nil, nil, fmt.Errorf("parsing extracted task result: %w", parseErr)
	}
	s.logDebug("extracted task result from stdout")
	return result, validErrs, nil
}

// emit sends a ScatterEvent to the events channel in a non-blocking manner.
// If events is nil or the channel is full, the event is silently dropped.
func (s *ScatterOrchestrator) emit(evt ScatterEvent) {
	if s.events == nil {
		return
	}
	select {
	case s.events <- evt:
	default:
	}
}

// logInfo logs at Info level if a logger is configured.
func (s *ScatterOrchestrator) logInfo(msg string, keyvals ...any) {
	if s.logger != nil {
		s.logger.Info(msg, keyvals...)
	}
}

// logDebug logs at Debug level if a logger is configured.
func (s *ScatterOrchestrator) logDebug(msg string, keyvals ...any) {
	if s.logger != nil {
		s.logger.Debug(msg, keyvals...)
	}
}

// logWarn logs at Warn level if a logger is configured.
func (s *ScatterOrchestrator) logWarn(msg string, keyvals ...any) {
	if s.logger != nil {
		s.logger.Warn(msg, keyvals...)
	}
}
