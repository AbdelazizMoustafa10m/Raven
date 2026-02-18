package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/buildinfo"
)

// resetVersionFlags resets the version command's local flag state so tests
// do not leak state between runs. It calls resetRootCmd and also resets
// the versionJSON package variable and the --json flag's Changed tracking.
func resetVersionFlags(t *testing.T) {
	t.Helper()
	resetRootCmd(t)
	versionJSON = false
	versionCmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
	})
}

func TestVersionCmd_HumanReadable(t *testing.T) {
	resetVersionFlags(t)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	rootCmd.SetArgs([]string{"version"})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stdout = oldStdout

	assert.Equal(t, 0, code, "exit code should be 0")

	output := buf.String()
	assert.Contains(t, output, "raven v", "output should contain 'raven v' prefix")
	assert.Contains(t, output, buildinfo.Version, "output should contain the version")
	assert.Contains(t, output, buildinfo.Commit, "output should contain the commit")
	assert.Contains(t, output, buildinfo.Date, "output should contain the date")
}

func TestVersionCmd_DefaultValues(t *testing.T) {
	resetVersionFlags(t)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	rootCmd.SetArgs([]string{"version"})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stdout = oldStdout

	assert.Equal(t, 0, code)

	output := buf.String()
	// Without ldflags, defaults are "dev", "unknown", "unknown".
	assert.Contains(t, output, "dev", "default version should be 'dev'")
	assert.Contains(t, output, "unknown", "default commit/date should be 'unknown'")
}

func TestVersionCmd_JSONOutput(t *testing.T) {
	resetVersionFlags(t)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	rootCmd.SetArgs([]string{"version", "--json"})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stdout = oldStdout

	assert.Equal(t, 0, code, "exit code should be 0")

	output := buf.Bytes()

	// Verify it is valid JSON.
	var parsed map[string]string
	err = json.Unmarshal(output, &parsed)
	require.NoError(t, err, "output must be valid JSON")

	// Verify expected fields.
	assert.Contains(t, parsed, "version", "JSON must contain 'version' field")
	assert.Contains(t, parsed, "commit", "JSON must contain 'commit' field")
	assert.Contains(t, parsed, "date", "JSON must contain 'date' field")

	// Verify exactly 3 fields (no extras).
	assert.Len(t, parsed, 3, "JSON should contain exactly 3 fields")

	// Verify values match buildinfo defaults.
	assert.Equal(t, buildinfo.Version, parsed["version"])
	assert.Equal(t, buildinfo.Commit, parsed["commit"])
	assert.Equal(t, buildinfo.Date, parsed["date"])
}

func TestVersionCmd_JSONOutput_Indented(t *testing.T) {
	resetVersionFlags(t)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	rootCmd.SetArgs([]string{"version", "--json"})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stdout = oldStdout

	assert.Equal(t, 0, code)

	output := buf.String()
	// Indented JSON should contain newlines and spaces.
	assert.Contains(t, output, "{\n", "JSON should be indented with newlines")
	assert.Contains(t, output, "  \"version\"", "JSON should use 2-space indent")
}

func TestVersionCmd_RejectsExtraArgs(t *testing.T) {
	resetVersionFlags(t)

	// Capture stderr.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldStderr
	})

	rootCmd.SetArgs([]string{"version", "unexpected-arg"})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stderr = oldStderr

	assert.Equal(t, 1, code, "extra args should cause exit code 1")
	assert.Contains(t, buf.String(), "unknown command",
		"error should indicate the unexpected argument")
}

func TestVersionCmd_RegisteredInRoot(t *testing.T) {
	// Verify that the version command is registered as a subcommand of root.
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "version" {
			found = true
			break
		}
	}
	assert.True(t, found, "version command must be registered in rootCmd")
}

func TestVersionCmd_AppearsInHelp(t *testing.T) {
	resetVersionFlags(t)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--help"})

	code := Execute()
	assert.Equal(t, 0, code)

	helpOutput := buf.String()
	assert.Contains(t, helpOutput, "version", "help output should list version command")
}

func TestVersionCmd_Metadata(t *testing.T) {
	assert.Equal(t, "version", versionCmd.Use)
	assert.Equal(t, "Show Raven version and build information", versionCmd.Short)
	assert.Contains(t, versionCmd.Long, "version")
	assert.Contains(t, versionCmd.Long, "git commit")
	assert.Contains(t, versionCmd.Long, "build date")
}

func TestVersionCmd_JSONFlag_Registered(t *testing.T) {
	flag := versionCmd.Flags().Lookup("json")
	require.NotNil(t, flag, "--json flag must be registered")
	assert.Equal(t, "false", flag.DefValue, "--json default should be false")
	assert.Equal(t, "Output version info as JSON", flag.Usage)
}

func TestVersionCmd_OutputToStdout_NotStderr(t *testing.T) {
	resetVersionFlags(t)

	// Capture both stdout and stderr.
	oldStdout := os.Stdout
	rOut, wOut, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = wOut

	oldStderr := os.Stderr
	rErr, wErr, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = wErr

	t.Cleanup(func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	})

	rootCmd.SetArgs([]string{"version"})

	code := Execute()

	wOut.Close()
	wErr.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	_, _ = stdoutBuf.ReadFrom(rOut)
	_, _ = stderrBuf.ReadFrom(rErr)

	os.Stdout = oldStdout
	os.Stderr = oldStderr

	assert.Equal(t, 0, code)
	assert.Contains(t, stdoutBuf.String(), "raven v",
		"version output should go to stdout")
	assert.NotContains(t, stderrBuf.String(), "raven v",
		"version output should not go to stderr")
}

func TestVersionCmd_JSONRoundTrip(t *testing.T) {
	resetVersionFlags(t)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	rootCmd.SetArgs([]string{"version", "--json"})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stdout = oldStdout

	assert.Equal(t, 0, code)

	// Unmarshal into buildinfo.Info and verify round-trip.
	var info buildinfo.Info
	err = json.Unmarshal(buf.Bytes(), &info)
	require.NoError(t, err, "JSON output should unmarshal to buildinfo.Info")

	expected := buildinfo.GetInfo()
	assert.Equal(t, expected, info, "round-tripped Info should match GetInfo()")
}
