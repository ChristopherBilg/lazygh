// Package pr implements the pull-request split-pane screen.
package pr

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// confirmKind identifies which impactful action a footer y/n prompt is guarding.
type confirmKind int

// Confirmation prompt states. confirmNone means no prompt is active.
const (
	confirmNone confirmKind = iota
	confirmMerge
	confirmClose
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

// prFilter is the active quick filter applied to the PR list.
type prFilter int

// Quick-filter values. filterAll shows every PR.
const (
	filterAll prFilter = iota
	filterMine
	filterNeedsReview
	filterDependabot
)

// dependabotLogin is the author login GitHub assigns to Dependabot PRs.
const dependabotLogin = "dependabot[bot]"

// label returns the human-readable filter name shown in the badge/footer.
func (f prFilter) label() string {
	switch f {
	case filterMine:
		return "My PRs"
	case filterNeedsReview:
		return "Needs my Review"
	case filterDependabot:
		return "Dependabot"
	default:
		return "All"
	}
}

// Backend is the subset of the github client the PR screen needs.
type Backend interface {
	RepoPRs(ctx context.Context, owner, name string, force bool) (ghClient.RepoContext, error)
	CheckoutPR(ctx context.Context, owner, name string, prNumber int) error
	OpenPRInBrowser(ctx context.Context, owner, name string, prNumber int) error
	ApprovePR(ctx context.Context, owner, name string, prNumber int) error
	MergePR(ctx context.Context, owner, name string, prNumber int) error
	ClosePR(ctx context.Context, owner, name string, prNumber int) error
	PRComments(ctx context.Context, owner, name string, prNumber int, force bool) ([]ghClient.PRComment, error)
	CurrentUser(ctx context.Context) (string, error)
	RepoPRChecks(ctx context.Context, owner, name string) (map[int]ghClient.CheckStatus, error)
}

// Model is the pull-request split-pane screen.
type Model struct {
	backend     Backend
	ctx         ghClient.RepoContext
	cursor      int
	focus       focus
	loading     bool
	refreshing  bool
	fetchErr    error
	message     string
	spinner     spinner.Model
	viewport    viewport.Model
	input       textinput.Model // the "/" filter field
	searching   bool            // input focused / capturing keystrokes
	query       string          // applied filter; "" = no filter
	confirm     confirmKind     // active impactful-action confirmation, or confirmNone
	confirmPR   int             // PR number snapshotted when the confirm prompt opened
	filter      prFilter        // active quick filter; filterAll = no filter
	currentUser string          // authenticated user's login; "" until resolved
	filtered    []int           // indices into ctx.PRs, in display (ranked) order
	width       int
	height      int
	ready       bool
	comments    map[int]commentState         // per-PR comment load state, keyed by PR number
	checks      map[int]ghClient.CheckStatus // per-PR aggregate check status, keyed by PR number
	activeTab   tabs.Tab                     // selected right-pane tab; persists across PR changes
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

// filterMatch reports whether pr belongs to the active quick filter. The
// user-dependent filters never match while the current user is unresolved.
func (m Model) filterMatch(pr ghClient.PullRequest) bool {
	switch m.filter {
	case filterMine:
		return m.currentUser != "" && strings.EqualFold(pr.User.Login, m.currentUser)
	case filterNeedsReview:
		if m.currentUser == "" {
			return false
		}
		for _, r := range pr.RequestedReviewers {
			if strings.EqualFold(r.Login, m.currentUser) {
				return true
			}
		}
		return false
	case filterDependabot:
		return strings.EqualFold(pr.User.Login, dependabotLogin)
	default: // filterAll
		return true
	}
}

// filterIndices returns the indices of ctx.PRs matching the active quick filter,
// in natural (unranked) order.
func (m Model) filterIndices() []int {
	out := make([]int, 0, len(m.ctx.PRs))
	for i := range m.ctx.PRs {
		if m.filterMatch(m.ctx.PRs[i]) {
			out = append(out, i)
		}
	}
	return out
}

// recompute rebuilds the filtered (visible) index list from the current query,
// active quick filter, and PR set, clamps the cursor into range, and refreshes
// the detail viewport. It is the single place the visible set is derived, so
// filtering, refresh, and cancel all stay consistent.
func (m *Model) recompute() {
	base := m.filterIndices()
	if m.query == "" {
		m.filtered = base
	} else {
		titles := make([]string, len(base))
		for i, idx := range base {
			titles[i] = m.ctx.PRs[idx].Title
		}
		ranked := fuzzy.Rank(m.query, titles) // indices into base
		m.filtered = make([]int, len(ranked))
		for i, r := range ranked {
			m.filtered[i] = base[r]
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(len(m.filtered)-1, 0)
	}
	m.updateViewportContent()
	m.viewport.GotoTop()
}

// beginForcedRefresh starts a cache-bypassing PR refetch, keeping existing PRs
// visible (footer spinner) and re-entering the full loading view only when there
// are no PRs yet. It returns the commands to run and deliberately does NOT touch
// m.message, so a status line set by the caller (e.g. "Merged PR #42") survives.
func (m *Model) beginForcedRefresh() []tea.Cmd {
	wasFetching := m.loading || m.refreshing
	m.fetchErr = nil
	if len(m.ctx.PRs) == 0 {
		m.loading = true
	} else {
		m.refreshing = true
	}
	cmds := []tea.Cmd{m.fetchPRsCmd(m.ctx.Owner, m.ctx.Name, true)}
	if !wasFetching {
		cmds = append(cmds, m.spinner.Tick)
	}
	return cmds
}

// setFilter applies quick filter f, toggling back to filterAll when f is already
// active. It resets the cursor to the top and recomputes. It is a no-op while the
// list is unavailable (loading, or a fatal error with no PRs to show).
func (m *Model) setFilter(f prFilter) {
	// Filters are a list-pane action (like search): ignore them when the detail
	// pane is focused, where the same keys drive viewport scrolling (e.g. "d" =
	// half-page-down). Also a no-op while the list is unavailable.
	if m.focus != focusList || m.loading || (m.fetchErr != nil && len(m.ctx.PRs) == 0) {
		return
	}
	if m.filter == f {
		f = filterAll // pressing the active filter's key again clears it
	}
	m.filter = f
	m.cursor = 0
	m.recompute()
}

// selectedPR returns the PR under the cursor within the filtered set, or false
// when the filtered set is empty.
func (m Model) selectedPR() (ghClient.PullRequest, bool) {
	if len(m.filtered) == 0 {
		return ghClient.PullRequest{}, false
	}
	return m.ctx.PRs[m.filtered[m.cursor]], true
}

// CapturingInput reports whether this screen must receive every key: while the
// filter field is focused (typing a search), or while an impactful-action
// confirmation prompt is pending. The router consults it (via
// screen.InputCapturer) to suppress global keys in those modes.
func (m Model) CapturingInput() bool { return m.searching || m.confirm != confirmNone }

// updateConfirm handles keys while an impactful-action confirmation prompt is up:
// "y"/"Y" runs the pending action against the snapshotted PR number; every other
// key cancels. The prompt is cleared in all cases.
func (m Model) updateConfirm(msg tea.KeyMsg) (screen.Model, tea.Cmd) {
	kind, prNumber := m.confirm, m.confirmPR
	m.confirm = confirmNone
	if s := msg.String(); s == "y" || s == "Y" {
		switch kind {
		case confirmMerge:
			m.message = fmt.Sprintf("Merging PR #%d...", prNumber)
			return m, m.mergeCmd(m.ctx.Owner, m.ctx.Name, prNumber)
		case confirmClose:
			m.message = fmt.Sprintf("Closing PR #%d...", prNumber)
			return m, m.closeCmd(m.ctx.Owner, m.ctx.Name, prNumber)
		}
	}
	return m, nil
}

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

// currentUserMsg carries the resolved authenticated-user login to the PR screen.
type currentUserMsg string

// TargetView addresses the resolved login to the pull-request screen so it is
// delivered even after a mid-fetch tab switch.
func (currentUserMsg) TargetView() screen.ViewID { return screen.ViewPR }

// statusMsg carries a transient footer status message.
type statusMsg string

// actionResultMsg carries the outcome of a PR action (approve/merge/close): a
// pre-formatted footer line and whether it succeeded (success triggers a refresh).
type actionResultMsg struct {
	text string
	ok   bool
}

// TargetView addresses an action result to the PR screen so it is delivered even
// after a mid-action tab switch.
func (actionResultMsg) TargetView() screen.ViewID { return screen.ViewPR }

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

func (m Model) approveCmd(owner, name string, prNumber int) tea.Cmd {
	return func() tea.Msg {
		// context.Background() for now; a program-scoped context is future work.
		if err := m.backend.ApprovePR(context.Background(), owner, name, prNumber); err != nil {
			return actionResultMsg{text: fmt.Sprintf("Approve PR #%d failed: %v", prNumber, err)}
		}
		return actionResultMsg{text: fmt.Sprintf("Approved PR #%d", prNumber), ok: true}
	}
}

func (m Model) mergeCmd(owner, name string, prNumber int) tea.Cmd {
	return func() tea.Msg {
		// context.Background() for now; a program-scoped context is future work.
		if err := m.backend.MergePR(context.Background(), owner, name, prNumber); err != nil {
			return actionResultMsg{text: fmt.Sprintf("Merge PR #%d failed: %v", prNumber, err)}
		}
		return actionResultMsg{text: fmt.Sprintf("Merged PR #%d", prNumber), ok: true}
	}
}

func (m Model) closeCmd(owner, name string, prNumber int) tea.Cmd {
	return func() tea.Msg {
		// context.Background() for now; a program-scoped context is future work.
		if err := m.backend.ClosePR(context.Background(), owner, name, prNumber); err != nil {
			return actionResultMsg{text: fmt.Sprintf("Close PR #%d failed: %v", prNumber, err)}
		}
		return actionResultMsg{text: fmt.Sprintf("Closed PR #%d", prNumber), ok: true}
	}
}

// currentUserCmd resolves the authenticated user's login (memoized in the
// client). A failure is logged and reported as an empty login, which simply
// leaves the user-dependent filters matching nothing.
func (m Model) currentUserCmd() tea.Cmd {
	return func() tea.Msg {
		// context.Background() for now; a program-scoped context is future work.
		login, err := m.backend.CurrentUser(context.Background())
		if err != nil {
			slog.Warn("could not resolve current user; My PRs / Needs my Review filters will be empty", "err", err)
			return currentUserMsg("")
		}
		return currentUserMsg(login)
	}
}

// prChecksMsg carries the freshly fetched per-PR aggregate check statuses.
type prChecksMsg struct{ checks map[int]ghClient.CheckStatus }

// TargetView addresses fetched check statuses to the pull-request screen so they are
// delivered even after a mid-fetch tab switch.
func (prChecksMsg) TargetView() screen.ViewID { return screen.ViewPR }

// prChecksErrMsg reports a failed check-status fetch; indicators are left neutral.
type prChecksErrMsg struct{ err error }

// TargetView addresses the check-fetch error to the pull-request screen.
func (prChecksErrMsg) TargetView() screen.ViewID { return screen.ViewPR }

// fetchChecksCmd fetches every open PR's aggregate check status in the background.
// RepoPRChecks already logs the cause on failure; the handler leaves indicators
// neutral. The fetch is non-blocking so the list renders before statuses arrive.
func (m Model) fetchChecksCmd(owner, name string) tea.Cmd {
	return func() tea.Msg {
		// context.Background() for now; a program-scoped context is future work.
		checks, err := m.backend.RepoPRChecks(context.Background(), owner, name)
		if err != nil {
			return prChecksErrMsg{err: err}
		}
		return prChecksMsg{checks: checks}
	}
}

// Init starts fetching pull requests (from cache when available), resolving
// the current user, and the spinner.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchPRsCmd(m.ctx.Owner, m.ctx.Name, false), m.currentUserCmd(), m.spinner.Tick)
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
		if m.confirm != confirmNone {
			return m.updateConfirm(msg)
		}
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
		case key.Matches(msg, keys.Map.FilterMine):
			m.setFilter(filterMine)
		case key.Matches(msg, keys.Map.FilterReview):
			m.setFilter(filterNeedsReview)
		case key.Matches(msg, keys.Map.FilterDependabot):
			m.setFilter(filterDependabot)
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
		case key.Matches(msg, keys.Map.Approve):
			if pr, ok := m.selectedPR(); ok {
				m.message = fmt.Sprintf("Approving PR #%d...", pr.Number)
				cmds = append(cmds, m.approveCmd(m.ctx.Owner, m.ctx.Name, pr.Number))
			}
		case key.Matches(msg, keys.Map.Merge):
			if pr, ok := m.selectedPR(); ok {
				m.confirm = confirmMerge
				m.confirmPR = pr.Number
			}
		case key.Matches(msg, keys.Map.Close):
			if pr, ok := m.selectedPR(); ok {
				m.confirm = confirmClose
				m.confirmPR = pr.Number
			}
		case key.Matches(msg, keys.Map.Refresh):
			// Manual refresh: bypass the cache, keep the current PRs visible.
			m.message = ""
			cmds = append(cmds, m.beginForcedRefresh()...)
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
		cmds = append(cmds, m.fetchChecksCmd(m.ctx.Owner, m.ctx.Name))
		if cmd := m.maybeFetchComments(); cmd != nil {
			m.updateViewportContent() // reflect the loading placeholder
			cmds = append(cmds, cmd)
		}

	case currentUserMsg:
		m.currentUser = string(msg)
		// Re-apply an active user-dependent filter now that the login is known.
		if m.filter == filterMine || m.filter == filterNeedsReview {
			m.recompute()
		}

	case screen.FetchErrMsg:
		m.loading = false
		m.refreshing = false
		m.fetchErr = msg.Err

	case statusMsg:
		m.message = string(msg)

	case actionResultMsg:
		m.message = msg.text
		if msg.ok {
			cmds = append(cmds, m.beginForcedRefresh()...)
		}

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

	case prChecksMsg:
		m.checks = msg.checks // the next View() frame renders the icons

	case prChecksErrMsg:
		// Non-fatal; indicators stay neutral. RepoPRChecks already logged the cause
		// at Warn; note it here at Debug for TUI-side tracing.
		slog.Debug("pr check statuses unavailable; indicators left neutral", "err", msg.err)

	default:
		// Forward any other message (e.g. the textinput's cursor-blink tick) to
		// the filter input while it is focused, so the blink loop keeps running.
		if m.searching {
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	// Forward to the detail viewport, but only let it consume key presses when the
	// detail pane is focused. Otherwise the viewport's built-in bindings (j/k and
	// d/u/f/b/space/pgup/pgdn) would scroll the detail pane while the user is
	// navigating the list or toggling a filter — e.g. "d" (Dependabot filter) also
	// triggers the viewport's half-page-down. Non-key messages (mouse, etc.) still pass.
	if _, isKey := msg.(tea.KeyMsg); !isKey || m.focus == focusDetails {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

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
		if m.query != "" || m.filter != filterAll {
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

// filterBadge returns the left-pane status badge for the active filter and/or
// query, or "" when neither is active.
func (m Model) filterBadge() string {
	filterActive := m.filter != filterAll
	queryActive := m.query != ""
	switch {
	case filterActive && queryActive:
		return fmt.Sprintf("filter: %s · %q (%d/%d)", m.filter.label(), m.query, len(m.filtered), len(m.ctx.PRs))
	case filterActive:
		return fmt.Sprintf("filter: %s (%d/%d)", m.filter.label(), len(m.filtered), len(m.ctx.PRs))
	case queryActive:
		return fmt.Sprintf("filter: %q (%d/%d)", m.query, len(m.filtered), len(m.ctx.PRs))
	default:
		return ""
	}
}

// emptyListMessage explains why the visible list is empty, given the active
// filter and/or query.
func (m Model) emptyListMessage() string {
	filterActive := m.filter != filterAll
	// A user-dependent filter with no resolved login can never match; say why.
	unresolvedLogin := (m.filter == filterMine || m.filter == filterNeedsReview) && m.currentUser == ""
	switch {
	case filterActive && m.query != "":
		if unresolvedLogin {
			return fmt.Sprintf("No PRs match filter: %s and %q (couldn't determine your GitHub login).", m.filter.label(), m.query)
		}
		return fmt.Sprintf("No PRs match filter: %s and %q.", m.filter.label(), m.query)
	case filterActive:
		if unresolvedLogin {
			return fmt.Sprintf("No PRs match filter: %s (couldn't determine your GitHub login).", m.filter.label())
		}
		return fmt.Sprintf("No PRs match filter: %s.", m.filter.label())
	case m.query != "":
		return fmt.Sprintf("No PRs match %q.", m.query)
	default:
		return "No open PRs."
	}
}

// checkIconWidth is the display columns reserved for a PR row's status cell (glyph
// plus trailing separator), so titles start at the same column on every row
// regardless of how the terminal sizes emoji.
const checkIconWidth = 3

// checkIcon returns s's status glyph padded with spaces to exactly checkIconWidth
// display columns (measured with lipgloss.Width, since an emoji may render as 1 or 2
// cells). CheckNone renders as blanks — the same width, with no misleading icon.
func checkIcon(s ghClient.CheckStatus) string {
	glyph := "" // CheckNone
	switch s {
	case ghClient.CheckPassing:
		glyph = "✅"
	case ghClient.CheckFailing:
		glyph = "❌"
	case ghClient.CheckPending:
		glyph = "🔄"
	}
	pad := max(checkIconWidth-lipgloss.Width(glyph), 0)
	return glyph + strings.Repeat(" ", pad)
}

// renderList renders the left-pane contents: an optional search input or filter
// badge, then the filtered PR rows (or a no-results message).
func (m Model) renderList(paneWidth int) string {
	var b strings.Builder

	switch {
	case m.searching:
		b.WriteString(m.input.View())
		b.WriteString("\n")
	default:
		if badge := m.filterBadge(); badge != "" {
			b.WriteString(styles.Title.Render(styles.TruncateEllipsis(badge, paneWidth-2)))
			b.WriteString("\n")
		}
	}

	if len(m.filtered) == 0 {
		b.WriteString(styles.TruncateEllipsis(m.emptyListMessage(), paneWidth-2))
		return b.String()
	}

	for row, idx := range m.filtered {
		pr := m.ctx.PRs[idx]
		cursorStr := "  "
		icon := checkIcon(m.checks[pr.Number]) // missing key → CheckNone → blank cell
		// Reserve columns for the 2-col cursor prefix and the status icon cell so the
		// title never overflows the pane. TruncateEllipsis is width-aware and never
		// panics on narrow widths.
		title := styles.TruncateEllipsis(fmt.Sprintf("#%d %s", pr.Number, pr.Title), paneWidth-2-checkIconWidth)
		if m.cursor == row {
			cursorStr = "> "
			title = styles.SelectedItem.Render(title)
		}
		fmt.Fprintf(&b, "%s%s%s\n", cursorStr, icon, title)
	}
	return b.String()
}

// footer renders the hint bar for the current state: an impactful-action
// confirmation prompt, search hints while typing, or otherwise the normal
// action hints (with any transient status prefixed).
func (m Model) footer() string {
	if m.confirm != confirmNone {
		verb := "Merge"
		if m.confirm == confirmClose {
			verb = "Close"
		}
		return fmt.Sprintf(" %s PR #%d?  •  [y] Yes  •  [n/esc] No", verb, m.confirmPR)
	}
	if m.searching {
		return fmt.Sprintf(" Search: %s  •  [esc] Cancel  •  [enter] Apply  •  [↑/↓] Move", m.query)
	}
	filterHint := "[m/v/d] Filter"
	if m.filter != filterAll {
		filterHint = "[m/v/d] Filter (again clears)"
	}
	footerText := fmt.Sprintf(" [1/2/3] Views  •  [esc] Repo  •  [tab] Focus  •  [[/]] Tabs  •  [j/k] Scroll  •  [/] Search  •  %s  •  [c] Checkout  •  [o] Web  •  [a/M/D] Approve/Merge/Close  •  [r] Refresh  •  [q] Quit", filterHint)
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
