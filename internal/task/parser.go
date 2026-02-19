package task

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// maxTaskFileSize is the maximum number of bytes read from a single task spec
// file. Files larger than this limit are rejected to prevent memory exhaustion.
const maxTaskFileSize = 1 << 20 // 1 MiB

// utf8BOM is the byte-order mark sequence prepended by some editors to UTF-8
// files. It is stripped before parsing so that regexes match reliably.
var utf8BOM = "\xef\xbb\xbf"

// Pre-compiled regexes for parsing task spec markdown files.
var (
	// reTitleLine matches "# T-001: Some Title" or "# T-001 - Some Title" at the start of a line.
	reTitleLine = regexp.MustCompile(`^#\s+T-(\d{3})(?::\s*|\s+-\s+)(.+)$`)

	// reMetaDeps matches "| Dependencies | T-001, T-003 |" in metadata table.
	reMetaDeps = regexp.MustCompile(`(?i)\|\s*Dependencies\s*\|\s*([^|]+)\|`)

	// reMetaPriority matches "| Priority | Must Have |" in metadata table.
	reMetaPriority = regexp.MustCompile(`(?i)\|\s*Priority\s*\|\s*([^|]+)\|`)

	// reMetaEffort matches "| Estimated Effort | Medium: 6-10hrs |".
	reMetaEffort = regexp.MustCompile(`(?i)\|\s*Estimated\s+Effort\s*\|\s*([^|]+)\|`)

	// reMetaBlockedBy matches "| Blocked By | T-001, T-003 |".
	reMetaBlockedBy = regexp.MustCompile(`(?i)\|\s*Blocked\s+By\s*\|\s*([^|]+)\|`)

	// reMetaBlocks matches "| Blocks | T-005, T-006 |".
	reMetaBlocks = regexp.MustCompile(`(?i)\|\s*Blocks\s*\|\s*([^|]+)\|`)

	// reTaskRef matches task ID references like "T-001", "T-123".
	reTaskRef = regexp.MustCompile(`T-(\d{3})`)

	// reTaskFilename matches task spec filenames like "T-001-some-description.md".
	reTaskFilename = regexp.MustCompile(`^T-(\d{3})-[\w-]+\.md$`)
)

// ParsedTaskSpec holds all data extracted from a task spec markdown file.
type ParsedTaskSpec struct {
	// ID is the zero-padded task identifier, e.g. "T-016".
	ID string
	// Title is the human-readable task name extracted from the heading.
	Title string
	// Priority is the value from the Priority metadata row, e.g. "Must Have".
	Priority string
	// Effort is the value from the Estimated Effort metadata row.
	Effort string
	// Dependencies are task IDs listed in the Dependencies metadata row.
	Dependencies []string
	// BlockedBy are task IDs listed in the Blocked By metadata row.
	BlockedBy []string
	// Blocks are task IDs listed in the Blocks metadata row.
	Blocks []string
	// Content is the complete, unmodified markdown content.
	Content string
	// SpecFile is the filesystem path the spec was read from.
	SpecFile string
}

// ParseTaskSpec parses raw markdown content of a task spec file.
// It returns a ParsedTaskSpec or an error if the content does not contain
// a valid task spec heading ("# T-NNN: Title" or "# T-NNN - Title").
func ParseTaskSpec(content string) (*ParsedTaskSpec, error) {
	// Strip UTF-8 BOM if present.
	content = strings.TrimPrefix(content, utf8BOM)

	// Normalise Windows line endings.
	content = strings.ReplaceAll(content, "\r\n", "\n")

	spec := &ParsedTaskSpec{
		Content:      content,
		Dependencies: []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
	}

	foundTitle := false
	for _, line := range strings.Split(content, "\n") {
		// --- Title line ---
		if !foundTitle {
			if m := reTitleLine.FindStringSubmatch(line); m != nil {
				spec.ID = "T-" + m[1]
				spec.Title = strings.TrimSpace(m[2])
				foundTitle = true
				continue
			}
		}

		// --- Metadata table rows (only lines that start with "|") ---
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			continue
		}

		if m := reMetaDeps.FindStringSubmatch(line); m != nil {
			spec.Dependencies = extractTaskRefs(m[1])
			continue
		}
		if m := reMetaBlockedBy.FindStringSubmatch(line); m != nil {
			spec.BlockedBy = extractTaskRefs(m[1])
			continue
		}
		if m := reMetaBlocks.FindStringSubmatch(line); m != nil {
			spec.Blocks = extractTaskRefs(m[1])
			continue
		}
		if m := reMetaPriority.FindStringSubmatch(line); m != nil {
			spec.Priority = strings.TrimSpace(m[1])
			continue
		}
		if m := reMetaEffort.FindStringSubmatch(line); m != nil {
			spec.Effort = strings.TrimSpace(m[1])
			continue
		}
	}

	if !foundTitle {
		return nil, fmt.Errorf("parsing task spec: no valid task heading found (expected '# T-NNN: Title' or '# T-NNN - Title')")
	}

	return spec, nil
}

