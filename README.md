# Lazy GitHub (lazygh) - Architectural Roadmap

This document outlines the epics and work items required to build `lazygh` into a production-grade, extensible terminal UI application.

> **Configuration:** lazygh reads an optional YAML config file at `~/.config/lazygh/config.yml`. See [docs/configuration.md](docs/configuration.md).

## Epic 0: Repository Foundations & Project Hygiene
Before writing feature code, establish the baseline scaffolding, quality gates, and contributor-facing files that every production repository needs.

* **Continuous Integration Quality Gate:**
    * Add a GitHub Actions workflow (`.github/workflows/ci.yml`) triggered on every pull request and push to `main`.
    * Run `go build ./...`, `go test ./...`, `go vet ./...`, and `golangci-lint run` as required status checks.
    * Cache Go modules to keep runs fast; pin action versions (SHA) for reproducibility.
    * Enforce `gofmt`/`goimports` formatting so style is verified automatically.
* **Community Health & Governance Files:**
    * Add an OSI-approved `LICENSE` (MIT) so usage terms are unambiguous.
    * Write `CONTRIBUTING.md` covering local setup, branch/PR conventions, and how to run tests and lint locally.
    * Add `CODE_OF_CONDUCT.md` (Contributor Covenant) to set community expectations.
    * Add `SECURITY.md` describing supported versions and the private vulnerability-disclosure process.
* **GitHub Templates & Review Routing:**
    * Add issue templates under `.github/ISSUE_TEMPLATE/` for bug reports and feature requests, plus a `config.yml` to route questions elsewhere.
    * Add `.github/PULL_REQUEST_TEMPLATE.md` with a checklist (tests pass, lint clean, docs updated).
    * Add `.github/CODEOWNERS` so reviews are auto-requested from the right owners.
* **Developer Tooling & Repo Hygiene:**
    * Add a Go `.gitignore` (compiled binaries, build output, editor/OS cruft, local `.local/state/lazygh` logs).
    * Add `.editorconfig` to standardize indentation and line endings across editors.
    * Add `.golangci.yml` pinning the linter set and rules that CI enforces.
    * Add a `Makefile` exposing common targets: `build`, `test`, `lint`, `run`, `fmt`.
    * Extend Dependabot (already covering Go modules via `.github/dependabot.yml`) to also update GitHub Actions and group minor/patch bumps.

## Epic 1: Architectural Foundations & State Management
Before adding new GitHub features, the core application skeleton must be refactored to support multiple contexts (PRs, Issues, Actions) without blocking the UI.

* **Sub-Model Routing (The Elm Architecture):**
    * Refactor the root `Model` to act as a router (handling state transitions like `stateRepoSelection` and `statePRView`). 
    * Extract the current PR logic into a dedicated `pr.Model` (implementing `Init`, `Update`, `View`).
    * Create placeholders for `issue.Model` and `action.Model`.
    * Implement global navigation (e.g., `1`, `2`, `3` to switch between PRs, Issues, Actions).
* **Asynchronous Data Layer & Caching:**
    * Implement an in-memory cache for API responses to prevent re-fetching data when switching between tabs or returning from a sub-view.
    * Introduce `bubbles/spinner` or a progress indicator for non-blocking background refreshes.
    * Ensure all `tea.Cmd` network requests handle timeouts gracefully.
* **TUI-Safe Logging System:**
    * Standard `fmt.Println` or `log.Fatal` destroys the TUI layout. Implement a hidden file-logger (using Go 1.21's `slog` or Uber's `zap`).
    * Route all debug information, API limits, and errors to `.local/state/lazygh/app.log`.
* **Configuration & Theming Management:**
    * Introduce a configuration parser (e.g., `viper`) to read a `~/.config/lazygh/config.yml` file.
    * Allow users to override default keybindings (e.g., changing checkout from `c` to `enter`).
    * Allow custom color palettes for `lipgloss` styles (overriding the default purple/blue).

## Epic 2: The Core Experience (Pull Requests)
Elevating the current PR implementation from a read-only list to a full workflow replacement.

* **Advanced Filtering & Sorting:**
    * Integrate `bubbles/textinput` to allow pressing `/` to fuzzy-search PR titles.
    * Add filters for "My PRs," "Needs my Review," and "Dependabot."
* **Interactive Review Workflow:**
    * Implement a new right-pane tab view: toggle between "Description", "Files Changed" (diff view), and "Comments."
    * Add a keybinding (e.g., `v`) to execute `gh pr diff` and render the syntax-highlighted diff in the viewport using a library like `alecthomas/chroma`.
    * Add quick actions: `a` (Approve), `m` (Merge), `d` (Draft/Close).
* **CI/CD Status Integration:**
    * Update the PR list to visually indicate the GitHub Actions status (✅ Pass, ❌ Fail, 🔄 Pending) directly in the left list pane.

## Epic 3: Expanding the Domain (Issues & Actions)
Bringing the rest of the daily GitHub workflow into the terminal.

* **Issue Management (`issue.Model`):**
    * Fetch and list open issues for the repository.
    * Render issue descriptions and comment threads in the viewport.
    * Keybindings for quick assignment (assign to self) and state changes (close issue).
* **GitHub Actions Monitor (`action.Model`):**
    * List recent workflow runs for the current repository.
    * Allow users to select a failed run and view the failure logs in the right viewport.
    * Add a keybinding (e.g., `r`) to re-run a failed workflow.

## Epic 4: UX Polish & Accessibility
Ensuring the tool feels exactly like `lazygit` and is immediately intuitive to new users.

* **Dynamic Help Overlay:**
    * Implement `bubbles/help`.
    * Map `?` to trigger a modal overlay that displays all available keybindings contextually (e.g., showing PR-specific keys only when the PR pane is active).
* **Global Status Bar:**
    * Pin a robust status bar to the bottom of the screen.
    * Display current repository, active branch, GitHub API rate limit status, and application version.
* **Graceful Degradation:**
    * Handle window resizing events (`tea.WindowSizeMsg`) more robustly, hiding the right pane entirely if the terminal drops below a certain width (e.g., `< 80 cols`).

## Epic 5: CI/CD & Distribution
Getting the tool out of your local environment and into the hands of other engineers.

* **Testing Suite:**
    * Write table-driven unit tests for the Bubble Tea `Update` functions (testing state changes based on simulated `tea.KeyMsg` inputs).
    * Mock the `go-gh` API responses to test UI rendering without hitting live endpoints.
* **Automated Release Pipeline:**
    * Reuse the CI quality gate from Epic 0 as a required check, then trigger the release workflow on version tags (`v*`).
    * Integrate `goreleaser` to automatically compile binaries for macOS, Linux, and Windows, generate a GitHub Release, and publish to a Homebrew tap on git tags.
