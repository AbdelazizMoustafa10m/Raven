// Command gen-manpages generates Unix man pages for Raven and all its
// subcommands using cobra's built-in doc package. GoReleaser invokes this
// program as a before.hook to pre-populate the man/man1/ directory that is
// bundled into release archives.
//
// Usage:
//
//	go run ./scripts/gen-manpages [output-dir]
//
// The default output directory is "man/man1".
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra/doc"

	"github.com/AbdelazizMoustafa10m/Raven/internal/cli"
)

func main() {
	outDir := "man/man1"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating output dir %q: %v\n", outDir, err)
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
		fmt.Fprintf(os.Stderr, "error generating man pages: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Man pages generated in %s/\n", outDir)
}
