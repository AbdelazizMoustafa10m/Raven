package prd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/charmbracelet/log"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
	"github.com/AbdelazizMoustafa10m/Raven/internal/jsonutil"
)

// maxPRDSize is the maximum allowed size for a PRD file (1 MB).
const maxPRDSize = 1 * 1024 * 1024

// ShredEventType identifies the kind of shred event emitted during processing.
type ShredEventType string

const (
	// ShredEventStarted is emitted when the shred operation begins.
	ShredEventStarted ShredEventType = "shred_started"
	// ShredEventCompleted is emitted when the shred operation succeeds.
	ShredEventCompleted ShredEventType = "shred_completed"
	// ShredEventRetry is emitted before each retry attempt.
	ShredEventRetry ShredEventType = "shred_retry"
	// ShredEventFailed is emitted when all retry attempts are exhausted.
	ShredEventFailed ShredEventType = "shred_failed"
)

// ShredEvent is emitted during the shred process for progress tracking.
type ShredEvent struct {
	// Type identifies the kind of event.
	Type ShredEventType
	// Message is a human-readable description of the event.
	Message string
	// Attempt is the 1-based attempt number associated with this event.
	Attempt int
	// Errors holds the validation errors that triggered a retry event.
	Errors []ValidationError
}

// ShredOpts specifies the parameters for a single Shred call.
type ShredOpts struct {
	// PRDPath is the path to the PRD markdown file.
	PRDPath string
	// OutputFile is the path where the epic-breakdown JSON will be written.
	// If empty, defaults to filepath.Join(workDir, "epic-breakdown.json").
	OutputFile string
	// Model is an optional model override for the agent invocation.
	Model string
	// Effort is an optional effort-level override for the agent invocation.
	Effort string
}

// ShredResult contains the validated EpicBreakdown and metadata.
type ShredResult struct {
	// Breakdown is the validated epic breakdown produced by the agent.
	Breakdown *EpicBreakdown
	// Duration is the total wall-clock time spent in Shred (including retries).
	Duration time.Duration
	// Retries is the number of retry attempts; 0 means the first try succeeded.
	Retries int
	// OutputFile is the absolute path where the JSON was written.
	OutputFile string
}

// ShredderOption is a functional option for configuring a Shredder.
type ShredderOption func(*Shredder)

// Shredder orchestrates the single-agent PRD-to-epics call.
// It reads a PRD file, sends it to an AI agent with a structured prompt,
// validates the resulting EpicBreakdown JSON, and retries on validation errors.
type Shredder struct {
	agent      agent.Agent
	workDir    string
	maxRetries int
	logger     *log.Logger
	events     chan<- ShredEvent
}

