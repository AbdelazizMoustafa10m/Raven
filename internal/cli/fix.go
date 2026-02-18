package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
	"github.com/AbdelazizMoustafa10m/Raven/internal/logging"
	"github.com/AbdelazizMoustafa10m/Raven/internal/review"
)

// fixFlags holds parsed flag values for the fix command.
type fixFlags struct {
	// Agent is the agent name to use for fixes (default: first configured agent).
	Agent string

	// MaxFixCycles is the maximum number of fix-verify cycles to attempt.
	MaxFixCycles int

	// ReviewReport is the path to a review report from "raven review".
	// When empty, the most recent .md file in LogDir is auto-detected.
	ReviewReport string
}

// newFixCmd creates the "raven fix" command.
func newFixCmd() *cobra.Command {
	var flags fixFlags

	cmd := &cobra.Command{
		Use:   "fix",
		Short: "Apply fixes from a review report using an AI agent",
		Long: `Apply fixes from a review report using an AI agent and verify the results.

The fix engine runs an iterative fix-verify cycle: it invokes the agent to apply
review findings, then runs the configured verification commands. If verification
fails and cycles remain, the process repeats with the previous failure context
appended to the prompt.

Exit codes:
  0 - All fixes applied and verification passed
  1 - Error during execution
  2 - Fixes applied but verification still failing after max cycles

Use --dry-run to preview the fix prompt without invoking any agent.`,
		Example: `  # Fix using agent from config and auto-detected review report
  raven fix

  # Fix using specific agent
  raven fix --agent claude

  # Fix using a specific review report
  raven fix --review-report .raven/logs/review-report.md

  # Limit fix cycles
  raven fix --max-fix-cycles 5

  # Dry-run: show fix prompt without invoking agent
  raven fix --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFix(cmd, flags)
		},
	}

	cmd.Flags().StringVar(&flags.Agent, "agent", "", "Agent to use for fixes (default: first configured agent)")
	cmd.Flags().IntVar(&flags.MaxFixCycles, "max-fix-cycles", 3, "Maximum number of fix-verify cycles")
	cmd.Flags().StringVar(&flags.ReviewReport, "review-report", "", "Path to review report from raven review (default: auto-detect most recent in log dir)")

	// Shell completion for --agent: list known agent names.
	_ = cmd.RegisterFlagCompletionFunc("agent", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"claude", "codex", "gemini"}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func init() {
	rootCmd.AddCommand(newFixCmd())
}

// runFix is the RunE implementation for the fix command.
func runFix(cmd *cobra.Command, flags fixFlags) error {
	logger := logging.New("fix")

	// Step 1: Load and resolve configuration.
	resolved, _, err := loadAndResolveConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	cfg := resolved.Config

	// Step 2: Resolve agent name -- use flag if provided, else first configured agent.
	agentName := flags.Agent
	if agentName == "" {
		agentName = firstConfiguredAgentName(cfg.Agents)
	}
	if agentName == "" {
		return fmt.Errorf(
			"no agent specified and no agents configured: use --agent to specify an agent (e.g. --agent claude) " +
				"or add [agents.claude] to raven.toml",
		)
	}

	// Step 3: Build agent registry.
	registry, err := buildAgentRegistry(cfg.Agents, implementFlags{})
	if err != nil {
		return err
	}

	// Step 4: Get agent from registry.
	ag, err := registry.Get(agentName)
	if err != nil {
		available := registry.List()
		return fmt.Errorf(
			"unknown agent %q: available agents are: %s\n"+
				"Use --agent to specify a different agent or add [agents.%s] to raven.toml.",
			agentName,
			strings.Join(available, ", "),
			agentName,
		)
	}

	// Step 5: Check agent prerequisites.
	if checkErr := ag.CheckPrerequisites(); checkErr != nil {
		return fmt.Errorf("agent prerequisite check failed for %q: %w", agentName, checkErr)
	}

	// Step 6: Build VerificationRunner from config.
	verifier := review.NewVerificationRunner(
		cfg.Project.VerificationCommands,
		"",            // use process working directory
		5*time.Minute, // per-command timeout
		logger,
	)

	// Step 7: Build FixEngine.
	fixEngine := review.NewFixEngine(ag, verifier, flags.MaxFixCycles, logger, nil)

	// Step 8: Build FixPromptBuilder with verification commands.
	// Conventions are not exposed in config yet -- use empty slice.
	pb := review.NewFixPromptBuilder(
		[]string{}, // conventions (empty for now)
		cfg.Project.VerificationCommands,
		logger,
	)
	fixEngine.WithPromptBuilder(pb)

	// Step 9: Resolve review report content.
	reportContent, reportPath := resolveReviewReport(flags.ReviewReport, cfg.Project.LogDir, logger)
	if reportPath != "" {
		logger.Info("using review report", "path", reportPath)
	}

	// Step 10: Build findings. If a review report was provided, create a synthetic
	// finding to ensure the fix engine actually runs (it fast-paths on empty findings).
	var findings []*review.Finding
	if reportContent != "" {
		findings = []*review.Finding{
			{
				File:        "review-report",
				Line:        0,
				Category:    "review",
				Severity:    review.SeverityMedium,
				Description: "Apply fixes from the review report.",
			},
		}
	}

	// Step 11: Determine effective dry-run flag (command-level or global).
	dryRun := flagDryRun

	// Step 12: Build FixOpts.
	opts := review.FixOpts{
		Findings:     findings,
		ReviewReport: reportContent,
		MaxCycles:    flags.MaxFixCycles,
		DryRun:       dryRun,
	}

	// Step 13: Set up signal context for graceful cancellation.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Step 14: Handle dry-run mode.
	if dryRun {
		prompt, dryErr := fixEngine.DryRun(ctx, opts)
		if dryErr != nil {
			if errors.Is(dryErr, context.Canceled) {
				fmt.Fprintln(cmd.ErrOrStderr(), "\nFix cancelled.")
				return dryErr
			}
			return fmt.Errorf("dry run: %w", dryErr)
		}
		if prompt == "" {
			fmt.Fprintln(cmd.OutOrStdout(), "(no findings to fix)")
		} else {
			fmt.Fprint(cmd.OutOrStdout(), prompt)
		}
		return nil
	}

	// Step 15: Log start and run the fix engine.
	logger.Info("starting fix",
		"agent", agentName,
		"max_cycles", flags.MaxFixCycles,
		"has_report", reportContent != "",
		"findings", len(findings),
	)

	report, fixErr := fixEngine.Fix(ctx, opts)
	if fixErr != nil {
		if errors.Is(fixErr, context.Canceled) || errors.Is(fixErr, context.DeadlineExceeded) {
			fmt.Fprintln(cmd.ErrOrStderr(), "\nFix cancelled.")
			return fixErr
		}
		return fmt.Errorf("running fix engine: %w", fixErr)
	}

	// Step 16: Map FixReport status to exit code.
	logger.Info("fix complete",
		"status", report.FinalStatus,
		"total_cycles", report.TotalCycles,
		"fixes_applied", report.FixesApplied,
		"duration", report.Duration,
	)

	switch report.FinalStatus {
	case review.VerificationPassed:
		// Exit code 0: fixes applied and verification passed (or no fixes needed).
		return nil
	default:
		// Exit code 2: fixes applied but verification still failing after max cycles.
		fmt.Fprintf(cmd.ErrOrStderr(),
			"\nVerification still failing after %d fix cycle(s). Run with --verbose for details.\n",
			report.TotalCycles,
		)
		os.Exit(2)
		return nil // unreachable; satisfies the compiler
	}
}

// resolveReviewReport returns the content of the review report and the path it
// was loaded from. When reportFlag is non-empty it is read directly. Otherwise
// the most recently modified .md file in logDir is used. Both values are empty
// strings when no report is found or readable.
func resolveReviewReport(reportFlag, logDir string, logger *log.Logger) (content string, path string) {
	if reportFlag != "" {
		data, err := os.ReadFile(reportFlag)
		if err != nil {
			logger.Warn("could not read review report", "path", reportFlag, "error", err)
			return "", ""
		}
		return string(data), reportFlag
	}

	if logDir == "" {
		return "", ""
	}

	// Auto-detect: find the most recently modified .md file in logDir.
	latest, err := mostRecentMDFile(logDir)
	if err != nil || latest == "" {
		if err != nil {
			logger.Debug("auto-detect review report: error scanning log dir", "dir", logDir, "error", err)
		}
		return "", ""
	}

	data, err := os.ReadFile(latest)
	if err != nil {
		logger.Warn("auto-detect review report: could not read file", "path", latest, "error", err)
		return "", ""
	}

	logger.Debug("auto-detected review report", "path", latest)
	return string(data), latest
}

// mostRecentMDFile returns the path of the most recently modified .md file in
// dir (non-recursive). Returns an empty string when no .md files are found.
func mostRecentMDFile(dir string) (string, error) {
	var (
		latestPath    string
		latestModTime time.Time
	)

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries
		}
		// Only scan the top-level directory; skip subdirectories.
		if d.IsDir() && path != dir {
			return fs.SkipDir
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		if info.ModTime().After(latestModTime) {
			latestModTime = info.ModTime()
			latestPath = path
		}
		return nil
	})

	return latestPath, err
}

// firstConfiguredAgentName returns the name of the first agent in priority
// order (claude, codex, gemini) that has a non-empty Command or Model in the
// agent config map. Returns an empty string when no agents are configured.
func firstConfiguredAgentName(agentCfgs map[string]config.AgentConfig) string {
	for _, name := range []string{"claude", "codex", "gemini"} {
		if ac, ok := agentCfgs[name]; ok && (ac.Command != "" || ac.Model != "") {
			return name
		}
	}
	return ""
}
