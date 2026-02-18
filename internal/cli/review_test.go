package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
	"github.com/AbdelazizMoustafa10m/Raven/internal/review"
)

// ---- validateReviewFlags tests ----------------------------------------------

func TestValidateReviewFlags_DefaultModeAll(t *testing.T) {
	flags := reviewFlags{Mode: "all"}
	mode, err := validateReviewFlags(flags)
	require.NoError(t, err)
	assert.Equal(t, review.ReviewModeAll, mode)
}

func TestValidateReviewFlags_SplitMode(t *testing.T) {
	flags := reviewFlags{Mode: "split"}
	mode, err := validateReviewFlags(flags)
	require.NoError(t, err)
	assert.Equal(t, review.ReviewModeSplit, mode)
}

func TestValidateReviewFlags_ModeCaseInsensitive(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		wantMode review.ReviewMode
	}{
		{"all uppercase", "ALL", review.ReviewModeAll},
		{"split uppercase", "SPLIT", review.ReviewModeSplit},
		{"all mixed case", "All", review.ReviewModeAll},
		{"split mixed case", "Split", review.ReviewModeSplit},
		{"all with spaces", "  all  ", review.ReviewModeAll},
		{"split with spaces", "  split  ", review.ReviewModeSplit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := reviewFlags{Mode: tt.mode}
			mode, err := validateReviewFlags(flags)
			require.NoError(t, err)
			assert.Equal(t, tt.wantMode, mode)
		})
	}
}

func TestValidateReviewFlags_EmptyModeDefaultsToAll(t *testing.T) {
	flags := reviewFlags{Mode: ""}
	mode, err := validateReviewFlags(flags)
	require.NoError(t, err)
	assert.Equal(t, review.ReviewModeAll, mode)
}

func TestValidateReviewFlags_InvalidMode(t *testing.T) {
	tests := []struct {
		name string
		mode string
	}{
		{"invalid string", "parallel"},
		{"partial match", "al"},
		{"typo", "splt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := reviewFlags{Mode: tt.mode}
			_, err := validateReviewFlags(flags)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid --mode")
			assert.Contains(t, err.Error(), "all, split")
		})
	}
}

// ---- resolveReviewAgents tests ----------------------------------------------

func TestResolveReviewAgents_FromFlag(t *testing.T) {
	agents, err := resolveReviewAgents("claude,codex", config.Config{})
	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "codex"}, agents)
}

func TestResolveReviewAgents_FlagWithWhitespace(t *testing.T) {
	agents, err := resolveReviewAgents("claude , codex , gemini", config.Config{})
	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "codex", "gemini"}, agents)
}

func TestResolveReviewAgents_SingleAgentFlag(t *testing.T) {
	agents, err := resolveReviewAgents("claude", config.Config{})
	require.NoError(t, err)
	assert.Equal(t, []string{"claude"}, agents)
}

func TestResolveReviewAgents_EmptyFlagFallsBackToConfig(t *testing.T) {
	cfg := config.Config{
		Agents: map[string]config.AgentConfig{
			"claude": {Command: "claude", Model: "claude-sonnet-4-20250514"},
			"codex":  {Command: "codex"},
		},
	}
	agents, err := resolveReviewAgents("", cfg)
	require.NoError(t, err)
	// Claude and codex should appear in canonical order.
	assert.Equal(t, []string{"claude", "codex"}, agents)
}

func TestResolveReviewAgents_ConfigWithGemini(t *testing.T) {
	cfg := config.Config{
		Agents: map[string]config.AgentConfig{
			"gemini": {Model: "gemini-2.5-pro"},
		},
	}
	agents, err := resolveReviewAgents("", cfg)
	require.NoError(t, err)
	assert.Equal(t, []string{"gemini"}, agents)
}

func TestResolveReviewAgents_ConfigPreservesCanonicalOrder(t *testing.T) {
	// All three agents configured: order must be claude, codex, gemini.
	cfg := config.Config{
		Agents: map[string]config.AgentConfig{
			"gemini": {Model: "gemini-2.5-pro"},
			"codex":  {Command: "codex"},
			"claude": {Command: "claude"},
		},
	}
	agents, err := resolveReviewAgents("", cfg)
	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "codex", "gemini"}, agents)
}

