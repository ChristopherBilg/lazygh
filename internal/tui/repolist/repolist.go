// Package repolist implements the repository-selection screen.
package repolist

import (
	"fmt"
	"strings"

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

	var s strings.Builder
	s.WriteString(" Select a Repository:\n\n")

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
		fmt.Fprintf(&s, "%s%s\n", cursor, repoName)
	}

	if len(m.repos) > maxVisible {
		fmt.Fprintf(&s, "\n  ...and %d more.\n", len(m.repos)-maxVisible)
	}

	box := styles.Menu.Width(m.width / 2).Render(s.String())
	centeredBox := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, box)

	footer := " [j/k] Navigate  •  [enter] Select  •  [q] Quit"
	return fmt.Sprintf("\n%s\n\n%s", centeredBox, lipgloss.PlaceHorizontal(m.width, lipgloss.Center, footer))
}
