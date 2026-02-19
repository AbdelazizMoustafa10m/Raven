package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
	"github.com/AbdelazizMoustafa10m/Raven/internal/git"
	"github.com/AbdelazizMoustafa10m/Raven/internal/logging"
	"github.com/AbdelazizMoustafa10m/Raven/internal/pipeline"
	"github.com/AbdelazizMoustafa10m/Raven/internal/task"
	"github.com/AbdelazizMoustafa10m/Raven/internal/workflow"
)

// safeBranchRE matches branch names that contain only safe characters:
// alphanumeric, hyphens, underscores, dots, and forward slashes.
var safeBranchRE = regexp.MustCompile(`^[a-zA-Z0-9_./-]+$`)

// pipelineFlags holds all parsed flag values for the pipeline command.
type pipelineFlags struct {
	// Phase is the phase specifier: a numeric ID or "all".
	// Mutually exclusive with FromPhase.
	Phase string

	// FromPhase starts pipeline execution from this phase (inclusive).
	// Mutually exclusive with Phase.
	FromPhase string

	// ImplAgent is the agent name to use for implementation (overrides config).
	ImplAgent string

	// ReviewAgent is the agent name to use for review (overrides config).
	ReviewAgent string

	// FixAgent is the agent name to use for fixes (overrides config).
	FixAgent string

	// ReviewConcurrency is the maximum number of concurrent review agents.
	ReviewConcurrency int

	// MaxReviewCycles caps the number of review-fix iterations per phase.
	MaxReviewCycles int

	// SkipImplement skips the implementation stage.
	SkipImplement bool

	// SkipReview skips the review stage.
	SkipReview bool

	// SkipFix skips the fix stage.
	SkipFix bool

	// SkipPR skips the PR creation stage.
	SkipPR bool

	// Interactive launches the configuration wizard before running.
	Interactive bool

	// DryRun describes planned execution without running.
	DryRun bool

	// Base is the base branch for pipeline phase branches.
	Base string

	// SyncBase fetches from origin before pipeline execution.
	SyncBase bool
}

// newPipelineCmd creates the "raven pipeline" command.
func newPipelineCmd() *cobra.Command {
	var flags pipelineFlags

	cmd := &cobra.Command{
		Use:   "pipeline",
		Short: "Run the full implement-review-fix-PR pipeline across phases",
		Long: `Run the full implement-review-fix-PR pipeline across one or more phases.

The pipeline orchestrates four stages per phase in sequence:
  1. Implement: run the AI agent to implement all tasks in the phase
  2. Review: run multi-agent code review on the phase diff
  3. Fix: apply review findings using an AI agent (iterative)
  4. PR: create a pull request for the phase branch

Each phase runs on its own git branch (raven/phase-N by default). Branches are
created from the base branch for the first phase, and from the previous phase
branch for subsequent phases.

Use --interactive (or run with no flags in a TTY) to launch the configuration
wizard. Use --dry-run to preview the execution plan without making any changes.

Exit codes:
  0 - All phases completed successfully
  1 - Fatal error
  2 - Partial success (some phases failed)
  3 - Cancelled by user (wizard cancelled or Ctrl+C)`,
		Example: `  # Run pipeline for phase 1
  raven pipeline --phase 1

  # Run pipeline for all phases
  raven pipeline --phase all

  # Start from phase 3 onwards
  raven pipeline --from-phase 3

  # Run with specific agents
  raven pipeline --phase all --impl-agent claude --review-agent codex --fix-agent claude

  # Skip review and PR stages
  raven pipeline --phase 1 --skip-review --skip-pr

  # Preview execution plan without running
  raven pipeline --phase all --dry-run

  # Launch interactive wizard
  raven pipeline --interactive`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPipeline(cmd, flags)
		},
	}

	// Phase selection flags (mutually exclusive; enforced in RunE).
	cmd.Flags().StringVar(&flags.Phase, "phase", "", `Phase to run (numeric ID or "all")`)
	cmd.Flags().StringVar(&flags.FromPhase, "from-phase", "", "Start pipeline from this phase (inclusive)")

	// Agent override flags.
	cmd.Flags().StringVar(&flags.ImplAgent, "impl-agent", "", "Agent for implementation step (default: from config)")
	cmd.Flags().StringVar(&flags.ReviewAgent, "review-agent", "", "Agent for review step (default: from config)")
	cmd.Flags().StringVar(&flags.FixAgent, "fix-agent", "", "Agent for fix step (default: from config)")

	// Numeric tuning flags.
	cmd.Flags().IntVar(&flags.ReviewConcurrency, "review-concurrency", 2, "Maximum concurrent review agents (>= 1)")
	cmd.Flags().IntVar(&flags.MaxReviewCycles, "max-review-cycles", 3, "Maximum review/fix iterations per phase (>= 1)")

	// Skip flags.
	cmd.Flags().BoolVar(&flags.SkipImplement, "skip-implement", false, "Skip the implementation stage")
	cmd.Flags().BoolVar(&flags.SkipReview, "skip-review", false, "Skip the review stage")
	cmd.Flags().BoolVar(&flags.SkipFix, "skip-fix", false, "Skip the fix stage")
	cmd.Flags().BoolVar(&flags.SkipPR, "skip-pr", false, "Skip the PR creation stage")

	// Mode flags.
	cmd.Flags().BoolVar(&flags.Interactive, "interactive", false, "Launch configuration wizard before running")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Show planned execution without running")

	// Branch flags.
	cmd.Flags().StringVar(&flags.Base, "base", "main", "Base branch for phase branches")
	cmd.Flags().BoolVar(&flags.SyncBase, "sync-base", false, "Fetch and fast-forward base branch from origin before running")

	// Shell completions for phase and agent flags.
	_ = cmd.RegisterFlagCompletionFunc("phase", completePipelinePhase)
	_ = cmd.RegisterFlagCompletionFunc("from-phase", completePipelinePhase)
	_ = cmd.RegisterFlagCompletionFunc("impl-agent", completePipelineAgent)
	_ = cmd.RegisterFlagCompletionFunc("review-agent", completePipelineAgent)
	_ = cmd.RegisterFlagCompletionFunc("fix-agent", completePipelineAgent)

	return cmd
}

