# Router + Pull Requests Sub-Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure the monolithic `tui.Model` into a thin state router that delegates to independent, per-screen sub-models, extracting repo-selection and pull-request logic into their own packages — with no change to user-visible behavior.

**Architecture:** The root `tui.Model` becomes a pure router implementing bubbletea's `tea.Model`; it owns only the active view, the child screens, global keys, window-resize propagation, and a sticky error overlay. Child screens (`repolist.Model`, `pr.Model`) implement a shared `screen.Model` interface (`Init`/`Update`/`View`, where `Update` returns `screen.Model`) so the router forwards messages generically without type assertions. Children signal upward via exported messages (`repolist.RepoSelectedMsg`) and report failures via `screen.ErrMsg`. Dependency flow is a one-way DAG: `router → {repolist, pr} → {screen, styles, github}`.

**Tech Stack:** Go 1.25, Bubble Tea (`charmbracelet/bubbletea`), Bubbles (`charmbracelet/bubbles/viewport`), Lipgloss (`charmbracelet/lipgloss`), `cli/go-gh`.

**Reference spec:** `docs/superpowers/specs/2026-07-06-router-pr-submodel-design.md`

**Build order rationale:** Tasks are ordered bottom-up so the project builds green at every commit. Foundation packages (Task 1) and the leaf screens (Tasks 2–3) are added while the existing `ui.go` still runs the app; the cutover (Task 4) atomically replaces `ui.go` with the router in a single commit to avoid duplicate `Model`/`NewModel` definitions.

---

### Task 1: Foundation packages (`screen` + `styles`)

**Goal:** Create the shared `screen.Model` contract + `screen.ErrMsg`, and move the five shared lipgloss styles into their own package. These are leaf packages the screens will depend on.

**Files:**
- Create: `internal/tui/screen/screen.go`
- Create: `internal/tui/styles/styles.go`

**Acceptance Criteria:**
- [ ] `screen.Model` interface declares `Init() tea.Cmd`, `Update(tea.Msg) (Model, tea.Cmd)`, `View() string`.
- [ ] `screen.ErrMsg` wraps an `error`.
- [ ] `styles` exports `Base`, `Active`, `SelectedItem`, `Title`, `Menu` with identical values to the current `ui.go` styles.
- [ ] `go build ./...` and `go vet ./...` pass (existing `ui.go` untouched, so the app still runs).

**Verify:** `go build ./... && go vet ./...` → no output, exit 0.

> **Note:** This task is pure type/variable declaration — there is no behavior to unit-test, so it is verified by compilation only. TDD resumes in Task 2 where behavior begins.

**Steps:**

- [ ] **Step 1: Create `internal/tui/screen/screen.go`**

```go
// Package screen defines the contract every routable sub-model implements,
// plus the cross-cutting messages the router handles centrally.
package screen

import tea "github.com/charmbracelet/bubbletea"

// Model is a self-contained, Elm-style sub-model hosted by the router. Only the
// router implements bubbletea's tea.Model; child screens implement this
// interface instead, so the router can forward messages generically without
// type assertions.
type Model interface {
	Init() tea.Cmd
	Update(tea.Msg) (Model, tea.Cmd)
	View() string
}

// ErrMsg reports a fatal error up to the router, which renders a global error
// overlay until the user quits.
type ErrMsg struct{ Err error }
```

- [ ] **Step 2: Create `internal/tui/styles/styles.go`**

```go
// Package styles holds the shared lipgloss styles used across the TUI screens.
package styles

import "github.com/charmbracelet/lipgloss"

var (
	Base         = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240"))
	Active       = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62"))
	SelectedItem = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	Title        = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1).Bold(true)
	Menu         = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(1, 2)
)
```

- [ ] **Step 3: Verify build + vet**

