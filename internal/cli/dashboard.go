package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/AbdelazizMoustafa10m/Raven/internal/buildinfo"
	"github.com/AbdelazizMoustafa10m/Raven/internal/tui"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Launch the TUI command center",
	Long: `Launch the interactive Raven TUI Command Center.

The dashboard provides a real-time view of workflow execution, agent output,
event logs, and task progress. Use keyboard shortcuts (press ? for help) to
navigate panels, pause/resume workflows, and skip tasks.`,
	Args: cobra.NoArgs,
	RunE: runDashboard,
}

func init() {
	rootCmd.AddCommand(dashboardCmd)
}

// runDashboard is the RunE handler for the dashboard command.
// It respects the global --dry-run flag (flagDryRun) defined on the root command.
func runDashboard(cmd *cobra.Command, args []string) error {
	if flagDryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "Would launch TUI dashboard (dry-run mode)")
		return nil
	}

	info := buildinfo.GetInfo()
	cfg := tui.AppConfig{
		Version:     info.Version,
		ProjectName: "",
	}

	return tui.RunTUI(cfg)
}
