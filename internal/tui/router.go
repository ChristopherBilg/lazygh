// Package tui contains the root router model that hosts and switches between
// the application's screens.
package tui

import (
	"fmt"
	"log/slog"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	ghClient "github.com/ChristopherBilg/lazygh/internal/github"
	"github.com/ChristopherBilg/lazygh/internal/tui/action"
	"github.com/ChristopherBilg/lazygh/internal/tui/help"
	"github.com/ChristopherBilg/lazygh/internal/tui/issue"
	"github.com/ChristopherBilg/lazygh/internal/tui/keys"
	"github.com/ChristopherBilg/lazygh/internal/tui/pr"
	"github.com/ChristopherBilg/lazygh/internal/tui/repolist"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
)

// view aliases screen.ViewID so async result messages (defined in the sub-model
// packages) can address a specific screen without importing the router. The
// local constant names are kept for readability within the router. An alias
// rather than a defined type avoids explicit conversions at every call site
// (view and screen.ViewID are interchangeable); the tradeoff is that any
// view-specific method would have to live on screen.ViewID in the screen
// package.
type view = screen.ViewID

const (
	viewRepoList = screen.ViewRepoList
	viewPR       = screen.ViewPR
	viewIssues   = screen.ViewIssues
	viewActions  = screen.ViewActions
)

// perRepoViews lists the per-repo views in tab order (1/2/3). It drives the
// Init-batching and resize iteration order so that behavior is deterministic.
// (The perRepo map is built from an explicit literal in the RepoSelectedMsg
// handler, because the three screens have different constructor signatures.)
var perRepoViews = []view{viewPR, viewIssues, viewActions}

// Model is the root router. It owns only routing state: the injected GitHub
// client (threaded into each screen that needs it), the active view, the
// repository-selection screen, the per-repo screens (kept alive so switching
// between them preserves each one's selection and scroll position), the modal
// help overlay's visibility and scroll offset, the sticky global error, and the
// window dimensions it propagates.
type Model struct {
	client      *ghClient.Client // injected github client; satisfies repolist.Backend and pr.Backend
	active      view
	helpVisible bool         // the contextual keybindings overlay (?) is open
	helpScroll  int          // vertical scroll offset while the overlay is open
	repoList    screen.Model // persistent: survives back-navigation so the cursor is retained
	// perRepo holds the per-repo screens (pr/issue/action), built on selection
	// and kept alive so switching preserves each screen's state. It is mutated in
	// place via map-reference semantics; do not clone Model and expect
	// independent per-repo state.
	perRepo map[view]screen.Model
	// generation counts repo selections. It stamps each per-repo model (via
	// pr.New) so the router can drop async results addressed to a model that has
	// since been rebuilt for a different repo (issue #46).
	generation uint64
	err        error // sticky: once set, the overlay shows until quit
	width      int
	height     int
}

var _ tea.Model = Model{}

