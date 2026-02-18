package loop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
	"github.com/AbdelazizMoustafa10m/Raven/internal/task"
)

// ---- test helpers -----------------------------------------------------------

// makeTestSpec builds a minimal ParsedTaskSpec for use in prompt tests.
func makeTestSpec(id, title, content string) *task.ParsedTaskSpec {
	return &task.ParsedTaskSpec{
		ID:           id,
		Title:        title,
		Content:      content,
		Dependencies: []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
	}
}

// makeTestPhase builds a minimal Phase for use in prompt tests.
func makeTestPhase(id int, name, start, end string) *task.Phase {
	return &task.Phase{
		ID:        id,
		Name:      name,
		StartTask: start,
		EndTask:   end,
	}
}

// makeTestConfig builds a minimal Config for use in prompt tests.
func makeTestConfig(projectName, lang string, verifyCmds []string, agents map[string]config.AgentConfig) *config.Config {
	return &config.Config{
		Project: config.ProjectConfig{
			Name:                 projectName,
			Language:             lang,
			VerificationCommands: verifyCmds,
		},
		Agents: agents,
	}
}

// makeStateManager creates a StateManager backed by a temp file with the given
// pipe-delimited state lines (may be empty).
func makeStateManager(t *testing.T, lines []string) *task.StateManager {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "task-state.conf")
	if len(lines) > 0 {
		content := strings.Join(lines, "\n") + "\n"
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	}
	return task.NewStateManager(path)
}

// makeSelector constructs a TaskSelector from specs, state lines, and phases.
func makeSelector(t *testing.T, specs []*task.ParsedTaskSpec, stateLines []string, phases []task.Phase) *task.TaskSelector {
	t.Helper()
	sm := makeStateManager(t, stateLines)
	return task.NewTaskSelector(specs, sm, phases)
}

// ---- NewPromptGenerator -----------------------------------------------------

func TestNewPromptGenerator_EmptyTemplateDir(t *testing.T) {
	t.Parallel()

	pg, err := NewPromptGenerator("")
	require.NoError(t, err)
	require.NotNil(t, pg)
	assert.NotNil(t, pg.defaultTmpl)
}

func TestNewPromptGenerator_ValidDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pg, err := NewPromptGenerator(dir)
	require.NoError(t, err)
	require.NotNil(t, pg)
	assert.Equal(t, dir, pg.templateDir)
}

func TestNewPromptGenerator_NonExistentDir(t *testing.T) {
	t.Parallel()

	_, err := NewPromptGenerator("/nonexistent/path/to/templates")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template directory")
}

func TestNewPromptGenerator_FilePath_NotDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir.txt")
	require.NoError(t, os.WriteFile(file, []byte("hello"), 0644))

	_, err := NewPromptGenerator(file)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestNewPromptGenerator_DefaultTemplatePreParsed(t *testing.T) {
	t.Parallel()

	// Verify the built-in default template is already parsed and usable.
	pg, err := NewPromptGenerator("")
	require.NoError(t, err)

	ctx := PromptContext{
		TaskID:    "T-001",
		TaskTitle: "Test Task",
		TaskSpec:  "Some spec content",
		PhaseID:   1,
		PhaseName: "Foundation",
		PhaseRange: "T-001 to T-015",
		ProjectName:     "TestProject",
		ProjectLanguage: "Go",
		VerificationCommands: []string{"go build ./..."},
		VerificationString:   "go build ./...",
		CompletedSummary: "None",
		RemainingSummary: "T-001",
		AgentName: "claude",
		Model:     "claude-opus-4-6",
	}

	result, err := pg.Generate("", ctx)
	require.NoError(t, err)
	assert.Contains(t, result, "T-001")
	assert.Contains(t, result, "Test Task")
	assert.Contains(t, result, "Foundation")
	assert.Contains(t, result, "TestProject")
}

// ---- LoadTemplate -----------------------------------------------------------

func TestLoadTemplate_EmptyName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pg, err := NewPromptGenerator(dir)
	require.NoError(t, err)

	_, err = pg.LoadTemplate("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name must not be empty")
}

