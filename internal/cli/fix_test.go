package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
)

// ---- firstConfiguredAgentName tests -----------------------------------------

func TestFirstConfiguredAgentName(t *testing.T) {
	tests := []struct {
		name      string
		agentCfgs map[string]config.AgentConfig
		want      string
	}{
		{
			name:      "empty config returns empty string",
			agentCfgs: map[string]config.AgentConfig{},
			want:      "",
		},
		{
			name: "claude configured returns claude",
			agentCfgs: map[string]config.AgentConfig{
				"claude": {Command: "claude"},
			},
			want: "claude",
		},
		{
			name: "codex configured returns codex when claude is absent",
			agentCfgs: map[string]config.AgentConfig{
				"codex": {Command: "codex"},
			},
			want: "codex",
		},
		{
			name: "gemini configured returns gemini when claude and codex absent",
			agentCfgs: map[string]config.AgentConfig{
				"gemini": {Model: "gemini-pro"},
			},
			want: "gemini",
		},
		{
			name: "claude takes priority over codex and gemini",
			agentCfgs: map[string]config.AgentConfig{
				"claude": {Command: "claude"},
				"codex":  {Command: "codex"},
				"gemini": {Model: "gemini-pro"},
			},
			want: "claude",
		},
		{
			name: "codex takes priority over gemini",
			agentCfgs: map[string]config.AgentConfig{
				"codex":  {Command: "codex"},
				"gemini": {Model: "gemini-pro"},
			},
			want: "codex",
		},
		{
			name: "agent with empty command and model is skipped",
			agentCfgs: map[string]config.AgentConfig{
				"claude": {Command: "", Model: ""},
				"codex":  {Command: "codex"},
			},
			want: "codex",
		},
		{
			name: "agent configured with only model counts",
			agentCfgs: map[string]config.AgentConfig{
				"claude": {Model: "claude-sonnet-4"},
			},
			want: "claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstConfiguredAgentName(tt.agentCfgs)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---- mostRecentMDFile tests --------------------------------------------------

func TestMostRecentMDFile_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	got, err := mostRecentMDFile(dir)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestMostRecentMDFile_NoMDFiles(t *testing.T) {
	dir := t.TempDir()
	// Create non-markdown files.
	writeFile(t, filepath.Join(dir, "report.txt"), "text content")
	writeFile(t, filepath.Join(dir, "data.json"), "{}")

	got, err := mostRecentMDFile(dir)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestMostRecentMDFile_SingleMDFile(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "report.md")
	writeFile(t, mdPath, "# Review Report")

	got, err := mostRecentMDFile(dir)
	require.NoError(t, err)
	assert.Equal(t, mdPath, got)
}

func TestMostRecentMDFile_PicksMostRecent(t *testing.T) {
	dir := t.TempDir()

	older := filepath.Join(dir, "older.md")
	newer := filepath.Join(dir, "newer.md")

	writeFile(t, older, "old content")
	// Sleep briefly to ensure different mod times.
	time.Sleep(10 * time.Millisecond)
	writeFile(t, newer, "new content")

	got, err := mostRecentMDFile(dir)
	require.NoError(t, err)
	assert.Equal(t, newer, got)
}

func TestMostRecentMDFile_NonRecursive(t *testing.T) {
	dir := t.TempDir()

	// Create a subdirectory with a more recent .md file.
	subdir := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(subdir, 0755))
	time.Sleep(10 * time.Millisecond)
	writeFile(t, filepath.Join(subdir, "sub.md"), "subdir content")

	// Create a top-level .md file (should be returned, not the subdir one).
	top := filepath.Join(dir, "top.md")
	writeFile(t, top, "top content")

	got, err := mostRecentMDFile(dir)
	require.NoError(t, err)
	// The subdir file should be skipped; top-level is returned.
	assert.Equal(t, top, got)
}

// ---- resolveReviewReport tests -----------------------------------------------

func TestResolveReviewReport_ExplicitPath(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.md")
	content := "# Review\n\nFindings here."
	writeFile(t, reportPath, content)

	logger := discardLogger()
	gotContent, gotPath := resolveReviewReport(reportPath, "", logger)
	assert.Equal(t, content, gotContent)
	assert.Equal(t, reportPath, gotPath)
}

func TestResolveReviewReport_ExplicitPathNotFound(t *testing.T) {
	logger := discardLogger()
	gotContent, gotPath := resolveReviewReport("/nonexistent/path/report.md", "", logger)
	assert.Empty(t, gotContent)
	assert.Empty(t, gotPath)
}

func TestResolveReviewReport_AutoDetect(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "review.md")
	content := "# Auto-detected report"
	writeFile(t, reportPath, content)

	logger := discardLogger()
	gotContent, gotPath := resolveReviewReport("", dir, logger)
	assert.Equal(t, content, gotContent)
	assert.Equal(t, reportPath, gotPath)
}

func TestResolveReviewReport_EmptyLogDir(t *testing.T) {
	logger := discardLogger()
	gotContent, gotPath := resolveReviewReport("", "", logger)
	assert.Empty(t, gotContent)
	assert.Empty(t, gotPath)
}

func TestResolveReviewReport_LogDirNoMDFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "data.json"), "{}")

	logger := discardLogger()
	gotContent, gotPath := resolveReviewReport("", dir, logger)
	assert.Empty(t, gotContent)
	assert.Empty(t, gotPath)
}

