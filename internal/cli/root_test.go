package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetRootCmd resets all global flag values and Cobra's internal "Changed"
// tracking to pristine state. This must be called at the start of every test
// that invokes Execute() or manipulates rootCmd.
func resetRootCmd(t *testing.T) {
	t.Helper()
	// Reset Go variable state immediately.
	flagVerbose = false
	flagQuiet = false
	flagConfig = ""
	flagDir = ""
	flagDryRun = false
	flagNoColor = false
	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	// Reset pflag "Changed" tracking so env var checks work correctly.
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
	})
}

// noopCmdName is the name of the test-only noop subcommand.
const noopCmdName = "__test_noop"

// addNoopCmd registers a minimal subcommand on rootCmd so that
// PersistentPreRunE is invoked during tests. Cobra does not call
// PersistentPreRunE when the root command has no RunE and no subcommand
// is given (it just prints help). This helper ensures the pre-run hook
// fires for tests that need to verify its behavior.
func addNoopCmd(t *testing.T) {
	t.Helper()
	noop := &cobra.Command{
		Use:    noopCmdName,
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	rootCmd.AddCommand(noop)
	t.Cleanup(func() {
		rootCmd.RemoveCommand(noop)
	})
}

func TestRootCmd_Use(t *testing.T) {
	assert.Equal(t, "raven", rootCmd.Use)
}

func TestRootCmd_Short(t *testing.T) {
	assert.Equal(t, "AI workflow orchestration command center", rootCmd.Short)
}

func TestRootCmd_Long(t *testing.T) {
	assert.Contains(t, rootCmd.Long, "AI workflow orchestration command center")
	assert.Contains(t, rootCmd.Long, "PRD decomposition")
}

func TestRootCmd_SilenceUsage(t *testing.T) {
	assert.True(t, rootCmd.SilenceUsage, "SilenceUsage must be true")
}

func TestRootCmd_SilenceErrors(t *testing.T) {
	assert.True(t, rootCmd.SilenceErrors, "SilenceErrors must be true")
}

func TestRootCmd_PersistentFlags(t *testing.T) {
	tests := []struct {
		name      string
		flagName  string
		shorthand string
	}{
		{name: "verbose", flagName: "verbose", shorthand: "v"},
		{name: "quiet", flagName: "quiet", shorthand: "q"},
		{name: "config", flagName: "config", shorthand: ""},
		{name: "dir", flagName: "dir", shorthand: ""},
		{name: "dry-run", flagName: "dry-run", shorthand: ""},
		{name: "no-color", flagName: "no-color", shorthand: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := rootCmd.PersistentFlags().Lookup(tt.flagName)
			require.NotNil(t, flag, "persistent flag %q must be registered", tt.flagName)
			if tt.shorthand != "" {
				assert.Equal(t, tt.shorthand, flag.Shorthand,
					"flag %q should have shorthand %q", tt.flagName, tt.shorthand)
			}
		})
	}
}

func TestRootCmd_FlagUsageContainsEnvHints(t *testing.T) {
	tests := []struct {
		flagName string
		envHint  string
	}{
		{flagName: "verbose", envHint: "RAVEN_VERBOSE"},
		{flagName: "quiet", envHint: "RAVEN_QUIET"},
		{flagName: "no-color", envHint: "RAVEN_NO_COLOR"},
		{flagName: "no-color", envHint: "NO_COLOR"},
	}

	for _, tt := range tests {
		t.Run(tt.flagName+"_"+tt.envHint, func(t *testing.T) {
			flag := rootCmd.PersistentFlags().Lookup(tt.flagName)
			require.NotNil(t, flag)
			assert.Contains(t, flag.Usage, tt.envHint,
				"flag %q usage should mention env var %q", tt.flagName, tt.envHint)
		})
	}
}

func TestExecute_NoSubcommand_ReturnsZero(t *testing.T) {
	resetRootCmd(t)

	code := Execute()
	assert.Equal(t, 0, code, "Execute with no subcommand should return exit code 0")
}

func TestExecute_UnknownSubcommand_ReturnsOne(t *testing.T) {
	resetRootCmd(t)
	// Register a known subcommand so Cobra can distinguish unknown ones.
	// Without any subcommands, Cobra just prints help for any input.
	addNoopCmd(t)

	// Capture stderr to verify error output.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldStderr
	})

	rootCmd.SetArgs([]string{"nonexistent-command"})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stderr = oldStderr

	assert.Equal(t, 1, code, "unknown subcommand should return exit code 1")
	assert.Contains(t, buf.String(), "unknown command",
		"stderr should contain error about unknown command")
}