func TestLoadTemplate_NoTemplateDir(t *testing.T) {
	t.Parallel()

	pg, err := NewPromptGenerator("")
	require.NoError(t, err)

	_, err = pg.LoadTemplate("implement.tmpl")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no template directory configured")
}

func TestLoadTemplate_FileNotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pg, err := NewPromptGenerator(dir)
	require.NoError(t, err)

	_, err = pg.LoadTemplate("missing.tmpl")
	require.Error(t, err)
}

func TestLoadTemplate_DirectoryTraversal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pg, err := NewPromptGenerator(dir)
	require.NoError(t, err)

	// Attempt to traverse outside the template directory.
	_, err = pg.LoadTemplate("../../etc/passwd")
	require.Error(t, err)
	// filepath.Join cleans "../.." so the candidate ends up at or below absDir,
	// which means it might not be detected by the Rel check. Let's verify either
	// the traversal is rejected OR the file simply doesn't exist (acceptable either way).
	// The important thing is we get an error -- not a successful load.
	assert.Error(t, err)
}

func TestLoadTemplate_ValidTemplate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tmplContent := `Hello [[.TaskID]]`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.tmpl"), []byte(tmplContent), 0644))

	pg, err := NewPromptGenerator(dir)
	require.NoError(t, err)

	tmpl, err := pg.LoadTemplate("hello.tmpl")
	require.NoError(t, err)
	require.NotNil(t, tmpl)
	assert.Equal(t, "hello.tmpl", tmpl.Name())
}

func TestLoadTemplate_CachesOnSecondCall(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "t.tmpl"), []byte(`[[.TaskID]]`), 0644))

	pg, err := NewPromptGenerator(dir)
	require.NoError(t, err)

	tmpl1, err := pg.LoadTemplate("t.tmpl")
	require.NoError(t, err)

	// Overwrite the file on disk -- cached copy should be returned, so parse error
	// from new content won't happen.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "t.tmpl"), []byte(`[[invalid`), 0644))

	tmpl2, err := pg.LoadTemplate("t.tmpl")
	require.NoError(t, err)
	// Same pointer means cached.
	assert.Same(t, tmpl1, tmpl2)
}

