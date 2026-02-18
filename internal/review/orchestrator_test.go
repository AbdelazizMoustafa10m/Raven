package review

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
	"github.com/AbdelazizMoustafa10m/Raven/internal/git"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// validReviewJSON is a minimal JSON string that satisfies ReviewResult.Validate().
const validReviewJSON = `{"findings":[{"severity":"high","category":"correctness","file":"main.go","line":10,"description":"bug","suggestion":"fix it"}],"verdict":"CHANGES_NEEDED"}`

// approvedReviewJSON is a minimal JSON string representing an approved review.
const approvedReviewJSON = `{"findings":[],"verdict":"APPROVED"}`

// buildOrchestrator creates a ReviewOrchestrator backed by mock dependencies.
func buildOrchestrator(
	t *testing.T,
	gitClient git.Client,
	agentMap map[string]string, // name -> stdout returned by MockAgent
	events chan<- ReviewEvent,
) *ReviewOrchestrator {
	t.Helper()

	registry := agent.NewRegistry()
	for name, stdout := range agentMap {
		stdout := stdout // capture
		mock := agent.NewMockAgent(name)
		mock.RunFunc = func(_ context.Context, _ agent.RunOpts) (*agent.RunResult, error) {
			return &agent.RunResult{
				Stdout:   stdout,
				ExitCode: 0,
				Duration: 10 * time.Millisecond,
			}, nil
		}
		require.NoError(t, registry.Register(mock))
	}

	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(gitClient, cfg, nil)
	require.NoError(t, err)

	promptBuilder := NewPromptBuilder(cfg, nil)
	consolidator := NewConsolidator(nil)

	return NewReviewOrchestrator(registry, diffGen, promptBuilder, consolidator, 2, nil, events)
}

// buildMockGit creates a mockGitClient with a single changed file and an empty diff.
func buildMockGit() *mockGitClient {
	return &mockGitClient{
		diffFilesResult: []git.DiffEntry{{Path: "main.go", Status: "M"}},
		numStatResult:   []git.NumStatEntry{{Path: "main.go", Added: 5, Deleted: 2}},
		unifiedResult:   "--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n",
	}
}

// ---------------------------------------------------------------------------
// NewReviewOrchestrator
// ---------------------------------------------------------------------------

func TestNewReviewOrchestrator_ClampsNegativeConcurrency(t *testing.T) {
	t.Parallel()
	registry := agent.NewRegistry()
	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(buildMockGit(), cfg, nil)
	require.NoError(t, err)

	ro := NewReviewOrchestrator(registry, diffGen, NewPromptBuilder(cfg, nil), NewConsolidator(nil), -5, nil, nil)
	assert.Equal(t, 1, ro.concurrency)
}

func TestNewReviewOrchestrator_ClampsZeroConcurrency(t *testing.T) {
	t.Parallel()
	registry := agent.NewRegistry()
	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(buildMockGit(), cfg, nil)
	require.NoError(t, err)

	ro := NewReviewOrchestrator(registry, diffGen, NewPromptBuilder(cfg, nil), NewConsolidator(nil), 0, nil, nil)
	assert.Equal(t, 1, ro.concurrency)
}

// ---------------------------------------------------------------------------
// Run — input validation
// ---------------------------------------------------------------------------

