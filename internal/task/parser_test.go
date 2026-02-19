package task

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ----------------------------------------------------------------

// writeFile writes content to a file inside dir and returns the full path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// makeFullSpec builds a well-formed task spec string for testing.
func makeFullSpec(t *testing.T, id, title, priority, effort, deps, blockedBy, blocks string) string {
	t.Helper()
	return fmt.Sprintf(`# %s: %s

## Metadata
| Field | Value |
|-------|-------|
| Priority | %s |
| Estimated Effort | %s |
| Dependencies | %s |
| Blocked By | %s |
| Blocks | %s |

## Goal
This is the task goal section.
`, id, title, priority, effort, deps, blockedBy, blocks)
}

// --- ParseTaskSpec ----------------------------------------------------------

func TestParseTaskSpec_ValidFull(t *testing.T) {
	t.Parallel()

	content := `# T-016: Task Spec Markdown Parser

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-004 |
| Blocked By | T-004 |
| Blocks | T-017, T-019, T-020 |

## Goal
Implement a markdown parser.
`
	spec, err := ParseTaskSpec(content)
	require.NoError(t, err)
	assert.Equal(t, "T-016", spec.ID)
	assert.Equal(t, "Task Spec Markdown Parser", spec.Title)
	assert.Equal(t, "Must Have", spec.Priority)
	assert.Equal(t, "Medium: 6-10hrs", spec.Effort)
	assert.Equal(t, []string{"T-004"}, spec.Dependencies)
	assert.Equal(t, []string{"T-004"}, spec.BlockedBy)
	assert.Equal(t, []string{"T-017", "T-019", "T-020"}, spec.Blocks)
	assert.Equal(t, content, spec.Content)
}

func TestParseTaskSpec_NoDependencies(t *testing.T) {
	t.Parallel()

	content := `# T-001: Go Project Init

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 4-8hrs |
| Dependencies | None |
| Blocked By | None |
| Blocks | T-002, T-003 |
`
	spec, err := ParseTaskSpec(content)
	require.NoError(t, err)
	assert.Equal(t, "T-001", spec.ID)
	assert.Equal(t, []string{}, spec.Dependencies)
	assert.Equal(t, []string{}, spec.BlockedBy)
	assert.Equal(t, []string{"T-002", "T-003"}, spec.Blocks)
}

func TestParseTaskSpec_MultipleDependencies(t *testing.T) {
	t.Parallel()

	content := `# T-019: Dependency Resolution

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-001, T-003, T-005 |
| Blocked By | T-001, T-003, T-005 |
| Blocks | T-027 |
`
	spec, err := ParseTaskSpec(content)
	require.NoError(t, err)
	assert.Equal(t, "T-019", spec.ID)
	assert.Equal(t, []string{"T-001", "T-003", "T-005"}, spec.Dependencies)
}

// TestParseTaskSpec_HeadingFormats verifies which title line formats are
// accepted and which are rejected. The parser only supports the colon
// separator ("# T-001: Title"); the dash form ("# T-001 - Title") is not
// part of the current specification and must return an error.
func TestParseTaskSpec_HeadingFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		wantID  string
		wantErr bool
	}{
		{
			name:    "colon separator (canonical form)",
			content: "# T-001: Setup\n",
			wantID:  "T-001",
		},
		{
			name:    "colon with extra spaces around title",
			content: "#  T-002:   Config Loading  \n",
			wantID:  "T-002",
		},
		{
			name:    "dash separator",
			content: "# T-001 - Setup\n",
			wantID:  "T-001",
		},
		{
			name:    "missing colon entirely",
			content: "# T-001 Setup\n",
			wantErr: true,
		},
		{
			name:    "two-digit ID rejected",
			content: "# T-01: Too Short\n",
			wantErr: true,
		},
		{
			name:    "four-digit ID rejected",
			content: "# T-0001: Too Long\n",
			wantErr: true,
		},
		{
			name:    "missing hash prefix rejected",
			content: "T-001: No Hash\n",
			wantErr: true,
		},
		{
			name:    "h2 heading rejected",
			content: "## T-001: Wrong Level\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec, err := ParseTaskSpec(tt.content)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "no valid task heading found")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, spec.ID)
		})
	}
}

