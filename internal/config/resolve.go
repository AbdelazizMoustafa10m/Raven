package config

// ConfigSource identifies where a configuration value came from.
type ConfigSource string

const (
	// SourceDefault indicates the value came from built-in defaults.
	SourceDefault ConfigSource = "default"
	// SourceFile indicates the value came from the raven.toml config file.
	SourceFile ConfigSource = "file"
	// SourceEnv indicates the value came from an environment variable.
	SourceEnv ConfigSource = "env"
	// SourceCLI indicates the value came from a CLI flag.
	SourceCLI ConfigSource = "cli"
)

// ResolvedConfig holds the fully-resolved configuration with source tracking.
// The Config field contains the merged values; Sources tracks where each came from.
type ResolvedConfig struct {
	Config  *Config
	Sources map[string]ConfigSource // key is dotted path, e.g., "project.name"
	Path    string                  // path to the config file used (empty if none)
}

// CLIOverrides captures flag values that can override configuration.
// Nil/zero values mean "not set" (do not override). A *string that is nil
// means "not overridden"; a *string pointing to "" means "override to empty string."
type CLIOverrides struct {
	ProjectName *string
	LogDir      *string
	TasksDir    *string
	Verbose     *bool
	Quiet       *bool
	AgentModel  *string
	AgentEffort *string
}

// EnvFunc is a function that looks up environment variables.
// Default implementation is os.LookupEnv. Injected for testability.
type EnvFunc func(key string) (string, bool)

// Resolve merges configuration from all sources in priority order:
// CLI flags > environment variables > config file > defaults.
//
// Parameters:
//   - defaults: built-in default config (from NewDefaults())
//   - fileConfig: parsed config from raven.toml (nil if no file found)
//   - envFn: function to look up environment variables
//   - overrides: CLI flag values (nil fields mean "not set")
//
// Returns the fully-resolved config with source annotations.
func Resolve(defaults *Config, fileConfig *Config, envFn EnvFunc, overrides *CLIOverrides) *ResolvedConfig {
	rc := &ResolvedConfig{
		Config:  &Config{},
		Sources: make(map[string]ConfigSource),
	}

	// Ensure we have a valid defaults to start from.
	if defaults == nil {
		defaults = &Config{}
	}

	// Ensure we have a valid envFn.
	if envFn == nil {
		envFn = func(string) (string, bool) { return "", false }
	}

	// Ensure we have a valid overrides.
	if overrides == nil {
		overrides = &CLIOverrides{}
	}

	// Layer 1: Start with defaults as the base.
	resolveProjectFromDefaults(rc, defaults)
	resolveReviewFromDefaults(rc, defaults)
	resolveAgentsFromDefaults(rc, defaults)
	resolveWorkflowsFromDefaults(rc, defaults)

	// Layer 2: Merge file config on top (non-zero string values override; maps merge keys).
	if fileConfig != nil {
		resolveProjectFromFile(rc, fileConfig)
		resolveReviewFromFile(rc, fileConfig)
		resolveAgentsFromFile(rc, fileConfig)
		resolveWorkflowsFromFile(rc, fileConfig)
	}

	// Layer 3: Merge environment variables on top.
	resolveFromEnv(rc, envFn)

	// Layer 4: Merge CLI overrides on top.
	resolveFromCLI(rc, overrides)

	return rc
}

// --- Layer 1: Defaults ---

func resolveProjectFromDefaults(rc *ResolvedConfig, defaults *Config) {
	p := &rc.Config.Project
	d := &defaults.Project

	setString(&p.Name, d.Name, "project.name", SourceDefault, rc.Sources)
	setString(&p.Language, d.Language, "project.language", SourceDefault, rc.Sources)
	setString(&p.TasksDir, d.TasksDir, "project.tasks_dir", SourceDefault, rc.Sources)
	setString(&p.TaskStateFile, d.TaskStateFile, "project.task_state_file", SourceDefault, rc.Sources)
	setString(&p.PhasesConf, d.PhasesConf, "project.phases_conf", SourceDefault, rc.Sources)
	setString(&p.ProgressFile, d.ProgressFile, "project.progress_file", SourceDefault, rc.Sources)
	setString(&p.LogDir, d.LogDir, "project.log_dir", SourceDefault, rc.Sources)
	setString(&p.PromptDir, d.PromptDir, "project.prompt_dir", SourceDefault, rc.Sources)
	setString(&p.BranchTemplate, d.BranchTemplate, "project.branch_template", SourceDefault, rc.Sources)

	if len(d.VerificationCommands) > 0 {
		rc.Config.Project.VerificationCommands = make([]string, len(d.VerificationCommands))
		copy(rc.Config.Project.VerificationCommands, d.VerificationCommands)
	}
	rc.Sources["project.verification_commands"] = SourceDefault
}

