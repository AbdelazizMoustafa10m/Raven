---
name: go-testing-cli
description: Testing patterns for Go CLI tools - command tests, golden tests, table-driven tests, filesystem tests, benchmarks, and fuzz tests. Use when writing or reviewing tests for Raven.
---

# Go CLI Testing Patterns

## 1 -- Command Tests

Execute CLI commands with arguments, capture stdout/stderr/exit code, and verify behavior.

### Rules

- No network calls unless explicitly mocked.
- Test flag validation and error messages.
- Test `--help` output exists and is well-formatted.
- Use `bytes.Buffer` for capturing output.
- Test both success and error paths.

```go
package cli_test

import (
    "bytes"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/raven/raven/internal/cli"
)

func TestGenerateCommand_Success(t *testing.T) {
    stdout := new(bytes.Buffer)
    stderr := new(bytes.Buffer)

    cmd := cli.NewRootCmd()
    cmd.SetOut(stdout)
    cmd.SetErr(stderr)
    cmd.SetArgs([]string{"generate", "testdata/sample-repo"})

    err := cmd.Execute()
    require.NoError(t, err)

    assert.Contains(t, stdout.String(), "files discovered")
    assert.Empty(t, stderr.String())
}

func TestGenerateCommand_InvalidFlag(t *testing.T) {
    stderr := new(bytes.Buffer)

    cmd := cli.NewRootCmd()
    cmd.SetErr(stderr)
    cmd.SetArgs([]string{"generate", "--format", "invalid"})

    err := cmd.Execute()
    require.Error(t, err)
    assert.Contains(t, err.Error(), "unknown format")
}

func TestGenerateCommand_HelpOutput(t *testing.T) {
    stdout := new(bytes.Buffer)

    cmd := cli.NewRootCmd()
    cmd.SetOut(stdout)
    cmd.SetArgs([]string{"generate", "--help"})

    err := cmd.Execute()
    require.NoError(t, err)

    output := stdout.String()
    assert.Contains(t, output, "Usage:")
    assert.Contains(t, output, "Examples:")
    assert.Contains(t, output, "--output-file")
    assert.Contains(t, output, "--profile")
}
```

## 2 -- Table-Driven Tests

### Rules

- Use for multiple input/output combinations.
- Name test cases so failures are immediately diagnosable.
- Split success and error paths into separate test functions.
- Use `testify/require` for fatal assertions, `testify/assert` for soft checks.
- Mark independent subtests with `t.Parallel()`.

```go
func TestTokenizer_Count_Success(t *testing.T) {
    tok, err := tokenizer.New()
    require.NoError(t, err)

    tests := []struct {
        name    string
        content string
        want    int
    }{
        {name: "empty string", content: "", want: 0},
        {name: "single token", content: "hello", want: 1},
        {name: "go function", content: "func main() {\n\tfmt.Println(\"hi\")\n}", want: 12},
        {name: "unicode", content: "cafe\u0301", want: 3},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            got, err := tok.Count(tt.content)
            require.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}

func TestTokenizer_Count_Errors(t *testing.T) {
    tests := []struct {
        name      string
        content   string
        wantErr   string
    }{
        {name: "nil tokenizer panics recovered", content: "x", wantErr: "tokenizer not initialized"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            var tok *tokenizer.Tokenizer // nil
            _, err := tok.Count(tt.content)
            require.Error(t, err)
            assert.Contains(t, err.Error(), tt.wantErr)
        })
    }
}
```

## 3 -- Golden Tests

Use golden tests for JSON schema verification and stable text output.

### Rules

- Provide `-update` flag to regenerate golden files.
- Normalize timestamps, hashes, and absolute paths before comparison.
- Ensure deterministic ordering in output.
- Add golden tests when output format changes (catches regressions).
- Store golden files in `testdata/expected-output/`.

```go
package output_test

import (
    "flag"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/raven/raven/internal/output"
)

var update = flag.Bool("update", false, "update golden files")

func TestMarkdownRenderer_Golden(t *testing.T) {
    tests := []struct {
        name   string
        input  output.RenderInput
        golden string
    }{
        {
            name:   "default profile",
            input:  loadTestInput(t, "default"),
            golden: "testdata/expected-output/default.md",
        },
        {
            name:   "minimal profile",
            input:  loadTestInput(t, "minimal"),
            golden: "testdata/expected-output/minimal.md",
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            actual, err := output.RenderMarkdown(tt.input)
            require.NoError(t, err)

            normalized := normalize(string(actual))

            if *update {
                err := os.WriteFile(tt.golden, []byte(normalized), 0644)
                require.NoError(t, err, "failed to update golden file")
                return
            }

            expected, err := os.ReadFile(tt.golden)
            require.NoError(t, err, "golden file missing; run with -update to create")

            assert.Equal(t, normalize(string(expected)), normalized)
        })
    }
}

// normalize removes non-deterministic content for stable comparisons.
func normalize(s string) string {
    // Replace timestamps with placeholder
    s = timestampRe.ReplaceAllString(s, "<TIMESTAMP>")
    // Normalize path separators
    s = strings.ReplaceAll(s, "\\", "/")
    // Trim trailing whitespace per line
    lines := strings.Split(s, "\n")
    for i, line := range lines {
        lines[i] = strings.TrimRight(line, " \t")
    }
    return strings.Join(lines, "\n")
}
```

