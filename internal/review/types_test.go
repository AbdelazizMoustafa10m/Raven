package review

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -- Finding JSON round-trip --------------------------------------------------

func TestFinding_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	f := Finding{
		Severity:    SeverityHigh,
		Category:    "security",
		File:        "internal/auth/token.go",
		Line:        42,
		Description: "token stored in plain text",
		Suggestion:  "use encrypted storage",
		Agent:       "claude",
	}

	data, err := json.Marshal(f)
	require.NoError(t, err)

	var decoded Finding
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, f, decoded)
}

func TestFinding_JSONStructTags(t *testing.T) {
	t.Parallel()

	f := Finding{
		Severity:    SeverityMedium,
		Category:    "style",
		File:        "main.go",
		Line:        1,
		Description: "missing doc comment",
		Suggestion:  "add doc comment",
		Agent:       "codex",
	}

	data, err := json.Marshal(f)
	require.NoError(t, err)

	raw := string(data)
	assert.Contains(t, raw, `"severity"`)
	assert.Contains(t, raw, `"category"`)
	assert.Contains(t, raw, `"file"`)
	assert.Contains(t, raw, `"line"`)
	assert.Contains(t, raw, `"description"`)
	assert.Contains(t, raw, `"suggestion"`)
	assert.Contains(t, raw, `"agent"`)
}

func TestFinding_AgentOmittedWhenEmpty(t *testing.T) {
	t.Parallel()

	// Agent field uses omitempty -- raw agent output has no agent field.
	f := Finding{
		Severity:    SeverityLow,
		Category:    "perf",
		File:        "worker.go",
		Line:        10,
		Description: "unnecessary allocation",
		Suggestion:  "reuse buffer",
	}

	data, err := json.Marshal(f)
	require.NoError(t, err)

	assert.NotContains(t, string(data), `"agent"`)
}

func TestFinding_AgentIncludedWhenNonEmpty(t *testing.T) {
	t.Parallel()

	f := Finding{
		Severity: SeverityInfo,
		Category: "docs",
		File:     "main.go",
		Line:     1,
		Agent:    "gemini",
	}

	data, err := json.Marshal(f)
	require.NoError(t, err)

	raw := string(data)
	assert.Contains(t, raw, `"agent"`)
	assert.Contains(t, raw, `"gemini"`)
}

// TestFinding_JSONRoundTrip_Variants exercises edge-case field values to ensure
// JSON serialisation is stable for all combinations of Finding fields.
func TestFinding_JSONRoundTrip_Variants(t *testing.T) {
	t.Parallel()

	longStr := strings.Repeat("x", 1024)

	tests := []struct {
		name    string
		finding Finding
	}{
		{
			name: "empty suggestion",
			finding: Finding{
				Severity:    SeverityCritical,
				Category:    "security",
				File:        "auth.go",
				Line:        7,
				Description: "hardcoded secret",
				Suggestion:  "",
			},
		},
		{
			name: "line zero -- file-level finding",
			finding: Finding{
				Severity:    SeverityInfo,
				Category:    "docs",
				File:        "README.md",
				Line:        0,
				Description: "missing README",
				Suggestion:  "add README",
			},
		},
		{
			name: "very long description and suggestion",
			finding: Finding{
				Severity:    SeverityMedium,
				Category:    "style",
				File:        "big.go",
				Line:        500,
				Description: longStr,
				Suggestion:  longStr,
				Agent:       "claude",
			},
		},
		{
			name: "category with spaces",
			finding: Finding{
				Severity:    SeverityLow,
				Category:    "code style",
				File:        "util.go",
				Line:        3,
				Description: "spacing issue",
				Suggestion:  "run gofmt",
			},
		},
		{
			name: "category with slashes",
			finding: Finding{
				Severity:    SeverityHigh,
				Category:    "security/injection",
				File:        "handler.go",
				Line:        88,
				Description: "SQL injection",
				Suggestion:  "use parameterised queries",
			},
		},
		{
			name: "category with colons",
			finding: Finding{
				Severity:    SeverityMedium,
				Category:    "perf:allocation",
				File:        "hot.go",
				Line:        12,
				Description: "allocation in hot path",
				Suggestion:  "pre-allocate",
			},
		},
		{
			name: "category with unicode",
			finding: Finding{
				Severity:    SeverityInfo,
				Category:    "文档",
				File:        "doc.go",
				Line:        1,
				Description: "缺少注释",
				Suggestion:  "添加注释",
			},
		},
		{
			name: "zero-value line with non-empty agent",
			finding: Finding{
				Severity: SeverityHigh,
				Category: "security",
				File:     "main.go",
				Line:     0,
				Agent:    "codex",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.finding)
			require.NoError(t, err)

			var decoded Finding
			require.NoError(t, json.Unmarshal(data, &decoded))
			assert.Equal(t, tt.finding, decoded)
		})
	}
}