func resolveReviewFromDefaults(rc *ResolvedConfig, defaults *Config) {
	r := &rc.Config.Review
	d := &defaults.Review

	setString(&r.Extensions, d.Extensions, "review.extensions", SourceDefault, rc.Sources)
	setString(&r.RiskPatterns, d.RiskPatterns, "review.risk_patterns", SourceDefault, rc.Sources)
	setString(&r.PromptsDir, d.PromptsDir, "review.prompts_dir", SourceDefault, rc.Sources)
	setString(&r.RulesDir, d.RulesDir, "review.rules_dir", SourceDefault, rc.Sources)
	setString(&r.ProjectBriefFile, d.ProjectBriefFile, "review.project_brief_file", SourceDefault, rc.Sources)
}

func resolveAgentsFromDefaults(rc *ResolvedConfig, defaults *Config) {
	rc.Config.Agents = make(map[string]AgentConfig)
	if defaults.Agents != nil {
		for name, agent := range defaults.Agents {
			rc.Config.Agents[name] = copyAgentConfig(agent)
			setAgentSources(rc.Sources, name, SourceDefault)
		}
	}
}

func resolveWorkflowsFromDefaults(rc *ResolvedConfig, defaults *Config) {
	rc.Config.Workflows = make(map[string]WorkflowConfig)
	if defaults.Workflows != nil {
		for name, wf := range defaults.Workflows {
			rc.Config.Workflows[name] = copyWorkflowConfig(wf)
		}
	}
}

// --- Layer 2: File ---

func resolveProjectFromFile(rc *ResolvedConfig, file *Config) {
	p := &rc.Config.Project
	f := &file.Project

	mergeString(&p.Name, f.Name, "project.name", SourceFile, rc.Sources)
	mergeString(&p.Language, f.Language, "project.language", SourceFile, rc.Sources)
	mergeString(&p.TasksDir, f.TasksDir, "project.tasks_dir", SourceFile, rc.Sources)
	mergeString(&p.TaskStateFile, f.TaskStateFile, "project.task_state_file", SourceFile, rc.Sources)
	mergeString(&p.PhasesConf, f.PhasesConf, "project.phases_conf", SourceFile, rc.Sources)
	mergeString(&p.ProgressFile, f.ProgressFile, "project.progress_file", SourceFile, rc.Sources)
	mergeString(&p.LogDir, f.LogDir, "project.log_dir", SourceFile, rc.Sources)
	mergeString(&p.PromptDir, f.PromptDir, "project.prompt_dir", SourceFile, rc.Sources)
	mergeString(&p.BranchTemplate, f.BranchTemplate, "project.branch_template", SourceFile, rc.Sources)

	if len(f.VerificationCommands) > 0 {
		rc.Config.Project.VerificationCommands = make([]string, len(f.VerificationCommands))
		copy(rc.Config.Project.VerificationCommands, f.VerificationCommands)
		rc.Sources["project.verification_commands"] = SourceFile
	}
}

func resolveReviewFromFile(rc *ResolvedConfig, file *Config) {
	r := &rc.Config.Review
	f := &file.Review

	mergeString(&r.Extensions, f.Extensions, "review.extensions", SourceFile, rc.Sources)
	mergeString(&r.RiskPatterns, f.RiskPatterns, "review.risk_patterns", SourceFile, rc.Sources)
	mergeString(&r.PromptsDir, f.PromptsDir, "review.prompts_dir", SourceFile, rc.Sources)
	mergeString(&r.RulesDir, f.RulesDir, "review.rules_dir", SourceFile, rc.Sources)
	mergeString(&r.ProjectBriefFile, f.ProjectBriefFile, "review.project_brief_file", SourceFile, rc.Sources)
}

func resolveAgentsFromFile(rc *ResolvedConfig, file *Config) {
	if file.Agents == nil {
		return
	}
	for name, agent := range file.Agents {
		rc.Config.Agents[name] = copyAgentConfig(agent)
		setAgentSources(rc.Sources, name, SourceFile)
	}
}

func resolveWorkflowsFromFile(rc *ResolvedConfig, file *Config) {
	if file.Workflows == nil {
		return
	}
	for name, wf := range file.Workflows {
		rc.Config.Workflows[name] = copyWorkflowConfig(wf)
	}
}

// --- Layer 3: Environment ---

