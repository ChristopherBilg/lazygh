// Package tui contains the root router model that hosts and switches between
// the application's screens.
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ChristopherBilg/lazygh/internal/tui/action"
	"github.com/ChristopherBilg/lazygh/internal/tui/issue"
	"github.com/ChristopherBilg/lazygh/internal/tui/pr"
	"github.com/ChristopherBilg/lazygh/internal/tui/repolist"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
)

// view identifies which screen is currently active.
type view int

const (
	viewRepoList view = iota
	viewPR
	viewIssues
	viewActions
)

// perRepoViews lists the per-repo views in tab order (1/2/3). It drives
// construction, Init, and resize so the behavior is deterministic.
var perRepoViews = []view{viewPR, viewIssues, viewActions}

// Model is the root router. It owns only routing state: the active view, the
// repository-selection screen, the per-repo screens (kept alive so switching
// between them preserves each one's selection and scroll position), the sticky
// global error, and the window dimensions it propagates.
type Model struct {
	active   view
	repoList screen.Model          // persistent: survives back-navigation so the cursor is retained
	perRepo  map[view]screen.Model // per-repo screens (pr/issue/action), built on selection, kept alive
	err      error                 // sticky: once set, the overlay shows until quit
	width    int
	height   int
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
			cmds = append(cmds, m.perRepo[v].Init())
		}
		return m, tea.Batch(cmds...)

	case screen.ErrMsg:
		m.err = msg.Err
		return m, nil
	}

	return m.forward(msg)
}

// forward sends a message to the active screen and stores the returned model.
func (m Model) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.active == viewRepoList {
		m.repoList, cmd = m.repoList.Update(msg)
		return m, cmd
	}
	if s, ok := m.perRepo[m.active]; ok {
		m.perRepo[m.active], cmd = s.Update(msg)
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