// ---- newFixCmd registration test --------------------------------------------

func TestNewFixCmd_Registration(t *testing.T) {
	cmd := newFixCmd()
	assert.Equal(t, "fix", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)

	// Verify flags are registered.
	assert.NotNil(t, cmd.Flags().Lookup("agent"))
	assert.NotNil(t, cmd.Flags().Lookup("max-fix-cycles"))
	assert.NotNil(t, cmd.Flags().Lookup("review-report"))

	// Verify default values.
	maxCycles, err := cmd.Flags().GetInt("max-fix-cycles")
	require.NoError(t, err)
	assert.Equal(t, 3, maxCycles)

	agentVal, err := cmd.Flags().GetString("agent")
	require.NoError(t, err)
	assert.Equal(t, "", agentVal)
}

// ---- TestFixCmd_FlagDefaults -------------------------------------------------

// TestFixCmd_FlagDefaults verifies that all flags on the fix command have the
// correct default values when no flags are explicitly set.
func TestFixCmd_FlagDefaults(t *testing.T) {
	cmd := newFixCmd()

	// --agent: default empty string (auto-detect from config).
	agent, err := cmd.Flags().GetString("agent")
	require.NoError(t, err)
	assert.Equal(t, "", agent, "--agent default should be empty string")

	// --max-fix-cycles: default 3.
	maxCycles, err := cmd.Flags().GetInt("max-fix-cycles")
	require.NoError(t, err)
	assert.Equal(t, 3, maxCycles, "--max-fix-cycles default should be 3")

	// --review-report: default empty string (auto-detect).
	report, err := cmd.Flags().GetString("review-report")
	require.NoError(t, err)
	assert.Equal(t, "", report, "--review-report default should be empty string")
}

// ---- TestFixCmd_AgentFlag ---------------------------------------------------

// TestFixCmd_AgentFlag verifies that --agent claude correctly sets the agent
// flag value to "claude" via cobra flag parsing.
func TestFixCmd_AgentFlag(t *testing.T) {
	cmd := newFixCmd()
	err := cmd.ParseFlags([]string{"--agent", "claude"})
	require.NoError(t, err)

	agent, err := cmd.Flags().GetString("agent")
	require.NoError(t, err)
	assert.Equal(t, "claude", agent)
}

// TestFixCmd_AgentFlag_Codex verifies --agent codex is accepted.
func TestFixCmd_AgentFlag_Codex(t *testing.T) {
	cmd := newFixCmd()
	require.NoError(t, cmd.ParseFlags([]string{"--agent", "codex"}))

	agent, err := cmd.Flags().GetString("agent")
	require.NoError(t, err)
	assert.Equal(t, "codex", agent)
}

// TestFixCmd_AgentFlag_Gemini verifies --agent gemini is accepted.
func TestFixCmd_AgentFlag_Gemini(t *testing.T) {
	cmd := newFixCmd()
	require.NoError(t, cmd.ParseFlags([]string{"--agent", "gemini"}))

	agent, err := cmd.Flags().GetString("agent")
	require.NoError(t, err)
	assert.Equal(t, "gemini", agent)
}

// ---- TestFixCmd_MaxFixCyclesFlag --------------------------------------------

