package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	ghClient "github.com/ChristopherBilg/lazygh/internal/github"
)

// --- Application States ---
const (
	stateRepoSelection = iota
	statePRView
)

// --- Focus Constants ---
const (
	focusList = iota
	focusDetails
)

// --- Styles ---
var (
	baseStyle         = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240"))
	activeStyle       = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62"))
	selectedItemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	titleStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1).Bold(true)
	menuStyle         = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(1, 2)
)

// --- Model ---
type Model struct {
	state       int // Tracks the current screen
	repos       []ghClient.Repository
	repoCursor  int
	repoContext ghClient.RepoContext
	prCursor    int
	focus       int

	ready   bool
	loading bool
	message string
	err     error

	viewport viewport.Model
	width    int
	height   int
}

func NewModel() Model {
	return Model{
		state:   stateRepoSelection,
		loading: true,
		focus:   focusList,
	}
}

// --- Commands ---
type reposMsg []ghClient.Repository
type prDataMsg ghClient.RepoContext
type errMsg struct{ err error }
type statusMsg string

// Fetch repositories on startup
func fetchReposCmd() tea.Cmd {
	return func() tea.Msg {
		repos, err := ghClient.FetchUserRepositories()
		if err != nil {
			return errMsg{err}
		}
		return reposMsg(repos)
	}
}

// Fetch PRs for a specific selected repository
func fetchPRsCmd(owner, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, err := ghClient.FetchRepoPRs(owner, name)
		if err != nil {
			return errMsg{err}
		}
		return prDataMsg(ctx)
	}
}

func checkoutCmd(prNumber int) tea.Cmd {
	return func() tea.Msg {
		if err := ghClient.CheckoutPR(prNumber); err != nil {
			return errMsg{err}
		}
		return statusMsg(fmt.Sprintf("Successfully checked out PR #%d", prNumber))
	}
}

func openBrowserCmd(prNumber int) tea.Cmd {
	return func() tea.Msg {
		if err := ghClient.OpenPRInBrowser(prNumber); err != nil {
			return errMsg{err}
		}
		return statusMsg(fmt.Sprintf("Opened PR #%d in browser", prNumber))
	}
}

// --- Update ---
func (m Model) Init() tea.Cmd {
	// Start by fetching repositories
	return fetchReposCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "ctrl+c", "q":
			return m, tea.Quit

		// Navigate Back
		case "esc", "backspace":
			if m.state == statePRView {
				m.state = stateRepoSelection
				m.message = ""
				return m, nil
			}
		}

		// Route keystrokes based on the active state
		if m.state == stateRepoSelection {
			return m.updateRepoSelection(msg)
		} else if m.state == statePRView {
			m, cmd = m.updatePRView(msg)
			cmds = append(cmds, cmd)
		}

	case reposMsg:
		m.repos = msg
		m.loading = false

	case prDataMsg:
		m.repoContext = ghClient.RepoContext(msg)
		m.loading = false
		m.updateViewportContent()

	case statusMsg:
		m.message = string(msg)

	case errMsg:
		m.err = msg.err
		m.loading = false
	}

	// Always update viewport for background events like mouse scrolls
	if m.state == statePRView {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

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
	if m.state == statePRView {
		m.updateViewportContent()
	}
}

// Sub-updater for the Repository Selection state
func (m Model) updateRepoSelection(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.repoCursor > 0 {
			m.repoCursor--
		}
	case "down", "j":
		if m.repoCursor < len(m.repos)-1 {
			m.repoCursor++
		}
	case "enter":
		if len(m.repos) > 0 {
			selected := m.repos[m.repoCursor]
			m.state = statePRView
			m.loading = true
			m.focus = focusList
			m.prCursor = 0 // Reset PR cursor for the new repo
			return m, fetchPRsCmd(selected.Owner.Login, selected.Name)
		}
	}
	return m, nil
}