func init() {
	rootCmd.AddCommand(newPipelineCmd())
}

// completePipelinePhase provides shell completion for --phase and --from-phase.
func completePipelinePhase(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	// Load phases for dynamic completions; fall back to static options on error.
	resolved, _, err := loadAndResolveConfig()
	if err != nil {
		return []string{"all", "1", "2", "3"}, cobra.ShellCompDirectiveNoFileComp
	}

	phases, err := task.LoadPhases(resolved.Config.Project.PhasesConf)
	if err != nil {
		return []string{"all", "1", "2", "3"}, cobra.ShellCompDirectiveNoFileComp
	}

	out := make([]string, 0, len(phases)+1)
	out = append(out, "all")
	for _, ph := range phases {
		out = append(out, strconv.Itoa(ph.ID))
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// completePipelineAgent provides shell completion for agent flags.
func completePipelineAgent(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{"claude", "codex", "gemini"}, cobra.ShellCompDirectiveNoFileComp
}

// runPipeline is the RunE implementation for the pipeline command.
func runPipeline(cmd *cobra.Command, flags pipelineFlags) error {
	logger := logging.New("pipeline")

	// Step 1: Validate mutual exclusivity of --phase and --from-phase.
	if flags.Phase != "" && flags.FromPhase != "" {
		return fmt.Errorf("--phase and --from-phase are mutually exclusive; specify only one")
	}

	// Step 2: Load and resolve configuration.
	resolved, _, err := loadAndResolveConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	cfg := resolved.Config

	// Step 3: Load phases for validation and wizard.
	var phases []task.Phase
	if cfg.Project.PhasesConf != "" {
		phases, err = task.LoadPhases(cfg.Project.PhasesConf)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("loading phases from %q: %w", cfg.Project.PhasesConf, err)
			}
			logger.Info("phases.conf not found", "path", cfg.Project.PhasesConf)
		}
	}

	// Step 4: Determine whether to launch the wizard.
	noPhaseFlags := flags.Phase == "" && flags.FromPhase == ""
	launchWizard := flags.Interactive || (noPhaseFlags && isStdinTTY())

	if noPhaseFlags && !flags.Interactive && !isStdinTTY() {
		// Non-interactive with no phase flags: print usage and error.
		_ = cmd.Usage()
		return fmt.Errorf("either --phase, --from-phase, or --interactive must be specified (or run in a TTY for the wizard)")
	}

	// Step 5: Run wizard if requested.
	if launchWizard {
		if len(phases) == 0 {
			return fmt.Errorf("interactive mode requires at least one phase in phases.conf")
		}

		// Build available agent list from config.
		agentNames := availableAgentNames()
		defaultAgent := firstConfiguredAgentName(cfg.Agents)
		if defaultAgent == "" {
			defaultAgent = "claude"
		}

		wizardCfg := pipeline.WizardConfig{
			Phases:       phases,
			Agents:       agentNames,
			DefaultAgent: defaultAgent,
			Config:       cfg,
		}

		wizardOpts, wizardErr := pipeline.RunWizard(wizardCfg)
		if wizardErr != nil {
			if errors.Is(wizardErr, pipeline.ErrWizardCancelled) {
				fmt.Fprintln(os.Stderr, "Pipeline cancelled.")
				os.Exit(3)
			}
			return fmt.Errorf("wizard: %w", wizardErr)
		}

		// Apply wizard output to flags, overriding only values that the wizard set.
		applyWizardOpts(wizardOpts, &flags)
	}

	// Step 6: Apply global --dry-run flag.
	if flagDryRun {
		flags.DryRun = true
	}

	// Step 7: Validate flag values.
	if err := validatePipelineFlags(flags, phases); err != nil {
		return err
	}

	// Step 8: Validate base branch name safety.
	if flags.Base != "" && !safeBranchRE.MatchString(flags.Base) {
		return fmt.Errorf("invalid --base branch name %q: must contain only alphanumeric characters, hyphens, underscores, dots, or slashes", flags.Base)
	}

	// Step 9: Build PipelineOpts from flags.
	opts := buildPipelineOpts(flags)

	// Step 10: Handle dry-run mode.
	if opts.DryRun {
		return runPipelineDryRun(cmd, opts, phases, cfg)
	}

	// Step 11: Construct subsystems.
	stateDir := ".raven/state"
	if cfg.Project.LogDir != "" {
		stateDir = cfg.Project.LogDir + "/state"
	}
	store, err := workflow.NewStateStore(stateDir)
	if err != nil {
		return fmt.Errorf("creating state store: %w", err)
	}

	registry := workflow.NewRegistry()
	workflow.RegisterBuiltinHandlers(registry, nil)

	engine := workflow.NewEngine(
		registry,
		workflow.WithLogger(logging.New("workflow")),
		workflow.WithCheckpointing(store),
	)

	// Create git client for branch management.
	gitClient, gitErr := git.NewGitClient("")
	if gitErr != nil {
		// Non-fatal: pipeline can still run without branch management.
		logger.Warn("git client unavailable; branch management disabled", "error", gitErr)
		gitClient = nil
	}

	// Create pipeline orchestrator.
	orchestrator := pipeline.NewPipelineOrchestrator(
		engine,
		store,
		gitClient,
		cfg,
		pipeline.WithPipelineLogger(logging.New("pipeline")),
	)

	// Step 12: Set up signal handling.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Step 13: Sync base branch if requested.
	if flags.SyncBase && gitClient != nil {
		logger.Info("fetching base branch from origin", "base", flags.Base)
		if fetchErr := gitClient.Fetch(ctx, ""); fetchErr != nil {
			// Non-fatal: log a warning and continue.
			logger.Warn("fetch from origin failed; proceeding without sync", "error", fetchErr)
		}
	}

	// Step 14: Log run start.
	logger.Info("starting pipeline",
		"phase", flags.Phase,
		"from_phase", flags.FromPhase,
		"impl_agent", opts.ImplAgent,
		"review_agent", opts.ReviewAgent,
		"fix_agent", opts.FixAgent,
		"review_concurrency", opts.ReviewConcurrency,
		"max_review_cycles", opts.MaxReviewCycles,
		"skip_implement", opts.SkipImplement,
		"skip_review", opts.SkipReview,
		"skip_fix", opts.SkipFix,
		"skip_pr", opts.SkipPR,
	)

	// Step 15: Run the pipeline orchestrator.
	result, runErr := orchestrator.Run(ctx, opts)

	// Step 16: Handle context cancellation (Ctrl+C).
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			fmt.Fprintln(os.Stderr, "\nPipeline cancelled.")
			os.Exit(3)
		}
		// Print summary before returning error.
		if result != nil {
			printPipelineSummary(cmd, result)
		}
		return fmt.Errorf("pipeline run failed: %w", runErr)
	}

	// Step 17: Print result summary.
	printPipelineSummary(cmd, result)

	// Step 18: Map result status to exit code.
	switch result.Status {
	case pipeline.PipelineStatusCompleted:
		logger.Info("pipeline complete", "status", result.Status, "duration", result.TotalDuration)
		return nil
	case pipeline.PipelineStatusPartial:
		logger.Info("pipeline partial", "status", result.Status, "duration", result.TotalDuration)
		os.Exit(2)
		return nil // unreachable; satisfies the compiler
	default:
		return fmt.Errorf("pipeline failed with status %q", result.Status)
	}
}