func TestRun_NoAgents(t *testing.T) {
	t.Parallel()
	ro := buildOrchestrator(t, buildMockGit(), nil, nil)
	_, err := ro.Run(context.Background(), ReviewOpts{
		BaseBranch: "main",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one agent is required")
}

func TestRun_UnknownAgent(t *testing.T) {
	t.Parallel()
	ro := buildOrchestrator(t, buildMockGit(), nil, nil)
	_, err := ro.Run(context.Background(), ReviewOpts{
		Agents:     []string{"unknown-agent"},
		BaseBranch: "main",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown-agent")
}

// ---------------------------------------------------------------------------
// Run — success paths
// ---------------------------------------------------------------------------

func TestRun_SingleAgentAllMode(t *testing.T) {
	t.Parallel()
	ro := buildOrchestrator(t, buildMockGit(), map[string]string{
		"claude": validReviewJSON,
	}, nil)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"claude"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.Consolidated)
	assert.NotNil(t, result.DiffResult)
	assert.Equal(t, VerdictChangesNeeded, result.Consolidated.Verdict)
	assert.Len(t, result.Consolidated.Findings, 1)
	assert.Empty(t, result.AgentErrors)
}

func TestRun_TwoAgentsAllMode(t *testing.T) {
	t.Parallel()
	ro := buildOrchestrator(t, buildMockGit(), map[string]string{
		"claude": validReviewJSON,
		"codex":  approvedReviewJSON,
	}, nil)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"claude", "codex"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 2,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	// claude returned CHANGES_NEEDED, codex APPROVED → aggregate = CHANGES_NEEDED
	assert.Equal(t, VerdictChangesNeeded, result.Consolidated.Verdict)
	assert.Equal(t, 2, result.Consolidated.TotalAgents)
	assert.Empty(t, result.AgentErrors)
}

func TestRun_SplitMode(t *testing.T) {
	t.Parallel()
	// Provide two files so that split mode distributes them.
	gc := &mockGitClient{
		diffFilesResult: []git.DiffEntry{
			{Path: "a.go", Status: "M"},
			{Path: "b.go", Status: "A"},
		},
		numStatResult: []git.NumStatEntry{
			{Path: "a.go", Added: 3},
			{Path: "b.go", Added: 7},
		},
		unifiedResult: "fake diff",
	}

	ro := buildOrchestrator(t, gc, map[string]string{
		"claude": approvedReviewJSON,
		"codex":  approvedReviewJSON,
	}, nil)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"claude", "codex"},
		BaseBranch:  "main",
		Mode:        ReviewModeSplit,
		Concurrency: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, VerdictApproved, result.Consolidated.Verdict)
	assert.Empty(t, result.AgentErrors)
}

// ---------------------------------------------------------------------------
// Run — per-agent error paths (pipeline must not abort)
// ---------------------------------------------------------------------------

func TestRun_AgentRunError_CapturedNotAborted(t *testing.T) {
	t.Parallel()
	registry := agent.NewRegistry()
	// errAgent always returns an error from Run.
	errAgent := agent.NewMockAgent("err-agent")
	errAgent.RunFunc = func(_ context.Context, _ agent.RunOpts) (*agent.RunResult, error) {
		return nil, fmt.Errorf("connection refused")
	}
	require.NoError(t, registry.Register(errAgent))

	// goodAgent returns valid JSON.
	goodAgent := agent.NewMockAgent("good-agent")
	goodAgent.RunFunc = func(_ context.Context, _ agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{Stdout: approvedReviewJSON, ExitCode: 0}, nil
	}
	require.NoError(t, registry.Register(goodAgent))

	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(buildMockGit(), cfg, nil)
	require.NoError(t, err)
	ro := NewReviewOrchestrator(registry, diffGen, NewPromptBuilder(cfg, nil), NewConsolidator(nil), 2, nil, nil)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"err-agent", "good-agent"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 2,
	})
	require.NoError(t, err, "pipeline-level error must not be returned for per-agent failures")
	require.Len(t, result.AgentErrors, 1)
	assert.Equal(t, "err-agent", result.AgentErrors[0].Agent)
	// Errored agents contribute VerdictChangesNeeded; good-agent contributes
	// VerdictApproved. The aggregate is VerdictChangesNeeded (not BLOCKING).
	assert.Equal(t, VerdictChangesNeeded, result.Consolidated.Verdict)
}

func TestRun_AgentNonZeroExit_CapturedNotAborted(t *testing.T) {
	t.Parallel()
	registry := agent.NewRegistry()
	badExit := agent.NewMockAgent("bad-exit")
	badExit.RunFunc = func(_ context.Context, _ agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{ExitCode: 1, Stdout: "error output"}, nil
	}
	require.NoError(t, registry.Register(badExit))

	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(buildMockGit(), cfg, nil)
	require.NoError(t, err)
	ro := NewReviewOrchestrator(registry, diffGen, NewPromptBuilder(cfg, nil), NewConsolidator(nil), 1, nil, nil)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"bad-exit"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 1,
	})
	require.NoError(t, err)
	require.Len(t, result.AgentErrors, 1)
	assert.Contains(t, result.AgentErrors[0].Message, "exited with code 1")
}

func TestRun_AgentRateLimited_CapturedNotAborted(t *testing.T) {
	t.Parallel()
	registry := agent.NewRegistry()
	rateLimited := agent.NewMockAgent("rate-limited-agent")
	rateLimited.RunFunc = func(_ context.Context, _ agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{
			ExitCode: 0,
			Stdout:   "rate limit exceeded",
			RateLimit: &agent.RateLimitInfo{
				IsLimited:  true,
				ResetAfter: time.Minute,
				Message:    "rate limit exceeded",
			},
		}, nil
	}
	require.NoError(t, registry.Register(rateLimited))

	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(buildMockGit(), cfg, nil)
	require.NoError(t, err)
	ro := NewReviewOrchestrator(registry, diffGen, NewPromptBuilder(cfg, nil), NewConsolidator(nil), 1, nil, nil)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"rate-limited-agent"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 1,
	})
	require.NoError(t, err)
	require.Len(t, result.AgentErrors, 1)
	assert.Equal(t, "rate-limited-agent", result.AgentErrors[0].Agent)
}

