// Package repolist implements the repository-selection screen.
package repolist

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	ghClient "github.com/ChristopherBilg/lazygh/internal/github"
	"github.com/ChristopherBilg/lazygh/internal/tui/help"
	"github.com/ChristopherBilg/lazygh/internal/tui/keys"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
	"github.com/ChristopherBilg/lazygh/internal/tui/styles"
)

// reservedRows is the vertical chrome around the repository rows: the leading
// blank line, the title and its blank line, the Menu box border and padding,
// the scroll-indicator line and its blank line, and the footer with its gap.
// capacity subtracts it from the terminal height. The chrome itself is ~11
// rows; reserving 12 leaves a one-row safety margin so the rendered block never
// overflows the terminal.
const reservedRows = 12

// Backend is the subset of the github client the repo-list screen needs.
type Backend interface {
	Repositories(ctx context.Context, force bool) ([]ghClient.Repository, error)
}

// Model is the repository-selection screen.
type Model struct {
	backend    Backend
	repos      []ghClient.Repository
	cursor     int
	loading    bool
	refreshing bool
	fetchErr   error
	spinner    spinner.Model
	width      int
	height     int
	top        int // index of the first visible repo (scroll offset)
}

// New returns a repository-selection screen in its initial loading state,
// backed by the given github client.
func New(backend Backend) Model {
	return Model{
		backend: backend,
		loading: true,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
}

// RepoSelectedMsg is emitted upward when the user selects a repository.
type RepoSelectedMsg struct {
	Owner string
	Name  string
}

// reposMsg carries the fetched repositories.
type reposMsg []ghClient.Repository

// TargetView addresses fetched repositories to the repo-list screen.
func (reposMsg) TargetView() screen.ViewID { return screen.ViewRepoList }

// fetchReposCmd fetches the authenticated user's repositories. force bypasses
// the in-memory cache and refreshes the stored entry. A client-init failure is
// fatal (ErrMsg); any other failure is a recoverable FetchErrMsg.
func (m Model) fetchReposCmd(force bool) tea.Cmd {
	return func() tea.Msg {
		// context.Background() for now; a program-scoped context is future work.
		repos, err := m.backend.Repositories(context.Background(), force)
		if err != nil {
			if errors.Is(err, ghClient.ErrClientInit) {
				return screen.ErrMsg{Err: err}
			}
			return screen.FetchErrMsg{View: screen.ViewRepoList, Err: err}
		}
		return reposMsg(repos)
	}
}

// Init starts fetching repositories (from cache when available) and the spinner.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchReposCmd(false), m.spinner.Tick)
}

