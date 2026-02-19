package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/AbdelazizMoustafa10m/Raven/internal/buildinfo"
	"github.com/AbdelazizMoustafa10m/Raven/internal/logging"
	"github.com/AbdelazizMoustafa10m/Raven/internal/loop"
	"github.com/AbdelazizMoustafa10m/Raven/internal/pipeline"
	"github.com/AbdelazizMoustafa10m/Raven/internal/tui"
	"github.com/AbdelazizMoustafa10m/Raven/internal/workflow"
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
// It loads configuration, initializes backend subsystems (workflow engine,
// event channels), and launches the TUI with live event wiring.
// It respects the global --dry-run flag (flagDryRun) defined on the root command.
func runDashboard(cmd *cobra.Command, _ []string) error {
	if flagDryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "Would launch TUI dashboard (dry-run mode)")
		return nil
	}

	logger := logging.New("dashboard")

	// Load and resolve configuration from raven.toml, env, and CLI flags.
	projectName := ""
	resolved, _, err := loadAndResolveConfig()
	if err != nil {
		// Non-fatal: the dashboard can launch in idle mode without a config.
		logger.Warn("loading config failed; launching in idle mode", "error", err)
	} else {
		projectName = resolved.Config.Project.Name
	}

	// Set up a cancellation context that also responds to OS signals.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create buffered event channels for backend-to-TUI communication.
	workflowEvents := make(chan workflow.WorkflowEvent, 64)
	loopEvents := make(chan loop.LoopEvent, 64)
	agentOutput := make(chan tui.AgentOutputMsg, 256)
	taskProgress := make(chan tui.TaskProgressMsg, 64)

	// Initialize the workflow engine with real handler dependencies so that
	// pipeline execution triggered from the wizard can run steps.
	registry := workflow.NewRegistry()
	var handlerDeps *workflow.HandlerDeps
	if resolved != nil {
		deps, depsErr := buildRuntimeHandlerDeps(resolved.Config, pipeline.PipelineOpts{}, nil, nil)
		if depsErr != nil {
			logger.Warn("building runtime handler deps; pipeline steps may fail", "error", depsErr)
		} else {
			handlerDeps = deps
		}
	}
	workflow.RegisterBuiltinHandlers(registry, handlerDeps)

	var stateDir string
	if resolved != nil && resolved.Config.Project.LogDir != "" {
		stateDir = resolved.Config.Project.LogDir + "/state"
	} else {
		stateDir = ".raven/state"
	}
	store, storeErr := workflow.NewStateStore(stateDir)
	if storeErr != nil {
		logger.Warn("creating state store failed; checkpointing disabled", "error", storeErr)
	}

	engineOpts := []workflow.EngineOption{
		workflow.WithEventChannel(workflowEvents),
		workflow.WithLogger(logging.New("workflow")),
	}
	if store != nil {
		engineOpts = append(engineOpts, workflow.WithCheckpointing(store))
	}
	engine := workflow.NewEngine(registry, engineOpts...)

	info := buildinfo.GetInfo()
	cfg := tui.AppConfig{
		Version:        info.Version,
		ProjectName:    projectName,
		Ctx:            ctx,
		Cancel:         cancel,
		WorkflowEvents: workflowEvents,
		LoopEvents:     loopEvents,
		AgentOutput:    agentOutput,
		TaskProgress:   taskProgress,
		Engine:         engine,
	}

	logger.Info("launching TUI dashboard",
		"version", info.Version,
		"project", projectName,
	)

	return tui.RunTUI(cfg)
}
