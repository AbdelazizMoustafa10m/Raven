package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
	"github.com/AbdelazizMoustafa10m/Raven/internal/logging"
	"github.com/AbdelazizMoustafa10m/Raven/internal/prd"
)

// prdFlags holds parsed flag values for the prd command.
type prdFlags struct {
	// File is the path to the PRD markdown file (required).
	File string
	// Concurrent enables parallel epic decomposition (default: true).
	Concurrent bool
	// Concurrency sets the max number of concurrent scatter workers (default: 3).
	Concurrency int
	// SinglePass uses sequential decomposition instead of parallel scatter.
	SinglePass bool
	// OutputDir is the directory to write output files to.
	OutputDir string
	// AgentName is the agent to use for decomposition.
	AgentName string
	// DryRun shows planned steps without executing.
	DryRun bool
	// Force allows overwriting existing output files.
	Force bool
	// StartID is the starting task number for global ID assignment (default: 1).
	StartID int
}

// prdPipeline orchestrates the full PRD decomposition pipeline.
type prdPipeline struct {
	cfg         *config.Config
	agent       agent.Agent
	logger      *log.Logger
	prdPath     string
	outputDir   string
	workDir     string
	concurrent  bool
	concurrency int
	singlePass  bool
	dryRun      bool
	force       bool
	startID     int
}

// errPartialSuccess signals that the pipeline completed but some epics failed.
// Callers should use errors.As to check for this type and return exit code 2.
type errPartialSuccess struct {
	totalEpics    int
	failedEpics   int
	succeededWith int
}

func (e *errPartialSuccess) Error() string {
	return fmt.Sprintf("partial success: %d/%d epics decomposed successfully (%d failed)",
		e.succeededWith, e.totalEpics, e.failedEpics)
}

// NewPRDCmd creates the "raven prd" command.
func NewPRDCmd() *cobra.Command {
	var flags prdFlags

	cmd := &cobra.Command{
		Use:   "prd",
		Short: "Decompose a PRD into tasks",
		Long: `Decompose a Product Requirements Document (PRD) into a structured set of
implementation tasks using a three-phase scatter-gather pipeline:

  Phase 1 (Shred):   Analyze the PRD and identify high-level epics.
  Phase 2 (Scatter): Decompose each epic into concrete development tasks (parallel).
  Phase 3 (Merge):   Assign global IDs, remap dependencies, deduplicate, validate DAG.
  Phase 4 (Emit):    Write task spec files, task-state.conf, phases.conf, PROGRESS.md, INDEX.md.

Use --single-pass to run scatter sequentially (concurrency=1) instead of in parallel.
Use --dry-run to preview planned steps without invoking the agent.`,
		Example: `  # Decompose a PRD using the default agent
  raven prd --file docs/prd/PRD.md

  # Use Codex agent, write to a custom output directory
  raven prd --file docs/prd/PRD.md --agent codex --output-dir ./my-tasks

  # Run with 5 concurrent scatter workers
  raven prd --file docs/prd/PRD.md --concurrency 5

  # Sequential (single-pass) decomposition
  raven prd --file docs/prd/PRD.md --single-pass

  # Dry-run: show what would be done without executing
  raven prd --file docs/prd/PRD.md --dry-run

  # Overwrite existing output files
  raven prd --file docs/prd/PRD.md --force

  # Start task numbering at T-050
  raven prd --file docs/prd/PRD.md --start-id 50`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPRD(cmd, flags)
		},
	}

	// Required flag.
	cmd.Flags().StringVar(&flags.File, "file", "", "Path to the PRD markdown file (required)")
	_ = cmd.MarkFlagRequired("file")

	// Optional flags with defaults.
	cmd.Flags().BoolVar(&flags.Concurrent, "concurrent", true, "Enable concurrent epic decomposition")
	cmd.Flags().IntVar(&flags.Concurrency, "concurrency", 3, "Max concurrent workers in scatter phase")
	cmd.Flags().BoolVar(&flags.SinglePass, "single-pass", false, "Sequential decomposition (concurrency=1)")
	cmd.Flags().StringVar(&flags.OutputDir, "output-dir", "", "Output directory for generated files (default: from config)")
	cmd.Flags().StringVar(&flags.AgentName, "agent", "", "Agent to use (default: from config, or \"claude\")")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Show planned steps without executing")
	cmd.Flags().BoolVar(&flags.Force, "force", false, "Overwrite existing output files")
	cmd.Flags().IntVar(&flags.StartID, "start-id", 1, "Starting task number for ID assignment (e.g. 50 -> T-050)")

	// Shell completion for --agent.
	_ = cmd.RegisterFlagCompletionFunc("agent", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"claude", "codex", "gemini"}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func init() {
	rootCmd.AddCommand(NewPRDCmd())
}