// runPipelineDryRun builds a PipelineDryRunInfo from the given options and
// phases, formats it using DryRunFormatter, and prints it to stdout.
func runPipelineDryRun(cmd *cobra.Command, opts pipeline.PipelineOpts, phases []task.Phase, cfg *config.Config) error {
	// If phases were not loaded yet (e.g. in all mode), try again.
	phasesConf := cfg.Project.PhasesConf
	if len(phases) == 0 && phasesConf != "" {
		var err error
		phases, err = task.LoadPhases(phasesConf)
		if err != nil {
			return fmt.Errorf("loading phases for dry-run: %w", err)
		}
	}

	// Filter phases according to opts.
	filtered, err := filterPhasesForDryRun(phases, opts)
	if err != nil {
		return fmt.Errorf("resolving phases for dry-run: %w", err)
	}

	// Build the dry-run info structure.
	skipped := buildSkippedList(opts)
	implAgent := opts.ImplAgent
	if implAgent == "" {
		implAgent = "claude"
	}
	reviewAgent := opts.ReviewAgent
	if reviewAgent == "" {
		reviewAgent = "claude"
	}
	fixAgent := opts.FixAgent
	if fixAgent == "" {
		fixAgent = "claude"
	}

	// Resolve branch template from config if available.
	branchTemplate := ""
	if cfg.Project.BranchTemplate != "" {
		branchTemplate = cfg.Project.BranchTemplate
	}
	baseBranchName := opts.BaseBranch
	if baseBranchName == "" {
		baseBranchName = "main"
	}
	branchMgr := pipeline.NewBranchManager(nil, branchTemplate, baseBranchName)
	projectName := cfg.Project.Name

	// Build the workflow definition to extract step-level detail.
	baseDef := workflow.GetDefinition(workflow.WorkflowImplementReview)
	registry := workflow.NewRegistry()
	workflow.RegisterBuiltinHandlers(registry, nil)

	phaseDryRunDetails := make([]workflow.PhaseDryRunDetail, len(filtered))
	for i, ph := range filtered {
		branchName := branchMgr.ResolveBranchName(ph.ID, ph.Name, projectName)
		baseBranch := baseBranchName
		if i > 0 {
			baseBranch = branchMgr.ResolveBranchName(filtered[i-1].ID, filtered[i-1].Name, projectName)
		}

		// Build step-level dry-run details from the workflow definition.
		var stepDetails []workflow.StepDryRunDetail
		if baseDef != nil {
			modifiedDef := pipeline.ApplySkipFlags(baseDef, opts)
			for _, sd := range modifiedDef.Steps {
				desc := sd.Name
				handler, handlerErr := registry.Get(sd.Name)
				if handlerErr == nil {
					desc = handler.DryRun(nil)
				}
				stepDetails = append(stepDetails, workflow.StepDryRunDetail{
					StepName:    sd.Name,
					Description: desc,
					Transitions: sd.Transitions,
				})
			}
		}

		phaseDryRunDetails[i] = workflow.PhaseDryRunDetail{
			PhaseID:     ph.ID,
			PhaseName:   ph.Name,
			BranchName:  branchName,
			BaseBranch:  baseBranch,
			Skipped:     skipped,
			ImplAgent:   implAgent,
			ReviewAgent: reviewAgent,
			FixAgent:    fixAgent,
			Steps:       stepDetails,
		}
	}

	info := workflow.PipelineDryRunInfo{
		TotalPhases: len(filtered),
		Phases:      phaseDryRunDetails,
	}

	// Use colour when stdout appears to be a TTY.
	styled := isStdoutTTY()
	formatter := workflow.NewDryRunFormatter(cmd.OutOrStdout(), styled)
	output := formatter.FormatPipelineDryRun(info)
	formatter.Write(output)
	return nil
}