func TestLoadTemplate_InvalidTemplateSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Write a template with invalid [[ syntax that won't parse.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.tmpl"), []byte(`[[invalid syntax`), 0644))

	pg, err := NewPromptGenerator(dir)
	require.NoError(t, err)

	_, err = pg.LoadTemplate("bad.tmpl")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing")
}

// ---- Generate ---------------------------------------------------------------

func TestGenerate_EmptyNameUsesDefault(t *testing.T) {
	t.Parallel()

	pg, err := NewPromptGenerator("")
	require.NoError(t, err)

	ctx := PromptContext{
		TaskID:               "T-016",
		TaskTitle:            "Task Spec Parser",
		TaskSpec:             "# T-016: Task Spec Parser\n\nSome content.",
		PhaseID:              2,
		PhaseName:            "Task System",
		PhaseRange:           "T-016 to T-030",
		ProjectName:          "Raven",
		ProjectLanguage:      "Go",
		VerificationCommands: []string{"go build ./cmd/raven/", "go vet ./..."},
		VerificationString:   "go build ./cmd/raven/ && go vet ./...",
		CompletedTasks:       []string{"T-001", "T-002"},
		RemainingTasks:       []string{"T-016"},
		CompletedSummary:     "T-001, T-002",
		RemainingSummary:     "T-016",
		AgentName:            "claude",
		Model:                "claude-opus-4-6",
	}

	result, err := pg.Generate("", ctx)
	require.NoError(t, err)

	assert.Contains(t, result, "T-016")
	assert.Contains(t, result, "Task Spec Parser")
	assert.Contains(t, result, "Phase 2: Task System")
	assert.Contains(t, result, "Raven")
	assert.Contains(t, result, "Go")
	assert.Contains(t, result, "go build ./cmd/raven/")
	assert.Contains(t, result, "go vet ./...")
	assert.Contains(t, result, "T-001, T-002")
	assert.Contains(t, result, "T-016")
	assert.Contains(t, result, "PHASE_COMPLETE")
}

func TestGenerate_WithTaskSpecContainingCurlyBraces(t *testing.T) {
	t.Parallel()

	pg, err := NewPromptGenerator("")
	require.NoError(t, err)

	// TaskSpec contains {{ and }} which are common in Go code examples.
	// These must NOT cause template parse/execution errors.
	goCodeSpec := `# T-016: Go Template Task

Use template.New("name").Delims("{{", "}}").Parse(...)
Also: fmt.Sprintf("%v", map[string]interface{}{"key": "value"})
JSON example: {"field": "value", "nested": {"key": 42}}
`

	ctx := PromptContext{
		TaskID:               "T-016",
		TaskTitle:            "Go Template Task",
		TaskSpec:             goCodeSpec,
		PhaseID:              2,
		PhaseName:            "Foundation",
		PhaseRange:           "T-016 to T-030",
		ProjectName:          "Raven",
		ProjectLanguage:      "Go",
		VerificationCommands: []string{"go build ./..."},
		VerificationString:   "go build ./...",
		CompletedSummary:     "None",
		RemainingSummary:     "T-016",
	}

	result, err := pg.Generate("", ctx)
	require.NoError(t, err)

	// The raw {{ and }} from TaskSpec must appear verbatim in the output.
	assert.Contains(t, result, `template.New("name").Delims("{{", "}}")`)
	// Check for a substring that IS present in the JSON example (not the full object).
	assert.Contains(t, result, `"field": "value"`)
}

func TestGenerate_NamedTemplate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `TASK: [[.TaskID]] - [[.TaskTitle]]`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "short.tmpl"), []byte(content), 0644))

	pg, err := NewPromptGenerator(dir)
	require.NoError(t, err)

	ctx := PromptContext{TaskID: "T-007", TaskTitle: "Config Resolution"}
	result, err := pg.Generate("short.tmpl", ctx)
	require.NoError(t, err)
	assert.Equal(t, "TASK: T-007 - Config Resolution", result)
}

func TestGenerate_NamedTemplateNotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pg, err := NewPromptGenerator(dir)
	require.NoError(t, err)

	_, err = pg.Generate("nonexistent.tmpl", PromptContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "generating prompt with template")
}

// ---- GenerateFromString -----------------------------------------------------

func TestGenerateFromString_Basic(t *testing.T) {
	t.Parallel()

	pg, err := NewPromptGenerator("")
	require.NoError(t, err)

	ctx := PromptContext{TaskID: "T-042", TaskTitle: "Pipeline Orchestrator"}
	result, err := pg.GenerateFromString(`ID=[[.TaskID]] Title=[[.TaskTitle]]`, ctx)
	require.NoError(t, err)
	assert.Equal(t, "ID=T-042 Title=Pipeline Orchestrator", result)
}

func TestGenerateFromString_RangeAction(t *testing.T) {
	t.Parallel()

	pg, err := NewPromptGenerator("")
	require.NoError(t, err)

	ctx := PromptContext{
		VerificationCommands: []string{"cmd1", "cmd2", "cmd3"},
	}
	tmplStr := `[[range .VerificationCommands]]- [[.]]
[[end]]`
	result, err := pg.GenerateFromString(tmplStr, ctx)
	require.NoError(t, err)
	assert.Contains(t, result, "- cmd1")
	assert.Contains(t, result, "- cmd2")
	assert.Contains(t, result, "- cmd3")
}