// runPRD is the RunE implementation for the prd command. It wires configuration,
// agent selection, and pipeline execution.
func runPRD(cmd *cobra.Command, flags prdFlags) error {
	logger := logging.New("prd")

	// Step 1: Validate --file exists and is readable.
	if err := validatePRDFile(flags.File); err != nil {
		return err
	}

	// Step 2: Load and resolve configuration.
	resolved, _, err := loadAndResolveConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	cfg := resolved.Config

	// Step 3: Resolve agent name (flag > config default > "claude").
	agentName := flags.AgentName
	if agentName == "" {
		// Try to find a default agent from config -- use "claude" as fallback.
		agentName = resolveDefaultAgentName(cfg.Agents)
	}

	// Step 4: Build agent registry and look up the resolved agent.
	// We reuse buildAgentRegistry but pass a zero-value implementFlags with
	// the Agent field set to the resolved name (Model override is not needed here).
	registry, err := buildAgentRegistry(cfg.Agents, implementFlags{Agent: agentName})
	if err != nil {
		return err
	}

	ag, err := registry.Get(agentName)
	if err != nil {
		available := registry.List()
		return fmt.Errorf("unknown agent %q: available agents are: %s",
			agentName, strings.Join(available, ", "))
	}

	// Step 5: Check agent prerequisites (skip in dry-run mode).
	dryRun := flags.DryRun || flagDryRun
	if !dryRun {
		if checkErr := ag.CheckPrerequisites(); checkErr != nil {
			return fmt.Errorf("agent prerequisite check failed for %q: %w", agentName, checkErr)
		}
		logger.Info("agent prerequisites satisfied", "agent", agentName)
	}

	// Step 6: Resolve output directory (flag > config.Project.TasksDir > "docs/tasks").
	outputDir := flags.OutputDir
	if outputDir == "" {
		outputDir = cfg.Project.TasksDir
	}
	if outputDir == "" {
		outputDir = "docs/tasks"
	}

	// Step 7: Create temp working directory for intermediate files.
	workDir, err := os.MkdirTemp("", "raven-prd-*")
	if err != nil {
		return fmt.Errorf("creating temp working directory: %w", err)
	}
	// Apply restrictive permissions to the temp dir.
	if chmodErr := os.Chmod(workDir, 0700); chmodErr != nil {
		logger.Debug("could not restrict temp dir permissions", "err", chmodErr)
	}

	// Step 8: Resolve single-pass vs concurrent mode.
	concurrent := flags.Concurrent
	singlePass := flags.SinglePass
	// --single-pass forces concurrency=1 regardless of --concurrent.
	concurrency := flags.Concurrency
	if singlePass {
		concurrency = 1
	}

	pipeline := &prdPipeline{
		cfg:         cfg,
		agent:       ag,
		logger:      logger,
		prdPath:     flags.File,
		outputDir:   outputDir,
		workDir:     workDir,
		concurrent:  concurrent,
		concurrency: concurrency,
		singlePass:  singlePass,
		dryRun:      dryRun,
		force:       flags.Force,
		startID:     flags.StartID,
	}

	// Step 9: Dry-run: print planned steps and return.
	if dryRun {
		if err := pipeline.printDryRun(); err != nil {
			return err
		}
		// Clean up temp dir on dry-run exit (nothing was written).
		os.RemoveAll(workDir) //nolint:errcheck
		return nil
	}

	// Step 10: Set up signal handling for graceful Ctrl+C.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger.Info("starting PRD decomposition",
		"file", flags.File,
		"agent", agentName,
		"outputDir", outputDir,
		"concurrent", concurrent,
		"concurrency", concurrency,
		"singlePass", singlePass,
		"force", flags.Force,
		"startID", flags.StartID,
	)

	// Step 11: Run the pipeline.
	runErr := pipeline.run(ctx)

	// Step 12: Handle results and cleanup.
	if runErr == nil {
		// Success: clean up temp dir.
		if removeErr := os.RemoveAll(workDir); removeErr != nil {
			logger.Debug("could not remove temp working directory", "dir", workDir, "err", removeErr)
		}
		return nil
	}

	// Partial success: some epics failed but we have output.
	if isErrPartialSuccess(runErr, nil) {
		// Clean up temp dir even on partial success.
		if removeErr := os.RemoveAll(workDir); removeErr != nil {
			logger.Debug("could not remove temp working directory", "dir", workDir, "err", removeErr)
		}
		// Return the partial success error so main.go can use exit code 2.
		return runErr
	}

	// Fatal error: preserve temp dir for debugging.
	logger.Error("pipeline failed; preserving temp working directory for debugging", "dir", workDir)
	return runErr
}

