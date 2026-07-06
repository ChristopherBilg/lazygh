# Design: Refactor the root model into a state router and extract a Pull Requests sub-model

- **Issue:** #1 — "Refactor the root model into a state router and extract a Pull Requests sub-model"
- **Epic:** 1 — Architectural Foundations & State Management
- **Date:** 2026-07-06
- **Status:** Approved
- **Branch:** `refactor/router-pr-submodel`

## Context

Today the entire TUI lives in a single `tui.Model` (`internal/tui/ui.go`). That one
struct owns:

- top-level screen state (`stateRepoSelection` / `statePRView`),
- the repository list + cursor + its rendering,
- all pull-request list/detail logic, the viewport, focus, and status message,
- the shared `lipgloss` styles,
- every command (`fetchReposCmd`, `fetchPRsCmd`, `checkoutCmd`, `openBrowserCmd`) and
  message (`reposMsg`, `prDataMsg`, `statusMsg`, `errMsg`).

This does not scale as Issues and Actions views are added (issue #2 and Epic 3). The
Elm Architecture that Bubble Tea encourages wants each screen to be a self-contained
unit with its own `Init`/`Update`/`View`, and a thin parent that routes messages to the
active child.

The `internal/github` API layer is already cleanly separated and needs **no changes**.
`cmd/lazygh/main.go` calls `tui.NewModel()` and needs **no changes**.

## Goals

- Reduce the root model to **routing responsibilities only**: active view, global keys
  (quit, back), window-resize propagation, message forwarding to the active child, and
  the global error overlay.
- Extract pull-request functionality into a dedicated, independently testable sub-model.
- Preserve **all** user-visible behavior: repo selection opens the PR split-pane; list
  navigation, focus toggling, checkout, open-in-browser, and window resizing behave
  exactly as before.
- Establish the sub-model pattern that Issues and Actions (#2) will reuse.
- `go build ./...` and `go vet ./...` clean.

## Non-goals / Out of scope (deferred to #2 and later)

- Placeholder Issues and Actions views.
- Global `1`/`2`/`3` view navigation.
- Caching, spinners, timeouts, logging, config/theming (later Epic 1 issues).
- "Fixing" any pre-existing behavioral quirks (see Behavior Preservation). This is a
  pure refactor.

## Key decisions

1. **Repo selection is extracted into its own sub-model**, so the root is a *pure*
   router that switches between the repo-selection view and the PR view. Most faithful
   to the "only routing responsibilities" acceptance criterion.
2. **Separate packages per view** (`internal/tui/pr`, `internal/tui/repolist`, plus a
   shared `internal/tui/styles` and `internal/tui/screen`). This gives
   compiler-enforced isolation, matches the `bubbles` ecosystem convention
   (`list.Model`, `viewport.Model`, …) and the roadmap's `pr.Model` naming, and makes
   "independently testable" true by construction. Go's implicit interface satisfaction
   keeps the dependency graph acyclic (parent imports children, never the reverse).
3. **Shared `screen.Model` interface (Approach 2)** for the router↔child contract. Each
   child's `Update` returns `screen.Model`, so the router forwards generically with no
   type assertions. Children signal upward via exported messages. This keeps the router
   genuinely generic and lets #2 add views by creating packages, not by editing the
   router's control flow.
4. **Router-owned, sticky global error overlay** via `screen.ErrMsg`. This preserves the
   current behavior exactly: any error replaces the whole screen with
   `Error: … Press 'q' to quit.` and stays until the user quits. (The alternative —
   per-screen error rendering — would let `esc` escape the overlay, a behavior change.)

## Package structure

```
cmd/lazygh/main.go              UNCHANGED — runs tui.NewModel() in a tea.Program
internal/
  github/client.go              UNCHANGED — API layer
  tui/
    router.go                   Root Model (router); implements tea.Model. (renamed from ui.go)
    screen/screen.go            screen.Model interface + screen.ErrMsg
    styles/styles.go            Shared lipgloss styles: Base, Active, SelectedItem, Title, Menu
    repolist/repolist.go        repolist.Model — repository-selection screen
    repolist/repolist_test.go   table-driven Update tests
    pr/pr.go                    pr.Model — PR split-pane screen
    pr/pr_test.go               table-driven Update tests
```

Dependency DAG (one-way, acyclic):
`router → {repolist, pr} → {screen, styles, github}`.

> Naming: the shared contract package is named `screen` (interface `screen.Model`)
> rather than a generic `shared`, since Go discourages catch-all package names.
> `pr.Model` / `repolist.Model` then read as "a `screen.Model`."

## The Screen contract (`internal/tui/screen`)

```go
package screen

import tea "github.com/charmbracelet/bubbletea"

// Model is a self-contained, Elm-style sub-model that the router hosts.
type Model interface {
    Init() tea.Cmd
    Update(tea.Msg) (Model, tea.Cmd)
    View() string
}

// ErrMsg reports a fatal error up to the router's global error overlay.
type ErrMsg struct{ Err error }
```

Only the router implements bubbletea's `tea.Model`; children implement `screen.Model`.
Because `Update` returns `screen.Model`, the router forwards generically without type
assertions.

## Router (`internal/tui/router.go`, package `tui`)

Holds only routing state:

```go
type view int
const ( viewRepoList view = iota; viewPR )

type Model struct {
    active        view
    repoList      screen.Model // persistent — survives back-navigation (cursor preserved)
    current       screen.Model // active per-repo screen (pr now; pr/issue/action in #2)
    err           error        // global, sticky fatal error
    width, height int          // for window-resize propagation
}

func NewModel() Model // active=viewRepoList, repoList=repolist.New(); signature unchanged
```

Responsibilities. `Update` is a type switch with explicit cases plus a default that
forwards to the active screen:

- **Init:** `return m.repoList.Init()` (starts the repo fetch).
- **`tea.KeyMsg` — global keys first:** `q`/`ctrl+c` → `tea.Quit`; `esc`/`backspace` → if
  `active != viewRepoList`, switch back to `viewRepoList` and return (not forwarded to the
  child, matching today). Any **non-global** key falls through to the default (forwarded to
  the active screen).
- **`tea.WindowSizeMsg` — broadcast:** store `width`/`height`, then call `Update` on **every
  held child** (`repoList`, and `current` if non-nil), storing each returned `screen.Model`
  back. Broadcasting (rather than active-only) keeps the persistent `repoList` at the
  current size, so an `esc` back after a resize renders correctly — matching today's single
  model, which always rendered at the live size. Also pass current size into any screen it
  constructs. WindowSizeMsg is handled only here (it does not also hit the default).
- **`repolist.RepoSelectedMsg{Owner, Name}` — upward signal:**
  `m.current = pr.New(owner, name, m.width, m.height); m.active = viewPR; return m, m.current.Init()`.
- **`screen.ErrMsg`:** set `m.err` (sticky; never cleared except by quit).
- **default — forwarding:** send the message to the active screen's `Update` (chosen by
  `active`), storing the returned `screen.Model` back into the corresponding field
  (`repoList` when `active == viewRepoList`, else `current`).
- **View:** if `m.err != nil`, render the exact `Error: … Press 'q' to quit.` overlay
  first; else delegate to the active screen's `View()`.

`repoList` is **kept** across navigation so the cursor survives an `esc`. `current` (the
PR screen) is **recreated** on each repo selection, matching today's re-fetch-on-enter.

## `repolist.Model` (`internal/tui/repolist`)

Lifted from today's `updateRepoSelection` + `viewRepoSelection`.

- **State:** `repos []github.Repository`, `cursor int`, `loading bool`, `width`, `height`.
- **Constructor:** `New() Model` (starts `loading: true`).
- **Init:** `fetchReposCmd()` — returns `reposMsg` on success, `screen.ErrMsg` on failure.
- **Update:** `up`/`k` and `down`/`j` move the cursor; `enter` emits
  `RepoSelectedMsg{Owner, Name}` upward (via a `tea.Cmd`); `WindowSizeMsg` stores size;
  `reposMsg` fills the list and clears `loading`.
- **View:** centered menu, "…and N more" windowing, "Fetching your repositories…"
  loading text — unchanged.
- **Exports:** `New`, `Model`, and `RepoSelectedMsg struct{ Owner, Name string }`.

## `pr.Model` (`internal/tui/pr`)

Lifted from today's `updatePRView`, `resizeViewport`, `updateViewportContent`,
`viewPRView`.

- **State:** `ctx github.RepoContext`, `cursor int`, `focus int` (list/details),
  `loading bool`, `message string`, `viewport viewport.Model`, `width`, `height`,
  `ready bool`. `focusList`/`focusDetails` constants move here.
- **Constructor:** `New(owner, name string, w, h int) Model` — stores owner/name, sets
  `loading: true`, `focus: focusList`, and sizes the viewport immediately (replicating
  today's resize-on-entering-PR-view).
- **Init:** `fetchPRsCmd(owner, name)`.
- **Update:** `tab`/`shift+tab` toggle focus; `up`/`k` and `down`/`j` move the PR cursor
  (with `GotoTop`) when focused on the list, else line-scroll the viewport; `c` checkout;
  `o` open-in-browser; `prDataMsg` fills context + refreshes viewport content; `statusMsg`
  sets the footer message; `WindowSizeMsg` resizes. Commands return `screen.ErrMsg` on
  failure. **Preserves today's exact sequencing:** handle the message in the switch, then
  forward the raw message to the embedded `viewport` at the tail (so mouse-scroll and the
  viewport's built-in keymap behave identically, including the current double-handling of
  `j`/`k`).
- **View:** two bordered panes, focus-based border styling, header, footer with optional
  status message — unchanged.
- **Owns:** `fetchPRsCmd`, `checkoutCmd`, `openBrowserCmd`, and its `prDataMsg` /
  `statusMsg` types.

## Shared styles (`internal/tui/styles`)

The five `lipgloss` styles move here as exported vars, imported by `repolist` and `pr`:
`Base`, `Active`, `SelectedItem`, `Title`, `Menu`. (This package is also the natural home
for the theming work in issue #7.)

## Data flow (end-to-end)

- **Startup:** `main` → `tui.NewModel()` (active=repoList) → `router.Init()` →
  `repoList.Init()` → `fetchReposCmd` → `reposMsg` forwarded to repoList.
- **Select repo:** `enter` in repoList → `RepoSelectedMsg` → router builds `pr.New(...)`,
  switches to PR view, runs `pr.Init()` → `fetchPRsCmd` → `prDataMsg` forwarded to pr.
- **Back:** `esc` → router switches to repoList (same instance, cursor intact). Next
  `enter` builds a fresh pr and re-fetches.
- **Resize:** `WindowSizeMsg` → router stores size + broadcasts to all held children; pr
  resizes both panes via its viewport math, repolist stays current for centering.
- **Checkout / open:** `c`/`o` in pr → command runs → `statusMsg` (footer) or
  `screen.ErrMsg` (overlay).
- **Error:** any `screen.ErrMsg` → router sets sticky `err` → overlay until `q`.

## Behavior preservation (deliberate)

This is a pure refactor; pre-existing quirks are preserved rather than fixed:

- The tail viewport-update means some keys (e.g. `j` while focused on details) reach the
  viewport's built-in keymap *in addition to* the manual scroll. Kept as-is.
- Once any error occurs, it is sticky until quit (`esc` won't escape the overlay). Kept.

**Approved deviation (one, tiny):** today the "Fetching PRs for …" loading line shows a
*blank* repo name on first load (the old model read empty context). Since each `pr.Model`
now knows its repo at construction, it will show the actual repo name. This is a
sub-second, strictly-better change to a transient loading string. Approved by the user.

## Testing & verification

- **Build/vet (required by AC):** `go build ./...` and `go vet ./...` clean.
- **Unit tests (included in this issue):** table-driven `Update` tests for the extracted
  sub-models, demonstrating they can be exercised independently:
  - `repolist`: cursor clamps at both ends; `enter` on a populated list emits
    `RepoSelectedMsg` with the selected owner/name; `enter` on an empty list is a no-op;
    `reposMsg` populates the list and clears `loading`.
  - `pr`: `tab` toggles focus between list and details; `up`/`down` move the PR cursor
    within bounds when focused on the list; `c`/`o` on a populated list emit the checkout
    / open commands (and set the pending status message); `prDataMsg` populates context
    and clears `loading`. Commands that hit the network/`gh` are exercised only at the
    message-handling boundary (inject `prDataMsg`/`statusMsg`), not by calling the live
    API.
- **Manual behavioral pass** (needs live `gh` auth + a TTY): repo list navigates →
  select opens the split pane → `j/k` list nav → `tab` focus toggle → `c` checkout →
  `o` browser → resize redraws both panes → `esc` returns with cursor intact → error path
  shows the overlay.

## File-by-file change summary

- `internal/tui/ui.go` → **removed**, its logic distributed as below.
- `internal/tui/router.go` → **new**: root router `Model`, `view` enum, `NewModel`, global
  key/resize/error handling, generic forwarding, error overlay.
- `internal/tui/screen/screen.go` → **new**: `screen.Model` interface, `screen.ErrMsg`.
- `internal/tui/styles/styles.go` → **new**: the five shared styles.
- `internal/tui/repolist/repolist.go` → **new**: `repolist.Model`, `RepoSelectedMsg`,
  `fetchReposCmd`, `reposMsg`.
- `internal/tui/repolist/repolist_test.go` → **new**: table-driven `Update` tests.
- `internal/tui/pr/pr.go` → **new**: `pr.Model`, viewport logic, `fetchPRsCmd`,
  `checkoutCmd`, `openBrowserCmd`, `prDataMsg`, `statusMsg`, focus constants.
- `internal/tui/pr/pr_test.go` → **new**: table-driven `Update` tests.
- `cmd/lazygh/main.go` → **unchanged**.
- `internal/github/client.go` → **unchanged**.
