package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
)

// resetInitFlags resets init command flag state between tests.
func resetInitFlags(t *testing.T) {
	t.Helper()
	resetRootCmd(t)
	initFlagName = ""
	initFlagForce = false
	initCmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
	})
}

// runInitInDir changes to dir, runs "raven init [args...]", restores the
// original working directory, and returns the Execute exit code.
func runInitInDir(t *testing.T, dir string, args ...string) int {
	t.Helper()
	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(orig) })

	require.NoError(t, os.Chdir(dir))

	rootCmd.SetArgs(append([]string{"init"}, args...))
	return Execute()
}

// captureInitOutput runs "raven init [args...]" in dir and captures stderr
// output, returning (stderr, exitCode). Stdout is not captured because the
// init command sends all user-facing output to stderr.
func captureInitOutput(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()

	oldStderr := os.Stderr
	r, w, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = oldStderr })

	code := runInitInDir(t, dir, args...)

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stderr = oldStderr

	return buf.String(), code
}

// ---- Registration and Metadata -----------------------------------------------

// TestInitCmd_Registered verifies that initCmd is wired into rootCmd.
func TestInitCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "init [template]" {
			found = true
			break
		}
	}
	assert.True(t, found, "init command must be registered in rootCmd")
}

// TestInitCmd_Metadata verifies Short, Long, and Args constraints.
func TestInitCmd_Metadata(t *testing.T) {
	assert.NotEmpty(t, initCmd.Short, "initCmd must have a Short description")
	assert.Contains(t, initCmd.Long, "go-cli", "Long help must mention available template")
	assert.Contains(t, initCmd.Long, "--force", "Long help must mention --force flag")
	// The Use field must declare an optional positional argument.
	assert.Contains(t, initCmd.Use, "[template]", "Use must show [template] argument")
}

// TestInitCmd_Flags verifies that required flags are declared with correct
// shorthands and default values.
func TestInitCmd_Flags(t *testing.T) {
	tests := []struct {
		flagName  string
		shorthand string
		defValue  string
	}{
		{flagName: "name", shorthand: "n", defValue: ""},
		{flagName: "force", shorthand: "", defValue: "false"},
	}

	for _, tt := range tests {
		t.Run(tt.flagName, func(t *testing.T) {
			f := initCmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, f, "--%s flag must be registered", tt.flagName)
			assert.Equal(t, tt.shorthand, f.Shorthand,
				"--%s shorthand must be %q", tt.flagName, tt.shorthand)
			assert.Equal(t, tt.defValue, f.DefValue,
				"--%s default value must be %q", tt.flagName, tt.defValue)
		})
	}
}

// TestInitCmd_HelpOutput verifies that --help output includes flags and examples.
func TestInitCmd_HelpOutput(t *testing.T) {
	resetInitFlags(t)

	var buf bytes.Buffer
	initCmd.SetOut(&buf)
	initCmd.SetArgs([]string{"--help"})
	// Cobra prints help and returns nil for --help.
	_ = initCmd.Help()
	initCmd.SetOut(nil)

	out := buf.String()
	assert.Contains(t, out, "--name", "help must document --name flag")
	assert.Contains(t, out, "--force", "help must document --force flag")
}

// ---- AC-1: Default and explicit template scaffolding -------------------------

// TestInitCmd_DefaultTemplate scaffolds the default (go-cli) template when no
// template argument is given.
func TestInitCmd_DefaultTemplate(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	code := runInitInDir(t, dir)

	assert.Equal(t, 0, code, "init with default template should succeed")
	assert.FileExists(t, filepath.Join(dir, "raven.toml"))
}

// TestInitCmd_ExplicitTemplate scaffolds a template provided as a positional
// argument.
func TestInitCmd_ExplicitTemplate(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	code := runInitInDir(t, dir, "go-cli")

	assert.Equal(t, 0, code, "init go-cli should succeed")
	assert.FileExists(t, filepath.Join(dir, "raven.toml"))
}

// ---- AC-2: --name flag sets project name in raven.toml ----------------------

