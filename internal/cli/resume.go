package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/AbdelazizMoustafa10m/Raven/internal/logging"
	"github.com/AbdelazizMoustafa10m/Raven/internal/workflow"
)

// defaultStateDir is the path used when no explicit state directory is configured.
const defaultStateDir = ".raven/state"

// runIDPattern validates that a --run value is a safe ID (not a file path).
// Only alphanumeric characters, hyphens, and underscores are permitted.
var runIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ErrWorkflowNotFound is returned when a workflow definition cannot be resolved.
// This occurs in resume mode when T-049 built-in definitions are not yet available.
var ErrWorkflowNotFound = errors.New("workflow definition not found")

// resumeFlags holds parsed flag values for the resume command.
type resumeFlags struct {
	// RunID is the specific run to resume (--run <id>).
	RunID string
	// List shows all resumable runs in a table (--list).
	List bool
	// DryRun shows what would be resumed without executing (--dry-run).
	DryRun bool
	// Clean deletes a specific checkpoint by run ID (--clean <id>).
	Clean string
	// CleanAll deletes all checkpoints (--clean-all).
	CleanAll bool
	// Force skips the confirmation prompt for --clean-all in non-interactive mode.
	Force bool
}

// definitionResolver resolves a workflow name to its definition.
// Accepting it as a dependency enables test-time injection of mock definitions.
type definitionResolver func(workflowName string) (*workflow.WorkflowDefinition, error)

// newResumeCmd creates the "raven resume" command.
func newResumeCmd() *cobra.Command {
	var flags resumeFlags

	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Resume an interrupted workflow",
		Long: `List resumable workflow runs or resume a specific interrupted workflow
from its last persisted checkpoint.

When invoked with no flags, the most recently updated run is resumed
automatically.

The state directory defaults to .raven/state/ relative to the current working
directory. Checkpoints are written there by the workflow engine after every
step transition.`,
		Example: `  # List all resumable workflow runs
  raven resume --list

  # Resume the most recently updated run
  raven resume

  # Resume a specific run by ID
  raven resume --run wf-1234567890

  # Show what would be resumed without executing
  raven resume --run wf-1234567890 --dry-run

  # Delete a specific checkpoint
  raven resume --clean wf-1234567890

  # Delete all checkpoints (prompts for confirmation)
  raven resume --clean-all

  # Delete all checkpoints without prompting (non-interactive environments)
  raven resume --clean-all --force`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResume(cmd, flags, resolveDefinition)
		},
	}

	cmd.Flags().StringVar(&flags.RunID, "run", "", "Resume a specific workflow run by ID")
	cmd.Flags().BoolVar(&flags.List, "list", false, "List all resumable workflow runs")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Show what would be resumed without executing")
	cmd.Flags().StringVar(&flags.Clean, "clean", "", "Delete a specific checkpoint by run ID")
	cmd.Flags().BoolVar(&flags.CleanAll, "clean-all", false, "Delete all checkpoints")
	cmd.Flags().BoolVar(&flags.Force, "force", false, "Skip confirmation prompt for --clean-all")

	return cmd
}

func init() {
	rootCmd.AddCommand(newResumeCmd())
}

// runResume is the RunE implementation for the resume command.
// The resolver parameter is injected so tests can supply mock definitions.
func runResume(cmd *cobra.Command, flags resumeFlags, resolver definitionResolver) error {
	// Validate --run flag: only allow safe ID patterns to prevent path traversal.
	if flags.RunID != "" && !runIDPattern.MatchString(flags.RunID) {
		return fmt.Errorf("resume: invalid run ID %q: only alphanumeric characters, hyphens, and underscores are allowed", flags.RunID)
	}

	// Validate --clean flag: same safety constraint.
	if flags.Clean != "" && !runIDPattern.MatchString(flags.Clean) {
		return fmt.Errorf("resume: invalid run ID %q for --clean: only alphanumeric characters, hyphens, and underscores are allowed", flags.Clean)
	}

	store, err := workflow.NewStateStore(defaultStateDir)
	if err != nil {
		return fmt.Errorf("resume: opening state store at %q: %w", defaultStateDir, err)
	}

	// Branch on flags: list, clean-all, clean, or resume.
	if flags.List {
		return runListMode(cmd, store)
	}

	if flags.CleanAll {
		return runCleanAllMode(cmd, store, flags.Force, os.Stdin)
	}

	if flags.Clean != "" {
		return runCleanMode(store, flags.Clean)
	}

	// Resume mode.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return runResumeMode(ctx, cmd, store, flags.RunID, flags.DryRun, resolver)
}

