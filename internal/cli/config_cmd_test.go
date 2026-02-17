package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
)

// ---- helpers ----------------------------------------------------------------

// resetConfigFlags resets root flags and also resets any command state used by
// the config commands. It must be called at the start of every test that
// invokes Execute() or inspects command output.
func resetConfigFlags(t *testing.T) {
	t.Helper()
	resetRootCmd(t)
}

// captureOutput runs Execute() with the provided args, capturing stdout and
// stderr. It returns (stdout, stderr, exitCode).
func captureOutput(t *testing.T, args ...string) (string, string, int) {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	rOut, wOut, err := os.Pipe()
	require.NoError(t, err)
	rErr, wErr, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = wOut
	os.Stderr = wErr
	t.Cleanup(func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	})

	rootCmd.SetArgs(args)

	code := Execute()

	wOut.Close()
	wErr.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	_, _ = stdoutBuf.ReadFrom(rOut)
	_, _ = stderrBuf.ReadFrom(rErr)

	os.Stdout = oldStdout
	os.Stderr = oldStderr

	return stdoutBuf.String(), stderrBuf.String(), code
}

// writeMinimalToml writes a minimal raven.toml to tmpDir and returns its path.
func writeMinimalToml(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "raven.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// ---- registration tests -----------------------------------------------------

func TestConfigCmd_RegisteredInRoot(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "config" {
			found = true
			break
		}
	}
	assert.True(t, found, "config command must be registered in rootCmd")
}

func TestConfigCmd_HasDebugSubcommand(t *testing.T) {
	found := false
	for _, cmd := range configCmd.Commands() {
		if cmd.Use == "debug" {
			found = true
			break
		}
	}
	assert.True(t, found, "debug subcommand must be registered in configCmd")
}

func TestConfigCmd_HasValidateSubcommand(t *testing.T) {
	found := false
	for _, cmd := range configCmd.Commands() {
		if cmd.Use == "validate" {
			found = true
			break
		}
	}
	assert.True(t, found, "validate subcommand must be registered in configCmd")
}

func TestConfigCmd_Metadata(t *testing.T) {
	assert.Equal(t, "config", configCmd.Use)
	assert.Equal(t, "Configuration management commands", configCmd.Short)
	assert.Contains(t, configCmd.Long, "Inspect")
}

func TestConfigDebugCmd_Metadata(t *testing.T) {
	assert.Equal(t, "debug", configDebugCmd.Use)
	assert.Contains(t, configDebugCmd.Short, "resolved configuration")
	assert.Contains(t, configDebugCmd.Long, "source")
}

func TestConfigValidateCmd_Metadata(t *testing.T) {
	assert.Equal(t, "validate", configValidateCmd.Use)
	assert.Contains(t, configValidateCmd.Short, "Validate")
}

// ---- "raven config" shows help ----------------------------------------------

func TestConfigCmd_NoSubcommand_ShowsHelp(t *testing.T) {
	resetConfigFlags(t)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"config"})

	code := Execute()

	assert.Equal(t, 0, code)
	output := buf.String()
	assert.Contains(t, output, "debug", "help should list debug subcommand")
	assert.Contains(t, output, "validate", "help should list validate subcommand")
}

// ---- configDebugCmd tests ---------------------------------------------------

func TestConfigDebugCmd_DefaultsOnly_NoFile(t *testing.T) {
	resetConfigFlags(t)

	// Use a temp dir with no raven.toml so only defaults apply.
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"config", "debug"})

	code := Execute()

	assert.Equal(t, 0, code)
	output := buf.String()

	// Should show "none found" when no file exists.
	assert.Contains(t, output, "none found", "should indicate no config file")

	// All sources should be "default".
	assert.Contains(t, output, "(source: default)", "all fields should show default source")
	assert.NotContains(t, output, "(source: file)", "no file source should appear")

	// Default values should be present.
	assert.Contains(t, output, "docs/tasks", "tasks_dir default should appear")
	assert.Contains(t, output, "phase/{phase_id}-{slug}", "branch_template default should appear")
}

func TestConfigDebugCmd_WithConfigFile(t *testing.T) {
	resetConfigFlags(t)

	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	writeMinimalToml(t, tmpDir, `
[project]
name = "myapp"
language = "go"
`)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"config", "debug"})

	code := Execute()

	assert.Equal(t, 0, code)
	output := buf.String()

	assert.Contains(t, output, "raven.toml", "should show config file path")
	assert.Contains(t, output, "myapp", "project.name should appear in output")
	assert.Contains(t, output, "(source: file)", "file-sourced fields should show file annotation")
	assert.Contains(t, output, "(source: default)", "default fields should still show default annotation")
}

