// Package pr implements the pull-request split-pane screen.
package pr

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ChristopherBilg/lazygh/internal/fuzzy"
	ghClient "github.com/ChristopherBilg/lazygh/internal/github"
	"github.com/ChristopherBilg/lazygh/internal/tui/keys"
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
	ctx        ghClient.RepoContext
	cursor     int
	focus      int
	loading    bool
	refreshing bool
	fetchErr   error
	message    string
	spinner    spinner.Model
	viewport   viewport.Model
	input      textinput.Model // the "/" filter field
	searching  bool            // input focused / capturing keystrokes
	query      string          // applied filter; "" = no filter
	filtered   []int           // indices into ctx.PRs, in display (ranked) order
	width      int
	height     int
	ready      bool
}

// New returns a PR screen for the given repository, sized to the current window.
// The repository owner/name are stored immediately so the loading view can show
// the repository name.
func New(owner, name string, width, height int) Model {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "filter titles"
	ti.CharLimit = 128

	m := Model{
		ctx:     ghClient.RepoContext{Owner: owner, Name: name},
		focus:   focusList,
		loading: true,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
		input:   ti,
		width:   width,
		height:  height,
	}
	m.resizeViewport()
	return m
}

// recompute rebuilds the filtered (visible) index list from the current query and
// PR set, clamps the cursor into range, and refreshes the detail viewport. It is
// the single place the visible set is derived, so filtering, refresh, and cancel
// all stay consistent.
func (m *Model) recompute() {
	if m.query == "" {
		m.filtered = make([]int, len(m.ctx.PRs))
		for i := range m.filtered {
			m.filtered[i] = i
		}
	} else {
		titles := make([]string, len(m.ctx.PRs))
		for i, pr := range m.ctx.PRs {
			titles[i] = pr.Title
		}
		m.filtered = fuzzy.Rank(m.query, titles)
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(len(m.filtered)-1, 0)
	}
	m.updateViewportContent()
	m.viewport.GotoTop()
}

// selectedPR returns the PR under the cursor within the filtered set, or false
// when the filtered set is empty.
func (m Model) selectedPR() (ghClient.PullRequest, bool) {
	if len(m.filtered) == 0 {
		return ghClient.PullRequest{}, false
	}
	return m.ctx.PRs[m.filtered[m.cursor]], true
}

// CapturingInput reports whether the filter field is focused and consuming keys.
// The router consults it (via screen.InputCapturer) to suppress global keys while
// the user is typing a filter.
func (m Model) CapturingInput() bool { return m.searching }

// updateSearch handles keys while the filter field is focused: arrow keys move the
// selection live, Enter commits (keeps the filter, blurs the field), Esc cancels
// (clears the filter, restores the full list), and every other key is typed into
// the field, re-filtering on each change.
func (m Model) updateSearch(msg tea.KeyMsg) (screen.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		m.searching = false
		m.input.Blur()
		return m, nil
	case tea.KeyEsc:
		m.searching = false
		m.input.Blur()
		m.input.Reset()
		m.query = ""
		m.cursor = 0
		m.recompute()
		return m, nil
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
			m.updateViewportContent()
			m.viewport.GotoTop()
		}
		return m, nil
	case tea.KeyDown:
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			m.updateViewportContent()
			m.viewport.GotoTop()
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != m.query {
		m.query = m.input.Value()
		m.cursor = 0 // best match to the top on each query change
		m.recompute()
	}
	return m, cmd
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
func fetchPRsCmd(owner, name string, force bool) tea.Cmd {
	return func() tea.Msg {
		ctx, err := ghClient.RepoPRs(owner, name, force)
		if err != nil {
			if errors.Is(err, ghClient.ErrClientInit) {
				return screen.ErrMsg{Err: err}
			}
			return screen.FetchErrMsg{View: screen.ViewPR, Err: err}
		}
		return prDataMsg(ctx)
	}
}

// checkoutPR is indirected so checkoutCmd's result handling can be tested
// without spawning `gh` or requiring a local git repository.
var checkoutPR = ghClient.CheckoutPR

func checkoutCmd(owner, name string, prNumber int) tea.Cmd {
	return func() tea.Msg {
		if err := checkoutPR(owner, name, prNumber); err != nil {
			if errors.Is(err, ghClient.ErrNotLocalRepo) {
				return statusMsg(fmt.Sprintf("Checkout unavailable: lazygh isn't running in a clone of %s/%s", owner, name))
			}
			return statusMsg(fmt.Sprintf("Checkout failed: %v", err))
		}
		return statusMsg(fmt.Sprintf("Successfully checked out PR #%d", prNumber))
	}
}

func openBrowserCmd(owner, name string, prNumber int) tea.Cmd {
	return func() tea.Msg {
		if err := ghClient.OpenPRInBrowser(owner, name, prNumber); err != nil {
			return statusMsg(fmt.Sprintf("Open in browser failed: %v", err))
		}
		return statusMsg(fmt.Sprintf("Opened PR #%d in browser", prNumber))
	}
}

