---
name: go-cli-cobra
description: Cobra CLI best practices for command design, flags/config, error handling, output contracts, and architecture. Use when building or reviewing CLI commands in Raven.
---

# Go CLI with Cobra

## 1 -- Architecture

### Project Layout

```
cmd/raven/main.go         # Minimal: call cmd.Execute() only
internal/
  cli/                     # Cobra command definitions
    root.go                # Root command, persistent flags
    generate.go            # generate subcommand
    version.go             # version subcommand
  config/                  # Config loading, validation
  discovery/               # Business logic (never in cli/)
  pipeline/                # Core pipeline logic
```

### Rules

- Keep `main.go` minimal: parse nothing, call `cmd.Execute()`, exit.
- All Cobra commands live in `internal/cli/` (or `cmd/`).
- Business logic lives in `internal/` packages. **Never put business logic in command files.**
- Construct dependencies once at startup; inject into commands via closures or struct fields.
- Prefer `RunE` (returns error) over `Run` (panics or swallows errors).
- Centralize error handling at the top-level `Execute()` function.

```go
// cmd/raven/main.go -- minimal entry point
package main

import (
    "os"

    "github.com/raven/raven/internal/cli"
)

func main() {
    if err := cli.Execute(); err != nil {
        os.Exit(1)
    }
}
```

```go
// internal/cli/root.go -- centralized error handling
package cli

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
    root := &cobra.Command{
        Use:   "raven",
        Short: "Package codebases into LLM-optimized context",
    }
    root.AddCommand(newGenerateCmd())
    root.AddCommand(newVersionCmd())
    return root
}

func Execute() error {
    cmd := NewRootCmd()
    if err := cmd.Execute(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        return err
    }
    return nil
}
```

## 2 -- Command Design

### Naming and Metadata

- Use clear verbs: `list`, `get`, `create`, `delete`, `sync`, `generate`.
- `Use`: include argument placeholders, e.g. `get <id>`.
- `Short`: one-line, user-facing summary.
- `Long`: include context, caveats, and when to use this command.
- `Example`: 3--6 concrete, copy-paste-ready examples.

### Arguments and Flags

- Use positional args for the primary noun (the thing being acted on).
- Use flags for options and modifiers.
- Validate args with Cobra validators: `cobra.ExactArgs(1)`, `cobra.MaximumNArgs(1)`.
- Error messages should point to `--help` when input is invalid.

```go
func newGenerateCmd() *cobra.Command {
    var opts generateOptions

    cmd := &cobra.Command{
        Use:   "generate [path]",
        Short: "Generate context document from a codebase",
        Long: `Generate an LLM-optimized context document from the target
repository. Discovers files, applies relevance tiers, counts tokens,
and renders output in Markdown or XML format.

If no path is given, uses the current directory.`,
        Example: `  # Generate from current directory
  raven generate

  # Generate from a specific repo with output file
  raven generate /path/to/repo -o context.md

  # Use a specific profile
  raven generate -p minimal

  # JSON output for scripting
  raven generate --output json`,
        Args: cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            root := "."
            if len(args) > 0 {
                root = args[0]
            }
            return runGenerate(cmd.Context(), root, opts)
        },
    }

    cmd.Flags().StringVarP(&opts.OutputFile, "output-file", "o", "", "output file path")
    cmd.Flags().StringVarP(&opts.Profile, "profile", "p", "", "profile name or path")
    cmd.Flags().StringVar(&opts.Format, "format", "markdown", "output format (markdown|xml)")
    cmd.Flags().BoolVar(&opts.JSON, "json", false, "output structured JSON")

    return cmd
}

type generateOptions struct {
    OutputFile string
    Profile    string
    Format     string
    JSON       bool
}
```

## 3 -- Flags and Config Precedence

### Precedence (highest to lowest)

1. Explicit CLI flags
2. Environment variables‚
3. Config file
4. Defaults

Define this precedence in one place and apply it everywhere.

### Rules

- Persistent flags on the root command (e.g. `--verbose`, `--config`).
- Command-specific flags on that command only.
- Validate config early, fail fast with actionable messages.
- Env vars: consistent prefix (`RAVEN_`), documented, treated as untrusted input.
- Support explicit `--config` path flag.

```go
// Root-level persistent flags
func NewRootCmd() *cobra.Command {
    root := &cobra.Command{
        Use: "raven",
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            return initConfig(cmd)
        },
    }
    root.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
    root.PersistentFlags().BoolP("verbose", "v", false, "enable verbose output")
    root.PersistentFlags().StringP("log-level", "l", "info", "log level (debug|info|warn|error)")
    return root
}

// Config loading with precedence
func initConfig(cmd *cobra.Command) error {
    cfg, err := config.Load(config.LoadInput{
        Path:     cfgFile,             // flag
        EnvPrefix: "RAVEN",           // env
        Defaults: config.Defaults(),   // defaults
    })
    if err != nil {
        return fmt.Errorf("load config: %w", err)
    }
    if err := cfg.Validate(); err != nil {
        return fmt.Errorf("invalid config: %w", err)
    }
    return nil
}
```

