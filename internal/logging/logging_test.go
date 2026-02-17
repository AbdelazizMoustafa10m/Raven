package logging

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetDefaults restores the default logger to a known state between tests.
// This is necessary because charmbracelet/log uses global state.
func resetDefaults(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		log.SetLevel(log.InfoLevel)
		log.SetOutput(os.Stderr)
		log.SetFormatter(log.TextFormatter)
	})
}

func TestSetup_DefaultLevel(t *testing.T) {
	resetDefaults(t)

	Setup(false, false, false)

	assert.Equal(t, log.InfoLevel, log.GetLevel(), "default level should be Info")
}

func TestSetup_VerboseSetsDebug(t *testing.T) {
	resetDefaults(t)

	Setup(true, false, false)

	assert.Equal(t, log.DebugLevel, log.GetLevel(), "verbose should set level to Debug")
}

func TestSetup_QuietSetsError(t *testing.T) {
	resetDefaults(t)

	Setup(false, true, false)

	assert.Equal(t, log.ErrorLevel, log.GetLevel(), "quiet should set level to Error")
}

func TestSetup_QuietWinsOverVerbose(t *testing.T) {
	resetDefaults(t)

	Setup(true, true, false)

	assert.Equal(t, log.ErrorLevel, log.GetLevel(),
		"when both verbose and quiet are set, quiet should win")
}

func TestSetup_OutputToStderr(t *testing.T) {
	resetDefaults(t)

	var buf bytes.Buffer
	log.SetOutput(&buf)

	Setup(false, false, false)

	// After Setup, output should be stderr. We verify by logging to the
	// default logger and confirming buf (the previous writer) gets nothing new.
	buf.Reset()
	log.Info("test message")

	assert.Empty(t, buf.String(),
		"after Setup, output should go to stderr, not the previous writer")
}

func TestSetup_JSONFormatter(t *testing.T) {
	resetDefaults(t)

	var buf bytes.Buffer
	Setup(false, false, true)
	SetOutput(&buf)

	log.Info("json test")

	output := buf.String()
	require.NotEmpty(t, output, "should produce output")

	// Validate it's parseable JSON
	var parsed map[string]any
	err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed)
	require.NoError(t, err, "JSON formatter should produce valid JSON: %s", output)

	assert.Equal(t, "info", parsed["level"], "should contain level field")
	assert.Equal(t, "json test", parsed["msg"], "should contain message field")
}

func TestSetup_TextFormatterResetsJSON(t *testing.T) {
	resetDefaults(t)

	var buf bytes.Buffer

	// First enable JSON
	Setup(false, false, true)
	SetOutput(&buf)
	log.Info("json mode")

	jsonOutput := buf.String()
	require.NotEmpty(t, jsonOutput)

	// Verify it was JSON
	var parsed map[string]any
	err := json.Unmarshal([]byte(strings.TrimSpace(jsonOutput)), &parsed)
	require.NoError(t, err, "should be valid JSON")

	// Now switch back to text
	buf.Reset()
	Setup(false, false, false)
	SetOutput(&buf)
	log.Info("text mode")

	textOutput := buf.String()
	require.NotEmpty(t, textOutput)

	// Should not be valid JSON anymore
	err = json.Unmarshal([]byte(strings.TrimSpace(textOutput)), &parsed)
	assert.Error(t, err, "text formatter output should not be valid JSON")
}

func TestNew_WithComponent(t *testing.T) {
	resetDefaults(t)

	var buf bytes.Buffer
	Setup(false, false, true)
	SetOutput(&buf)

	logger := New("config")
	require.NotNil(t, logger)

	logger.Info("loading file", "path", "raven.toml")

	output := buf.String()
	require.NotEmpty(t, output, "logger should produce output")

	var parsed map[string]any
	err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed)
	require.NoError(t, err, "should produce valid JSON: %s", output)

	assert.Equal(t, "config", parsed["prefix"], "should contain the component prefix")
	assert.Equal(t, "loading file", parsed["msg"], "should contain the message")
	assert.Equal(t, "raven.toml", parsed["path"], "should contain structured field")
}

func TestNew_EmptyComponent(t *testing.T) {
	resetDefaults(t)

	var buf bytes.Buffer
	Setup(false, false, true)
	SetOutput(&buf)

	logger := New("")
	require.NotNil(t, logger, "empty component should not cause a crash")

	logger.Info("no prefix")

	output := buf.String()
	require.NotEmpty(t, output)

	var parsed map[string]any
	err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed)
	require.NoError(t, err, "should produce valid JSON: %s", output)

	_, hasPrefix := parsed["prefix"]
	assert.False(t, hasPrefix, "empty component should not produce a prefix field")
}

func TestNew_MultipleLoggers(t *testing.T) {
	resetDefaults(t)

	var buf bytes.Buffer
	Setup(false, false, true)
	SetOutput(&buf)

	loggerA := New("agent")
	loggerB := New("config")

	loggerA.Info("agent message")
	loggerB.Info("config message")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 2, "should have two log lines")

	var first, second map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &first))
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &second))

	assert.Equal(t, "agent", first["prefix"])
	assert.Equal(t, "config", second["prefix"])
}