// filterPhasesForDryRun applies PhaseID / FromPhase filtering to the supplied
// phases slice without requiring a full orchestrator, matching the logic used
// in PipelineOrchestrator.resolvePhases.
func filterPhasesForDryRun(phases []task.Phase, opts pipeline.PipelineOpts) ([]task.Phase, error) {
	if opts.PhaseID == "" || strings.EqualFold(opts.PhaseID, "all") {
		if opts.FromPhase != "" {
			from, err := strconv.Atoi(opts.FromPhase)
			if err != nil {
				return nil, fmt.Errorf("invalid --from-phase %q: must be a positive integer", opts.FromPhase)
			}
			var out []task.Phase
			for _, ph := range phases {
				if ph.ID >= from {
					out = append(out, ph)
				}
			}
			if len(out) == 0 {
				return nil, fmt.Errorf("no phases found with ID >= %d", from)
			}
			return out, nil
		}
		return phases, nil
	}

	id, err := strconv.Atoi(opts.PhaseID)
	if err != nil {
		return nil, fmt.Errorf("invalid --phase %q: must be a positive integer or \"all\"", opts.PhaseID)
	}
	ph := task.PhaseByID(phases, id)
	if ph == nil {
		return nil, fmt.Errorf("phase %d not found", id)
	}
	return []task.Phase{*ph}, nil
}