// NewShredder creates a Shredder with the given agent, working directory, and options.
// The default maxRetries is 3. Pass functional options to customize behavior.
func NewShredder(a agent.Agent, workDir string, opts ...ShredderOption) *Shredder {
	s := &Shredder{
		agent:      a,
		workDir:    workDir,
		maxRetries: 3,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// WithMaxRetries sets the maximum number of retry attempts on the Shredder.
func WithMaxRetries(n int) ShredderOption {
	return func(s *Shredder) {
		s.maxRetries = n
	}
}

// WithLogger sets the structured logger on the Shredder.
func WithLogger(l *log.Logger) ShredderOption {
	return func(s *Shredder) {
		s.logger = l
	}
}

// WithEvents sets the event channel for progress tracking.
// Events are sent non-blocking; if the channel is full the event is dropped.
func WithEvents(ch chan<- ShredEvent) ShredderOption {
	return func(s *Shredder) {
		s.events = ch
	}
}

// shredPromptTemplate is the built-in prompt template for the shred phase.
// It uses [[ and ]] as delimiters to avoid conflicts with JSON content.
const shredPromptTemplate = `You are performing Phase 1 of a PRD decomposition workflow.

Your task is to analyze the PRD (Product Requirements Document) provided below and
produce a structured JSON breakdown of the high-level epics.

## PRD Content

[[ .PRDContent ]]

## Output Instructions

Write a JSON object to the file: [[ .OutputFile ]]

The JSON must conform exactly to this schema:

{
  "epics": [
    {
      "id": "E-001",
      "title": "Foundation & Setup",
      "description": "Core scaffolding, module setup, and project structure",
      "prd_sections": ["Section 1", "Section 2"],
      "estimated_task_count": 8,
      "dependencies_on_epics": []
    }
  ]
}

## Schema Rules

- "epics" must be a non-empty array
- Each epic "id" must use the format E-NNN (e.g., E-001, E-002)
- "title" and "description" must not be empty
- "prd_sections" lists the PRD section headings covered by this epic
- "estimated_task_count" must be >= 0
- "dependencies_on_epics" lists epic IDs that must be completed before this epic
- No self-dependencies; dependency IDs must reference other epics in the same list
- Produce between 3 and 10 epics for a typical PRD
[[ if .ValidationErrors ]]
## Validation Errors from Previous Attempt

The previous JSON output contained the following validation errors that you must fix:

[[ .ValidationErrors ]]
[[ end ]]
## Instructions

1. Read and understand the entire PRD
2. Identify high-level epics (groups of related features or development areas)
3. Order epics so that dependencies come first
4. Write the JSON object to the output file path specified above
5. Output only a JSON object -- do not include any prose before or after it in the file
`

// shredPromptData holds the data injected into the shred prompt template.
type shredPromptData struct {
	// PRDContent is the full text content of the PRD file.
	PRDContent string
	// OutputFile is the path where the agent must write the JSON.
	OutputFile string
	// ValidationErrors is a formatted list of validation errors from the previous attempt.
	// Empty string on the first attempt.
	ValidationErrors string
}

// parsedShredTemplate is the parsed shred prompt template, initialized once.
var parsedShredTemplate = func() *template.Template {
	tmpl, err := template.New("shred").Delims("[[", "]]").Parse(shredPromptTemplate)
	if err != nil {
		// The template is a package-level constant; a parse error is a programming bug.
		panic(fmt.Sprintf("prd: failed to parse shred prompt template: %v", err))
	}
	return tmpl
}()

// Shred reads the PRD file, invokes the agent to produce epic JSON, validates it,
// and returns the EpicBreakdown. It retries up to maxRetries times on validation failure.
// Context cancellation is honored at the start of each retry iteration.
func (s *Shredder) Shred(ctx context.Context, opts ShredOpts) (*ShredResult, error) {
	start := time.Now()

	outputFile := opts.OutputFile
	if outputFile == "" {
		outputFile = filepath.Join(s.workDir, "epic-breakdown.json")
	}

	// Read and validate the PRD file.
	prdContent, err := s.readPRD(opts.PRDPath)
	if err != nil {
		return nil, fmt.Errorf("shred: reading PRD file: %w", err)
	}

	s.emit(ShredEvent{
		Type:    ShredEventStarted,
		Message: fmt.Sprintf("starting shred of %s", opts.PRDPath),
		Attempt: 1,
	})

	s.logInfo("starting shred",
		"prd_path", opts.PRDPath,
		"output_file", outputFile,
		"max_retries", s.maxRetries,
	)

	var lastValidationErrs []ValidationError
	retries := 0

	for attempt := 1; attempt <= s.maxRetries+1; attempt++ {
		// Honor context cancellation before each attempt.
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("shred: context cancelled: %w", err)
		}

		if attempt > 1 {
			retries++
			s.emit(ShredEvent{
				Type:    ShredEventRetry,
				Message: fmt.Sprintf("retrying shred (attempt %d/%d)", attempt, s.maxRetries+1),
				Attempt: attempt,
				Errors:  lastValidationErrs,
			})
			s.logInfo("retrying shred",
				"attempt", attempt,
				"validation_errors", len(lastValidationErrs),
			)
		}

		// Build the prompt for this attempt.
		validationErrText := ""
		if len(lastValidationErrs) > 0 {
			validationErrText = FormatValidationErrors(lastValidationErrs)
		}

		prompt, err := s.buildPrompt(shredPromptData{
			PRDContent:       prdContent,
			OutputFile:       outputFile,
			ValidationErrors: validationErrText,
		})
		if err != nil {
			return nil, fmt.Errorf("shred: building prompt for attempt %d: %w", attempt, err)
		}

		// Remove any stale output file from a previous attempt so we can detect
		// whether the agent wrote a fresh result.
		if attempt > 1 {
			_ = os.Remove(outputFile)
		}

		// Invoke the agent.
		result, err := s.agent.Run(ctx, agent.RunOpts{
			Prompt:  prompt,
			Model:   opts.Model,
			Effort:  opts.Effort,
			WorkDir: s.workDir,
		})
		if err != nil {
			return nil, fmt.Errorf("shred: agent run (attempt %d): %w", attempt, err)
		}

		s.logDebug("agent run complete",
			"attempt", attempt,
			"exit_code", result.ExitCode,
			"stdout_bytes", len(result.Stdout),
			"duration", result.Duration,
		)

		// Extract the EpicBreakdown: first try the output file, then stdout.
		breakdown, validErrs, extractErr := s.extractBreakdown(outputFile, result.Stdout)
		if extractErr != nil {
			// Cannot parse JSON at all; treat as validation failure and retry.
			lastValidationErrs = []ValidationError{{
				Field:   "json",
				Message: extractErr.Error(),
			}}
			continue
		}

		if len(validErrs) == 0 {
			// Success.
			duration := time.Since(start)
			s.emit(ShredEvent{
				Type:    ShredEventCompleted,
				Message: fmt.Sprintf("shred completed successfully (attempt %d)", attempt),
				Attempt: attempt,
			})
			s.logInfo("shred completed",
				"attempt", attempt,
				"epics", len(breakdown.Epics),
				"retries", retries,
				"duration", duration,
			)
			return &ShredResult{
				Breakdown:  breakdown,
				Duration:   duration,
				Retries:    retries,
				OutputFile: outputFile,
			}, nil
		}

		// Validation failed -- prepare for retry.
		lastValidationErrs = validErrs
		s.logWarn("shred validation failed",
			"attempt", attempt,
			"error_count", len(validErrs),
		)
	}

	// All attempts exhausted.
	s.emit(ShredEvent{
		Type:    ShredEventFailed,
		Message: fmt.Sprintf("shred failed after %d attempts", s.maxRetries+1),
		Attempt: s.maxRetries + 1,
		Errors:  lastValidationErrs,
	})

	return nil, fmt.Errorf(
		"shred: exceeded %d retries; last validation errors:\n%s",
		s.maxRetries,
		FormatValidationErrors(lastValidationErrs),
	)
}

// readPRD reads the PRD file and enforces the 1 MB size cap.
func (s *Shredder) readPRD(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read PRD file %q: %w", path, err)
	}
	if len(data) > maxPRDSize {
		return "", fmt.Errorf("PRD file %q is %d bytes; maximum allowed is %d bytes (1 MB)", path, len(data), maxPRDSize)
	}
	return string(data), nil
}

