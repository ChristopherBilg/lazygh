// Package pr implements the pull-request split-pane screen.
package pr

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	ghClient "github.com/ChristopherBilg/lazygh/internal/github"
	"github.com/ChristopherBilg/lazygh/internal/tui/keys"
	"github.com/ChristopherBilg/lazygh/internal/tui/nav"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
	"github.com/ChristopherBilg/lazygh/internal/tui/styles"
)

// focus identifies which pane of the split view has keyboard focus.
type focus int

// Focus targets within the split pane.
const (
	focusList focus = iota
	focusDetails
)

// Backend is the subset of the github client the PR screen needs.
type Backend interface {
	RepoPRs(ctx context.Context, owner, name string, force bool) (ghClient.RepoContext, error)
	CheckoutPR(ctx context.Context, owner, name string, prNumber int) error
	OpenPRInBrowser(ctx context.Context, owner, name string, prNumber int) error
}

// Model is the pull-request split-pane screen.
type Model struct {
	backend    Backend
	ctx        ghClient.RepoContext
	cursor     int
	focus      focus
	loading    bool
	refreshing bool
	fetchErr   error
	message    string
	spinner    spinner.Model
	viewport   viewport.Model
	width      int
	height     int
	ready      bool
}

// New returns a PR screen for the given repository, sized to the current window,
// backed by the given github client. The repository owner/name are stored
// immediately so the loading view can show the repository name.
func New(backend Backend, owner, name string, width, height int) Model {
	m := Model{
		backend: backend,
		ctx:     ghClient.RepoContext{Owner: owner, Name: name},
		focus:   focusList,
		loading: true,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
		width:   width,
		height:  height,
	}
	m.resizeViewport()
	return m
}

// prDataMsg carries fetched pull-request data.
type prDataMsg ghClient.RepoContext

// TargetView addresses fetched PR data to the pull-request screen.
func (prDataMsg) TargetView() screen.ViewID { return screen.ViewPR }

// statusMsg carries a transient footer status message.
type statusMsg string

// fetchPRsCmd fetches the open PRs for the given repository. force bypasses the
// in-memory cache. A client-init failure is fatal (ErrMsg); any other failure is
// a recoverable FetchErrMsg.
func (m Model) fetchPRsCmd(owner, name string, force bool) tea.Cmd {
	return func() tea.Msg {
		// context.Background() for now; a program-scoped context is future work.
		data, err := m.backend.RepoPRs(context.Background(), owner, name, force)
		if err != nil {
			if errors.Is(err, ghClient.ErrClientInit) {
				return screen.ErrMsg{Err: err}
			}
			return screen.FetchErrMsg{View: screen.ViewPR, Err: err}
		}
		return prDataMsg(data)
	}
}

func (m Model) checkoutCmd(owner, name string, prNumber int) tea.Cmd {
	return func() tea.Msg {
		// context.Background() for now; a program-scoped context is future work.
		if err := m.backend.CheckoutPR(context.Background(), owner, name, prNumber); err != nil {
			if errors.Is(err, ghClient.ErrNotLocalRepo) {
				return statusMsg(fmt.Sprintf("Checkout unavailable: lazygh isn't running in a clone of %s/%s", owner, name))
			}
			return statusMsg(fmt.Sprintf("Checkout failed: %v", err))
		}
		return statusMsg(fmt.Sprintf("Successfully checked out PR #%d", prNumber))
	}
}

func (m Model) openBrowserCmd(owner, name string, prNumber int) tea.Cmd {
	return func() tea.Msg {
		// context.Background() for now; a program-scoped context is future work.
		if err := m.backend.OpenPRInBrowser(context.Background(), owner, name, prNumber); err != nil {
			return statusMsg(fmt.Sprintf("Open in browser failed: %v", err))
		}
		return statusMsg(fmt.Sprintf("Opened PR #%d in browser", prNumber))
	}
}

// Init starts fetching pull requests (from cache when available) and the spinner.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchPRsCmd(m.ctx.Owner, m.ctx.Name, false), m.spinner.Tick)
}