// TestFixCmd_MaxFixCyclesFlag verifies that --max-fix-cycles 5 sets the flag
// value to 5.
func TestFixCmd_MaxFixCyclesFlag(t *testing.T) {
	cmd := newFixCmd()
	err := cmd.ParseFlags([]string{"--max-fix-cycles", "5"})
	require.NoError(t, err)

	maxCycles, err := cmd.Flags().GetInt("max-fix-cycles")
	require.NoError(t, err)
	assert.Equal(t, 5, maxCycles)
}

// TestFixCmd_MaxFixCyclesFlag_One verifies boundary value 1.
func TestFixCmd_MaxFixCyclesFlag_One(t *testing.T) {
	cmd := newFixCmd()
	require.NoError(t, cmd.ParseFlags([]string{"--max-fix-cycles", "1"}))

	maxCycles, err := cmd.Flags().GetInt("max-fix-cycles")
	require.NoError(t, err)
	assert.Equal(t, 1, maxCycles)
}

// TestFixCmd_MaxFixCyclesFlag_Zero verifies that 0 (no cycles) is accepted
// as a valid value (disables fix engine).
func TestFixCmd_MaxFixCyclesFlag_Zero(t *testing.T) {
	cmd := newFixCmd()
	require.NoError(t, cmd.ParseFlags([]string{"--max-fix-cycles", "0"}))

	maxCycles, err := cmd.Flags().GetInt("max-fix-cycles")
	require.NoError(t, err)
	assert.Equal(t, 0, maxCycles)
}

// ---- TestFixCmd_ReviewReportFlag -------------------------------------------

// TestFixCmd_ReviewReportFlag verifies --review-report sets the path.
func TestFixCmd_ReviewReportFlag(t *testing.T) {
	cmd := newFixCmd()
	require.NoError(t, cmd.ParseFlags([]string{"--review-report", "/tmp/review.md"}))

	report, err := cmd.Flags().GetString("review-report")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/review.md", report)
}

// ---- TestFixCmd_FlagsSetViaCommand ------------------------------------------

// TestFixCmd_FlagsSetViaCommand verifies that all fix flags are correctly wired
// by invoking cmd.Execute() with a stub RunE that captures the flag values.
func TestFixCmd_FlagsSetViaCommand(t *testing.T) {
	cmd := newFixCmd()

	var capturedAgent string
	var capturedMaxCycles int
	var capturedReport string

	cmd.RunE = func(c *cobra.Command, args []string) error {
		capturedAgent, _ = c.Flags().GetString("agent")
		capturedMaxCycles, _ = c.Flags().GetInt("max-fix-cycles")
		capturedReport, _ = c.Flags().GetString("review-report")
		return nil
	}

	cmd.SetArgs([]string{
		"--agent", "codex",
		"--max-fix-cycles", "7",
		"--review-report", ".raven/logs/review-report.md",
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "codex", capturedAgent)
	assert.Equal(t, 7, capturedMaxCycles)
	assert.Equal(t, ".raven/logs/review-report.md", capturedReport)
}

// ---- TestFixCmd_DefaultsAppliedByCommand ------------------------------------

// TestFixCmd_DefaultsAppliedByCommand verifies that Cobra applies the correct
// defaults when the fix command is executed without any flags.
func TestFixCmd_DefaultsAppliedByCommand(t *testing.T) {
	cmd := newFixCmd()

	var capturedAgent string
	var capturedMaxCycles int
	var capturedReport string

	cmd.RunE = func(c *cobra.Command, args []string) error {
		capturedAgent, _ = c.Flags().GetString("agent")
		capturedMaxCycles, _ = c.Flags().GetInt("max-fix-cycles")
		capturedReport, _ = c.Flags().GetString("review-report")
		return nil
	}

	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "", capturedAgent, "--agent default should be empty")
	assert.Equal(t, 3, capturedMaxCycles, "--max-fix-cycles default should be 3")
	assert.Equal(t, "", capturedReport, "--review-report default should be empty")
}

// ---- TestFixCmd_NoArgsAllowed -----------------------------------------------

// TestFixCmd_NoArgsAllowed verifies the fix command rejects positional arguments.
func TestFixCmd_NoArgsAllowed(t *testing.T) {
	cmd := newFixCmd()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"unexpected-positional-arg"})

	err := cmd.Execute()
	assert.Error(t, err, "fix command should reject positional arguments")
}