// resolveDefaultAgentName returns the first agent configured in the agents map,
// or "claude" if no agents are configured.
func resolveDefaultAgentName(agents map[string]config.AgentConfig) string {
	// Prefer "claude" if it's configured.
	if _, ok := agents["claude"]; ok {
		return "claude"
	}
	// Fall back to any configured agent.
	for name := range agents {
		return name
	}
	// Hard default.
	return "claude"
}

// validatePRDFile checks that the PRD file exists and is readable.
func validatePRDFile(path string) error {
	if path == "" {
		return fmt.Errorf("--file is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("PRD file not found: %s", path)
		}
		return fmt.Errorf("checking PRD file %q: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("PRD path %q is a directory, not a file", path)
	}
	// Check readability by attempting to open.
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening PRD file %q: %w", path, err)
	}
	f.Close()
	return nil
}

// isErrPartialSuccess checks if err is an *errPartialSuccess and assigns it to target.
func isErrPartialSuccess(err error, target **errPartialSuccess) bool {
	if ps, ok := err.(*errPartialSuccess); ok {
		if target != nil {
			*target = ps
		}
		return true
	}
	return false
}

// run executes the full PRD decomposition pipeline.
func (p *prdPipeline) run(ctx context.Context) error {
	// Single-pass mode: use sequential scatter (concurrency=1).
	// This satisfies the "one agent handles epics then tasks" spirit while
	// reusing all existing pipeline stages. The distinction is logged for clarity.
	if p.singlePass {
		return p.runSinglePass(ctx)
	}
	return p.runConcurrent(ctx)
}