## 4 -- Errors and Exit Codes

### Output Routing

- Human/machine output goes to **stdout**.
- Errors and diagnostics go to **stderr**.
- Never mix logs or debug output into stdout.

### Error Message Quality

- What failed + what was invalid + what is expected + how to fix.
- No stack traces by default.
- User errors: short message + help hint.
- System errors: actionable message + underlying error.

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Partial success (some files failed) |

```go
func runGenerate(ctx context.Context, root string, opts generateOptions) error {
    // Validate early with helpful messages
    if opts.Format != "markdown" && opts.Format != "xml" {
        return fmt.Errorf(
            "unknown format %q (expected markdown or xml); see --help",
            opts.Format,
        )
    }

    result, err := pipeline.Run(ctx, pipeline.RunInput{
        Root:    root,
        Profile: opts.Profile,
    })
    if err != nil {
        return fmt.Errorf("generate %s: %w", root, err)
    }

    // Output routing: data to stdout
    if opts.JSON {
        return writeJSON(os.Stdout, result)
    }
    return writeText(os.Stdout, result)
}
```

## 5 -- Output Contracts

### Modes

- **Default**: human-friendly text (tables, colors, summaries).
- **Machine mode**: `--json` or `--output json` for structured output.
- **Quiet mode**: `--quiet` suppresses success noise; only errors printed.

### Rules

- Keep JSON schema stable; version if breaking changes needed.
- Define JSON output structs explicitly; never use `map[string]any`.
- Deterministic ordering for reproducible tests and scripts.
- Support piping: no interactive prompts unless `--interactive` is set.

```go
// Explicit JSON output struct -- never map[string]any
type GenerateOutput struct {
    Files      []FileEntry `json:"files"`
    TotalFiles int         `json:"total_files"`
    Tokens     int         `json:"tokens"`
    Budget     int         `json:"budget"`
    Profile    string      `json:"profile"`
}

type FileEntry struct {
    Path   string `json:"path"`
    Tier   int    `json:"tier"`
    Tokens int    `json:"tokens"`
    Size   int64  `json:"size"`
}

func writeJSON(w io.Writer, out GenerateOutput) error {
    enc := json.NewEncoder(w)
    enc.SetIndent("", "  ")
    return enc.Encode(out)
}
```

## 6 -- Shell Completions

- Provide a `completion` subcommand for bash, zsh, fish, and powershell.
- Add custom completion for enum flags and resource names.
- Keep completion functions fast: no network calls, no heavy I/O.

```go
func newCompletionCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "completion [bash|zsh|fish|powershell]",
        Short: "Generate shell completion script",
        Args:  cobra.ExactArgs(1),
        ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
        RunE: func(cmd *cobra.Command, args []string) error {
            switch args[0] {
            case "bash":
                return cmd.Root().GenBashCompletion(os.Stdout)
            case "zsh":
                return cmd.Root().GenZshCompletion(os.Stdout)
            case "fish":
                return cmd.Root().GenFishCompletion(os.Stdout, true)
            case "powershell":
                return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
            default:
                return fmt.Errorf("unsupported shell %q", args[0])
            }
        },
    }
}

// Custom completion for enum flags
cmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
    return []string{"markdown", "xml"}, cobra.ShellCompDirectiveNoFileComp
})
```

## 7 -- Dependency Injection Pattern

Wire dependencies at the root level and pass them into subcommands via closures or a shared app struct.

```go
// App struct holds shared dependencies
type App struct {
    Logger *slog.Logger
    Config *config.Config
}

func NewRootCmd() *cobra.Command {
    app := &App{}

    root := &cobra.Command{
        Use: "raven",
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            app.Logger = initLogger(cmd)
            cfg, err := config.Load(cfgFile)
            if err != nil {
                return err
            }
            app.Config = cfg
            return nil
        },
    }

    root.AddCommand(newGenerateCmd(app))
    root.AddCommand(newVersionCmd())
    return root
}

func newGenerateCmd(app *App) *cobra.Command {
    return &cobra.Command{
        Use:  "generate [path]",
        RunE: func(cmd *cobra.Command, args []string) error {
            svc := pipeline.New(pipeline.Options{
                Logger: app.Logger,
                Config: app.Config,
            })
            return svc.Run(cmd.Context(), args[0])
        },
    }
}
```