Run: `go build ./... && go vet ./...`
Expected: no output, exit 0.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/screen/screen.go internal/tui/styles/styles.go
git commit -m "feat: add screen contract and shared styles packages (#1)"
```

---

### Task 2: `repolist.Model` (repository-selection screen)

**Goal:** Extract the repository-selection screen — list, cursor, loading, rendering, and the repo-fetch command — into `internal/tui/repolist`, implementing `screen.Model` and emitting `RepoSelectedMsg` on selection.

**Files:**
- Create: `internal/tui/repolist/repolist.go`
- Test: `internal/tui/repolist/repolist_test.go`

**Acceptance Criteria:**
- [ ] `repolist.New()` returns a screen in the loading state; `Init()` returns the repo-fetch command.
- [ ] `up`/`k` and `down`/`j` move the cursor and clamp at both ends.
- [ ] `enter` on a populated list emits `RepoSelectedMsg{Owner, Name}` for the highlighted repo; on an empty list it is a no-op (nil command).
- [ ] `reposMsg` populates the list and clears `loading`.
- [ ] Fetch failures return `screen.ErrMsg`.
- [ ] `go test ./internal/tui/repolist/...` passes; `go build ./...` and `go vet ./...` pass.

**Verify:** `go test ./internal/tui/repolist/... -v` → all PASS; `go build ./... && go vet ./...` → exit 0.

**Steps:**

- [ ] **Step 1: Write the failing test — `internal/tui/repolist/repolist_test.go`**

```go
package repolist

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	ghClient "github.com/ChristopherBilg/lazygh/internal/github"
)

func repo(owner, name string) ghClient.Repository {
	r := ghClient.Repository{Name: name}
	r.Owner.Login = owner
	return r
}

func TestUpdateCursorNavigation(t *testing.T) {
	repos := []ghClient.Repository{repo("o", "a"), repo("o", "b"), repo("o", "c")}

	tests := []struct {
		name       string
		startCur   int
		key        tea.KeyMsg
		wantCursor int
	}{
		{"up at top stays", 0, tea.KeyMsg{Type: tea.KeyUp}, 0},
		{"down moves", 0, tea.KeyMsg{Type: tea.KeyDown}, 1},
		{"k at top stays", 0, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, 0},
		{"j moves", 1, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, 2},
		{"down at bottom stays", 2, tea.KeyMsg{Type: tea.KeyDown}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{repos: repos, cursor: tt.startCur}
			updated, _ := m.Update(tt.key)
			if got := updated.(Model).cursor; got != tt.wantCursor {
				t.Fatalf("cursor = %d, want %d", got, tt.wantCursor)
			}
		})
	}
}

func TestEnterEmitsRepoSelectedMsg(t *testing.T) {
	repos := []ghClient.Repository{repo("octocat", "hello"), repo("octocat", "world")}
	m := Model{repos: repos, cursor: 1}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	msg := cmd()
	sel, ok := msg.(RepoSelectedMsg)
	if !ok {
		t.Fatalf("expected RepoSelectedMsg, got %T", msg)
	}
	if sel.Owner != "octocat" || sel.Name != "world" {
		t.Fatalf("got %+v, want {octocat world}", sel)
	}
}

func TestEnterEmptyListNoOp(t *testing.T) {
	m := Model{}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("expected nil command for empty list")
	}
}

func TestReposMsgPopulates(t *testing.T) {
	m := Model{loading: true}
	updated, _ := m.Update(reposMsg([]ghClient.Repository{repo("o", "a")}))
	um := updated.(Model)
	if um.loading {
		t.Fatal("expected loading=false after reposMsg")
	}
	if len(um.repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(um.repos))
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/repolist/... -v`
Expected: FAIL — build error (`undefined: Model`, `undefined: reposMsg`, `undefined: RepoSelectedMsg`).

- [ ] **Step 3: Write the implementation — `internal/tui/repolist/repolist.go`**

```go
// Package repolist implements the repository-selection screen.
package repolist

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	ghClient "github.com/ChristopherBilg/lazygh/internal/github"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
	"github.com/ChristopherBilg/lazygh/internal/tui/styles"
)