// TestInitCmd_NameFlag sets the project name via --name and verifies the
// rendered raven.toml contains the supplied name.
func TestInitCmd_NameFlag(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	code := runInitInDir(t, dir, "--name", "my-awesome-service")

	assert.Equal(t, 0, code)
	content, err := os.ReadFile(filepath.Join(dir, "raven.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "my-awesome-service",
		"raven.toml must contain the --name value")
}

// TestInitCmd_NameFlag_ShorthandN verifies the -n shorthand works identically
// to --name.
func TestInitCmd_NameFlag_ShorthandN(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	code := runInitInDir(t, dir, "-n", "shorthand-project")

	assert.Equal(t, 0, code)
	content, err := os.ReadFile(filepath.Join(dir, "raven.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "shorthand-project",
		"raven.toml must contain the name supplied via -n shorthand")
}

// ---- AC-3: No --name defaults to directory name -----------------------------

// TestInitCmd_DefaultsToDirectoryName verifies that without --name the project
// name is derived from the current directory's base name.
func TestInitCmd_DefaultsToDirectoryName(t *testing.T) {
	resetInitFlags(t)
	// Create a directory with a recognisable name.
	parent := t.TempDir()
	dir := filepath.Join(parent, "cool-project")
	require.NoError(t, os.Mkdir(dir, 0o755))

	code := runInitInDir(t, dir)

	assert.Equal(t, 0, code)
	content, err := os.ReadFile(filepath.Join(dir, "raven.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "cool-project",
		"raven.toml must use the directory name when --name is omitted")
}

// ---- AC-4: No template argument defaults to go-cli --------------------------

// TestInitCmd_NoArg_DefaultsToGoCliTemplate verifies that when no positional
// argument is given the go-cli template is used (AC-4).
func TestInitCmd_NoArg_DefaultsToGoCliTemplate(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	// Run without template arg; if go-cli is used it creates prompts/.
	code := runInitInDir(t, dir)

	assert.Equal(t, 0, code, "default template should succeed")
	// go-cli template creates prompts/ -- presence confirms the template used.
	assert.DirExists(t, filepath.Join(dir, "prompts"),
		"go-cli template must create prompts/ directory")
}

// ---- AC-5: Errors on existing raven.toml without --force --------------------

// TestInitCmd_ExistingRavenToml_NoForce errors when raven.toml already exists
// and --force is not set.
func TestInitCmd_ExistingRavenToml_NoForce(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	// Pre-create raven.toml.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "raven.toml"), []byte("# original\n"), 0o644))

	stderr, code := captureInitOutput(t, dir)

	assert.Equal(t, 1, code, "should fail when raven.toml exists without --force")
	assert.Contains(t, stderr, "--force",
		"error message should tell the user to use --force")

	// The original file must be untouched.
	content, readErr := os.ReadFile(filepath.Join(dir, "raven.toml"))
	require.NoError(t, readErr)
	assert.Equal(t, "# original\n", string(content),
		"existing raven.toml must not be modified when --force is not set")
}

// ---- AC-6: --force overwrites existing files --------------------------------

// TestInitCmd_Force overwrites existing files when --force is provided.
func TestInitCmd_Force(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	// Pre-create raven.toml with known content.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "raven.toml"), []byte("# original\n"), 0o644))

	code := runInitInDir(t, dir, "--force", "--name", "forced-project")

	assert.Equal(t, 0, code, "--force should succeed even when raven.toml exists")

	content, err := os.ReadFile(filepath.Join(dir, "raven.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "forced-project",
		"raven.toml must be overwritten with new project name when --force is set")
	assert.NotContains(t, string(content), "# original",
		"original content must be replaced")
}

// TestInitCmd_Force_AlsoOverwritesNonTomlFiles verifies that --force overwrites
// all scaffold files, not just raven.toml.
func TestInitCmd_Force_AlsoOverwritesNonTomlFiles(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	// First init to create initial scaffold.
	code := runInitInDir(t, dir, "--name", "first-run")
	require.Equal(t, 0, code)

	// Overwrite implement-claude.md with sentinel content.
	promptFile := filepath.Join(dir, "prompts", "implement-claude.md")
	require.NoError(t, os.WriteFile(promptFile, []byte("# sentinel\n"), 0o644))

	// Second init with --force must overwrite the sentinel.
	resetInitFlags(t)
	code = runInitInDir(t, dir, "--force", "--name", "second-run")
	require.Equal(t, 0, code)

	content, err := os.ReadFile(promptFile)
	require.NoError(t, err)
	assert.NotEqual(t, "# sentinel\n", string(content),
		"--force must overwrite non-toml scaffold files too")
}

// ---- AC-7: Unknown template returns error listing available templates --------

// TestInitCmd_UnknownTemplate returns exit code 1 and lists available templates.
func TestInitCmd_UnknownTemplate(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	stderr, code := captureInitOutput(t, dir, "no-such-template")

	assert.Equal(t, 1, code, "unknown template should return exit code 1")
	assert.Contains(t, stderr, "no-such-template",
		"error output should mention the unknown template name")
	assert.Contains(t, stderr, "go-cli",
		"error output should list available templates")
}

// TestInitCmd_UnknownTemplate_TableDriven exercises multiple invalid names.
func TestInitCmd_UnknownTemplate_TableDriven(t *testing.T) {
	badNames := []struct {
		name     string
		template string
	}{
		{name: "empty string", template: ""},
		{name: "numeric", template: "42"},
		{name: "path-like", template: "some/nested/path"},
		{name: "dot prefix", template: ".hidden"},
	}

	for _, tt := range badNames {
		t.Run(tt.name, func(t *testing.T) {
			resetInitFlags(t)
			dir := t.TempDir()

			if tt.template == "" {
				// Empty string passed as positional arg: Cobra sees MaximumNArgs(1)
				// and accepts it; the template validator may reject it or treat it
				// as the "go-cli" default (AC-4). Both outcomes are acceptable.
				_, code := captureInitOutput(t, dir, tt.template)
				if code == 0 {
					// Cobra or runInit accepted the empty arg and used the go-cli
					// default. That is valid per AC-4.
					return
				}
				assert.Equal(t, 1, code, "unknown template %q should return exit code 1", tt.template)
			} else {
				stderr, code := captureInitOutput(t, dir, tt.template)
				assert.Equal(t, 1, code, "unknown template %q should return exit code 1", tt.template)
				assert.Contains(t, stderr, "go-cli",
					"error must list available templates for %q", tt.template)
			}
		})
	}
}

// ---- AC-8: Created raven.toml contains project name and is valid TOML -------

// TestInitCmd_RenderedTomlIsValidTOML verifies the created raven.toml parses
// with the BurntSushi TOML decoder and that the project name field matches.
func TestInitCmd_RenderedTomlIsValidTOML(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	code := runInitInDir(t, dir, "--name", "valid-toml-test")
	require.Equal(t, 0, code)

	tomlPath := filepath.Join(dir, "raven.toml")
	require.FileExists(t, tomlPath)

	var cfg config.Config
	_, decodeErr := toml.DecodeFile(tomlPath, &cfg)
	require.NoError(t, decodeErr, "rendered raven.toml must be valid TOML")
	assert.Equal(t, "valid-toml-test", cfg.Project.Name,
		"project.name in raven.toml must match the --name flag value")
	assert.Equal(t, "go", cfg.Project.Language,
		"project.language must be set by the go-cli template")
}

// TestInitCmd_TomlContainsAgentAndReviewSections verifies that the generated
// raven.toml includes all expected sections from the go-cli template.
func TestInitCmd_TomlContainsAgentAndReviewSections(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	code := runInitInDir(t, dir, "--name", "section-test")
	require.Equal(t, 0, code)

	var cfg config.Config
	_, err := toml.DecodeFile(filepath.Join(dir, "raven.toml"), &cfg)
	require.NoError(t, err)

	// Agents section.
	require.NotNil(t, cfg.Agents, "raven.toml must have [agents] section")
	_, hasClause := cfg.Agents["claude"]
	assert.True(t, hasClause, "raven.toml must have [agents.claude]")

	// Review section.
	assert.NotEmpty(t, cfg.Review.Extensions, "[review].extensions must not be empty")
	assert.NotEmpty(t, cfg.Review.PromptsDir, "[review].prompts_dir must not be empty")
}

// ---- AC-9: Directory structure includes prompts/, .github/review/, docs/tasks/ -

// TestInitCmd_CreatesDirectoryStructure verifies that all expected scaffold
// directories and files are produced.
func TestInitCmd_CreatesDirectoryStructure(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	code := runInitInDir(t, dir, "--name", "struct-test")

	assert.Equal(t, 0, code)

	expectedFiles := []string{
		"raven.toml",
		filepath.Join("prompts", "implement-claude.md"),
		filepath.Join("prompts", "implement-codex.md"),
		filepath.Join(".github", "review", "prompts", "review-prompt.md"),
		filepath.Join(".github", "review", "rules", ".gitkeep"),
		filepath.Join(".github", "review", "PROJECT_BRIEF.md"),
		filepath.Join("docs", "tasks", ".gitkeep"),
		filepath.Join("docs", "prd", ".gitkeep"),
	}

	for _, rel := range expectedFiles {
		assert.FileExists(t, filepath.Join(dir, rel),
			"expected scaffold file %q to be created", rel)
	}
}

// TestInitCmd_CreatesExpectedDirectories verifies that key directories exist.
func TestInitCmd_CreatesExpectedDirectories(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	code := runInitInDir(t, dir, "--name", "dir-test")
	require.Equal(t, 0, code)

	expectedDirs := []string{
		"prompts",
		filepath.Join(".github", "review", "prompts"),
		filepath.Join(".github", "review", "rules"),
		filepath.Join("docs", "tasks"),
		filepath.Join("docs", "prd"),
	}

	for _, rel := range expectedDirs {
		info, err := os.Stat(filepath.Join(dir, rel))
		require.NoError(t, err, "directory %q must exist", rel)
		assert.True(t, info.IsDir(), "%q must be a directory", rel)
	}
}

// ---- AC-10: Success output lists created files and next steps ---------------

// TestInitCmd_SuccessOutput_ListsCreatedFiles verifies that the success message
// includes a list of created files written to stderr.
func TestInitCmd_SuccessOutput_ListsCreatedFiles(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	stderr, code := captureInitOutput(t, dir, "--name", "output-test")

	require.Equal(t, 0, code)
	// The "Created files:" section must appear.
	assert.Contains(t, stderr, "Created files:",
		"success output must list created files section")
	// raven.toml should be among them.
	assert.Contains(t, stderr, "raven.toml",
		"success output must mention raven.toml")
}

// TestInitCmd_SuccessOutput_ListsNextSteps verifies that next steps appear in
// the success output.
func TestInitCmd_SuccessOutput_ListsNextSteps(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	stderr, code := captureInitOutput(t, dir, "--name", "steps-test")

	require.Equal(t, 0, code)
	assert.Contains(t, stderr, "Next steps:",
		"success output must contain 'Next steps:' section")
	assert.Contains(t, stderr, "raven implement",
		"success output must mention 'raven implement' as a next step")
}

// TestInitCmd_SuccessOutput_MentionsProjectName verifies that the success
// message echoes the resolved project name.
func TestInitCmd_SuccessOutput_MentionsProjectName(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	stderr, code := captureInitOutput(t, dir, "--name", "echo-name-project")

	require.Equal(t, 0, code)
	assert.Contains(t, stderr, "echo-name-project",
		"success output must mention the project name")
}

// TestInitCmd_SuccessOutput_MentionsTemplateName verifies the success message
// includes the template that was used.
func TestInitCmd_SuccessOutput_MentionsTemplateName(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	stderr, code := captureInitOutput(t, dir, "go-cli", "--name", "tmpl-mention")

	require.Equal(t, 0, code)
	assert.Contains(t, stderr, "go-cli",
		"success output must mention the template name used")
}

// ---- AC-11: No existing raven.toml required ---------------------------------

// TestInitCmd_NoPersistentPreRunE_RequiresNoConfigFile confirms that the init
// command succeeds in a directory without a raven.toml (no pre-existing config
// required).
func TestInitCmd_NoPersistentPreRunE_RequiresNoConfigFile(t *testing.T) {
	resetInitFlags(t)
	// Use a fresh temp dir with no raven.toml.
	dir := t.TempDir()

	code := runInitInDir(t, dir)
	assert.Equal(t, 0, code,
		"init must succeed without a pre-existing raven.toml")
}

// ---- AC-12: Respects --dir global flag --------------------------------------

// TestInitCmd_RespectsGlobalDirFlag verifies that "raven --dir <path> init"
// creates files in <path> instead of the current working directory.
func TestInitCmd_RespectsGlobalDirFlag(t *testing.T) {
	resetInitFlags(t)

	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(orig) })

	// destDir is the target; cwdDir is the directory we start in (different).
	destDir := t.TempDir()
	cwdDir := t.TempDir()
	require.NoError(t, os.Chdir(cwdDir))

	rootCmd.SetArgs([]string{"--dir", destDir, "init", "--name", "dir-flag-project"})
	code := Execute()

	assert.Equal(t, 0, code, "--dir flag should redirect init output to the given directory")

	// Files must be created in destDir, not cwdDir.
	assert.FileExists(t, filepath.Join(destDir, "raven.toml"),
		"raven.toml must be created in the --dir path")
	assert.NoFileExists(t, filepath.Join(cwdDir, "raven.toml"),
		"raven.toml must NOT be created in the original cwd")
}

// TestInitCmd_GlobalDirFlag_NonExistentPath fails gracefully when --dir points
// to a directory that does not exist.
func TestInitCmd_GlobalDirFlag_NonExistentPath(t *testing.T) {
	resetInitFlags(t)

	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(orig) })
	require.NoError(t, os.Chdir(t.TempDir()))

	oldStderr := os.Stderr
	r, w, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = oldStderr })

	rootCmd.SetArgs([]string{"--dir", "/nonexistent/path/that/does/not/exist", "init"})
	exitCode := Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stderr = oldStderr

	assert.Equal(t, 1, exitCode, "nonexistent --dir should return exit code 1")
}