// Update handles focus, navigation, PR actions, and data messages.
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

	case spinner.TickMsg:
		// Ticks are not addressed, so they only reach the active screen; a
		// mid-refresh tab switch can freeze the animation until the fetch
		// resolves — an accepted cosmetic limitation (see the plan). Correctness
		// is unaffected: the addressed prDataMsg/FetchErrMsg still clears state.
		if !m.loading && !m.refreshing {
			return m, nil // stop the tick loop once fetching ends
		}
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Map.TogglePane):
			if m.focus == focusList {
				m.focus = focusDetails
			} else {
				m.focus = focusList
			}
		case key.Matches(msg, keys.Map.Up):
			if m.focus == focusList {
				if m.cursor > 0 {
					m.cursor--
					m.updateViewportContent()
					m.viewport.GotoTop()
				}
			} else {
				m.viewport.ScrollUp(1)
			}
		case key.Matches(msg, keys.Map.Down):
			if m.focus == focusList {
				if m.cursor < len(m.ctx.PRs)-1 {
					m.cursor++
					m.updateViewportContent()
					m.viewport.GotoTop()
				}
			} else {
				m.viewport.ScrollDown(1)
			}
		case key.Matches(msg, keys.Map.Checkout):
			if len(m.ctx.PRs) > 0 {
				m.message = "Checking out branch..."
				cmds = append(cmds, m.checkoutCmd(m.ctx.Owner, m.ctx.Name, m.ctx.PRs[m.cursor].Number))
			}
		case key.Matches(msg, keys.Map.Open):
			if len(m.ctx.PRs) > 0 {
				m.message = "Opening browser..."
				cmds = append(cmds, m.openBrowserCmd(m.ctx.Owner, m.ctx.Name, m.ctx.PRs[m.cursor].Number))
			}
		case key.Matches(msg, keys.Map.Refresh):
			// Manual refresh: bypass the cache, keep the current PRs visible.
			wasFetching := m.loading || m.refreshing
			m.fetchErr = nil
			m.message = ""
			if len(m.ctx.PRs) == 0 {
				m.loading = true
			} else {
				m.refreshing = true
			}
			cmds = append(cmds, m.fetchPRsCmd(m.ctx.Owner, m.ctx.Name, true))
			if !wasFetching {
				cmds = append(cmds, m.spinner.Tick)
			}
		}

	case prDataMsg:
		m.ctx = ghClient.RepoContext(msg)
		m.loading = false
		m.refreshing = false
		m.fetchErr = nil
		if m.cursor >= len(m.ctx.PRs) {
			m.cursor = max(len(m.ctx.PRs)-1, 0)
		}
		m.updateViewportContent()
		m.viewport.GotoTop()

	case screen.FetchErrMsg:
		m.loading = false
		m.refreshing = false
		m.fetchErr = msg.Err

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
	contentHeight := max(m.height-headerHeight-footerHeight, 1)
	rightPaneWidth := (m.width * 7) / 10
	vpWidth := max(rightPaneWidth-2, 1)
	vpHeight := max(contentHeight-2, 1)

	if !m.ready {
		m.viewport = viewport.New(vpWidth, vpHeight)
		m.ready = true
	} else {
		m.viewport.Width = vpWidth
		m.viewport.Height = vpHeight
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

// View renders the split pane, a loading spinner, or a non-fatal load error.
func (m Model) View() string {
	if m.loading {
		return fmt.Sprintf("%s\n\n  %sFetching PRs for %s...\n", nav.Bar(nav.TabPRs), m.spinner.View(), m.ctx.Name)
	}

	if m.fetchErr != nil && len(m.ctx.PRs) == 0 {
		msg := styles.Error.Render(fmt.Sprintf("Failed to load PRs: %v (press r to retry)", m.fetchErr))
		return fmt.Sprintf("%s\n\n  %s\n", nav.Bar(nav.TabPRs), styles.Truncate(msg, m.width))
	}

	leftPaneWidth := (m.width * 3) / 10
	paneHeight := max(m.height-7, 1)

	var listStr strings.Builder
	for i, pr := range m.ctx.PRs {
		cursorStr := "  "
		// Reserve 2 columns for the cursor prefix so the title never overflows the
		// pane. TruncateEllipsis is width-aware and never panics on narrow widths.
		title := styles.TruncateEllipsis(fmt.Sprintf("#%d %s", pr.Number, pr.Title), leftPaneWidth-2)

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

	footerText := " [1/2/3] Views  •  [esc] Repo  •  [tab] Focus  •  [j/k] Scroll  •  [c] Checkout  •  [o] Web  •  [r] Refresh  •  [q] Quit"
	if status := m.statusLine(); status != "" {
		footerText = fmt.Sprintf(" %s | %s", status, footerText)
	}

	return header + ui + "\n\n" + footerText
}

// statusLine renders the highest-priority transient status: a refresh spinner, a
// non-fatal refresh error, or a plain status message (checkout/browser result).
func (m Model) statusLine() string {
	switch {
	case m.refreshing:
		return fmt.Sprintf("%sRefreshing...", m.spinner.View())
	case m.fetchErr != nil:
		return styles.Truncate(styles.Error.Render(fmt.Sprintf("Refresh failed: %v (press r to retry)", m.fetchErr)), m.width)
	case m.message != "":
		return styles.Truncate(styles.Title.Render(m.message), m.width)
	default:
		return ""
	}
}
