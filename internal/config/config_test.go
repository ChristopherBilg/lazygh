package config

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain silences slog by default so passing runs stay quiet; tests that
// assert on log output install their own buffer-backed logger.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

// writeConfig writes content to <dir>/lazygh/config.yml, creating the dir.
func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	cfgDir := filepath.Join(dir, "lazygh")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yml"), []byte(content), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

// captureLogs installs a buffer-backed slog logger at DEBUG and restores the
// previous default on cleanup, returning the buffer for assertions.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return &buf
}

func TestConfigPathXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdgcfg")
	got, err := configPath()
	if err != nil {
		t.Fatalf("configPath() error: %v", err)
	}
	if want := filepath.Join("/tmp/xdgcfg", "lazygh", "config.yml"); got != want {
		t.Fatalf("configPath() = %q, want %q", got, want)
	}
}

func TestConfigPathHomeFallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/tmp/fakehome")
	got, err := configPath()
	if err != nil {
		t.Fatalf("configPath() error: %v", err)
	}
	if want := filepath.Join("/tmp/fakehome", ".config", "lazygh", "config.yml"); got != want {
		t.Fatalf("configPath() = %q, want %q", got, want)
	}
}

func TestLoadDefaultsWhenAbsent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // empty dir: no config.yml
	if got := Load(); got != Default() {
		t.Fatalf("Load() = %+v, want Default() %+v", got, Default())
	}
}

func TestLoadAppliesValues(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeConfig(t, dir, "github:\n  rest_timeout: 5s\n  subprocess_timeout: 1m\n  repo_page_size: 10\n")

	got := Load()
	if got.GitHub.RESTTimeout != 5*time.Second {
		t.Errorf("RESTTimeout = %v, want 5s", got.GitHub.RESTTimeout)
	}
	if got.GitHub.SubprocessTimeout != time.Minute {
		t.Errorf("SubprocessTimeout = %v, want 1m", got.GitHub.SubprocessTimeout)
	}
	if got.GitHub.RepoPageSize != 10 {
		t.Errorf("RepoPageSize = %d, want 10", got.GitHub.RepoPageSize)
	}
}

func TestLoadPartialFileDefaultsRest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeConfig(t, dir, "github:\n  repo_page_size: 25\n")

	got := Load()
	if got.GitHub.RepoPageSize != 25 {
		t.Errorf("RepoPageSize = %d, want 25", got.GitHub.RepoPageSize)
	}
	if got.GitHub.RESTTimeout != Default().GitHub.RESTTimeout {
		t.Errorf("RESTTimeout = %v, want default %v", got.GitHub.RESTTimeout, Default().GitHub.RESTTimeout)
	}
	if got.GitHub.SubprocessTimeout != Default().GitHub.SubprocessTimeout {
		t.Errorf("SubprocessTimeout = %v, want default %v", got.GitHub.SubprocessTimeout, Default().GitHub.SubprocessTimeout)
	}
}

func TestLoadMalformedLogsErrorAndDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	buf := captureLogs(t)
	// A sequence where a mapping is expected: yaml.Unmarshal into raw errors.
	writeConfig(t, dir, "github:\n  - 1\n  - 2\n")

	if got := Load(); got != Default() {
		t.Fatalf("Load() = %+v, want Default() on malformed file", got)
	}
	if out := buf.String(); !strings.Contains(out, "level=ERROR") || !strings.Contains(out, "malformed file") {
		t.Fatalf("expected an ERROR 'malformed file' log; got: %s", out)
	}
}

func TestLoadInvalidFieldsWarnAndDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	buf := captureLogs(t)
	// rest_timeout unparseable; repo_page_size out of range; subprocess_timeout valid.
	writeConfig(t, dir, "github:\n  rest_timeout: nope\n  subprocess_timeout: 2s\n  repo_page_size: 999\n")

	got := Load()
	if got.GitHub.RESTTimeout != Default().GitHub.RESTTimeout {
		t.Errorf("RESTTimeout = %v, want default (invalid value ignored)", got.GitHub.RESTTimeout)
	}
	if got.GitHub.RepoPageSize != Default().GitHub.RepoPageSize {
		t.Errorf("RepoPageSize = %d, want default (out of range ignored)", got.GitHub.RepoPageSize)
	}
	if got.GitHub.SubprocessTimeout != 2*time.Second {
		t.Errorf("SubprocessTimeout = %v, want 2s (valid sibling applied)", got.GitHub.SubprocessTimeout)
	}
	out := buf.String()
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "rest_timeout") {
		t.Errorf("expected a WARN for rest_timeout; got: %s", out)
	}
	if !strings.Contains(out, "repo_page_size") {
		t.Errorf("expected a WARN for repo_page_size; got: %s", out)
	}
}