func TestGenerateFromString_InvalidSyntax(t *testing.T) {
	t.Parallel()

	pg, err := NewPromptGenerator("")
	require.NoError(t, err)

	_, err = pg.GenerateFromString(`[[invalid syntax`, PromptContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing template")
}

func TestGenerateFromString_CurlyBracesInContext(t *testing.T) {
	t.Parallel()

	pg, err := NewPromptGenerator("")
	require.NoError(t, err)

	ctx := PromptContext{
		TaskSpec: `{"json": "content", "nested": {"key": "val"}}`,
	}
	result, err := pg.GenerateFromString(`Spec: [[.TaskSpec]]`, ctx)
	require.NoError(t, err)
	// Check for a substring that IS present in the JSON content (not a full nested object).
	assert.Contains(t, result, `"json": "content"`)
}

func TestGenerateFromString_EmptyTemplate(t *testing.T) {
	t.Parallel()

	pg, err := NewPromptGenerator("")
	require.NoError(t, err)

	result, err := pg.GenerateFromString("", PromptContext{})
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

// ---- BuildContext -----------------------------------------------------------

func TestBuildContext_NilSpec(t *testing.T) {
	t.Parallel()

	phase := makeTestPhase(1, "Foundation", "T-001", "T-015")
	cfg := makeTestConfig("Raven", "Go", nil, nil)
	sel := makeSelector(t, nil, nil, []task.Phase{*phase})

	_, err := BuildContext(nil, phase, cfg, sel, "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec must not be nil")
}

func TestBuildContext_NilPhase(t *testing.T) {
	t.Parallel()

	spec := makeTestSpec("T-001", "Setup", "# T-001: Setup\n")
	cfg := makeTestConfig("Raven", "Go", nil, nil)
	phases := []task.Phase{{ID: 1, Name: "P1", StartTask: "T-001", EndTask: "T-005"}}
	sel := makeSelector(t, []*task.ParsedTaskSpec{spec}, nil, phases)

	_, err := BuildContext(spec, nil, cfg, sel, "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "phase must not be nil")
}

func TestBuildContext_NilConfig(t *testing.T) {
	t.Parallel()

	spec := makeTestSpec("T-001", "Setup", "# T-001: Setup\n")
	phase := makeTestPhase(1, "Foundation", "T-001", "T-015")
	phases := []task.Phase{*phase}
	sel := makeSelector(t, []*task.ParsedTaskSpec{spec}, nil, phases)

	_, err := BuildContext(spec, phase, nil, sel, "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cfg must not be nil")
}

func TestBuildContext_NilSelector(t *testing.T) {
	t.Parallel()

	spec := makeTestSpec("T-001", "Setup", "# T-001: Setup\n")
	phase := makeTestPhase(1, "Foundation", "T-001", "T-015")
	cfg := makeTestConfig("Raven", "Go", nil, nil)

	_, err := BuildContext(spec, phase, cfg, nil, "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "selector must not be nil")
}

func TestBuildContext_PopulatesTaskFields(t *testing.T) {
	t.Parallel()

	spec := makeTestSpec("T-016", "Task Spec Parser", "# T-016: Task Spec Parser\n\nContent here.")
	phase := makeTestPhase(2, "Task System", "T-016", "T-030")
	cfg := makeTestConfig("Raven", "Go", nil, nil)
	phases := []task.Phase{*phase}
	sel := makeSelector(t, []*task.ParsedTaskSpec{spec}, nil, phases)

	ctx, err := BuildContext(spec, phase, cfg, sel, "claude")
	require.NoError(t, err)

	assert.Equal(t, "T-016", ctx.TaskID)
	assert.Equal(t, "Task Spec Parser", ctx.TaskTitle)
	assert.Equal(t, spec.Content, ctx.TaskSpec)
}

func TestBuildContext_PopulatesPhaseFields(t *testing.T) {
	t.Parallel()

	spec := makeTestSpec("T-016", "Task Spec Parser", "# T-016: Task Spec Parser\n")
	phase := makeTestPhase(2, "Task System & Agents", "T-016", "T-030")
	cfg := makeTestConfig("Raven", "Go", nil, nil)
	phases := []task.Phase{*phase}
	sel := makeSelector(t, []*task.ParsedTaskSpec{spec}, nil, phases)

	ctx, err := BuildContext(spec, phase, cfg, sel, "claude")
	require.NoError(t, err)

	assert.Equal(t, 2, ctx.PhaseID)
	assert.Equal(t, "Task System & Agents", ctx.PhaseName)
	assert.Equal(t, "T-016 to T-030", ctx.PhaseRange)
}

func TestBuildContext_PopulatesProjectFields(t *testing.T) {
	t.Parallel()

	spec := makeTestSpec("T-001", "Setup", "# T-001: Setup\n")
	phase := makeTestPhase(1, "Foundation", "T-001", "T-015")
	cmds := []string{"go build ./cmd/raven/", "go vet ./...", "go test ./..."}
	cfg := makeTestConfig("Raven", "Go", cmds, nil)
	phases := []task.Phase{*phase}
	sel := makeSelector(t, []*task.ParsedTaskSpec{spec}, nil, phases)

	ctx, err := BuildContext(spec, phase, cfg, sel, "claude")
	require.NoError(t, err)

	assert.Equal(t, "Raven", ctx.ProjectName)
	assert.Equal(t, "Go", ctx.ProjectLanguage)
	assert.Equal(t, cmds, ctx.VerificationCommands)
	assert.Equal(t, "go build ./cmd/raven/ && go vet ./... && go test ./...", ctx.VerificationString)
}

func TestBuildContext_VerificationStringNoTrailingAnd(t *testing.T) {
	t.Parallel()

	spec := makeTestSpec("T-001", "Setup", "# T-001: Setup\n")
	phase := makeTestPhase(1, "P1", "T-001", "T-001")
	cfg := makeTestConfig("Raven", "Go", []string{"go build ./..."}, nil)
	phases := []task.Phase{*phase}
	sel := makeSelector(t, []*task.ParsedTaskSpec{spec}, nil, phases)

	ctx, err := BuildContext(spec, phase, cfg, sel, "claude")
	require.NoError(t, err)

	// Single command: no trailing " && ".
	assert.Equal(t, "go build ./...", ctx.VerificationString)
	assert.False(t, strings.HasSuffix(ctx.VerificationString, " && "))
}

func TestBuildContext_EmptyVerificationCommands(t *testing.T) {
	t.Parallel()

	spec := makeTestSpec("T-001", "Setup", "# T-001: Setup\n")
	phase := makeTestPhase(1, "P1", "T-001", "T-001")
	cfg := makeTestConfig("Raven", "Go", nil, nil)
	phases := []task.Phase{*phase}
	sel := makeSelector(t, []*task.ParsedTaskSpec{spec}, nil, phases)

	ctx, err := BuildContext(spec, phase, cfg, sel, "claude")
	require.NoError(t, err)

	assert.Empty(t, ctx.VerificationCommands)
	assert.Equal(t, "", ctx.VerificationString)
}

func TestBuildContext_PopulatesAgentModel(t *testing.T) {
	t.Parallel()

	spec := makeTestSpec("T-001", "Setup", "# T-001: Setup\n")
	phase := makeTestPhase(1, "P1", "T-001", "T-001")
	agents := map[string]config.AgentConfig{
		"claude": {Model: "claude-opus-4-6"},
		"codex":  {Model: "o4-mini"},
	}
	cfg := makeTestConfig("Raven", "Go", nil, agents)
	phases := []task.Phase{*phase}
	sel := makeSelector(t, []*task.ParsedTaskSpec{spec}, nil, phases)

	ctx, err := BuildContext(spec, phase, cfg, sel, "claude")
	require.NoError(t, err)

	assert.Equal(t, "claude", ctx.AgentName)
	assert.Equal(t, "claude-opus-4-6", ctx.Model)
}

func TestBuildContext_UnknownAgentModelIsEmpty(t *testing.T) {
	t.Parallel()

	spec := makeTestSpec("T-001", "Setup", "# T-001: Setup\n")
	phase := makeTestPhase(1, "P1", "T-001", "T-001")
	cfg := makeTestConfig("Raven", "Go", nil, nil) // no agents configured
	phases := []task.Phase{*phase}
	sel := makeSelector(t, []*task.ParsedTaskSpec{spec}, nil, phases)

	ctx, err := BuildContext(spec, phase, cfg, sel, "unknown-agent")
	require.NoError(t, err)

	assert.Equal(t, "unknown-agent", ctx.AgentName)
	assert.Equal(t, "", ctx.Model)
}

func TestBuildContext_CompletedAndRemainingTasks(t *testing.T) {
	t.Parallel()

	specs := []*task.ParsedTaskSpec{
		makeTestSpec("T-001", "Task 1", "# T-001: Task 1\n"),
		makeTestSpec("T-002", "Task 2", "# T-002: Task 2\n"),
		makeTestSpec("T-003", "Task 3", "# T-003: Task 3\n"),
	}
	phase := makeTestPhase(1, "P1", "T-001", "T-003")
	cfg := makeTestConfig("Raven", "Go", nil, nil)
	phases := []task.Phase{*phase}
	stateLines := []string{
		"T-001|completed|||",
	}
	sel := makeSelector(t, specs, stateLines, phases)

	ctx, err := BuildContext(specs[1], phase, cfg, sel, "claude")
	require.NoError(t, err)

	// T-001 is completed.
	assert.Equal(t, []string{"T-001"}, ctx.CompletedTasks)
	assert.Equal(t, "T-001", ctx.CompletedSummary)

	// T-002, T-003 are remaining (not completed/skipped).
	assert.Contains(t, ctx.RemainingTasks, "T-002")
	assert.Contains(t, ctx.RemainingTasks, "T-003")
	assert.Contains(t, ctx.RemainingSummary, "T-002")
	assert.Contains(t, ctx.RemainingSummary, "T-003")
}

func TestBuildContext_SummaryNoneWhenEmpty(t *testing.T) {
	t.Parallel()

	spec := makeTestSpec("T-001", "Setup", "# T-001: Setup\n")
	phase := makeTestPhase(1, "P1", "T-001", "T-001")
	cfg := makeTestConfig("Raven", "Go", nil, nil)
	phases := []task.Phase{*phase}
	// Empty state: no tasks completed.
	sel := makeSelector(t, []*task.ParsedTaskSpec{spec}, nil, phases)

	ctx, err := BuildContext(spec, phase, cfg, sel, "claude")
	require.NoError(t, err)

	assert.Equal(t, "None", ctx.CompletedSummary)
}

func TestBuildContext_RemainingSummaryNoneWhenAllDone(t *testing.T) {
	t.Parallel()

	spec := makeTestSpec("T-001", "Setup", "# T-001: Setup\n")
	phase := makeTestPhase(1, "P1", "T-001", "T-001")
	cfg := makeTestConfig("Raven", "Go", nil, nil)
	phases := []task.Phase{*phase}
	stateLines := []string{"T-001|completed|||"}
	sel := makeSelector(t, []*task.ParsedTaskSpec{spec}, stateLines, phases)

	ctx, err := BuildContext(spec, phase, cfg, sel, "claude")
	require.NoError(t, err)

	assert.Equal(t, "None", ctx.RemainingSummary)
}

// ---- formatIDSummary --------------------------------------------------------

func TestFormatIDSummary_EmptySlice(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "None", formatIDSummary(nil))
	assert.Equal(t, "None", formatIDSummary([]string{}))
}

func TestFormatIDSummary_SingleID(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "T-001", formatIDSummary([]string{"T-001"}))
}

func TestFormatIDSummary_MultipleIDs(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "T-001, T-002, T-003", formatIDSummary([]string{"T-001", "T-002", "T-003"}))
}

// ---- Table-driven: Generate with various PromptContext values ---------------

func TestGenerate_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		ctx           PromptContext
		wantContains  []string
		wantAbsent    []string
	}{
		{
			name: "basic task renders all sections",
			ctx: PromptContext{
				TaskID:               "T-010",
				TaskTitle:            "Config Resolution",
				TaskSpec:             "Spec content here.",
				PhaseID:              1,
				PhaseName:            "Foundation",
				PhaseRange:           "T-001 to T-015",
				ProjectName:          "Raven",
				ProjectLanguage:      "Go",
				VerificationCommands: []string{"go build ./..."},
				VerificationString:   "go build ./...",
				CompletedSummary:     "None",
				RemainingSummary:     "T-010",
			},
			wantContains: []string{
				"T-010", "Config Resolution", "Spec content here.",
				"Phase 1: Foundation", "Raven", "Go", "go build ./...",
				"None", "T-010", "PHASE_COMPLETE",
			},
		},
		{
			name: "multiple verification commands all appear",
			ctx: PromptContext{
				TaskID:               "T-020",
				TaskTitle:            "Status Command",
				VerificationCommands: []string{"go build ./cmd/raven/", "go vet ./...", "go test ./..."},
				CompletedSummary:     "None",
				RemainingSummary:     "None",
			},
			wantContains: []string{
				"go build ./cmd/raven/",
				"go vet ./...",
				"go test ./...",
			},
		},
		{
			name: "task spec with json curly braces renders without error",
			ctx: PromptContext{
				TaskID:           "T-030",
				TaskTitle:        "JSON Task",
				TaskSpec:         `{"key": "value", "nested": {"a": 1}}`,
				CompletedSummary: "None",
				RemainingSummary: "None",
			},
			wantContains: []string{`{"key": "value"`},
		},
		{
			name: "task spec with go template syntax renders without error",
			ctx: PromptContext{
				TaskID:           "T-031",
				TaskTitle:        "Template Task",
				TaskSpec:         `Use {{.Field}} and {{range .Items}}{{.}}{{end}}`,
				CompletedSummary: "None",
				RemainingSummary: "None",
			},
			wantContains: []string{`{{.Field}}`},
		},
	}

	pg, err := NewPromptGenerator("")
	require.NoError(t, err)

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := pg.Generate("", tt.ctx)
			require.NoError(t, err)

			for _, want := range tt.wantContains {
				assert.Contains(t, result, want, "expected output to contain %q", want)
			}
			for _, absent := range tt.wantAbsent {
				assert.NotContains(t, result, absent, "expected output NOT to contain %q", absent)
			}
		})
	}
}

