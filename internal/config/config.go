package config

// Config is the top-level configuration structure mapping to raven.toml.
type Config struct {
	Project   ProjectConfig             `toml:"project"`
	Agents    map[string]AgentConfig    `toml:"agents"`
	Review    ReviewConfig              `toml:"review"`
	Workflows map[string]WorkflowConfig `toml:"workflows"`
}

// ProjectConfig maps to the [project] section in raven.toml.
type ProjectConfig struct {
	Name                 string   `toml:"name"`
	Language             string   `toml:"language"`
	TasksDir             string   `toml:"tasks_dir"`
	TaskStateFile        string   `toml:"task_state_file"`
	PhasesConf           string   `toml:"phases_conf"`
	ProgressFile         string   `toml:"progress_file"`
	LogDir               string   `toml:"log_dir"`
	PromptDir            string   `toml:"prompt_dir"`
	BranchTemplate       string   `toml:"branch_template"`
	VerificationCommands []string `toml:"verification_commands"`
}

// AgentConfig maps to an [agents.<name>] section in raven.toml.
type AgentConfig struct {
	Command        string `toml:"command"`
	Model          string `toml:"model"`
	Effort         string `toml:"effort"`
	PromptTemplate string `toml:"prompt_template"`
	AllowedTools   string `toml:"allowed_tools"`
}

// ReviewConfig maps to the [review] section in raven.toml.
type ReviewConfig struct {
	Extensions       string `toml:"extensions"`
	RiskPatterns     string `toml:"risk_patterns"`
	PromptsDir       string `toml:"prompts_dir"`
	RulesDir         string `toml:"rules_dir"`
	ProjectBriefFile string `toml:"project_brief_file"`
}

// WorkflowConfig maps to a [workflows.<name>] section in raven.toml.
type WorkflowConfig struct {
	Description string                       `toml:"description"`
	Steps       []string                     `toml:"steps"`
	Transitions map[string]map[string]string `toml:"transitions"`
}