// Model is the repository-selection screen.
type Model struct {
	repos   []ghClient.Repository
	cursor  int
	loading bool
	width   int
	height  int
}

// New returns a repository-selection screen in its initial loading state.
func New() Model {
	return Model{loading: true}
}

// RepoSelectedMsg is emitted upward when the user selects a repository.
type RepoSelectedMsg struct {
	Owner string
	Name  string
}

// reposMsg carries the fetched repositories.
type reposMsg []ghClient.Repository

// fetchReposCmd fetches the authenticated user's repositories.
func fetchReposCmd() tea.Cmd {
	return func() tea.Msg {
		repos, err := ghClient.FetchUserRepositories()
		if err != nil {
			return screen.ErrMsg{Err: err}
		}
		return reposMsg(repos)
	}
}

// Init starts fetching repositories.
func (m Model) Init() tea.Cmd {
	return fetchReposCmd()
}

// Update handles navigation and selection.
func (m Model) Update(msg tea.Msg) (screen.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case reposMsg:
		m.repos = msg
		m.loading = false

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.repos)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.repos) > 0 {
				selected := m.repos[m.cursor]
				return m, func() tea.Msg {
					return RepoSelectedMsg{Owner: selected.Owner.Login, Name: selected.Name}
				}
			}
		}
	}
	return m, nil
}

// View renders the repository list.
func (m Model) View() string {
	if m.loading {
		return "\n  Fetching your repositories...\n"
	}

	s := " Select a Repository:\n\n"

	start := 0
	end := len(m.repos)
	maxVisible := m.height - 10
	if end > maxVisible {
		end = maxVisible
	}

	for i := start; i < end; i++ {
		cursor := "  "
		repoName := m.repos[i].FullName
		if m.cursor == i {
			cursor = "> "
			repoName = styles.SelectedItem.Render(repoName)
		}
		s += fmt.Sprintf("%s%s\n", cursor, repoName)
	}

	if len(m.repos) > maxVisible {
		s += fmt.Sprintf("\n  ...and %d more.\n", len(m.repos)-maxVisible)
	}

	box := styles.Menu.Width(m.width / 2).Render(s)
	centeredBox := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, box)

	footer := " [j/k] Navigate  •  [enter] Select  •  [q] Quit"
	return fmt.Sprintf("\n%s\n\n%s", centeredBox, lipgloss.PlaceHorizontal(m.width, lipgloss.Center, footer))
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/tui/repolist/... -v`
Expected: PASS (all four tests).

- [ ] **Step 5: Verify build + vet**

Run: `go build ./... && go vet ./...`
Expected: no output, exit 0.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/repolist/
git commit -m "feat: extract repository-selection screen into repolist package (#1)"
```

---

### Task 3: `pr.Model` (pull-request split-pane screen)

**Goal:** Extract the PR split-pane — context, cursor, focus, viewport, status message, resize/content logic, and the fetch/checkout/open commands — into `internal/tui/pr`, implementing `screen.Model`. Preserve today's exact `Update` sequencing (message handled, then the raw message forwarded to the embedded viewport).

**Files:**
- Create: `internal/tui/pr/pr.go`
- Test: `internal/tui/pr/pr_test.go`

**Acceptance Criteria:**
- [ ] `pr.New(owner, name, w, h)` returns a loading screen whose viewport is sized immediately; `Init()` returns the PR-fetch command.
- [ ] `tab`/`shift+tab` toggles focus between list and details.
- [ ] `up`/`k` and `down`/`j` move the PR cursor (clamped) when focused on the list.
- [ ] `c`/`o` on a populated list set the pending status message and emit a command; on an empty list they are a no-op.
- [ ] `prDataMsg` populates context and clears `loading`; `statusMsg` sets the footer message.
- [ ] Command failures return `screen.ErrMsg`.
- [ ] `go test ./internal/tui/pr/...` passes; `go build ./...` and `go vet ./...` pass.