// -- Finding.DeduplicationKey -------------------------------------------------

func TestFinding_DeduplicationKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		finding Finding
		want    string
	}{
		{
			name: "standard file:line:category",
			finding: Finding{
				File:     "internal/auth/token.go",
				Line:     42,
				Category: "security",
			},
			want: "internal/auth/token.go:42:security",
		},
		{
			name: "spec example -- main.go line 42 security",
			finding: Finding{
				File:     "main.go",
				Line:     42,
				Category: "security",
			},
			want: "main.go:42:security",
		},
		{
			name: "line zero",
			finding: Finding{
				File:     "main.go",
				Line:     0,
				Category: "style",
			},
			want: "main.go:0:style",
		},
		{
			name: "line zero -- file-level finding with security category",
			finding: Finding{
				File:     "main.go",
				Line:     0,
				Category: "security",
			},
			want: "main.go:0:security",
		},
		{
			name: "empty category",
			finding: Finding{
				File:     "main.go",
				Line:     42,
				Category: "",
			},
			want: "main.go:42:",
		},
		{
			name: "empty file and category",
			finding: Finding{
				File:     "",
				Line:     5,
				Category: "",
			},
			want: ":5:",
		},
		{
			name: "deep nested path",
			finding: Finding{
				File:     "internal/review/runner.go",
				Line:     100,
				Category: "complexity",
			},
			want: "internal/review/runner.go:100:complexity",
		},
		{
			name: "category with spaces",
			finding: Finding{
				File:     "main.go",
				Line:     1,
				Category: "code style",
			},
			want: "main.go:1:code style",
		},
		{
			name: "category with forward slash",
			finding: Finding{
				File:     "handler.go",
				Line:     10,
				Category: "security/injection",
			},
			want: "handler.go:10:security/injection",
		},
		{
			name: "category with colon",
			finding: Finding{
				File:     "hot.go",
				Line:     3,
				Category: "perf:allocation",
			},
			want: "hot.go:3:perf:allocation",
		},
		{
			name: "category with unicode",
			finding: Finding{
				File:     "doc.go",
				Line:     1,
				Category: "文档",
			},
			want: "doc.go:1:文档",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.finding.DeduplicationKey())
		})
	}
}

// -- Verdict constants --------------------------------------------------------

func TestVerdictConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Verdict("APPROVED"), VerdictApproved)
	assert.Equal(t, Verdict("CHANGES_NEEDED"), VerdictChangesNeeded)
	assert.Equal(t, Verdict("BLOCKING"), VerdictBlocking)
}

// TestVerdictBlocking_IsMostSevere verifies the string value of the most severe
// verdict constant matches the PRD specification.
func TestVerdictBlocking_IsMostSevere(t *testing.T) {
	t.Parallel()

	// VerdictBlocking must equal "BLOCKING" as defined in the PRD.
	assert.Equal(t, Verdict("BLOCKING"), VerdictBlocking)

	// Verify that blocking is distinct from the other two less-severe verdicts.
	assert.NotEqual(t, VerdictBlocking, VerdictApproved)
	assert.NotEqual(t, VerdictBlocking, VerdictChangesNeeded)
}

// -- Severity constants -------------------------------------------------------

func TestSeverityConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Severity("info"), SeverityInfo)
	assert.Equal(t, Severity("low"), SeverityLow)
	assert.Equal(t, Severity("medium"), SeverityMedium)
	assert.Equal(t, Severity("high"), SeverityHigh)
	assert.Equal(t, Severity("critical"), SeverityCritical)
}

// -- ReviewResult.Validate ----------------------------------------------------

