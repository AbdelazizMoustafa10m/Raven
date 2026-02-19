package cli

import (
	"fmt"
	"time"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
	"github.com/AbdelazizMoustafa10m/Raven/internal/git"
	"github.com/AbdelazizMoustafa10m/Raven/internal/logging"
	"github.com/AbdelazizMoustafa10m/Raven/internal/loop"
	"github.com/AbdelazizMoustafa10m/Raven/internal/pipeline"
	"github.com/AbdelazizMoustafa10m/Raven/internal/review"
	"github.com/AbdelazizMoustafa10m/Raven/internal/task"
	"github.com/AbdelazizMoustafa10m/Raven/internal/workflow"
)

// buildRuntimeHandlerDeps constructs real HandlerDeps for built-in workflow
// step handlers. It wires together the implementation loop runner, review
// orchestrator, fix engine, and PR creator using the resolved config, pipeline
// options, and available phases.
//
// gitClient may be nil when git is unavailable; the diff generator will still
// be created (with a fresh client) if possible. opts and phases may be
// zero-valued when called from the resume path where the original flags are
// not available.
func buildRuntimeHandlerDeps(
	cfg *config.Config,
	opts pipeline.PipelineOpts,
	phases []task.Phase,
	gitClient *git.GitClient,
) (*workflow.HandlerDeps, error) {
	logger := logging.New("deps")

	// --- 1. Discover tasks and create state manager ---
	specs, err := task.DiscoverTasks(cfg.Project.TasksDir)
	if err != nil {
		return nil, fmt.Errorf("discovering tasks in %q: %w", cfg.Project.TasksDir, err)
	}

	stateManager := task.NewStateManager(cfg.Project.TaskStateFile)

	// --- 2. Load phases if not provided ---
	if len(phases) == 0 && cfg.Project.PhasesConf != "" {
		loaded, loadErr := task.LoadPhases(cfg.Project.PhasesConf)
		if loadErr == nil {
			phases = loaded
		}
		// Ignore load errors; phases are optional for some handlers.
	}

	// --- 3. Create task selector ---
	selector := task.NewTaskSelector(specs, stateManager, phases)

	// --- 4. Build agent registry ---
	agentRegistry, err := buildAgentRegistry(cfg.Agents, implementFlags{})
	if err != nil {
		return nil, fmt.Errorf("building agent registry: %w", err)
	}

	// --- 5. Resolve implementation agent ---
	implAgentName := opts.ImplAgent
	if implAgentName == "" {
		implAgentName = firstConfiguredAgentName(cfg.Agents)
	}
	if implAgentName == "" {
		implAgentName = "claude"
	}

	implAgent, err := agentRegistry.Get(implAgentName)
	if err != nil {
		return nil, fmt.Errorf("resolving impl agent %q: %w", implAgentName, err)
	}

	// --- 6. Create prompt generator ---
	promptGen, err := loop.NewPromptGenerator(cfg.Project.PromptDir)
	if err != nil {
		return nil, fmt.Errorf("creating prompt generator: %w", err)
	}

	// --- 7. Create rate-limit coordinator ---
	rateLimiter := agent.NewRateLimitCoordinator(agent.DefaultBackoffConfig())

	// --- 8. Create Runner ---
	runnerLog := &runnerLogger{logger: logging.New("loop")}
	runner := loop.NewRunner(
		selector,
		promptGen,
		implAgent,
		stateManager,
		rateLimiter,
		cfg,
		phases,
		nil, // events channel (nil = no event fan-out)
		runnerLog,
	)

	// --- 9. Create ReviewOrchestrator ---
	reviewCfg := configToReviewConfig(cfg.Review)
	reviewLogger := logging.New("review")

	// Use the provided gitClient for diff generation; create one if nil.
	var diffGitClient git.Client
	if gitClient != nil {
		diffGitClient = gitClient
	} else {
		freshClient, clientErr := git.NewGitClient("")
		if clientErr != nil {
			logger.Warn("git client unavailable for diff generation", "error", clientErr)
		} else {
			diffGitClient = freshClient
		}
	}

	var orchestrator *review.ReviewOrchestrator
	if diffGitClient != nil {
		diffGen, diffErr := review.NewDiffGenerator(diffGitClient, reviewCfg, reviewLogger)
		if diffErr != nil {
			return nil, fmt.Errorf("creating diff generator: %w", diffErr)
		}

		promptBuilder := review.NewPromptBuilder(reviewCfg, reviewLogger)
		consolidator := review.NewConsolidator(reviewLogger)

		concurrency := opts.ReviewConcurrency
		if concurrency <= 0 {
			concurrency = 2
		}

		orchestrator = review.NewReviewOrchestrator(
			agentRegistry,
			diffGen,
			promptBuilder,
			consolidator,
			concurrency,
			reviewLogger,
			nil, // events channel
		)
	}

	// --- 10. Create FixEngine ---
	fixAgentName := opts.FixAgent
	if fixAgentName == "" {
		fixAgentName = firstConfiguredAgentName(cfg.Agents)
	}
	if fixAgentName == "" {
		fixAgentName = "claude"
	}

	fixAgent, fixAgentErr := agentRegistry.Get(fixAgentName)
	if fixAgentErr != nil {
		// Fall back to impl agent if fix agent lookup fails.
		fixAgent = implAgent
	}

	fixLogger := logging.New("fix")
	verifier := review.NewVerificationRunner(
		cfg.Project.VerificationCommands,
		"",            // use process working directory
		2*time.Minute, // reasonable default timeout
		fixLogger,
	)

	maxCycles := opts.MaxReviewCycles
	if maxCycles <= 0 {
		maxCycles = 3
	}

	fixEngine := review.NewFixEngine(fixAgent, verifier, maxCycles, fixLogger, nil)

	// Wire up a prompt builder for the fix engine with verification commands.
	fixPB := review.NewFixPromptBuilder(
		[]string{}, // conventions (empty for now)
		cfg.Project.VerificationCommands,
		fixLogger,
	)
	fixEngine.WithPromptBuilder(fixPB)

	// --- 11. Create PRCreator ---
	prCreator := review.NewPRCreator("", logging.New("pr"))

	// --- 12. Build RunConfig with reasonable defaults ---
	runCfg := loop.RunConfig{
		AgentName:     implAgentName,
		MaxIterations: 50,
		MaxLimitWaits: 5,
		SleepBetween:  5 * time.Second,
	}

	return &workflow.HandlerDeps{
		Runner:             runner,
		RunConfig:          runCfg,
		ReviewOrchestrator: orchestrator,
		FixEngine:          fixEngine,
		PRCreator:          prCreator,
	}, nil
}
