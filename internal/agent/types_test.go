package agent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunResult_Success(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		want     bool
	}{
		{name: "exit code 0 is success", exitCode: 0, want: true},
		{name: "exit code 1 is failure", exitCode: 1, want: false},
		{name: "exit code 2 is failure", exitCode: 2, want: false},
		{name: "exit code -1 is failure", exitCode: -1, want: false},
		{name: "exit code 127 is failure", exitCode: 127, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := &RunResult{ExitCode: tt.exitCode}
			assert.Equal(t, tt.want, r.Success())
		})
	}
}

func TestRunResult_WasRateLimited(t *testing.T) {
	tests := []struct {
		name      string
		rateLimit *RateLimitInfo
		want      bool
	}{
		{
			name:      "nil rate limit info",
			rateLimit: nil,
			want:      false,
		},
		{
			name:      "rate limit not limited",
			rateLimit: &RateLimitInfo{IsLimited: false},
			want:      false,
		},
		{
			name: "rate limit is limited",
			rateLimit: &RateLimitInfo{
				IsLimited:  true,
				ResetAfter: 30 * time.Second,
				Message:    "too many requests",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := &RunResult{RateLimit: tt.rateLimit}
			assert.Equal(t, tt.want, r.WasRateLimited())
		})
	}
}

func TestRunOpts_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	opts := RunOpts{
		Prompt:       "implement the feature",
		PromptFile:   "/tmp/prompt.md",
		Model:        "claude-sonnet-4-20250514",
		Effort:       "high",
		AllowedTools: "bash,edit",
		OutputFormat: "json",
		WorkDir:      "/tmp/project",
		Env:          []string{"FOO=bar", "BAZ=qux"},
	}

	data, err := json.Marshal(opts)
	require.NoError(t, err)

	var decoded RunOpts
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, opts, decoded)
}

func TestRunOpts_OmitEmpty(t *testing.T) {
	t.Parallel()

	// Zero-value RunOpts should produce a minimal JSON with no fields.
	opts := RunOpts{}
	data, err := json.Marshal(opts)
	require.NoError(t, err)

	assert.Equal(t, `{}`, string(data))
}

func TestRunResult_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	result := RunResult{
		Stdout:   "hello world",
		Stderr:   "warning: something",
		ExitCode: 0,
		Duration: 5 * time.Second,
		RateLimit: &RateLimitInfo{
			IsLimited:  true,
			ResetAfter: 60 * time.Second,
			Message:    "rate limited",
		},
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var decoded RunResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, result.Stdout, decoded.Stdout)
	assert.Equal(t, result.Stderr, decoded.Stderr)
	assert.Equal(t, result.ExitCode, decoded.ExitCode)
	assert.Equal(t, result.Duration, decoded.Duration)
	require.NotNil(t, decoded.RateLimit)
	assert.Equal(t, result.RateLimit.IsLimited, decoded.RateLimit.IsLimited)
	assert.Equal(t, result.RateLimit.ResetAfter, decoded.RateLimit.ResetAfter)
	assert.Equal(t, result.RateLimit.Message, decoded.RateLimit.Message)
}

func TestRunResult_RateLimitOmittedWhenNil(t *testing.T) {
	t.Parallel()

	result := RunResult{
		Stdout:   "output",
		ExitCode: 0,
		Duration: time.Second,
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(data), `"rate_limit"`)
}

func TestRunOpts_JSONStructTags(t *testing.T) {
	t.Parallel()

	opts := RunOpts{
		Prompt:       "test",
		PromptFile:   "file.md",
		Model:        "model",
		Effort:       "high",
		AllowedTools: "bash",
		OutputFormat: "json",
		WorkDir:      "/tmp",
		Env:          []string{"A=B"},
	}

	data, err := json.Marshal(opts)
	require.NoError(t, err)

	raw := string(data)
	assert.Contains(t, raw, `"prompt"`)
	assert.Contains(t, raw, `"prompt_file"`)
	assert.Contains(t, raw, `"model"`)
	assert.Contains(t, raw, `"effort"`)
	assert.Contains(t, raw, `"allowed_tools"`)
	assert.Contains(t, raw, `"output_format"`)
	assert.Contains(t, raw, `"work_dir"`)
	assert.Contains(t, raw, `"env"`)
}

func TestRunResult_JSONStructTags(t *testing.T) {
	t.Parallel()

	result := RunResult{
		Stdout:   "out",
		Stderr:   "err",
		ExitCode: 1,
		Duration: time.Second,
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	raw := string(data)
	assert.Contains(t, raw, `"stdout"`)
	assert.Contains(t, raw, `"stderr"`)
	assert.Contains(t, raw, `"exit_code"`)
	assert.Contains(t, raw, `"duration"`)
}

func TestRateLimitInfo_JSONStructTags(t *testing.T) {
	t.Parallel()

	info := RateLimitInfo{
		IsLimited:  true,
		ResetAfter: 30 * time.Second,
		Message:    "limited",
	}

	data, err := json.Marshal(info)
	require.NoError(t, err)

	raw := string(data)
	assert.Contains(t, raw, `"is_limited"`)
	assert.Contains(t, raw, `"reset_after"`)
	assert.Contains(t, raw, `"message"`)
}

func TestDuration_SerializesAsNanoseconds(t *testing.T) {
	t.Parallel()

	result := RunResult{
		Duration: 3 * time.Second,
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var raw map[string]any
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	// 3 seconds = 3,000,000,000 nanoseconds.
	assert.Equal(t, float64(3*time.Second), raw["duration"])
}