func TestParseTaskSpec_MissingTitleLine(t *testing.T) {
	t.Parallel()

	content := `## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
`
	_, err := ParseTaskSpec(content)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid task heading found")
}

func TestParseTaskSpec_EmptyContent(t *testing.T) {
	t.Parallel()

	_, err := ParseTaskSpec("")
	require.Error(t, err)
}

func TestParseTaskSpec_TitleOnly(t *testing.T) {
	t.Parallel()

	content := "# T-042: Some Task\n"
	spec, err := ParseTaskSpec(content)
	require.NoError(t, err)
	assert.Equal(t, "T-042", spec.ID)
	assert.Equal(t, "Some Task", spec.Title)
	assert.Equal(t, []string{}, spec.Dependencies)
	assert.Equal(t, []string{}, spec.BlockedBy)
	assert.Equal(t, []string{}, spec.Blocks)
	assert.Equal(t, "", spec.Priority)
	assert.Equal(t, "", spec.Effort)
}

func TestParseTaskSpec_WindowsLineEndings(t *testing.T) {
	t.Parallel()

	content := "# T-010: Config Resolution\r\n\r\n## Metadata\r\n| Field | Value |\r\n|-------|-------|\r\n| Priority | Must Have |\r\n| Dependencies | T-009 |\r\n"
	spec, err := ParseTaskSpec(content)
	require.NoError(t, err)
	assert.Equal(t, "T-010", spec.ID)
	assert.Equal(t, "Config Resolution", spec.Title)
	assert.Equal(t, []string{"T-009"}, spec.Dependencies)
	// Content must be normalised: no \r\n sequences remain.
	assert.NotContains(t, spec.Content, "\r\n", "CRLF should be normalised to LF")
}

func TestParseTaskSpec_UTF8BOM(t *testing.T) {
	t.Parallel()

	// Prepend UTF-8 BOM
	content := "\xef\xbb\xbf# T-003: Buildinfo Package\n\n## Metadata\n| Field | Value |\n|-------|-------|\n| Dependencies | None |\n"
	spec, err := ParseTaskSpec(content)
	require.NoError(t, err)
	assert.Equal(t, "T-003", spec.ID)
	assert.Equal(t, "Buildinfo Package", spec.Title)
	assert.Equal(t, []string{}, spec.Dependencies)
	// BOM must not appear in the returned content.
	assert.False(t, strings.HasPrefix(spec.Content, "\xef\xbb\xbf"), "BOM must be stripped from Content")
}

func TestParseTaskSpec_ExtraWhitespaceInMetadata(t *testing.T) {
	t.Parallel()

	content := `# T-020: Status Command

## Metadata
|   Field   |   Value   |
|-----------|-----------|
|   Priority   |   Must Have   |
|   Dependencies   |   T-016 , T-017   |
`
	spec, err := ParseTaskSpec(content)
	require.NoError(t, err)
	assert.Equal(t, "T-020", spec.ID)
	assert.Equal(t, "Must Have", spec.Priority)
	assert.Equal(t, []string{"T-016", "T-017"}, spec.Dependencies)
}

func TestParseTaskSpec_DependenciesWithoutSpaces(t *testing.T) {
	t.Parallel()

	content := "# T-050: Pipeline Core\n\n| Dependencies | T-044,T-045,T-046 |\n"
	spec, err := ParseTaskSpec(content)
	require.NoError(t, err)
	assert.Equal(t, []string{"T-044", "T-045", "T-046"}, spec.Dependencies)
}

func TestParseTaskSpec_ContentPreserved(t *testing.T) {
	t.Parallel()

	content := "# T-007: Version Command\n\nSome longer body text.\n"
	spec, err := ParseTaskSpec(content)
	require.NoError(t, err)
	// Content is normalised (CRLF -> LF, BOM stripped) but otherwise intact.
	assert.Equal(t, content, spec.Content)
}

// TestParseTaskSpec_ContentNormalisedFromCRLF verifies that Windows-style
// CRLF line endings are replaced by LF in the returned Content field.
func TestParseTaskSpec_ContentNormalisedFromCRLF(t *testing.T) {
	t.Parallel()

	raw := "# T-011: Config Validation\r\n\r\nBody text here.\r\n"
	spec, err := ParseTaskSpec(raw)
	require.NoError(t, err)
	// No carriage returns should survive in Content.
	assert.NotContains(t, spec.Content, "\r")
	// Newlines must still be present.
	assert.Contains(t, spec.Content, "\n")
}

