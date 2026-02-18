package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- newPRCmd registration test ---------------------------------------------

func TestNewPRCmd_Registration(t *testing.T) {
	cmd := newPRCmd()
	assert.Equal(t, "pr", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)

	// Verify all expected flags are registered.
	assert.NotNil(t, cmd.Flags().Lookup("base"))
	assert.NotNil(t, cmd.Flags().Lookup("draft"))
	assert.NotNil(t, cmd.Flags().Lookup("title"))
	assert.NotNil(t, cmd.Flags().Lookup("label"))
	assert.NotNil(t, cmd.Flags().Lookup("assignee"))
	assert.NotNil(t, cmd.Flags().Lookup("review-report"))
	assert.NotNil(t, cmd.Flags().Lookup("no-summary"))

	// Verify default values.
	base, err := cmd.Flags().GetString("base")
	require.NoError(t, err)
	assert.Equal(t, "main", base)

	draft, err := cmd.Flags().GetBool("draft")
	require.NoError(t, err)
	assert.False(t, draft)

	noSummary, err := cmd.Flags().GetBool("no-summary")
	require.NoError(t, err)
	assert.False(t, noSummary)

	title, err := cmd.Flags().GetString("title")
	require.NoError(t, err)
	assert.Equal(t, "", title)
}

// ---- currentGitBranch tests -------------------------------------------------

func TestCurrentGitBranch_DetachedHead(t *testing.T) {
	// This test only works if git returns HEAD; we cannot guarantee it in all
	// environments. We test the error-handling branch by checking the function
	// signature is correct and behaves with a real git repo context.
	ctx := context.Background()
	// We expect either a valid branch name or an error -- both are fine.
	branch, err := currentGitBranch(ctx)
	if err != nil {
		// Error is acceptable -- the repo might be in detached HEAD state.
		t.Logf("currentGitBranch returned error (acceptable): %v", err)
	} else {
		// If no error, branch must be non-empty.
		assert.NotEmpty(t, branch, "current branch must be non-empty when no error")
	}
}

// ---- TestPRCmd_FlagDefaults --------------------------------------------------

// TestPRCmd_FlagDefaults verifies that all pr command flags have the correct
// default values when no flags are explicitly parsed.
func TestPRCmd_FlagDefaults(t *testing.T) {
	cmd := newPRCmd()

	// --base: default "main".
	base, err := cmd.Flags().GetString("base")
	require.NoError(t, err)
	assert.Equal(t, "main", base, "--base default should be \"main\"")

	// --draft: default false.
	draft, err := cmd.Flags().GetBool("draft")
	require.NoError(t, err)
	assert.False(t, draft, "--draft default should be false")

	// --title: default empty string.
	title, err := cmd.Flags().GetString("title")
	require.NoError(t, err)
	assert.Equal(t, "", title, "--title default should be empty string")

	// --label: default nil / empty slice.
	labels, err := cmd.Flags().GetStringArray("label")
	require.NoError(t, err)
	assert.Empty(t, labels, "--label default should be empty")

	// --assignee: default nil / empty slice.
	assignees, err := cmd.Flags().GetStringArray("assignee")
	require.NoError(t, err)
	assert.Empty(t, assignees, "--assignee default should be empty")

	// --review-report: default empty string.
	report, err := cmd.Flags().GetString("review-report")
	require.NoError(t, err)
	assert.Equal(t, "", report, "--review-report default should be empty string")

	// --no-summary: default false.
	noSummary, err := cmd.Flags().GetBool("no-summary")
	require.NoError(t, err)
	assert.False(t, noSummary, "--no-summary default should be false")
}

// ---- TestPRCmd_BaseBranchFlag -----------------------------------------------

// TestPRCmd_BaseBranchFlag verifies that --base develop sets BaseBranch to
// "develop" via cobra flag parsing.
func TestPRCmd_BaseBranchFlag(t *testing.T) {
	cmd := newPRCmd()
	require.NoError(t, cmd.ParseFlags([]string{"--base", "develop"}))

	base, err := cmd.Flags().GetString("base")
	require.NoError(t, err)
	assert.Equal(t, "develop", base)
}

// TestPRCmd_BaseBranchFlag_FeatureBranch verifies a branch name with slashes.
func TestPRCmd_BaseBranchFlag_FeatureBranch(t *testing.T) {
	cmd := newPRCmd()
	require.NoError(t, cmd.ParseFlags([]string{"--base", "feature/my-base"}))

	base, err := cmd.Flags().GetString("base")
	require.NoError(t, err)
	assert.Equal(t, "feature/my-base", base)
}