func TestReviewResult_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rr      *ReviewResult
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil ReviewResult returns nil",
			rr:      nil,
			wantErr: false,
		},
		{
			name: "empty findings with APPROVED verdict is valid",
			rr: &ReviewResult{
				Findings: []Finding{},
				Verdict:  VerdictApproved,
			},
			wantErr: false,
		},
		{
			name: "nil findings with CHANGES_NEEDED verdict is valid",
			rr: &ReviewResult{
				Findings: nil,
				Verdict:  VerdictChangesNeeded,
			},
			wantErr: false,
		},
		{
			name: "valid findings and BLOCKING verdict",
			rr: &ReviewResult{
				Findings: []Finding{
					{Severity: SeverityCritical, Category: "security", File: "main.go", Line: 1},
					{Severity: SeverityHigh, Category: "logic", File: "util.go", Line: 5},
				},
				Verdict: VerdictBlocking,
			},
			wantErr: false,
		},
		{
			name: "invalid verdict rejected",
			rr: &ReviewResult{
				Findings: []Finding{},
				Verdict:  Verdict("approved"), // lowercase -- not a valid constant
			},
			wantErr: true,
			errMsg:  "invalid verdict",
		},
		{
			name: "empty verdict rejected",
			rr: &ReviewResult{
				Findings: []Finding{},
				Verdict:  Verdict(""),
			},
			wantErr: true,
			errMsg:  "invalid verdict",
		},
		{
			name: "unknown verdict rejected",
			rr: &ReviewResult{
				Findings: []Finding{},
				Verdict:  Verdict("LGTM"),
			},
			wantErr: true,
			errMsg:  "invalid verdict",
		},
		{
			name: "finding with invalid severity rejected",
			rr: &ReviewResult{
				Findings: []Finding{
					{Severity: SeverityLow, Category: "style", File: "a.go", Line: 1},
					{Severity: Severity("CRITICAL"), Category: "security", File: "b.go", Line: 2}, // uppercase -- invalid
				},
				Verdict: VerdictApproved,
			},
			wantErr: true,
			errMsg:  "invalid severity",
		},
		{
			name: "finding with empty severity rejected",
			rr: &ReviewResult{
				Findings: []Finding{
					{Severity: Severity(""), Category: "style", File: "a.go", Line: 1},
				},
				Verdict: VerdictApproved,
			},
			wantErr: true,
			errMsg:  "invalid severity",
		},
		{
			name: "all valid severities accepted",
			rr: &ReviewResult{
				Findings: []Finding{
					{Severity: SeverityInfo, Category: "docs", File: "a.go", Line: 1},
					{Severity: SeverityLow, Category: "style", File: "b.go", Line: 2},
					{Severity: SeverityMedium, Category: "perf", File: "c.go", Line: 3},
					{Severity: SeverityHigh, Category: "logic", File: "d.go", Line: 4},
					{Severity: SeverityCritical, Category: "security", File: "e.go", Line: 5},
				},
				Verdict: VerdictChangesNeeded,
			},
			wantErr: false,
		},
		{
			name: "first finding invalid severity -- error references finding[0]",
			rr: &ReviewResult{
				Findings: []Finding{
					{Severity: Severity("unknown"), Category: "misc", File: "x.go", Line: 1},
				},
				Verdict: VerdictApproved,
			},
			wantErr: true,
			errMsg:  "finding[0]",
		},
		{
			name: "second finding invalid severity -- error references finding[1]",
			rr: &ReviewResult{
				Findings: []Finding{
					{Severity: SeverityInfo, Category: "docs", File: "a.go", Line: 1},
					{Severity: Severity("bad"), Category: "misc", File: "b.go", Line: 2},
				},
				Verdict: VerdictApproved,
			},
			wantErr: true,
			errMsg:  "finding[1]",
		},
		{
			name: "mixed valid and invalid findings -- first valid, second invalid",
			rr: &ReviewResult{
				Findings: []Finding{
					{Severity: SeverityHigh, Category: "security", File: "a.go", Line: 1},
					{Severity: Severity("blocker"), Category: "logic", File: "b.go", Line: 2},
				},
				Verdict: VerdictChangesNeeded,
			},
			wantErr: true,
			errMsg:  "invalid severity",
		},
		{
			name: "verdict error takes priority over severity error -- verdict checked first",
			rr: &ReviewResult{
				Findings: []Finding{
					{Severity: Severity("bogus"), Category: "misc", File: "z.go", Line: 9},
				},
				Verdict: Verdict("UNKNOWN"),
			},
			wantErr: true,
			errMsg:  "invalid verdict",
		},
		{
			name: "single finding with line zero is valid",
			rr: &ReviewResult{
				Findings: []Finding{
					{Severity: SeverityInfo, Category: "docs", File: "README.md", Line: 0},
				},
				Verdict: VerdictApproved,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.rr.Validate()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

// -- ReviewResult JSON round-trip ---------------------------------------------

func TestReviewResult_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	rr := ReviewResult{
		Findings: []Finding{
			{
				Severity:    SeverityCritical,
				Category:    "security",
				File:        "internal/auth/token.go",
				Line:        99,
				Description: "secret exposed",
				Suggestion:  "use vault",
				Agent:       "claude",
			},
		},
		Verdict: VerdictBlocking,
	}

	data, err := json.Marshal(rr)
	require.NoError(t, err)

	var decoded ReviewResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, rr, decoded)
}