// runListMode lists all resumable workflow runs in a formatted table.
// Output goes to cmd's stdout per the structured-output convention.
func runListMode(cmd *cobra.Command, store *workflow.StateStore) error {
	summaries, err := store.List()
	if err != nil {
		return fmt.Errorf("resume: listing runs: %w", err)
	}

	if len(summaries) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "No resumable workflow runs found.")
		return nil
	}

	formatRunTable(summaries, cmd.OutOrStdout())
	return nil
}

// runCleanMode deletes a single checkpoint by run ID.
func runCleanMode(store *workflow.StateStore, runID string) error {
	if err := store.Delete(runID); err != nil {
		return fmt.Errorf("resume: deleting checkpoint for run %q: %w", runID, err)
	}
	logger := logging.New("resume")
	logger.Info("checkpoint deleted", "run_id", runID)
	return nil
}

// runCleanAllMode deletes all checkpoints. When the process is running in a
// terminal it prompts for confirmation unless --force is set. In non-interactive
// mode (e.g. CI) --force is required; without it the command returns an error
// rather than silently destroying state.
// The stdin parameter is the file to read confirmation from; callers pass
// os.Stdin in production and a pipe in tests for deterministic behaviour.
func runCleanAllMode(cmd *cobra.Command, store *workflow.StateStore, force bool, stdin *os.File) error {
	if !force {
		if isTerminal(stdin) {
			// Interactive: ask the user.
			fmt.Fprint(cmd.ErrOrStderr(), "This will delete all workflow checkpoints. Continue? [y/N] ")
			scanner := bufio.NewScanner(stdin)
			if !scanner.Scan() || !strings.EqualFold(strings.TrimSpace(scanner.Text()), "y") {
				fmt.Fprintln(cmd.ErrOrStderr(), "Aborted.")
				return nil
			}
		} else {
			// Non-interactive without --force: refuse to proceed.
			return fmt.Errorf("resume: --clean-all in non-interactive mode requires --force to confirm deletion of all checkpoints")
		}
	}

	summaries, err := store.List()
	if err != nil {
		return fmt.Errorf("resume: listing runs for clean-all: %w", err)
	}

	if len(summaries) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "No workflow checkpoints found.")
		return nil
	}

	logger := logging.New("resume")
	var deleteErr error
	deleted := 0
	for _, s := range summaries {
		if err := store.Delete(s.ID); err != nil {
			logger.Error("failed to delete checkpoint", "run_id", s.ID, "error", err)
			deleteErr = err
			continue
		}
		deleted++
		logger.Info("checkpoint deleted", "run_id", s.ID)
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Deleted %d checkpoint(s).\n", deleted)

	return deleteErr
}