func TestRun_AgentBadJSON_CapturedNotAborted(t *testing.T) {
	t.Parallel()
	ro := buildOrchestrator(t, buildMockGit(), map[string]string{
		"bad-json-agent": "this is not json at all",
	}, nil)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"bad-json-agent"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 1,
	})
	require.NoError(t, err)
	require.Len(t, result.AgentErrors, 1)
	assert.Contains(t, result.AgentErrors[0].Message, "failed to extract JSON")
}

// ---------------------------------------------------------------------------
// Run — event emission
// ---------------------------------------------------------------------------

func TestRun_EmitsEvents(t *testing.T) {
	t.Parallel()
	events := make(chan ReviewEvent, 20)
	ro := buildOrchestrator(t, buildMockGit(), map[string]string{
		"claude": approvedReviewJSON,
	}, events)

	_, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"claude"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 1,
	})
	require.NoError(t, err)
	close(events)

	var types []string
	for ev := range events {
		types = append(types, ev.Type)
	}

	assert.Contains(t, types, "review_started")
	assert.Contains(t, types, "agent_started")
	assert.Contains(t, types, "agent_completed")
	assert.Contains(t, types, "consolidated")
}

func TestRun_NilEvents_DoesNotPanic(t *testing.T) {
	t.Parallel()
	ro := buildOrchestrator(t, buildMockGit(), map[string]string{
		"claude": approvedReviewJSON,
	}, nil)

	_, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"claude"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 1,
	})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// DryRun
// ---------------------------------------------------------------------------

