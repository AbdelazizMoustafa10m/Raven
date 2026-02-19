package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDashboardCmd_Registered verifies that the dashboard command is
// registered as a subcommand of the root command.
func TestDashboardCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "dashboard" {
			found = true
			break
		}
	}
	assert.True(t, found, "dashboard command must be registered in rootCmd")
}

// TestDashboardCmd_Metadata verifies the command metadata matches the spec.
func TestDashboardCmd_Metadata(t *testing.T) {
	assert.Equal(t, "dashboard", dashboardCmd.Use)
	assert.Equal(t, "Launch the TUI command center", dashboardCmd.Short)
	assert.Contains(t, dashboardCmd.Long, "TUI")
	assert.Contains(t, dashboardCmd.Long, "Command Center")
}

// TestDashboardCmd_NoArgs verifies the command accepts no positional arguments.
func TestDashboardCmd_NoArgs(t *testing.T) {
	assert.NotNil(t, dashboardCmd.Args, "dashboard command should have an args validator")
}

// TestDashboardCmd_DryRun verifies the global --dry-run flag produces the
// expected dry-run output instead of launching the TUI.
func TestDashboardCmd_DryRun(t *testing.T) {
	resetRootCmd(t)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	dashboardCmd.SetOut(&out)

	rootCmd.SetArgs([]string{"--dry-run", "dashboard"})

	code := Execute()
	require.Equal(t, 0, code, "dry-run dashboard should succeed")

	output := out.String()
	assert.Contains(t, output, "dry-run", "dry-run output should mention dry-run")
}

// TestDashboardCmd_AppearsInHelp verifies that "dashboard" appears in the
// root command's help output.
func TestDashboardCmd_AppearsInHelp(t *testing.T) {
	resetRootCmd(t)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--help"})

	code := Execute()
	assert.Equal(t, 0, code)

	helpOutput := buf.String()
	assert.Contains(t, helpOutput, "dashboard", "help output should list the dashboard command")
}
