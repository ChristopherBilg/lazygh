// Package config loads optional user settings from a YAML file, applies
// documented defaults, and exposes typed settings to the rest of the app. It
// never fails the application: any problem (missing/unreadable/malformed file,
// or an individual invalid value) is reported through the file logger (slog)
// and defaults are used, so the TUI never crashes on bad configuration.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the typed, validated application configuration.
type Config struct {
	GitHub GitHubConfig
	Keys   KeysConfig
	Theme  ThemeConfig
}

// GitHubConfig holds settings for outbound GitHub calls: timeouts, page size,
// and the merge method used by the PR merge action.
type GitHubConfig struct {
	RESTTimeout       time.Duration
	SubprocessTimeout time.Duration
	RepoPageSize      int
	MergeMethod       string
}

// KeysConfig holds the resolved key bindings: one non-empty []string per action.
type KeysConfig struct {
	Quit             []string
	Back             []string
	Up               []string
	Down             []string
	Select           []string
	Refresh          []string
	TogglePane       []string
	Checkout         []string
	Open             []string
	Approve          []string
	Merge            []string
	Close            []string
	Search           []string
	NavPRs           []string
	NavIssues        []string
	NavActions       []string
	FilterMine       []string
	FilterReview     []string
	FilterDependabot []string
	PrevTab          []string
	NextTab          []string
}

// ThemeConfig holds the resolved, validated color per semantic role.
type ThemeConfig struct {
	Accent   string
	Border   string
	Selected string
	Title    string
	Error    string
}

// Default returns the documented defaults. They match the values previously
// hardcoded in internal/github, so an absent config file changes nothing.
func Default() Config {
	return Config{
		GitHub: GitHubConfig{
			RESTTimeout:       10 * time.Second,
			SubprocessTimeout: 30 * time.Second,
			RepoPageSize:      50,
			MergeMethod:       "merge",
		},
		Keys: KeysConfig{
			Quit:             []string{"q"},
			Back:             []string{"esc", "backspace"},
			Up:               []string{"up", "k"},
			Down:             []string{"down", "j"},
			Select:           []string{"enter"},
			Refresh:          []string{"r"},
			TogglePane:       []string{"tab", "shift+tab"},
			Checkout:         []string{"c"},
			Open:             []string{"o"},
			Approve:          []string{"a"},
			Merge:            []string{"M"},
			Close:            []string{"D"},
			Search:           []string{"/"},
			NavPRs:           []string{"1"},
			NavIssues:        []string{"2"},
			NavActions:       []string{"3"},
			FilterMine:       []string{"m"},
			FilterReview:     []string{"v"},
			FilterDependabot: []string{"d"},
			PrevTab:          []string{"["},
			NextTab:          []string{"]"},
		},
		Theme: ThemeConfig{
			Accent:   "62",
			Border:   "240",
			Selected: "212",
			Title:    "230",
			Error:    "196",
		},
	}
}

// rawGitHub mirrors the on-disk "github" mapping. Pointer fields distinguish an
// absent key from a zero value, and durations arrive as strings so
// time.ParseDuration gives friendly "10s"/"500ms" syntax.
type rawGitHub struct {
	RESTTimeout       *string `yaml:"rest_timeout"`
	SubprocessTimeout *string `yaml:"subprocess_timeout"`
	RepoPageSize      *int    `yaml:"repo_page_size"`
	MergeMethod       *string `yaml:"merge_method"`
}

// keyList is a []string that also accepts a single YAML scalar, so both
// `checkout: c` and `checkout: [c, x]` decode. yaml.v3 reads scalar numbers as
// their string form (e.g. 1 -> "1"), matching bubbletea's key names.
type keyList []string

func (k *keyList) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*k = keyList{value.Value}
		return nil
	}
	var s []string
	if err := value.Decode(&s); err != nil {
		return err
	}
	*k = keyList(s)
	return nil
}

type rawKeys struct {
	Quit             *keyList `yaml:"quit"`
	Back             *keyList `yaml:"back"`
	Up               *keyList `yaml:"up"`
	Down             *keyList `yaml:"down"`
	Select           *keyList `yaml:"select"`
	Refresh          *keyList `yaml:"refresh"`
	TogglePane       *keyList `yaml:"toggle_pane"`
	Checkout         *keyList `yaml:"checkout"`
	Open             *keyList `yaml:"open"`
	Approve          *keyList `yaml:"approve"`
	Merge            *keyList `yaml:"merge"`
	Close            *keyList `yaml:"close"`
	Search           *keyList `yaml:"search"`
	NavPRs           *keyList `yaml:"nav_prs"`
	NavIssues        *keyList `yaml:"nav_issues"`
	NavActions       *keyList `yaml:"nav_actions"`
	FilterMine       *keyList `yaml:"filter_mine"`
	FilterReview     *keyList `yaml:"filter_review"`
	FilterDependabot *keyList `yaml:"filter_dependabot"`
	PrevTab          *keyList `yaml:"prev_tab"`
	NextTab          *keyList `yaml:"next_tab"`
}