**Verify:** `go test ./internal/tui/pr/... -v` → all PASS; `go build ./... && go vet ./...` → exit 0.

**Steps:**

- [ ] **Step 1: Write the failing test — `internal/tui/pr/pr_test.go`**

```go
package pr

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	ghClient "github.com/ChristopherBilg/lazygh/internal/github"
)

// withPRs builds a loaded PR screen with n synthetic pull requests.
func withPRs(n int) Model {
	m := New("octocat", "hello", 100, 40)
	m.loading = false
	prs := make([]ghClient.PullRequest, n)
	for i := range prs {
		prs[i] = ghClient.PullRequest{Number: i + 1, Title: "Title", State: "open"}
	}
	m.ctx.PRs = prs
	m.updateViewportContent()
	return m
}

func TestTabTogglesFocus(t *testing.T) {
	m := withPRs(2)
	if m.focus != focusList {
		t.Fatalf("initial focus = %d, want focusList", m.focus)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if updated.(Model).focus != focusDetails {
		t.Fatal("tab did not switch focus to details")
	}
	updated2, _ := updated.(Model).Update(tea.KeyMsg{Type: tea.KeyTab})
	if updated2.(Model).focus != focusList {
		t.Fatal("second tab did not switch focus back to list")
	}
}

func TestCursorNavigationInList(t *testing.T) {
	tests := []struct {
		name       string
		start      int
		key        tea.KeyMsg
		wantCursor int
	}{
		{"down moves", 0, tea.KeyMsg{Type: tea.KeyDown}, 1},
		{"up moves", 2, tea.KeyMsg{Type: tea.KeyUp}, 1},
		{"up at top stays", 0, tea.KeyMsg{Type: tea.KeyUp}, 0},
		{"down at bottom stays", 2, tea.KeyMsg{Type: tea.KeyDown}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := withPRs(3)
			m.cursor = tt.start
			updated, _ := m.Update(tt.key)
			if got := updated.(Model).cursor; got != tt.wantCursor {
				t.Fatalf("cursor = %d, want %d", got, tt.wantCursor)
			}
		})
	}
}

func TestCheckoutEmitsCommandAndSetsMessage(t *testing.T) {
	m := withPRs(2)
	m.cursor = 1
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd == nil {
		t.Fatal("expected a command from checkout")
	}
	if got := updated.(Model).message; got != "Checking out branch..." {
		t.Fatalf("message = %q, want %q", got, "Checking out branch...")
	}
}

func TestOpenEmitsCommandAndSetsMessage(t *testing.T) {
	m := withPRs(2)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd == nil {
		t.Fatal("expected a command from open")
	}
	if got := updated.(Model).message; got != "Opening browser..." {
		t.Fatalf("message = %q, want %q", got, "Opening browser...")
	}
}

func TestCheckoutEmptyListNoOp(t *testing.T) {
	m := withPRs(0)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd != nil {
		t.Fatal("expected no command for empty PR list")
	}
	if got := updated.(Model).message; got != "" {
		t.Fatalf("message = %q, want empty", got)
	}
}

func TestPRDataMsgPopulates(t *testing.T) {
	m := New("octocat", "hello", 100, 40)
	ctx := ghClient.RepoContext{
		Owner: "octocat",
		Name:  "hello",
		PRs:   []ghClient.PullRequest{{Number: 1, Title: "T", State: "open"}, {Number: 2}},
	}
	updated, _ := m.Update(prDataMsg(ctx))
	um := updated.(Model)
	if um.loading {
		t.Fatal("expected loading=false after prDataMsg")
	}
	if len(um.ctx.PRs) != 2 {
		t.Fatalf("expected 2 PRs, got %d", len(um.ctx.PRs))
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/pr/... -v`
Expected: FAIL — build error (`undefined: New`, `undefined: Model`, `undefined: prDataMsg`, `undefined: focusList`).

- [ ] **Step 3: Write the implementation — `internal/tui/pr/pr.go`**