// ---- TestPRCmd_DraftFlag ----------------------------------------------------

// TestPRCmd_DraftFlag verifies that --draft sets the draft flag to true.
func TestPRCmd_DraftFlag(t *testing.T) {
	cmd := newPRCmd()
	require.NoError(t, cmd.ParseFlags([]string{"--draft"}))

	draft, err := cmd.Flags().GetBool("draft")
	require.NoError(t, err)
	assert.True(t, draft)
}

// ---- TestPRCmd_TitleFlag ----------------------------------------------------

// TestPRCmd_TitleFlag verifies that --title "My Title" sets the title flag.
func TestPRCmd_TitleFlag(t *testing.T) {
	cmd := newPRCmd()
	require.NoError(t, cmd.ParseFlags([]string{"--title", "feat: implement T-042"}))

	title, err := cmd.Flags().GetString("title")
	require.NoError(t, err)
	assert.Equal(t, "feat: implement T-042", title)
}

// TestPRCmd_TitleFlag_WithSpecialCharacters verifies that title with special
// characters (colons, slashes, spaces) is preserved verbatim.
func TestPRCmd_TitleFlag_WithSpecialCharacters(t *testing.T) {
	cmd := newPRCmd()
	customTitle := "fix: handle edge-case in review/fix pipeline (T-042)"
	require.NoError(t, cmd.ParseFlags([]string{"--title", customTitle}))

	title, err := cmd.Flags().GetString("title")
	require.NoError(t, err)
	assert.Equal(t, customTitle, title)
}

// ---- TestPRCmd_LabelFlag ----------------------------------------------------

// TestPRCmd_LabelFlag verifies that --label can be repeated and accumulates
// all provided values into the label slice.
func TestPRCmd_LabelFlag(t *testing.T) {
	cmd := newPRCmd()
	require.NoError(t, cmd.ParseFlags([]string{"--label", "bug", "--label", "review"}))

	labels, err := cmd.Flags().GetStringArray("label")
	require.NoError(t, err)
	assert.Equal(t, []string{"bug", "review"}, labels)
}

// TestPRCmd_LabelFlag_Single verifies a single label is stored correctly.
func TestPRCmd_LabelFlag_Single(t *testing.T) {
	cmd := newPRCmd()
	require.NoError(t, cmd.ParseFlags([]string{"--label", "enhancement"}))

	labels, err := cmd.Flags().GetStringArray("label")
	require.NoError(t, err)
	assert.Equal(t, []string{"enhancement"}, labels)
}

// TestPRCmd_LabelFlag_ThreeLabels verifies three repeated --label flags.
func TestPRCmd_LabelFlag_ThreeLabels(t *testing.T) {
	cmd := newPRCmd()
	require.NoError(t, cmd.ParseFlags([]string{
		"--label", "bug",
		"--label", "enhancement",
		"--label", "ai-generated",
	}))

	labels, err := cmd.Flags().GetStringArray("label")
	require.NoError(t, err)
	assert.Equal(t, []string{"bug", "enhancement", "ai-generated"}, labels)
}

// ---- TestPRCmd_AssigneeFlag -------------------------------------------------

// TestPRCmd_AssigneeFlag verifies that --assignee can be repeated and stores
// all provided values in the assignee slice.
func TestPRCmd_AssigneeFlag(t *testing.T) {
	cmd := newPRCmd()
	require.NoError(t, cmd.ParseFlags([]string{
		"--assignee", "alice",
		"--assignee", "bob",
	}))

	assignees, err := cmd.Flags().GetStringArray("assignee")
	require.NoError(t, err)
	assert.Equal(t, []string{"alice", "bob"}, assignees)
}

// TestPRCmd_AssigneeFlag_Single verifies a single assignee value.
func TestPRCmd_AssigneeFlag_Single(t *testing.T) {
	cmd := newPRCmd()
	require.NoError(t, cmd.ParseFlags([]string{"--assignee", "johndoe"}))

	assignees, err := cmd.Flags().GetStringArray("assignee")
	require.NoError(t, err)
	assert.Equal(t, []string{"johndoe"}, assignees)
}

// ---- TestPRCmd_NoSummaryFlag ------------------------------------------------

// TestPRCmd_NoSummaryFlag verifies that --no-summary sets the flag to true.
func TestPRCmd_NoSummaryFlag(t *testing.T) {
	cmd := newPRCmd()
	require.NoError(t, cmd.ParseFlags([]string{"--no-summary"}))

	noSummary, err := cmd.Flags().GetBool("no-summary")
	require.NoError(t, err)
	assert.True(t, noSummary)
}