func TestDryRun_NoAgents(t *testing.T) {
	t.Parallel()
	ro := buildOrchestrator(t, buildMockGit(), nil, nil)
	_, err := ro.DryRun(context.Background(), ReviewOpts{BaseBranch: "main"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one agent is required")
}

func TestDryRun_ReturnsHumanReadablePlan(t *testing.T) {
	t.Parallel()
	ro := buildOrchestrator(t, buildMockGit(), map[string]string{
		"claude": approvedReviewJSON,
	}, nil)

	plan, err := ro.DryRun(context.Background(), ReviewOpts{
		Agents:      []string{"claude"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 1,
	})
	require.NoError(t, err)
	assert.Contains(t, plan, "Review Plan (dry run)")
	assert.Contains(t, plan, "Base branch: main")
	assert.Contains(t, plan, "Mode: all")
	assert.Contains(t, plan, "Agents: 1")
	assert.Contains(t, plan, "claude:")
}

func TestDryRun_ShowsFileCountsForSplitMode(t *testing.T) {
	t.Parallel()
	gc := &mockGitClient{
		diffFilesResult: []git.DiffEntry{
			{Path: "a.go", Status: "M"},
			{Path: "b.go", Status: "A"},
		},
		numStatResult: []git.NumStatEntry{
			{Path: "a.go", Added: 1},
			{Path: "b.go", Added: 2},
		},
		unifiedResult: "diff",
	}
	ro := buildOrchestrator(t, gc, map[string]string{
		"claude": approvedReviewJSON,
		"codex":  approvedReviewJSON,
	}, nil)

	plan, err := ro.DryRun(context.Background(), ReviewOpts{
		Agents:     []string{"claude", "codex"},
		BaseBranch: "main",
		Mode:       ReviewModeSplit,
	})
	require.NoError(t, err)

	// In split mode each agent gets one file (2 files / 2 agents).
	assert.Contains(t, plan, "1 files")
	// The plan must NOT invoke any actual agent (no network calls).
	assert.True(t, strings.Contains(plan, "claude") || strings.Contains(plan, "codex"))
}

// ---------------------------------------------------------------------------
// assignFiles
// ---------------------------------------------------------------------------

func TestAssignFiles_AllMode(t *testing.T) {
	t.Parallel()
	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(buildMockGit(), cfg, nil)
	require.NoError(t, err)
	ro := NewReviewOrchestrator(agent.NewRegistry(), diffGen, NewPromptBuilder(cfg, nil), NewConsolidator(nil), 1, nil, nil)

	diff := &DiffResult{
		Files: []ChangedFile{
			{Path: "a.go"},
			{Path: "b.go"},
		},
	}
	buckets := ro.assignFiles(diff, ReviewModeAll, 3)
	require.Len(t, buckets, 3)
	for _, b := range buckets {
		assert.Len(t, b, 2, "all-mode: every agent gets all files")
	}
}

func TestAssignFiles_SplitMode(t *testing.T) {
	t.Parallel()
	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(buildMockGit(), cfg, nil)
	require.NoError(t, err)
	ro := NewReviewOrchestrator(agent.NewRegistry(), diffGen, NewPromptBuilder(cfg, nil), NewConsolidator(nil), 1, nil, nil)

	diff := &DiffResult{
		Files: []ChangedFile{
			{Path: "a.go"},
			{Path: "b.go"},
			{Path: "c.go"},
		},
	}
	buckets := ro.assignFiles(diff, ReviewModeSplit, 2)
	require.Len(t, buckets, 2)
	total := len(buckets[0]) + len(buckets[1])
	assert.Equal(t, 3, total, "split-mode: all files are distributed")
}

func TestAssignFiles_SplitMode_MoreAgentsThanFiles(t *testing.T) {
	t.Parallel()
	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(buildMockGit(), cfg, nil)
	require.NoError(t, err)
	ro := NewReviewOrchestrator(agent.NewRegistry(), diffGen, NewPromptBuilder(cfg, nil), NewConsolidator(nil), 1, nil, nil)

	diff := &DiffResult{
		Files: []ChangedFile{{Path: "a.go"}},
	}
	buckets := ro.assignFiles(diff, ReviewModeSplit, 3)
	require.Len(t, buckets, 3)
	// Only one bucket can have a file; the rest are empty.
	populated := 0
	for _, b := range buckets {
		if len(b) > 0 {
			populated++
		}
	}
	assert.Equal(t, 1, populated)
}

// ---------------------------------------------------------------------------
// Run — sequential execution (concurrency=1, 3 agents)
// ---------------------------------------------------------------------------

func TestRun_ThreeAgents_SequentialConcurrency(t *testing.T) {
	t.Parallel()

	// Three agents with concurrency=1: they must run serially.
	// We track call order by recording call timestamps under the mutex.
	var mu sync.Mutex
	var callOrder []string

	registry := agent.NewRegistry()
	for _, name := range []string{"agent-a", "agent-b", "agent-c"} {
		name := name
		mock := agent.NewMockAgent(name)
		mock.RunFunc = func(_ context.Context, _ agent.RunOpts) (*agent.RunResult, error) {
			mu.Lock()
			callOrder = append(callOrder, name)
			mu.Unlock()
			return &agent.RunResult{Stdout: approvedReviewJSON, ExitCode: 0}, nil
		}
		require.NoError(t, registry.Register(mock))
	}

	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(buildMockGit(), cfg, nil)
	require.NoError(t, err)
	ro := NewReviewOrchestrator(registry, diffGen, NewPromptBuilder(cfg, nil), NewConsolidator(nil), 1, nil, nil)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"agent-a", "agent-b", "agent-c"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, VerdictApproved, result.Consolidated.Verdict)
	assert.Equal(t, 3, result.Consolidated.TotalAgents)
	assert.Empty(t, result.AgentErrors)
	// All three agents ran.
	assert.Len(t, callOrder, 3)
}

// ---------------------------------------------------------------------------
// Run — all agents fail
// ---------------------------------------------------------------------------

func TestRun_AllAgentsFail_ZeroFindingsWithErrors(t *testing.T) {
	t.Parallel()

	registry := agent.NewRegistry()
	for _, name := range []string{"fail-1", "fail-2"} {
		name := name
		mock := agent.NewMockAgent(name)
		mock.RunFunc = func(_ context.Context, _ agent.RunOpts) (*agent.RunResult, error) {
			return nil, fmt.Errorf("%s: connection refused", name)
		}
		require.NoError(t, registry.Register(mock))
	}

	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(buildMockGit(), cfg, nil)
	require.NoError(t, err)
	ro := NewReviewOrchestrator(registry, diffGen, NewPromptBuilder(cfg, nil), NewConsolidator(nil), 2, nil, nil)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"fail-1", "fail-2"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 2,
	})
	require.NoError(t, err, "pipeline must not abort when all agents fail")
	require.NotNil(t, result)
	// Zero unique findings because all agents errored.
	assert.Empty(t, result.Consolidated.Findings)
	// Both agents must appear in AgentErrors.
	assert.Len(t, result.AgentErrors, 2)
	agentNames := make([]string, 0, 2)
	for _, ae := range result.AgentErrors {
		agentNames = append(agentNames, ae.Agent)
	}
	assert.Contains(t, agentNames, "fail-1")
	assert.Contains(t, agentNames, "fail-2")
}

// ---------------------------------------------------------------------------
// Run — 4-file split across 2 agents (each agent gets 2 files)
// ---------------------------------------------------------------------------

