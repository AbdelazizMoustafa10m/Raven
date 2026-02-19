package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
	"github.com/AbdelazizMoustafa10m/Raven/internal/git"
	"github.com/AbdelazizMoustafa10m/Raven/internal/logging"
	"github.com/AbdelazizMoustafa10m/Raven/internal/review"
)

// reviewFlags holds parsed flag values for the review command.
type reviewFlags struct {
	// Agents is a comma-separated list of agent names (e.g. "claude,codex").
	// When empty, agents are sourced from the resolved config.
	Agents string

	// Concurrency caps the number of agents running simultaneously.
	Concurrency int

	// Mode is the review distribution mode: "all" or "split".
	Mode string

	// BaseBranch is the git ref to diff against (e.g. "main").
	BaseBranch string

	// Output is an optional file path to write the report to.
	// When empty, the report is written to stdout.
	Output string
}

// newReviewCmd creates the "raven review" command.
func newReviewCmd() *cobra.Command {
	var flags reviewFlags

	cmd := &cobra.Command{
		Use:   "review",
		Short: "Run multi-agent parallel code review",
		Long: `Run a code review pipeline that fans out review requests to multiple AI agents
concurrently, consolidates the findings, and outputs a structured markdown report.

In "all" mode (default), every agent receives the full diff. In "split" mode,
files are partitioned across agents so each reviews a non-overlapping subset.

The exit code encodes the review verdict:
  0 - APPROVED: no blocking issues found
  1 - Error during review execution
  2 - CHANGES_NEEDED or BLOCKING: issues require attention

Use --dry-run to preview the planned review actions without invoking any agent.`,
		Example: `  # Review using agents from config
  raven review

  # Review with specific agents
  raven review --agents claude,codex

  # Review in split mode with custom base branch
  raven review --mode split --base develop

  # Review with 3 concurrent agents
  raven review --agents claude,codex,gemini --concurrency 3

  # Write report to file
  raven review --output review-report.md

  # Dry-run: show plan without invoking agents
  raven review --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReview(cmd, flags)
		},
	}

	// Optional flags with defaults.
	cmd.Flags().StringVar(&flags.Agents, "agents", "", "Comma-separated list of agent names (default: from config)")
	cmd.Flags().IntVar(&flags.Concurrency, "concurrency", 2, "Maximum concurrent review agents")
	cmd.Flags().StringVar(&flags.Mode, "mode", "all", `Review mode: "all" (full diff to every agent) or "split" (partition files across agents)`)
	cmd.Flags().StringVar(&flags.BaseBranch, "base", "main", "Base branch for diff")
	cmd.Flags().StringVar(&flags.Output, "output", "", "Write report to file instead of stdout")

	// Shell completion for --mode: provide the two valid mode values.
	_ = cmd.RegisterFlagCompletionFunc("mode", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"all", "split"}, cobra.ShellCompDirectiveNoFileComp
	})

	// Shell completion for --agents: list the known agent names.
	_ = cmd.RegisterFlagCompletionFunc("agents", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"claude", "codex", "gemini"}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func init() {
	rootCmd.AddCommand(newReviewCmd())
}

// runReview is the RunE implementation for the review command. It wires together
// config loading, agent registry construction, diff generation, prompt building,
// consolidation, and report generation.
func runReview(cmd *cobra.Command, flags reviewFlags) error {
	logger := logging.New("review")

	// Step 1: Validate flags.
	reviewMode, err := validateReviewFlags(flags)
	if err != nil {
		return err
	}

	// Step 2: Load and resolve configuration.
	resolved, _, err := loadAndResolveConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	cfg := resolved.Config

	// Step 3: Resolve the agent list from --agents flag or config.
	agents, err := resolveReviewAgents(flags.Agents, *cfg)
	if err != nil {
		return err
	}

	if flagVerbose {
		logger.Info("review configuration",
			"agents", agents,
			"mode", reviewMode,
			"base_branch", flags.BaseBranch,
			"concurrency", flags.Concurrency,
		)
	}

	// Step 4: Build agent registry.
	// buildAgentRegistry requires implementFlags for its --model override logic.
	// For review we pass a zero-value implementFlags (no model override needed).
	registry, err := buildAgentRegistry(cfg.Agents, implementFlags{})
	if err != nil {
		return err
	}

	// Step 5: Verify all requested agents are present in the registry.
	for _, name := range agents {
		if _, lookupErr := registry.Get(name); lookupErr != nil {
			available := registry.List()
			return fmt.Errorf(
				"unknown agent %q: available agents are: %s\n"+
					"Use --agents to specify a different agent or add [agents.%s] to raven.toml.",
				name,
				strings.Join(available, ", "),
				name,
			)
		}
	}

	// Step 6: Build review config from the resolved config.
	reviewCfg := configToReviewConfig(cfg.Review)

	// Step 7: Create git client for diff generation.
	gitClient, err := git.NewGitClient(".")
	if err != nil {
		return fmt.Errorf("initializing git client: %w", err)
	}

	// Step 8: Construct review pipeline components.
	diffGen, err := review.NewDiffGenerator(gitClient, reviewCfg, logger)
	if err != nil {
		return fmt.Errorf("creating diff generator: %w", err)
	}

	promptBuilder := review.NewPromptBuilder(reviewCfg, logger)
	consolidator := review.NewConsolidator(logger)

	// Step 9: Build review opts.
	// The global --dry-run flag (flagDryRun) is honoured alongside any command-level dry-run state.
	dryRun := flagDryRun
	opts := review.ReviewOpts{
		Agents:      agents,
		Concurrency: flags.Concurrency,
		Mode:        reviewMode,
		BaseBranch:  flags.BaseBranch,
		DryRun:      dryRun,
	}

	// Step 10: Set up signal handling for graceful Ctrl+C.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Step 11: Construct the orchestrator.
	// Pass nil for the events channel -- the CLI uses the logger directly.
	orchestrator := review.NewReviewOrchestrator(
		registry,
		diffGen,
		promptBuilder,
		consolidator,
		flags.Concurrency,
		logger,
		nil, // events channel (nil = no event fan-out in CLI mode)
	)

	// Step 12: Handle dry-run mode -- print the plan and exit 0.
	if dryRun {
		plan, dryErr := orchestrator.DryRun(ctx, opts)
		if dryErr != nil {
			if errors.Is(dryErr, context.Canceled) {
				fmt.Fprintln(cmd.ErrOrStderr(), "\nReview cancelled.")
				return dryErr
			}
			return fmt.Errorf("dry run: %w", dryErr)
		}
		fmt.Fprint(cmd.OutOrStdout(), plan)
		return nil
	}

	// Step 13: Execute the full review pipeline.
	logger.Info("starting review",
		"agents", agents,
		"mode", reviewMode,
		"base", flags.BaseBranch,
		"concurrency", flags.Concurrency,
	)

	result, err := orchestrator.Run(ctx, opts)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintln(cmd.ErrOrStderr(), "\nReview cancelled.")
			return err
		}
		return fmt.Errorf("running review pipeline: %w", err)
	}

	// Step 14: Handle empty diff -- no changed files to review.
	if result.DiffResult != nil && len(result.DiffResult.Files) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No changes to review (empty diff against base branch).")
		return nil
	}

	// Step 15: Log per-agent errors as warnings (non-fatal -- pipeline continues).
	for _, agErr := range result.AgentErrors {
		logger.Warn("agent encountered an error",
			"agent", agErr.Agent,
			"error", agErr.Message,
		)
	}

	// Step 16: Generate the markdown report.
	reportGen := review.NewReportGenerator(logger)
	report, err := reportGen.Generate(result.Consolidated, result.Stats, result.DiffResult)
	if err != nil {
		return fmt.Errorf("generating review report: %w", err)
	}

	// Step 17: Write report to file or stdout.
	if flags.Output != "" {
		if writeErr := os.WriteFile(flags.Output, []byte(report), 0o600); writeErr != nil {
			return fmt.Errorf("writing report to %q: %w", flags.Output, writeErr)
		}
		logger.Info("report written", "path", flags.Output)
	} else {
		fmt.Fprint(cmd.OutOrStdout(), report)
	}

	// Step 18: Map verdict to exit code.
	verdict := result.Consolidated.Verdict
	logger.Info("review complete",
		"verdict", verdict,
		"findings", len(result.Consolidated.Findings),
		"duration", result.Duration,
	)

	switch verdict {
	case review.VerdictApproved:
		// Exit code 0: review passed with no issues.
		return nil
	default:
		// Exit code 2: CHANGES_NEEDED or BLOCKING.
		// os.Exit(2) is required here because RunE returning a non-nil error always
		// produces exit code 1. Exit code 2 is a semantic signal (like git's exit 1
		// for "diff detected") and must bypass Cobra's error-handling path.
		os.Exit(2)
		return nil // unreachable; satisfies the compiler
	}
}

// validateReviewFlags validates the review command flags and returns the parsed
// ReviewMode. Returns an error if the mode value is not a recognised option.
func validateReviewFlags(flags reviewFlags) (review.ReviewMode, error) {
	switch strings.ToLower(strings.TrimSpace(flags.Mode)) {
	case "all", "":
		return review.ReviewModeAll, nil
	case "split":
		return review.ReviewModeSplit, nil
	default:
		return "", fmt.Errorf("invalid --mode %q: must be one of: all, split", flags.Mode)
	}
}

// resolveReviewAgents returns the ordered list of agent names for the review.
// If the --agents flag is non-empty, that value is split on commas and used
// directly. Otherwise, any agent in config that has a non-empty Command or
// Model is included (in the fixed order: claude, codex, gemini). If neither
// source yields any agents, an error with setup instructions is returned.
func resolveReviewAgents(agentsFlag string, cfg config.Config) ([]string, error) {
	if agentsFlag != "" {
		raw := strings.Split(agentsFlag, ",")
		agents := make([]string, 0, len(raw))
		for _, a := range raw {
			a = strings.TrimSpace(a)
			if a != "" {
				agents = append(agents, a)
			}
		}
		if len(agents) == 0 {
			return nil, fmt.Errorf("--agents value %q produced an empty agent list", agentsFlag)
		}
		return agents, nil
	}

	// Fall back to agents configured in raven.toml.
	var agents []string
	for _, name := range []string{"claude", "codex", "gemini"} {
		if ac, ok := cfg.Agents[name]; ok && (ac.Command != "" || ac.Model != "") {
			agents = append(agents, name)
		}
	}
	if len(agents) == 0 {
		return nil, fmt.Errorf(
			"no agents configured for review: use --agents to specify agents (e.g. --agents claude,codex) " +
				"or add [agents.claude] / [agents.codex] sections to raven.toml",
		)
	}
	return agents, nil
}

// configToReviewConfig converts a config.ReviewConfig to a review.ReviewConfig.
// Both types have identical fields; the conversion is required because they live
// in separate packages.
func configToReviewConfig(c config.ReviewConfig) review.ReviewConfig {
	return review.ReviewConfig{
		Extensions:       c.Extensions,
		RiskPatterns:     c.RiskPatterns,
		PromptsDir:       c.PromptsDir,
		RulesDir:         c.RulesDir,
		ProjectBriefFile: c.ProjectBriefFile,
	}
}