// TestParseTaskSpec_MetadataFieldsCaseInsensitive checks that metadata row
// labels are matched case-insensitively (e.g. "PRIORITY" or "priority").
func TestParseTaskSpec_MetadataFieldsCaseInsensitive(t *testing.T) {
	t.Parallel()

	content := "# T-055: Pipeline Command\n\n" +
		"| PRIORITY | Should Have |\n" +
		"| estimated effort | Large: 10-16hrs |\n" +
		"| DEPENDENCIES | T-050, T-051 |\n" +
		"| BLOCKED BY | T-049 |\n" +
		"| BLOCKS | T-060 |\n"

	spec, err := ParseTaskSpec(content)
	require.NoError(t, err)
	assert.Equal(t, "Should Have", spec.Priority)
	assert.Equal(t, "Large: 10-16hrs", spec.Effort)
	assert.Equal(t, []string{"T-050", "T-051"}, spec.Dependencies)
	assert.Equal(t, []string{"T-049"}, spec.BlockedBy)
	assert.Equal(t, []string{"T-060"}, spec.Blocks)
}

// TestParseTaskSpec_AllMetadataFieldsEmpty verifies that when none of the
// optional metadata table rows are present, all optional fields are empty but
// non-nil slices are returned (not nil).
func TestParseTaskSpec_AllMetadataFieldsEmpty(t *testing.T) {
	t.Parallel()

	spec, err := ParseTaskSpec("# T-099: Minimal Task\n")
	require.NoError(t, err)
	assert.Equal(t, "T-099", spec.ID)
	assert.Equal(t, "Minimal Task", spec.Title)
	assert.Equal(t, "", spec.Priority)
	assert.Equal(t, "", spec.Effort)
	// Slices must be non-nil and empty, not nil.
	assert.NotNil(t, spec.Dependencies)
	assert.NotNil(t, spec.BlockedBy)
	assert.NotNil(t, spec.Blocks)
	assert.Len(t, spec.Dependencies, 0)
	assert.Len(t, spec.BlockedBy, 0)
	assert.Len(t, spec.Blocks, 0)
}

// TestParseTaskSpec_IDZeroPaddingPreserved confirms that the three-digit
// zero-padded format is preserved exactly in spec.ID ("T-001", not "T-1").
func TestParseTaskSpec_IDZeroPaddingPreserved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		heading string
		wantID  string
	}{
		{"# T-001: First Task\n", "T-001"},
		{"# T-010: Tenth Task\n", "T-010"},
		{"# T-100: Hundredth Task\n", "T-100"},
		{"# T-999: Last Task\n", "T-999"},
	}

	for _, tt := range tests {
		t.Run(tt.wantID, func(t *testing.T) {
			t.Parallel()
			spec, err := ParseTaskSpec(tt.heading)
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, spec.ID)
		})
	}
}

// TestParseTaskSpec_MakeFullSpecHelper exercises the test helper itself and
// validates a complete round-trip using makeFullSpec.
func TestParseTaskSpec_MakeFullSpecHelper(t *testing.T) {
	t.Parallel()

	content := makeFullSpec(t,
		"T-027", "Implementation Loop Runner",
		"Must Have", "Large: 10-16hrs",
		"T-016, T-021, T-022", "T-016, T-021, T-022", "T-028",
	)

	spec, err := ParseTaskSpec(content)
	require.NoError(t, err)
	assert.Equal(t, "T-027", spec.ID)
	assert.Equal(t, "Implementation Loop Runner", spec.Title)
	assert.Equal(t, "Must Have", spec.Priority)
	assert.Equal(t, "Large: 10-16hrs", spec.Effort)
	assert.Equal(t, []string{"T-016", "T-021", "T-022"}, spec.Dependencies)
	assert.Equal(t, []string{"T-016", "T-021", "T-022"}, spec.BlockedBy)
	assert.Equal(t, []string{"T-028"}, spec.Blocks)
}