```go
// Package pr implements the pull-request split-pane screen.
package pr

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	ghClient "github.com/ChristopherBilg/lazygh/internal/github"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
	"github.com/ChristopherBilg/lazygh/internal/tui/styles"
)

// Focus targets within the split pane.
const (
	focusList = iota
	focusDetails
)

// Model is the pull-request split-pane screen.
type Model struct {
	ctx      ghClient.RepoContext
	cursor   int
	focus    int
	loading  bool
	message  string
	viewport viewport.Model
	width    int
	height   int
	ready    bool
}

// New returns a PR screen for the given repository, sized to the current window.
// The repository owner/name are stored immediately so the loading view can show
// the repository name (a deliberate, approved improvement over the previous
// blank name on first load).
func New(owner, name string, width, height int) Model {
	m := Model{
		ctx:     ghClient.RepoContext{Owner: owner, Name: name},
		focus:   focusList,
		loading: true,
		width:   width,
		height:  height,
	}
	m.resizeViewport()
	return m
}

// prDataMsg carries fetched pull-request data.
type prDataMsg ghClient.RepoContext

// statusMsg carries a transient footer status message.
type statusMsg string

// fetchPRsCmd fetches the open PRs for the given repository.
func fetchPRsCmd(owner, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, err := ghClient.FetchRepoPRs(owner, name)
		if err != nil {
			return screen.ErrMsg{Err: err}
		}
		return prDataMsg(ctx)
	}
}

func checkoutCmd(prNumber int) tea.Cmd {
	return func() tea.Msg {
		if err := ghClient.CheckoutPR(prNumber); err != nil {
			return screen.ErrMsg{Err: err}
		}
		return statusMsg(fmt.Sprintf("Successfully checked out PR #%d", prNumber))
	}
}

func openBrowserCmd(prNumber int) tea.Cmd {
	return func() tea.Msg {
		if err := ghClient.OpenPRInBrowser(prNumber); err != nil {
			return screen.ErrMsg{Err: err}
		}
		return statusMsg(fmt.Sprintf("Opened PR #%d in browser", prNumber))
	}
}

// Init starts fetching pull requests for this screen's repository.
func (m Model) Init() tea.Cmd {
	return fetchPRsCmd(m.ctx.Owner, m.ctx.Name)
}

// Update handles focus, navigation, PR actions, and data messages. It preserves
// the original sequencing: handle the message, then forward the raw message to
// the embedded viewport (so mouse-scroll and the viewport's own keymap behave
// exactly as before).
func (m Model) Update(msg tea.Msg) (screen.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewport()

	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab":
			if m.focus == focusList {
				m.focus = focusDetails
			} else {
				m.focus = focusList
			}
		case "up", "k":
			if m.focus == focusList {
				if m.cursor > 0 {
					m.cursor--
					m.updateViewportContent()
					m.viewport.GotoTop()
				}
			} else {
				m.viewport.LineUp(1)
			}
		case "down", "j":
			if m.focus == focusList {
				if m.cursor < len(m.ctx.PRs)-1 {
					m.cursor++
					m.updateViewportContent()
					m.viewport.GotoTop()
				}
			} else {
				m.viewport.LineDown(1)
			}
		case "c":
			if len(m.ctx.PRs) > 0 {
				m.message = "Checking out branch..."
				cmds = append(cmds, checkoutCmd(m.ctx.PRs[m.cursor].Number))
			}
		case "o":
			if len(m.ctx.PRs) > 0 {
				m.message = "Opening browser..."
				cmds = append(cmds, openBrowserCmd(m.ctx.PRs[m.cursor].Number))
			}
		}

	case prDataMsg:
		m.ctx = ghClient.RepoContext(msg)
		m.loading = false
		m.updateViewportContent()

	case statusMsg:
		m.message = string(msg)
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) resizeViewport() {
	headerHeight := 4
	footerHeight := 2
	contentHeight := m.height - headerHeight - footerHeight
	rightPaneWidth := (m.width * 7) / 10

	if !m.ready {
		m.viewport = viewport.New(rightPaneWidth-2, contentHeight-2)
		m.ready = true
	} else {
		m.viewport.Width = rightPaneWidth - 2
		m.viewport.Height = contentHeight - 2
	}
	m.updateViewportContent()
}

func (m *Model) updateViewportContent() {
	if len(m.ctx.PRs) == 0 {
		m.viewport.SetContent("No open PRs.")
		return
	}
	activePR := m.ctx.PRs[m.cursor]

	contentStyle := lipgloss.NewStyle().Width(m.viewport.Width)

	body := activePR.Body
	if body == "" {
		body = "*No description provided.*"
	}

	fullText := fmt.Sprintf("%s\nState: %s\n\n%s",
		styles.Title.Render(activePR.Title),
		activePR.State,
		body)

	m.viewport.SetContent(contentStyle.Render(fullText))
}

// View renders the split pane, or a loading message while PRs are fetched.
func (m Model) View() string {
	if m.loading {
		return fmt.Sprintf("\n  Fetching PRs for %s...\n", m.ctx.Name)
	}

	leftPaneWidth := (m.width * 3) / 10
	paneHeight := m.height - 6

	listStr := ""
	for i, pr := range m.ctx.PRs {
		cursorStr := "  "
		title := fmt.Sprintf("#%d %s", pr.Number, pr.Title)

		if len(title) > leftPaneWidth-6 {
			title = title[:leftPaneWidth-9] + "..."
		}

		if m.cursor == i {
			cursorStr = "> "
			title = styles.SelectedItem.Render(title)
		}
		listStr += fmt.Sprintf("%s%s\n", cursorStr, title)
	}

	var listBorder, detailBorder lipgloss.Style
	if m.focus == focusList {
		listBorder = styles.Active
		detailBorder = styles.Base
	} else {
		listBorder = styles.Base
		detailBorder = styles.Active
	}

	left := listBorder.Width(leftPaneWidth).Height(paneHeight).Render(listStr)
	right := detailBorder.Width(m.viewport.Width + 2).Height(paneHeight).Render(m.viewport.View())

	header := fmt.Sprintf(" Lazy GitHub | %s/%s \n\n", m.ctx.Owner, m.ctx.Name)
	ui := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	footerText := " [esc] Change Repo  •  [tab] Focus  •  [j/k] Scroll  •  [c] Checkout  •  [o] Web  •  [q] Quit"
	if m.message != "" {
		footerText = fmt.Sprintf(" %s | %s", styles.Title.Render(m.message), footerText)
	}

	return header + ui + "\n\n" + footerText
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/tui/pr/... -v`
Expected: PASS (all six tests).