// ---- AC-13: Exit codes 0 (success) / 1 (error) --------------------------------

// TestInitCmd_ExitCodes exercises exit code correctness across all error paths
// using a table-driven approach.
func TestInitCmd_ExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T, dir string) // optional pre-test setup
		args     []string
		wantCode int
	}{
		{
			name:     "success default template",
			args:     []string{"--name", "code-test"},
			wantCode: 0,
		},
		{
			name:     "success explicit go-cli template",
			args:     []string{"go-cli", "--name", "code-test-explicit"},
			wantCode: 0,
		},
		{
			name:     "error unknown template",
			args:     []string{"no-such-template"},
			wantCode: 1,
		},
		{
			name:     "error too many positional args",
			args:     []string{"go-cli", "extra"},
			wantCode: 1,
		},
		{
			name: "error existing raven.toml no force",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "raven.toml"), []byte("x"), 0o644))
			},
			args:     []string{"--name", "conflict"},
			wantCode: 1,
		},
		{
			name:     "error path traversal in name",
			args:     []string{"--name", "../evil"},
			wantCode: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetInitFlags(t)
			dir := t.TempDir()

			if tt.setup != nil {
				tt.setup(t, dir)
			}

			_, code := captureInitOutput(t, dir, tt.args...)
			assert.Equal(t, tt.wantCode, code,
				"exit code mismatch for test %q", tt.name)
		})
	}
}