func TestConfigDebugCmd_WithExplicitConfigFlag(t *testing.T) {
	resetConfigFlags(t)

	tmpDir := t.TempDir()
	cfgPath := writeMinimalToml(t, tmpDir, `
[project]
name = "flagproject"
language = "go"
`)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--config", cfgPath, "config", "debug"})

	code := Execute()

	assert.Equal(t, 0, code)
	output := buf.String()

	assert.Contains(t, output, cfgPath, "config file path should appear in output")
	assert.Contains(t, output, "flagproject", "project.name from explicit config should appear")
}

func TestConfigDebugCmd_ExplicitConfigFlag_FileNotFound(t *testing.T) {
	resetConfigFlags(t)

	_, _, code := captureOutput(t, "--config", "/nonexistent/path/raven.toml", "config", "debug")

	assert.Equal(t, 1, code, "missing explicit config should produce error exit code")
}

func TestConfigDebugCmd_ShowsAllProjectFields(t *testing.T) {
	resetConfigFlags(t)

	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"config", "debug"})

	code := Execute()
	require.Equal(t, 0, code)

	output := buf.String()

	// All project fields should appear.
	fields := []string{
		"name",
		"language",
		"tasks_dir",
		"task_state_file",
		"phases_conf",
		"progress_file",
		"log_dir",
		"prompt_dir",
		"branch_template",
		"verification_commands",
	}
	for _, field := range fields {
		assert.Contains(t, output, field, "project field %q should appear in debug output", field)
	}
}

func TestConfigDebugCmd_ShowsReviewSection(t *testing.T) {
	resetConfigFlags(t)

	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"config", "debug"})

	code := Execute()
	require.Equal(t, 0, code)

	output := buf.String()
	assert.Contains(t, output, "[review]", "review section header should appear")
	assert.Contains(t, output, "extensions", "review.extensions field should appear")
}

func TestConfigDebugCmd_WithAgents(t *testing.T) {
	resetConfigFlags(t)

	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	writeMinimalToml(t, tmpDir, `
[project]
name = "agenttest"
language = "go"

[agents.claude]
command = "claude"
model = "claude-opus-4-6"
effort = "high"
`)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"config", "debug"})

	code := Execute()
	require.Equal(t, 0, code)

	output := buf.String()
	assert.Contains(t, output, "[agents.claude]", "agents section should appear")
	assert.Contains(t, output, "claude-opus-4-6", "model value should appear")
}

func TestConfigDebugCmd_RejectsExtraArgs(t *testing.T) {
	resetConfigFlags(t)

	_, _, code := captureOutput(t, "config", "debug", "unexpected-arg")
	assert.Equal(t, 1, code, "extra args should produce exit code 1")
}

// ---- configValidateCmd tests ------------------------------------------------

func TestConfigValidateCmd_ValidConfig_ExitsZero(t *testing.T) {
	resetConfigFlags(t)

	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	// Create default dirs so validation finds them and produces no warnings.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "docs", "tasks"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "scripts", "logs"), 0o755))
	writeMinimalToml(t, tmpDir, `
[project]
name = "valid-project"
language = "go"
`)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"config", "validate"})

	code := Execute()

	assert.Equal(t, 0, code, "valid config should exit 0")
	output := buf.String()
	assert.Contains(t, output, "No issues found.", "should report no issues for valid config")
}

func TestConfigValidateCmd_InvalidConfig_ExitsOne(t *testing.T) {
	resetConfigFlags(t)

	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	// project.name is empty -- required field.
	writeMinimalToml(t, tmpDir, `
[project]
language = "go"
`)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"config", "validate"})

	code := Execute()

	assert.Equal(t, 1, code, "invalid config should exit 1")
	output := buf.String()
	assert.Contains(t, output, "project.name", "should mention the failing field")
	assert.Contains(t, output, "must not be empty", "should describe the error")
}

func TestConfigValidateCmd_WithWarnings(t *testing.T) {
	resetConfigFlags(t)

	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	// Valid name but tasks_dir does not exist -- should produce a warning.
	writeMinimalToml(t, tmpDir, `
[project]
name = "warn-project"
language = "go"
tasks_dir = "nonexistent-dir"
`)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"config", "validate"})

	code := Execute()

	// Warnings alone do not cause a non-zero exit.
	assert.Equal(t, 0, code, "warnings-only config should exit 0")
	output := buf.String()
	assert.Contains(t, output, "Warnings:", "should list warnings section")
	assert.Contains(t, output, "nonexistent-dir", "should mention the non-existent directory")
}

