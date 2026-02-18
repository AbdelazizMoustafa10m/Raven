package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
	"github.com/AbdelazizMoustafa10m/Raven/internal/loop"
	"github.com/AbdelazizMoustafa10m/Raven/internal/logging"
	"github.com/AbdelazizMoustafa10m/Raven/internal/task"
)

// implementFlags holds parsed flag values for the implement command.
type implementFlags struct {
	// Agent is the agent name to use (required).
	Agent string
	// PhaseStr is the phase specifier: an integer string or "all".
	// Mutually exclusive with Task.
	PhaseStr string
	// Task is the specific task ID to implement. Mutually exclusive with PhaseStr.
	Task string
	// MaxIterations limits the number of loop iterations (default: 50).
	MaxIterations int
	// MaxLimitWaits limits the number of rate-limit wait cycles (default: 5).
	MaxLimitWaits int
	// Sleep is the seconds to sleep between iterations (default: 5).
	Sleep int
	// DryRun generates and displays prompts without invoking the agent.
	DryRun bool
	// Model overrides the agent's configured model.
	Model string
}

// newImplementCmd creates the "raven implement" command.
func newImplementCmd() *cobra.Command {
	var flags implementFlags

	cmd := &cobra.Command{
		Use:   "implement",
		Short: "Run the implementation loop for a phase or single task",
		Long: `Run the implementation loop, orchestrating an AI agent to implement tasks.

In phase mode (--phase), the loop iterates over all not-started tasks in the
specified phase, running the agent on each until the phase is complete or limits
are reached.

In single-task mode (--task), the loop runs the agent on the specified task ID
exactly once (unless retries are needed for rate limits).

Use --dry-run to preview generated prompts and agent commands without invoking
the agent.`,
		Example: `  # Implement all tasks in phase 2 using Claude
  raven implement --agent claude --phase 2

  # Implement all phases using Codex
  raven implement --agent codex --phase all

  # Implement a specific task
  raven implement --agent claude --task T-029

  # Dry-run: show prompts without invoking agent
  raven implement --agent claude --phase 2 --dry-run

  # Override model for this run
  raven implement --agent claude --phase 2 --model claude-opus-4-6

  # Custom iteration and wait limits
  raven implement --agent claude --phase 2 --max-iterations 100 --max-limit-waits 3 --sleep 10`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImplement(cmd, flags)
		},
	}

	// Required flags.
	cmd.Flags().StringVar(&flags.Agent, "agent", "", "Agent to use (required): claude, codex, gemini")
	_ = cmd.MarkFlagRequired("agent")

	// Phase/task flags (mutually exclusive, validated in RunE).
	cmd.Flags().StringVar(&flags.PhaseStr, "phase", "", `Phase to implement (integer or "all")`)
	cmd.Flags().StringVar(&flags.Task, "task", "", "Specific task ID to implement (e.g. T-029)")

	// Optional tuning flags with defaults.
	cmd.Flags().IntVar(&flags.MaxIterations, "max-iterations", 50, "Maximum number of loop iterations")
	cmd.Flags().IntVar(&flags.MaxLimitWaits, "max-limit-waits", 5, "Maximum number of rate-limit wait cycles")
	cmd.Flags().IntVar(&flags.Sleep, "sleep", 5, "Seconds to sleep between iterations")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Show prompts and commands without invoking the agent")
	cmd.Flags().StringVar(&flags.Model, "model", "", "Override the agent's configured model for this run")

	// Shell completion for --agent: provide list of known agent names.
	_ = cmd.RegisterFlagCompletionFunc("agent", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"claude", "codex", "gemini"}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func init() {
	rootCmd.AddCommand(newImplementCmd())
}