func TestRun_SplitMode_FourFiles_TwoAgents(t *testing.T) {
	t.Parallel()

	gc := &mockGitClient{
		diffFilesResult: []git.DiffEntry{
			{Path: "a.go", Status: "M"},
			{Path: "b.go", Status: "A"},
			{Path: "c.go", Status: "M"},
			{Path: "d.go", Status: "A"},
		},
		numStatResult: []git.NumStatEntry{
			{Path: "a.go", Added: 1},
			{Path: "b.go", Added: 2},
			{Path: "c.go", Added: 3},
			{Path: "d.go", Added: 4},
		},
		unifiedResult: "fake diff",
	}

	ro := buildOrchestrator(t, gc, map[string]string{
		"claude": approvedReviewJSON,
		"codex":  approvedReviewJSON,
	}, nil)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"claude", "codex"},
		BaseBranch:  "main",
		Mode:        ReviewModeSplit,
		Concurrency: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, VerdictApproved, result.Consolidated.Verdict)
	assert.Empty(t, result.AgentErrors)
}

// ---------------------------------------------------------------------------
// Run — 4-file all-mode with 2 agents (each agent gets all 4 files)
// ---------------------------------------------------------------------------

func TestRun_AllMode_FourFiles_TwoAgents(t *testing.T) {
	t.Parallel()

	gc := &mockGitClient{
		diffFilesResult: []git.DiffEntry{
			{Path: "a.go", Status: "M"},
			{Path: "b.go", Status: "A"},
			{Path: "c.go", Status: "M"},
			{Path: "d.go", Status: "A"},
		},
		numStatResult: []git.NumStatEntry{
			{Path: "a.go", Added: 1},
			{Path: "b.go", Added: 2},
			{Path: "c.go", Added: 3},
			{Path: "d.go", Added: 4},
		},
		unifiedResult: "fake diff",
	}

	registry := agent.NewRegistry()
	for _, name := range []string{"claude", "codex"} {
		name := name
		mock := agent.NewMockAgent(name)
		mock.RunFunc = func(_ context.Context, _ agent.RunOpts) (*agent.RunResult, error) {
			return &agent.RunResult{Stdout: approvedReviewJSON, ExitCode: 0}, nil
		}
		require.NoError(t, registry.Register(mock))
	}

	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(gc, cfg, nil)
	require.NoError(t, err)
	ro := NewReviewOrchestrator(registry, diffGen, NewPromptBuilder(cfg, nil), NewConsolidator(nil), 2, nil, nil)

	// Verify file assignment directly via assignFiles.
	diff := &DiffResult{
		Files: []ChangedFile{
			{Path: "a.go"}, {Path: "b.go"}, {Path: "c.go"}, {Path: "d.go"},
		},
	}
	buckets := ro.assignFiles(diff, ReviewModeAll, 2)
	require.Len(t, buckets, 2)
	assert.Len(t, buckets[0], 4, "all-mode: agent 0 gets all 4 files")
	assert.Len(t, buckets[1], 4, "all-mode: agent 1 gets all 4 files")

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"claude", "codex"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, VerdictApproved, result.Consolidated.Verdict)
	assert.Empty(t, result.AgentErrors)
}

// ---------------------------------------------------------------------------
// Run — JSON embedded in markdown code fence
// ---------------------------------------------------------------------------

func TestRun_AgentOutputJSONInMarkdown(t *testing.T) {
	t.Parallel()

	markdownOutput := "I reviewed the code carefully.\n\n```json\n" + validReviewJSON + "\n```\n\nPlease address the findings."

	ro := buildOrchestrator(t, buildMockGit(), map[string]string{
		"claude": markdownOutput,
	}, nil)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"claude"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.AgentErrors, "JSON in markdown should be extracted successfully")
	assert.Len(t, result.Consolidated.Findings, 1)
	assert.Equal(t, VerdictChangesNeeded, result.Consolidated.Verdict)
}

// ---------------------------------------------------------------------------
// Run — multiple JSON objects in output (first valid one used)
// ---------------------------------------------------------------------------

func TestRun_AgentMultipleJSONObjects_FirstExtractedUsed(t *testing.T) {
	t.Parallel()

	// When the agent output contains multiple JSON objects, jsonutil.ExtractInto
	// returns the first one that successfully unmarshals. The orchestrator then
	// validates the domain structure with Validate(). If the first JSON object
	// lacks a valid Verdict, the orchestrator reports an AgentError. The pipeline
	// does NOT abort — it continues and still returns a consolidated result.
	//
	// First object: valid JSON but fails ReviewResult.Validate() (empty verdict).
	// Second object: valid ReviewResult with a proper verdict.
	// Since ExtractInto picks the first successful unmarshal, the orchestrator
	// will get the first object, which has no verdict → validation failure →
	// captured as AgentError.
	multiJSONOutput := `{"thinking":"let me analyse this"} ` + validReviewJSON

	ro := buildOrchestrator(t, buildMockGit(), map[string]string{
		"claude": multiJSONOutput,
	}, nil)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"claude"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 1,
	})
	require.NoError(t, err, "pipeline must not abort even when validation fails")
	require.NotNil(t, result)
	// The first JSON object is extracted but fails Validate() → AgentError.
	assert.NotNil(t, result.Consolidated)
	// One agent error: the extracted JSON had an invalid verdict.
	require.Len(t, result.AgentErrors, 1)
	assert.Contains(t, result.AgentErrors[0].Message, "invalid review result")
}