func TestExecute_HelpFlag_ReturnsZero(t *testing.T) {
	resetRootCmd(t)

	rootCmd.SetArgs([]string{"--help"})

	code := Execute()
	assert.Equal(t, 0, code, "--help should return exit code 0")
}

func TestPersistentPreRunE_VerboseFlag(t *testing.T) {
	resetRootCmd(t)
	addNoopCmd(t)

	rootCmd.SetArgs([]string{"--verbose", noopCmdName})

	code := Execute()
	assert.Equal(t, 0, code)
	assert.True(t, flagVerbose, "flagVerbose should be set to true")
}

func TestPersistentPreRunE_QuietFlag(t *testing.T) {
	resetRootCmd(t)
	addNoopCmd(t)

	rootCmd.SetArgs([]string{"--quiet", noopCmdName})

	code := Execute()
	assert.Equal(t, 0, code)
	assert.True(t, flagQuiet, "flagQuiet should be set to true")
}

func TestPersistentPreRunE_ConfigFlag(t *testing.T) {
	resetRootCmd(t)
	addNoopCmd(t)

	rootCmd.SetArgs([]string{"--config", "/path/to/raven.toml", noopCmdName})

	code := Execute()
	assert.Equal(t, 0, code)
	assert.Equal(t, "/path/to/raven.toml", flagConfig,
		"flagConfig should store the provided path")
}

func TestPersistentPreRunE_DryRunFlag(t *testing.T) {
	resetRootCmd(t)
	addNoopCmd(t)

	rootCmd.SetArgs([]string{"--dry-run", noopCmdName})

	code := Execute()
	assert.Equal(t, 0, code)
	assert.True(t, flagDryRun, "flagDryRun should be set to true")
}

func TestPersistentPreRunE_DirFlag_ValidDirectory(t *testing.T) {
	resetRootCmd(t)
	addNoopCmd(t)

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
	})

	tmpDir := t.TempDir()

	rootCmd.SetArgs([]string{"--dir", tmpDir, noopCmdName})

	code := Execute()
	assert.Equal(t, 0, code)

	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Resolve symlinks for comparison (macOS /tmp -> /private/tmp).
	resolvedCwd, err := filepath.EvalSymlinks(cwd)
	require.NoError(t, err)
	resolvedTmp, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, resolvedTmp, resolvedCwd,
		"working directory should be changed to the --dir value")
}

func TestPersistentPreRunE_DirFlag_InvalidDirectory(t *testing.T) {
	resetRootCmd(t)
	addNoopCmd(t)

	// Capture stderr.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldStderr
	})

	rootCmd.SetArgs([]string{"--dir", "/nonexistent/path/that/does/not/exist", noopCmdName})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stderr = oldStderr

	assert.Equal(t, 1, code, "invalid --dir should return exit code 1")
	assert.Contains(t, buf.String(), "changing directory to",
		"error message should contain context about the directory change")
}

func TestPersistentPreRunE_DirFlag_RelativePath(t *testing.T) {
	resetRootCmd(t)
	addNoopCmd(t)

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
	})

	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0o755))

	// Change to tmpDir first so "subdir" is a valid relative path.
	require.NoError(t, os.Chdir(tmpDir))

	rootCmd.SetArgs([]string{"--dir", "subdir", noopCmdName})

	code := Execute()
	assert.Equal(t, 0, code)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	resolvedCwd, err := filepath.EvalSymlinks(cwd)
	require.NoError(t, err)
	resolvedSub, err := filepath.EvalSymlinks(subDir)
	require.NoError(t, err)

	assert.Equal(t, resolvedSub, resolvedCwd,
		"relative --dir should resolve relative to current CWD")
}

func TestPersistentPreRunE_DirFlag_PointsToFile(t *testing.T) {
	resetRootCmd(t)
	addNoopCmd(t)

	tmpFile := filepath.Join(t.TempDir(), "not-a-dir.txt")
	require.NoError(t, os.WriteFile(tmpFile, []byte("hello"), 0o644))

	// Capture stderr.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldStderr
	})

	rootCmd.SetArgs([]string{"--dir", tmpFile, noopCmdName})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stderr = oldStderr

	assert.Equal(t, 1, code, "--dir pointing to a file should return exit code 1")
	assert.Contains(t, buf.String(), "changing directory to",
		"error should mention directory change failure")
}

