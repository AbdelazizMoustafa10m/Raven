package cli

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	"github.com/AbdelazizMoustafa10m/Raven/internal/logging"
)

// Global flag values accessible to all subcommands.
var (
	flagVerbose bool
	flagQuiet   bool
	flagConfig  string
	flagDir     string
	flagDryRun  bool
	flagNoColor bool
)

// rootCmd is the base command for Raven.
var rootCmd = &cobra.Command{
	Use:   "raven",
	Short: "AI workflow orchestration command center",
	Long: `Raven is an AI workflow orchestration command center that manages the
full lifecycle of AI-assisted software development -- from PRD decomposition
to implementation, code review, fix application, and pull request creation.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	// RunE shows full help when invoked with no subcommand. Without RunE,
	// Cobra only prints the Long description (omitting Usage and Flags).
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Check env vars for flags not explicitly set on command line.
		if !cmd.Flags().Changed("verbose") && os.Getenv("RAVEN_VERBOSE") != "" {
			flagVerbose = true
		}
		if !cmd.Flags().Changed("quiet") && os.Getenv("RAVEN_QUIET") != "" {
			flagQuiet = true
		}
		if !cmd.Flags().Changed("no-color") && (os.Getenv("NO_COLOR") != "" || os.Getenv("RAVEN_NO_COLOR") != "") {
			flagNoColor = true
		}

		// Initialize logging.
		jsonFormat := os.Getenv("RAVEN_LOG_FORMAT") == "json"
		logging.Setup(flagVerbose, flagQuiet, jsonFormat)

		// Handle --no-color: disable colored output.
		if flagNoColor {
			lipgloss.SetColorProfile(termenv.Ascii)
		}

		// Handle --dir (change working directory).
		if flagDir != "" {
			if err := os.Chdir(flagDir); err != nil {
				return fmt.Errorf("changing directory to %s: %w", flagDir, err)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Enable verbose (debug) output (env: RAVEN_VERBOSE)")
	rootCmd.PersistentFlags().BoolVarP(&flagQuiet, "quiet", "q", false, "Suppress all output except errors (env: RAVEN_QUIET)")
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", "", "Path to raven.toml config file")
	rootCmd.PersistentFlags().StringVar(&flagDir, "dir", "", "Override working directory")
	rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "Show planned actions without executing")
	rootCmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "Disable colored output (env: RAVEN_NO_COLOR, NO_COLOR)")
}

// Execute runs the root command and returns the exit code.
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// NewRootCmd returns a new instance of the root command for use in external
// tools such as the shell completion generator. It initialises a fresh cobra
// command tree so that it can be used independently of the global rootCmd.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           rootCmd.Use,
		Short:         rootCmd.Short,
		Long:          rootCmd.Long,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// Attach all registered subcommands from the global tree.
	for _, child := range rootCmd.Commands() {
		cmd.AddCommand(child)
	}
	return cmd
}
