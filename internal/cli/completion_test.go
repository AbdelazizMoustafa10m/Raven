package cli

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompletionCmd_Bash(t *testing.T) {
	resetRootCmd(t)

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	rootCmd.SetArgs([]string{"completion", "bash"})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stdout = oldStdout

	assert.Equal(t, 0, code, "exit code should be 0")
	output := buf.String()
	assert.NotEmpty(t, output, "bash completion output should not be empty")
	assert.Contains(t, output, "bash", "bash completion should contain 'bash'")
}

func TestCompletionCmd_Zsh(t *testing.T) {
	resetRootCmd(t)

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	rootCmd.SetArgs([]string{"completion", "zsh"})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stdout = oldStdout

	assert.Equal(t, 0, code, "exit code should be 0")
	output := buf.String()
	assert.NotEmpty(t, output, "zsh completion output should not be empty")
	// Cobra's zsh completion references the compdef function or _raven.
	assert.True(t,
		bytes.Contains(buf.Bytes(), []byte("zsh")) ||
			bytes.Contains(buf.Bytes(), []byte("_raven")) ||
			bytes.Contains(buf.Bytes(), []byte("compdef")),
		"zsh completion should contain 'zsh', '_raven', or 'compdef'")
}

func TestCompletionCmd_Fish(t *testing.T) {
	resetRootCmd(t)

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	rootCmd.SetArgs([]string{"completion", "fish"})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stdout = oldStdout

	assert.Equal(t, 0, code, "exit code should be 0")
	output := buf.String()
	assert.NotEmpty(t, output, "fish completion output should not be empty")
	assert.True(t,
		bytes.Contains(buf.Bytes(), []byte("fish")) ||
			bytes.Contains(buf.Bytes(), []byte("complete")),
		"fish completion should contain 'fish' or 'complete'")
}

func TestCompletionCmd_PowerShell(t *testing.T) {
	resetRootCmd(t)

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	rootCmd.SetArgs([]string{"completion", "powershell"})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stdout = oldStdout

	assert.Equal(t, 0, code, "exit code should be 0")
	output := buf.String()
	assert.NotEmpty(t, output, "powershell completion output should not be empty")
	assert.True(t,
		bytes.Contains(buf.Bytes(), []byte("PowerShell")) ||
			bytes.Contains(buf.Bytes(), []byte("Register")),
		"powershell completion should contain 'PowerShell' or 'Register'")
}

func TestCompletionCmd_NoArgs(t *testing.T) {
	resetRootCmd(t)

	// Capture stderr for error output.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldStderr
	})

	rootCmd.SetArgs([]string{"completion"})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stderr = oldStderr

	assert.Equal(t, 1, code, "missing shell argument should cause exit code 1")
}

func TestCompletionCmd_InvalidShell(t *testing.T) {
	resetRootCmd(t)

	// Capture stderr for error output.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldStderr
	})

	rootCmd.SetArgs([]string{"completion", "ksh"})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stderr = oldStderr

	assert.Equal(t, 1, code, "invalid shell name should cause exit code 1")
	assert.Contains(t, buf.String(), "invalid argument",
		"error should indicate invalid argument")
}

func TestCompletionCmd_ExtraArgs(t *testing.T) {
	resetRootCmd(t)

	// Capture stderr for error output.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldStderr
	})

	rootCmd.SetArgs([]string{"completion", "bash", "extra"})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stderr = oldStderr

	assert.Equal(t, 1, code, "extra arguments should cause exit code 1")
}

func TestCompletionCmd_CaseSensitive(t *testing.T) {
	resetRootCmd(t)

	// Capture stderr for error output.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldStderr
	})

	rootCmd.SetArgs([]string{"completion", "Bash"})

	code := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stderr = oldStderr

	assert.Equal(t, 1, code, "mixed-case shell name should be rejected")
}

func TestCompletionCmd_RegisteredInRoot(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "completion [bash|zsh|fish|powershell]" {
			found = true
			break
		}
	}
	assert.True(t, found, "completion command must be registered in rootCmd")
}

func TestCompletionCmd_AppearsInHelp(t *testing.T) {
	resetRootCmd(t)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--help"})

	code := Execute()
	assert.Equal(t, 0, code)

	helpOutput := buf.String()
	assert.Contains(t, helpOutput, "completion",
		"help output should list completion command")
}

func TestCompletionCmd_Metadata(t *testing.T) {
	assert.Equal(t, "completion [bash|zsh|fish|powershell]", completionCmd.Use)
	assert.Equal(t, "Generate shell completion scripts", completionCmd.Short)
	assert.Contains(t, completionCmd.Long, "Generate shell completion scripts for Raven")
	assert.True(t, completionCmd.DisableFlagsInUseLine,
		"DisableFlagsInUseLine should be true")
}

func TestCompletionCmd_ValidArgs(t *testing.T) {
	expected := []string{"bash", "zsh", "fish", "powershell"}
	assert.Equal(t, expected, completionCmd.ValidArgs,
		"ValidArgs should contain bash, zsh, fish, powershell")
}

func TestCompletionCmd_HelpContainsInstallExamples(t *testing.T) {
	examples := []struct {
		name    string
		snippet string
	}{
		{name: "bash_linux", snippet: "/etc/bash_completion.d/raven"},
		{name: "bash_macos", snippet: "brew --prefix"},
		{name: "zsh_fpath", snippet: `"${fpath[1]}/_raven"`},
		{name: "zsh_alt", snippet: "~/.zsh/completions/_raven"},
		{name: "fish", snippet: "~/.config/fish/completions/raven.fish"},
		{name: "powershell", snippet: "raven.ps1"},
		{name: "powershell_profile", snippet: `. raven.ps1`},
	}

	for _, tt := range examples {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, completionCmd.Long, tt.snippet,
				"Long description should contain install example for %s", tt.name)
		})
	}
}

func TestCompletionCmd_AllShells_TableDriven(t *testing.T) {
	shells := []struct {
		name     string
		contains string
	}{
		{name: "bash", contains: "bash"},
		{name: "zsh", contains: "compdef"},
		{name: "fish", contains: "complete"},
		{name: "powershell", contains: "Register"},
	}

	for _, tt := range shells {
		t.Run(tt.name, func(t *testing.T) {
			resetRootCmd(t)

			oldStdout := os.Stdout
			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stdout = w
			t.Cleanup(func() {
				os.Stdout = oldStdout
			})

			rootCmd.SetArgs([]string{"completion", tt.name})

			code := Execute()

			w.Close()
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r)
			os.Stdout = oldStdout

			assert.Equal(t, 0, code, "exit code should be 0 for %s", tt.name)
			output := buf.String()
			assert.NotEmpty(t, output, "%s completion output should not be empty", tt.name)
			assert.Contains(t, output, tt.contains,
				"%s completion should contain %q", tt.name, tt.contains)
		})
	}
}

func TestCompletionCmd_OutputToStdout_NotStderr(t *testing.T) {
	resetRootCmd(t)

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

	rootCmd.SetArgs([]string{"completion", "bash"})

	code := Execute()

	wOut.Close()
	wErr.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	_, _ = stdoutBuf.ReadFrom(rOut)
	_, _ = stderrBuf.ReadFrom(rErr)

	os.Stdout = oldStdout
	os.Stderr = oldStderr

	assert.Equal(t, 0, code)
	assert.NotEmpty(t, stdoutBuf.String(),
		"completion output should go to stdout")
	// Stderr might contain logging output but should not contain the completion script.
	assert.NotContains(t, stderrBuf.String(), "bash_completion",
		"completion script should not go to stderr")
}