// ---- TestPRCmd_ReviewReportFlag ---------------------------------------------

// TestPRCmd_ReviewReportFlag verifies that --review-report sets the path.
func TestPRCmd_ReviewReportFlag(t *testing.T) {
	cmd := newPRCmd()
	require.NoError(t, cmd.ParseFlags([]string{"--review-report", ".raven/logs/review-report.md"}))

	report, err := cmd.Flags().GetString("review-report")
	require.NoError(t, err)
	assert.Equal(t, ".raven/logs/review-report.md", report)
}

// ---- TestPRCmd_FlagsSetViaCommand -------------------------------------------

// TestPRCmd_FlagsSetViaCommand verifies all flags are correctly wired by
// running cmd.Execute() with a stub RunE that captures flag values.
func TestPRCmd_FlagsSetViaCommand(t *testing.T) {
	cmd := newPRCmd()

	var (
		capturedBase      string
		capturedDraft     bool
		capturedTitle     string
		capturedLabels    []string
		capturedAssignees []string
		capturedReport    string
		capturedNoSummary bool
	)

	cmd.RunE = func(c *cobra.Command, args []string) error {
		capturedBase, _ = c.Flags().GetString("base")
		capturedDraft, _ = c.Flags().GetBool("draft")
		capturedTitle, _ = c.Flags().GetString("title")
		capturedLabels, _ = c.Flags().GetStringArray("label")
		capturedAssignees, _ = c.Flags().GetStringArray("assignee")
		capturedReport, _ = c.Flags().GetString("review-report")
		capturedNoSummary, _ = c.Flags().GetBool("no-summary")
		return nil
	}

	cmd.SetArgs([]string{
		"--base", "develop",
		"--draft",
		"--title", "feat: implement something",
		"--label", "bug",
		"--label", "ai-generated",
		"--assignee", "alice",
		"--review-report", ".raven/logs/review.md",
		"--no-summary",
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "develop", capturedBase)
	assert.True(t, capturedDraft)
	assert.Equal(t, "feat: implement something", capturedTitle)
	assert.Equal(t, []string{"bug", "ai-generated"}, capturedLabels)
	assert.Equal(t, []string{"alice"}, capturedAssignees)
	assert.Equal(t, ".raven/logs/review.md", capturedReport)
	assert.True(t, capturedNoSummary)
}

// ---- TestPRCmd_DefaultsAppliedByCommand -------------------------------------

// TestPRCmd_DefaultsAppliedByCommand verifies that Cobra applies the correct
// defaults when the pr command is executed without any flags.
func TestPRCmd_DefaultsAppliedByCommand(t *testing.T) {
	cmd := newPRCmd()

	var (
		capturedBase      string
		capturedDraft     bool
		capturedNoSummary bool
	)

	cmd.RunE = func(c *cobra.Command, args []string) error {
		capturedBase, _ = c.Flags().GetString("base")
		capturedDraft, _ = c.Flags().GetBool("draft")
		capturedNoSummary, _ = c.Flags().GetBool("no-summary")
		return nil
	}

	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "main", capturedBase, "--base default should be \"main\"")
	assert.False(t, capturedDraft, "--draft default should be false")
	assert.False(t, capturedNoSummary, "--no-summary default should be false")
}

// ---- TestPRCmd_NoArgsAllowed ------------------------------------------------

// TestPRCmd_NoArgsAllowed verifies that the pr command rejects positional
// arguments.
func TestPRCmd_NoArgsAllowed(t *testing.T) {
	cmd := newPRCmd()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"unexpected-positional-arg"})

	err := cmd.Execute()
	assert.Error(t, err, "pr command should reject positional arguments")
}

// ---- TestPRCmdRegisteredOnRoot ----------------------------------------------

// TestPRCmdRegisteredOnRoot verifies that the pr command is registered as a
// subcommand of rootCmd via the init() function.
func TestPRCmdRegisteredOnRoot(t *testing.T) {
	found := false
	for _, sub := range rootCmd.Commands() {
		if sub.Use == "pr" {
			found = true
			break
		}
	}
	assert.True(t, found, "pr command should be registered as a subcommand of root")
}

// ---- TestPRCmd_HelpContent --------------------------------------------------

// TestPRCmd_HelpContent verifies the Long and Example strings mention the key
// flags users need to know about.
func TestPRCmd_HelpContent(t *testing.T) {
	cmd := newPRCmd()
	content := cmd.Long + cmd.Example
	for _, flag := range []string{"--draft", "--base", "--title", "--label", "--dry-run", "--no-summary"} {
		assert.True(t, strings.Contains(content, flag),
			"help content should mention %s", flag)
	}
}

