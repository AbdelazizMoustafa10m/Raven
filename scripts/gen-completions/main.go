// Command gen-completions generates shell completion scripts for all supported
// shells (bash, zsh, fish, powershell) and writes them to an output directory.
// GoReleaser invokes this program as a before.hook to pre-populate the
// completions/ directory that is bundled into release archives.
//
// Usage:
//
//	go run ./scripts/gen-completions [output-dir]
//
// The default output directory is "completions".
package main

import (
	"fmt"
	"os"

	"github.com/AbdelazizMoustafa10m/Raven/internal/cli"
)

func main() {
	outDir := "completions"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating output dir %q: %v\n", outDir, err)
		os.Exit(1)
	}

	rootCmd := cli.NewRootCmd()

	type completionEntry struct {
		filename string
		generate func(f *os.File) error
	}

	entries := []completionEntry{
		{
			filename: outDir + "/raven.bash",
			generate: func(f *os.File) error {
				return rootCmd.GenBashCompletionV2(f, true)
			},
		},
		{
			filename: outDir + "/_raven",
			generate: func(f *os.File) error {
				return rootCmd.GenZshCompletion(f)
			},
		},
		{
			filename: outDir + "/raven.fish",
			generate: func(f *os.File) error {
				return rootCmd.GenFishCompletion(f, true)
			},
		},
		{
			filename: outDir + "/raven.ps1",
			generate: func(f *os.File) error {
				return rootCmd.GenPowerShellCompletionWithDesc(f)
			},
		},
	}

	for _, e := range entries {
		f, err := os.Create(e.filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error creating %q: %v\n", e.filename, err)
			os.Exit(1)
		}
		if err := e.generate(f); err != nil {
			f.Close()
			fmt.Fprintf(os.Stderr, "error generating completion for %q: %v\n", e.filename, err)
			os.Exit(1)
		}
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "error closing %q: %v\n", e.filename, err)
			os.Exit(1)
		}
		fmt.Printf("Generated %s\n", e.filename)
	}

	fmt.Printf("All completions written to %s/\n", outDir)
}