// Sub-updater for the PR Split-Pane state
func (m Model) updatePRView(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "shift+tab":
		if m.focus == focusList {
			m.focus = focusDetails
		} else {
			m.focus = focusList
		}
	case "up", "k":
		if m.focus == focusList {
			if m.prCursor > 0 {
				m.prCursor--
				m.updateViewportContent()
				m.viewport.GotoTop()
			}
		} else {
			m.viewport.LineUp(1)
		}
	case "down", "j":
		if m.focus == focusList {
			if m.prCursor < len(m.repoContext.PRs)-1 {
				m.prCursor++
				m.updateViewportContent()
				m.viewport.GotoTop()
			}
		} else {
			m.viewport.LineDown(1)
		}
	case "c":
		if len(m.repoContext.PRs) > 0 {
			m.message = "Checking out branch..."
			return m, checkoutCmd(m.repoContext.PRs[m.prCursor].Number)
		}
	case "o":
		if len(m.repoContext.PRs) > 0 {
			m.message = "Opening browser..."
			return m, openBrowserCmd(m.repoContext.PRs[m.prCursor].Number)
		}
	}
	return m, nil
}

func (m *Model) updateViewportContent() {
	if len(m.repoContext.PRs) == 0 {
		m.viewport.SetContent("No open PRs.")
		return
	}
	activePR := m.repoContext.PRs[m.prCursor]

	contentStyle := lipgloss.NewStyle().Width(m.viewport.Width)

	body := activePR.Body
	if body == "" {
		body = "*No description provided.*"
	}

	fullText := fmt.Sprintf("%s\nState: %s\n\n%s",
		titleStyle.Render(activePR.Title),
		activePR.State,
		body)

	m.viewport.SetContent(contentStyle.Render(fullText))
}

// --- View ---
func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  Error: %v\n\n  Press 'q' to quit.\n", m.err)
	}

	if m.state == stateRepoSelection {
		return m.viewRepoSelection()
	}

	return m.viewPRView()
}

// Render the Repository List
func (m Model) viewRepoSelection() string {
	if m.loading {
		return "\n  Fetching your repositories...\n"
	}

	s := " Select a Repository:\n\n"

	// Only show a window of repos to avoid scrolling off screen in this basic view
	start := 0
	end := len(m.repos)
	maxVisible := m.height - 10
	if end > maxVisible {
		end = maxVisible
	}

	for i := start; i < end; i++ {
		cursor := "  "
		repoName := m.repos[i].FullName
		if m.repoCursor == i {
			cursor = "> "
			repoName = selectedItemStyle.Render(repoName)
		}
		s += fmt.Sprintf("%s%s\n", cursor, repoName)
	}

	if len(m.repos) > maxVisible {
		s += fmt.Sprintf("\n  ...and %d more.\n", len(m.repos)-maxVisible)
	}

	box := menuStyle.Width(m.width / 2).Render(s)

	// Center the box horizontally
	centeredBox := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, box)

	footer := " [j/k] Navigate  •  [enter] Select  •  [q] Quit"
	return fmt.Sprintf("\n%s\n\n%s", centeredBox, lipgloss.PlaceHorizontal(m.width, lipgloss.Center, footer))
}

// Render the PR Split-Pane
func (m Model) viewPRView() string {
	if m.loading {
		return fmt.Sprintf("\n  Fetching PRs for %s...\n", m.repoContext.Name)
	}

	leftPaneWidth := (m.width * 3) / 10
	paneHeight := m.height - 6

	listStr := ""
	for i, pr := range m.repoContext.PRs {
		cursorStr := "  "
		title := fmt.Sprintf("#%d %s", pr.Number, pr.Title)

		if len(title) > leftPaneWidth-6 {
			title = title[:leftPaneWidth-9] + "..."
		}

		if m.prCursor == i {
			cursorStr = "> "
			title = selectedItemStyle.Render(title)
		}
		listStr += fmt.Sprintf("%s%s\n", cursorStr, title)
	}

	var listBorder, detailBorder lipgloss.Style
	if m.focus == focusList {
		listBorder = activeStyle
		detailBorder = baseStyle
	} else {
		listBorder = baseStyle
		detailBorder = activeStyle
	}

	left := listBorder.Width(leftPaneWidth).Height(paneHeight).Render(listStr)
	right := detailBorder.Width(m.viewport.Width + 2).Height(paneHeight).Render(m.viewport.View())

	header := fmt.Sprintf(" Lazy GitHub | %s/%s \n\n", m.repoContext.Owner, m.repoContext.Name)
	ui := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	footerText := " [esc] Change Repo  •  [tab] Focus  •  [j/k] Scroll  •  [c] Checkout  •  [o] Web  •  [q] Quit"
	if m.message != "" {
		footerText = fmt.Sprintf(" %s | %s", titleStyle.Render(m.message), footerText)
	}

	return header + ui + "\n\n" + footerText
}