// runConcurrent executes the three-phase scatter-gather pipeline with optional
// parallelism in the scatter phase.
func (p *prdPipeline) runConcurrent(ctx context.Context) error {
	// --- Phase 1: Shred ---
	p.logger.Info("Phase 1: shredding PRD into epics", "file", p.prdPath)
	shredStart := time.Now()

	shredder := prd.NewShredder(
		p.agent,
		p.workDir,
		prd.WithLogger(p.logger),
		prd.WithMaxRetries(3),
	)

	shredResult, err := shredder.Shred(ctx, prd.ShredOpts{
		PRDPath: p.prdPath,
	})
	if err != nil {
		return fmt.Errorf("phase 1 (shred): %w", err)
	}
	shredDuration := time.Since(shredStart)

	p.logger.Info("Phase 1 complete",
		"epics", len(shredResult.Breakdown.Epics),
		"duration", shredDuration.Round(time.Millisecond),
		"retries", shredResult.Retries,
	)

	// --- Phase 2: Scatter ---
	p.logger.Info("Phase 2: scattering epics into tasks",
		"epics", len(shredResult.Breakdown.Epics),
		"concurrency", p.concurrency,
	)
	scatterStart := time.Now()

	// Read PRD content for scatter prompts.
	prdContent, err := os.ReadFile(p.prdPath)
	if err != nil {
		return fmt.Errorf("reading PRD content for scatter phase: %w", err)
	}

	scatter := prd.NewScatterOrchestrator(
		p.agent,
		p.workDir,
		prd.WithConcurrency(p.concurrency),
		prd.WithScatterLogger(p.logger),
	)

	scatterResult, err := scatter.Scatter(ctx, prd.ScatterOpts{
		PRDContent: string(prdContent),
		Breakdown:  shredResult.Breakdown,
	})
	if err != nil {
		// Context cancellation is fatal.
		return fmt.Errorf("phase 2 (scatter): %w", err)
	}
	scatterDuration := time.Since(scatterStart)

	p.logger.Info("Phase 2 complete",
		"tasks_across_epics", countTotalTasks(scatterResult.Results),
		"succeeded_epics", len(scatterResult.Results),
		"failed_epics", len(scatterResult.Failures),
		"duration", scatterDuration.Round(time.Millisecond),
	)

	if len(scatterResult.Failures) > 0 {
		for _, f := range scatterResult.Failures {
			p.logger.Warn("epic decomposition failed",
				"epic_id", f.EpicID,
				"error", f.Err,
			)
		}
	}

	// Require at least one successful epic result to proceed.
	if len(scatterResult.Results) == 0 {
		return fmt.Errorf("phase 2 (scatter): all %d epics failed; no tasks to merge",
			len(shredResult.Breakdown.Epics))
	}

	// --- Phase 3: Merge ---
	p.logger.Info("Phase 3: merging, remapping, deduplicating, and validating tasks")
	mergeStart := time.Now()

	// 3a: Build results map and sort epics by dependency order.
	resultsMap := make(map[string]*prd.EpicTaskResult, len(scatterResult.Results))
	for _, r := range scatterResult.Results {
		resultsMap[r.EpicID] = r
	}

	epicOrder, err := prd.SortEpicsByDependency(shredResult.Breakdown)
	if err != nil {
		return fmt.Errorf("phase 3 (sort epics): %w", err)
	}

	// 3b: Assign global IDs.
	mergedTasks, idMapping := prd.AssignGlobalIDs(epicOrder, resultsMap)
	p.logger.Debug("assigned global IDs", "tasks", len(mergedTasks))

	// 3c: Build per-epic task map for cross-epic dependency resolution.
	epicTasksMap := buildEpicTasksMap(mergedTasks)

	// 3d: Remap dependencies.
	mergedTasks, remapReport := prd.RemapDependencies(mergedTasks, idMapping, epicTasksMap)
	p.logger.Debug("remapped dependencies",
		"remapped", remapReport.Remapped,
		"unresolved", len(remapReport.Unresolved),
		"ambiguous", len(remapReport.Ambiguous),
	)

	if len(remapReport.Unresolved) > 0 {
		for _, u := range remapReport.Unresolved {
			p.logger.Warn("unresolved dependency reference",
				"task", u.TaskID,
				"reference", u.Reference,
			)
		}
	}

	// 3e: Deduplicate tasks.
	mergedTasks, dedupReport := prd.DeduplicateTasks(mergedTasks)
	p.logger.Debug("deduplicated tasks",
		"original", dedupReport.OriginalCount,
		"removed", dedupReport.RemovedCount,
		"final", dedupReport.FinalCount,
	)

	if len(dedupReport.Merges) > 0 {
		for _, m := range dedupReport.Merges {
			p.logger.Info("deduplicated tasks",
				"kept", m.KeptTaskID,
				"removed", m.RemovedTaskIDs,
				"merged_criteria", m.MergedCriteria,
			)
		}
	}

	// 3f: Validate DAG.
	dagValidation := prd.ValidateDAG(mergedTasks)
	if !dagValidation.Valid {
		// Report all DAG errors.
		for _, dagErr := range dagValidation.Errors {
			p.logger.Error("DAG validation error",
				"type", dagErr.Type,
				"task", dagErr.TaskID,
				"details", dagErr.Details,
			)
		}
		return fmt.Errorf("phase 3 (DAG validation): dependency graph is invalid with %d error(s)",
			len(dagValidation.Errors))
	}
	mergeDuration := time.Since(mergeStart)

	p.logger.Info("Phase 3 complete",
		"tasks", len(mergedTasks),
		"max_depth", dagValidation.MaxDepth,
		"duration", mergeDuration.Round(time.Millisecond),
	)

	// --- Phase 4: Emit ---
	p.logger.Info("Phase 4: emitting output files", "output_dir", p.outputDir)
	emitStart := time.Now()

	emitter := prd.NewEmitter(
		p.outputDir,
		prd.WithEmitterLogger(p.logger),
		prd.WithForce(p.force),
	)

	emitResult, err := emitter.Emit(prd.EmitOpts{
		Tasks:      mergedTasks,
		Validation: dagValidation,
		Epics:      shredResult.Breakdown,
		StartID:    p.startID,
	})
	if err != nil {
		return fmt.Errorf("phase 4 (emit): %w", err)
	}
	emitDuration := time.Since(emitStart)

	// Print summary to stderr.
	p.printSummary(emitResult, shredDuration, scatterDuration, mergeDuration, emitDuration)

	// If some epics failed in scatter but we still produced output, return partial success.
	if len(scatterResult.Failures) > 0 {
		return &errPartialSuccess{
			totalEpics:    len(shredResult.Breakdown.Epics),
			failedEpics:   len(scatterResult.Failures),
			succeededWith: len(scatterResult.Results),
		}
	}

	return nil
}

