# T-029: Implementation CLI Command -- raven implement

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 8-12hrs |
| Dependencies | T-006, T-009, T-015, T-016, T-017, T-018, T-019, T-021, T-022, T-023, T-025, T-026, T-027, T-028 |
| Blocked By | T-006, T-009, T-027, T-028 |
| Blocks | T-049 |

## Goal
Implement the `raven implement` CLI command that ties together all Phase 2 components into a user-facing command. This command wires up configuration, task discovery, agent selection, prompt generation, and the implementation loop into a single cohesive entry point. It supports `--agent claude --phase 1` for phase-based execution and `--task T-007` for single-task execution, with `--dry-run` mode for previewing prompts.

## Background
Per PRD Section 5.11, the implement command has two primary modes:
- `raven implement --agent <name> --phase <id>` -- Run the implementation loop for all tasks in a phase
- `raven implement --agent <name> --task <id>` -- Run the implementation loop for a single specific task

Additional flags: `--max-iterations`, `--max-limit-waits`, `--sleep`, `--dry-run`, `--model` (override agent config model). The command is the primary deliverable of Phase 2: "raven implement --agent claude --phase 1 runs the full implementation loop for a phase."

This command is the CLI layer that composes all internal components: config loading (T-009), task discovery (T-016), task state (T-017), phase config (T-018), task selection (T-019), agent registry (T-021), agent adapters (T-022, T-023), rate-limit coordination (T-025), prompt generation (T-026), loop runner (T-027), and recovery (T-028).

## Technical Specifications
### Implementation Approach
Create `internal/cli/implement.go` containing the Cobra command for `raven implement`. The command's RunE function performs setup (load config, discover tasks, create state manager, load phases, build selector, create agent, create prompt generator, create loop runner) then delegates to the Runner. This is a composition root that wires up all dependencies. Flags map to RunConfig fields.

### Key Components
- **implementCmd**: Cobra command with flags for agent, phase, task, limits, dry-run
- **RunE function**: Composition root that wires all dependencies
- **Flag validation**: Mutual exclusivity of --phase and --task, agent name validation
- **Agent prerequisite checking**: Verify agent CLI is installed before starting

