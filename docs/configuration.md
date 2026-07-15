# Configuration

lazygh reads an optional YAML configuration file at startup. **No config file is
required** — with none present, lazygh runs on the documented defaults.

## Location

lazygh loads:

```
$XDG_CONFIG_HOME/lazygh/config.yml
```

If `XDG_CONFIG_HOME` is unset, it defaults to:

```
~/.config/lazygh/config.yml
```

On first run, if no config file exists, lazygh writes a commented template to
this path (best-effort) so it is easy to find and edit. Every key starts
commented out, so the fresh file behaves exactly like having no file — uncomment
a key to override its default. If the file cannot be written (e.g. a read-only
home), lazygh logs a warning and continues on defaults; it never overwrites an
existing file.

## Options

All keys are optional; any omitted key uses its default.

| Key | Type | Default | Description |
|---|---|---|---|
| `github.rest_timeout` | duration | `10s` | Timeout for each GitHub REST API request. |
| `github.subprocess_timeout` | duration | `30s` | Timeout for each `gh` subprocess call (PR checkout, open in browser). |
| `github.repo_page_size` | integer (1–100) | `50` | Number of repositories fetched for the repository picker. Capped at 100 by the GitHub API. |

Durations use Go's duration syntax — e.g. `500ms`, `10s`, `2m`, `1h` — as
strings, not bare numbers (`10s`, not `10`).

## Example

```yaml
# ~/.config/lazygh/config.yml — all keys optional; omitted keys use defaults.
github:
  rest_timeout: 10s        # bound each REST request
  subprocess_timeout: 30s  # bound each `gh` subprocess call
  repo_page_size: 50       # repositories fetched for the picker (1–100)
```

## Error handling

lazygh never crashes on bad configuration:

- **No file** → a commented default template is written to the config path on
  first run (best-effort) and defaults are used; if the write fails, a warning is
  logged and defaults are still used.
- **Unreadable or malformed file** (invalid YAML syntax, or a value whose type
  can't be decoded — e.g. `github:` set to a list, or `repo_page_size: "50"` as a
  quoted string) → a single error is written to the log file and all defaults
  are used.
- **A single out-of-range or unparseable value** (e.g. `repo_page_size: 999`, or a
  bare number like `rest_timeout: 10` that is missing a duration unit) → that one
  setting falls back to its default (logged as a warning); valid settings in the
  same file still apply.

Configuration problems are reported through the file logger, not the terminal.
The log file is at `$XDG_STATE_HOME/lazygh/app.log` (default
`~/.local/state/lazygh/app.log`); raise verbosity with `LAZYGH_LOG_LEVEL=debug`.