type rawTheme struct {
	Accent   *string `yaml:"accent"`
	Border   *string `yaml:"border"`
	Selected *string `yaml:"selected"`
	Title    *string `yaml:"title"`
	Error    *string `yaml:"error"`
}

// raw mirrors the on-disk YAML. Unknown keys are ignored (no strict decoding)
// so a config written for a newer lazygh does not break an older binary.
type raw struct {
	GitHub *rawGitHub `yaml:"github"`
	Keys   *rawKeys   `yaml:"keys"`
	Theme  *rawTheme  `yaml:"theme"`
}

// Load reads the config file, applies defaults, validates values, and returns a
// usable Config. It never returns an error: a missing file causes a commented
// default template to be written (best-effort; a write failure is logged and
// ignored) and defaults to be used; an unreadable/malformed file -> defaults
// (error log); an individual invalid value -> that field's default (warn log),
// keeping valid siblings. Unknown or misspelled keys are ignored silently (not
// logged), since decoding is not strict.
func Load() Config {
	cfg := Default()

	path, err := configPath()
	if err != nil {
		slog.Error("config: cannot resolve path; using defaults", "err", err)
		return cfg
	}

	data, err := os.ReadFile(path) // #nosec G304 -- path is the app's own XDG config path, not user-controlled input
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// First run: scaffold a commented template so the file is easy to
			// find and edit. Best-effort — the app still runs on defaults if the
			// write fails (e.g. a read-only home).
			if werr := writeDefaultConfig(path); werr != nil {
				slog.Warn("config: no file and could not write default; using defaults", "path", path, "err", werr)
			} else {
				slog.Info("config: wrote default config; using defaults", "path", path)
			}
			return cfg
		}
		slog.Error("config: cannot read file; using defaults", "path", path, "err", err)
		return cfg
	}

	var r raw
	if err := yaml.Unmarshal(data, &r); err != nil {
		slog.Error("config: malformed file; using defaults", "path", path, "err", err)
		return cfg
	}

	applyRaw(&cfg, r)
	slog.Info("config loaded", "path", path)
	return cfg
}

// configPath returns $XDG_CONFIG_HOME/lazygh/config.yml, or
// ~/.config/lazygh/config.yml when XDG_CONFIG_HOME is unset. It mirrors
// logging.logPath's XDG resolution.
func configPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "lazygh", "config.yml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "lazygh", "config.yml"), nil
}

// defaultConfigTemplate is the commented template written on first run. Every key is
// commented out, so the freshly written file behaves exactly like "no file" (all
// built-in defaults) until the user uncomments a key. A commented template (vs.
// active values) avoids pinning users to today's defaults if one changes later.
const defaultConfigTemplate = `# lazygh configuration - uncomment and edit any key to override its default.
# All keys are optional; a commented key uses lazygh's built-in default.
# See docs/configuration.md for the full list of options.
github:
  # rest_timeout: 10s        # timeout for each GitHub REST API request
  # subprocess_timeout: 30s  # timeout for each gh subprocess call
  # repo_page_size: 50       # repos per page request; up to 20 pages loaded (1-100)
  # merge_method: merge      # how 'M' merges a PR: merge, squash, or rebase
keys:
  # quit: [q]                # ctrl+c always quits too
  # back: [esc, backspace]
  # up: [up, k]
  # down: [down, j]
  # select: [enter]
  # refresh: [r]
  # toggle_pane: [tab, shift+tab]
  # checkout: [c]
  # open: [o]
  # approve: [a]                # submit an approving review on the selected PR
  # merge: [M]                  # merge the selected PR (asks to confirm)
  # close: [D]                  # close the selected PR (asks to confirm)
  # search: [/]
  # nav_prs: ["1"]
  # nav_issues: ["2"]
  # nav_actions: ["3"]
  # filter_mine: [m]              # show only PRs you authored
  # filter_review: [v]            # show only PRs awaiting your review
  # filter_dependabot: [d]        # show only Dependabot PRs
  # prev_tab: ["["]
  # next_tab: ["]"]
theme:
  # accent: "62"             # active borders, title bar, boxes
  # border: "240"            # inactive pane borders
  # selected: "212"          # highlighted list row
  # title: "230"             # title text
  # error: "196"             # error text
`

// writeDefaultConfig writes defaultConfigTemplate to path, creating the parent
// directory (0o700; file 0o600, matching internal/logging). O_EXCL guarantees an
// existing file is never overwritten.
func writeDefaultConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- path is the app's own XDG config path, not user-controlled input
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	if _, err := f.WriteString(defaultConfigTemplate); err != nil {
		_ = f.Close()
		return fmt.Errorf("write config file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close config file: %w", err)
	}
	return nil
}

// applyRaw overlays present, valid fields onto cfg (which starts at Default()).
func applyRaw(cfg *Config, r raw) {
	applyGitHub(cfg, r.GitHub)
	applyKeys(cfg, r.Keys)
	applyTheme(cfg, r.Theme)
}