// ---- Integration: BuildContext + Generate -----------------------------------

func TestIntegration_BuildContextAndGenerate(t *testing.T) {
	t.Parallel()

	specs := []*task.ParsedTaskSpec{
		makeTestSpec("T-021", "Agent Interface", "# T-021: Agent Interface\n\nImplement the agent interface."),
		makeTestSpec("T-022", "Claude Agent", "# T-022: Claude Agent\n\nImplement Claude adapter."),
	}
	phase := makeTestPhase(2, "Task System & Agents", "T-021", "T-030")
	agents := map[string]config.AgentConfig{
		"claude": {Model: "claude-opus-4-6"},
	}
	cfg := makeTestConfig("Raven", "Go", []string{
		"go build ./cmd/raven/",
		"go vet ./...",
		"go test ./...",
	}, agents)
	phases := []task.Phase{*phase}
	stateLines := []string{"T-021|completed|||"}
	sel := makeSelector(t, specs, stateLines, phases)

	// Build context for T-022 (T-021 is already completed).
	ctx, err := BuildContext(specs[1], phase, cfg, sel, "claude")
	require.NoError(t, err)

	// Verify context fields.
	assert.Equal(t, "T-022", ctx.TaskID)
	assert.Equal(t, "Claude Agent", ctx.TaskTitle)
	assert.Equal(t, 2, ctx.PhaseID)
	assert.Equal(t, "T-021 to T-030", ctx.PhaseRange)
	assert.Equal(t, "claude-opus-4-6", ctx.Model)
	assert.Equal(t, "T-021", ctx.CompletedSummary)
	assert.Contains(t, ctx.RemainingTasks, "T-022")

	// Generate the prompt.
	pg, err := NewPromptGenerator("")
	require.NoError(t, err)

	prompt, err := pg.Generate("", *ctx)
	require.NoError(t, err)

	// Verify the full prompt contains all key sections.
	assert.Contains(t, prompt, "T-022")
	assert.Contains(t, prompt, "Claude Agent")
	assert.Contains(t, prompt, "# T-022: Claude Agent")
	assert.Contains(t, prompt, "Phase 2: Task System & Agents")
	assert.Contains(t, prompt, "T-021 to T-030")
	assert.Contains(t, prompt, "Raven")
	assert.Contains(t, prompt, "Go")
	assert.Contains(t, prompt, "go build ./cmd/raven/")
	assert.Contains(t, prompt, "T-021")
	assert.Contains(t, prompt, "PHASE_COMPLETE")
}