// Environment variable mapping:
//
//	RAVEN_PROJECT_NAME       -> project.name
//	RAVEN_TASKS_DIR          -> project.tasks_dir
//	RAVEN_LOG_DIR            -> project.log_dir
//	RAVEN_PROMPT_DIR         -> project.prompt_dir
//	RAVEN_BRANCH_TEMPLATE    -> project.branch_template
//	RAVEN_AGENT_MODEL        -> agents.*.model (applies to all agents)
//	RAVEN_AGENT_EFFORT       -> agents.*.effort (applies to all agents)
func resolveFromEnv(rc *ResolvedConfig, envFn EnvFunc) {
	p := &rc.Config.Project

	// Project-level env vars with explicit field-by-field mapping.
	if val, ok := envFn("RAVEN_PROJECT_NAME"); ok {
		p.Name = val
		rc.Sources["project.name"] = SourceEnv
	}
	if val, ok := envFn("RAVEN_TASKS_DIR"); ok {
		p.TasksDir = val
		rc.Sources["project.tasks_dir"] = SourceEnv
	}
	if val, ok := envFn("RAVEN_LOG_DIR"); ok {
		p.LogDir = val
		rc.Sources["project.log_dir"] = SourceEnv
	}
	if val, ok := envFn("RAVEN_PROMPT_DIR"); ok {
		p.PromptDir = val
		rc.Sources["project.prompt_dir"] = SourceEnv
	}
	if val, ok := envFn("RAVEN_BRANCH_TEMPLATE"); ok {
		p.BranchTemplate = val
		rc.Sources["project.branch_template"] = SourceEnv
	}

	// Agent-level env vars apply to ALL agents in the merged map.
	modelVal, modelSet := envFn("RAVEN_AGENT_MODEL")
	effortVal, effortSet := envFn("RAVEN_AGENT_EFFORT")

	if modelSet || effortSet {
		for name, agent := range rc.Config.Agents {
			if modelSet {
				agent.Model = modelVal
				rc.Sources["agents."+name+".model"] = SourceEnv
			}
			if effortSet {
				agent.Effort = effortVal
				rc.Sources["agents."+name+".effort"] = SourceEnv
			}
			rc.Config.Agents[name] = agent
		}
	}
}

// --- Layer 4: CLI overrides ---

func resolveFromCLI(rc *ResolvedConfig, overrides *CLIOverrides) {
	p := &rc.Config.Project

	if overrides.ProjectName != nil {
		p.Name = *overrides.ProjectName
		rc.Sources["project.name"] = SourceCLI
	}
	if overrides.LogDir != nil {
		p.LogDir = *overrides.LogDir
		rc.Sources["project.log_dir"] = SourceCLI
	}
	if overrides.TasksDir != nil {
		p.TasksDir = *overrides.TasksDir
		rc.Sources["project.tasks_dir"] = SourceCLI
	}

	// Agent-level CLI overrides apply to ALL agents in the merged map.
	if overrides.AgentModel != nil || overrides.AgentEffort != nil {
		for name, agent := range rc.Config.Agents {
			if overrides.AgentModel != nil {
				agent.Model = *overrides.AgentModel
				rc.Sources["agents."+name+".model"] = SourceCLI
			}
			if overrides.AgentEffort != nil {
				agent.Effort = *overrides.AgentEffort
				rc.Sources["agents."+name+".effort"] = SourceCLI
			}
			rc.Config.Agents[name] = agent
		}
	}
}

// --- Helpers ---

// setString unconditionally sets the target to the given value and records the source.
func setString(target *string, value string, path string, source ConfigSource, sources map[string]ConfigSource) {
	*target = value
	sources[path] = source
}

// mergeString overwrites the target only if value is non-empty (non-zero string).
// For file-layer merging, an empty string in the file means "not set in file",
// so it does not override the default.
func mergeString(target *string, value string, path string, source ConfigSource, sources map[string]ConfigSource) {
	if value != "" {
		*target = value
		sources[path] = source
	}
}

// copyAgentConfig returns a deep copy of an AgentConfig.
func copyAgentConfig(src AgentConfig) AgentConfig {
	return AgentConfig{
		Command:        src.Command,
		Model:          src.Model,
		Effort:         src.Effort,
		PromptTemplate: src.PromptTemplate,
		AllowedTools:   src.AllowedTools,
	}
}

// setAgentSources records the source for all fields of a named agent.
func setAgentSources(sources map[string]ConfigSource, name string, source ConfigSource) {
	prefix := "agents." + name
	sources[prefix+".command"] = source
	sources[prefix+".model"] = source
	sources[prefix+".effort"] = source
	sources[prefix+".prompt_template"] = source
	sources[prefix+".allowed_tools"] = source
}

// copyWorkflowConfig returns a deep copy of a WorkflowConfig.
func copyWorkflowConfig(src WorkflowConfig) WorkflowConfig {
	wf := WorkflowConfig{
		Description: src.Description,
	}
	if src.Steps != nil {
		wf.Steps = make([]string, len(src.Steps))
		copy(wf.Steps, src.Steps)
	}
	if src.Transitions != nil {
		wf.Transitions = make(map[string]map[string]string, len(src.Transitions))
		for k, v := range src.Transitions {
			inner := make(map[string]string, len(v))
			for ik, iv := range v {
				inner[ik] = iv
			}
			wf.Transitions[k] = inner
		}
	}
	return wf
}