// buildSkippedList returns the list of stage names that would be skipped given opts.
func buildSkippedList(opts pipeline.PipelineOpts) []string {
	var skipped []string
	if opts.SkipImplement {
		skipped = append(skipped, "implement")
	}
	if opts.SkipReview {
		skipped = append(skipped, "review")
	}
	if opts.SkipFix {
		skipped = append(skipped, "fix")
	}
	if opts.SkipPR {
		skipped = append(skipped, "pr")
	}
	return skipped
}

// printPipelineSummary writes a human-readable result summary to cmd's stdout.
func printPipelineSummary(cmd *cobra.Command, result *pipeline.PipelineResult) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "\nPipeline %s (duration: %s)\n", result.Status, result.TotalDuration.Round(1e6))
	fmt.Fprintln(out, strings.Repeat("-", 60))

	for _, ph := range result.Phases {
		statusPad := fmt.Sprintf("%-12s", ph.Status)
		line := fmt.Sprintf("  Phase %-4s  %s  branch: %-30s", ph.PhaseID, statusPad, ph.BranchName)
		if ph.PRURL != "" {
			line += fmt.Sprintf("  PR: %s", ph.PRURL)
		}
		if ph.Error != "" {
			line += fmt.Sprintf("  Error: %s", ph.Error)
		}
		fmt.Fprintln(out, line)
	}
	fmt.Fprintln(out)
}

// buildPipelineOpts converts pipelineFlags into a pipeline.PipelineOpts.
func buildPipelineOpts(flags pipelineFlags) pipeline.PipelineOpts {
	opts := pipeline.PipelineOpts{
		PhaseID:           flags.Phase,
		FromPhase:         flags.FromPhase,
		ImplAgent:         flags.ImplAgent,
		ReviewAgent:       flags.ReviewAgent,
		FixAgent:          flags.FixAgent,
		ReviewConcurrency: flags.ReviewConcurrency,
		MaxReviewCycles:   flags.MaxReviewCycles,
		SkipImplement:     flags.SkipImplement,
		SkipReview:        flags.SkipReview,
		SkipFix:           flags.SkipFix,
		SkipPR:            flags.SkipPR,
		DryRun:            flags.DryRun,
		Interactive:       flags.Interactive,
		BaseBranch:        flags.Base,
	}

	// Normalise "all" to empty string (the orchestrator treats empty as "all").
	if strings.EqualFold(opts.PhaseID, "all") {
		opts.PhaseID = ""
	}

	return opts
}