- [ ] **Step 5: Verify build + vet**

Run: `go build ./... && go vet ./...`
Expected: no output, exit 0.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/pr/
git commit -m "feat: extract pull-request split-pane into pr package (#1)"
```

---

### Task 4: Router cutover (replace `ui.go` with `router.go`)

**Goal:** Replace the monolithic `internal/tui/ui.go` with a thin router in `internal/tui/router.go` that owns only routing state and delegates to the `repolist` and `pr` screens. Keep `tui.NewModel()`'s signature so `cmd/lazygh/main.go` is unchanged. This is the atomic cutover to the new architecture.

**Files:**
- Create: `internal/tui/router.go`
- Delete: `internal/tui/ui.go`
- Test: `internal/tui/router_test.go`

**Acceptance Criteria:**
- [ ] `tui.NewModel()` returns a router with the repo-list active; `Init()` starts the repo fetch.
- [ ] `q`/`ctrl+c` quit globally; `esc`/`backspace` return from the PR view to the repo list (no-op on the repo list).
- [ ] `repolist.RepoSelectedMsg` builds a `pr` screen sized to the current window, switches to it, and runs its `Init`.
- [ ] `tea.WindowSizeMsg` is stored and broadcast to every held child.
- [ ] `screen.ErrMsg` sets a sticky error rendered as the full-screen overlay.
- [ ] `internal/tui/ui.go` no longer exists; `cmd/lazygh/main.go` is unchanged.
- [ ] `go test ./...` passes; `go build ./...` and `go vet ./...` pass; `gofmt -l internal cmd` prints nothing.

**Verify:** `go build ./... && go vet ./... && go test ./... && gofmt -l internal cmd` → all pass, `gofmt` prints nothing.

**Steps:**

- [ ] **Step 1: Write the failing test — `internal/tui/router_test.go`**

```go
package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ChristopherBilg/lazygh/internal/tui/repolist"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
)