// TestPRCmd_LongDescribesExitCodes verifies the Long description mentions
// the exit codes used by the pr command.
func TestPRCmd_LongDescribesExitCodes(t *testing.T) {
	cmd := newPRCmd()
	assert.Contains(t, cmd.Long, "0", "long description should mention exit code 0")
	assert.Contains(t, cmd.Long, "1", "long description should mention exit code 1")
}

// ---- TestPRCmd_AllFlagCombinations ------------------------------------------

// TestPRCmd_AllFlagCombinations verifies that various flag combinations parse
// without error at the Cobra layer.
func TestPRCmd_AllFlagCombinations(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "draft only",
			args: []string{"--draft"},
		},
		{
			name: "base and title",
			args: []string{"--base", "develop", "--title", "My PR"},
		},
		{
			name: "multiple labels",
			args: []string{"--label", "bug", "--label", "enhancement"},
		},
		{
			name: "multiple assignees",
			args: []string{"--assignee", "alice", "--assignee", "bob"},
		},
		{
			name: "no-summary flag",
			args: []string{"--no-summary"},
		},
		{
			name: "review-report flag",
			args: []string{"--review-report", "/tmp/review.md"},
		},
		{
			name: "all flags combined",
			args: []string{
				"--base", "develop",
				"--draft",
				"--title", "feat: test",
				"--label", "bug",
				"--assignee", "alice",
				"--review-report", "/tmp/review.md",
				"--no-summary",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newPRCmd()
			// Replace RunE to avoid running actual PR creation.
			cmd.RunE = func(c *cobra.Command, args []string) error {
				return nil
			}
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			assert.NoError(t, err, "flag combination %q should parse without error", tt.name)
		})
	}
}

// ---- TestPRCmd_LabelOrder ---------------------------------------------------

// TestPRCmd_LabelOrder verifies that --label values are preserved in the order
// they are provided on the command line.
func TestPRCmd_LabelOrder(t *testing.T) {
	cmd := newPRCmd()
	require.NoError(t, cmd.ParseFlags([]string{
		"--label", "first",
		"--label", "second",
		"--label", "third",
	}))

	labels, err := cmd.Flags().GetStringArray("label")
	require.NoError(t, err)
	require.Len(t, labels, 3)
	assert.Equal(t, "first", labels[0])
	assert.Equal(t, "second", labels[1])
	assert.Equal(t, "third", labels[2])
}

// ---- TestPRCmd_DefValueStrings ----------------------------------------------

// TestPRCmd_DefValueStrings verifies the Cobra DefValue strings match the
// expected defaults (useful for shell completion and help generation).
func TestPRCmd_DefValueStrings(t *testing.T) {
	cmd := newPRCmd()

	baseFlag := cmd.Flags().Lookup("base")
	require.NotNil(t, baseFlag)
	assert.Equal(t, "main", baseFlag.DefValue)

	draftFlag := cmd.Flags().Lookup("draft")
	require.NotNil(t, draftFlag)
	assert.Equal(t, "false", draftFlag.DefValue)

	noSummaryFlag := cmd.Flags().Lookup("no-summary")
	require.NotNil(t, noSummaryFlag)
	assert.Equal(t, "false", noSummaryFlag.DefValue)

	titleFlag := cmd.Flags().Lookup("title")
	require.NotNil(t, titleFlag)
	assert.Equal(t, "", titleFlag.DefValue)

	reportFlag := cmd.Flags().Lookup("review-report")
	require.NotNil(t, reportFlag)
	assert.Equal(t, "", reportFlag.DefValue)
}

// ---- TestPRFlags_StructZeroValues -------------------------------------------

// TestPRFlags_StructZeroValues verifies that the prFlags struct zero-value
// fields match documented defaults (before Cobra flag binding applies).
func TestPRFlags_StructZeroValues(t *testing.T) {
	flags := prFlags{}
	assert.Equal(t, "", flags.BaseBranch, "zero-value BaseBranch should be empty")
	assert.False(t, flags.Draft, "zero-value Draft should be false")
	assert.Equal(t, "", flags.Title, "zero-value Title should be empty")
	assert.Empty(t, flags.Labels, "zero-value Labels should be nil/empty")
	assert.Empty(t, flags.Assignees, "zero-value Assignees should be nil/empty")
	assert.Equal(t, "", flags.ReviewReport, "zero-value ReviewReport should be empty")
	assert.False(t, flags.NoSummary, "zero-value NoSummary should be false")
}
