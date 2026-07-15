package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

func TestLoadWritesDefaultConfigWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir) // empty dir: no config.yml yet
	path := filepath.Join(dir, "lazygh", "config.yml")

	if got := Load(); got != Default() {
		t.Fatalf("Load() = %+v, want Default() %+v", got, Default())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected a default config to be written at %s: %v", path, err)
	}
	content := string(data)
	// Every key is commented out (so the written file is equivalent to "no file"),
	// and each commented example shows the current default value — asserting the
	// values guards against the template drifting from Default().
	def := Default()
	for _, want := range []string{
		fmt.Sprintf("# rest_timeout: %s", def.GitHub.RESTTimeout),
		fmt.Sprintf("# subprocess_timeout: %s", def.GitHub.SubprocessTimeout),
		fmt.Sprintf("# repo_page_size: %d", def.GitHub.RepoPageSize),
	} {
		if !strings.Contains(content, want) {
			t.Errorf("written template missing commented default %q; got:\n%s", want, content)
		}
	}
	// Loading again reads the freshly written template and still yields defaults.
	if got := Load(); got != Default() {
		t.Fatalf("reload of written template = %+v, want Default()", got)
	}
}

func TestLoadDoesNotOverwriteExistingConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeConfig(t, dir, "github:\n  repo_page_size: 7\n")
	path := filepath.Join(dir, "lazygh", "config.yml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("setup read: %v", err)
	}

	got := Load()
	if got.GitHub.RepoPageSize != 7 {
		t.Errorf("RepoPageSize = %d, want 7 (existing file must be honored)", got.GitHub.RepoPageSize)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after Load: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("existing config was modified;\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestLoadDefaultsWhenTemplateWriteFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits do not restrict writes")
	}
	parent := t.TempDir()
	roDir := filepath.Join(parent, "ro")
	if err := os.Mkdir(roDir, 0o500); err != nil { // read+execute, no write
		t.Fatalf("setup: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o700) }) // let TempDir cleanup remove it
	t.Setenv("XDG_CONFIG_HOME", roDir)
	buf := captureLogs(t)

	if got := Load(); got != Default() {
		t.Fatalf("Load() = %+v, want Default() when the template can't be written", got)
	}
	if out := buf.String(); !strings.Contains(out, "level=WARN") || !strings.Contains(out, "could not write default") {
		t.Fatalf("expected a WARN about failing to write the default; got: %s", out)
	}
	if _, err := os.Stat(filepath.Join(roDir, "lazygh", "config.yml")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected no config file to be created; stat err = %v", err)
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

func TestLoadUnreadableFileLogsErrorAndDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	buf := captureLogs(t)
	// Put a directory where config.yml is expected so ReadFile fails (non-ENOENT).
	if err := os.MkdirAll(filepath.Join(dir, "lazygh", "config.yml"), 0o700); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if got := Load(); got != Default() {
		t.Fatalf("Load() = %+v, want Default() on unreadable file", got)
	}
	if out := buf.String(); !strings.Contains(out, "level=ERROR") || !strings.Contains(out, "cannot read file") {
		t.Fatalf("expected an ERROR 'cannot read file' log; got: %s", out)
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

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

// TestApplyRawValidationBoundaries exercises applyRaw's validation guards at
// their boundaries directly: non-positive durations and out-of-range page sizes
// default (with a WARN naming the field), while in-range values are applied.
func TestApplyRawValidationBoundaries(t *testing.T) {
	def := Default()
	tests := []struct {
		name        string
		in          rawGitHub
		wantREST    time.Duration
		wantSub     time.Duration
		wantPage    int
		wantWarnFor string // substring a WARN must contain; "" means no WARN expected
	}{
		{
			name: "rest_timeout zero -> default + warn",
			in:   rawGitHub{RESTTimeout: strPtr("0s")}, wantREST: def.GitHub.RESTTimeout,
			wantSub: def.GitHub.SubprocessTimeout, wantPage: def.GitHub.RepoPageSize, wantWarnFor: "rest_timeout",
		},
		{
			name: "rest_timeout negative -> default + warn",
			in:   rawGitHub{RESTTimeout: strPtr("-5s")}, wantREST: def.GitHub.RESTTimeout,
			wantSub: def.GitHub.SubprocessTimeout, wantPage: def.GitHub.RepoPageSize, wantWarnFor: "rest_timeout",
		},
		{
			name: "rest_timeout positive -> applied",
			in:   rawGitHub{RESTTimeout: strPtr("5s")}, wantREST: 5 * time.Second,
			wantSub: def.GitHub.SubprocessTimeout, wantPage: def.GitHub.RepoPageSize,
		},
		{
			name: "repo_page_size 0 -> default + warn",
			in:   rawGitHub{RepoPageSize: intPtr(0)}, wantREST: def.GitHub.RESTTimeout,
			wantSub: def.GitHub.SubprocessTimeout, wantPage: def.GitHub.RepoPageSize, wantWarnFor: "repo_page_size",
		},
		{
			name: "repo_page_size 1 -> applied",
			in:   rawGitHub{RepoPageSize: intPtr(1)}, wantREST: def.GitHub.RESTTimeout,
			wantSub: def.GitHub.SubprocessTimeout, wantPage: 1,
		},
		{
			name: "repo_page_size 100 -> applied",
			in:   rawGitHub{RepoPageSize: intPtr(100)}, wantREST: def.GitHub.RESTTimeout,
			wantSub: def.GitHub.SubprocessTimeout, wantPage: 100,
		},
		{
			name: "repo_page_size 101 -> default + warn",
			in:   rawGitHub{RepoPageSize: intPtr(101)}, wantREST: def.GitHub.RESTTimeout,
			wantSub: def.GitHub.SubprocessTimeout, wantPage: def.GitHub.RepoPageSize, wantWarnFor: "repo_page_size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureLogs(t)
			cfg := Default()
			in := tt.in
			applyRaw(&cfg, raw{GitHub: &in})

			if cfg.GitHub.RESTTimeout != tt.wantREST {
				t.Errorf("RESTTimeout = %v, want %v", cfg.GitHub.RESTTimeout, tt.wantREST)
			}
			if cfg.GitHub.SubprocessTimeout != tt.wantSub {
				t.Errorf("SubprocessTimeout = %v, want %v", cfg.GitHub.SubprocessTimeout, tt.wantSub)
			}
			if cfg.GitHub.RepoPageSize != tt.wantPage {
				t.Errorf("RepoPageSize = %d, want %d", cfg.GitHub.RepoPageSize, tt.wantPage)
			}

			out := buf.String()
			if tt.wantWarnFor == "" {
				if strings.Contains(out, "level=WARN") {
					t.Errorf("expected no WARN log for valid value; got: %s", out)
				}
			} else if !strings.Contains(out, "level=WARN") || !strings.Contains(out, tt.wantWarnFor) {
				t.Errorf("expected a WARN naming %q; got: %s", tt.wantWarnFor, out)
			}
		})
	}
}