// ---- TestFixCmd_AgentFlagHasShellCompletion ---------------------------------

// TestFixCmd_AgentFlagHasShellCompletion verifies that --agent has a shell
// completion function returning the three known agent names.
func TestFixCmd_AgentFlagHasShellCompletion(t *testing.T) {
	cmd := newFixCmd()

	completionFn, exists := cmd.GetFlagCompletionFunc("agent")
	require.True(t, exists, "--agent flag should have a completion function registered")
	require.NotNil(t, completionFn)

	completions, directive := completionFn(cmd, []string{}, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.ElementsMatch(t, []string{"claude", "codex", "gemini"}, completions)
}

// ---- TestFixCmdRegisteredOnRoot ---------------------------------------------

// TestFixCmdRegisteredOnRoot verifies that the fix command is registered as a
// subcommand of rootCmd via the init() function.
func TestFixCmdRegisteredOnRoot(t *testing.T) {
	found := false
	for _, sub := range rootCmd.Commands() {
		if sub.Use == "fix" {
			found = true
			break
		}
	}
	assert.True(t, found, "fix command should be registered as a subcommand of root")
}

// ---- TestFixCmd_HelpContent -------------------------------------------------

// TestFixCmd_HelpContent verifies the Long and Example strings mention the
// key flags users need to know about.
func TestFixCmd_HelpContent(t *testing.T) {
	cmd := newFixCmd()
	content := cmd.Long + cmd.Example
	for _, flag := range []string{"--agent", "--max-fix-cycles", "--review-report", "--dry-run"} {
		assert.Contains(t, content, flag,
			"help content should mention %s", flag)
	}
}

// TestFixCmd_LongDescribesExitCodes verifies that the Long description
// mentions the exit codes used by the fix command.
func TestFixCmd_LongDescribesExitCodes(t *testing.T) {
	cmd := newFixCmd()
	assert.Contains(t, cmd.Long, "0", "long description should mention exit code 0")
	assert.Contains(t, cmd.Long, "2", "long description should mention exit code 2")
}

// ---- TestResolveReviewReport_AutoDetectPicksNewest --------------------------

// TestResolveReviewReport_AutoDetectPicksNewest verifies that when multiple .md
// files exist in the log dir, the most recently modified one is picked.
func TestResolveReviewReport_AutoDetectPicksNewest(t *testing.T) {
	dir := t.TempDir()

	olderPath := filepath.Join(dir, "older-report.md")
	newerPath := filepath.Join(dir, "newer-report.md")

	writeFile(t, olderPath, "# Older Report\nSome findings here.")
	// Ensure a distinct mod-time by sleeping briefly.
	time.Sleep(15 * time.Millisecond)
	newerContent := "# Newer Report\nMore recent findings."
	writeFile(t, newerPath, newerContent)

	logger := discardLogger()
	gotContent, gotPath := resolveReviewReport("", dir, logger)

	assert.Equal(t, newerPath, gotPath, "auto-detect should pick the most recently modified .md file")
	assert.Equal(t, newerContent, gotContent)
}

// ---- TestFirstConfiguredAgentName_NilMap ------------------------------------

// TestFirstConfiguredAgentName_NilMap verifies that firstConfiguredAgentName
// handles a nil map gracefully without panicking, returning an empty string.
func TestFirstConfiguredAgentName_NilMap(t *testing.T) {
	// This must not panic.
	got := firstConfiguredAgentName(nil)
	assert.Equal(t, "", got, "nil agent map should return empty string")
}

// ---- TestMostRecentMDFile_MixedFiles ----------------------------------------

// TestMostRecentMDFile_MixedFiles verifies that non-.md files in the directory
// are ignored and only the most recent .md file is returned.
func TestMostRecentMDFile_MixedFiles(t *testing.T) {
	dir := t.TempDir()

	// Create non-md files first.
	writeFile(t, filepath.Join(dir, "notes.txt"), "some notes")
	writeFile(t, filepath.Join(dir, "data.json"), `{"key": "value"}`)
	writeFile(t, filepath.Join(dir, "config.yaml"), "config: true")

	// Create an .md file after the non-md files.
	time.Sleep(10 * time.Millisecond)
	mdPath := filepath.Join(dir, "review.md")
	writeFile(t, mdPath, "# Review Report\nFindings.")

	got, err := mostRecentMDFile(dir)
	require.NoError(t, err)
	assert.Equal(t, mdPath, got, "should return the only .md file, ignoring non-.md files")
}

// TestMostRecentMDFile_MultipleMixedWithMD verifies that when the directory
// contains both .md files and non-.md files, only the most recent .md file
// is returned.
func TestMostRecentMDFile_MultipleMixedWithMD(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "first.md"), "first")
	time.Sleep(10 * time.Millisecond)
	writeFile(t, filepath.Join(dir, "second.txt"), "second txt")
	time.Sleep(10 * time.Millisecond)
	writeFile(t, filepath.Join(dir, "second.md"), "second md")
	// non-md created after the last md -- should be ignored.
	time.Sleep(10 * time.Millisecond)
	writeFile(t, filepath.Join(dir, "later.txt"), "later non-md")

	got, err := mostRecentMDFile(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "second.md"), got)
}