func TestSetup_LevelFiltering(t *testing.T) {
	tests := []struct {
		name       string
		verbose    bool
		quiet      bool
		logLevel   string
		shouldShow bool
	}{
		{
			name:       "debug hidden at info level",
			verbose:    false,
			quiet:      false,
			logLevel:   "debug",
			shouldShow: false,
		},
		{
			name:       "info visible at info level",
			verbose:    false,
			quiet:      false,
			logLevel:   "info",
			shouldShow: true,
		},
		{
			name:       "warn visible at info level",
			verbose:    false,
			quiet:      false,
			logLevel:   "warn",
			shouldShow: true,
		},
		{
			name:       "error visible at info level",
			verbose:    false,
			quiet:      false,
			logLevel:   "error",
			shouldShow: true,
		},
		{
			name:       "debug visible at debug level",
			verbose:    true,
			quiet:      false,
			logLevel:   "debug",
			shouldShow: true,
		},
		{
			name:       "info hidden at error level",
			verbose:    false,
			quiet:      true,
			logLevel:   "info",
			shouldShow: false,
		},
		{
			name:       "warn hidden at error level",
			verbose:    false,
			quiet:      true,
			logLevel:   "warn",
			shouldShow: false,
		},
		{
			name:       "error visible at error level",
			verbose:    false,
			quiet:      true,
			logLevel:   "error",
			shouldShow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetDefaults(t)

			var buf bytes.Buffer
			Setup(tt.verbose, tt.quiet, false)
			SetOutput(&buf)

			switch tt.logLevel {
			case "debug":
				log.Debug("test message")
			case "info":
				log.Info("test message")
			case "warn":
				log.Warn("test message")
			case "error":
				log.Error("test message")
			}

			if tt.shouldShow {
				assert.NotEmpty(t, buf.String(), "message should be visible")
			} else {
				assert.Empty(t, buf.String(), "message should be hidden")
			}
		})
	}
}

func TestNew_LoggerRespectsLevel(t *testing.T) {
	resetDefaults(t)

	var buf bytes.Buffer
	Setup(false, false, false) // Info level
	SetOutput(&buf)

	logger := New("test")

	logger.Debug("should be hidden")
	assert.Empty(t, buf.String(), "debug should be hidden at info level")

	logger.Info("should be visible")
	assert.NotEmpty(t, buf.String(), "info should be visible at info level")
}

func TestSetOutput(t *testing.T) {
	resetDefaults(t)

	var buf bytes.Buffer
	Setup(false, false, false)
	SetOutput(&buf)

	log.Info("captured message")

	assert.NotEmpty(t, buf.String(), "SetOutput should redirect log output to buffer")
	assert.Contains(t, buf.String(), "captured message")
}

func TestNoStdoutOutput(t *testing.T) {
	resetDefaults(t)

	// Save and replace stdout with a pipe we can read from
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	t.Cleanup(func() {
		os.Stdout = origStdout
	})

	Setup(true, false, false) // Debug level to get maximum output

	log.Debug("debug message")
	log.Info("info message")
	log.Warn("warn message")
	log.Error("error message")

	// Close the write end so we can read
	w.Close()

	var stdoutBuf bytes.Buffer
	_, err = stdoutBuf.ReadFrom(r)
	require.NoError(t, err)

	assert.Empty(t, stdoutBuf.String(),
		"no log output should go to stdout; got: %q", stdoutBuf.String())
}

// syncBuffer is a thread-safe wrapper around bytes.Buffer.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *syncBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *syncBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

func TestConcurrentLogging(t *testing.T) {
	resetDefaults(t)

	var buf syncBuffer
	Setup(false, false, true) // JSON for easy line counting
	SetOutput(&buf)

	const goroutines = 10
	const messagesPerGoroutine = 5

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			logger := New("worker")
			for j := 0; j < messagesPerGoroutine; j++ {
				logger.Info("concurrent message", "worker", id, "msg", j)
			}
		}(i)
	}

	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	expectedLines := goroutines * messagesPerGoroutine
	assert.Equal(t, expectedLines, len(lines),
		"should have %d log lines from concurrent goroutines", expectedLines)

	// Verify each line is valid JSON
	for i, line := range lines {
		var parsed map[string]any
		err := json.Unmarshal([]byte(line), &parsed)
		assert.NoError(t, err, "line %d should be valid JSON: %s", i, line)
	}
}

func TestLevelConstants(t *testing.T) {
	// Verify our re-exported constants match the library's constants.
	assert.Equal(t, log.DebugLevel, LevelDebug)
	assert.Equal(t, log.InfoLevel, LevelInfo)
	assert.Equal(t, log.WarnLevel, LevelWarn)
	assert.Equal(t, log.ErrorLevel, LevelError)
	assert.Equal(t, log.FatalLevel, LevelFatal)
}

func TestSetup_LevelOrder(t *testing.T) {
	// Verify the level ordering is correct (lower = more verbose).
	assert.Less(t, int(LevelDebug), int(LevelInfo))
	assert.Less(t, int(LevelInfo), int(LevelWarn))
	assert.Less(t, int(LevelWarn), int(LevelError))
	assert.Less(t, int(LevelError), int(LevelFatal))
}
