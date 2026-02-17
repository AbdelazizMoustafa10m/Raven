// Package logging provides Raven's logging infrastructure built on charmbracelet/log.
//
// It wraps charmbracelet/log to provide a centralized logger factory with component
// prefixes, level configuration, and stderr-only output. All log output goes to
// stderr; stdout is reserved for structured output (JSON, tables, etc.).
//
// Usage:
//
//	// During CLI initialization (PersistentPreRun):
//	logging.Setup(verbose, quiet, jsonFormat)
//
//	// In each package:
//	var logger = logging.New("config")
//	logger.Info("loading file", "path", "raven.toml")
//
// Setup must be called before New to ensure child loggers inherit the correct
// level and formatter settings. The charmbracelet/log library creates child
// loggers by copying state at creation time; later changes to the default
// logger do not propagate to existing children.
package logging

import (
	"io"
	"os"

	"github.com/charmbracelet/log"
)

// Level aliases for charmbracelet/log levels.
// Re-exported so consumers do not need to import charmbracelet/log directly.
const (
	LevelDebug = log.DebugLevel
	LevelInfo  = log.InfoLevel
	LevelWarn  = log.WarnLevel
	LevelError = log.ErrorLevel
	LevelFatal = log.FatalLevel
)

// Setup configures the global logging defaults. Call once during CLI initialization.
//
// Parameters:
//   - verbose: sets level to Debug (shows all messages)
//   - quiet: sets level to Error (hides Info and Warn messages)
//   - jsonFormat: switches to JSON formatter (NDJSON, suitable for CI/log aggregation)
//
// If both verbose and quiet are set, quiet wins. This is intentional: in scripted
// environments, --quiet should always suppress noise regardless of other flags.
//
// All loggers write to stderr to keep stdout clean for structured output.
func Setup(verbose, quiet, jsonFormat bool) {
	level := log.InfoLevel
	if verbose {
		level = log.DebugLevel
	}
	if quiet {
		level = log.ErrorLevel
	}

	log.SetLevel(level)
	log.SetOutput(os.Stderr)

	if jsonFormat {
		log.SetFormatter(log.JSONFormatter)
	} else {
		log.SetFormatter(log.TextFormatter)
	}
}

// New creates a logger with the given component prefix.
//
// The returned logger inherits global level and output settings from the
// default logger at creation time. Call Setup before New to ensure the
// correct configuration is inherited.
//
// An empty component string produces a logger without a prefix.
//
// Example:
//
//	logger := logging.New("config")
//	logger.Info("loading raven.toml")
//	// Output: INFO <config> loading raven.toml
func New(component string) *log.Logger {
	return log.WithPrefix(component)
}

// SetOutput overrides the output writer for the default logger.
//
// This is primarily useful for testing, where output can be captured
// with a bytes.Buffer. Remember to restore the original output using
// t.Cleanup.
func SetOutput(w io.Writer) {
	log.SetOutput(w)
}