// TestParseTaskSpec_NonTableLinesIgnored ensures that lines not beginning
// with "|" are skipped when scanning for metadata, so that task IDs appearing
// in the body do not pollute the parsed fields.
func TestParseTaskSpec_NonTableLinesIgnored(t *testing.T) {
	t.Parallel()

	content := `# T-030: Progress File Generation

## Goal
This task depends on T-001 and T-002 (mentioned in narrative, not metadata).
See also T-003 in the description.

## Metadata
| Field | Value |
|-------|-------|
| Dependencies | T-027 |
`
	spec, err := ParseTaskSpec(content)
	require.NoError(t, err)
	// Only the explicit metadata row should be parsed; narrative IDs are ignored.
	assert.Equal(t, []string{"T-027"}, spec.Dependencies)
}

// --- ParseTaskFile ----------------------------------------------------------

func TestParseTaskFile_Valid(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeFile(t, dir, "T-001-setup-project.md", `# T-001: Setup Project

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Small: 1-3hrs |
| Dependencies | None |
| Blocked By | None |
| Blocks | T-002, T-003 |
`)

	spec, err := ParseTaskFile(path)
	require.NoError(t, err)
	assert.Equal(t, "T-001", spec.ID)
	assert.Equal(t, "Setup Project", spec.Title)
	assert.Equal(t, path, spec.SpecFile)
}

func TestParseTaskFile_NonExistentFile(t *testing.T) {
	t.Parallel()

	_, err := ParseTaskFile("/nonexistent/path/T-999-missing.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening task file")
}

func TestParseTaskFile_OversizedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create a header so the file is a valid task spec except for its size.
	header := "# T-088: Giant Task\n\n"
	// Build a content larger than 1 MiB.
	padding := make([]byte, maxTaskFileSize+1)
	for i := range padding {
		padding[i] = 'x'
	}
	content := header + string(padding)

	path := writeFile(t, dir, "T-088-giant-task.md", content)
	_, err := ParseTaskFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds 1 MiB limit")
}

// TestParseTaskFile_SpecFileFieldPopulated verifies that ParseTaskFile sets
// SpecFile to the absolute path of the file that was read.
func TestParseTaskFile_SpecFileFieldPopulated(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeFile(t, dir, "T-022-claude-agent.md", "# T-022: Claude Agent Adapter\n")
	spec, err := ParseTaskFile(path)
	require.NoError(t, err)
	assert.Equal(t, path, spec.SpecFile)
}

// TestParseTaskFile_WindowsLineEndings verifies that CRLF files on disk are
// parsed correctly; the SpecFile path is still set correctly after
// normalisation.
func TestParseTaskFile_WindowsLineEndings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	raw := "# T-023: Codex Agent Adapter\r\n\r\n| Dependencies | T-021 |\r\n"
	path := writeFile(t, dir, "T-023-codex-agent.md", raw)

	spec, err := ParseTaskFile(path)
	require.NoError(t, err)
	assert.Equal(t, "T-023", spec.ID)
	assert.Equal(t, path, spec.SpecFile)
	assert.Equal(t, []string{"T-021"}, spec.Dependencies)
}

// TestParseTaskFile_UTF8BOMOnDisk verifies that a file with a BOM byte prefix
// is handled correctly by ParseTaskFile.
func TestParseTaskFile_UTF8BOMOnDisk(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Write BOM + valid task spec directly as bytes.
	raw := "\xef\xbb\xbf# T-024: Gemini Agent Stub\n\n| Dependencies | T-021 |\n"
	path := writeFile(t, dir, "T-024-gemini-agent.md", raw)

	spec, err := ParseTaskFile(path)
	require.NoError(t, err)
	assert.Equal(t, "T-024", spec.ID)
	assert.Equal(t, "Gemini Agent Stub", spec.Title)
	assert.Equal(t, []string{"T-021"}, spec.Dependencies)
}

// TestParseTaskFile_InvalidMarkupError checks that a parse error from
// ParseTaskSpec is correctly wrapped and returned by ParseTaskFile.
func TestParseTaskFile_InvalidMarkupError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeFile(t, dir, "T-090-bad-markup.md", "## Not a valid task heading\n\nSome content.\n")

	_, err := ParseTaskFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing task file")
}

// --- DiscoverTasks ----------------------------------------------------------