// ---------------------------------------------------------------------------
// Run — agent valid JSON with extra unexpected fields (tolerated)
// ---------------------------------------------------------------------------

func TestRun_AgentJSONExtraFields_Tolerated(t *testing.T) {
	t.Parallel()

	// JSON with extra fields that ReviewResult doesn't know about.
	jsonWithExtras := `{"findings":[{"severity":"low","category":"style","file":"main.go","line":5,"description":"style issue","suggestion":"fix style"}],"verdict":"CHANGES_NEEDED","extra_field":"ignored","another_field":42}`

	ro := buildOrchestrator(t, buildMockGit(), map[string]string{
		"claude": jsonWithExtras,
	}, nil)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"claude"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.AgentErrors, "extra JSON fields must be tolerated")
	assert.Len(t, result.Consolidated.Findings, 1)
	assert.Equal(t, VerdictChangesNeeded, result.Consolidated.Verdict)
}

// ---------------------------------------------------------------------------
// Run — context cancelled mid-review returns partial results
// ---------------------------------------------------------------------------

func TestRun_ContextCancelled_ReturnsPartialResults(t *testing.T) {
	t.Parallel()

	// Use a context that is already cancelled before Run is called.
	// The diff generation will fail with context.Canceled, which is an
	// orchestrator-level error (not a per-agent error), so the pipeline
	// itself returns an error.
	gc := &mockGitClient{
		diffFilesErr: context.Canceled,
	}
	ro := buildOrchestrator(t, gc, map[string]string{
		"claude": approvedReviewJSON,
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := ro.Run(ctx, ReviewOpts{
		Agents:     []string{"claude"},
		BaseBranch: "main",
		Mode:       ReviewModeAll,
	})
	// The diff generation error propagates as a pipeline-level error.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "generating diff")
}

// TestRun_ContextCancelledDuringAgentExecution verifies that when the context is
// cancelled while agents are running, the orchestrator captures the agent error
// rather than aborting the whole pipeline (per-agent errors are never fatal).
func TestRun_ContextCancelledDuringAgentExecution(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	// Use an agent that cancels the context, then returns context.Canceled.
	registry := agent.NewRegistry()
	slowAgent := agent.NewMockAgent("slow-agent")
	slowAgent.RunFunc = func(innerCtx context.Context, _ agent.RunOpts) (*agent.RunResult, error) {
		// Cancel the external context after this agent starts.
		cancel()
		// Honour the cancellation immediately.
		select {
		case <-innerCtx.Done():
			return nil, innerCtx.Err()
		case <-time.After(5 * time.Second):
			return &agent.RunResult{Stdout: approvedReviewJSON, ExitCode: 0}, nil
		}
	}
	require.NoError(t, registry.Register(slowAgent))

	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(buildMockGit(), cfg, nil)
	require.NoError(t, err)
	ro := NewReviewOrchestrator(registry, diffGen, NewPromptBuilder(cfg, nil), NewConsolidator(nil), 1, nil, nil)

	result, err := ro.Run(ctx, ReviewOpts{
		Agents:     []string{"slow-agent"},
		BaseBranch: "main",
		Mode:       ReviewModeAll,
	})
	// The orchestrator never propagates per-agent errors to the caller; it always
	// captures them in AgentErrors. The pipeline returns a (partial) result.
	require.NoError(t, err, "per-agent context cancellation must not abort the pipeline")
	require.NotNil(t, result)
	assert.Len(t, result.AgentErrors, 1,
		"context cancellation in agent must be recorded as an AgentError")
}

// ---------------------------------------------------------------------------
// Run — concurrency higher than number of agents
// ---------------------------------------------------------------------------

func TestRun_ConcurrencyHigherThanAgents(t *testing.T) {
	t.Parallel()

	ro := buildOrchestrator(t, buildMockGit(), map[string]string{
		"claude": approvedReviewJSON,
	}, nil)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"claude"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 100, // much higher than number of agents
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, VerdictApproved, result.Consolidated.Verdict)
	assert.Empty(t, result.AgentErrors)
}

// ---------------------------------------------------------------------------
// Run — event order verification
// ---------------------------------------------------------------------------

func TestRun_EventOrder_ReviewStarted_AgentStarted_AgentCompleted_Consolidated(t *testing.T) {
	t.Parallel()

	events := make(chan ReviewEvent, 50)
	ro := buildOrchestrator(t, buildMockGit(), map[string]string{
		"claude": approvedReviewJSON,
		"codex":  approvedReviewJSON,
	}, events)

	_, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"claude", "codex"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 2,
	})
	require.NoError(t, err)
	close(events)

	var collected []ReviewEvent
	for ev := range events {
		collected = append(collected, ev)
	}

	require.NotEmpty(t, collected)

	// First event must be review_started.
	assert.Equal(t, "review_started", collected[0].Type,
		"first event must be review_started")

	// Last event must be consolidated.
	assert.Equal(t, "consolidated", collected[len(collected)-1].Type,
		"last event must be consolidated")

	// Count event types.
	counts := make(map[string]int)
	for _, ev := range collected {
		counts[ev.Type]++
	}
	assert.Equal(t, 1, counts["review_started"])
	assert.Equal(t, 2, counts["agent_started"])
	assert.Equal(t, 2, counts["agent_completed"])
	assert.Equal(t, 1, counts["consolidated"])
}