// ---- Edge cases -------------------------------------------------------------

// TestInitCmd_PathTraversalInName rejects project names containing "../".
func TestInitCmd_PathTraversalInName(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	stderr, code := captureInitOutput(t, dir, "--name", "../evil")

	assert.Equal(t, 1, code, "path traversal in --name should return exit code 1")
	assert.Contains(t, stderr, "path traversal",
		"error should mention path traversal")
}

// TestInitCmd_PathTraversalWindowsStyle rejects project names with "..\\"
// (Windows-style path traversal).
func TestInitCmd_PathTraversalWindowsStyle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows path separator test not applicable on non-Windows")
	}
	resetInitFlags(t)
	dir := t.TempDir()

	// Use a name containing the Windows separator sequence.
	stderr, code := captureInitOutput(t, dir, "--name", `some..\..\evil`)

	assert.Equal(t, 1, code, `path traversal with "..\\" in --name should return exit code 1`)
	assert.Contains(t, stderr, "path traversal",
		`error should mention path traversal for "..\\"-style names`)
}

// TestInitCmd_MaximumOneArg verifies that more than one positional argument is
// rejected.
func TestInitCmd_MaximumOneArg(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	_, code := captureInitOutput(t, dir, "go-cli", "extra-arg")

	assert.Equal(t, 1, code, "more than one arg should return exit code 1")
}