func TestDiscoverTasks_SortedByID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "T-003-implement-feature.md", "# T-003: Implement Feature\n\n| Dependencies | T-001, T-002 |\n")
	writeFile(t, dir, "T-001-setup-project.md", "# T-001: Setup Project\n\n| Dependencies | None |\n")
	writeFile(t, dir, "T-002-config-loading.md", "# T-002: Config Loading\n\n| Dependencies | None |\n")

	specs, err := DiscoverTasks(dir)
	require.NoError(t, err)
	require.Len(t, specs, 3)
	assert.Equal(t, "T-001", specs[0].ID)
	assert.Equal(t, "T-002", specs[1].ID)
	assert.Equal(t, "T-003", specs[2].ID)
}

func TestDiscoverTasks_EmptyDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	specs, err := DiscoverTasks(dir)
	require.NoError(t, err)
	assert.Empty(t, specs)
}

func TestDiscoverTasks_SkipsNonTaskFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "T-001-setup.md", "# T-001: Setup\n")
	writeFile(t, dir, "INDEX.md", "# Index\n")
	writeFile(t, dir, "PROGRESS.md", "# Progress\n")
	writeFile(t, dir, "README.md", "# Readme\n")
	// T-NNN.md without description segment should also be skipped.
	writeFile(t, dir, "T-002.md", "# T-002: No Description\n")

	specs, err := DiscoverTasks(dir)
	require.NoError(t, err)
	require.Len(t, specs, 1)
	assert.Equal(t, "T-001", specs[0].ID)
}

func TestDiscoverTasks_DuplicateIDReturnsError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "T-001-setup-project.md", "# T-001: Setup Project\n")
	writeFile(t, dir, "T-001-alternative-setup.md", "# T-001: Alternative Setup\n")

	_, err := DiscoverTasks(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate task ID")
	assert.Contains(t, err.Error(), "T-001")
}

func TestDiscoverTasks_ParseErrorBubblesUp(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// A file that matches the glob pattern but has no valid heading.
	writeFile(t, dir, "T-005-invalid.md", "## No title here\n")

	_, err := DiscoverTasks(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "discovering tasks")
}

// TestDiscoverTasks_SpecFilePathsAreAbsolute confirms that every spec
// returned by DiscoverTasks has SpecFile set to a non-empty path that
// includes the task ID in the filename component.
func TestDiscoverTasks_SpecFilePathsAreAbsolute(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "T-010-config-resolution.md", "# T-010: Config Resolution\n")
	writeFile(t, dir, "T-011-config-validation.md", "# T-011: Config Validation\n")

	specs, err := DiscoverTasks(dir)
	require.NoError(t, err)
	require.Len(t, specs, 2)
	for _, s := range specs {
		assert.NotEmpty(t, s.SpecFile, "SpecFile must be set for %s", s.ID)
		assert.Contains(t, filepath.Base(s.SpecFile), s.ID,
			"SpecFile base name must contain the task ID for %s", s.ID)
	}
}

// TestDiscoverTasks_ManyFiles verifies sorting is stable across a larger
// set of discovered files (five tasks).
func TestDiscoverTasks_ManyFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ids := []string{"T-050", "T-021", "T-005", "T-073", "T-009"}
	for _, id := range ids {
		slug := strings.ToLower(strings.ReplaceAll(id, "-", "")) + "-slug"
		filename := id + "-" + slug + ".md"
		writeFile(t, dir, filename, "# "+id+": Some Task Title\n")
	}

	specs, err := DiscoverTasks(dir)
	require.NoError(t, err)
	require.Len(t, specs, 5)
	// Verify strictly ascending order.
	for i := 1; i < len(specs); i++ {
		assert.Less(t, specs[i-1].ID, specs[i].ID,
			"expected ascending order at index %d (got %s then %s)",
			i, specs[i-1].ID, specs[i].ID)
	}
}

// TestDiscoverTasks_FilesOnlyAtTopLevel verifies that DiscoverTasks does not
// recurse into subdirectories (filepath.Glob is non-recursive).
func TestDiscoverTasks_FilesOnlyAtTopLevel(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "T-001-top-level.md", "# T-001: Top Level\n")
	// Create a subdirectory with a valid task file that must NOT be picked up.
	sub := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	writeFile(t, sub, "T-002-in-subdir.md", "# T-002: In Subdir\n")

	specs, err := DiscoverTasks(dir)
	require.NoError(t, err)
	require.Len(t, specs, 1)
	assert.Equal(t, "T-001", specs[0].ID)
}