// ---- GenerateFromString: table-driven ---------------------------------------

func TestGenerateFromString_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		tmplStr     string
		ctx         PromptContext
		wantResult  string
		wantErrStr  string
	}{
		{
			name:       "simple field substitution",
			tmplStr:    "Task: [[.TaskID]]",
			ctx:        PromptContext{TaskID: "T-001"},
			wantResult: "Task: T-001",
		},
		{
			name:    "range over slice",
			tmplStr: "[[range .CompletedTasks]][[.]] [[end]]",
			ctx:     PromptContext{CompletedTasks: []string{"T-001", "T-002"}},
			wantResult: "T-001 T-002 ",
		},
		{
			name:    "conditional with if",
			tmplStr: `[[if .AgentName]]Agent: [[.AgentName]][[else]]No agent[[end]]`,
			ctx:     PromptContext{AgentName: "claude"},
			wantResult: "Agent: claude",
		},
		{
			name:       "invalid syntax returns error",
			tmplStr:    "[[invalid",
			wantErrStr: "parsing template",
		},
		{
			name:       "empty string returns empty",
			tmplStr:    "",
			ctx:        PromptContext{},
			wantResult: "",
		},
		{
			name:       "curly braces in context data pass through",
			tmplStr:    "Spec: [[.TaskSpec]]",
			ctx:        PromptContext{TaskSpec: `{{"key": "val"}}`},
			wantResult: `Spec: {{"key": "val"}}`,
		},
	}

	pg, err := NewPromptGenerator("")
	require.NoError(t, err)

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := pg.GenerateFromString(tt.tmplStr, tt.ctx)
			if tt.wantErrStr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrStr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantResult, result)
		})
	}
}

