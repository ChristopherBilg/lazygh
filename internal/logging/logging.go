// Package logging configures a process-wide, TUI-safe structured logger that
// writes to a file in the XDG state directory. Because the TUI owns the
// terminal, logs must never go to stdout/stderr during normal operation.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// levelEnvVar overrides the default log level when set.
const levelEnvVar = "LAZYGH_LOG_LEVEL"

// defaultLevel is the quiet default used when levelEnvVar is unset/unrecognized.
const defaultLevel = slog.LevelWarn

// Init points slog's default logger at the app log file under the XDG state
// directory, creating the directory if needed. The level comes from
// LAZYGH_LOG_LEVEL (debug|info|warn|error), defaulting to warn.
//
// Init ALWAYS installs a working default logger and ALWAYS returns a non-nil
// closeFn, so callers can unconditionally `defer closeFn()`. If the file cannot
// be opened it installs a discard logger and returns the error; the app still
// runs without ever writing to the terminal.
func Init() (closeFn func() error, err error) {
	noop := func() error { return nil }
	level := parseLevel(os.Getenv(levelEnvVar))

	path, err := logPath()
	if err != nil {
		slog.SetDefault(discardLogger(level))
		return noop, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		slog.SetDefault(discardLogger(level))
		return noop, fmt.Errorf("create log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		slog.SetDefault(discardLogger(level))
		return noop, fmt.Errorf("open log file: %w", err)
	}

	handler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: level, AddSource: true})
	slog.SetDefault(slog.New(handler))
	return f.Close, nil
}

// discardLogger drops all output; the fallback when the log file can't be opened
// so the app never writes to the terminal.
func discardLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: level}))
}

// logPath returns $XDG_STATE_HOME/lazygh/app.log, or ~/.local/state/lazygh/app.log
// when XDG_STATE_HOME is unset.
func logPath() (string, error) {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "lazygh", "app.log"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", "lazygh", "app.log"), nil
}

// parseLevel maps a level string (case-insensitive, trimmed) to a slog.Level,
// returning the quiet default for empty/unrecognized values.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return defaultLevel
	}
}