### API/Interface Contracts
```go
// internal/cli/implement.go
package cli

import (
    "github.com/spf13/cobra"
)

// newImplementCmd creates the "raven implement" command.
func newImplementCmd() *cobra.Command

// implementFlags holds parsed flag values.
type implementFlags struct {
    Agent         string        // --agent <name> (required)
    Phase         int           // --phase <id> (mutually exclusive with Task)
    Task          string        // --task <id> (mutually exclusive with Phase)
    MaxIterations int           // --max-iterations (default: 50)
    MaxLimitWaits int           // --max-limit-waits (default: 5)
    Sleep         int           // --sleep <seconds> (default: 5)
    DryRun        bool          // --dry-run
    Model         string        // --model <override>
}

// runImplement is the command's RunE function.
// 1. Load configuration
// 2. Discover tasks from tasks_dir
// 3. Create StateManager for task_state_file
// 4. Load phases from phases_conf
// 5. Create TaskSelector
// 6. Look up agent from registry (or create from config)
// 7. Check agent prerequisites
// 8. Create PromptGenerator
// 9. Create RateLimitCoordinator
// 10. Create Runner
// 11. Delegate to Runner.Run() or Runner.RunSingleTask()
// 12. Return appropriate exit code
func runImplement(cmd *cobra.Command, args []string, flags implementFlags) error
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| spf13/cobra | v1.10+ | CLI command framework |
| internal/config (T-009) | - | Configuration loading |
| internal/task (T-016) | - | DiscoverTasks |
| internal/task (T-017) | - | StateManager |
| internal/task (T-018) | - | LoadPhases |
| internal/task (T-019) | - | TaskSelector |
| internal/agent (T-021) | - | Agent interface, Registry |
| internal/agent (T-022) | - | ClaudeAgent |
| internal/agent (T-023) | - | CodexAgent |
| internal/agent (T-025) | - | RateLimitCoordinator |
| internal/loop (T-026) | - | PromptGenerator |
| internal/loop (T-027) | - | Runner |
| internal/loop (T-028) | - | Recovery mechanisms |
| internal/git (T-015) | - | Git client for dirty-tree recovery |
| charmbracelet/log | latest | Structured logging |

## Acceptance Criteria
- [ ] `raven implement --agent claude --phase 1` runs the full implementation loop for phase 1
- [ ] `raven implement --agent claude --task T-007` runs the loop for a single task
- [ ] `--agent` flag is required (error if missing)
- [ ] Either `--phase` or `--task` must be specified (error if neither or both)
- [ ] `--phase all` or `--phase 0` runs all phases sequentially
- [ ] `--dry-run` generates and displays prompts without invoking the agent
- [ ] `--max-iterations` limits loop iterations (default: 50)
- [ ] `--max-limit-waits` limits rate-limit wait cycles (default: 5)
- [ ] `--sleep` configures sleep between iterations in seconds (default: 5)
- [ ] `--model` overrides the agent's configured model
- [ ] Unknown agent name produces clear error with list of available agents
- [ ] Agent prerequisite check runs before loop starts (clear error if agent CLI not installed)
- [ ] Exit code 0 on success, 1 on error, 2 on partial success (some tasks completed), 3 on user cancellation
- [ ] Progress output goes to stderr, structured data to stdout
- [ ] Command registered as subcommand of root (accessible as `raven implement`)
- [ ] Command help text includes usage examples
- [ ] Shell completion for `--agent` provides available agent names

## Testing Requirements
### Unit Tests
- Flag validation: missing --agent returns error
- Flag validation: neither --phase nor --task returns error
- Flag validation: both --phase and --task returns error
- Flag validation: --phase 0 treated as "all phases"
- Flag validation: unknown agent name returns error with suggestions
- Default flag values: max-iterations=50, max-limit-waits=5, sleep=5
- Model override: --model flag overrides config model in RunOpts

### Integration Tests
- Full implement command with mock agent: `raven implement --agent mock --task T-001`
- Implement with --dry-run: prompt displayed, no agent invocation
- Implement with nonexistent phase: clear error
- Implement with already-completed task: loop exits immediately

### Edge Cases to Handle
- Config file not found: use defaults for paths, but warn
- Tasks directory does not exist: clear error message
- Task state file does not exist: initialize with not_started entries
- Phases.conf does not exist: error for --phase mode, ok for --task mode
- Agent CLI not installed: error from CheckPrerequisites with installation help
- No tasks in specified phase: loop exits immediately with success
- All tasks in phase already completed: loop exits immediately with success
- Ctrl+C during agent execution: graceful shutdown with exit code 3
- Git not available: warn but continue (dirty-tree recovery disabled)

## Implementation Notes
### Recommended Approach
1. Define implementFlags struct and create Cobra command with all flags
2. Register command in root command's init (or AddCommand call)
3. RunE function is the composition root:
   a. Load config via config.Load()
   b. Validate flags (agent required, phase xor task)
   c. Discover tasks from config.Project.TasksDir
   d. Create StateManager with config.Project.TaskStateFile
   e. Load phases from config.Project.PhasesConf
   f. Create TaskSelector
   g. Create agent registry, register Claude + Codex adapters
   h. Look up agent by name, check prerequisites
   i. If --model set, override agent config model
   j. Create PromptGenerator with config.Project.PromptDir
   k. Create RateLimitCoordinator with defaults
   l. Create Runner with all deps
   m. Build RunConfig from flags
   n. Call Runner.Run() or Runner.RunSingleTask()
   o. Handle return error -> exit code mapping
4. For shell completion of --agent: register a ValidArgsFunction that returns registry.List()
5. Add usage examples in the command's Long description

### Potential Pitfalls
- The implement command is a composition root with many dependencies. If any initialization fails, the error must be clear and point to the specific issue (e.g., "failed to load phases.conf: file not found at docs/tasks/phases.conf")
- Agent creation depends on config sections (e.g., `[agents.claude]`). If the config section is missing, use sensible defaults for the agent
- The --phase and --task flags are mutually exclusive -- use Cobra's MarkFlagsMutuallyExclusive or manual validation
- Exit code mapping: error from Runner.Run() -> exit 1; context.Canceled -> exit 3; partial completion (some tasks done) -> exit 2
- Do not import all agent packages directly -- use the registry pattern to avoid tight coupling

### Security Considerations
- The implement command invokes external agent CLI tools -- ensure no command injection via flag values
- Log the agent command at debug level (may contain prompts with proprietary code)
- Environment variables passed to agents inherit from the parent process

## References
- [PRD Section 5.11 - raven implement](docs/prd/PRD-Raven.md)
- [PRD Section 5.4 - Implementation Loop Engine](docs/prd/PRD-Raven.md)
- [PRD Section 7 Phase 2 - Deliverable](docs/prd/PRD-Raven.md)
- [spf13/cobra command documentation](https://pkg.go.dev/github.com/spf13/cobra)
- [Cobra flag validation patterns](https://github.com/spf13/cobra/blob/main/site/content/user_guide.md)