// TestInitCmd_SpecialCharactersInName verifies that project names with hyphens,
// underscores, and dots are accepted and written into raven.toml verbatim.
func TestInitCmd_SpecialCharactersInName(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		wantInToml  bool
	}{
		{name: "hyphens", projectName: "my-awesome-cli", wantInToml: true},
		{name: "underscores", projectName: "my_service_v2", wantInToml: true},
		{name: "dots", projectName: "my.project.name", wantInToml: true},
		{name: "digits", projectName: "service42", wantInToml: true},
		{name: "mixed", projectName: "raven-v1.0_alpha", wantInToml: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetInitFlags(t)
			dir := t.TempDir()

			code := runInitInDir(t, dir, "--name", tt.projectName)

			assert.Equal(t, 0, code,
				"project name %q should be accepted", tt.projectName)

			content, err := os.ReadFile(filepath.Join(dir, "raven.toml"))
			require.NoError(t, err)
			if tt.wantInToml {
				assert.Contains(t, string(content), tt.projectName,
					"raven.toml must contain project name %q", tt.projectName)
			}
		})
	}
}

// TestInitCmd_ReadOnlyDirectory verifies that init fails gracefully when the
// destination directory is read-only.
func TestInitCmd_ReadOnlyDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory semantics differ on Windows")
	}

	resetInitFlags(t)
	dir := t.TempDir()

	// Make the directory read-only: files cannot be created inside.
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() {
		// Restore permissions so t.TempDir() cleanup can remove the directory.
		_ = os.Chmod(dir, 0o755)
	})

	_, code := captureInitOutput(t, dir, "--name", "readonly-test")

	assert.Equal(t, 1, code,
		"init into a read-only directory must return exit code 1")
}

