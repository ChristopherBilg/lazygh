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

- **No file** → defaults are used.
- **Unreadable or malformed file** (invalid YAML, or a value of the wrong type
  such as `rest_timeout: 10`) → a single error is written to the log file and
  all defaults are used.
- **A single out-of-range or unparseable value** (e.g. `repo_page_size: 999`) →
  that one setting falls back to its default (logged as a warning); valid
  settings in the same file still apply.

Configuration problems are reported through the file logger, not the terminal.
The log file is at `$XDG_STATE_HOME/lazygh/app.log` (default
`~/.local/state/lazygh/app.log`); raise verbosity with `LAZYGH_LOG_LEVEL=debug`.
