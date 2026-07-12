// Package tui contains the root router model that hosts and switches between
// the application's screens.
package tui

import (
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ChristopherBilg/lazygh/internal/tui/action"
	"github.com/ChristopherBilg/lazygh/internal/tui/issue"
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

// Model is the root router. It owns only routing state: the active view, the
// repository-selection screen, the per-repo screens (kept alive so switching
// between them preserves each one's selection and scroll position), the sticky
// global error, and the window dimensions it propagates.
type Model struct {
	active   view
	repoList screen.Model // persistent: survives back-navigation so the cursor is retained
	// perRepo holds the per-repo screens (pr/issue/action), built on selection
	// and kept alive so switching preserves each screen's state. It is mutated in
	// place via map-reference semantics; do not clone Model and expect
	// independent per-repo state.
	perRepo map[view]screen.Model
	err     error // sticky: once set, the overlay shows until quit
	width   int
	height  int
}

var _ tea.Model = Model{}

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
			// already at the repo list: fall through to forward (repolist ignores esc/backspace)
		case "1", "2", "3":
			// Global view switch. Only meaningful once a repo is selected; on the
			// repo list these keys are ignored (forwarded, repolist drops them).
			if m.active != viewRepoList {
				switch msg.String() {
				case "1":
					m.active = viewPR
				case "2":
					m.active = viewIssues
				case "3":
					m.active = viewActions
				}
				return m, nil
			}
		}
		return m.forward(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m.broadcastResize(msg)

	case repolist.RepoSelectedMsg:
		m.perRepo = map[view]screen.Model{
			viewPR:      pr.New(msg.Owner, msg.Name, m.width, m.height),
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
		return fmt.Sprintf("\n  Error: %v\n\n  Press 'q' to quit.\n", m.err)
	}

	if m.active == viewRepoList {
		return m.repoList.View()
	}
	if s, ok := m.perRepo[m.active]; ok {
		return s.View()
	}
	return ""
}