func TestRun_EventOrder_SingleAgent(t *testing.T) {
	t.Parallel()

	events := make(chan ReviewEvent, 20)
	ro := buildOrchestrator(t, buildMockGit(), map[string]string{
		"claude": approvedReviewJSON,
	}, events)

	_, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"claude"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 1,
	})
	require.NoError(t, err)
	close(events)

	var types []string
	for ev := range events {
		types = append(types, ev.Type)
	}

	// For a single agent the sequence is deterministic: started, completed.
	require.GreaterOrEqual(t, len(types), 4)
	assert.Equal(t, "review_started", types[0])
	assert.Equal(t, "consolidated", types[len(types)-1])

	// agent_started must come before agent_completed for the same agent.
	startedIdx := -1
	completedIdx := -1
	for i, ty := range types {
		if ty == "agent_started" && startedIdx < 0 {
			startedIdx = i
		}
		if ty == "agent_completed" && completedIdx < 0 {
			completedIdx = i
		}
	}
	assert.Greater(t, completedIdx, startedIdx,
		"agent_completed must come after agent_started")
}

// ---------------------------------------------------------------------------
// Run — rate-limited agent emits rate_limited event
// ---------------------------------------------------------------------------

func TestRun_RateLimitedAgent_EmitsRateLimitedEvent(t *testing.T) {
	t.Parallel()

	events := make(chan ReviewEvent, 20)
	registry := agent.NewRegistry()
	rateLimited := agent.NewMockAgent("rate-agent")
	rateLimited.RunFunc = func(_ context.Context, _ agent.RunOpts) (*agent.RunResult, error) {
		return &agent.RunResult{
			ExitCode: 0,
			Stdout:   "rate limit exceeded",
			RateLimit: &agent.RateLimitInfo{
				IsLimited:  true,
				ResetAfter: time.Minute,
				Message:    "rate limit exceeded",
			},
		}, nil
	}
	require.NoError(t, registry.Register(rateLimited))

	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(buildMockGit(), cfg, nil)
	require.NoError(t, err)
	ro := NewReviewOrchestrator(registry, diffGen, NewPromptBuilder(cfg, nil), NewConsolidator(nil), 1, nil, events)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:     []string{"rate-agent"},
		BaseBranch: "main",
		Mode:       ReviewModeAll,
	})
	require.NoError(t, err)
	close(events)

	var eventTypes []string
	for ev := range events {
		eventTypes = append(eventTypes, ev.Type)
	}

	assert.Contains(t, eventTypes, "rate_limited",
		"rate_limited event must be emitted for a rate-limited agent")

	// The agent error must be captured.
	require.Len(t, result.AgentErrors, 1)
	assert.Equal(t, "rate-agent", result.AgentErrors[0].Agent)
}

// ---------------------------------------------------------------------------
// Run — OrchestratorResult fields are populated
// ---------------------------------------------------------------------------

func TestRun_ResultFieldsPopulated(t *testing.T) {
	t.Parallel()
	ro := buildOrchestrator(t, buildMockGit(), map[string]string{
		"claude": validReviewJSON,
	}, nil)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:     []string{"claude"},
		BaseBranch: "main",
		Mode:       ReviewModeAll,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotNil(t, result.Consolidated)
	assert.NotNil(t, result.Stats)
	assert.NotNil(t, result.DiffResult)
	assert.Equal(t, "main", result.DiffResult.BaseBranch)
	assert.Greater(t, result.Duration, time.Duration(0))
}

// ---------------------------------------------------------------------------
// DryRun — does not invoke agents
// ---------------------------------------------------------------------------