// --- ToTask -----------------------------------------------------------------

func TestParsedTaskSpec_ToTask(t *testing.T) {
	t.Parallel()

	spec := &ParsedTaskSpec{
		ID:           "T-016",
		Title:        "Task Spec Markdown Parser",
		Priority:     "Must Have",
		Effort:       "Medium: 6-10hrs",
		Dependencies: []string{"T-004"},
		BlockedBy:    []string{"T-004"},
		Blocks:       []string{"T-017", "T-019"},
		Content:      "# T-016: Task Spec Markdown Parser\n",
		SpecFile:     "docs/tasks/T-016-task-spec-parser.md",
	}

	task := spec.ToTask()

	assert.Equal(t, "T-016", task.ID)
	assert.Equal(t, "Task Spec Markdown Parser", task.Title)
	assert.Equal(t, StatusNotStarted, task.Status)
	assert.Equal(t, 0, task.Phase)
	assert.Equal(t, []string{"T-004"}, task.Dependencies)
	assert.Equal(t, "docs/tasks/T-016-task-spec-parser.md", task.SpecFile)
}

func TestParsedTaskSpec_ToTask_IsolatesDependencies(t *testing.T) {
	t.Parallel()

	// Verify that mutating the returned Task's Dependencies does not affect
	// the original ParsedTaskSpec (defensive copy).
	deps := []string{"T-001", "T-002"}
	spec := &ParsedTaskSpec{
		ID:           "T-010",
		Title:        "Config Resolution",
		Dependencies: deps,
	}

	task := spec.ToTask()
	task.Dependencies[0] = "T-999"

	assert.Equal(t, "T-001", spec.Dependencies[0], "original must not be mutated")
}

func TestParsedTaskSpec_ToTask_EmptyDependencies(t *testing.T) {
	t.Parallel()

	spec := &ParsedTaskSpec{
		ID:           "T-001",
		Title:        "Setup",
		Dependencies: []string{},
	}
	task := spec.ToTask()
	assert.Equal(t, []string{}, task.Dependencies)
}

// TestParsedTaskSpec_ToTask_ValidTaskStruct verifies that ToTask produces a
// Task whose Status and Phase match the expected initial values per T-004.
func TestParsedTaskSpec_ToTask_ValidTaskStruct(t *testing.T) {
	t.Parallel()

	spec := &ParsedTaskSpec{
		ID:           "T-017",
		Title:        "Task State Management",
		Dependencies: []string{"T-016"},
		SpecFile:     "/docs/tasks/T-017-task-state-management.md",
	}

	task := spec.ToTask()

	// Status must be StatusNotStarted ("not_started") per T-004 spec.
	assert.Equal(t, StatusNotStarted, task.Status)
	assert.True(t, ValidStatus(task.Status), "ToTask must produce a valid status")
	// Phase must be 0 (unset) to be populated later by the phase resolver.
	assert.Equal(t, 0, task.Phase)
	// SpecFile must be passed through.
	assert.Equal(t, spec.SpecFile, task.SpecFile)
	// ID and Title must match.
	assert.Equal(t, spec.ID, task.ID)
	assert.Equal(t, spec.Title, task.Title)
}

// TestParsedTaskSpec_ToTask_NilDependencies tests that a nil Dependencies
// field on ParsedTaskSpec produces an empty (not nil) slice on Task.
func TestParsedTaskSpec_ToTask_NilDependencies(t *testing.T) {
	t.Parallel()

	spec := &ParsedTaskSpec{
		ID:           "T-001",
		Title:        "Setup",
		Dependencies: nil,
	}
	task := spec.ToTask()
	// make([]string, 0) copies nil as an empty slice.
	assert.NotNil(t, task.Dependencies)
	assert.Len(t, task.Dependencies, 0)
}

// --- extractTaskRefs --------------------------------------------------------