// Update handles navigation and selection.
func (m Model) Update(msg tea.Msg) (screen.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.top = clampTop(m.top, m.cursor, len(m.repos), m.capacity())

	case spinner.TickMsg:
		// Ticks are not addressed, so they only reach the active screen. If the
		// user switches tabs mid-refresh the animation can freeze until the fetch
		// resolves — an accepted cosmetic limitation (see the plan's known-limitation
		// note). Correctness is unaffected: the non-fatal result still lands via the
		// addressed reposMsg/FetchErrMsg, which clears refreshing regardless.
		if !m.loading && !m.refreshing {
			return m, nil // stop the tick loop once fetching ends
		}
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case reposMsg:
		m.repos = msg
		m.loading = false
		m.refreshing = false
		m.fetchErr = nil
		if m.cursor >= len(m.repos) {
			m.cursor = max(len(m.repos)-1, 0)
		}
		m.top = clampTop(m.top, m.cursor, len(m.repos), m.capacity())

	case screen.FetchErrMsg:
		m.loading = false
		m.refreshing = false
		m.fetchErr = msg.Err

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Map.Up):
			if m.cursor > 0 {
				m.cursor--
				m.top = clampTop(m.top, m.cursor, len(m.repos), m.capacity())
			}
		case key.Matches(msg, keys.Map.Down):
			if m.cursor < len(m.repos)-1 {
				m.cursor++
				m.top = clampTop(m.top, m.cursor, len(m.repos), m.capacity())
			}
		case key.Matches(msg, keys.Map.Select):
			if len(m.repos) > 0 {
				selected := m.repos[m.cursor]
				return m, func() tea.Msg {
					return RepoSelectedMsg{Owner: selected.Owner.Login, Name: selected.Name}
				}
			}
		case key.Matches(msg, keys.Map.Refresh):
			// Manual refresh: bypass the cache, keep the current list visible.
			wasFetching := m.loading || m.refreshing
			m.fetchErr = nil
			if len(m.repos) == 0 {
				m.loading = true
			} else {
				m.refreshing = true
			}
			cmds = append(cmds, m.fetchReposCmd(true))
			if !wasFetching {
				cmds = append(cmds, m.spinner.Tick)
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// View renders the repository list.
func (m Model) View() string {
	if m.loading {
		return fmt.Sprintf("\n  %sFetching your repositories...\n", m.spinner.View())
	}

	if m.fetchErr != nil && len(m.repos) == 0 {
		msg := styles.Error.Render(fmt.Sprintf("Failed to load repositories: %v (%s)", m.fetchErr, help.RetryHint()))
		return fmt.Sprintf("\n  %s\n", styles.Truncate(msg, m.width))
	}

	var s strings.Builder
	s.WriteString(" Select a Repository:\n\n")

	capacity := m.capacity()
	end := min(m.top+capacity, len(m.repos))

	for i := m.top; i < end; i++ {
		cursor := "  "
		repoName := m.repos[i].FullName
		if m.cursor == i {
			cursor = "> "
			repoName = styles.SelectedItem.Render(repoName)
		}
		fmt.Fprintf(&s, "%s%s\n", cursor, repoName)
	}

	if len(m.repos) > capacity {
		fmt.Fprintf(&s, "\n  %s\n", scrollIndicator(m.top, end, len(m.repos)))
	}

	box := styles.Menu.Width(m.width / 2).Render(s.String())
	centeredBox := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, box)

	footer := m.footer()
	return fmt.Sprintf("\n%s\n\n%s", centeredBox, lipgloss.PlaceHorizontal(m.width, lipgloss.Center, footer))
}

// footer renders the hint bar, or a refresh spinner / non-fatal refresh error
// when one is active. The full keybinding list lives in the ? help overlay.
func (m Model) footer() string {
	hints := help.Footer(keys.Map.Select, keys.Map.Refresh, keys.Map.Help, keys.Map.Quit)
	switch {
	case m.refreshing:
		return fmt.Sprintf(" %sRefreshing...  %s", m.spinner.View(), hints)
	case m.fetchErr != nil:
		return styles.Truncate(styles.Error.Render(fmt.Sprintf(" Refresh failed: %v (%s)", m.fetchErr, help.RetryHint())), m.width)
	default:
		return hints
	}
}

// capacity returns how many repository rows fit in the current window, always at
// least 1 so the highlighted row and the scroll indicator stay usable even on
// very short terminals.
func (m Model) capacity() int {
	return max(m.height-reservedRows, 1)
}

// clampTop returns the scroll offset (index of the first visible row) that keeps
// cursor within the visible window [top, top+capacity) while scrolling the
// minimum distance from the current top. It never scrolls past the end of the
// list, so the window stays full when the list is longer than capacity.
func clampTop(top, cursor, total, capacity int) int {
	if capacity < 1 {
		capacity = 1
	}
	switch {
	case cursor < top:
		top = cursor
	case cursor >= top+capacity:
		top = cursor - capacity + 1
	}
	if maxTop := max(total-capacity, 0); top > maxTop {
		top = maxTop
	}
	return max(top, 0)
}

// scrollIndicator renders a compact "x–y of N" counter for the visible window
// [top, end) over total rows, with up/down arrows shown only when there is
// content scrolled off above or below.
func scrollIndicator(top, end, total int) string {
	up, down := " ", " "
	if top > 0 {
		up = "↑"
	}
	if end < total {
		down = "↓"
	}
	return fmt.Sprintf("%s %d–%d of %d %s", up, top+1, end, total, down)
}