// applyWizardOpts copies values from wizard output into pipelineFlags.
// Only non-empty values are applied so that explicit CLI flags take precedence
// when the wizard is run alongside them.
func applyWizardOpts(wizardOpts *pipeline.PipelineOpts, flags *pipelineFlags) {
	if wizardOpts.PhaseID != "" {
		flags.Phase = wizardOpts.PhaseID
	}
	if wizardOpts.FromPhase != "" {
		flags.FromPhase = wizardOpts.FromPhase
	}
	if wizardOpts.ImplAgent != "" {
		flags.ImplAgent = wizardOpts.ImplAgent
	}
	if wizardOpts.ReviewAgent != "" {
		flags.ReviewAgent = wizardOpts.ReviewAgent
	}
	if wizardOpts.FixAgent != "" {
		flags.FixAgent = wizardOpts.FixAgent
	}
	if wizardOpts.ReviewConcurrency > 0 {
		flags.ReviewConcurrency = wizardOpts.ReviewConcurrency
	}
	if wizardOpts.MaxReviewCycles > 0 {
		flags.MaxReviewCycles = wizardOpts.MaxReviewCycles
	}
	flags.SkipImplement = wizardOpts.SkipImplement
	flags.SkipReview = wizardOpts.SkipReview
	flags.SkipFix = wizardOpts.SkipFix
	flags.SkipPR = wizardOpts.SkipPR
	if wizardOpts.DryRun {
		flags.DryRun = true
	}
}

// validatePipelineFlags performs semantic validation of pipeline flags
// after they have been populated (either from CLI or from the wizard).
func validatePipelineFlags(flags pipelineFlags, phases []task.Phase) error {
	// --review-concurrency must be >= 1.
	if flags.ReviewConcurrency < 1 {
		return fmt.Errorf("--review-concurrency must be >= 1, got %d", flags.ReviewConcurrency)
	}

	// --max-review-cycles must be >= 1.
	if flags.MaxReviewCycles < 1 {
		return fmt.Errorf("--max-review-cycles must be >= 1, got %d", flags.MaxReviewCycles)
	}

	// Warn when all stages are skipped.
	if flags.SkipImplement && flags.SkipReview && flags.SkipFix && flags.SkipPR {
		return fmt.Errorf("all pipeline stages are skipped: at least one stage must be active")
	}

	// Validate specific phase ID when provided (not "all" and not empty).
	if flags.Phase != "" && !strings.EqualFold(flags.Phase, "all") {
		if _, err := strconv.Atoi(flags.Phase); err != nil {
			return fmt.Errorf("invalid --phase value %q: must be a positive integer or \"all\"", flags.Phase)
		}
		if len(phases) > 0 {
			id, _ := strconv.Atoi(flags.Phase)
			if task.PhaseByID(phases, id) == nil {
				available := phaseIDList(phases)
				return fmt.Errorf("phase %q not found; available phases: %s", flags.Phase, available)
			}
		}
	}

	// Validate --from-phase when provided.
	if flags.FromPhase != "" {
		n, err := strconv.Atoi(flags.FromPhase)
		if err != nil || n < 1 {
			return fmt.Errorf("invalid --from-phase value %q: must be a positive integer", flags.FromPhase)
		}
	}

	// Validate agent names when explicitly provided.
	validAgents := map[string]bool{"claude": true, "codex": true, "gemini": true}
	for flag, val := range map[string]string{
		"--impl-agent":   flags.ImplAgent,
		"--review-agent": flags.ReviewAgent,
		"--fix-agent":    flags.FixAgent,
	} {
		if val != "" && !validAgents[val] {
			return fmt.Errorf("%s: unknown agent %q; available agents are: claude, codex, gemini", flag, val)
		}
	}

	return nil
}

// availableAgentNames returns the list of known agent names for wizard and
// completion use. All three built-in agents are always returned so the wizard
// shows all options regardless of what is configured.
func availableAgentNames() []string {
	return []string{"claude", "codex", "gemini"}
}

// phaseIDList returns a comma-separated string of phase IDs from phases.
func phaseIDList(phases []task.Phase) string {
	ids := make([]string, len(phases))
	for i, ph := range phases {
		ids[i] = strconv.Itoa(ph.ID)
	}
	return strings.Join(ids, ", ")
}

// isStdinTTY reports whether stdin is attached to a terminal.
// It uses os.ModeCharDevice on the file info to avoid adding new dependencies.
func isStdinTTY() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// isStdoutTTY reports whether stdout is attached to a terminal.
func isStdoutTTY() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