// ParseTaskFile reads a task spec file from disk and parses it.
// It enforces a 1 MiB size limit and delegates to ParseTaskSpec.
func ParseTaskFile(path string) (*ParsedTaskSpec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening task file %q: %w", path, err)
	}
	defer f.Close() //nolint:errcheck

	// Enforce size limit: read at most maxTaskFileSize+1 bytes so we can
	// detect oversized files without loading them entirely into memory.
	limited := io.LimitReader(f, maxTaskFileSize+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("reading task file %q: %w", path, err)
	}
	if int64(len(raw)) > maxTaskFileSize {
		return nil, fmt.Errorf("task file %q exceeds 1 MiB limit", path)
	}

	spec, err := ParseTaskSpec(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parsing task file %q: %w", path, err)
	}
	spec.SpecFile = path
	return spec, nil
}

// DiscoverTasks scans dir for files matching "T-[0-9][0-9][0-9]-*.md",
// parses each one, and returns the results sorted by task ID.
// An error is returned if any file cannot be parsed or if duplicate task IDs
// are found.
func DiscoverTasks(dir string) ([]*ParsedTaskSpec, error) {
	pattern := filepath.Join(dir, "T-[0-9][0-9][0-9]-*.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("globbing task specs in %q: %w", dir, err)
	}

	// Filter using the strict filename regex to reject edge cases that
	// filepath.Glob might allow (e.g. filenames with unusual characters).
	var paths []string
	for _, p := range matches {
		base := filepath.Base(p)
		if reTaskFilename.MatchString(base) {
			paths = append(paths, p)
		}
	}

	seen := make(map[string]string, len(paths)) // id -> first path
	specs := make([]*ParsedTaskSpec, 0, len(paths))

	for _, p := range paths {
		spec, err := ParseTaskFile(p)
		if err != nil {
			return nil, fmt.Errorf("discovering tasks: %w", err)
		}

		if first, dup := seen[spec.ID]; dup {
			return nil, fmt.Errorf(
				"discovering tasks: duplicate task ID %q found in %q and %q",
				spec.ID, first, p,
			)
		}
		seen[spec.ID] = p
		specs = append(specs, spec)
	}

	// Sort lexicographically by ID; zero-padded IDs sort correctly.
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].ID < specs[j].ID
	})

	return specs, nil
}

// ToTask converts a ParsedTaskSpec into the central Task type.
// Phase is set to 0 and must be populated later by cross-referencing phases
// configuration. Status is initialised to StatusNotStarted.
func (p *ParsedTaskSpec) ToTask() *Task {
	deps := make([]string, len(p.Dependencies))
	copy(deps, p.Dependencies)

	return &Task{
		ID:           p.ID,
		Title:        p.Title,
		Status:       StatusNotStarted,
		Phase:        0,
		Dependencies: deps,
		SpecFile:     p.SpecFile,
	}
}

// extractTaskRefs returns all T-NNN references found in s.
// If s is "None" (case-insensitive) or contains no task references,
// an empty (non-nil) slice is returned.
func extractTaskRefs(s string) []string {
	trimmed := strings.TrimSpace(s)
	if strings.EqualFold(trimmed, "none") {
		return []string{}
	}

	all := reTaskRef.FindAllStringSubmatch(trimmed, -1)
	if len(all) == 0 {
		return []string{}
	}

	refs := make([]string, 0, len(all))
	for _, m := range all {
		refs = append(refs, "T-"+m[1])
	}
	return refs
}
