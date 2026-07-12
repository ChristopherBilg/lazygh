package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"  Error  ", slog.LevelError},
		{"", slog.LevelWarn},
		{"nonsense", slog.LevelWarn},
	}
	for _, tt := range tests {
		if got := parseLevel(tt.in); got != tt.want {
			t.Errorf("parseLevel(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestLogPathXDG(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/xdgstate")
	got, err := logPath()
	if err != nil {
		t.Fatalf("logPath() error: %v", err)
	}
	if want := filepath.Join("/tmp/xdgstate", "lazygh", "app.log"); got != want {
		t.Fatalf("logPath() = %q, want %q", got, want)
	}
}

func TestLogPathHomeFallback(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/tmp/fakehome")
	got, err := logPath()
	if err != nil {
		t.Fatalf("logPath() error: %v", err)
	}
	if want := filepath.Join("/tmp/fakehome", ".local", "state", "lazygh", "app.log"); got != want {
		t.Fatalf("logPath() = %q, want %q", got, want)
	}
}

func TestInitCreatesAndWrites(t *testing.T) {
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })

	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	t.Setenv("LAZYGH_LOG_LEVEL", "info")

	closeFn, err := Init()
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	t.Cleanup(func() { _ = closeFn() })

	slog.Info("hello", "k", "v")

	path := filepath.Join(dir, "lazygh", "app.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if !strings.Contains(string(data), "hello") || !strings.Contains(string(data), "k=v") {
		t.Fatalf("log file missing expected entry, got: %q", data)
	}
}

func TestInitFallbackWhenDirUnwritable(t *testing.T) {
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })

	dir := t.TempDir()
	// Create a *file* where the "lazygh" dir must go, so MkdirAll fails.
	if err := os.WriteFile(filepath.Join(dir, "lazygh"), []byte("x"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Setenv("XDG_STATE_HOME", dir)

	closeFn, err := Init()
	if err == nil {
		t.Fatal("expected Init() to error when the log dir can't be created")
	}
	if closeFn == nil {
		t.Fatal("expected a non-nil closeFn even on failure")
	}
	if cerr := closeFn(); cerr != nil {
		t.Fatalf("no-op closeFn returned error: %v", cerr)
	}
	slog.Warn("still works") // must not panic on the discard logger
}

func TestInitFallbackWhenFileUnwritable(t *testing.T) {
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })

	dir := t.TempDir()
	// Put a *directory* at the app.log path: MkdirAll(parent) succeeds, but
	// OpenFile(path, ...O_WRONLY...) then fails because the target is a dir.
	if err := os.MkdirAll(filepath.Join(dir, "lazygh", "app.log"), 0o700); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Setenv("XDG_STATE_HOME", dir)

	closeFn, err := Init()
	if err == nil {
		t.Fatal("expected Init() to error when the log file can't be opened")
	}
	if closeFn == nil {
		t.Fatal("expected a non-nil closeFn even on failure")
	}
	if cerr := closeFn(); cerr != nil {
		t.Fatalf("no-op closeFn returned error: %v", cerr)
	}
	slog.Warn("still works") // discard logger must not panic
}
