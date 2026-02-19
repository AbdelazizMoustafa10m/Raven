package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
)

// configCmd is the parent "config" namespace command. It has no action of its
// own -- it groups debug and validate subcommands.
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management commands",
	Long:  "Inspect, validate, and debug Raven configuration.",
	// RunE shows help when invoked with no subcommand.
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// configDebugCmd implements "raven config debug".
// It prints the fully-resolved configuration with source annotations.
var configDebugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Show resolved configuration with source annotations",
	Long: `Display the fully-resolved configuration showing each value and
the source where it came from (cli flag, environment variable, config file, or default).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		resolved, _, err := loadAndResolveConfig()
		if err != nil {
			return err
		}
		printResolvedConfig(cmd, resolved)
		return nil
	},
}

// configValidateCmd implements "raven config validate".
// It validates the resolved configuration and reports all errors and warnings.
var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration and report issues",
	Long:  "Check the configuration for errors and warnings.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		resolved, meta, err := loadAndResolveConfig()
		if err != nil {
			return err
		}
		result := config.Validate(resolved.Config, meta)
		printValidationResult(cmd, result)
		if result.HasErrors() {
			return fmt.Errorf("configuration has %d error(s)", len(result.Errors()))
		}
		return nil
	},
}

func init() {
	configCmd.AddCommand(configDebugCmd)
	configCmd.AddCommand(configValidateCmd)
	rootCmd.AddCommand(configCmd)
}

// loadAndResolveConfig loads and resolves the configuration from all sources
// (file, env, CLI flags). It returns the resolved config, the TOML metadata
// (nil when no file was found), and any loading error.
//
// When flagConfig is set, that path is used directly. Otherwise,
// config.FindConfigFile searches upward from the current directory.
func loadAndResolveConfig() (*config.ResolvedConfig, *toml.MetaData, error) {
	var (
		fileCfg *config.Config
		meta    *toml.MetaData
		cfgPath string
	)

	if flagConfig != "" {
		// Explicit --config path provided.
		cfgPath = flagConfig
		fc, md, err := config.LoadFromFile(cfgPath)
		if err != nil {
			return nil, nil, fmt.Errorf("loading config: %w", err)
		}
		fileCfg = fc
		meta = &md
	} else {
		// Auto-detect raven.toml by walking up from cwd.
		found, err := config.FindConfigFile(".")
		if err != nil {
			return nil, nil, fmt.Errorf("finding config file: %w", err)
		}
		if found != "" {
			cfgPath = found
			fc, md, err := config.LoadFromFile(cfgPath)
			if err != nil {
				return nil, nil, fmt.Errorf("loading config: %w", err)
			}
			fileCfg = fc
			meta = &md
		}
	}

	resolved := config.Resolve(config.NewDefaults(), fileCfg, os.LookupEnv, nil)
	resolved.Path = cfgPath

	return resolved, meta, nil
}

// ---- Lipgloss styles --------------------------------------------------------

// sourceStyle returns a lipgloss style for a given ConfigSource.
// When --no-color is active, lipgloss automatically strips ANSI because
// the root PersistentPreRunE sets the color profile to Ascii.
func sourceStyle(src config.ConfigSource) lipgloss.Style {
	switch src {
	case config.SourceFile:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("12")) // bright blue
	case config.SourceEnv:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // bright yellow
	case config.SourceCLI:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")) // bright red
	default: // SourceDefault
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // bright green
	}
}

var (
	styleHeader    = lipgloss.NewStyle().Bold(true)
	styleSeparator = lipgloss.NewStyle()
	styleSection   = lipgloss.NewStyle().Bold(true)
	styleErrorLbl  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)  // red
	styleWarnLbl   = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true) // yellow
	styleSuccess   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))            // green
)

// ---- printResolvedConfig ----------------------------------------------------

const fieldWidth = 24 // column width for field names

// printResolvedConfig writes the formatted resolved configuration to cmd's
// output writer (stdout by default).
func printResolvedConfig(cmd *cobra.Command, rc *config.ResolvedConfig) {
	out := cmd.OutOrStdout()

	header := styleHeader.Render("Configuration Debug")
	sep := styleSeparator.Render(strings.Repeat("=", len("Configuration Debug")))
	fmt.Fprintln(out, header)
	fmt.Fprintln(out, sep)
	fmt.Fprintln(out)

	if rc.Path != "" {
		fmt.Fprintf(out, "Config file: %s\n", rc.Path)
	} else {
		fmt.Fprintln(out, "Config file: none found")
	}
	fmt.Fprintln(out)

	// --- [project] ---
	fmt.Fprintln(out, styleSection.Render("[project]"))
	p := rc.Config.Project
	printField(out, "name", fmtStr(p.Name), rc.Sources["project.name"])
	printField(out, "language", fmtStr(p.Language), rc.Sources["project.language"])
	printField(out, "tasks_dir", fmtStr(p.TasksDir), rc.Sources["project.tasks_dir"])
	printField(out, "task_state_file", fmtStr(p.TaskStateFile), rc.Sources["project.task_state_file"])
	printField(out, "phases_conf", fmtStr(p.PhasesConf), rc.Sources["project.phases_conf"])
	printField(out, "progress_file", fmtStr(p.ProgressFile), rc.Sources["project.progress_file"])
	printField(out, "log_dir", fmtStr(p.LogDir), rc.Sources["project.log_dir"])
	printField(out, "prompt_dir", fmtStr(p.PromptDir), rc.Sources["project.prompt_dir"])
	printField(out, "branch_template", fmtStr(p.BranchTemplate), rc.Sources["project.branch_template"])
	printField(out, "verification_commands", fmtSlice(p.VerificationCommands), rc.Sources["project.verification_commands"])
	fmt.Fprintln(out)

	// --- [agents.*] (sorted for determinism) ---
	if len(rc.Config.Agents) > 0 {
		names := make([]string, 0, len(rc.Config.Agents))
		for n := range rc.Config.Agents {
			names = append(names, n)
		}
		sort.Strings(names)

		for _, name := range names {
			agent := rc.Config.Agents[name]
			prefix := "agents." + name
			fmt.Fprintln(out, styleSection.Render(fmt.Sprintf("[agents.%s]", name)))
			printField(out, "command", fmtStr(agent.Command), rc.Sources[prefix+".command"])
			printField(out, "model", fmtStr(agent.Model), rc.Sources[prefix+".model"])
			printField(out, "effort", fmtStr(agent.Effort), rc.Sources[prefix+".effort"])
			printField(out, "prompt_template", fmtStr(agent.PromptTemplate), rc.Sources[prefix+".prompt_template"])
			printField(out, "allowed_tools", fmtStr(agent.AllowedTools), rc.Sources[prefix+".allowed_tools"])
			fmt.Fprintln(out)
		}
	}

	// --- [review] ---
	fmt.Fprintln(out, styleSection.Render("[review]"))
	r := rc.Config.Review
	printField(out, "extensions", fmtStr(r.Extensions), rc.Sources["review.extensions"])
	printField(out, "risk_patterns", fmtStr(r.RiskPatterns), rc.Sources["review.risk_patterns"])
	printField(out, "prompts_dir", fmtStr(r.PromptsDir), rc.Sources["review.prompts_dir"])
	printField(out, "rules_dir", fmtStr(r.RulesDir), rc.Sources["review.rules_dir"])
	printField(out, "project_brief_file", fmtStr(r.ProjectBriefFile), rc.Sources["review.project_brief_file"])
	fmt.Fprintln(out)

	// --- [workflows.*] (sorted for determinism) ---
	if len(rc.Config.Workflows) > 0 {
		wfNames := make([]string, 0, len(rc.Config.Workflows))
		for n := range rc.Config.Workflows {
			wfNames = append(wfNames, n)
		}
		sort.Strings(wfNames)

		for _, name := range wfNames {
			wf := rc.Config.Workflows[name]
			fmt.Fprintln(out, styleSection.Render(fmt.Sprintf("[workflows.%s]", name)))
			printField(out, "description", fmtStr(wf.Description), config.SourceDefault)
			printField(out, "steps", fmtSlice(wf.Steps), config.SourceDefault)
			fmt.Fprintln(out)
		}
	}
}

// printField writes a single key = value (source: ...) line.
func printField(out io.Writer, name, value string, src config.ConfigSource) {
	// Left-pad the field name to fieldWidth.
	padded := fmt.Sprintf("  %-*s", fieldWidth, name)
	srcLabel := sourceStyle(src).Render(fmt.Sprintf("(source: %s)", src))
	line := fmt.Sprintf("%s = %-40s %s\n", padded, value, srcLabel)
	fmt.Fprint(out, line)
}

// fmtStr formats a string value for display (quoted).
func fmtStr(s string) string {
	return fmt.Sprintf("%q", s)
}

// fmtSlice formats a string slice for display.
func fmtSlice(ss []string) string {
	if len(ss) == 0 {
		return "[]"
	}
	quoted := make([]string, len(ss))
	for i, s := range ss {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// ---- printValidationResult --------------------------------------------------

// printValidationResult writes the formatted validation report to cmd's
// output writer.
func printValidationResult(cmd *cobra.Command, result *config.ValidationResult) {
	out := cmd.OutOrStdout()

	header := styleHeader.Render("Configuration Validation")
	sep := styleSeparator.Render(strings.Repeat("=", len("Configuration Validation")))
	fmt.Fprintln(out, header)
	fmt.Fprintln(out, sep)
	fmt.Fprintln(out)

	errs := result.Errors()
	warns := result.Warnings()

	if len(errs) == 0 && len(warns) == 0 {
		fmt.Fprintln(out, styleSuccess.Render("No issues found."))
		return
	}

	if len(errs) > 0 {
		fmt.Fprintln(out, styleErrorLbl.Render("Errors:"))
		for _, issue := range errs {
			fmt.Fprintf(out, "  [%s] %s\n", issue.Field, issue.Message)
		}
		fmt.Fprintln(out)
	}

	if len(warns) > 0 {
		fmt.Fprintln(out, styleWarnLbl.Render("Warnings:"))
		for _, issue := range warns {
			fmt.Fprintf(out, "  [%s] %s\n", issue.Field, issue.Message)
		}
		fmt.Fprintln(out)
	}

	fmt.Fprintf(out, "%d error(s), %d warning(s)\n", len(errs), len(warns))
}
