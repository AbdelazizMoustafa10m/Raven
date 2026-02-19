package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
)

// minimalValidTOML is a complete raven.toml fixture that passes Validate with
// no errors. Directory and file paths intentionally use non-existent paths so
// that the benchmark does not depend on the host filesystem layout; those
// produce only warnings, not errors.
const minimalValidTOML = `
[project]
name = "bench-project"
language = "go"
tasks_dir = "docs/tasks"
task_state_file = "docs/tasks/task-state.conf"
phases_conf = "docs/tasks/phases.conf"
progress_file = "docs/tasks/PROGRESS.md"
log_dir = "logs"
prompt_dir = "prompts"
branch_template = "phase/{phase_id}-{slug}"
verification_commands = ["go build ./..."]

[agents.claude]
command = "claude"
model = "claude-opus-4-6"
effort = "high"
prompt_template = "prompts/implement.md"
allowed_tools = "Edit,Write,Read"

[review]
extensions = "\\.go$"
risk_patterns = "^(cmd/|internal/)"
`

// writeBenchConfig writes minimalValidTOML to a temp file and returns the path.
// The file is created once per benchmark; b.TempDir() cleans up automatically.
func writeBenchConfig(b *testing.B) string {
	b.Helper()
	dir := b.TempDir()
	path := filepath.Join(dir, "raven.toml")
	if err := os.WriteFile(path, []byte(minimalValidTOML), 0o644); err != nil {
		b.Fatalf("writing bench config: %v", err)
	}
	return path
}

// BenchmarkLoadFromFile measures the cost of parsing a TOML config file from
// disk, including file I/O and TOML decoding.
func BenchmarkLoadFromFile(b *testing.B) {
	path := writeBenchConfig(b)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		cfg, _, err := LoadFromFile(path)
		if err != nil {
			b.Fatalf("LoadFromFile: %v", err)
		}
		_ = cfg
	}
}

// BenchmarkValidate measures the cost of validating a fully-populated Config
// against TOML metadata. Setup is excluded from the measured region.
func BenchmarkValidate(b *testing.B) {
	path := writeBenchConfig(b)
	cfg, md, err := LoadFromFile(path)
	if err != nil {
		b.Fatalf("LoadFromFile: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		result := Validate(cfg, &md)
		_ = result
	}
}

// BenchmarkValidate_NilMeta measures Validate when no TOML metadata is
// available (the unknown-key detection path is skipped).
func BenchmarkValidate_NilMeta(b *testing.B) {
	cfg := &Config{
		Project: ProjectConfig{
			Name:     "bench-project",
			Language: "go",
		},
		Agents: map[string]AgentConfig{
			"claude": {Command: "claude", Model: "claude-opus-4-6", Effort: "high"},
		},
		Review:    ReviewConfig{Extensions: `\.go$`, RiskPatterns: `^(cmd/|internal/)`},
		Workflows: map[string]WorkflowConfig{},
	}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		result := Validate(cfg, nil)
		_ = result
	}
}

// BenchmarkNewDefaults measures the cost of constructing a default Config.
func BenchmarkNewDefaults(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		cfg := NewDefaults()
		_ = cfg
	}
}

// BenchmarkLoadAndValidate measures the end-to-end hot path: loading a config
// file from disk and immediately validating it.
func BenchmarkLoadAndValidate(b *testing.B) {
	path := writeBenchConfig(b)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		cfg, md, err := LoadFromFile(path)
		if err != nil {
			b.Fatalf("LoadFromFile: %v", err)
		}
		result := Validate(cfg, &md)
		_ = result
	}
}

// BenchmarkValidate_ManyAgents measures Validate when the config contains a
// large number of agent entries, stressing the per-agent validation loop.
func BenchmarkValidate_ManyAgents(b *testing.B) {
	cfg := &Config{
		Project: ProjectConfig{Name: "bench-project", Language: "go"},
		Agents:  make(map[string]AgentConfig, 20),
		Review:  ReviewConfig{},
	}
	for i := 0; i < 20; i++ {
		name := string(rune('a'+i)) + "-agent"
		cfg.Agents[name] = AgentConfig{
			Command: "claude",
			Model:   "claude-opus-4-6",
			Effort:  "high",
		}
	}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		result := Validate(cfg, nil)
		_ = result
	}
}

// BenchmarkValidate_ManyWorkflows measures Validate when the config contains
// multiple workflow definitions with transitions, stressing the workflow
// validation and step-set construction paths.
func BenchmarkValidate_ManyWorkflows(b *testing.B) {
	cfg := &Config{
		Project: ProjectConfig{Name: "bench-project", Language: "go"},
		Agents:  map[string]AgentConfig{},
		Review:  ReviewConfig{},
		Workflows: map[string]WorkflowConfig{
			"wf1": {
				Steps: []string{"implement", "review", "fix", "pr"},
				Transitions: map[string]map[string]string{
					"implement": {"success": "review", "failure": "implement"},
					"review":    {"success": "fix", "failure": "review"},
					"fix":       {"success": "pr"},
				},
			},
			"wf2": {
				Steps: []string{"build", "test", "deploy"},
				Transitions: map[string]map[string]string{
					"build": {"success": "test", "failure": "build"},
					"test":  {"success": "deploy"},
				},
			},
			"wf3": {
				Steps: []string{"lint", "vet", "test"},
				Transitions: map[string]map[string]string{
					"lint": {"success": "vet"},
					"vet":  {"success": "test"},
				},
			},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		result := Validate(cfg, nil)
		_ = result
	}
}

// BenchmarkDecodeAndValidate measures the cost of decoding raw TOML bytes in
// memory and validating the result, isolating the TOML parse and validation
// costs from disk I/O.
func BenchmarkDecodeAndValidate(b *testing.B) {
	raw := []byte(minimalValidTOML)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var cfg Config
		md, err := toml.Decode(string(raw), &cfg)
		if err != nil {
			b.Fatalf("toml.Decode: %v", err)
		}
		result := Validate(&cfg, &md)
		_ = result
	}
}
