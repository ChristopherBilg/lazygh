// Package pr implements the pull-request split-pane screen.
package pr

import (
	"context"
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
	"github.com/ChristopherBilg/lazygh/internal/tui/pr/tabs"
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

// commentStatus tracks the lifecycle of a PR's lazily-loaded comments.
type commentStatus int

// Comment load lifecycle states, in the order a PR's comments progress through.
const (
	commentsNotLoaded commentStatus = iota
	commentsLoading
	commentsLoaded
	commentsErrored
)

// commentState is the per-PR comment load state stored in Model.comments.
type commentState struct {
	status commentStatus
	list   []ghClient.PRComment
	err    error
}

// Backend is the subset of the github client the PR screen needs.
type Backend interface {
	RepoPRs(ctx context.Context, owner, name string, force bool) (ghClient.RepoContext, error)
	CheckoutPR(ctx context.Context, owner, name string, prNumber int) error
	OpenPRInBrowser(ctx context.Context, owner, name string, prNumber int) error
	PRComments(ctx context.Context, owner, name string, prNumber int, force bool) ([]ghClient.PRComment, error)
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
	input      textinput.Model // the "/" filter field
	searching  bool            // input focused / capturing keystrokes
	query      string          // applied filter; "" = no filter
	filtered   []int           // indices into ctx.PRs, in display (ranked) order
	width      int
	height     int
	ready      bool
	comments   map[int]commentState // per-PR comment load state, keyed by PR number
	activeTab  tabs.Tab             // selected right-pane tab; persists across PR changes
}

// New returns a PR screen for the given repository, sized to the current window,
// backed by the given github client. The repository owner/name are stored
// immediately so the loading view can show the repository name.
func New(backend Backend, owner, name string, width, height int) Model {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "filter titles"
	ti.CharLimit = 128

	m := Model{
		backend:  backend,
		ctx:      ghClient.RepoContext{Owner: owner, Name: name},
		focus:    focusList,
		loading:  true,
		spinner:  spinner.New(spinner.WithSpinner(spinner.Dot)),
		input:    ti,
		width:    width,
		height:   height,
		comments: make(map[int]commentState),
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
		return m, m.maybeFetchComments()
	case tea.KeyUp:
		var cmd tea.Cmd
		if m.cursor > 0 {
			m.cursor--
			cmd = m.maybeFetchComments()
			m.updateViewportContent()
			m.viewport.GotoTop()
		}
		return m, cmd
	case tea.KeyDown:
		var cmd tea.Cmd
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			cmd = m.maybeFetchComments()
			m.updateViewportContent()
			m.viewport.GotoTop()
		}
		return m, cmd
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != m.query {
		m.query = m.input.Value()
		m.cursor = 0 // best match to the top on each query change
		m.recompute()
		// A query-driven recompute can change the selected PR; fetch its comments
		// if the Comments tab is active (batched with the input's own command).
		return m, tea.Batch(cmd, m.maybeFetchComments())
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

// prCommentsMsg carries fetched conversation comments for a specific PR.
type prCommentsMsg struct {
	prNumber int
	comments []ghClient.PRComment
}

// TargetView addresses fetched comments to the pull-request screen.
func (prCommentsMsg) TargetView() screen.ViewID { return screen.ViewPR }

// prCommentsErrMsg reports a failed comment fetch for a specific PR; it renders
// inside the Comments tab rather than as a global error.
type prCommentsErrMsg struct {
	prNumber int
	err      error
}

// TargetView addresses the comment error to the pull-request screen.
func (prCommentsErrMsg) TargetView() screen.ViewID { return screen.ViewPR }

// fetchCommentsCmd fetches the given PR's conversation comments. A client-init
// failure is fatal (ErrMsg); any other failure renders inside the Comments tab
// (prCommentsErrMsg).
func (m Model) fetchCommentsCmd(prNumber int) tea.Cmd {
	owner, name := m.ctx.Owner, m.ctx.Name
	return func() tea.Msg {
		// context.Background() for now; a program-scoped context is future work.
		cs, err := m.backend.PRComments(context.Background(), owner, name, prNumber, false)
		if err != nil {
			if errors.Is(err, ghClient.ErrClientInit) {
				return screen.ErrMsg{Err: err}
			}
			return prCommentsErrMsg{prNumber: prNumber, err: err}
		}
		return prCommentsMsg{prNumber: prNumber, comments: cs}
	}
}

// maybeFetchComments returns a command to load the selected PR's comments when
// the Comments tab is active and that PR's comments are not already loaded or in
// flight (an errored PR is retried). It marks the per-PR state loading so the view
// shows the placeholder and no duplicate fetch is issued, and returns the fetch
// command; it returns nil when no fetch is needed (safe to append to a cmd slice —
// tea.Batch drops nils). Callers run updateViewportContent afterward so the
// loading placeholder renders immediately.
func (m *Model) maybeFetchComments() tea.Cmd {
	if m.activeTab != tabs.Comments {
		return nil
	}
	pr, ok := m.selectedPR()
	if !ok {
		return nil
	}
	switch m.comments[pr.Number].status {
	case commentsLoading, commentsLoaded:
		return nil
	}
	m.comments[pr.Number] = commentState{status: commentsLoading}
	return m.fetchCommentsCmd(pr.Number)
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
		if m.searching {
			return m.updateSearch(msg)
		}
		switch {
		case key.Matches(msg, keys.Map.Search):
			// Only open search when the list pane is actually on screen. View()
			// returns early while loading or on a fatal load error, so entering
			// capture then would route keys (e.g. the "r" retry) into an invisible
			// input.
			if m.focus == focusList && !m.loading && (m.fetchErr == nil || len(m.ctx.PRs) > 0) {
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
					cmds = append(cmds, m.maybeFetchComments())
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
					cmds = append(cmds, m.maybeFetchComments())
					m.updateViewportContent()
					m.viewport.GotoTop()
				}
			} else {
				m.viewport.ScrollDown(1)
			}
		case key.Matches(msg, keys.Map.Checkout):
			if pr, ok := m.selectedPR(); ok {
				m.message = "Checking out branch..."
				cmds = append(cmds, m.checkoutCmd(m.ctx.Owner, m.ctx.Name, pr.Number))
			}
		case key.Matches(msg, keys.Map.Open):
			if pr, ok := m.selectedPR(); ok {
				m.message = "Opening browser..."
				cmds = append(cmds, m.openBrowserCmd(m.ctx.Owner, m.ctx.Name, pr.Number))
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
		case key.Matches(msg, keys.Map.NextTab):
			m.activeTab = m.activeTab.Next()
			cmds = append(cmds, m.maybeFetchComments())
			m.updateViewportContent()
			m.viewport.GotoTop()
		case key.Matches(msg, keys.Map.PrevTab):
			m.activeTab = m.activeTab.Prev()
			cmds = append(cmds, m.maybeFetchComments())
			m.updateViewportContent()
			m.viewport.GotoTop()
		}

	case prDataMsg:
		m.ctx = ghClient.RepoContext(msg)
		m.loading = false
		m.refreshing = false
		m.fetchErr = nil
		m.recompute() // re-apply the active query to the new data and clamp the cursor
		if cmd := m.maybeFetchComments(); cmd != nil {
			m.updateViewportContent() // reflect the loading placeholder
			cmds = append(cmds, cmd)
		}

	case screen.FetchErrMsg:
		m.loading = false
		m.refreshing = false
		m.fetchErr = msg.Err

	case statusMsg:
		m.message = string(msg)

	case prCommentsMsg:
		m.comments[msg.prNumber] = commentState{status: commentsLoaded, list: msg.comments}
		if pr, ok := m.selectedPR(); ok && pr.Number == msg.prNumber {
			m.updateViewportContent()
		}

	case prCommentsErrMsg:
		m.comments[msg.prNumber] = commentState{status: commentsErrored, err: msg.err}
		if pr, ok := m.selectedPR(); ok && pr.Number == msg.prNumber {
			m.updateViewportContent()
		}

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
	contentHeight := max(m.height-headerHeight-footerHeight, 1)
	rightPaneWidth := (m.width * 7) / 10
	vpWidth := max(rightPaneWidth-2, 1)
	vpHeight := max(contentHeight-4, 1) // -2 pane border, -2 tab bar + blank line

	if !m.ready {
		m.viewport = viewport.New(vpWidth, vpHeight)
		m.ready = true
	} else {
		m.viewport.Width = vpWidth
		m.viewport.Height = vpHeight
	}
	// The filter input sits inside the left pane; size it to the inner width
	// (minus borders and the "/ " prompt) so a long query scrolls within the box.
	leftPaneWidth := (m.width * 3) / 10
	m.input.Width = max(leftPaneWidth-5, 1)
	m.updateViewportContent()
}

func (m *Model) updateViewportContent() {
	activePR, ok := m.selectedPR()
	if !ok {
		if m.query != "" {
			m.viewport.SetContent("") // the list pane shows the "no match" message
		} else {
			m.viewport.SetContent("No open PRs.")
		}
		return
	}

	contentStyle := lipgloss.NewStyle().Width(m.viewport.Width)

	var body string
	switch m.activeTab {
	case tabs.FilesChanged:
		body = filesChangedPlaceholder
	case tabs.Comments:
		body = m.renderComments(activePR.Number)
	default: // tabs.Description
		body = descriptionContent(activePR)
	}

	m.viewport.SetContent(contentStyle.Render(body))
}

// descriptionContent renders the Description tab: the PR title, state, and body.
func descriptionContent(pr ghClient.PullRequest) string {
	body := pr.Body
	if body == "" {
		body = "*No description provided.*"
	}
	return fmt.Sprintf("%s\nState: %s\n\n%s", styles.Title.Render(pr.Title), pr.State, body)
}

// filesChangedPlaceholder is shown on the Files Changed tab until the diff-viewer
// work item fills it in.
const filesChangedPlaceholder = "Files changed\n\n" +
	"The diff viewer for this tab is coming in a separate work item."

// renderComments renders the Comments tab for the given PR from its cached load
// state: a loading placeholder, an in-tab error, an empty state, or the thread in
// API (chronological, reading) order.
func (m Model) renderComments(prNumber int) string {
	st := m.comments[prNumber]
	switch st.status {
	case commentsErrored:
		return styles.Error.Render(fmt.Sprintf("Failed to load comments: %v", st.err))
	case commentsLoaded:
		if len(st.list) == 0 {
			return "No comments yet."
		}
		var b strings.Builder
		for i, c := range st.list {
			if i > 0 {
				b.WriteString("\n\n")
			}
			fmt.Fprintf(&b, "%s · %s\n%s",
				styles.Title.Render(c.User.Login),
				c.CreatedAt.Format("2006-01-02 15:04"),
				c.Body)
		}
		return b.String()
	default: // commentsNotLoaded, commentsLoading
		return "Loading comments…"
	}
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

	var listBorder, detailBorder lipgloss.Style
	if m.focus == focusList {
		listBorder = styles.Active
		detailBorder = styles.Base
	} else {
		listBorder = styles.Base
		detailBorder = styles.Active
	}

	left := listBorder.Width(leftPaneWidth).Height(paneHeight).Render(m.renderList(leftPaneWidth))
	bar := styles.Truncate(tabs.Bar(m.activeTab), m.viewport.Width)
	right := detailBorder.Width(m.viewport.Width + 2).Height(paneHeight).Render(bar + "\n\n" + m.viewport.View())

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
		b.WriteString(styles.Title.Render(styles.TruncateEllipsis(badge, paneWidth-2)))
		b.WriteString("\n")
	}

	if len(m.filtered) == 0 {
		if m.query != "" {
			b.WriteString(styles.TruncateEllipsis(fmt.Sprintf("No PRs match %q.", m.query), paneWidth-2))
		} else {
			b.WriteString("No open PRs.")
		}
		return b.String()
	}

	for row, idx := range m.filtered {
		pr := m.ctx.PRs[idx]
		cursorStr := "  "
		// Reserve 2 columns for the cursor prefix so the title never overflows the
		// pane. TruncateEllipsis is width-aware and never panics on narrow widths.
		title := styles.TruncateEllipsis(fmt.Sprintf("#%d %s", pr.Number, pr.Title), paneWidth-2)
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
	footerText := " [1/2/3] Views  •  [esc] Repo  •  [tab] Focus  •  [[/]] Tabs  •  [j/k] Scroll  •  [/] Search  •  [c] Checkout  •  [o] Web  •  [r] Refresh  •  [q] Quit"
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