// NewModel returns the root model with the repository-selection screen active.
// The github client is injected and threaded into each screen that needs it.
func NewModel(client *ghClient.Client) Model {
	return Model{
		client:   client,
		active:   viewRepoList,
		repoList: repolist.New(client),
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
		if msg.String() == "ctrl+c" { // safety hatch: always quits, before any configurable match
			return m, tea.Quit
		}
		// The help overlay is modal: while open it swallows every key except its
		// own scroll keys, close keys (help/back), and quit — nothing reaches the
		// screen beneath.
		if m.helpVisible {
			switch {
			case key.Matches(msg, keys.Map.Quit):
				return m, tea.Quit
			case key.Matches(msg, keys.Map.Help), key.Matches(msg, keys.Map.Back):
				m.helpVisible = false
				m.helpScroll = 0
			case key.Matches(msg, keys.Map.Up):
				m.helpScroll = max(m.helpScroll-1, 0)
			case key.Matches(msg, keys.Map.Down):
				m.helpScroll = min(m.helpScroll+1, help.MaxScroll(m.active, m.height))
			}
			return m, nil
		}
		// While the active screen is capturing text input (e.g. the PR title
		// filter), forward every key to it so keystrokes like "1", "q", or "esc"
		// are typed into the field instead of switching views, quitting, or going
		// back. ctrl+c above is the sole exception.
		if s := m.activeScreen(); s != nil {
			if c, ok := s.(screen.InputCapturer); ok && c.CapturingInput() {
				return m.forward(msg)
			}
		}
		switch {
		case key.Matches(msg, keys.Map.Help):
			m.helpVisible = true
			m.helpScroll = 0
			return m, nil
		case key.Matches(msg, keys.Map.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Map.Back):
			if m.active != viewRepoList {
				m.active = viewRepoList
				return m, nil
			}
			// already at the repo list: fall through to forward (repolist ignores back)
		// Global view switch, only meaningful once a repo is selected; on the repo
		// list these keys fall out of the switch and are forwarded (repolist drops them).
		case key.Matches(msg, keys.Map.NavPRs):
			if m.active != viewRepoList {
				m.active = viewPR
				return m, nil
			}
		case key.Matches(msg, keys.Map.NavIssues):
			if m.active != viewRepoList {
				m.active = viewIssues
				return m, nil
			}
		case key.Matches(msg, keys.Map.NavActions):
			if m.active != viewRepoList {
				m.active = viewActions
				return m, nil
			}
		}
		return m.forward(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Keep the overlay's scroll offset in range when the terminal grows, so a
		// post-resize Up press isn't swallowed re-counting down from a stale offset.
		if m.helpVisible {
			m.helpScroll = min(m.helpScroll, help.MaxScroll(m.active, m.height))
		}
		return m.broadcastResize(msg)

	case repolist.RepoSelectedMsg:
		m.generation++
		m.perRepo = map[view]screen.Model{
			viewPR:      pr.New(m.client, msg.Owner, msg.Name, m.width, m.height, m.generation),
			viewIssues:  issue.New(m.width, m.height),
			viewActions: action.New(m.width, m.height),
		}
		m.active = viewPR

		var cmds []tea.Cmd
		for _, v := range perRepoViews {
			if s, ok := m.perRepo[v]; ok {
				cmds = append(cmds, s.Init())
			}
		}
		return m, tea.Batch(cmds...)

	case screen.ErrMsg:
		slog.Error("fatal view error", "err", msg.Err)
		m.err = msg.Err
		return m, nil
	}

	// Async fetch results are addressed to their originating view so they are
	// delivered even when the user has switched to another tab mid-fetch.
	if a, ok := msg.(screen.Addressed); ok {
		// Stale-result guard: drop an async result addressed to a per-repo view
		// that has been rebuilt for a different repo since the result was issued
		// (its generation no longer matches). The repo list is persistent — never
		// rebuilt — so its addressed results (generation 0, e.g. a repo-list fetch
		// error) are always delivered (issue #46).
		if g, ok := msg.(screen.Generational); ok && a.TargetView() != viewRepoList && g.Generation() != m.generation {
			slog.Debug("dropping stale addressed message from a superseded repo selection",
				"target", a.TargetView(), "msgGen", g.Generation(), "curGen", m.generation)
			return m, nil
		}
		if fe, ok := msg.(screen.FetchErrMsg); ok {
			slog.Warn("view fetch error", "view", fe.View, "err", fe.Err)
		}
		return m.routeTo(a.TargetView(), msg)
	}

	return m.forward(msg)
}

// forward sends a message to the active screen; it is routeTo targeted at the
// active view.
func (m Model) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m.routeTo(m.active, msg)
}

// activeScreen returns the sub-model for the active view: the persistent repo
// list, or the current per-repo screen (nil if the per-repo screens are not built
// yet, i.e. before a repository is selected).
func (m Model) activeScreen() screen.Model {
	if m.active == viewRepoList {
		return m.repoList
	}
	return m.perRepo[m.active]
}

// routeTo delivers a message to a specific view regardless of which one is
// active, storing the returned model. It backs addressed async delivery so a
// background fetch's data/error reaches the view that started it.
func (m Model) routeTo(target view, msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if target == viewRepoList {
		m.repoList, cmd = m.repoList.Update(msg)
		return m, cmd
	}
	if s, ok := m.perRepo[target]; ok {
		m.perRepo[target], cmd = s.Update(msg)
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

	for _, v := range perRepoViews {
		if s, ok := m.perRepo[v]; ok {
			m.perRepo[v], cmd = s.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// View renders the global error overlay, or delegates to the active screen.
func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  Error: %v\n\n  Press ctrl+c to quit.\n", m.err)
	}

	if m.helpVisible {
		return help.Render(m.active, m.width, m.height, m.helpScroll)
	}

	if m.active == viewRepoList {
		return m.repoList.View()
	}
	if s, ok := m.perRepo[m.active]; ok {
		return s.View()
	}
	return ""
}