// -- ReviewMode constants -----------------------------------------------------

func TestReviewModeConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ReviewMode("all"), ReviewModeAll)
	assert.Equal(t, ReviewMode("split"), ReviewModeSplit)
}

// -- Integration: JSON array of findings (PRD Section 5.5 format) -------------

// TestFinding_JSONArray_Integration verifies that a JSON array of findings, as
// produced by a reviewing agent according to PRD Section 5.5, can be decoded
// into a slice of Finding structs with all fields intact.
func TestFinding_JSONArray_Integration(t *testing.T) {
	t.Parallel()

	// This JSON mimics the format an agent emits: no "agent" field in each
	// finding (populated later during consolidation).
	const rawJSON = `[
		{
			"severity": "critical",
			"category": "security",
			"file": "internal/auth/token.go",
			"line": 42,
			"description": "Token stored in plain text",
			"suggestion": "Use encrypted storage"
		},
		{
			"severity": "high",
			"category": "logic",
			"file": "internal/workflow/engine.go",
			"line": 100,
			"description": "Missing nil check",
			"suggestion": "Add nil guard before dereference"
		},
		{
			"severity": "medium",
			"category": "perf",
			"file": "internal/prd/shredder.go",
			"line": 0,
			"description": "File-level: excessive allocations",
			"suggestion": "Pre-allocate slices"
		},
		{
			"severity": "low",
			"category": "style",
			"file": "cmd/raven/main.go",
			"line": 5,
			"description": "Missing doc comment",
			"suggestion": ""
		},
		{
			"severity": "info",
			"category": "docs",
			"file": "README.md",
			"line": 1,
			"description": "Consider adding examples",
			"suggestion": "Add a quick-start section"
		}
	]`

	var findings []Finding
	require.NoError(t, json.Unmarshal([]byte(rawJSON), &findings))
	require.Len(t, findings, 5)

	// Verify each finding decoded correctly.
	assert.Equal(t, SeverityCritical, findings[0].Severity)
	assert.Equal(t, "security", findings[0].Category)
	assert.Equal(t, "internal/auth/token.go", findings[0].File)
	assert.Equal(t, 42, findings[0].Line)
	assert.Empty(t, findings[0].Agent) // agent absent in raw output

	assert.Equal(t, SeverityHigh, findings[1].Severity)
	assert.Equal(t, 100, findings[1].Line)

	assert.Equal(t, SeverityMedium, findings[2].Severity)
	assert.Equal(t, 0, findings[2].Line) // file-level finding

	assert.Equal(t, SeverityLow, findings[3].Severity)
	assert.Empty(t, findings[3].Suggestion)

	assert.Equal(t, SeverityInfo, findings[4].Severity)
	assert.Equal(t, "README.md", findings[4].File)

	// All findings must pass Validate when wrapped in a ReviewResult.
	rr := &ReviewResult{Findings: findings, Verdict: VerdictChangesNeeded}
	require.NoError(t, rr.Validate())
}

// -- ConsolidatedReview struct -------------------------------------------------

