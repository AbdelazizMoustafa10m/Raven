package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
	"github.com/AbdelazizMoustafa10m/Raven/internal/logging"
	"github.com/AbdelazizMoustafa10m/Raven/internal/review"
)

// prFlags holds parsed flag values for the pr command.
type prFlags struct {
	// BaseBranch is the base branch the PR targets.
	BaseBranch string

	// Draft creates the PR in draft state when true.
	Draft bool

	// Title is an optional override for the PR title.
	Title string

	// Labels is a list of label names to apply to the PR (repeatable).
	Labels []string

	// Assignees is a list of GitHub usernames to assign to the PR (repeatable).
	Assignees []string

	// ReviewReport is the path to a review report to include in the PR body.
	ReviewReport string

	// NoSummary skips AI summary generation when true.
	NoSummary bool
}

// newPRCmd creates the "raven pr" command.
func newPRCmd() *cobra.Command {
	var flags prFlags

	cmd := &cobra.Command{
		Use:   "pr",
		Short: "Create a GitHub pull request from the current branch",
		Long: `Create a GitHub pull request from the current branch using the gh CLI.

The PR body is automatically generated from the review pipeline results and an
optional AI-generated summary. Use --review-report to include a review report
in the PR body.

Exit codes:
  0 - PR created successfully
  1 - Error during execution

Use --dry-run to preview the PR title and body without creating the PR.`,
		Example: `  # Create PR with defaults (base branch: main)
  raven pr

  # Create a draft PR
  raven pr --draft

  # Create PR with custom title and labels
  raven pr --title "feat: implement T-042" --label "enhancement" --label "ai-generated"

  # Create PR targeting a different base branch
  raven pr --base develop

  # Include a review report in the PR body
  raven pr --review-report .raven/logs/review-report.md

  # Skip AI summary generation
  raven pr --no-summary

  # Dry-run: show PR title and body without creating
  raven pr --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPR(cmd, flags)
		},
	}

	cmd.Flags().StringVar(&flags.BaseBranch, "base", "main", "Base branch for the pull request")
	cmd.Flags().BoolVar(&flags.Draft, "draft", false, "Create the PR as a draft")
	cmd.Flags().StringVar(&flags.Title, "title", "", "PR title override (default: auto-generated)")
	cmd.Flags().StringArrayVar(&flags.Labels, "label", nil, "Label to apply to the PR (can be repeated)")
	cmd.Flags().StringArrayVar(&flags.Assignees, "assignee", nil, "GitHub username to assign to the PR (can be repeated)")
	cmd.Flags().StringVar(&flags.ReviewReport, "review-report", "", "Path to review report to include in PR body")
	cmd.Flags().BoolVar(&flags.NoSummary, "no-summary", false, "Skip AI summary generation")

	return cmd
}

func init() {
	rootCmd.AddCommand(newPRCmd())
}

// runPR is the RunE implementation for the pr command.
func runPR(cmd *cobra.Command, flags prFlags) error {
	logger := logging.New("pr")

	// Step 1: Load and resolve configuration.
	resolved, _, err := loadAndResolveConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	cfg := resolved.Config

	// Step 2: Set up signal context for graceful cancellation.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Step 3: Construct PRCreator.
	prCreator := review.NewPRCreator(".", logger)

	// Step 4: Check prerequisites (gh installed, authenticated, not on base branch).
	if err := prCreator.CheckPrerequisites(ctx, flags.BaseBranch); err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(cmd.ErrOrStderr(), "\nPR creation cancelled.")
			return err
		}
		return fmt.Errorf("pr prerequisites: %w", err)
	}

	// Step 5: Ensure branch is pushed to origin (unless dry-run).
	dryRun := flagDryRun
	if !dryRun {
		if err := prCreator.EnsureBranchPushed(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				fmt.Fprintln(cmd.ErrOrStderr(), "\nPR creation cancelled.")
				return err
			}
			return fmt.Errorf("ensuring branch pushed: %w", err)
		}
	}

	// Step 6: Resolve optional agent for summary generation.
	// Best-effort: if no agent is configured, use nil (body generator falls back
	// to structured summary).
	var summaryAgent agent.Agent
	if !flags.NoSummary {
		agentName := firstConfiguredAgentName(cfg.Agents)
		if agentName != "" {
			registry, regErr := buildAgentRegistry(cfg.Agents, implementFlags{})
			if regErr == nil {
				if ag, agErr := registry.Get(agentName); agErr == nil {
					summaryAgent = ag
				}
			}
		}
	}

	// Step 7: Build PRBodyGenerator.
	bodyGen := review.NewPRBodyGenerator(summaryAgent, ".github/PULL_REQUEST_TEMPLATE.md", logger)

	// Step 8: Determine current branch name.
	branchName, err := currentGitBranch(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(cmd.ErrOrStderr(), "\nPR creation cancelled.")
			return err
		}
		// Non-fatal: proceed with empty branch name.
		logger.Warn("could not determine current branch", "error", err)
		branchName = ""
	}

	// Step 9: Load review report content if provided.
	reviewReportContent := ""
	if flags.ReviewReport != "" {
		data, readErr := os.ReadFile(flags.ReviewReport)
		if readErr != nil {
			return fmt.Errorf("reading review report %q: %w", flags.ReviewReport, readErr)
		}
		reviewReportContent = string(data)
	}

	// Step 10: Generate AI summary (empty diff, no tasks -- user hasn't specified tasks).
	summary, err := bodyGen.GenerateSummary(ctx, "", []review.TaskSummary{})
	if err != nil {
		// Non-fatal: proceed with empty summary.
		logger.Warn("could not generate PR summary", "error", err)
		summary = ""
	}

	// Step 11: Build PRBodyData.
	data := review.PRBodyData{
		Summary:      summary,
		ReviewReport: reviewReportContent,
		BaseBranch:   flags.BaseBranch,
		BranchName:   branchName,
	}

	// Step 12: Generate PR title (use flag override if provided).
	title := flags.Title
	if title == "" {
		title = bodyGen.GenerateTitle(data)
	}

	// Step 13: Generate PR body.
	body, err := bodyGen.Generate(ctx, data)
	if err != nil {
		return fmt.Errorf("generating PR body: %w", err)
	}

	// Step 14: Handle dry-run mode.
	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Title: %s\n\n", title)
		fmt.Fprintf(cmd.OutOrStdout(), "Body:\n%s\n", body)
		return nil
	}

	// Step 15: Create the PR via gh CLI.
	logger.Info("creating pull request",
		"title", title,
		"base", flags.BaseBranch,
		"draft", flags.Draft,
		"labels", flags.Labels,
		"assignees", flags.Assignees,
	)

	result, err := prCreator.Create(ctx, review.PRCreateOpts{
		Title:      title,
		Body:       body,
		BaseBranch: flags.BaseBranch,
		Draft:      flags.Draft,
		Labels:     flags.Labels,
		Assignees:  flags.Assignees,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(cmd.ErrOrStderr(), "\nPR creation cancelled.")
			return err
		}
		return fmt.Errorf("creating pull request: %w", err)
	}

	// Step 16: Print PR URL to stdout on success.
	if result.URL != "" {
		fmt.Fprintln(cmd.OutOrStdout(), result.URL)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Pull request created successfully.")
	}

	logger.Info("pull request created",
		"url", result.URL,
		"number", result.Number,
		"draft", result.Draft,
	)

	return nil
}

// currentGitBranch returns the name of the currently checked-out git branch
// by running "git rev-parse --abbrev-ref HEAD".
func currentGitBranch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --abbrev-ref HEAD: %w", err)
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" {
		return "", fmt.Errorf("repository is in detached HEAD state")
	}
	return branch, nil
}