func TestRepoSelectedSwitchesToPRView(t *testing.T) {
	m := NewModel()
	updated, cmd := m.Update(repolist.RepoSelectedMsg{Owner: "octocat", Name: "hello"})
	rm := updated.(Model)
	if rm.active != viewPR {
		t.Fatal("expected active view to be viewPR after RepoSelectedMsg")
	}
	if rm.current == nil {
		t.Fatal("expected current screen to be set")
	}
	if cmd == nil {
		t.Fatal("expected an init command (PR fetch) after selection")
	}
}

func TestEscReturnsToRepoList(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(repolist.RepoSelectedMsg{Owner: "o", Name: "n"})
	afterSelect := updated.(Model)
	if afterSelect.active != viewPR {
		t.Fatal("precondition: should be in PR view")
	}
	back, _ := afterSelect.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if back.(Model).active != viewRepoList {
		t.Fatal("expected esc to return to repo list")
	}
}

func TestErrMsgSetsError(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(screen.ErrMsg{Err: errors.New("boom")})
	if updated.(Model).err == nil {
		t.Fatal("expected err to be set after ErrMsg")
	}
}

func TestWindowSizeStored(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 123, Height: 45})
	rm := updated.(Model)
	if rm.width != 123 || rm.height != 45 {
		t.Fatalf("dimensions = %dx%d, want 123x45", rm.width, rm.height)
	}
}

func TestQuitOnQ(t *testing.T) {
	m := NewModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected quit command on 'q'")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected command to be tea.Quit")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/... -v`
Expected: FAIL — `undefined: viewPR`, `undefined: viewRepoList`, and/or a duplicate-symbol build error once `router.go` is added alongside `ui.go`. (This confirms the router symbols don't exist yet.)

- [ ] **Step 3: Delete the old monolith**

```bash
git rm internal/tui/ui.go
```

- [ ] **Step 4: Write the implementation — `internal/tui/router.go`**

```go
// Package tui contains the root router model that hosts and switches between
// the application's screens.
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ChristopherBilg/lazygh/internal/tui/pr"
	"github.com/ChristopherBilg/lazygh/internal/tui/repolist"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
)

// view identifies which screen is currently active.
type view int

const (
	viewRepoList view = iota
	viewPR
)

// Model is the root router. It owns only routing state: the active view, the
// child screens, the sticky global error, and the window dimensions it
// propagates.
type Model struct {
	active   view
	repoList screen.Model
	current  screen.Model
	err      error
	width    int
	height   int
}

// NewModel returns the root model with the repository-selection screen active.
func NewModel() Model {
	return Model{
		active:   viewRepoList,
		repoList: repolist.New(),
	}
}

// Init starts the active screen (fetches repositories).
func (m Model) Init() tea.Cmd {
	return m.repoList.Init()
}

// Update routes messages: it handles global keys, resize propagation, upward
// signals, and errors, and forwards everything else to the active screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc", "backspace":
			if m.active != viewRepoList {
				m.active = viewRepoList
				return m, nil
			}
		}
		return m.forward(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m.broadcastResize(msg)

	case repolist.RepoSelectedMsg:
		m.current = pr.New(msg.Owner, msg.Name, m.width, m.height)
		m.active = viewPR
		return m, m.current.Init()

	case screen.ErrMsg:
		m.err = msg.Err
		return m, nil
	}

	return m.forward(msg)
}