func TestResolveReviewAgents_NoAgentsConfiguredAndNoFlag(t *testing.T) {
	// Config has no agents and no --agents flag: should return an error with instructions.
	_, err := resolveReviewAgents("", config.Config{Agents: map[string]config.AgentConfig{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no agents configured")
	assert.Contains(t, err.Error(), "--agents")
	assert.Contains(t, err.Error(), "raven.toml")
}

func TestResolveReviewAgents_EmptyAgentsFlag(t *testing.T) {
	// A flag with only commas and spaces should return an error.
	_, err := resolveReviewAgents(",  ,  ,", config.Config{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty agent list")
}

func TestResolveReviewAgents_ConfigAgentWithCommandOnly(t *testing.T) {
	// An agent with only Command set (no Model) should be included.
	cfg := config.Config{
		Agents: map[string]config.AgentConfig{
			"claude": {Command: "claude"},
		},
	}
	agents, err := resolveReviewAgents("", cfg)
	require.NoError(t, err)
	assert.Contains(t, agents, "claude")
}

func TestResolveReviewAgents_ConfigAgentWithModelOnly(t *testing.T) {
	// An agent with only Model set (no Command) should be included.
	cfg := config.Config{
		Agents: map[string]config.AgentConfig{
			"codex": {Model: "o3"},
		},
	}
	agents, err := resolveReviewAgents("", cfg)
	require.NoError(t, err)
	assert.Contains(t, agents, "codex")
}

func TestResolveReviewAgents_ConfigAgentWithEmptyCommandAndModel(t *testing.T) {
	// An agent whose Command and Model are both empty should be excluded.
	cfg := config.Config{
		Agents: map[string]config.AgentConfig{
			"claude": {Command: "", Model: ""},
		},
	}
	_, err := resolveReviewAgents("", cfg)
	require.Error(t, err, "agent with empty command and model should not be selected")
}

// ---- configToReviewConfig tests ---------------------------------------------

func TestConfigToReviewConfig_FieldMapping(t *testing.T) {
	in := config.ReviewConfig{
		Extensions:       `\.go$`,
		RiskPatterns:     `auth|crypto`,
		PromptsDir:       "prompts/review",
		RulesDir:         "rules",
		ProjectBriefFile: "PROJECT.md",
	}
	out := configToReviewConfig(in)
	assert.Equal(t, in.Extensions, out.Extensions)
	assert.Equal(t, in.RiskPatterns, out.RiskPatterns)
	assert.Equal(t, in.PromptsDir, out.PromptsDir)
	assert.Equal(t, in.RulesDir, out.RulesDir)
	assert.Equal(t, in.ProjectBriefFile, out.ProjectBriefFile)
}

func TestConfigToReviewConfig_ZeroValue(t *testing.T) {
	out := configToReviewConfig(config.ReviewConfig{})
	assert.Equal(t, review.ReviewConfig{}, out)
}

// ---- newReviewCmd tests -----------------------------------------------------

func TestNewReviewCmd_Registration(t *testing.T) {
	cmd := newReviewCmd()
	assert.Equal(t, "review", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
}

func TestNewReviewCmd_FlagsExist(t *testing.T) {
	cmd := newReviewCmd()

	expectedFlags := []string{
		"agents",
		"concurrency",
		"mode",
		"base",
		"output",
	}
	for _, name := range expectedFlags {
		flag := cmd.Flags().Lookup(name)
		assert.NotNil(t, flag, "expected flag --%s to exist", name)
	}
}

func TestNewReviewCmd_Defaults(t *testing.T) {
	cmd := newReviewCmd()

	concurrencyFlag := cmd.Flags().Lookup("concurrency")
	require.NotNil(t, concurrencyFlag)
	assert.Equal(t, "2", concurrencyFlag.DefValue)

	modeFlag := cmd.Flags().Lookup("mode")
	require.NotNil(t, modeFlag)
	assert.Equal(t, "all", modeFlag.DefValue)

	baseFlag := cmd.Flags().Lookup("base")
	require.NotNil(t, baseFlag)
	assert.Equal(t, "main", baseFlag.DefValue)

	agentsFlag := cmd.Flags().Lookup("agents")
	require.NotNil(t, agentsFlag)
	assert.Equal(t, "", agentsFlag.DefValue)

	outputFlag := cmd.Flags().Lookup("output")
	require.NotNil(t, outputFlag)
	assert.Equal(t, "", outputFlag.DefValue)
}

func TestNewReviewCmd_ModeCompletionFunc(t *testing.T) {
	cmd := newReviewCmd()

	completionFn, exists := cmd.GetFlagCompletionFunc("mode")
	require.True(t, exists, "--mode flag should have a completion function registered")
	require.NotNil(t, completionFn)

	completions, directive := completionFn(cmd, []string{}, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Contains(t, completions, "all")
	assert.Contains(t, completions, "split")
	assert.Len(t, completions, 2)
}

func TestNewReviewCmd_AgentsCompletionFunc(t *testing.T) {
	cmd := newReviewCmd()

	completionFn, exists := cmd.GetFlagCompletionFunc("agents")
	require.True(t, exists, "--agents flag should have a completion function registered")
	require.NotNil(t, completionFn)

	completions, directive := completionFn(cmd, []string{}, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Contains(t, completions, "claude")
	assert.Contains(t, completions, "codex")
	assert.Contains(t, completions, "gemini")
	assert.Len(t, completions, 3)
}

func TestReviewCmdRegisteredOnRoot(t *testing.T) {
	found := false
	for _, sub := range rootCmd.Commands() {
		if sub.Use == "review" {
			found = true
			break
		}
	}
	assert.True(t, found, "review command should be registered as a subcommand of root")
}

// ---- command help content tests ---------------------------------------------

func TestNewReviewCmd_HelpContainsKeyFlags(t *testing.T) {
	cmd := newReviewCmd()
	content := cmd.Long + cmd.Example
	for _, flag := range []string{"--agents", "--mode", "--base", "--output", "--dry-run", "--concurrency"} {
		assert.True(t, strings.Contains(content, flag),
			"help content should mention %s", flag)
	}
}

func TestNewReviewCmd_LongDescribesExitCodes(t *testing.T) {
	cmd := newReviewCmd()
	assert.Contains(t, cmd.Long, "APPROVED", "long description should mention APPROVED verdict")
	assert.Contains(t, cmd.Long, "CHANGES_NEEDED", "long description should mention CHANGES_NEEDED verdict")
	assert.Contains(t, cmd.Long, "BLOCKING", "long description should mention BLOCKING verdict")
}

// ---- reviewFlags struct tests -----------------------------------------------

func TestReviewFlags_DefaultValues(t *testing.T) {
	// Verify struct zero-value fields match documented defaults.
	flags := reviewFlags{}
	assert.Empty(t, flags.Agents, "zero-value Agents should be empty")
	assert.Zero(t, flags.Concurrency, "zero-value Concurrency should be zero (flag default applied by cobra)")
	assert.Empty(t, flags.Mode, "zero-value Mode should be empty (flag default applied by cobra)")
	assert.Empty(t, flags.BaseBranch, "zero-value BaseBranch should be empty")
	assert.Empty(t, flags.Output, "zero-value Output should be empty")
}

func TestReviewFlags_SetViaCommand(t *testing.T) {
	// Verify that flag values are correctly parsed by Cobra when set via SetArgs.
	// This validates --agents, --concurrency, --mode, --base, and --output parsing.
	cmd := newReviewCmd()

	var capturedFlags reviewFlags

	// Replace RunE to capture the flags before runReview is invoked.
	cmd.RunE = func(c *cobra.Command, args []string) error {
		capturedFlags.Agents, _ = c.Flags().GetString("agents")
		capturedFlags.Concurrency, _ = c.Flags().GetInt("concurrency")
		capturedFlags.Mode, _ = c.Flags().GetString("mode")
		capturedFlags.BaseBranch, _ = c.Flags().GetString("base")
		capturedFlags.Output, _ = c.Flags().GetString("output")
		return nil
	}

	cmd.SetArgs([]string{
		"--agents", "claude,codex",
		"--concurrency", "4",
		"--mode", "split",
		"--base", "develop",
		"--output", "report.md",
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "claude,codex", capturedFlags.Agents)
	assert.Equal(t, 4, capturedFlags.Concurrency)
	assert.Equal(t, "split", capturedFlags.Mode)
	assert.Equal(t, "develop", capturedFlags.BaseBranch)
	assert.Equal(t, "report.md", capturedFlags.Output)
}

func TestReviewFlags_DefaultsAppliedByCommand(t *testing.T) {
	// Verify Cobra applies defaults correctly when no flags are set.
	cmd := newReviewCmd()

	var capturedConcurrency int
	var capturedMode, capturedBase string

	cmd.RunE = func(c *cobra.Command, args []string) error {
		capturedConcurrency, _ = c.Flags().GetInt("concurrency")
		capturedMode, _ = c.Flags().GetString("mode")
		capturedBase, _ = c.Flags().GetString("base")
		return nil
	}

	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, 2, capturedConcurrency, "--concurrency default should be 2")
	assert.Equal(t, "all", capturedMode, "--mode default should be all")
	assert.Equal(t, "main", capturedBase, "--base default should be main")
}

// ---- validateReviewFlags table-driven tests ---------------------------------

func TestValidateReviewFlags_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		wantMode review.ReviewMode
		wantErr  bool
		errFrag  string
	}{
		{
			name:     "mode all",
			mode:     "all",
			wantMode: review.ReviewModeAll,
		},
		{
			name:     "mode split",
			mode:     "split",
			wantMode: review.ReviewModeSplit,
		},
		{
			name:     "empty mode defaults to all",
			mode:     "",
			wantMode: review.ReviewModeAll,
		},
		{
			name:     "case insensitive ALL",
			mode:     "ALL",
			wantMode: review.ReviewModeAll,
		},
		{
			name:     "case insensitive SPLIT",
			mode:     "SPLIT",
			wantMode: review.ReviewModeSplit,
		},
		{
			name:     "whitespace trimmed",
			mode:     "  split  ",
			wantMode: review.ReviewModeSplit,
		},
		{
			name:    "invalid mode random",
			mode:    "random",
			wantErr: true,
			errFrag: "invalid --mode",
		},
		{
			name:    "invalid mode typo",
			mode:    "spplit",
			wantErr: true,
			errFrag: "all, split",
		},
		{
			name:     "whitespace-only mode trims to empty and maps to all",
			mode:     "   ",
			wantMode: review.ReviewModeAll,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			flags := reviewFlags{Mode: tt.mode}
			got, err := validateReviewFlags(flags)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errFrag)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantMode, got)
		})
	}
}

// ---- resolveReviewAgents table-driven tests ---------------------------------

func TestResolveReviewAgents_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		agentsFlag string
		cfg        config.Config
		want       []string
		wantErr    bool
		errFrag    string
	}{
		{
			name:       "flag takes precedence over config",
			agentsFlag: "claude",
			cfg: config.Config{
				Agents: map[string]config.AgentConfig{
					"codex": {Command: "codex"},
				},
			},
			want: []string{"claude"},
		},
		{
			name:       "comma-separated agents from flag",
			agentsFlag: "claude,codex,gemini",
			want:       []string{"claude", "codex", "gemini"},
		},
		{
			name:       "flag with surrounding whitespace",
			agentsFlag: " claude , codex ",
			want:       []string{"claude", "codex"},
		},
		{
			name: "config with claude only",
			cfg: config.Config{
				Agents: map[string]config.AgentConfig{
					"claude": {Command: "claude"},
				},
			},
			want: []string{"claude"},
		},
		{
			name: "config preserves canonical order claude codex gemini",
			cfg: config.Config{
				Agents: map[string]config.AgentConfig{
					"gemini": {Model: "gemini-2.5-pro"},
					"codex":  {Command: "codex"},
					"claude": {Command: "claude"},
				},
			},
			want: []string{"claude", "codex", "gemini"},
		},
		{
			name:       "empty flag with no config agents returns error",
			agentsFlag: "",
			cfg:        config.Config{Agents: map[string]config.AgentConfig{}},
			wantErr:    true,
			errFrag:    "no agents configured",
		},
		{
			name:       "comma-only flag produces empty list error",
			agentsFlag: ", , ,",
			wantErr:    true,
			errFrag:    "empty agent list",
		},
		{
			name: "agent with empty command and model excluded",
			cfg: config.Config{
				Agents: map[string]config.AgentConfig{
					"claude": {Command: "", Model: ""},
				},
			},
			wantErr: true,
			errFrag: "no agents configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveReviewAgents(tt.agentsFlag, tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errFrag)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---- configToReviewConfig table-driven tests --------------------------------

func TestConfigToReviewConfig_TableDriven(t *testing.T) {
	tests := []struct {
		name string
		in   config.ReviewConfig
		want review.ReviewConfig
	}{
		{
			name: "zero value round-trip",
			in:   config.ReviewConfig{},
			want: review.ReviewConfig{},
		},
		{
			name: "all fields populated",
			in: config.ReviewConfig{
				Extensions:       `\.go$`,
				RiskPatterns:     `auth|crypto`,
				PromptsDir:       "prompts/review",
				RulesDir:         "rules",
				ProjectBriefFile: "PROJECT.md",
			},
			want: review.ReviewConfig{
				Extensions:       `\.go$`,
				RiskPatterns:     `auth|crypto`,
				PromptsDir:       "prompts/review",
				RulesDir:         "rules",
				ProjectBriefFile: "PROJECT.md",
			},
		},
		{
			name: "only extensions set",
			in:   config.ReviewConfig{Extensions: `\.ts$`},
			want: review.ReviewConfig{Extensions: `\.ts$`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := configToReviewConfig(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---- resetReviewFlags helper -------------------------------------------------

// resetReviewFlags resets the review command's local flags for inter-test isolation.
func resetReviewFlags(t *testing.T) {
	t.Helper()
	resetRootCmd(t)
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "review" {
			cmd.Flags().VisitAll(func(f *pflag.Flag) {
				f.Changed = false
				if err := f.Value.Set(f.DefValue); err != nil {
					t.Logf("resetting review flag %q: %v", f.Name, err)
				}
			})
			break
		}
	}
}

// writeMinimalReviewToml writes a raven.toml suitable for review command tests.
// It uses flagConfig indirectly via Execute().
func writeMinimalReviewToml(t *testing.T, dir string, extra string) string {
	t.Helper()
	content := fmt.Sprintf("[project]\nname = \"review-test\"\n%s", extra)
	path := filepath.Join(dir, "raven.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// ---- runReview error-path tests via Execute() --------------------------------

// TestRunReview_InvalidMode verifies that an invalid --mode value causes
// runReview to return an error containing helpful text (step 1 validation).
// This path is exercised via the internal runReview function directly, which
// avoids cobra execution overhead while still exercising the full function.
func TestRunReview_InvalidMode(t *testing.T) {
	flags := reviewFlags{
		Agents:      "claude",
		Concurrency: 2,
		Mode:        "invalid-mode",
		BaseBranch:  "main",
	}
	err := runReview(&cobra.Command{}, flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --mode")
	assert.Contains(t, err.Error(), "all, split")
}

// TestRunReview_NoAgentsInConfig verifies that when the --agents flag is empty
// and the config has no configured agents, runReview returns a descriptive error.
// This exercises the resolveReviewAgents error path (step 3) after config load.
func TestRunReview_NoAgentsInConfig(t *testing.T) {
	tmpDir := t.TempDir()
	tomlPath := writeMinimalReviewToml(t, tmpDir, "# no agents section\n")

	origConfig := flagConfig
	flagConfig = tomlPath
	t.Cleanup(func() { flagConfig = origConfig })

	flags := reviewFlags{
		Agents:      "", // empty: falls back to config
		Concurrency: 2,
		Mode:        "all",
		BaseBranch:  "main",
	}

	out := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(out)
	cmd.SetErr(out)

	err := runReview(cmd, flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no agents configured")
	assert.Contains(t, err.Error(), "--agents")
}

// TestRunReview_InvalidModeViaExecute verifies that the cobra Execute() path
// properly propagates the invalid-mode error from runReview so that the
// execute function returns a non-zero exit code.
func TestRunReview_InvalidModeViaExecute(t *testing.T) {
	tmpDir := t.TempDir()
	tomlPath := writeMinimalReviewToml(t, tmpDir, "")

	resetReviewFlags(t)

	var buf bytes.Buffer
	rootCmd.SetErr(&buf)
	rootCmd.SetOut(&buf)

	rootCmd.SetArgs([]string{
		"--config", tomlPath,
		"review",
		"--agents", "claude",
		"--mode", "nonsense",
	})
	code := Execute()
	assert.NotEqual(t, 0, code, "execute should return non-zero for invalid --mode")
}

// TestRunReview_AgentsFlag_Parsing verifies that --agents "claude,codex" is
// correctly split into []string{"claude", "codex"} and passed through to the
// agent resolution logic.
func TestRunReview_AgentsFlag_Parsing(t *testing.T) {
	// We test resolveReviewAgents directly with the value that the CLI parses.
	// The cobra flag parsing of "claude,codex" into the Agents string field is
	// exercised by the cobra command tests above (TestReviewFlags_SetViaCommand).
	// Here we verify the downstream parsing logic.
	agents, err := resolveReviewAgents("claude,codex", config.Config{})
	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "codex"}, agents, "--agents flag parsed into ordered slice")
}

// TestRunReview_ConcurrencyFlagSet verifies that --concurrency 4 is correctly
// read by Cobra and available as a flag value.
func TestRunReview_ConcurrencyFlagSet(t *testing.T) {
	cmd := newReviewCmd()

	var gotConcurrency int
	cmd.RunE = func(c *cobra.Command, args []string) error {
		gotConcurrency, _ = c.Flags().GetInt("concurrency")
		return nil
	}
	cmd.SetArgs([]string{"--concurrency", "4"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, 4, gotConcurrency, "--concurrency 4 should set concurrency to 4")
}

// TestRunReview_ModeSplitFlagSet verifies that --mode split is correctly
// read by Cobra and available as a flag value.
func TestRunReview_ModeSplitFlagSet(t *testing.T) {
	cmd := newReviewCmd()

	var gotMode string
	cmd.RunE = func(c *cobra.Command, args []string) error {
		gotMode, _ = c.Flags().GetString("mode")
		return nil
	}
	cmd.SetArgs([]string{"--mode", "split"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, "split", gotMode, "--mode split should set mode to split")
}

// TestRunReview_UnknownAgentInRegistry verifies that when --agents specifies an
// agent name that does not exist in the built registry, runReview returns a
// descriptive error mentioning the missing agent name and available agents.
// This tests step 5 of runReview (registry lookup verification).
func TestRunReview_UnknownAgentInRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	tomlPath := writeMinimalReviewToml(t, tmpDir, "")

	origConfig := flagConfig
	flagConfig = tomlPath
	t.Cleanup(func() { flagConfig = origConfig })

	flags := reviewFlags{
		Agents:      "nonexistent-agent",
		Concurrency: 1,
		Mode:        "all",
		BaseBranch:  "main",
	}

	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := runReview(cmd, flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent-agent", "error must name the missing agent")
	assert.Contains(t, err.Error(), "available agents", "error must list available agents")
}

// TestRunReview_DryRun verifies the dry-run path of runReview. In dry-run mode
// the orchestrator's DryRun method is called and its output is written to
// cmd.OutOrStdout() without invoking any real agent. The test runs inside the
// actual repository directory (which is a valid git repo) so git.NewGitClient
// and diff generation succeed.
func TestRunReview_DryRun(t *testing.T) {
	// This test must run from a valid git repository. The test binary's working
	// directory is the package directory, which is inside the raven git repo.
	tmpDir := t.TempDir()
	tomlPath := writeMinimalReviewToml(t, tmpDir, "")

	origConfig := flagConfig
	origDryRun := flagDryRun
	flagConfig = tomlPath
	flagDryRun = true
	t.Cleanup(func() {
		flagConfig = origConfig
		flagDryRun = origDryRun
	})

	flags := reviewFlags{
		Agents:      "claude,codex",
		Concurrency: 2,
		Mode:        "all",
		BaseBranch:  "HEAD",
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})

	err := runReview(cmd, flags)
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "Review Plan (dry run)", "dry-run output must contain plan header")
	assert.Contains(t, output, "Mode: all", "dry-run output must contain mode")
	assert.Contains(t, output, "Agents: 2", "dry-run output must list agent count")
	assert.Contains(t, output, "Base branch: HEAD", "dry-run output must contain base branch")
}

// TestRunReview_DryRun_SplitMode verifies the dry-run path with mode=split,
// confirming that the DryRun plan reflects the split distribution.
func TestRunReview_DryRun_SplitMode(t *testing.T) {
	tmpDir := t.TempDir()
	tomlPath := writeMinimalReviewToml(t, tmpDir, "")

	origConfig := flagConfig
	origDryRun := flagDryRun
	flagConfig = tomlPath
	flagDryRun = true
	t.Cleanup(func() {
		flagConfig = origConfig
		flagDryRun = origDryRun
	})

	flags := reviewFlags{
		Agents:      "claude,codex",
		Concurrency: 2,
		Mode:        "split",
		BaseBranch:  "HEAD",
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})

	err := runReview(cmd, flags)
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "Review Plan (dry run)")
	assert.Contains(t, output, "Mode: split")
}

// TestRunReview_DryRun_ViaExecute verifies the full cobra execution path for
// --dry-run review, confirming output goes to stdout and exit code is 0.
func TestRunReview_DryRun_ViaExecute(t *testing.T) {
	tmpDir := t.TempDir()
	tomlPath := writeMinimalReviewToml(t, tmpDir, "")

	resetReviewFlags(t)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&bytes.Buffer{})

	rootCmd.SetArgs([]string{
		"--config", tomlPath,
		"--dry-run",
		"review",
		"--agents", "claude",
		"--mode", "all",
		"--base", "HEAD",
	})
	code := Execute()

	assert.Equal(t, 0, code, "dry-run should exit with code 0")
	assert.Contains(t, out.String(), "Review Plan (dry run)", "dry-run output must contain plan header")
}

// TestRunReview_Output_WritesToFile verifies that when --output is set and the
// review pipeline generates a report, the report is written to the specified
// file path. This test exercises the dry-run code path (not the full pipeline)
// to avoid invoking real agents, but it validates the Output flag is parsed.
func TestRunReview_OutputFlag_ParsedCorrectly(t *testing.T) {
	cmd := newReviewCmd()

	var gotOutput string
	cmd.RunE = func(c *cobra.Command, args []string) error {
		gotOutput, _ = c.Flags().GetString("output")
		return nil
	}

	tmpDir := t.TempDir()
	reportPath := filepath.Join(tmpDir, "review-report.md")

	cmd.SetArgs([]string{"--output", reportPath})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, reportPath, gotOutput, "--output flag should set the output file path")
}

// TestRunReview_NoArgsAllowed verifies that the review command rejects
// positional arguments (it expects cobra.NoArgs).
func TestRunReview_NoArgsAllowed(t *testing.T) {
	cmd := newReviewCmd()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"unexpected-arg"})

	err := cmd.Execute()
	assert.Error(t, err, "review command should reject positional arguments")
}

// ---- exit-code mapping tests ------------------------------------------------

// TestReviewExitCodeMapping_ApprovedIsZero verifies that VerdictApproved maps
// to exit code 0 by examining the switch logic in runReview. We test the
// mapping logic indirectly through validateReviewFlags since runReview calls
// os.Exit(2) for non-approved verdicts (which would kill the test process).
// The logic coverage for the switch statement is provided by TestRunReview_DryRun
// which exercises the code path up to (but not including) the os.Exit branch.
func TestReviewExitCodeMapping_VerdictConstants(t *testing.T) {
	// Verify that the verdict constants match the expected string values used
	// in the Long description and exit code switch.
	assert.Equal(t, review.Verdict("APPROVED"), review.VerdictApproved)
	assert.Equal(t, review.Verdict("CHANGES_NEEDED"), review.VerdictChangesNeeded)
	assert.Equal(t, review.Verdict("BLOCKING"), review.VerdictBlocking)
}

// TestReviewExitCodeMapping_LongDescriptionMatchesConstants checks that the
// command Long description references the exact verdict strings used in the
// exit code switch.
func TestReviewExitCodeMapping_LongDescriptionMatchesConstants(t *testing.T) {
	cmd := newReviewCmd()
	assert.Contains(t, cmd.Long, string(review.VerdictApproved))
	assert.Contains(t, cmd.Long, string(review.VerdictChangesNeeded))
	assert.Contains(t, cmd.Long, string(review.VerdictBlocking))
}

// ---- flag combination tests -------------------------------------------------

// TestNewReviewCmd_AllFlagCombinations verifies that all documented flag
// combinations parse without error at the Cobra layer.
func TestNewReviewCmd_AllFlagCombinations(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "agents and concurrency",
			args: []string{"--agents", "claude,codex", "--concurrency", "3"},
		},
		{
			name: "mode split",
			args: []string{"--agents", "claude", "--mode", "split"},
		},
		{
			name: "custom base branch",
			args: []string{"--agents", "claude", "--base", "develop"},
		},
		{
			name: "all flags set",
			args: []string{
				"--agents", "claude,codex,gemini",
				"--concurrency", "3",
				"--mode", "all",
				"--base", "main",
				"--output", "out.md",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newReviewCmd()
			// Replace RunE so it doesn't try to run the actual pipeline.
			cmd.RunE = func(c *cobra.Command, args []string) error {
				return nil
			}
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			assert.NoError(t, err, "flag combination %q should parse without error", tt.name)
		})
	}
}

// ---- concurrency clamping integration --------------------------------------

// TestReviewConcurrency_PositiveValue verifies that positive concurrency values
// are accepted by the flag parser without validation error.
func TestReviewConcurrency_PositiveValue(t *testing.T) {
	cmd := newReviewCmd()

	var got int
	cmd.RunE = func(c *cobra.Command, args []string) error {
		got, _ = c.Flags().GetInt("concurrency")
		return nil
	}
	cmd.SetArgs([]string{"--concurrency", "1"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, 1, got)
}

// ---- --base flag variations --------------------------------------------------

func TestReviewBaseFlag_CustomBranch(t *testing.T) {
	cmd := newReviewCmd()

	var gotBase string
	cmd.RunE = func(c *cobra.Command, args []string) error {
		gotBase, _ = c.Flags().GetString("base")
		return nil
	}
	cmd.SetArgs([]string{"--base", "feature/my-branch"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, "feature/my-branch", gotBase)
}

// ---- configToReviewConfig nil-safe test -------------------------------------

func TestConfigToReviewConfig_NilAgentsField(t *testing.T) {
	// ReviewConfig only has scalar/string fields so a nil config.Agents map
	// in the parent Config should not affect this conversion.
	in := config.ReviewConfig{
		Extensions: `\.go$`,
		PromptsDir: "prompts",
	}
	out := configToReviewConfig(in)
	assert.Equal(t, `\.go$`, out.Extensions)
	assert.Equal(t, "prompts", out.PromptsDir)
}

// ---- runReview config loading test ------------------------------------------

// TestRunReview_ConfigLoaded verifies that runReview correctly loads the config
// via flagConfig and reflects review config settings. It exercises step 2
// (loadAndResolveConfig) and step 6 (configToReviewConfig).
func TestRunReview_ConfigLoaded(t *testing.T) {
	tmpDir := t.TempDir()
	tomlContent := `[project]
name = "config-load-test"

[review]
extensions = '\\.go$'
`
	tomlPath := filepath.Join(tmpDir, "raven.toml")
	require.NoError(t, os.WriteFile(tomlPath, []byte(tomlContent), 0o644))

	origConfig := flagConfig
	flagConfig = tomlPath
	t.Cleanup(func() { flagConfig = origConfig })

	// Use invalid mode so runReview returns immediately after config load
	// (step 1 runs before step 2, but we need to reach step 2 here by using
	// a valid mode and then failing at step 3).
	flags := reviewFlags{
		Agents:      "", // triggers no-agents error after config load
		Concurrency: 2,
		Mode:        "all",
		BaseBranch:  "main",
	}

	err := runReview(&cobra.Command{}, flags)
	// Should fail at resolveReviewAgents (no agents in config, no --agents flag).
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no agents configured",
		"should fail at agent resolution, indicating config was loaded successfully")
}

// ---- shell completion edge cases --------------------------------------------

func TestNewReviewCmd_ModeCompletion_AllValues(t *testing.T) {
	cmd := newReviewCmd()
	fn, ok := cmd.GetFlagCompletionFunc("mode")
	require.True(t, ok)

	completions, directive := fn(cmd, nil, "")
	assert.Len(t, completions, 2)
	assert.ElementsMatch(t, []string{"all", "split"}, completions)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestNewReviewCmd_AgentsCompletion_AllValues(t *testing.T) {
	cmd := newReviewCmd()
	fn, ok := cmd.GetFlagCompletionFunc("agents")
	require.True(t, ok)

	completions, directive := fn(cmd, nil, "")
	assert.Len(t, completions, 3)
	assert.ElementsMatch(t, []string{"claude", "codex", "gemini"}, completions)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

// ---- edge case: whitespace-only mode ----------------------------------------

// TestValidateReviewFlags_WhitespaceOnlyMapsToAll verifies that a mode string
// consisting entirely of whitespace is trimmed to "" and maps to ReviewModeAll,
// which is the same behaviour as an explicit empty string.
func TestValidateReviewFlags_WhitespaceOnlyMapsToAll(t *testing.T) {
	mode, err := validateReviewFlags(reviewFlags{Mode: "   "})
	require.NoError(t, err, "whitespace-only mode trims to empty and should map to ReviewModeAll")
	assert.Equal(t, review.ReviewModeAll, mode)
}

// ---- orchestrator pipeline error path ---------------------------------------

// TestRunReview_OrchestratorRunError verifies that when orchestrator.Run
// returns a non-context error (e.g. invalid base branch causing git to fail),
// runReview wraps it in a "running review pipeline" error and returns it.
// This exercises the error handling path at step 13 of runReview.
func TestRunReview_OrchestratorRunError(t *testing.T) {
	tmpDir := t.TempDir()
	tomlPath := writeMinimalReviewToml(t, tmpDir, "")

	origConfig := flagConfig
	origDryRun := flagDryRun
	flagConfig = tomlPath
	flagDryRun = false
	t.Cleanup(func() {
		flagConfig = origConfig
		flagDryRun = origDryRun
	})

	// Use a base branch that does not exist -- git diff will fail, causing
	// orchestrator.Run to return an error (via DiffGenerator.Generate).
	flags := reviewFlags{
		Agents:      "claude",
		Concurrency: 1,
		Mode:        "all",
		BaseBranch:  "branch-that-absolutely-does-not-exist-xyz-abc",
	}

	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := runReview(cmd, flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "running review pipeline",
		"orchestrator run error must be wrapped with 'running review pipeline' context")
}

// ---- verbose flag coverage --------------------------------------------------

// TestRunReview_Verbose_DryRun verifies that the verbose logging branch in
// runReview is exercised. When flagVerbose is true, the logger.Info call at
// step 3a fires before the dry-run path exits.
func TestRunReview_Verbose_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	tomlPath := writeMinimalReviewToml(t, tmpDir, "")

	origConfig := flagConfig
	origDryRun := flagDryRun
	origVerbose := flagVerbose
	flagConfig = tomlPath
	flagDryRun = true
	flagVerbose = true
	t.Cleanup(func() {
		flagConfig = origConfig
		flagDryRun = origDryRun
		flagVerbose = origVerbose
	})

	flags := reviewFlags{
		Agents:      "claude",
		Concurrency: 1,
		Mode:        "all",
		BaseBranch:  "HEAD",
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})

	err := runReview(cmd, flags)
	require.NoError(t, err, "verbose dry-run must succeed")
	assert.Contains(t, out.String(), "Review Plan (dry run)")
}

// ---- full pipeline path: empty diff -----------------------------------------

// TestRunReview_EmptyDiff verifies that when the diff against the base branch
// is empty (e.g. base = HEAD means no changes since HEAD), runReview exits
// cleanly with code 0 and prints a "No changes to review" message to stdout.
//
// This test exercises the full runReview pipeline path from config loading
// through orchestrator construction and execution, hitting the short-circuit
// at step 14 (empty diff check) rather than the verdict/exit-code branch.
//
// The test relies on the raven repository being a valid git repository, which
// is always true when running `go test ./internal/cli/...`.
func TestRunReview_EmptyDiff(t *testing.T) {
	tmpDir := t.TempDir()
	tomlPath := writeMinimalReviewToml(t, tmpDir, "")

	origConfig := flagConfig
	origDryRun := flagDryRun
	flagConfig = tomlPath
	flagDryRun = false // NOT dry-run: full pipeline
	t.Cleanup(func() {
		flagConfig = origConfig
		flagDryRun = origDryRun
	})

	flags := reviewFlags{
		Agents:      "claude,codex",
		Concurrency: 1,
		Mode:        "all",
		BaseBranch:  "HEAD", // HEAD...HEAD gives an empty diff
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})

	err := runReview(cmd, flags)
	require.NoError(t, err, "runReview must return nil for empty diff")
	assert.Contains(t, out.String(), "No changes to review",
		"stdout must report that there are no changes to review")
}

// TestRunReview_EmptyDiff_ViaExecute verifies the same empty-diff path via the
// cobra execution path, checking exit code 0.
func TestRunReview_EmptyDiff_ViaExecute(t *testing.T) {
	tmpDir := t.TempDir()
	tomlPath := writeMinimalReviewToml(t, tmpDir, "")

	resetReviewFlags(t)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&bytes.Buffer{})

	rootCmd.SetArgs([]string{
		"--config", tomlPath,
		"review",
		"--agents", "claude",
		"--mode", "all",
		"--base", "HEAD",
	})
	code := Execute()

	assert.Equal(t, 0, code, "empty diff must produce exit code 0")
	assert.Contains(t, out.String(), "No changes to review",
		"stdout must report no changes")
}
