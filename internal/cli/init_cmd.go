package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
	"github.com/AbdelazizMoustafa10m/Raven/internal/logging"
)

// initFlagName and initFlagForce are the flag values for the init subcommand.
var (
	initFlagName  string
	initFlagForce bool
)

// initCmd implements "raven init [template]".
// It scaffolds a new Raven project from an embedded template without requiring
// an existing raven.toml -- making it safe to run in a fresh directory.
var initCmd = &cobra.Command{
	Use:   "init [template]",
	Short: "Initialize a new Raven project from a template",
	Long: `Initialize a new Raven project directory by rendering an embedded
project template. Existing files are preserved unless --force is supplied.

Available templates can be listed with: raven init --help

Examples:
  raven init                        # scaffold go-cli template in current directory
  raven init go-cli --name my-svc   # scaffold with explicit project name
  raven init go-cli --force         # overwrite existing files`,
	Args: cobra.MaximumNArgs(1),

	// Override PersistentPreRunE so the init command never attempts to load a
	// raven.toml.  We still replicate the env-var checks, logging setup, color
	// disable, and --dir handling from the root PersistentPreRunE.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Check env vars for flags not explicitly set on the command line.
		if !cmd.Root().PersistentFlags().Changed("verbose") && os.Getenv("RAVEN_VERBOSE") != "" {
			flagVerbose = true
		}
		if !cmd.Root().PersistentFlags().Changed("quiet") && os.Getenv("RAVEN_QUIET") != "" {
			flagQuiet = true
		}
		if !cmd.Root().PersistentFlags().Changed("no-color") &&
			(os.Getenv("NO_COLOR") != "" || os.Getenv("RAVEN_NO_COLOR") != "") {
			flagNoColor = true
		}

		// Initialize logging.
		jsonFormat := os.Getenv("RAVEN_LOG_FORMAT") == "json"
		logging.Setup(flagVerbose, flagQuiet, jsonFormat)

		// Handle --no-color: disable coloured output.
		if flagNoColor {
			lipgloss.SetColorProfile(termenv.Ascii)
		}

		// Handle --dir (change working directory).
		if flagDir != "" {
			if err := os.Chdir(flagDir); err != nil {
				return fmt.Errorf("changing directory to %s: %w", flagDir, err)
			}
		}

		return nil
	},

	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVarP(&initFlagName, "name", "n", "", "Project name (defaults to current directory name)")
	initCmd.Flags().BoolVar(&initFlagForce, "force", false, "Overwrite existing files")
	rootCmd.AddCommand(initCmd)
}

// runInit is the RunE handler for the init command.
func runInit(cmd *cobra.Command, args []string) error {
	// Resolve the template name (default: go-cli).
	templateName := "go-cli"
	if len(args) > 0 {
		templateName = args[0]
	}

	// Validate that the requested template exists.
	if !config.TemplateExists(templateName) {
		available, listErr := config.ListTemplates()
		if listErr != nil {
			return fmt.Errorf("listing available templates: %w", listErr)
		}
		return fmt.Errorf("template %q not found; available templates: %s",
			templateName, strings.Join(available, ", "))
	}

	// Resolve the destination directory (current working directory after any
	// --dir change applied in PersistentPreRunE).
	destDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	// Resolve the project name.
	projectName := initFlagName
	if projectName == "" {
		projectName = filepath.Base(destDir)
	}

	// Reject path traversal in project name.
	if strings.Contains(projectName, "../") || strings.Contains(projectName, "..\\") {
		return fmt.Errorf("invalid project name %q: must not contain path traversal sequences", projectName)
	}

	// Guard against overwriting an existing raven.toml unless --force is set.
	ravenToml := filepath.Join(destDir, "raven.toml")
	if _, statErr := os.Stat(ravenToml); statErr == nil && !initFlagForce {
		return fmt.Errorf("raven.toml already exists in %s; use --force to overwrite", destDir)
	}

	vars := config.TemplateVars{
		ProjectName: projectName,
		Language:    "go",
		ModulePath:  "github.com/example/" + projectName,
	}

	// Render the template.
	created, err := config.RenderTemplate(templateName, destDir, vars, initFlagForce)
	if err != nil {
		return fmt.Errorf("rendering template %q: %w", templateName, err)
	}

	// --- Success output (all to stderr) ---
	stderr := os.Stderr

	fmt.Fprintf(stderr, "Initialized project %q from template %q\n\n", projectName, templateName)

	if len(created) > 0 {
		fmt.Fprintln(stderr, "Created files:")
		for _, f := range created {
			// Print relative paths when possible for readability.
			rel, relErr := filepath.Rel(destDir, f)
			if relErr != nil {
				rel = f
			}
			fmt.Fprintf(stderr, "  %s\n", rel)
		}
		fmt.Fprintln(stderr)
	}

	fmt.Fprintln(stderr, "Next steps:")
	fmt.Fprintf(stderr, "  1. Edit %s to configure your project\n", ravenToml)
	fmt.Fprintln(stderr, "  2. Add task specs to docs/tasks/")
	fmt.Fprintln(stderr, "  3. Run: raven implement")

	return nil
}