// runResumeMode loads a checkpoint and resumes the workflow from its current step.
// If runID is empty, the most recently updated run is used.
func runResumeMode(ctx context.Context, cmd *cobra.Command, store *workflow.StateStore, runID string, dryRun bool, resolver definitionResolver) error {
	var state *workflow.WorkflowState
	var err error

	if runID == "" {
		// No --run specified: use the most recent run.
		state, err = store.LatestRun()
		if err != nil {
			return fmt.Errorf("resume: loading latest run: %w", err)
		}
		if state == nil {
			return fmt.Errorf("resume: no resumable workflow runs found")
		}
	} else {
		state, err = store.Load(runID)
		if err != nil {
			return fmt.Errorf("resume: loading run %q: %w", runID, err)
		}
	}

	// In dry-run mode, describe what would happen without resolving the
	// definition or executing any steps. This allows --dry-run to work even
	// before T-049 built-in definitions are available.
	if dryRun || flagDryRun {
		fmt.Fprintf(cmd.ErrOrStderr(), "Dry-run: would resume workflow %q (run %q) at step %q\n",
			state.WorkflowName, state.ID, state.CurrentStep)
		fmt.Fprintf(cmd.ErrOrStderr(), "  Steps completed: %d\n", len(state.StepHistory))
		fmt.Fprintf(cmd.ErrOrStderr(), "  Last updated:    %s\n", state.UpdatedAt.Format(time.RFC3339))
		return nil
	}

	// Resolve the workflow definition by name.
	def, err := resolver(state.WorkflowName)
	if err != nil {
		if errors.Is(err, ErrWorkflowNotFound) {
			return fmt.Errorf(
				"resume: cannot resume run %q: workflow %q is not registered (built-in workflow definitions are not yet available -- see T-049)",
				state.ID, state.WorkflowName,
			)
		}
		return fmt.Errorf("resume: resolving workflow definition %q: %w", state.WorkflowName, err)
	}

	// Build the engine with checkpointing so each step is persisted.
	logger := logging.New("resume")
	engine := workflow.NewEngine(
		workflow.DefaultRegistry,
		workflow.WithLogger(logger),
		workflow.WithCheckpointing(store),
		workflow.WithDryRun(false),
	)

	logger.Info("resuming workflow",
		"workflow", state.WorkflowName,
		"run_id", state.ID,
		"step", state.CurrentStep,
		"steps_completed", len(state.StepHistory),
	)

	finalState, err := engine.Run(ctx, def, state)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintln(cmd.ErrOrStderr(), "\nWorkflow resume cancelled.")
			return err
		}
		return fmt.Errorf("resume: workflow %q (run %q): %w", state.WorkflowName, state.ID, err)
	}

	logger.Info("workflow completed",
		"workflow", finalState.WorkflowName,
		"run_id", finalState.ID,
		"steps_completed", len(finalState.StepHistory),
	)
	return nil
}

// resolveDefinition looks up a workflow definition by name.
// Until T-049 (built-in workflow definitions) is implemented, this always
// returns ErrWorkflowNotFound with a descriptive message.
func resolveDefinition(workflowName string) (*workflow.WorkflowDefinition, error) {
	// T-049 will register built-in definitions into a lookup table here.
	// For now, return an informative error so resume --run fails gracefully
	// while --list and --clean modes work without requiring definition resolution.
	return nil, fmt.Errorf("workflow %q: %w", workflowName, ErrWorkflowNotFound)
}

// formatRunTable writes a tabwriter-aligned table of RunSummary records to w.
// It uses text/tabwriter rather than lipgloss to avoid import-cycle issues.
func formatRunTable(summaries []workflow.RunSummary, w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	defer tw.Flush()

	// Header row.
	fmt.Fprintln(tw, "RUN ID\tWORKFLOW\tSTEP\tSTATUS\tLAST UPDATED\tSTEPS")
	fmt.Fprintln(tw, "------\t--------\t----\t------\t------------\t-----")

	for _, s := range summaries {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\n",
			s.ID,
			s.WorkflowName,
			s.CurrentStep,
			s.Status,
			s.UpdatedAt.Format("2006-01-02 15:04:05"),
			s.StepCount,
		)
	}
}

// isTerminal reports whether f is connected to a terminal (TTY).
// It uses the charmbracelet/x/term package which is already a transitive
// dependency of the project. On non-Unix platforms it conservatively returns
// false so non-interactive behaviour is assumed.
func isTerminal(f *os.File) bool {
	// Use os.ModeCharDevice to detect a character device (TTY) on all platforms
	// without importing an additional package.
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
