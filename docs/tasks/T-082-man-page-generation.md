# T-082: Man Page Generation Using cobra/doc

## Metadata
| Field | Value |
|-------|-------|
| Priority | Should Have |
| Estimated Effort | Small: 2-4hrs |
| Dependencies | T-006, T-079 |
| Blocked By | T-006 |
| Blocks | T-087 |

## Goal
Generate Unix man pages for Raven and all its subcommands using Cobra's built-in `doc` package. Create a Go program that produces man pages at build time, and integrate man page generation into the GoReleaser build so they ship with release archives. Users can install man pages to view `man raven`, `man raven-implement`, etc.

## Background
Per PRD Section 7 (Phase 7), Raven requires "Man page generation" as part of the polish and distribution phase. Cobra provides a `doc` subpackage (`github.com/spf13/cobra/doc`) with `GenManTree` and `GenManTreeFromOpts` functions that generate troff-formatted man pages for a command tree. These are standard Section 1 man pages viewable with the `man` command on Unix systems.

Man pages complement shell completions (T-081) and the README (T-086) as part of Raven's documentation story. They provide offline, terminal-native documentation accessible via `man raven` without internet access.

## Technical Specifications
### Implementation Approach
Create `scripts/gen-manpages/main.go` that imports the root Cobra command and calls `doc.GenManTree()` to generate man pages for all commands. Configure GoReleaser to run this program as a `before.hook` and include the generated man pages in release archives. Provide an installation script for copying man pages to system directories.

### Key Components
- **`scripts/gen-manpages/main.go`**: Go program that generates man pages for all Raven commands
- **`man/`**: Output directory for generated man pages (gitignored, generated at build time)
- **GoReleaser integration**: Include man pages in release archives
- **`scripts/install-manpages.sh`**: Simple installer for copying man pages to system paths

### API/Interface Contracts
```go
// scripts/gen-manpages/main.go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra/doc"

	"raven/internal/cli"
)

func main() {
	outDir := "man/man1"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "creating output dir: %v\n", err)
		os.Exit(1)
	}

	rootCmd := cli.NewRootCmd()

	header := &doc.GenManHeader{
		Title:   "RAVEN",
		Section: "1",
		Source:  "Raven",
		Manual:  "Raven Manual",
	}

	if err := doc.GenManTree(rootCmd, header, outDir); err != nil {
		fmt.Fprintf(os.Stderr, "generating man pages: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Man pages generated in %s/\n", outDir)
}
```

```bash
#!/usr/bin/env bash
# scripts/install-manpages.sh
# Install Raven man pages to system man directory
set -euo pipefail

MANDIR="${1:-/usr/local/share/man/man1}"
SRCDIR="man/man1"

if [[ ! -d "$SRCDIR" ]]; then
    echo "Man pages not found in $SRCDIR. Run 'make manpages' first."
    exit 1
fi

mkdir -p "$MANDIR"
cp "$SRCDIR"/*.1 "$MANDIR/"
echo "Man pages installed to $MANDIR"
echo "Try: man raven"
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| github.com/spf13/cobra/doc | v1.10+ (part of cobra) | Man page generation from command tree |
| github.com/cpuguy83/go-md2man/v2 | v2.0.4+ | Transitive dependency of cobra/doc for troff formatting |
| internal/cli | - | Root command for man page generation |

## Acceptance Criteria
- [ ] `scripts/gen-manpages/main.go` compiles and generates man pages for all Raven commands
- [ ] Man pages are generated in troff format (Section 1) in `man/man1/` directory
- [ ] `man raven` displays the root command help (when installed)
- [ ] `man raven-implement` displays the implement subcommand help
- [ ] All subcommands have corresponding man pages generated
- [ ] Man pages include command name, synopsis, description, options, and see-also references
- [ ] GoReleaser includes man pages in release archives under `man/man1/`
- [ ] `make manpages` target generates man pages locally
- [ ] `scripts/install-manpages.sh` copies man pages to `/usr/local/share/man/man1/`
- [ ] `man/` directory is in `.gitignore` (generated artifacts)

## Testing Requirements
### Unit Tests
- No Go unit tests for the generator itself (it is a thin wrapper around cobra/doc)

### Integration Tests
- Run `go run scripts/gen-manpages/main.go` and verify man page files are created
- Verify at least `raven.1` and `raven-implement.1` exist
- Verify man pages are valid troff format (pipe through `man -l raven.1` or `nroff -man raven.1`)
- Verify man pages contain the correct section number and title
- Count generated files matches expected subcommand count

### Edge Cases to Handle
- Subcommands with hyphens in names (e.g., `raven config-debug` vs `raven config debug`)
- Hidden commands should not generate man pages (or should be marked as such)
- Very long command descriptions or examples should not break troff formatting
- Running on systems without `man` installed (Windows)

## Implementation Notes
### Recommended Approach
1. Add `github.com/spf13/cobra/doc` to imports (it is a separate import path within the cobra module)
2. Create `scripts/gen-manpages/main.go` with the code above
3. Run `go run scripts/gen-manpages/main.go` and inspect the output
4. Verify with `man -l man/man1/raven.1` on macOS or Linux
5. Add `man/` to `.gitignore`
6. Update `.goreleaser.yaml` to add a before hook: `go run scripts/gen-manpages/main.go`
7. Update `.goreleaser.yaml` archives to include `man/man1/*.1` as extra files
8. Add `manpages` and `install-manpages` targets to the Makefile
9. Create `scripts/install-manpages.sh`

### Potential Pitfalls
- Cobra's `GenManTree` generates files named `<command>-<subcommand>.1`. For deeply nested commands (e.g., `raven config debug`), the file will be `raven-config-debug.1`. This is standard behavior.
- The `cobra/doc` package is a separate import (`github.com/spf13/cobra/doc`) but part of the same module. It pulls in `cpuguy83/go-md2man` as a transitive dependency for markdown-to-troff conversion.
- Cobra's `GenManTree` generates man pages for all commands including the `completion` and `help` built-in commands. This is generally desirable but can be suppressed by marking commands as `Hidden`.
- Man page generation requires the full command tree to be initialized. Ensure `NewRootCmd()` registers all subcommands without requiring config files or runtime state.

### Security Considerations
- Man pages are static text files with no executable content
- Installation to `/usr/local/share/man/man1/` may require elevated privileges; the script does not use `sudo` automatically
- No sensitive information should be included in man pages (no API keys, tokens, or internal paths)

## References
- [Cobra doc Package Documentation](https://pkg.go.dev/github.com/spf13/cobra/doc)
- [Cobra Man Page Generation Source](https://github.com/spf13/cobra/blob/main/doc/man_docs.go)
- [man-pages(7) Linux Man Page Format](https://man7.org/linux/man-pages/man7/man-pages.7.html)
- [go-md2man: Markdown to Man Page Converter](https://github.com/cpuguy83/go-md2man)