// TestConsolidatedReview_ZeroValue verifies that the zero value of
// ConsolidatedReview is safe to use (no nil-pointer panics on field access).
func TestConsolidatedReview_ZeroValue(t *testing.T) {
	t.Parallel()

	var cr ConsolidatedReview
	assert.Nil(t, cr.Findings)
	assert.Empty(t, cr.AgentResults)
	assert.Zero(t, cr.TotalAgents)
	assert.Zero(t, cr.Duration)
}

// TestConsolidatedReview_Fields verifies that all fields can be set and read
// back correctly, covering the struct layout defined in types.go.
func TestConsolidatedReview_Fields(t *testing.T) {
	t.Parallel()

	f1 := &Finding{Severity: SeverityCritical, Category: "security", File: "a.go", Line: 1}
	f2 := &Finding{Severity: SeverityHigh, Category: "logic", File: "b.go", Line: 2, Agent: "claude"}

	agentResult := AgentReviewResult{
		Agent: "claude",
		Result: &ReviewResult{
			Findings: []Finding{*f1},
			Verdict:  VerdictBlocking,
		},
		Duration:  150 * time.Millisecond,
		RawOutput: `[{"severity":"critical","category":"security","file":"a.go","line":1}]`,
	}

	cr := ConsolidatedReview{
		Findings:     []*Finding{f1, f2},
		Verdict:      VerdictBlocking,
		AgentResults: []AgentReviewResult{agentResult},
		TotalAgents:  1,
		Duration:     200 * time.Millisecond,
	}

	assert.Len(t, cr.Findings, 2)
	assert.Equal(t, VerdictBlocking, cr.Verdict)
	assert.Len(t, cr.AgentResults, 1)
	assert.Equal(t, 1, cr.TotalAgents)
	assert.Equal(t, 200*time.Millisecond, cr.Duration)

	// Verify the embedded AgentReviewResult fields.
	ar := cr.AgentResults[0]
	assert.Equal(t, "claude", ar.Agent)
	assert.NotNil(t, ar.Result)
	assert.Equal(t, 150*time.Millisecond, ar.Duration)
	assert.Nil(t, ar.Err)
	assert.NotEmpty(t, ar.RawOutput)
}

// TestAgentReviewResult_WithError verifies that the Err field is preserved
// when an agent run fails.
func TestAgentReviewResult_WithError(t *testing.T) {
	t.Parallel()

	ar := AgentReviewResult{
		Agent:     "codex",
		Result:    nil,
		Duration:  50 * time.Millisecond,
		Err:       assert.AnError,
		RawOutput: "",
	}

	assert.Equal(t, "codex", ar.Agent)
	assert.Nil(t, ar.Result)
	assert.ErrorIs(t, ar.Err, assert.AnError)
}

// -- ReviewConfig struct -------------------------------------------------------

// TestReviewConfig_ZeroValue verifies that the zero value of ReviewConfig is
// safe (all string fields are empty, which is valid for optional config).
func TestReviewConfig_ZeroValue(t *testing.T) {
	t.Parallel()

	var rc ReviewConfig
	assert.Empty(t, rc.Extensions)
	assert.Empty(t, rc.RiskPatterns)
	assert.Empty(t, rc.PromptsDir)
	assert.Empty(t, rc.RulesDir)
	assert.Empty(t, rc.ProjectBriefFile)
}

// TestReviewConfig_Fields verifies all ReviewConfig fields can be set and
// read back, covering the toml-tagged struct layout.
func TestReviewConfig_Fields(t *testing.T) {
	t.Parallel()

	rc := ReviewConfig{
		Extensions:       ".go,.ts,.py",
		RiskPatterns:     "internal/auth/**,**/crypto/**",
		PromptsDir:       ".raven/prompts",
		RulesDir:         ".raven/rules",
		ProjectBriefFile: "docs/brief.md",
	}

	assert.Equal(t, ".go,.ts,.py", rc.Extensions)
	assert.Equal(t, "internal/auth/**,**/crypto/**", rc.RiskPatterns)
	assert.Equal(t, ".raven/prompts", rc.PromptsDir)
	assert.Equal(t, ".raven/rules", rc.RulesDir)
	assert.Equal(t, "docs/brief.md", rc.ProjectBriefFile)
}

// -- ReviewOpts struct ---------------------------------------------------------