func TestExtractTaskRefs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single reference",
			input: "T-004",
			want:  []string{"T-004"},
		},
		{
			name:  "multiple comma-separated with spaces",
			input: "T-001, T-003",
			want:  []string{"T-001", "T-003"},
		},
		{
			name:  "multiple without spaces",
			input: "T-001,T-003",
			want:  []string{"T-001", "T-003"},
		},
		{
			name:  "none lowercase",
			input: "none",
			want:  []string{},
		},
		{
			name:  "None titlecase",
			input: "None",
			want:  []string{},
		},
		{
			name:  "NONE uppercase",
			input: "NONE",
			want:  []string{},
		},
		{
			name:  "None with whitespace",
			input: "  None  ",
			want:  []string{},
		},
		{
			name:  "empty string",
			input: "",
			want:  []string{},
		},
		{
			name:  "string with no task refs",
			input: "some random text",
			want:  []string{},
		},
		{
			name:  "many references",
			input: "T-017, T-019, T-020, T-026, T-030",
			want:  []string{"T-017", "T-019", "T-020", "T-026", "T-030"},
		},
		{
			name:  "refs embedded in prose",
			input: "depends on T-001 and possibly T-003 for utilities",
			want:  []string{"T-001", "T-003"},
		},
		{
			name:  "whitespace-only string",
			input: "   ",
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractTaskRefs(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestExtractTaskRefs_ReturnTypeIsNonNil verifies that extractTaskRefs never
// returns a nil slice, only an empty non-nil slice.
func TestExtractTaskRefs_ReturnTypeIsNonNil(t *testing.T) {
	t.Parallel()

	got := extractTaskRefs("None")
	assert.NotNil(t, got, "must return non-nil slice for 'None'")

	got2 := extractTaskRefs("")
	assert.NotNil(t, got2, "must return non-nil slice for empty string")

	got3 := extractTaskRefs("no refs here at all")
	assert.NotNil(t, got3, "must return non-nil slice when no refs found")
}

// --- reTaskFilename regex ---------------------------------------------------

func TestReTaskFilename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{name: "valid simple", filename: "T-001-setup.md", want: true},
		{name: "valid multi-word", filename: "T-016-task-spec-parser.md", want: true},
		{name: "valid three digit", filename: "T-085-ci-cd-pipeline.md", want: true},
		{name: "valid with underscores", filename: "T-010-config_resolution.md", want: true},
		{name: "missing description", filename: "T-001.md", want: false},
		{name: "README", filename: "README.md", want: false},
		{name: "INDEX", filename: "INDEX.md", want: false},
		{name: "PROGRESS", filename: "PROGRESS.md", want: false},
		{name: "wrong extension txt", filename: "T-001-setup.txt", want: false},
		{name: "wrong extension no extension", filename: "T-001-setup", want: false},
		{name: "two digit ID", filename: "T-01-setup.md", want: false},
		{name: "four digit ID", filename: "T-0001-setup.md", want: false},
		{name: "lowercase t prefix", filename: "t-001-setup.md", want: false},
		{name: "space in filename", filename: "T-001-some task.md", want: false},
		{name: "description starts with digit", filename: "T-002-2nd-task.md", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := reTaskFilename.MatchString(tt.filename)
			assert.Equal(t, tt.want, got, "filename %q", tt.filename)
		})
	}
}

// --- integration: parse real task files -------------------------------------

func TestDiscoverTasks_TestdataFixtures(t *testing.T) {
	t.Parallel()

	// Use the testdata/task-specs directory created alongside this test.
	dir := "testdata/task-specs"
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skip("testdata/task-specs not present")
	}

	specs, err := DiscoverTasks(dir)
	require.NoError(t, err)
	require.NotEmpty(t, specs)

	// IDs must be ordered.
	for i := 1; i < len(specs); i++ {
		assert.Less(t, specs[i-1].ID, specs[i].ID,
			"expected ascending ID order at index %d", i)
	}
}

// TestParseTaskFile_RealFixture parses one of the bundled testdata fixtures
// directly via ParseTaskFile, verifying that SpecFile is set correctly.
func TestParseTaskFile_RealFixture(t *testing.T) {
	t.Parallel()

	path := "testdata/task-specs/T-001-setup-project.md"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("testdata/task-specs/T-001-setup-project.md not present")
	}

	spec, err := ParseTaskFile(path)
	require.NoError(t, err)
	assert.Equal(t, "T-001", spec.ID)
	assert.Equal(t, path, spec.SpecFile)
	assert.NotEmpty(t, spec.Title)
}

