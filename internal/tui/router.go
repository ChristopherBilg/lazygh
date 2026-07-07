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
	repoList screen.Model // persistent: survives back-navigation so the cursor is retained
	current  screen.Model // active per-repo screen (pr now; pr/issue/action in #2), rebuilt on each selection
	err      error        // sticky: once set, the overlay shows until quit
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