// forward sends a message to the active screen and stores the returned model.
func (m Model) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.active {
	case viewRepoList:
		m.repoList, cmd = m.repoList.Update(msg)
	default:
		m.current, cmd = m.current.Update(msg)
	}
	return m, cmd
}

// broadcastResize forwards a window-size message to every held screen so none
// renders at a stale size after navigation.
func (m Model) broadcastResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	m.repoList, cmd = m.repoList.Update(msg)
	cmds = append(cmds, cmd)

	if m.current != nil {
		m.current, cmd = m.current.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the global error overlay, or delegates to the active screen.
func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  Error: %v\n\n  Press 'q' to quit.\n", m.err)
	}

	switch m.active {
	case viewRepoList:
		return m.repoList.View()
	default:
		return m.current.View()
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v`
Expected: PASS (router tests + repolist + pr tests).

- [ ] **Step 6: Full verification (build, vet, all tests, formatting)**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l internal cmd`
Expected: build/vet/test all pass; `gofmt -l` prints nothing (no unformatted files).

- [ ] **Step 7: Manual behavioral pass** (requires a TTY + authenticated `gh`)

Run: `go run ./cmd/lazygh`
Confirm, matching pre-refactor behavior:
- Repo list renders and `j`/`k` navigate.
- `enter` opens the PR split-pane for the selected repo.
- `j`/`k` move the PR list; `tab` toggles the active (highlighted-border) pane; scrolling works when details are focused.
- `c` shows "Checking out branch…" then the success status; `o` opens the PR in the browser.
- Resizing the terminal resizes both panes.
- `esc` returns to the repo list with the cursor preserved; re-selecting re-fetches.
- `q` / `ctrl+c` quits from either screen.

- [ ] **Step 8: Commit**

```bash
git add internal/tui/router.go internal/tui/router_test.go
git commit -m "refactor: replace monolithic model with thin state router (#1)"
```

---

## Self-Review

**1. Spec coverage:**
- Router holds only routing responsibilities → Task 4 (`Model` has only `active`, `repoList`, `current`, `err`, `width`, `height`).
- PR functionality in a dedicated, independently testable sub-model → Task 3 (`pr` package + tests).
- Repo selection extracted → Task 2 (`repolist` package + tests).
- Shared `screen.Model` interface + `screen.ErrMsg` → Task 1.
- Shared styles package → Task 1.
- Selecting a repo opens the PR pane; nav/focus/checkout/open unchanged → Tasks 3 + 4 (ported verbatim; router wires `RepoSelectedMsg` → `pr.New`).
- Window resize resizes both panes → Task 4 `broadcastResize` + Task 3 `resizeViewport`.
- Sticky global error overlay → Task 4 `View` + `screen.ErrMsg`.
- Builds and vets cleanly → Verify lines in every task; full gate in Task 4.
- Unit tests included → Tasks 2, 3, 4.
- Approved loading-message deviation → Task 3 `New` docstring + `View`.
- `main.go` / `github` unchanged → `NewModel()` signature preserved (Task 4).

**2. Placeholder scan:** No TBD/TODO/"handle edge cases"/"similar to". All code is complete and ported from the current `ui.go`.

**3. Type consistency:** `screen.Model` (Init/Update→`(screen.Model, tea.Cmd)`/View) is implemented identically by `repolist.Model` and `pr.Model`. `repolist.RepoSelectedMsg{Owner, Name}` is emitted in Task 2 and consumed in Task 4. `screen.ErrMsg{Err}` is emitted by both screens' commands and consumed by the router. `pr.New(owner, name, width, height)` signature matches its call site in the router. `styles.{Base,Active,SelectedItem,Title,Menu}` names match their uses in `repolist`/`pr`. Router field names (`active`, `repoList`, `current`, `err`, `width`, `height`) and `view` constants (`viewRepoList`, `viewPR`) match the router tests.

No gaps found.