// TestDiscoverTasks_RealProjectTasks runs DiscoverTasks against the actual
// docs/tasks/ directory in the Raven project. It verifies basic invariants:
//   - at least one task is found
//   - all IDs follow the T-NNN pattern
//   - results are in strictly ascending order
//   - every spec has a non-empty Title
func TestDiscoverTasks_RealProjectTasks(t *testing.T) {
	t.Parallel()

	// Relative to the package directory (internal/task/).
	dir := "../../docs/tasks"
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skip("docs/tasks directory not present")
	}

	specs, err := DiscoverTasks(dir)
	require.NoError(t, err)
	require.NotEmpty(t, specs, "expected at least one task in docs/tasks/")

	// Every spec must have a non-empty ID and Title.
	for _, s := range specs {
		assert.NotEmpty(t, s.ID, "spec must have a non-empty ID")
		assert.NotEmpty(t, s.Title, "spec %s must have a non-empty Title", s.ID)
		assert.Regexp(t, `^T-\d{3}$`, s.ID, "ID must match T-NNN pattern")
	}

	// Results must be strictly ascending by ID.
	for i := 1; i < len(specs); i++ {
		assert.Less(t, specs[i-1].ID, specs[i].ID,
			"expected ascending order at index %d (got %s before %s)",
			i, specs[i-1].ID, specs[i].ID)
	}
}

// --- Benchmark tests --------------------------------------------------------

// BenchmarkParseTaskSpec benchmarks parsing of a representative task spec.
func BenchmarkParseTaskSpec(b *testing.B) {
	content := `# T-016: Task Spec Markdown Parser

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-004 |
| Blocked By | T-004 |
| Blocks | T-017, T-019, T-020, T-026, T-030 |

## Goal
Implement a markdown parser that reads task specification files and extracts
structured data into Task structs. This parser is the entry point for the
entire task management system.

## Acceptance Criteria
- [ ] ParseTaskSpec extracts task ID from heading
- [ ] ParseTaskSpec extracts dependencies from metadata table
- [ ] ParseTaskSpec extracts priority and effort from metadata table
`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseTaskSpec(content)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkExtractTaskRefs benchmarks the task reference extraction function.
func BenchmarkExtractTaskRefs(b *testing.B) {
	input := "T-001, T-003, T-005, T-007, T-009, T-011, T-013, T-015"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = extractTaskRefs(input)
	}
}

// BenchmarkDiscoverTasks benchmarks directory scanning on a directory with
// ten task files.
func BenchmarkDiscoverTasks(b *testing.B) {
	dir := b.TempDir()
	for i := 1; i <= 10; i++ {
		name := fmt.Sprintf("T-%03d-benchmark-task.md", i)
		path := filepath.Join(dir, name)
		content := fmt.Sprintf("# T-%03d: Benchmark Task %d\n\n| Dependencies | None |\n", i, i)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := DiscoverTasks(dir)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// --- Fuzz tests -------------------------------------------------------------

// FuzzParseTaskSpec is a fuzz target for the task spec parser. It verifies
// that no input causes a panic and that if parsing succeeds the result is
// self-consistent.
func FuzzParseTaskSpec(f *testing.F) {
	// Seed corpus with representative inputs.
	f.Add("# T-001: Setup\n")
	f.Add("# T-016: Task Spec Markdown Parser\n\n| Dependencies | T-004 |\n| Priority | Must Have |\n")
	f.Add("## Not a valid heading\n")
	f.Add("")
	f.Add("\xef\xbb\xbf# T-003: BOM Task\n")
	f.Add("# T-001: CRLF Task\r\n\r\n| Dependencies | None |\r\n")
	f.Add("# T-999: Last Task\n\n| Dependencies | T-001, T-002, T-003 |\n")
	f.Add(strings.Repeat("x", 4096))
	f.Add("# T-050:\n") // empty title after colon

	f.Fuzz(func(t *testing.T, input string) {
		spec, err := ParseTaskSpec(input)
		if err != nil {
			// Error is acceptable; just must not panic.
			return
		}
		// If parsing succeeded, invariants must hold.
		if spec.ID == "" {
			t.Error("successful parse must produce non-empty ID")
		}
		if spec.Dependencies == nil {
			t.Error("Dependencies must never be nil on success")
		}
		if spec.BlockedBy == nil {
			t.Error("BlockedBy must never be nil on success")
		}
		if spec.Blocks == nil {
			t.Error("Blocks must never be nil on success")
		}
	})
}