## 4 -- Filesystem Tests

### Rules

- Use `t.TempDir()` for isolated directories (auto-cleaned).
- Use `t.Helper()` in all setup helper functions.
- Create realistic test structures that mirror actual repo layouts.
- Test `.gitignore` behavior with real ignore files.

```go
func setupTestRepo(t *testing.T) string {
    t.Helper()
    dir := t.TempDir()

    // Source files
    createFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
    createFile(t, dir, "lib/util.go", "package lib\n\nfunc Helper() {}\n")
    createFile(t, dir, "README.md", "# Test Project\n")

    // Ignored paths
    createFile(t, dir, ".gitignore", "*.log\n/dist/\nnode_modules/\n")
    createFile(t, dir, "dist/bundle.js", "compiled code")
    createFile(t, dir, "debug.log", "log content")

    // Binary file
    createFile(t, dir, "icon.png", string([]byte{0x89, 0x50, 0x4E, 0x47}))

    return dir
}

func createFile(t *testing.T, base, rel, content string) {
    t.Helper()
    path := filepath.Join(base, rel)
    require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
    require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func createDir(t *testing.T, base, rel string) {
    t.Helper()
    require.NoError(t, os.MkdirAll(filepath.Join(base, rel), 0755))
}

func TestWalker_IgnoresGitignorePatterns(t *testing.T) {
    dir := setupTestRepo(t)

    w, err := discovery.New(discovery.Options{Root: dir})
    require.NoError(t, err)

    files, err := w.Walk(context.Background())
    require.NoError(t, err)

    paths := extractPaths(files)
    assert.Contains(t, paths, "main.go")
    assert.Contains(t, paths, "lib/util.go")
    assert.Contains(t, paths, "README.md")
    assert.NotContains(t, paths, "dist/bundle.js")
    assert.NotContains(t, paths, "debug.log")
}

func extractPaths(files []discovery.FileDescriptor) []string {
    paths := make([]string, len(files))
    for i, f := range files {
        paths[i] = f.RelPath
    }
    return paths
}
```

## 5 -- Benchmark Tests

Add benchmarks for performance-critical paths: file walking, token counting, output rendering.

```go
func BenchmarkWalker_LargeRepo(b *testing.B) {
    dir := setupLargeBenchRepo(b) // creates 1000+ files
    w, err := discovery.New(discovery.Options{Root: dir})
    require.NoError(b, err)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := w.Walk(context.Background())
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkTokenizer_Count(b *testing.B) {
    tok, _ := tokenizer.New()
    content := strings.Repeat("func main() { fmt.Println(\"hello\") }\n", 1000)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        tok.Count(content)
    }
}

func setupLargeBenchRepo(b *testing.B) string {
    b.Helper()
    dir := b.TempDir()
    for i := 0; i < 1000; i++ {
        path := filepath.Join(dir, fmt.Sprintf("pkg%d/file.go", i))
        os.MkdirAll(filepath.Dir(path), 0755)
        os.WriteFile(path, []byte(fmt.Sprintf("package pkg%d\n", i)), 0644)
    }
    return dir
}
```

## 6 -- Fuzz Tests

Add fuzz tests for security-critical code: secret redaction, config parsing, input validation.

```go
func FuzzRedactor_Redact(f *testing.F) {
    // Seed corpus
    f.Add("normal text without secrets")
    f.Add("aws_secret_access_key = AKIAIOSFODNN7EXAMPLE")
    f.Add("password: hunter2\ntoken: ghp_abc123")
    f.Add(strings.Repeat("A", 10000))
    f.Add("")

    redactor := security.NewRedactor(security.DefaultPatterns())

    f.Fuzz(func(t *testing.T, input string) {
        output := redactor.Redact(input)
        // Invariants that must always hold:
        // 1. Output is never longer than input + redaction markers
        // 2. No panic
        // 3. Known patterns are redacted
        if len(output) > len(input)*2+100 {
            t.Errorf("output unexpectedly large: input=%d output=%d", len(input), len(output))
        }
    })
}
```

## 7 -- Test Execution Cheatsheet

```bash
go test ./...                              # All tests
go test ./internal/cli/...                 # Specific package
go test -v -run TestGenerate ./...         # Specific test
go test -race ./...                        # Race detector
go test -count=1 ./...                     # No cache
go test -run TestGolden -update ./...      # Update golden files
go test -bench=. ./...                     # Benchmarks
go test -bench=BenchmarkWalker -benchmem   # Benchmark with allocs
go test -fuzz=FuzzRedactor ./internal/security/ # Fuzz test
```