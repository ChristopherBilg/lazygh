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
	"slices"
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

	got := Load()
	def := Default()
	if got.GitHub != def.GitHub || !slices.Equal(got.Keys.Quit, def.Keys.Quit) {
		t.Fatalf("Load() = %+v, want Default() %+v", got, def)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected a default config to be written at %s: %v", path, err)
	}
	content := string(data)
	// Every key is commented out (so the written file is equivalent to "no file"),
	// and each commented example shows the current default value — asserting the
	// values guards against the template drifting from Default().
	for _, want := range []string{
		fmt.Sprintf("# rest_timeout: %s", def.GitHub.RESTTimeout),
		fmt.Sprintf("# subprocess_timeout: %s", def.GitHub.SubprocessTimeout),
		fmt.Sprintf("# repo_page_size: %d", def.GitHub.RepoPageSize),
	} {
		if !strings.Contains(content, want) {
			t.Errorf("written template missing commented default %q; got:\n%s", want, content)
		}
	}
	for _, want := range []string{"keys:", "# checkout: [c]", "theme:", `# accent: "62"`} {
		if !strings.Contains(content, want) {
			t.Errorf("written template missing %q; got:\n%s", want, content)
		}
	}
	// Loading again reads the freshly written template and still yields defaults.
	got2 := Load()
	def2 := Default()
	if got2.GitHub != def2.GitHub || !slices.Equal(got2.Keys.Quit, def2.Keys.Quit) {
		t.Fatalf("reload of written template = %+v, want Default()", got2)
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

	got := Load()
	def := Default()
	if got.GitHub != def.GitHub || !slices.Equal(got.Keys.Quit, def.Keys.Quit) {
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

	got := Load()
	def := Default()
	if got.GitHub != def.GitHub || !slices.Equal(got.Keys.Quit, def.Keys.Quit) {
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
	got := Load()
	def := Default()
	if got.GitHub != def.GitHub || !slices.Equal(got.Keys.Quit, def.Keys.Quit) {
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

func TestApplyKeysValidation(t *testing.T) {
	def := Default()
	tests := []struct {
		name         string
		in           rawKeys
		wantCheckout []string
		wantWarn     bool
	}{
		{"list applied", rawKeys{Checkout: keyListPtr("x", "y")}, []string{"x", "y"}, false},
		{"single applied", rawKeys{Checkout: keyListPtr("x")}, []string{"x"}, false},
		{"empty -> default + warn", rawKeys{Checkout: keyListPtr()}, def.Keys.Checkout, true},
		{"all blank -> default + warn", rawKeys{Checkout: keyListPtr("  ", "")}, def.Keys.Checkout, true},
		{"blanks dropped, rest kept", rawKeys{Checkout: keyListPtr(" x ", "", "y")}, []string{"x", "y"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureLogs(t)
			cfg := Default()
			in := tt.in
			applyRaw(&cfg, raw{Keys: &in})
			if !slices.Equal(cfg.Keys.Checkout, tt.wantCheckout) {
				t.Errorf("Checkout = %v, want %v", cfg.Keys.Checkout, tt.wantCheckout)
			}
			if gotWarn := strings.Contains(buf.String(), "level=WARN"); gotWarn != tt.wantWarn {
				t.Errorf("warn = %v, want %v; log: %s", gotWarn, tt.wantWarn, buf.String())
			}
		})
	}
}

func TestApplyThemeValidation(t *testing.T) {
	def := Default()
	tests := []struct {
		name       string
		in         rawTheme
		wantAccent string
		wantWarn   bool
	}{
		{"ansi index", rawTheme{Accent: strPtr("205")}, "205", false},
		{"ansi 0", rawTheme{Accent: strPtr("0")}, "0", false},
		{"ansi 255", rawTheme{Accent: strPtr("255")}, "255", false},
		{"hex short", rawTheme{Accent: strPtr("#abc")}, "#abc", false},
		{"hex long", rawTheme{Accent: strPtr("#7D56F4")}, "#7D56F4", false},
		{"ansi 256 -> default", rawTheme{Accent: strPtr("256")}, def.Theme.Accent, true},
		{"negative -> default", rawTheme{Accent: strPtr("-1")}, def.Theme.Accent, true},
		{"bad hex -> default", rawTheme{Accent: strPtr("#zz")}, def.Theme.Accent, true},
		{"name -> default", rawTheme{Accent: strPtr("purple")}, def.Theme.Accent, true},
		{"empty -> default", rawTheme{Accent: strPtr("")}, def.Theme.Accent, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureLogs(t)
			cfg := Default()
			in := tt.in
			applyRaw(&cfg, raw{Theme: &in})
			if cfg.Theme.Accent != tt.wantAccent {
				t.Errorf("Accent = %q, want %q", cfg.Theme.Accent, tt.wantAccent)
			}
			if gotWarn := strings.Contains(buf.String(), "level=WARN"); gotWarn != tt.wantWarn {
				t.Errorf("warn = %v, want %v; log: %s", gotWarn, tt.wantWarn, buf.String())
			}
		})
	}
}

func TestApplyThemeValidSiblingApplies(t *testing.T) {
	cfg := Default()
	applyRaw(&cfg, raw{Theme: &rawTheme{Accent: strPtr("nope"), Border: strPtr("15")}})
	if cfg.Theme.Accent != Default().Theme.Accent {
		t.Errorf("Accent = %q, want default (invalid ignored)", cfg.Theme.Accent)
	}
	if cfg.Theme.Border != "15" {
		t.Errorf("Border = %q, want 15 (valid sibling applied)", cfg.Theme.Border)
	}
}

func TestLoadScalarAndListKeys(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeConfig(t, dir, "keys:\n  checkout: x\n  open: [a, b]\n")

	got := Load()
	if !slices.Equal(got.Keys.Checkout, []string{"x"}) {
		t.Errorf("Checkout = %v, want [x] (scalar accepted)", got.Keys.Checkout)
	}
	if !slices.Equal(got.Keys.Open, []string{"a", "b"}) {
		t.Errorf("Open = %v, want [a b]", got.Keys.Open)
	}
	if !slices.Equal(got.Keys.Refresh, Default().Keys.Refresh) {
		t.Errorf("Refresh = %v, want default (untouched)", got.Keys.Refresh)
	}
}

func TestLoadAppliesTheme(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeConfig(t, dir, "theme:\n  accent: \"205\"\n  selected: \"#ff8800\"\n")

	got := Load()
	if got.Theme.Accent != "205" {
		t.Errorf("Accent = %q, want 205", got.Theme.Accent)
	}
	if got.Theme.Selected != "#ff8800" {
		t.Errorf("Selected = %q, want #ff8800", got.Theme.Selected)
	}
	if got.Theme.Border != Default().Theme.Border {
		t.Errorf("Border = %q, want default (untouched)", got.Theme.Border)
	}
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func keyListPtr(s ...string) *keyList {
	k := keyList(s)
	return &k
}

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

func TestDefaultSearchKey(t *testing.T) {
	if got := Default().Keys.Search; !slices.Equal(got, []string{"/"}) {
		t.Fatalf("Default search key = %v, want [/]", got)
	}
}

func TestApplySearchOverride(t *testing.T) {
	cfg := Default()
	applyRaw(&cfg, raw{Keys: &rawKeys{Search: keyListPtr("f")}})
	if !slices.Equal(cfg.Keys.Search, []string{"f"}) {
		t.Fatalf("Search = %v, want [f] after override", cfg.Keys.Search)
	}
}

func TestTemplateIncludesSearch(t *testing.T) {
	if !strings.Contains(defaultConfigTemplate, "# search: [/]") {
		t.Fatalf("default config template missing the commented search key:\n%s", defaultConfigTemplate)
	}
}

func TestDefaultTabKeys(t *testing.T) {
	def := Default()
	if !slices.Equal(def.Keys.PrevTab, []string{"["}) {
		t.Errorf("PrevTab default = %v, want [[]", def.Keys.PrevTab)
	}
	if !slices.Equal(def.Keys.NextTab, []string{"]"}) {
		t.Errorf("NextTab default = %v, want []]", def.Keys.NextTab)
	}
}

func TestApplyTabKeyOverrides(t *testing.T) {
	cfg := Default()
	applyRaw(&cfg, raw{Keys: &rawKeys{PrevTab: keyListPtr("p"), NextTab: keyListPtr("n")}})
	if !slices.Equal(cfg.Keys.PrevTab, []string{"p"}) {
		t.Errorf("PrevTab = %v, want [p]", cfg.Keys.PrevTab)
	}
	if !slices.Equal(cfg.Keys.NextTab, []string{"n"}) {
		t.Errorf("NextTab = %v, want [n]", cfg.Keys.NextTab)
	}
}

func TestTemplateIncludesTabKeys(t *testing.T) {
	for _, want := range []string{`# prev_tab: ["["]`, `# next_tab: ["]"]`} {
		if !strings.Contains(defaultConfigTemplate, want) {
			t.Fatalf("default config template missing %q:\n%s", want, defaultConfigTemplate)
		}
	}
}

func TestDefaultFilterKeys(t *testing.T) {
	t.Parallel()
	def := Default()
	if !slices.Equal(def.Keys.FilterMine, []string{"m"}) {
		t.Errorf("FilterMine default = %v, want [m]", def.Keys.FilterMine)
	}
	if !slices.Equal(def.Keys.FilterReview, []string{"v"}) {
		t.Errorf("FilterReview default = %v, want [v]", def.Keys.FilterReview)
	}
	if !slices.Equal(def.Keys.FilterDependabot, []string{"d"}) {
		t.Errorf("FilterDependabot default = %v, want [d]", def.Keys.FilterDependabot)
	}
}

func TestApplyKeysFilterBindings(t *testing.T) {
	t.Parallel()
	cfg := Default()
	applyRaw(&cfg, raw{Keys: &rawKeys{
		FilterMine:       keyListPtr("a"),
		FilterReview:     keyListPtr("b"),
		FilterDependabot: keyListPtr("c"),
	}})
	if !slices.Equal(cfg.Keys.FilterMine, []string{"a"}) {
		t.Errorf("FilterMine = %v, want [a]", cfg.Keys.FilterMine)
	}
	if !slices.Equal(cfg.Keys.FilterReview, []string{"b"}) {
		t.Errorf("FilterReview = %v, want [b]", cfg.Keys.FilterReview)
	}
	if !slices.Equal(cfg.Keys.FilterDependabot, []string{"c"}) {
		t.Errorf("FilterDependabot = %v, want [c]", cfg.Keys.FilterDependabot)
	}
}