func TestConfigValidateCmd_DefaultsOnly_EmptyName_ExitsOne(t *testing.T) {
	resetConfigFlags(t)

	// No config file -- defaults have empty project.name, which is an error.
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"config", "validate"})

	code := Execute()

	assert.Equal(t, 1, code, "defaults with empty project.name should exit 1")
	output := buf.String()
	assert.Contains(t, output, "project.name", "should mention project.name error")
}

func TestConfigValidateCmd_UnknownKeys_ShowsWarning(t *testing.T) {
	resetConfigFlags(t)

	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	// project.taks_dir is a typo -- should show as unknown key warning.
	writeMinimalToml(t, tmpDir, `
[project]
name = "typo-project"
language = "go"
taks_dir = "docs/tasks"
`)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"config", "validate"})

	code := Execute()

	// Unknown keys are warnings, not errors. But project.name is present, so no errors.
	assert.Equal(t, 0, code, "unknown keys are warnings, exit 0 if no errors")
	output := buf.String()
	assert.Contains(t, output, "taks_dir", "unknown key should appear in warnings")
}

func TestConfigValidateCmd_RejectsExtraArgs(t *testing.T) {
	resetConfigFlags(t)

	_, _, code := captureOutput(t, "config", "validate", "unexpected-arg")
	assert.Equal(t, 1, code, "extra args should produce exit code 1")
}

func TestConfigValidateCmd_WithExplicitConfigFlag(t *testing.T) {
	resetConfigFlags(t)

	tmpDir := t.TempDir()
	cfgPath := writeMinimalToml(t, tmpDir, `
[project]
name = "explicit-flag-project"
language = "go"
`)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--config", cfgPath, "config", "validate"})

	code := Execute()

	assert.Equal(t, 0, code, "valid config via --config should exit 0")
}

// ---- formatResolvedConfig / formatValidationResult unit tests ---------------

func TestPrintResolvedConfig_DefaultSources(t *testing.T) {
	resolved := config.Resolve(config.NewDefaults(), nil, func(string) (string, bool) { return "", false }, nil)

	var buf bytes.Buffer
	configDebugCmd.SetOut(&buf)
	printResolvedConfig(configDebugCmd, resolved)
	configDebugCmd.SetOut(nil)

	output := buf.String()

	assert.Contains(t, output, "Configuration Debug")
	assert.Contains(t, output, "(source: default)")
	assert.NotContains(t, output, "(source: file)")
	assert.NotContains(t, output, "(source: env)")
	assert.NotContains(t, output, "(source: cli)")
}

func TestPrintResolvedConfig_FileSources(t *testing.T) {
	fileCfg := &config.Config{
		Project: config.ProjectConfig{
			Name:     "fileproject",
			Language: "go",
		},
	}
	resolved := config.Resolve(config.NewDefaults(), fileCfg, func(string) (string, bool) { return "", false }, nil)

	var buf bytes.Buffer
	configDebugCmd.SetOut(&buf)
	printResolvedConfig(configDebugCmd, resolved)
	configDebugCmd.SetOut(nil)

	output := buf.String()

	assert.Contains(t, output, "fileproject")
	assert.Contains(t, output, "(source: file)")
	assert.Contains(t, output, "(source: default)")
}

func TestPrintValidationResult_NoIssues(t *testing.T) {
	result := &config.ValidationResult{}

	var buf bytes.Buffer
	configValidateCmd.SetOut(&buf)
	printValidationResult(configValidateCmd, result)
	configValidateCmd.SetOut(nil)

	output := buf.String()

	assert.Contains(t, output, "No issues found.")
	assert.NotContains(t, output, "Errors:")
	assert.NotContains(t, output, "Warnings:")
}

func TestPrintValidationResult_WithErrors(t *testing.T) {
	fileCfg := &config.Config{
		Project: config.ProjectConfig{
			Name:     "",
			Language: "go",
		},
	}
	result := config.Validate(fileCfg, nil)

	var buf bytes.Buffer
	configValidateCmd.SetOut(&buf)
	printValidationResult(configValidateCmd, result)
	configValidateCmd.SetOut(nil)

	output := buf.String()

	assert.Contains(t, output, "Errors:")
	assert.Contains(t, output, "project.name")
	assert.Contains(t, output, "1 error(s)")
}

func TestPrintValidationResult_WithWarnings(t *testing.T) {
	fileCfg := &config.Config{
		Project: config.ProjectConfig{
			Name:     "warnproject",
			Language: "go",
			TasksDir: "/nonexistent/path/to/tasks",
		},
	}
	result := config.Validate(fileCfg, nil)

	var buf bytes.Buffer
	configValidateCmd.SetOut(&buf)
	printValidationResult(configValidateCmd, result)
	configValidateCmd.SetOut(nil)

	output := buf.String()

	assert.Contains(t, output, "Warnings:")
	assert.False(t, result.HasErrors(), "should have no errors")
	assert.True(t, result.HasWarnings(), "should have warnings")
}

