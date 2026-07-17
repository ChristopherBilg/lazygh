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

### GitHub (`github`)

| Key | Type | Default | Description |
|---|---|---|---|
| `github.rest_timeout` | duration | `10s` | Timeout for each GitHub REST API request. |
| `github.subprocess_timeout` | duration | `30s` | Timeout for each `gh` subprocess call (PR checkout, open in browser). |
| `github.repo_page_size` | integer (1–100) | `50` | Number of repositories fetched for the repository picker. Capped at 100 by the GitHub API. |

Durations use Go's duration syntax — e.g. `500ms`, `10s`, `2m`, `1h` — as
strings, not bare numbers (`10s`, not `10`).

### Keybindings (`keys`)

Remap any action. Each action takes a single key or a list; omitted actions use
their defaults. All keys are optional.

| Action | Default | Where it acts |
|---|---|---|
| `quit` | `q` (+ `ctrl+c`) | global |
| `back` | `esc`, `backspace` | global (return to repo list) |
| `up` | `up`, `k` | repo list, PR view |
| `down` | `down`, `j` | repo list, PR view |
| `select` | `enter` | repo list (open repo) |
| `refresh` | `r` | repo list, PR view |
| `toggle_pane` | `tab`, `shift+tab` | PR view (list ↔ detail focus) |
| `checkout` | `c` | PR view |
| `open` | `o` | PR view (open in browser) |
| `search` | `/` | PR view (fuzzy-filter titles) |
| `nav_prs` | `1` | global (after a repo is selected) |
| `nav_issues` | `2` | global |
| `nav_actions` | `3` | global |

Key names use Bubble Tea's vocabulary: printable characters (`c`, `o`, `/`),
named keys (`enter`, `esc`, `tab`, `shift+tab`, `up`, `down`, `backspace`,
`space`), and modifier forms (`ctrl+r`, `alt+enter`).

- **`ctrl+c` always quits**, regardless of how `quit` is bound — it can't be
  remapped away, so you can never trap yourself.
- **Digit keys read the same quoted or not** (`nav_prs: 1` and `nav_prs: "1"` are
  equivalent); the examples quote them just to match the generated template's
  style.
- **Conflicts are your responsibility.** If two actions share a key, the global
  actions (quit/back/nav) win over view actions, and otherwise the first match in
  code order wins. lazygh does not warn about conflicts.
- **On-screen hint bars show the default keys.** The footer hints and the error
  overlay currently display the built-in keys even after you remap; they don't yet
  reflect your bindings (a contextual help overlay is planned). Your remapped keys
  still work — only the on-screen labels lag.

```yaml
keys:
  checkout: [c, x]      # a list…
  refresh: r            # …or a single key
```

### Theme (`theme`)

Override the color palette. Each role is one color; omitted roles use their
defaults.

| Role | Default | What it colors |
|---|---|---|
| `accent` | `62` | active pane border, title background, menu/box border |
| `border` | `240` | inactive pane borders |
| `selected` | `212` | highlighted (selected) list row |
| `title` | `230` | title text (on the `accent` background) |
| `error` | `196` | error text |

`accent` intentionally drives three styles at once (the single "primary" color).
A color is either a **hex** string (`"#7D56F4"`, `"#abc"`) or an **ANSI-256
index** (`"0"`–`"255"`). **Hex must be quoted** — an unquoted `#` starts a YAML
comment.

```yaml
theme:
  accent: "205"         # ANSI-256 index
  selected: "#ff8800"   # hex (quoted)
```

## Example

```yaml
# ~/.config/lazygh/config.yml — all keys optional; omitted keys use defaults.
github:
  rest_timeout: 10s        # bound each REST request
  subprocess_timeout: 30s  # bound each `gh` subprocess call
  repo_page_size: 50       # repositories fetched for the picker (1–100)
keys:
  checkout: [c, x]         # remap / add keys; single key or a list
  # ...any of: quit, back, up, down, select, refresh, toggle_pane, open, search, nav_prs, nav_issues, nav_actions
theme:
  accent: "205"            # ANSI-256 index or quoted hex like "#7D56F4"
  selected: "#ff8800"
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
- **An invalid key list or color** (e.g. `theme.accent: nope`, `keys.checkout: []`)
  → that one action/role falls back to its default (logged as a warning); all
  other keys, roles, and settings still apply.

Configuration problems are reported through the file logger, not the terminal.
The log file is at `$XDG_STATE_HOME/lazygh/app.log` (default
`~/.local/state/lazygh/app.log`); raise verbosity with `LAZYGH_LOG_LEVEL=debug`.