// runSinglePass executes sequential decomposition (concurrency=1).
// This is equivalent to runConcurrent with concurrency=1; it processes
// each epic sequentially rather than in parallel. The design decision to
// reuse the same pipeline stages means single-pass mode is simple and correct.
func (p *prdPipeline) runSinglePass(ctx context.Context) error {
	p.logger.Info("single-pass mode: running scatter sequentially (concurrency=1)")
	// Single-pass mode reuses runConcurrent with concurrency already set to 1
	// (set in runPRD when singlePass=true). We temporarily set singlePass=false
	// to avoid infinite recursion.
	prev := p.singlePass
	p.singlePass = false
	defer func() { p.singlePass = prev }()
	return p.runConcurrent(ctx)
}

// printDryRun shows the planned pipeline steps without executing them.
func (p *prdPipeline) printDryRun() error {
	stderr := os.Stderr

	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "=== raven prd: Dry Run ===")
	fmt.Fprintln(stderr, "")
	fmt.Fprintf(stderr, "PRD File:     %s\n", p.prdPath)
	fmt.Fprintf(stderr, "Output Dir:   %s\n", p.outputDir)
	fmt.Fprintf(stderr, "Agent:        %s\n", p.agent.Name())
	fmt.Fprintf(stderr, "Concurrency:  %d\n", p.concurrency)
	if p.singlePass {
		fmt.Fprintln(stderr, "Mode:         single-pass (sequential)")
	} else if p.concurrent {
		fmt.Fprintf(stderr, "Mode:         concurrent (%d workers)\n", p.concurrency)
	} else {
		fmt.Fprintln(stderr, "Mode:         sequential")
	}
	fmt.Fprintf(stderr, "Start ID:     T-%03d\n", p.startID)
	fmt.Fprintf(stderr, "Force:        %v\n", p.force)
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "Pipeline steps:")
	fmt.Fprintln(stderr, "  Phase 1 (Shred):   Analyze PRD and identify epics")
	fmt.Fprintln(stderr, "                     -> epic-breakdown.json")
	fmt.Fprintln(stderr, "  Phase 2 (Scatter): Decompose each epic into tasks")
	fmt.Fprintln(stderr, "                     -> epic-<ID>.json per epic")
	fmt.Fprintln(stderr, "  Phase 3 (Merge):   Assign global IDs, remap deps, dedup, validate DAG")
	fmt.Fprintln(stderr, "  Phase 4 (Emit):    Write output files:")
	fmt.Fprintf(stderr, "                     -> %s/T-NNN-slug.md (one per task)\n", p.outputDir)
	fmt.Fprintf(stderr, "                     -> %s/task-state.conf\n", p.outputDir)
	fmt.Fprintf(stderr, "                     -> %s/phases.conf\n", p.outputDir)
	fmt.Fprintf(stderr, "                     -> %s/PROGRESS.md\n", p.outputDir)
	fmt.Fprintf(stderr, "                     -> %s/INDEX.md\n", p.outputDir)
	fmt.Fprintln(stderr, "")
	fmt.Fprintf(stderr, "Agent dry-run command: %s\n", p.agent.DryRunCommand(agent.RunOpts{
		Prompt:  "(PRD shred prompt)",
		WorkDir: p.workDir,
	}))
	fmt.Fprintln(stderr, "")

	return nil
}