func applyGitHub(cfg *Config, g *rawGitHub) {
	if g == nil {
		return
	}
	if g.RESTTimeout != nil {
		if d, err := time.ParseDuration(*g.RESTTimeout); err == nil && d > 0 {
			cfg.GitHub.RESTTimeout = d
		} else {
			slog.Warn("config: invalid github.rest_timeout; using default",
				"value", *g.RESTTimeout, "default", cfg.GitHub.RESTTimeout)
		}
	}
	if g.SubprocessTimeout != nil {
		if d, err := time.ParseDuration(*g.SubprocessTimeout); err == nil && d > 0 {
			cfg.GitHub.SubprocessTimeout = d
		} else {
			slog.Warn("config: invalid github.subprocess_timeout; using default",
				"value", *g.SubprocessTimeout, "default", cfg.GitHub.SubprocessTimeout)
		}
	}
	if g.RepoPageSize != nil {
		if *g.RepoPageSize >= 1 && *g.RepoPageSize <= 100 {
			cfg.GitHub.RepoPageSize = *g.RepoPageSize
		} else {
			slog.Warn("config: github.repo_page_size out of range [1,100]; using default",
				"value", *g.RepoPageSize, "default", cfg.GitHub.RepoPageSize)
		}
	}
	if g.MergeMethod != nil {
		if isValidMergeMethod(*g.MergeMethod) {
			cfg.GitHub.MergeMethod = *g.MergeMethod
		} else {
			slog.Warn("config: invalid github.merge_method; using default",
				"value", *g.MergeMethod, "default", cfg.GitHub.MergeMethod)
		}
	}
}

func applyKeys(cfg *Config, rk *rawKeys) {
	if rk == nil {
		return
	}
	for _, b := range []struct {
		name string
		raw  *keyList
		dst  *[]string
	}{
		{"quit", rk.Quit, &cfg.Keys.Quit},
		{"back", rk.Back, &cfg.Keys.Back},
		{"up", rk.Up, &cfg.Keys.Up},
		{"down", rk.Down, &cfg.Keys.Down},
		{"select", rk.Select, &cfg.Keys.Select},
		{"refresh", rk.Refresh, &cfg.Keys.Refresh},
		{"toggle_pane", rk.TogglePane, &cfg.Keys.TogglePane},
		{"checkout", rk.Checkout, &cfg.Keys.Checkout},
		{"open", rk.Open, &cfg.Keys.Open},
		{"approve", rk.Approve, &cfg.Keys.Approve},
		{"merge", rk.Merge, &cfg.Keys.Merge},
		{"close", rk.Close, &cfg.Keys.Close},
		{"search", rk.Search, &cfg.Keys.Search},
		{"nav_prs", rk.NavPRs, &cfg.Keys.NavPRs},
		{"nav_issues", rk.NavIssues, &cfg.Keys.NavIssues},
		{"nav_actions", rk.NavActions, &cfg.Keys.NavActions},
		{"filter_mine", rk.FilterMine, &cfg.Keys.FilterMine},
		{"filter_review", rk.FilterReview, &cfg.Keys.FilterReview},
		{"filter_dependabot", rk.FilterDependabot, &cfg.Keys.FilterDependabot},
		{"prev_tab", rk.PrevTab, &cfg.Keys.PrevTab},
		{"next_tab", rk.NextTab, &cfg.Keys.NextTab},
	} {
		if b.raw == nil {
			continue
		}
		if ks, ok := sanitizeKeys(*b.raw); ok {
			*b.dst = ks
		} else {
			slog.Warn("config: keys."+b.name+" has no usable keys; using default",
				"value", []string(*b.raw), "default", *b.dst)
		}
	}
}

func applyTheme(cfg *Config, rt *rawTheme) {
	if rt == nil {
		return
	}
	for _, role := range []struct {
		name string
		raw  *string
		dst  *string
	}{
		{"accent", rt.Accent, &cfg.Theme.Accent},
		{"border", rt.Border, &cfg.Theme.Border},
		{"selected", rt.Selected, &cfg.Theme.Selected},
		{"title", rt.Title, &cfg.Theme.Title},
		{"error", rt.Error, &cfg.Theme.Error},
	} {
		if role.raw == nil {
			continue
		}
		if isValidColor(*role.raw) {
			*role.dst = *role.raw
		} else {
			slog.Warn("config: theme."+role.name+" is not a valid color; using default",
				"value", *role.raw, "default", *role.dst)
		}
	}
}

// sanitizeKeys trims entries and drops blanks; ok is false if none remain.
func sanitizeKeys(in []string) (out []string, ok bool) {
	for _, k := range in {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out, len(out) > 0
}

// isValidMergeMethod reports whether m is one of gh's supported merge methods.
func isValidMergeMethod(m string) bool {
	switch m {
	case "merge", "squash", "rebase":
		return true
	default:
		return false
	}
}

// hexColor matches #RGB and #RRGGBB.
var hexColor = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// isValidColor accepts a hex string or an ANSI-256 index (0–255).
func isValidColor(s string) bool {
	if hexColor.MatchString(s) {
		return true
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 0 && n <= 255 {
		return true
	}
	return false
}