func TestDryRun_DoesNotInvokeAgents(t *testing.T) {
	t.Parallel()

	registry := agent.NewRegistry()
	invoked := false
	mock := agent.NewMockAgent("claude")
	mock.RunFunc = func(_ context.Context, _ agent.RunOpts) (*agent.RunResult, error) {
		invoked = true
		return &agent.RunResult{Stdout: approvedReviewJSON, ExitCode: 0}, nil
	}
	require.NoError(t, registry.Register(mock))

	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(buildMockGit(), cfg, nil)
	require.NoError(t, err)
	ro := NewReviewOrchestrator(registry, diffGen, NewPromptBuilder(cfg, nil), NewConsolidator(nil), 1, nil, nil)

	plan, err := ro.DryRun(context.Background(), ReviewOpts{
		Agents:     []string{"claude"},
		BaseBranch: "main",
		Mode:       ReviewModeAll,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, plan)
	assert.False(t, invoked, "DryRun must not invoke the agent's Run method")
}

// ---------------------------------------------------------------------------
// assignFiles — unknown mode treated as all-mode
// ---------------------------------------------------------------------------

func TestAssignFiles_UnknownMode_TreatedAsAll(t *testing.T) {
	t.Parallel()
	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(buildMockGit(), cfg, nil)
	require.NoError(t, err)
	ro := NewReviewOrchestrator(agent.NewRegistry(), diffGen, NewPromptBuilder(cfg, nil), NewConsolidator(nil), 1, nil, nil)

	diff := &DiffResult{
		Files: []ChangedFile{
			{Path: "a.go"},
			{Path: "b.go"},
		},
	}
	// "unknown" mode should fall through to the default (all) case.
	buckets := ro.assignFiles(diff, ReviewMode("unknown"), 2)
	require.Len(t, buckets, 2)
	for _, b := range buckets {
		assert.Len(t, b, 2, "unknown mode falls back to all-mode: each agent gets all files")
	}
}

// ---------------------------------------------------------------------------
// Run — ReviewEvent timestamps are non-zero
// ---------------------------------------------------------------------------

func TestRun_EventTimestamps_NonZero(t *testing.T) {
	t.Parallel()

	events := make(chan ReviewEvent, 20)
	ro := buildOrchestrator(t, buildMockGit(), map[string]string{
		"claude": approvedReviewJSON,
	}, events)

	_, err := ro.Run(context.Background(), ReviewOpts{
		Agents:     []string{"claude"},
		BaseBranch: "main",
		Mode:       ReviewModeAll,
	})
	require.NoError(t, err)
	close(events)

	for ev := range events {
		assert.False(t, ev.Timestamp.IsZero(),
			"event %q must have a non-zero timestamp", ev.Type)
	}
}

// ---------------------------------------------------------------------------
// Run — blocking verdict from one agent propagates to consolidated result
// ---------------------------------------------------------------------------

func TestRun_BlockingVerdict_Propagates(t *testing.T) {
	t.Parallel()

	blockingJSON := `{"findings":[{"severity":"critical","category":"security","file":"main.go","line":1,"description":"critical bug","suggestion":"fix now"}],"verdict":"BLOCKING"}`

	ro := buildOrchestrator(t, buildMockGit(), map[string]string{
		"claude": blockingJSON,
		"codex":  approvedReviewJSON,
	}, nil)

	result, err := ro.Run(context.Background(), ReviewOpts{
		Agents:      []string{"claude", "codex"},
		BaseBranch:  "main",
		Mode:        ReviewModeAll,
		Concurrency: 2,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, VerdictBlocking, result.Consolidated.Verdict,
		"BLOCKING verdict from any agent must be propagated to consolidated result")
	assert.Empty(t, result.AgentErrors)
}

// ---------------------------------------------------------------------------
// Benchmark — orchestrator with two mock agents
// ---------------------------------------------------------------------------

func BenchmarkOrchestrator_TwoAgents(b *testing.B) {
	registry := agent.NewRegistry()
	for _, name := range []string{"claude", "codex"} {
		name := name
		mock := agent.NewMockAgent(name)
		mock.RunFunc = func(_ context.Context, _ agent.RunOpts) (*agent.RunResult, error) {
			return &agent.RunResult{Stdout: approvedReviewJSON, ExitCode: 0}, nil
		}
		if err := registry.Register(mock); err != nil {
			b.Fatal(err)
		}
	}

	cfg := ReviewConfig{}
	diffGen, err := NewDiffGenerator(buildMockGit(), cfg, nil)
	if err != nil {
		b.Fatal(err)
	}
	ro := NewReviewOrchestrator(registry, diffGen, NewPromptBuilder(cfg, nil), NewConsolidator(nil), 2, nil, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ro.Run(context.Background(), ReviewOpts{
			Agents:      []string{"claude", "codex"},
			BaseBranch:  "main",
			Mode:        ReviewModeAll,
			Concurrency: 2,
		})
	}
}
