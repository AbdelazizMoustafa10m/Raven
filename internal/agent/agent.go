package agent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
)

// agentNameRe validates agent names: alphanumeric characters and hyphens only.
var agentNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`)

// ErrNotFound is returned by Registry.Get when no agent with the requested
// name has been registered.
var ErrNotFound = errors.New("agent not found")

// ErrDuplicateName is returned by Registry.Register when an agent with the
// same name is already present in the registry.
var ErrDuplicateName = errors.New("agent already registered")

// ErrInvalidName is returned by Registry.Register when the agent name is
// empty or contains invalid characters.
var ErrInvalidName = errors.New("invalid agent name")

// Agent is the interface that all AI agent adapters must implement.
// It abstracts the differences between Claude, Codex, Gemini, and other
// agents behind a common contract for prompt execution and rate-limit handling.
type Agent interface {
	// Name returns the agent's identifier (e.g., "claude", "codex").
	// The name must be lowercase and contain only alphanumeric characters
	// and hyphens.
	Name() string

	// Run executes a prompt using the agent and returns the result.
	// The context is used for cancellation and timeout.
	Run(ctx context.Context, opts RunOpts) (*RunResult, error)

	// CheckPrerequisites verifies that the agent's CLI tool is installed
	// and accessible. Returns an error describing what is missing.
	CheckPrerequisites() error

	// ParseRateLimit examines agent output for rate-limit signals.
	// Returns rate-limit info and true if a rate limit was detected,
	// or nil and false if no rate limit is present.
	ParseRateLimit(output string) (*RateLimitInfo, bool)

	// DryRunCommand returns the command string that would be executed,
	// without actually running it. Used for --dry-run mode.
	DryRunCommand(opts RunOpts) string
}

// AgentConfig holds agent-specific configuration from raven.toml.
// It maps to the [agents.<name>] section in the configuration file and is
// consumed by agent constructors (Claude, Codex, Gemini adapters).
type AgentConfig struct {
	// Command is the CLI executable name (e.g., "claude", "codex").
	Command string `toml:"command"`

	// Model is the AI model identifier (e.g., "claude-sonnet-4-20250514").
	Model string `toml:"model"`

	// Effort controls the effort/reasoning level (e.g., "high", "medium", "low").
	Effort string `toml:"effort"`

	// PromptTemplate is a path or name of the prompt template to use.
	PromptTemplate string `toml:"prompt_template"`

	// AllowedTools is a comma-separated list of tools the agent may invoke
	// (e.g., "bash,edit,computer").
	AllowedTools string `toml:"allowed_tools"`
}

// Registry stores named agent instances for lookup.
// Agents are registered at startup and looked up by name at runtime.
// Registry is safe for concurrent reads after all registrations are complete.
type Registry struct {
	agents map[string]Agent
}

// NewRegistry creates an empty agent registry.
func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[string]Agent),
	}
}

// Register adds an agent to the registry under its Name().
// Returns ErrInvalidName if the agent is nil or has an invalid name.
// Returns ErrDuplicateName if an agent with the same name is already registered.
func (r *Registry) Register(a Agent) error {
	if a == nil {
		return fmt.Errorf("register agent: %w", ErrInvalidName)
	}
	name := a.Name()
	if name == "" || !agentNameRe.MatchString(name) {
		return fmt.Errorf("register agent %q: %w", name, ErrInvalidName)
	}
	if _, exists := r.agents[name]; exists {
		return fmt.Errorf("register agent %q: %w", name, ErrDuplicateName)
	}
	r.agents[name] = a
	return nil
}

// Get returns the agent registered under the given name.
// Returns ErrNotFound if no agent with that name is registered.
func (r *Registry) Get(name string) (Agent, error) {
	a, ok := r.agents[name]
	if !ok {
		return nil, fmt.Errorf("get agent %q: %w", name, ErrNotFound)
	}
	return a, nil
}

// MustGet returns the agent registered under the given name or panics.
// Only use this in initialization/setup code, never in request-handling paths.
func (r *Registry) MustGet(name string) Agent {
	a, err := r.Get(name)
	if err != nil {
		panic(fmt.Sprintf("agent.Registry.MustGet: agent %q not registered", name))
	}
	return a
}

// List returns the names of all registered agents, sorted alphabetically.
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.agents))
	for name := range r.agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Has returns true if an agent with the given name is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.agents[name]
	return ok
}
