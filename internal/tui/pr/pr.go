// Package pr implements the pull-request split-pane screen.
package pr

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	ghClient "github.com/ChristopherBilg/lazygh/internal/github"
	"github.com/ChristopherBilg/lazygh/internal/tui/nav"
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
				m.viewport.ScrollUp(1)
			}
		case "down", "j":
			if m.focus == focusList {
				if m.cursor < len(m.ctx.PRs)-1 {
					m.cursor++
					m.updateViewportContent()
					m.viewport.GotoTop()
				}
			} else {
				m.viewport.ScrollDown(1)
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
	headerHeight := 5 // +1 for the nav.Bar line above the repo header
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
		return fmt.Sprintf("%s\n\n  Fetching PRs for %s...\n", nav.Bar(nav.TabPRs), m.ctx.Name)
	}

	leftPaneWidth := (m.width * 3) / 10
	paneHeight := m.height - 7

	var listStr strings.Builder
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
		fmt.Fprintf(&listStr, "%s%s\n", cursorStr, title)
	}

	var listBorder, detailBorder lipgloss.Style
	if m.focus == focusList {
		listBorder = styles.Active
		detailBorder = styles.Base
	} else {
		listBorder = styles.Base
		detailBorder = styles.Active
	}

	left := listBorder.Width(leftPaneWidth).Height(paneHeight).Render(listStr.String())
	right := detailBorder.Width(m.viewport.Width + 2).Height(paneHeight).Render(m.viewport.View())

	header := fmt.Sprintf("%s\n Lazy GitHub | %s/%s \n\n", nav.Bar(nav.TabPRs), m.ctx.Owner, m.ctx.Name)
	ui := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	footerText := " [1/2/3] Views  •  [esc] Repo  •  [tab] Focus  •  [j/k] Scroll  •  [c] Checkout  •  [o] Web  •  [q] Quit"
	if m.message != "" {
		footerText = fmt.Sprintf(" %s | %s", styles.Title.Render(m.message), footerText)
	}

	return header + ui + "\n\n" + footerText
}