// ---- fmtStr / fmtSlice unit tests -------------------------------------------

func TestFmtStr(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: `""`},
		{name: "simple", input: "hello", want: `"hello"`},
		{name: "with spaces", input: "hello world", want: `"hello world"`},
		{name: "with special chars", input: "docs/tasks", want: `"docs/tasks"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fmtStr(tt.input))
		})
	}
}

func TestFmtSlice(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  string
	}{
		{name: "empty", input: nil, want: "[]"},
		{name: "empty slice", input: []string{}, want: "[]"},
		{name: "one item", input: []string{"go build ./..."}, want: `["go build ./..."]`},
		{name: "two items", input: []string{"go build ./...", "go test ./..."}, want: `["go build ./...", "go test ./..."]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fmtSlice(tt.input))
		})
	}
}

// ---- loadAndResolveConfig unit tests ----------------------------------------

func TestLoadAndResolveConfig_NoFile(t *testing.T) {
	// Save and restore flagConfig.
	orig := flagConfig
	flagConfig = ""
	t.Cleanup(func() { flagConfig = orig })

	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	resolved, meta, err := loadAndResolveConfig()
	require.NoError(t, err)
	assert.NotNil(t, resolved)
	assert.Nil(t, meta, "meta should be nil when no file is found")
	assert.Empty(t, resolved.Path, "path should be empty when no file found")
}

func TestLoadAndResolveConfig_WithFile(t *testing.T) {
	orig := flagConfig
	flagConfig = ""
	t.Cleanup(func() { flagConfig = orig })

	tmpDir := t.TempDir()
	writeMinimalToml(t, tmpDir, `
[project]
name = "loadtest"
language = "go"
`)
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	resolved, meta, err := loadAndResolveConfig()
	require.NoError(t, err)
	assert.NotNil(t, resolved)
	assert.NotNil(t, meta, "meta should be non-nil when file was loaded")
	assert.Equal(t, "loadtest", resolved.Config.Project.Name)
	assert.NotEmpty(t, resolved.Path)
}

func TestLoadAndResolveConfig_ExplicitFlagPath(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := writeMinimalToml(t, tmpDir, `
[project]
name = "explicit"
language = "go"
`)

	orig := flagConfig
	flagConfig = cfgPath
	t.Cleanup(func() { flagConfig = orig })

	resolved, meta, err := loadAndResolveConfig()
	require.NoError(t, err)
	assert.NotNil(t, resolved)
	assert.NotNil(t, meta)
	assert.Equal(t, "explicit", resolved.Config.Project.Name)
	assert.Equal(t, cfgPath, resolved.Path)
}

func TestLoadAndResolveConfig_ExplicitFlagPath_Missing(t *testing.T) {
	orig := flagConfig
	flagConfig = "/nonexistent/raven.toml"
	t.Cleanup(func() { flagConfig = orig })

	_, _, err := loadAndResolveConfig()
	assert.Error(t, err, "should return error for missing explicit config file")
	assert.Contains(t, err.Error(), "loading config")
}

// ---- output routing: stdout not stderr --------------------------------------

func TestConfigDebugCmd_OutputToStdout(t *testing.T) {
	resetConfigFlags(t)

	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	stdout, stderr, code := captureOutput(t, "config", "debug")

	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Configuration Debug", "debug output should go to stdout")
	assert.NotContains(t, stderr, "Configuration Debug", "debug output should not go to stderr")
}

func TestConfigValidateCmd_OutputToStdout(t *testing.T) {
	resetConfigFlags(t)

	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	writeMinimalToml(t, tmpDir, `
[project]
name = "routingtest"
language = "go"
`)

	stdout, stderr, code := captureOutput(t, "config", "validate")

	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Configuration Validation", "validate output should go to stdout")
	assert.NotContains(t, stderr, "Configuration Validation", "validate output should not go to stderr")
}

// ---- sourceStyle tests ------------------------------------------------------

func TestSourceStyle_AllSources(t *testing.T) {
	sources := []config.ConfigSource{
		config.SourceDefault,
		config.SourceFile,
		config.SourceEnv,
		config.SourceCLI,
	}
	// Just verify sourceStyle returns without panicking for all known sources.
	for _, src := range sources {
		style := sourceStyle(src)
		rendered := style.Render(string(src))
		// rendered should at least contain the source name (stripped of ANSI in test env).
		assert.True(t, strings.Contains(rendered, string(src)) || len(rendered) > 0,
			"sourceStyle should produce non-empty render for source %q", src)
	}
}