// runImplement is the RunE implementation for the implement command.
// It wires up configuration, task discovery, agent selection, prompt
// generation, and the implementation loop.
func runImplement(cmd *cobra.Command, flags implementFlags) error {
	rawLogger := logging.New("implement")
	// logger adapts *log.Logger (which uses Info(msg interface{}, ...)) to the
	// loop.Runner logger interface (which requires Info(msg string, ...)).
	logger := &runnerLogger{logger: rawLogger}

	// Step 1: Validate flags -- phase/task mutual exclusivity.
	phaseID, err := validateImplementFlags(flags)
	if err != nil {
		return err
	}

	// Step 2: Load and resolve configuration.
	resolved, _, err := loadAndResolveConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	cfg := resolved.Config

	// Step 3: Discover tasks from configured tasks directory.
	specs, err := task.DiscoverTasks(cfg.Project.TasksDir)
	if err != nil {
		return fmt.Errorf("discovering tasks in %q: %w", cfg.Project.TasksDir, err)
	}
	logger.Info("discovered tasks", "count", len(specs), "dir", cfg.Project.TasksDir)

	// Step 4: Create state manager.
	stateManager := task.NewStateManager(cfg.Project.TaskStateFile)

	// Step 5: Load phases (gracefully handle missing file for --task mode).
	var phases []task.Phase
	if cfg.Project.PhasesConf != "" {
		phases, err = task.LoadPhases(cfg.Project.PhasesConf)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("loading phases from %q: %w", cfg.Project.PhasesConf, err)
			}
			// phases.conf not found -- acceptable in --task mode.
			logger.Info("phases.conf not found; proceeding without phases",
				"path", cfg.Project.PhasesConf,
			)
		}
	}

	// In phase mode, verify the requested phase exists when a specific phase
	// was requested (i.e., not "all" and not the 0 sentinel).
	if flags.PhaseStr != "" && phaseID != 0 {
		if task.PhaseByID(phases, phaseID) == nil {
			return fmt.Errorf("phase %d not found in phases.conf", phaseID)
		}
	}

	// Step 6: Create task selector.
	selector := task.NewTaskSelector(specs, stateManager, phases)

	// Step 7: Build agent registry and register all known agents.
	registry, err := buildAgentRegistry(cfg.Agents, flags)
	if err != nil {
		return err
	}

	// Step 8: Look up the requested agent.
	ag, err := registry.Get(flags.Agent)
	if err != nil {
		available := registry.List()
		return fmt.Errorf(
			"unknown agent %q: available agents are: %s",
			flags.Agent,
			strings.Join(available, ", "),
		)
	}

	// Step 9: Check agent prerequisites.
	if checkErr := ag.CheckPrerequisites(); checkErr != nil {
		return fmt.Errorf("agent prerequisite check failed for %q: %w", flags.Agent, checkErr)
	}
	logger.Info("agent prerequisites satisfied", "agent", flags.Agent)

	// Step 10: Create prompt generator.
	promptGen, err := loop.NewPromptGenerator(cfg.Project.PromptDir)
	if err != nil {
		return fmt.Errorf("creating prompt generator: %w", err)
	}

	// Step 11: Create rate-limit coordinator with configured max waits.
	backoffCfg := agent.DefaultBackoffConfig()
	backoffCfg.MaxWaits = flags.MaxLimitWaits
	rateLimiter := agent.NewRateLimitCoordinator(backoffCfg)

	// Step 12: Create the loop runner.
	// Pass nil for the events channel -- the CLI prints via the logger.
	runner := loop.NewRunner(
		selector,
		promptGen,
		ag,
		stateManager,
		rateLimiter,
		cfg,
		phases,
		nil, // events channel (nil = no event fan-out in CLI mode)
		logger,
	)

	// Step 13: Build run configuration from flags.
	runCfg := loop.RunConfig{
		AgentName:     flags.Agent,
		PhaseID:       phaseID,
		TaskID:        flags.Task,
		MaxIterations: flags.MaxIterations,
		MaxLimitWaits: flags.MaxLimitWaits,
		SleepBetween:  time.Duration(flags.Sleep) * time.Second,
		DryRun:        flags.DryRun || flagDryRun, // honour global --dry-run too
	}

	// Determine template name from agent config.
	if agentCfg, ok := cfg.Agents[flags.Agent]; ok && agentCfg.PromptTemplate != "" {
		runCfg.TemplateName = agentCfg.PromptTemplate
	}

	// Step 14: Set up signal handling for graceful Ctrl+C.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Step 15: Run the loop.
	logger.Info("starting implement",
		"agent", flags.Agent,
		"phase", flags.PhaseStr,
		"task", flags.Task,
		"dryRun", runCfg.DryRun,
		"maxIterations", runCfg.MaxIterations,
	)

	if flags.Task != "" {
		// Single-task mode.
		err = runner.RunSingleTask(ctx, runCfg)
	} else if phaseID == 0 {
		// All-phases mode: phaseID 0 is the "all phases" sentinel.
		if len(phases) == 0 {
			return fmt.Errorf("--phase all requires at least one phase in phases.conf")
		}
		// SelectNext(0) would fail because phase 0 does not exist, so we
		// iterate over each known phase sequentially.
		err = runAllPhases(ctx, runner, runCfg, phases, logger)
	} else {
		// Single-phase mode.
		err = runner.Run(ctx, runCfg)
	}

	// Step 16: Map errors to appropriate exit signals.
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// User pressed Ctrl+C or context was cancelled.
			fmt.Fprintln(cmd.ErrOrStderr(), "\nImplementation loop cancelled.")
			// Exit code 3 for user cancellation. Cobra will print the error
			// and exit 1 unless we handle it ourselves. For now, return the
			// error and let the caller (main.go via Execute()) handle it.
			return err
		}
		return err
	}

	logger.Info("implementation complete", "agent", flags.Agent)
	return nil
}