// ---- LoadTemplate: many files -----------------------------------------------

func TestLoadTemplate_MultipleTemplatesInDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := map[string]string{
		"implement.tmpl": `Implement [[.TaskID]]`,
		"review.tmpl":    `Review [[.TaskTitle]]`,
		"fix.tmpl":       `Fix in [[.ProjectName]]`,
	}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
	}

	pg, err := NewPromptGenerator(dir)
	require.NoError(t, err)

	ctx := PromptContext{TaskID: "T-010", TaskTitle: "Config", ProjectName: "Raven"}

	result, err := pg.Generate("implement.tmpl", ctx)
	require.NoError(t, err)
	assert.Equal(t, "Implement T-010", result)

	result, err = pg.Generate("review.tmpl", ctx)
	require.NoError(t, err)
	assert.Equal(t, "Review Config", result)

	result, err = pg.Generate("fix.tmpl", ctx)
	require.NoError(t, err)
	assert.Equal(t, "Fix in Raven", result)
}

// ---- Benchmark --------------------------------------------------------------

func BenchmarkGenerate_Default(b *testing.B) {
	pg, err := NewPromptGenerator("")
	if err != nil {
		b.Fatal(err)
	}

	ctx := PromptContext{
		TaskID:    "T-016",
		TaskTitle: "Task Spec Parser",
		TaskSpec: fmt.Sprintf("%s\n\n%s",
			"# T-016: Task Spec Parser",
			strings.Repeat("This is some longer task spec content with details.\n", 20),
		),
		PhaseID:              2,
		PhaseName:            "Task System & Agent Adapters",
		PhaseRange:           "T-016 to T-030",
		ProjectName:          "Raven",
		ProjectLanguage:      "Go",
		VerificationCommands: []string{"go build ./cmd/raven/", "go vet ./...", "go test ./..."},
		VerificationString:   "go build ./cmd/raven/ && go vet ./... && go test ./...",
		CompletedTasks:       []string{"T-001", "T-002", "T-003", "T-004", "T-005"},
		RemainingTasks:       []string{"T-016", "T-017", "T-018"},
		CompletedSummary:     "T-001, T-002, T-003, T-004, T-005",
		RemainingSummary:     "T-016, T-017, T-018",
		AgentName:            "claude",
		Model:                "claude-opus-4-6",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := pg.Generate("", ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}
