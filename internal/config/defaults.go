package config

// NewDefaults returns a Config populated with all default values.
// These defaults match the PRD-specified defaults for a Go CLI project.
func NewDefaults() *Config {
	return &Config{
		Project: ProjectConfig{
			TasksDir:       "docs/tasks",
			TaskStateFile:  "docs/tasks/task-state.conf",
			PhasesConf:     "docs/tasks/phases.conf",
			ProgressFile:   "docs/tasks/PROGRESS.md",
			LogDir:         "scripts/logs",
			PromptDir:      "prompts",
			BranchTemplate: "phase/{phase_id}-{slug}",
		},
		Agents:    map[string]AgentConfig{},
		Workflows: map[string]WorkflowConfig{},
	}
}
