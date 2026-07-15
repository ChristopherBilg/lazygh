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
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the typed, validated application configuration.
type Config struct {
	GitHub GitHubConfig
}

// GitHubConfig holds limits for outbound GitHub calls.
type GitHubConfig struct {
	RESTTimeout       time.Duration
	SubprocessTimeout time.Duration
	RepoPageSize      int
}

// Default returns the documented defaults. They match the values previously
// hardcoded in internal/github, so an absent config file changes nothing.
func Default() Config {
	return Config{GitHub: GitHubConfig{
		RESTTimeout:       10 * time.Second,
		SubprocessTimeout: 30 * time.Second,
		RepoPageSize:      50,
	}}
}

// rawGitHub mirrors the on-disk "github" mapping. Pointer fields distinguish an
// absent key from a zero value, and durations arrive as strings so
// time.ParseDuration gives friendly "10s"/"500ms" syntax.
type rawGitHub struct {
	RESTTimeout       *string `yaml:"rest_timeout"`
	SubprocessTimeout *string `yaml:"subprocess_timeout"`
	RepoPageSize      *int    `yaml:"repo_page_size"`
}

// raw mirrors the on-disk YAML. Unknown keys are ignored (no strict decoding)
// so a config written for a newer lazygh does not break an older binary.
type raw struct {
	GitHub *rawGitHub `yaml:"github"`
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

	data, err := os.ReadFile(path)
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
  # repo_page_size: 50       # repositories fetched for the picker (1-100)
`

// writeDefaultConfig writes defaultConfigTemplate to path, creating the parent
// directory (0o700; file 0o600, matching internal/logging). O_EXCL guarantees an
// existing file is never overwritten.
func writeDefaultConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
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

// applyRaw overlays present, valid fields from r onto cfg (which starts at
// Default()). Each invalid field is logged at WARN and left at its default.
func applyRaw(cfg *Config, r raw) {
	if r.GitHub == nil {
		return
	}
	g := r.GitHub

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
}