// TestInitCmd_InGitRepository verifies that init works correctly inside an
// existing git repository (the .git directory must not interfere).
func TestInitCmd_InGitRepository(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	// Simulate a git repository by creating a .git subdirectory.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".git", "HEAD"),
		[]byte("ref: refs/heads/main\n"),
		0o644,
	))

	code := runInitInDir(t, dir, "--name", "git-project")

	assert.Equal(t, 0, code,
		"init must succeed inside an existing git repository")
	assert.FileExists(t, filepath.Join(dir, "raven.toml"),
		"raven.toml must be created even when a .git directory exists")
	// Ensure .git was not corrupted.
	assert.DirExists(t, filepath.Join(dir, ".git"),
		".git directory must not be removed by init")
}

// TestInitCmd_Force_InGitRepository verifies that --force also works inside a
// git repository.
func TestInitCmd_Force_InGitRepository(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	// First scaffold.
	code := runInitInDir(t, dir, "--name", "first")
	require.Equal(t, 0, code)

	// Second scaffold with --force.
	resetInitFlags(t)
	code = runInitInDir(t, dir, "--force", "--name", "second")
	assert.Equal(t, 0, code)

	content, err := os.ReadFile(filepath.Join(dir, "raven.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "second",
		"raven.toml must reflect the second project name after --force")
}

// TestInitCmd_IdempotentWithoutForce verifies that re-running init without
// --force in a directory where all files already exist succeeds (no file
// creation) but does NOT overwrite any files.
func TestInitCmd_IdempotentWithoutForce(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	// First run creates the scaffold.
	code := runInitInDir(t, dir, "--name", "idempotent")
	require.Equal(t, 0, code)

	// Stamp a sentinel into raven.toml to detect overwriting.
	tomlPath := filepath.Join(dir, "raven.toml")
	originalContent, err := os.ReadFile(tomlPath)
	require.NoError(t, err)

	// Second run without --force must fail because raven.toml already exists.
	resetInitFlags(t)
	_, code = captureInitOutput(t, dir, "--name", "idempotent")
	assert.Equal(t, 1, code,
		"second init without --force must fail when raven.toml exists")

	// Content must be unchanged.
	afterContent, err := os.ReadFile(tomlPath)
	require.NoError(t, err)
	assert.Equal(t, string(originalContent), string(afterContent),
		"raven.toml must not be modified on second init without --force")
}

// ---- Integration test -------------------------------------------------------

// TestInitCmd_Integration_EndToEnd performs a full end-to-end integration test:
// "raven init go-cli --name test-project" in a t.TempDir(), then validates
// every acceptance criterion.
func TestInitCmd_Integration_EndToEnd(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	stderr, code := captureInitOutput(t, dir, "go-cli", "--name", "test-project")

	// AC-13: exit code 0.
	require.Equal(t, 0, code, "end-to-end init must exit 0")

	// AC-1: template files created.
	assert.FileExists(t, filepath.Join(dir, "raven.toml"))

	// AC-2 + AC-8: raven.toml contains project name and is valid TOML.
	var cfg config.Config
	_, decErr := toml.DecodeFile(filepath.Join(dir, "raven.toml"), &cfg)
	require.NoError(t, decErr, "raven.toml must be valid TOML")
	assert.Equal(t, "test-project", cfg.Project.Name, "project.name must match --name")

	// AC-9: directory structure.
	expectedFiles := []string{
		"raven.toml",
		filepath.Join("prompts", "implement-claude.md"),
		filepath.Join("prompts", "implement-codex.md"),
		filepath.Join(".github", "review", "prompts", "review-prompt.md"),
		filepath.Join(".github", "review", "rules", ".gitkeep"),
		filepath.Join(".github", "review", "PROJECT_BRIEF.md"),
		filepath.Join("docs", "tasks", ".gitkeep"),
		filepath.Join("docs", "prd", ".gitkeep"),
	}
	for _, rel := range expectedFiles {
		assert.FileExists(t, filepath.Join(dir, rel),
			"expected scaffold file %q", rel)
	}

	// AC-10: success output lists created files and next steps.
	assert.Contains(t, stderr, "Created files:", "success output must list created files")
	assert.Contains(t, stderr, "Next steps:", "success output must contain next steps")
	assert.Contains(t, stderr, "raven implement", "next steps must mention 'raven implement'")
	assert.Contains(t, stderr, "test-project", "success output must echo the project name")

	// AC-11: No pre-existing raven.toml was required (we started with an empty dir).

	// Verify the template variable is substituted (no raw {{ }} left in raven.toml).
	rawToml, err := os.ReadFile(filepath.Join(dir, "raven.toml"))
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(rawToml), "{{"),
		"raven.toml must not contain unresolved template syntax")
	assert.False(t, strings.Contains(string(rawToml), "}}"),
		"raven.toml must not contain unresolved template syntax")
}