// printSummary prints a summary of decomposition results to stderr.
func (p *prdPipeline) printSummary(
	result *prd.EmitResult,
	shredDuration, scatterDuration, mergeDuration, emitDuration time.Duration,
) {
	stderr := os.Stderr
	totalDuration := shredDuration + scatterDuration + mergeDuration + emitDuration

	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "=== PRD Decomposition Complete ===")
	fmt.Fprintln(stderr, "")
	fmt.Fprintf(stderr, "Output directory:  %s\n", result.OutputDir)
	fmt.Fprintf(stderr, "Total tasks:       %d\n", result.TotalTasks)
	fmt.Fprintf(stderr, "Total phases:      %d\n", result.TotalPhases)
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "Files generated:")
	fmt.Fprintf(stderr, "  Task specs:      %d files (T-XXX-slug.md)\n", len(result.TaskFiles))
	fmt.Fprintf(stderr, "  Task state:      %s\n", result.TaskStateFile)
	fmt.Fprintf(stderr, "  Phases:          %s\n", result.PhasesFile)
	fmt.Fprintf(stderr, "  Progress:        %s\n", result.ProgressFile)
	fmt.Fprintf(stderr, "  Index:           %s\n", result.IndexFile)
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "Timing:")
	fmt.Fprintf(stderr, "  Phase 1 (Shred):   %s\n", shredDuration.Round(time.Millisecond))
	fmt.Fprintf(stderr, "  Phase 2 (Scatter): %s\n", scatterDuration.Round(time.Millisecond))
	fmt.Fprintf(stderr, "  Phase 3 (Merge):   %s\n", mergeDuration.Round(time.Millisecond))
	fmt.Fprintf(stderr, "  Phase 4 (Emit):    %s\n", emitDuration.Round(time.Millisecond))
	fmt.Fprintf(stderr, "  Total:             %s\n", totalDuration.Round(time.Millisecond))
	fmt.Fprintln(stderr, "")
}

// buildEpicTasksMap groups merged tasks by their EpicID, returning a map
// suitable for use as the epicTasks parameter in prd.RemapDependencies.
func buildEpicTasksMap(tasks []prd.MergedTask) map[string][]prd.MergedTask {
	m := make(map[string][]prd.MergedTask)
	for _, t := range tasks {
		m[t.EpicID] = append(m[t.EpicID], t)
	}
	return m
}

// countTotalTasks returns the total number of tasks across all EpicTaskResult values.
func countTotalTasks(results []*prd.EpicTaskResult) int {
	total := 0
	for _, r := range results {
		total += len(r.Tasks)
	}
	return total
}
