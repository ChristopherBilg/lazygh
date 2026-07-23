# lazygh

> A lazygit-style terminal UI for GitHub, in your terminal.

[![CI](https://github.com/ChristopherBilg/lazygh/actions/workflows/ci.yml/badge.svg)](https://github.com/ChristopherBilg/lazygh/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](go.mod)

**lazygh** ("Lazy GitHub") is a keyboard-driven terminal UI for working with GitHub, inspired by [lazygit](https://github.com/jesseduffield/lazygit). It's built on the [Charm](https://charm.sh) stack ([Bubble Tea](https://github.com/charmbracelet/bubbletea), Bubbles, and Lip Gloss) and reuses the GitHub CLI's credentials through [`go-gh`](https://github.com/cli/go-gh), so there's no separate login to manage.

> **Project status:** lazygh is early and under active development. Browsing repositories and pull requests works today; the Issues and Actions tabs are navigable placeholders for now. See the [architectural roadmap](ARCHITECTURE_ROADMAP.md) for the full plan.

## Features

- ✅ **Repository picker** — jump into any of your most recently pushed repositories
- ✅ **Pull-request browsing** — a split-pane list + detail view of a repo's open PRs
- ✅ **Tabbed PR detail** — switch the right pane between Description, Files Changed, and Comments with `[`/`]`
- ✅ **Syntax-highlighted diff viewer** — the Files Changed tab renders the PR's diff (via `gh pr diff`) with per-language syntax highlighting and `+`/`-` gutters, scrollable like the rest of the detail pane
- ✅ **PR actions** — check a PR out locally (`c`) or open it in your browser (`o`)
- ✅ **Fuzzy PR search** (`/`) — filter the pull-request list by title as you type
- ✅ **Quick PR filters** — one keystroke to focus on **My PRs** (`m`), **Needs my Review** (`v`), or **Dependabot** (`d`); press the active key again to clear
- ✅ **PR check status** — each PR row shows its aggregate CI/checks state (✅ passing, ❌ failing, 🔄 running, blank when none) at a glance
- ✅ **Refresh with in-memory caching** (`r`) — switching tabs and going back never re-fetches unnecessarily
- ✅ **Fully configurable** — remap every keybinding, recolor the theme, and tune network timeouts from one optional YAML file
- ✅ **TUI-safe logging** — diagnostics go to a log file, never to the screen, so the interface is never corrupted
- 🚧 **Issues & Actions tabs** — navigable placeholders today; full functionality is on the roadmap

## Requirements

- **[Go](https://go.dev/dl/) 1.25 or newer** — to install or build from source.
- **The [GitHub CLI](https://cli.github.com/) (`gh`)** — installed and authenticated. lazygh reads `gh`'s stored token and host, so run `gh auth login` once and you're set; there is no separate lazygh login.

## Installation

Install the latest release with Go:

```sh
go install github.com/ChristopherBilg/lazygh/cmd/lazygh@latest
```

This drops a `lazygh` binary in `$(go env GOPATH)/bin` — make sure that directory is on your `PATH`.

Or build from source:

```sh
git clone https://github.com/ChristopherBilg/lazygh.git
cd lazygh
make build        # compiles ./bin/lazygh
```

You can also run it straight from a source checkout without building a binary:

```sh
make run          # go run ./cmd/lazygh
```

## Usage

1. **Authenticate once** (if you haven't already):

   ```sh
   gh auth login
   ```

2. **Start lazygh:**

   ```sh
   lazygh            # or `make run` from a source checkout
   ```

3. **Pick a repository** from your most recently pushed repos, then browse its open pull requests. Use `tab` to move focus to the detail pane, `c` to check a PR out locally, or `o` to open it in your browser.

> **Note:** Checking out a PR (`c`) runs `gh pr checkout` in your current working directory, so it only works when you launch lazygh from inside a local clone of the selected repository. Everything else works from anywhere.

### Keybindings

These are the defaults; every action can be remapped via configuration (see below).

**Global**

| Key | Action |
|---|---|
| `1` / `2` / `3` | Switch to Pull Requests / Issues / Actions (once a repo is selected) |
| `esc`, `backspace` | Back to the repository list |
| `?` | Open the contextual keybindings overlay (shows the current screen's keys) |
| `q`, `ctrl+c` | Quit (`ctrl+c` always quits and can't be remapped away) |

**Repository list**

| Key | Action |
|---|---|
| `↑`/`k`, `↓`/`j` | Move the selection |
| `enter` | Open the selected repository |
| `r` | Refresh the repository list |

**Pull requests**

| Key | Action |
|---|---|
| `↑`/`k`, `↓`/`j` | Move the selection |
| `/` | Fuzzy-filter the list by PR title (`esc` cancels, `enter` keeps the filter) |
| `m` | Filter to PRs you authored (press again to clear) |
| `v` | Filter to PRs awaiting your review (press again to clear) |
| `d` | Filter to Dependabot PRs (press again to clear) |
| `tab`, `shift+tab` | Toggle focus between the list and the detail pane |
| `[`, `]` | Switch the detail pane's tab (Description / Files Changed / Comments) |
| `c` | Check out the selected PR locally |
| `o` | Open the selected PR in your browser |
| `a` | Approve the selected PR (submit an approving review) |
| `M` | Merge the selected PR (asks to confirm) |
| `D` | Close the selected PR (asks to confirm) |
| `r` | Refresh the pull-request list |

## Configuration

lazygh runs with sensible defaults and **needs no configuration**. To customize it, edit the optional YAML file at:

```
$XDG_CONFIG_HOME/lazygh/config.yml     # or ~/.config/lazygh/config.yml
```

On first run lazygh writes a fully-commented template there, so it's easy to find and edit. Every key is optional — uncomment one to override its default. You can configure:

- **GitHub limits** — per-request REST timeout, `gh` subprocess timeout, and the per-page batch size for fetching your repositories (paged, up to a safety limit on very large accounts).
- **Keybindings** — remap any action to a single key or a list of keys.
- **Theme** — override the accent, border, selected-row, title, and error colors (hex or ANSI-256).

Bad configuration never crashes lazygh: invalid values fall back to their defaults and are noted in the log. The log file lives at `$XDG_STATE_HOME/lazygh/app.log` (default `~/.local/state/lazygh/app.log`); set `LAZYGH_LOG_LEVEL=debug` for more detail.

See **[docs/configuration.md](docs/configuration.md)** for the full reference and examples.

## Architecture

lazygh follows the Elm Architecture that Bubble Tea is built around: each screen is a self-contained model with `Init`, `Update`, and `View`. A root **router** (`internal/tui`) owns the active screen and switches between the repository picker and the per-repo screens (pull requests, issues, actions), keeping each one alive so its selection and scroll position persist as you navigate.

GitHub access is isolated in `internal/github`: REST calls (`go-gh`) fetch lists like repositories and PRs, while `gh` subprocesses drive actions like checkout and open-in-browser — each bounded by a configurable timeout and cached in memory. Logging is file-only (`log/slog`) so diagnostics never corrupt the terminal UI.

For the full architectural plan and the roadmap of upcoming work, see **[ARCHITECTURE_ROADMAP.md](ARCHITECTURE_ROADMAP.md)**.

## Contributing

Contributions are welcome! See **[CONTRIBUTING.md](CONTRIBUTING.md)** for local setup and conventions. The common development tasks are exposed through the `Makefile`:

| Command | What it does |
|---|---|
| `make build` | Compile the binary to `./bin/lazygh` |
| `make run` | Run the app without building a binary |
| `make test` | Run the test suite (`go test ./...`) |
| `make vet` | Run `go vet ./...` |
| `make lint` | Run `golangci-lint run` |
| `make fmt` | Format the code (`golangci-lint fmt`) |
| `make tidy` | Tidy `go.mod` / `go.sum` |

Please also review our [Code of Conduct](CODE_OF_CONDUCT.md) and [Security Policy](SECURITY.md).

## License

lazygh is released under the [MIT License](LICENSE).
