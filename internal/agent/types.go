package agent

import "time"

// RunOpts specifies options for a single agent invocation.
type RunOpts struct {
	Prompt       string   `json:"prompt,omitempty"`
	PromptFile   string   `json:"prompt_file,omitempty"`
	Model        string   `json:"model,omitempty"`
	Effort       string   `json:"effort,omitempty"`
	AllowedTools string   `json:"allowed_tools,omitempty"`
	OutputFormat string   `json:"output_format,omitempty"`
	WorkDir      string   `json:"work_dir,omitempty"`
	Env          []string `json:"env,omitempty"`
}

// RunResult captures the output of an agent invocation.
// Duration is serialized as nanoseconds (int64) in JSON, which is the
// default Go behavior for time.Duration.
type RunResult struct {
	Stdout    string         `json:"stdout"`
	Stderr    string         `json:"stderr"`
	ExitCode  int            `json:"exit_code"`
	Duration  time.Duration  `json:"duration"`
	RateLimit *RateLimitInfo `json:"rate_limit,omitempty"`
}

// RateLimitInfo describes a detected rate-limit condition.
// ResetAfter is serialized as nanoseconds (int64) in JSON.
type RateLimitInfo struct {
	IsLimited  bool          `json:"is_limited"`
	ResetAfter time.Duration `json:"reset_after"`
	Message    string        `json:"message"`
}

// Success returns true if the agent exited with code 0.
func (r *RunResult) Success() bool {
	return r.ExitCode == 0
}

// WasRateLimited returns true if the result indicates a rate-limit condition.
func (r *RunResult) WasRateLimited() bool {
	return r.RateLimit != nil && r.RateLimit.IsLimited
}