// TestReviewOpts_Fields verifies all ReviewOpts fields can be set and read back.
func TestReviewOpts_Fields(t *testing.T) {
	t.Parallel()

	opts := ReviewOpts{
		Agents:      []string{"claude", "codex"},
		Concurrency: 2,
		Mode:        ReviewModeAll,
		BaseBranch:  "main",
		DryRun:      true,
	}

	assert.Equal(t, []string{"claude", "codex"}, opts.Agents)
	assert.Equal(t, 2, opts.Concurrency)
	assert.Equal(t, ReviewModeAll, opts.Mode)
	assert.Equal(t, "main", opts.BaseBranch)
	assert.True(t, opts.DryRun)
}

// TestReviewOpts_SplitMode verifies ReviewModeSplit is accepted as a Mode value.
func TestReviewOpts_SplitMode(t *testing.T) {
	t.Parallel()

	opts := ReviewOpts{
		Agents:     []string{"claude"},
		Mode:       ReviewModeSplit,
		BaseBranch: "origin/main",
	}

	assert.Equal(t, ReviewModeSplit, opts.Mode)
}

// -- Benchmark: DeduplicationKey ----------------------------------------------

// BenchmarkFinding_DeduplicationKey measures the allocation and CPU cost of
// building a deduplication key from a typical Finding. Run with:
//
//	go test -bench=BenchmarkFinding_DeduplicationKey -benchmem ./internal/review/
func BenchmarkFinding_DeduplicationKey(b *testing.B) {
	f := Finding{
		File:     "internal/auth/token.go",
		Line:     42,
		Category: "security",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.DeduplicationKey()
	}
}

// BenchmarkReviewResult_Validate_LargePayload measures Validate on a result
// with many findings to detect any O(n²) behaviour.
func BenchmarkReviewResult_Validate_LargePayload(b *testing.B) {
	const numFindings = 500
	findings := make([]Finding, numFindings)
	severities := []Severity{SeverityInfo, SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical}
	for i := range findings {
		findings[i] = Finding{
			Severity: severities[i%len(severities)],
			Category: "mixed",
			File:     "file.go",
			Line:     i + 1,
		}
	}
	rr := &ReviewResult{Findings: findings, Verdict: VerdictChangesNeeded}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rr.Validate()
	}
}

// -- Fuzz: DeduplicationKey ---------------------------------------------------

// FuzzFinding_DeduplicationKey verifies that DeduplicationKey never panics and
// always returns a non-nil string for arbitrary File/Category/Line combinations.
func FuzzFinding_DeduplicationKey(f *testing.F) {
	// Seed corpus covering representative cases.
	f.Add("main.go", 0, "security")
	f.Add("internal/auth/token.go", 42, "security")
	f.Add("", 0, "")
	f.Add("file with spaces.go", 1000, "category/with/slashes")
	f.Add("unicode_文件.go", 1, "文档:注释")

	f.Fuzz(func(t *testing.T, file string, line int, category string) {
		finding := Finding{File: file, Line: line, Category: category}
		key := finding.DeduplicationKey()
		// Invariant 1: the key is never empty -- it always contains at least ":N:".
		if key == "" {
			t.Errorf("DeduplicationKey returned empty string for file=%q line=%d category=%q", file, line, category)
		}
		// Invariant 2: the key must contain the file as a prefix.
		if !strings.HasPrefix(key, file) {
			t.Errorf("DeduplicationKey %q does not start with file %q", key, file)
		}
		// Invariant 3: the key must end with the category.
		if !strings.HasSuffix(key, category) {
			t.Errorf("DeduplicationKey %q does not end with category %q", key, category)
		}
	})
}

// FuzzReviewResult_Validate verifies that Validate never panics on arbitrary
// severity and verdict strings.
func FuzzReviewResult_Validate(f *testing.F) {
	// Seed with known-valid and known-invalid values.
	f.Add("APPROVED", "critical")
	f.Add("BLOCKING", "high")
	f.Add("CHANGES_NEEDED", "info")
	f.Add("", "")
	f.Add("approved", "CRITICAL")
	f.Add("LGTM", "blocker")

	f.Fuzz(func(t *testing.T, verdict, severity string) {
		rr := &ReviewResult{
			Verdict: Verdict(verdict),
			Findings: []Finding{
				{Severity: Severity(severity), Category: "fuzz", File: "f.go", Line: 1},
			},
		}
		// Must not panic. Return value can be error or nil.
		_ = rr.Validate()
	})
}
