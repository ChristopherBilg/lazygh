// Package action implements a placeholder GitHub Actions screen. Full
// functionality is tracked under Epic 3; this is a navigable shell only.
package action

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ChristopherBilg/lazygh/internal/tui/nav"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
	"github.com/ChristopherBilg/lazygh/internal/tui/styles"
)

// Model is the placeholder Actions screen.
type Model struct {
	width  int
	height int
}

var _ screen.Model = Model{}

// New returns a placeholder Actions screen sized to the current window.
func New(width, height int) Model {
	return Model{width: width, height: height}
}

// Init does nothing yet (no data fetching until Epic 3).
func (m Model) Init() tea.Cmd { return nil }

// Update only tracks window size; there is no interactive state yet.
func (m Model) Update(msg tea.Msg) (screen.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		m.height = ws.Height
	}
	return m, nil
}

// View renders the tab bar plus a centered placeholder panel.
func (m Model) View() string {
	panel := styles.Menu.Width(m.width / 2).Render(
		fmt.Sprintf("%s\n\nComing soon — tracked under Epic 3.", styles.Title.Render("Actions")),
	)
	footer := " [1/2/3] Views  •  [esc] Repo  •  [q] Quit"
	return fmt.Sprintf(
		"%s\n\n%s\n\n%s",
		nav.Bar(nav.TabActions),
		lipgloss.PlaceHorizontal(m.width, lipgloss.Center, panel),
		lipgloss.PlaceHorizontal(m.width, lipgloss.Center, footer),
	)
}