// charmLogger is the minimal interface satisfied by *charmbracelet/log.Logger.
// It uses interface{} for the message argument, unlike the string-typed
// interfaces required by internal packages.
type charmLogger interface {
	Info(msg interface{}, kv ...interface{})
	Debug(msg interface{}, kv ...interface{})
}

// runnerLogger wraps a charmbracelet/log.Logger to satisfy the
// loop.Runner logger interface, which requires Info(msg string, ...) with a
// string first argument rather than interface{}.
type runnerLogger struct {
	logger charmLogger
}

func (l *runnerLogger) Info(msg string, kv ...interface{}) {
	l.logger.Info(msg, kv...)
}

func (l *runnerLogger) Debug(msg string, kv ...interface{}) {
	l.logger.Debug(msg, kv...)
}

// agentDebugLogger wraps a charmbracelet/log.Logger to satisfy the agent
// package's unexported claudeLogger and codexLogger interfaces, which require
// Debug(msg string, ...).
type agentDebugLogger struct {
	logger charmLogger
}

func (l *agentDebugLogger) Debug(msg string, kv ...interface{}) {
	l.logger.Debug(msg, kv...)
}

// runAllPhases runs the implementation loop for each phase in sequence.
// It is called when phaseID is 0 (the "all phases" sentinel). Each phase is
// run to completion before the next is started. If any phase encounters an
// error the loop stops immediately and returns that error.
func runAllPhases(
	ctx context.Context,
	runner *loop.Runner,
	baseCfg loop.RunConfig,
	phases []task.Phase,
	logger interface {
		Info(msg string, kv ...interface{})
	},
) error {
	for _, phase := range phases {
		select {
		case <-ctx.Done():
			return fmt.Errorf("all-phases loop cancelled: %w", ctx.Err())
		default:
		}

		logger.Info("starting phase", "phase", phase.ID, "name", phase.Name)

		phaseCfg := baseCfg
		phaseCfg.PhaseID = phase.ID

		if err := runner.Run(ctx, phaseCfg); err != nil {
			return fmt.Errorf("phase %d (%s): %w", phase.ID, phase.Name, err)
		}

		logger.Info("phase complete", "phase", phase.ID, "name", phase.Name)
	}
	return nil
}