// ---- TestResolveReviewReport_LogDirExistsNoMDFiles --------------------------

// TestResolveReviewReport_LogDirExistsNoMDFiles verifies that when the log dir
// exists but contains no .md files, both returned values are empty strings.
func TestResolveReviewReport_LogDirExistsNoMDFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "output.txt"), "not a markdown file")
	writeFile(t, filepath.Join(dir, "data.json"), "{}")

	logger := discardLogger()
	gotContent, gotPath := resolveReviewReport("", dir, logger)
	assert.Empty(t, gotContent)
	assert.Empty(t, gotPath)
}

// ---- TestFirstConfiguredAgentName_TableDriven -------------------------------

// TestFirstConfiguredAgentName_TableDriven is a comprehensive table-driven test
// covering the priority logic of firstConfiguredAgentName.
func TestFirstConfiguredAgentName_TableDriven(t *testing.T) {
	tests := []struct {
		name      string
		agentCfgs map[string]config.AgentConfig
		want      string
	}{
		{
			name:      "nil map returns empty string",
			agentCfgs: nil,
			want:      "",
		},
		{
			name:      "empty map returns empty string",
			agentCfgs: map[string]config.AgentConfig{},
			want:      "",
		},
		{
			name: "only unknown agent returns empty string",
			agentCfgs: map[string]config.AgentConfig{
				"unknown-agent": {Command: "unknown"},
			},
			want: "",
		},
		{
			name: "claude with model only counts",
			agentCfgs: map[string]config.AgentConfig{
				"claude": {Model: "claude-opus-4-6"},
			},
			want: "claude",
		},
		{
			name: "codex with model only counts",
			agentCfgs: map[string]config.AgentConfig{
				"codex": {Model: "o3"},
			},
			want: "codex",
		},
		{
			name: "gemini with model only counts",
			agentCfgs: map[string]config.AgentConfig{
				"gemini": {Model: "gemini-2.5-pro"},
			},
			want: "gemini",
		},
		{
			name: "priority: claude > codex > gemini when all configured",
			agentCfgs: map[string]config.AgentConfig{
				"gemini": {Model: "gemini-2.5-pro"},
				"codex":  {Command: "codex"},
				"claude": {Command: "claude"},
			},
			want: "claude",
		},
		{
			name: "priority: codex > gemini when claude absent",
			agentCfgs: map[string]config.AgentConfig{
				"gemini": {Model: "gemini-2.5-pro"},
				"codex":  {Command: "codex"},
			},
			want: "codex",
		},
		{
			name: "claude with empty command and model is skipped, codex returned",
			agentCfgs: map[string]config.AgentConfig{
				"claude": {Command: "", Model: ""},
				"codex":  {Command: "codex"},
			},
			want: "codex",
		},
		{
			name: "all agents have empty command and model",
			agentCfgs: map[string]config.AgentConfig{
				"claude": {Command: "", Model: ""},
				"codex":  {Command: "", Model: ""},
				"gemini": {Command: "", Model: ""},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := firstConfiguredAgentName(tt.agentCfgs)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---- helpers -----------------------------------------------------------------

// writeFile creates a file with the given content at path for use in tests.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

// discardLogger returns a charmbracelet/log.Logger that discards all output,
// suitable for use in tests that exercise code paths accepting *log.Logger.
func discardLogger() *log.Logger {
	return log.New(io.Discard)
}