// buildPrompt renders the shred prompt template with the given data.
func (s *Shredder) buildPrompt(data shredPromptData) (string, error) {
	var buf bytes.Buffer
	if err := parsedShredTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing shred prompt template: %w", err)
	}
	return buf.String(), nil
}

// extractBreakdown attempts to parse an EpicBreakdown, first from the output
// file (if it exists and is non-empty) and then from the agent's stdout.
// Returns the parsed breakdown, any validation errors, and any fatal parse error.
func (s *Shredder) extractBreakdown(outputFile, stdout string) (*EpicBreakdown, []ValidationError, error) {
	// Try the output file first.
	if data, err := os.ReadFile(outputFile); err == nil && len(data) > 0 {
		breakdown, validErrs, parseErr := ParseEpicBreakdown(data)
		if parseErr == nil {
			s.logDebug("extracted breakdown from output file", "file", outputFile)
			return breakdown, validErrs, nil
		}
		s.logDebug("output file present but not parseable; falling back to stdout", "parse_err", parseErr)
	}

	// Fall back to extracting from stdout.
	var eb EpicBreakdown
	if err := jsonutil.ExtractInto(stdout, &eb); err != nil {
		return nil, nil, fmt.Errorf("no valid EpicBreakdown JSON found in agent output: %w", err)
	}

	// Re-marshal and parse through ParseEpicBreakdown for uniform validation.
	data, err := json.Marshal(&eb)
	if err != nil {
		return nil, nil, fmt.Errorf("re-marshalling extracted breakdown: %w", err)
	}
	breakdown, validErrs, parseErr := ParseEpicBreakdown(data)
	if parseErr != nil {
		return nil, nil, fmt.Errorf("parsing extracted breakdown: %w", parseErr)
	}
	s.logDebug("extracted breakdown from stdout")
	return breakdown, validErrs, nil
}

// emit sends a ShredEvent to the events channel in a non-blocking manner.
// If events is nil or the channel is full, the event is silently dropped.
func (s *Shredder) emit(evt ShredEvent) {
	if s.events == nil {
		return
	}
	select {
	case s.events <- evt:
	default:
	}
}

// logInfo logs at Info level if a logger is configured.
func (s *Shredder) logInfo(msg string, keyvals ...any) {
	if s.logger != nil {
		s.logger.Info(msg, keyvals...)
	}
}

// logDebug logs at Debug level if a logger is configured.
func (s *Shredder) logDebug(msg string, keyvals ...any) {
	if s.logger != nil {
		s.logger.Debug(msg, keyvals...)
	}
}

// logWarn logs at Warn level if a logger is configured.
func (s *Shredder) logWarn(msg string, keyvals ...any) {
	if s.logger != nil {
		s.logger.Warn(msg, keyvals...)
	}
}