// Init starts fetching pull requests (from cache when available) and the spinner.
func (m Model) Init() tea.Cmd {
	return tea.Batch(fetchPRsCmd(m.ctx.Owner, m.ctx.Name, false), m.spinner.Tick)
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
		if m.searching {
			return m.updateSearch(msg)
		}
		switch {
		case key.Matches(msg, keys.Map.Search):
			if m.focus == focusList { // only from list focus
				m.searching = true
				m.input.SetValue(m.query) // pre-fill so "/" re-opens to refine
				m.input.CursorEnd()
				cmds = append(cmds, m.input.Focus())
			}
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
				if m.cursor < len(m.filtered)-1 {
					m.cursor++
					m.updateViewportContent()
					m.viewport.GotoTop()
				}
			} else {
				m.viewport.ScrollDown(1)
			}
		case key.Matches(msg, keys.Map.Checkout):
			if pr, ok := m.selectedPR(); ok {
				m.message = "Checking out branch..."
				cmds = append(cmds, checkoutCmd(m.ctx.Owner, m.ctx.Name, pr.Number))
			}
		case key.Matches(msg, keys.Map.Open):
			if pr, ok := m.selectedPR(); ok {
				m.message = "Opening browser..."
				cmds = append(cmds, openBrowserCmd(m.ctx.Owner, m.ctx.Name, pr.Number))
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
			cmds = append(cmds, fetchPRsCmd(m.ctx.Owner, m.ctx.Name, true))
			if !wasFetching {
				cmds = append(cmds, m.spinner.Tick)
			}
		}

	case prDataMsg:
		m.ctx = ghClient.RepoContext(msg)
		m.loading = false
		m.refreshing = false
		m.fetchErr = nil
		m.recompute() // re-apply the active query to the new data and clamp the cursor

	case screen.FetchErrMsg:
		m.loading = false
		m.refreshing = false
		m.fetchErr = msg.Err

	case statusMsg:
		m.message = string(msg)

	default:
		// Forward any other message (e.g. the textinput's cursor-blink tick) to
		// the filter input while it is focused, so the blink loop keeps running.
		if m.searching {
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)
		}
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
	leftPaneWidth := (m.width * 3) / 10

	if !m.ready {
		m.viewport = viewport.New(rightPaneWidth-2, contentHeight-2)
		m.ready = true
	} else {
		m.viewport.Width = rightPaneWidth - 2
		m.viewport.Height = contentHeight - 2
	}
	// The filter input sits inside the left pane; size it to the inner width
	// (minus borders and the "/ " prompt) so a long query scrolls within the box.
	if w := leftPaneWidth - 5; w > 0 {
		m.input.Width = w
	}
	m.updateViewportContent()
}

func (m *Model) updateViewportContent() {
	activePR, ok := m.selectedPR()
	if !ok {
		if m.query != "" {
			m.viewport.SetContent("") // the list pane shows the "no match" message (Task 5)
		} else {
			m.viewport.SetContent("No open PRs.")
		}
		return
	}

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
	paneHeight := m.height - 7

	var listBorder, detailBorder lipgloss.Style
	if m.focus == focusList {
		listBorder = styles.Active
		detailBorder = styles.Base
	} else {
		listBorder = styles.Base
		detailBorder = styles.Active
	}

	left := listBorder.Width(leftPaneWidth).Height(paneHeight).Render(m.renderList(leftPaneWidth))
	right := detailBorder.Width(m.viewport.Width + 2).Height(paneHeight).Render(m.viewport.View())

	header := fmt.Sprintf("%s\n Lazy GitHub | %s/%s \n\n", nav.Bar(nav.TabPRs), m.ctx.Owner, m.ctx.Name)
	ui := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	return header + ui + "\n\n" + m.footer()
}

// renderList renders the left-pane contents: an optional search input or filter
// badge, then the filtered PR rows (or a no-results message).
func (m Model) renderList(paneWidth int) string {
	var b strings.Builder

	switch {
	case m.searching:
		b.WriteString(m.input.View())
		b.WriteString("\n")
	case m.query != "":
		badge := fmt.Sprintf("filter: %q (%d/%d)", m.query, len(m.filtered), len(m.ctx.PRs))
		b.WriteString(styles.Title.Render(styles.Truncate(badge, paneWidth-4)))
		b.WriteString("\n")
	}

	if len(m.filtered) == 0 {
		if m.query != "" {
			fmt.Fprintf(&b, "No PRs match %q.", m.query)
		} else {
			b.WriteString("No open PRs.")
		}
		return b.String()
	}

	for row, idx := range m.filtered {
		pr := m.ctx.PRs[idx]
		cursorStr := "  "
		title := fmt.Sprintf("#%d %s", pr.Number, pr.Title)
		if len(title) > paneWidth-6 {
			title = title[:paneWidth-9] + "..."
		}
		if m.cursor == row {
			cursorStr = "> "
			title = styles.SelectedItem.Render(title)
		}
		fmt.Fprintf(&b, "%s%s\n", cursorStr, title)
	}
	return b.String()
}

// footer renders the hint bar for the current state: search hints while typing,
// otherwise the normal action hints (with any transient status prefixed).
func (m Model) footer() string {
	if m.searching {
		return fmt.Sprintf(" Search: %s  •  [esc] Cancel  •  [enter] Apply  •  [↑/↓] Move", m.query)
	}
	footerText := " [1/2/3] Views  •  [esc] Repo  •  [tab] Focus  •  [j/k] Scroll  •  [/] Search  •  [c] Checkout  •  [o] Web  •  [r] Refresh  •  [q] Quit"
	if status := m.statusLine(); status != "" {
		footerText = fmt.Sprintf(" %s | %s", status, footerText)
	}
	return footerText
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