// validateImplementFlags checks that exactly one of --phase or --task is
// specified, and that they are not both provided simultaneously. Returns the
// parsed phase ID (0 for "all") and any validation error.
func validateImplementFlags(flags implementFlags) (int, error) {
	phaseSet := flags.PhaseStr != ""
	taskSet := flags.Task != ""

	if !phaseSet && !taskSet {
		return 0, fmt.Errorf("either --phase or --task must be specified")
	}
	if phaseSet && taskSet {
		return 0, fmt.Errorf("--phase and --task are mutually exclusive; specify only one")
	}

	if taskSet {
		// Single-task mode; phaseID is irrelevant.
		return 0, nil
	}

	// Parse --phase value.
	lower := strings.ToLower(strings.TrimSpace(flags.PhaseStr))
	if lower == "all" {
		return 0, nil // 0 = all phases sentinel
	}

	n, err := strconv.Atoi(lower)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid --phase value %q: must be a non-negative integer or \"all\"", flags.PhaseStr)
	}
	// 0 is the all-phases sentinel (equivalent to "all").
	return n, nil
}

// buildAgentRegistry creates an agent registry populated with Claude, Codex,
// and Gemini adapters. Agent configurations are sourced from the resolved
// config (config.AgentConfig) and converted to agent.AgentConfig for the
// agent constructors. If --model is set and matches the selected agent, that
// agent's configured model is overridden.
func buildAgentRegistry(agentCfgs map[string]config.AgentConfig, flags implementFlags) (*agent.Registry, error) {
	registry := agent.NewRegistry()

	// toAgentCfg converts a config.AgentConfig to agent.AgentConfig.
	// Both types have identical fields; this conversion is required because
	// they are defined in separate packages.
	toAgentCfg := func(c config.AgentConfig) agent.AgentConfig {
		return agent.AgentConfig{
			Command:        c.Command,
			Model:          c.Model,
			Effort:         c.Effort,
			PromptTemplate: c.PromptTemplate,
			AllowedTools:   c.AllowedTools,
		}
	}

	// Retrieve configs and convert. Zero-value config.AgentConfig is safe.
	claudeCfg := toAgentCfg(agentCfgs["claude"])
	codexCfg := toAgentCfg(agentCfgs["codex"])
	geminiCfg := toAgentCfg(agentCfgs["gemini"])

	// Apply --model override only to the selected agent.
	if flags.Model != "" {
		switch flags.Agent {
		case "claude":
			claudeCfg.Model = flags.Model
		case "codex":
			codexCfg.Model = flags.Model
		case "gemini":
			geminiCfg.Model = flags.Model
		}
	}

	// Set default CLI commands when not configured.
	if claudeCfg.Command == "" {
		claudeCfg.Command = "claude"
	}
	if codexCfg.Command == "" {
		codexCfg.Command = "codex"
	}

	// Construct and register agents.
	// Wrap charmbracelet loggers in agentDebugLogger adapters to satisfy
	// the agent package's unexported logger interfaces (Debug(string, ...)).
	claudeLog := &agentDebugLogger{logger: logging.New("claude")}
	codexLog := &agentDebugLogger{logger: logging.New("codex")}

	if err := registry.Register(agent.NewClaudeAgent(claudeCfg, claudeLog)); err != nil {
		return nil, fmt.Errorf("registering claude agent: %w", err)
	}
	if err := registry.Register(agent.NewCodexAgent(codexCfg, codexLog)); err != nil {
		return nil, fmt.Errorf("registering codex agent: %w", err)
	}
	if err := registry.Register(agent.NewGeminiAgent(geminiCfg)); err != nil {
		return nil, fmt.Errorf("registering gemini agent: %w", err)
	}

	return registry, nil
}