func TestPersistentPreRunE_NoColorFlag(t *testing.T) {
	resetRootCmd(t)
	addNoopCmd(t)

	rootCmd.SetArgs([]string{"--no-color", noopCmdName})

	code := Execute()
	assert.Equal(t, 0, code)
	assert.True(t, flagNoColor, "flagNoColor should be set to true")
}

func TestPersistentPreRunE_EnvVerbose(t *testing.T) {
	resetRootCmd(t)
	addNoopCmd(t)

	t.Setenv("RAVEN_VERBOSE", "1")

	rootCmd.SetArgs([]string{noopCmdName})

	code := Execute()
	assert.Equal(t, 0, code)
	assert.True(t, flagVerbose, "RAVEN_VERBOSE env should set flagVerbose to true")
}

func TestPersistentPreRunE_EnvQuiet(t *testing.T) {
	resetRootCmd(t)
	addNoopCmd(t)

	t.Setenv("RAVEN_QUIET", "1")

	rootCmd.SetArgs([]string{noopCmdName})

	code := Execute()
	assert.Equal(t, 0, code)
	assert.True(t, flagQuiet, "RAVEN_QUIET env should set flagQuiet to true")
}

func TestPersistentPreRunE_EnvNoColor(t *testing.T) {
	resetRootCmd(t)
	addNoopCmd(t)

	t.Setenv("NO_COLOR", "1")

	rootCmd.SetArgs([]string{noopCmdName})

	code := Execute()
	assert.Equal(t, 0, code)
	assert.True(t, flagNoColor, "NO_COLOR env should set flagNoColor to true")
}

func TestPersistentPreRunE_EnvRavenNoColor(t *testing.T) {
	resetRootCmd(t)
	addNoopCmd(t)

	t.Setenv("RAVEN_NO_COLOR", "1")

	rootCmd.SetArgs([]string{noopCmdName})

	code := Execute()
	assert.Equal(t, 0, code)
	assert.True(t, flagNoColor, "RAVEN_NO_COLOR env should set flagNoColor to true")
}

func TestPersistentPreRunE_VerboseAndQuiet_QuietWins(t *testing.T) {
	resetRootCmd(t)
	addNoopCmd(t)

	rootCmd.SetArgs([]string{"--verbose", "--quiet", noopCmdName})

	code := Execute()
	assert.Equal(t, 0, code)
	// Both flags are set; logging.Setup handles the precedence (quiet wins).
	assert.True(t, flagVerbose, "flagVerbose should be true")
	assert.True(t, flagQuiet, "flagQuiet should be true (quiet wins in logging.Setup)")
}

func TestPersistentPreRunE_ConfigNonexistentFile(t *testing.T) {
	resetRootCmd(t)
	addNoopCmd(t)

	// Config validation happens in T-009/T-010, not here.
	rootCmd.SetArgs([]string{"--config", "/does/not/exist/raven.toml", noopCmdName})

	code := Execute()
	assert.Equal(t, 0, code, "non-existent config file should not cause an error at this stage")
	assert.Equal(t, "/does/not/exist/raven.toml", flagConfig)
}

func TestRootCmd_HelpOutput_ContainsAllFlags(t *testing.T) {
	resetRootCmd(t)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--help"})

	code := Execute()
	assert.Equal(t, 0, code)

	helpOutput := buf.String()
	expectedFlags := []string{
		"--verbose",
		"--quiet",
		"--config",
		"--dir",
		"--dry-run",
		"--no-color",
		"-v",
		"-q",
	}

	for _, flag := range expectedFlags {
		assert.Contains(t, helpOutput, flag,
			"help output should contain %q", flag)
	}
}

func TestRootCmd_HelpOutput_ContainsUsage(t *testing.T) {
	resetRootCmd(t)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--help"})

	code := Execute()
	assert.Equal(t, 0, code)

	helpOutput := buf.String()
	assert.Contains(t, helpOutput, "Usage:", "help output should contain Usage section")
	assert.Contains(t, helpOutput, "Flags:", "help output should contain Flags section")
}