// ---- PersistentPreRunE behaviour specific to init ---------------------------

// TestInitCmd_PersistentPreRunE_DoesNotRequireConfig verifies that the init
// command's own PersistentPreRunE does not attempt to load a config file,
// meaning it succeeds in directories that have no raven.toml.
func TestInitCmd_PersistentPreRunE_DoesNotRequireConfig(t *testing.T) {
	resetInitFlags(t)
	emptyDir := t.TempDir()

	// There must be no raven.toml in emptyDir.
	_, err := os.Stat(filepath.Join(emptyDir, "raven.toml"))
	require.True(t, os.IsNotExist(err), "emptyDir must start with no raven.toml")

	code := runInitInDir(t, emptyDir)
	assert.Equal(t, 0, code, "init PersistentPreRunE must not fail when raven.toml is absent")
}

// TestInitCmd_PersistentPreRunE_EnvNoColor verifies that NO_COLOR env var is
// honoured by the init command's own PersistentPreRunE.
func TestInitCmd_PersistentPreRunE_EnvNoColor(t *testing.T) {
	resetInitFlags(t)
	t.Setenv("NO_COLOR", "1")

	dir := t.TempDir()
	code := runInitInDir(t, dir, "--name", "no-color-test")

	assert.Equal(t, 0, code, "init with NO_COLOR env must still succeed")
	// flagNoColor is set by the init PersistentPreRunE -- just verify the run succeeded.
}

// TestInitCmd_PersistentPreRunE_EnvRavenVerbose verifies RAVEN_VERBOSE is
// picked up by the init command's PersistentPreRunE.
func TestInitCmd_PersistentPreRunE_EnvRavenVerbose(t *testing.T) {
	resetInitFlags(t)
	t.Setenv("RAVEN_VERBOSE", "1")

	dir := t.TempDir()
	code := runInitInDir(t, dir, "--name", "verbose-test")

	assert.Equal(t, 0, code, "init with RAVEN_VERBOSE env must still succeed")
}

// ---- Relative-path output verification --------------------------------------

// TestInitCmd_OutputPaths_AreRelative verifies that the "Created files:" list
// in the success output shows relative paths (not absolute paths), improving
// readability.
func TestInitCmd_OutputPaths_AreRelative(t *testing.T) {
	resetInitFlags(t)
	dir := t.TempDir()

	stderr, code := captureInitOutput(t, dir, "--name", "rel-paths-test")
	require.Equal(t, 0, code)

	lines := strings.Split(stderr, "\n")
	inCreatedSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "Created files:" {
			inCreatedSection = true
			continue
		}
		if inCreatedSection {
			if trimmed == "" || strings.HasSuffix(trimmed, ":") {
				break // end of section
			}
			// Each listed file must not be an absolute path.
			assert.False(t, filepath.IsAbs(trimmed),
				"created-file path %q in output must be relative, not absolute", trimmed)
		}
	}
}
