package pr

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ChristopherBilg/lazygh/internal/config"
	ghClient "github.com/ChristopherBilg/lazygh/internal/github"
	"github.com/ChristopherBilg/lazygh/internal/tui/keys"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
)

// fakeBackend is a pr.Backend test double. RepoPRs/OpenPRInBrowser return the
// injected prs/prsErr/openErr (zero values mean "succeed trivially"); CheckoutPR
// returns checkoutErr. This lets checkout/open/fetch result handling be
// exercised without spawning `gh` or requiring a local git repository.
type fakeBackend struct {
	checkoutErr    error
	prs            []ghClient.PullRequest
	prsErr         error
	openErr        error
	currentUser    string
	currentUserErr error
}

func (f fakeBackend) RepoPRs(_ context.Context, owner, name string, _ bool) (ghClient.RepoContext, error) {
	return ghClient.RepoContext{Owner: owner, Name: name, PRs: f.prs}, f.prsErr
}

func (f fakeBackend) CheckoutPR(_ context.Context, _, _ string, _ int) error { return f.checkoutErr }

func (f fakeBackend) OpenPRInBrowser(_ context.Context, _, _ string, _ int) error { return f.openErr }

func (f fakeBackend) CurrentUser(context.Context) (string, error) {
	return f.currentUser, f.currentUserErr
}

// withPRs builds a loaded PR screen with n synthetic pull requests.
func withPRs(n int) Model {
	m := New(fakeBackend{}, "octocat", "hello", 100, 40)
	m.loading = false
	prs := make([]ghClient.PullRequest, n)
	for i := range prs {
		prs[i] = ghClient.PullRequest{Number: i + 1, Title: "Title", State: "open"}
	}
	m.ctx.PRs = prs
	m.recompute()
	return m
}

// withTitledPRs builds a loaded PR screen with one PR per given title (numbered
// from 1), so search/ranking behavior can be asserted.
func withTitledPRs(titles ...string) Model {
	m := New(fakeBackend{}, "octocat", "hello", 120, 40)
	m.loading = false
	prs := make([]ghClient.PullRequest, len(titles))
	for i, title := range titles {
		prs[i] = ghClient.PullRequest{Number: i + 1, Title: title, State: "open"}
	}
	m.ctx.PRs = prs
	m.recompute()
	return m
}

// enterSearch presses "/" and returns the resulting model.
func enterSearch(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	return updated.(Model)
}

// typeRunes feeds each rune of s to the model as a key press.
func typeRunes(m Model, s string) Model {
	for _, r := range s {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	return m
}

func TestTabTogglesFocus(t *testing.T) {
	t.Parallel()
	m := withPRs(2)
	if m.focus != focusList {
		t.Fatalf("initial focus = %d, want focusList", m.focus)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if updated.(Model).focus != focusDetails {
		t.Fatal("tab did not switch focus to details")
	}
	updated2, _ := updated.(Model).Update(tea.KeyMsg{Type: tea.KeyTab})
	if updated2.(Model).focus != focusList {
		t.Fatal("second tab did not switch focus back to list")
	}
}

func TestCursorNavigationInList(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		start      int
		key        tea.KeyMsg
		wantCursor int
	}{
		{"down moves", 0, tea.KeyMsg{Type: tea.KeyDown}, 1},
		{"up moves", 2, tea.KeyMsg{Type: tea.KeyUp}, 1},
		{"up at top stays", 0, tea.KeyMsg{Type: tea.KeyUp}, 0},
		{"down at bottom stays", 2, tea.KeyMsg{Type: tea.KeyDown}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := withPRs(3)
			m.cursor = tt.start
			updated, _ := m.Update(tt.key)
			if got := updated.(Model).cursor; got != tt.wantCursor {
				t.Fatalf("cursor = %d, want %d", got, tt.wantCursor)
			}
		})
	}
}

func TestCheckoutEmitsCommandAndSetsMessage(t *testing.T) {
	t.Parallel()
	m := withPRs(2)
	m.cursor = 1
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd == nil {
		t.Fatal("expected a command from checkout")
	}
	if got := updated.(Model).message; got != "Checking out branch..." {
		t.Fatalf("message = %q, want %q", got, "Checking out branch...")
	}
}

func TestOpenEmitsCommandAndSetsMessage(t *testing.T) {
	t.Parallel()
	m := withPRs(2)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd == nil {
		t.Fatal("expected a command from open")
	}
	if got := updated.(Model).message; got != "Opening browser..." {
		t.Fatalf("message = %q, want %q", got, "Opening browser...")
	}
}

func TestCheckoutEmptyListNoOp(t *testing.T) {
	t.Parallel()
	m := withPRs(0)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd != nil {
		t.Fatal("expected no command for empty PR list")
	}
	if got := updated.(Model).message; got != "" {
		t.Fatalf("message = %q, want empty", got)
	}
}

func TestPRDataMsgPopulates(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{}, "octocat", "hello", 100, 40)
	ctx := ghClient.RepoContext{
		Owner: "octocat",
		Name:  "hello",
		PRs:   []ghClient.PullRequest{{Number: 1, Title: "T", State: "open"}, {Number: 2}},
	}
	updated, _ := m.Update(prDataMsg(ctx))
	um := updated.(Model)
	if um.loading {
		t.Fatal("expected loading=false after prDataMsg")
	}
	if len(um.ctx.PRs) != 2 {
		t.Fatalf("expected 2 PRs, got %d", len(um.ctx.PRs))
	}
}

func TestViewRendersTabBar(t *testing.T) {
	t.Parallel()
	m := withPRs(1)
	v := m.View()
	for _, label := range []string{"Pull Requests", "Issues", "Actions"} {
		if !strings.Contains(v, label) {
			t.Fatalf("PR view missing tab label %q", label)
		}
	}
	if !strings.Contains(v, "[1/2/3] Views") {
		t.Fatalf("PR footer missing views hint:\n%s", v)
	}
}

func TestLoadingViewRendersTabBar(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{}, "octocat", "hello", 100, 40) // loading == true
	v := m.View()
	for _, label := range []string{"Pull Requests", "Issues", "Actions"} {
		if !strings.Contains(v, label) {
			t.Fatalf("loading PR view missing tab label %q:\n%s", label, v)
		}
	}
	if !strings.Contains(v, "Fetching PRs for hello") {
		t.Fatalf("loading view lost its fetch message:\n%s", v)
	}
}

func TestRefreshEntersRefreshingState(t *testing.T) {
	t.Parallel()
	m := withPRs(2)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	um := updated.(Model)
	if cmd == nil {
		t.Fatal("expected a refresh command, got nil")
	}
	if !um.refreshing {
		t.Fatal("expected refreshing=true after 'r' with existing PRs")
	}
	if um.loading {
		t.Fatal("refresh must not re-enter the loading state (existing PRs stay visible)")
	}
	if len(um.ctx.PRs) != 2 {
		t.Fatal("refresh must keep the existing PRs visible")
	}
}

func TestPRDataMsgClearsRefreshingState(t *testing.T) {
	t.Parallel()
	m := withPRs(2)
	m.refreshing = true
	ctx := ghClient.RepoContext{
		Owner: "octocat",
		Name:  "hello",
		PRs:   []ghClient.PullRequest{{Number: 1, Title: "T", State: "open"}},
	}
	updated, _ := m.Update(prDataMsg(ctx))
	if updated.(Model).refreshing {
		t.Fatal("expected refreshing=false after data lands")
	}
}

func TestPRDataMsgClampsCursorWhenListShrinks(t *testing.T) {
	t.Parallel()
	m := withPRs(3)
	m.cursor = 2
	ctx := ghClient.RepoContext{
		Owner: "octocat",
		Name:  "hello",
		PRs:   []ghClient.PullRequest{{Number: 1, Title: "T", State: "open"}},
	}
	// Must not panic: updateViewportContent indexes PRs[cursor].
	updated, _ := m.Update(prDataMsg(ctx))
	if got := updated.(Model).cursor; got != 0 {
		t.Fatalf("cursor = %d, want 0 after list shrank to 1", got)
	}
}

func TestPRDataMsgResetsScrollOnRefresh(t *testing.T) {
	t.Parallel()
	m := withPRs(1)
	// A tall body so the viewport can actually scroll.
	m.ctx.PRs[0].Body = strings.Repeat("line\n", 200)
	m.updateViewportContent()
	m.viewport.ScrollDown(50)
	if m.viewport.AtTop() {
		t.Fatal("precondition: expected viewport scrolled away from top")
	}
	ctx := ghClient.RepoContext{
		Owner: "octocat",
		Name:  "hello",
		PRs:   []ghClient.PullRequest{{Number: 1, Title: "T", State: "open", Body: strings.Repeat("line\n", 200)}},
	}
	updated, _ := m.Update(prDataMsg(ctx))
	if !updated.(Model).viewport.AtTop() {
		t.Fatal("refresh should reset the viewport scroll to the top")
	}
}

func TestPRDataMsgPreservesNonRefreshMessage(t *testing.T) {
	t.Parallel()
	m := withPRs(2)
	m.message = "Opened PR #1 in browser"
	ctx := ghClient.RepoContext{
		Owner: "octocat",
		Name:  "hello",
		PRs:   []ghClient.PullRequest{{Number: 1, Title: "T", State: "open"}},
	}
	updated, _ := m.Update(prDataMsg(ctx))
	if got := updated.(Model).message; got != "Opened PR #1 in browser" {
		t.Fatalf("message = %q, want it preserved (only \"Refreshing...\" should be cleared)", got)
	}
}

func TestPRDataMsgTargetView(t *testing.T) {
	t.Parallel()
	if prDataMsg(ghClient.RepoContext{}).TargetView() != screen.ViewPR {
		t.Fatal("prDataMsg must target the PR view")
	}
}

func TestPRDataMsgClearsFetchErr(t *testing.T) {
	t.Parallel()
	m := withPRs(1)
	m.fetchErr = errors.New("old")
	ctx := ghClient.RepoContext{Owner: "o", Name: "n", PRs: []ghClient.PullRequest{{Number: 1}}}
	updated, _ := m.Update(prDataMsg(ctx))
	if updated.(Model).fetchErr != nil {
		t.Fatal("expected fetchErr cleared after successful data")
	}
}

func TestFetchErrMsgWithNoDataShowsError(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{}, "octocat", "hello", 100, 40) // loading, no PRs
	updated, _ := m.Update(screen.FetchErrMsg{View: screen.ViewPR, Err: errors.New("boom")})
	um := updated.(Model)
	if um.loading {
		t.Fatal("expected loading cleared after fetch error")
	}
	v := um.View()
	if !strings.Contains(v, "Failed to load PRs") || !strings.Contains(v, "boom") {
		t.Fatalf("expected error view with reason, got:\n%s", v)
	}
	if !strings.Contains(v, "press r to retry") {
		t.Fatalf("expected retry hint, got:\n%s", v)
	}
}

func TestFetchErrMsgWithDataKeepsListAndFooterError(t *testing.T) {
	t.Parallel()
	m := withPRs(2)
	m.refreshing = true
	updated, _ := m.Update(screen.FetchErrMsg{View: screen.ViewPR, Err: errors.New("timeout")})
	um := updated.(Model)
	if um.refreshing {
		t.Fatal("expected refreshing cleared after fetch error")
	}
	if len(um.ctx.PRs) != 2 {
		t.Fatal("stale PRs must be retained on refresh error")
	}
	v := um.View()
	if !strings.Contains(v, "Refresh failed") || !strings.Contains(v, "timeout") {
		t.Fatalf("expected footer refresh error, got:\n%s", v)
	}
	if !strings.Contains(v, "Pull Requests") {
		t.Fatalf("expected the PR list still rendered, got:\n%s", v)
	}
}

func TestSpinnerTickIgnoredWhenIdle(t *testing.T) {
	t.Parallel()
	m := withPRs(1) // not loading, not refreshing
	_, cmd := m.Update(spinner.TickMsg{})
	if cmd != nil {
		t.Fatal("expected no tick command when idle (tick loop should stop)")
	}
}

func TestSpinnerTickContinuesWhileFetching(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{}, "octocat", "hello", 100, 40) // loading == true
	_, cmd := m.Update(spinner.TickMsg{})
	if cmd == nil {
		t.Fatal("expected the tick loop to continue while loading")
	}
}

func TestRefreshingViewShowsSpinnerFooter(t *testing.T) {
	t.Parallel()
	m := withPRs(2)
	m.refreshing = true
	v := m.View()
	if !strings.Contains(v, "Refreshing...") {
		t.Fatalf("expected footer to show Refreshing..., got:\n%s", v)
	}
	if !strings.Contains(v, "Pull Requests") {
		t.Fatalf("expected the PR list to stay visible while refreshing, got:\n%s", v)
	}
}

func TestStatusMessageShownInFooter(t *testing.T) {
	t.Parallel()
	m := withPRs(2)
	m.message = "Opened PR #1 in browser"
	v := m.View()
	if !strings.Contains(v, "Opened PR #1 in browser") {
		t.Fatalf("expected footer to show the status message, got:\n%s", v)
	}
}

func TestCheckoutCmdUnavailableMessage(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{checkoutErr: ghClient.ErrNotLocalRepo}, "octocat", "other", 100, 40)
	msg := m.checkoutCmd("octocat", "other", 42)()
	status, ok := msg.(statusMsg)
	if !ok {
		t.Fatalf("expected statusMsg, got %T", msg)
	}
	if got := string(status); !strings.Contains(got, "Checkout unavailable") || !strings.Contains(got, "octocat/other") {
		t.Fatalf("status = %q, want 'Checkout unavailable' mentioning octocat/other", got)
	}
}

func TestCheckoutCmdSuccessMessage(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{}, "octocat", "hello", 100, 40)
	msg := m.checkoutCmd("octocat", "hello", 7)()
	status, ok := msg.(statusMsg)
	if !ok {
		t.Fatalf("expected statusMsg, got %T", msg)
	}
	if got := string(status); !strings.Contains(got, "Successfully checked out PR #7") {
		t.Fatalf("status = %q, want success message", got)
	}
}

func TestCheckoutCmdFailureMessage(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{checkoutErr: errors.New("boom")}, "octocat", "hello", 100, 40)
	msg := m.checkoutCmd("octocat", "hello", 7)()
	status, ok := msg.(statusMsg)
	if !ok {
		t.Fatalf("expected statusMsg, got %T", msg)
	}
	if got := string(status); !strings.Contains(got, "Checkout failed") || !strings.Contains(got, "boom") {
		t.Fatalf("status = %q, want failure message", got)
	}
}

func TestOpenBrowserCmdSuccessMessage(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{}, "octocat", "hello", 100, 40)
	msg := m.openBrowserCmd("octocat", "hello", 9)()
	status, ok := msg.(statusMsg)
	if !ok {
		t.Fatalf("expected statusMsg, got %T", msg)
	}
	if got := string(status); !strings.Contains(got, "Opened PR #9 in browser") {
		t.Fatalf("status = %q, want success message", got)
	}
}

func TestOpenBrowserCmdFailureMessage(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{openErr: errors.New("boom")}, "octocat", "hello", 100, 40)
	msg := m.openBrowserCmd("octocat", "hello", 9)()
	status, ok := msg.(statusMsg)
	if !ok {
		t.Fatalf("expected statusMsg, got %T", msg)
	}
	if got := string(status); !strings.Contains(got, "Open in browser failed") || !strings.Contains(got, "boom") {
		t.Fatalf("status = %q, want failure message", got)
	}
}

func TestFetchPRsCmdSuccessReturnsData(t *testing.T) {
	t.Parallel()
	prs := []ghClient.PullRequest{{Number: 1, Title: "T", State: "open"}}
	m := New(fakeBackend{prs: prs}, "octocat", "hello", 100, 40)
	msg := m.fetchPRsCmd("octocat", "hello", false)()
	data, ok := msg.(prDataMsg)
	if !ok {
		t.Fatalf("expected prDataMsg, got %T", msg)
	}
	if len(data.PRs) != 1 {
		t.Fatalf("len(PRs) = %d, want 1", len(data.PRs))
	}
}

func TestFetchPRsCmdClientInitErrorIsFatal(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{prsErr: ghClient.ErrClientInit}, "octocat", "hello", 100, 40)
	msg := m.fetchPRsCmd("octocat", "hello", false)()
	errMsg, ok := msg.(screen.ErrMsg)
	if !ok {
		t.Fatalf("expected screen.ErrMsg, got %T", msg)
	}
	if !errors.Is(errMsg.Err, ghClient.ErrClientInit) {
		t.Fatalf("err = %v, want ErrClientInit", errMsg.Err)
	}
}

func TestFetchPRsCmdOtherErrorIsRecoverable(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{prsErr: errors.New("boom")}, "octocat", "hello", 100, 40)
	msg := m.fetchPRsCmd("octocat", "hello", false)()
	fetchErr, ok := msg.(screen.FetchErrMsg)
	if !ok {
		t.Fatalf("expected screen.FetchErrMsg, got %T", msg)
	}
	if fetchErr.View != screen.ViewPR {
		t.Fatalf("View = %v, want ViewPR", fetchErr.View)
	}
}

func TestCheckoutRemapTakesEffect(t *testing.T) {
	// No t.Parallel: keys.Configure mutates the process-wide key map.
	t.Cleanup(func() { keys.Configure(config.Default().Keys) })
	kc := config.Default().Keys
	kc.Checkout = []string{"x"}
	keys.Configure(kc)

	// Remapped key checks out.
	m := withPRs(2)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatal("expected checkout command on remapped key 'x'")
	}
	if got := updated.(Model).message; got != "Checking out branch..." {
		t.Fatalf("message = %q, want checkout message", got)
	}

	// Old default key is now inert for checkout.
	old, cmd2 := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if got := old.(Model).message; got != "" {
		t.Fatalf("message = %q, want empty ('c' should be inert after remap)", got)
	}
	_ = cmd2 // viewport may return a nil cmd; the message assertion above is the signal
}

func TestViewNeverPanicsOnNarrowWidths(t *testing.T) {
	t.Parallel()
	for w := 1; w <= 40; w++ {
		m := withPRs(3)
		m.width = w
		m.height = 24
		m.resizeViewport()
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("width=%d: View panicked: %v", w, r)
				}
			}()
			_ = m.View()
		}()
	}
}

func TestViewTruncatesWideRuneTitleByDisplayWidth(t *testing.T) {
	t.Parallel()
	m := withPRs(1)
	m.width = 60
	m.ctx.PRs[0].Title = strings.Repeat("世", 100) // each rune is 2 display cells
	m.updateViewportContent()
	v := m.View() // must not panic
	if !utf8.ValidString(v) {
		t.Fatal("View produced invalid UTF-8 (a multibyte rune was split)")
	}
}

func TestSlashEntersSearch(t *testing.T) {
	t.Parallel()
	m := enterSearch(t, withTitledPRs("Fix cache", "Add docs"))
	if !m.searching {
		t.Fatal("expected searching=true after '/'")
	}
	if !m.CapturingInput() {
		t.Fatal("expected CapturingInput()=true while searching")
	}
}

func TestSlashIgnoredWhenDetailFocused(t *testing.T) {
	t.Parallel()
	m := withTitledPRs("Fix cache", "Add docs")
	m.focus = focusDetails
	if enterSearch(t, m).searching {
		t.Fatal("'/' must not start search when the detail pane is focused")
	}
}

func TestSlashIgnoredWhileLoading(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{}, "octocat", "hello", 100, 40) // loading == true, no PRs
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if updated.(Model).searching {
		t.Fatal("'/' must not start search while the loading screen is shown")
	}
}

func TestSlashIgnoredOnFatalErrorScreen(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{}, "octocat", "hello", 100, 40)
	fe, _ := m.Update(screen.FetchErrMsg{View: screen.ViewPR, Err: errors.New("boom")})
	m = fe.(Model) // fatal error screen: loading cleared, fetchErr set, no PRs
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if updated.(Model).searching {
		t.Fatal("'/' must not start search on the fatal error screen (retry key would be swallowed)")
	}
}

func TestTypingFiltersList(t *testing.T) {
	t.Parallel()
	m := typeRunes(enterSearch(t, withTitledPRs("Fix cache race", "Add dark mode", "Refactor cache")), "cache")
	if m.query != "cache" {
		t.Fatalf("query = %q, want cache", m.query)
	}
	if len(m.filtered) != 2 {
		t.Fatalf("filtered len = %d, want 2", len(m.filtered))
	}
}

func TestArrowsNavigateAndLettersTypeWhileSearching(t *testing.T) {
	t.Parallel()
	m := typeRunes(enterSearch(t, withTitledPRs("cat", "car", "cab")), "a")
	if len(m.filtered) != 3 {
		t.Fatalf("filtered len = %d, want 3", len(m.filtered))
	}
	dn, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = dn.(Model)
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1 after Down while typing", m.cursor)
	}
	jm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if got := jm.(Model).query; got != "aj" {
		t.Fatalf("query = %q, want \"aj\" (j must be typed, not navigate)", got)
	}
}

func TestUpArrowNavigatesWhileSearching(t *testing.T) {
	t.Parallel()
	m := typeRunes(enterSearch(t, withTitledPRs("cat", "car", "cab")), "a") // all match; cursor 0
	dn, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = dn.(Model)
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1 after Down", m.cursor)
	}
	up, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := up.(Model).cursor; got != 0 {
		t.Fatalf("cursor = %d, want 0 after Up", got)
	}
}

func TestEnterCommitsFilter(t *testing.T) {
	t.Parallel()
	m := typeRunes(enterSearch(t, withTitledPRs("Fix cache", "Add docs")), "cache")
	ent, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = ent.(Model)
	if m.searching {
		t.Fatal("expected searching=false after Enter")
	}
	if m.query != "cache" || len(m.filtered) != 1 {
		t.Fatalf("query=%q filtered=%d, want cache/1 retained", m.query, len(m.filtered))
	}
	if m.CapturingInput() {
		t.Fatal("expected CapturingInput()=false after commit")
	}
}

func TestEscCancelsFilter(t *testing.T) {
	t.Parallel()
	m := typeRunes(enterSearch(t, withTitledPRs("Fix cache", "Add docs")), "cache")
	esc, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = esc.(Model)
	if m.searching {
		t.Fatal("expected searching=false after Esc")
	}
	if m.query != "" || len(m.filtered) != 2 {
		t.Fatalf("query=%q filtered=%d, want empty/2 (full list restored)", m.query, len(m.filtered))
	}
}

func TestSelectedPRUsesFilteredIndex(t *testing.T) {
	t.Parallel()
	m := typeRunes(enterSearch(t, withTitledPRs("Add docs", "Refactor cache", "Fix tests")), "cache")
	pr, ok := m.selectedPR()
	if !ok || pr.Number != 2 {
		t.Fatalf("selectedPR() = %+v ok=%v, want PR #2", pr, ok)
	}
}

func TestCheckoutActsOnFilteredSelection(t *testing.T) {
	t.Parallel()
	m := typeRunes(enterSearch(t, withTitledPRs("Add docs", "Refactor cache", "Fix tests")), "cache")
	ent, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = ent.(Model)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd == nil {
		t.Fatal("expected a checkout command on the filtered selection")
	}
	if got := updated.(Model).message; got != "Checking out branch..." {
		t.Fatalf("message = %q, want checkout message", got)
	}
}

func TestCheckoutNoOpWhenFilterEmpty(t *testing.T) {
	t.Parallel()
	m := typeRunes(enterSearch(t, withTitledPRs("Add docs", "Fix tests")), "zzz")
	ent, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = ent.(Model)
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}); cmd != nil {
		t.Fatal("expected no checkout command when the filter has no matches")
	}
}

func TestRefreshPreservesFilter(t *testing.T) {
	t.Parallel()
	m := typeRunes(enterSearch(t, withTitledPRs("Fix cache", "Add docs", "Refactor cache")), "cache")
	ent, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = ent.(Model)
	ctx := ghClient.RepoContext{Owner: "octocat", Name: "hello", PRs: []ghClient.PullRequest{
		{Number: 10, Title: "cache warmup", State: "open"},
		{Number: 11, Title: "unrelated", State: "open"},
	}}
	dm, _ := m.Update(prDataMsg(ctx))
	m = dm.(Model)
	if m.query != "cache" || len(m.filtered) != 1 {
		t.Fatalf("query=%q filtered=%d, want cache/1 preserved across refresh", m.query, len(m.filtered))
	}
	if pr, ok := m.selectedPR(); !ok || pr.Number != 10 {
		t.Fatalf("selected=%+v, want PR #10", pr)
	}
}

func TestCursorStaysValidAsFilterShrinks(t *testing.T) {
	t.Parallel()
	m := typeRunes(enterSearch(t, withTitledPRs("cab", "cad", "cae")), "ca")
	m.cursor = 2
	m = typeRunes(m, "b") // query "cab" → only "cab" matches
	if len(m.filtered) != 1 {
		t.Fatalf("filtered len = %d, want 1", len(m.filtered))
	}
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 (reset to top on query change)", m.cursor)
	}
	if _, ok := m.selectedPR(); !ok {
		t.Fatal("selectedPR must stay valid after the set shrank")
	}
}

func TestViewSearchingShowsHintsAndQuery(t *testing.T) {
	t.Parallel()
	m := typeRunes(enterSearch(t, withTitledPRs("Fix cache", "Add docs")), "cache")
	v := m.View()
	if !strings.Contains(v, "cache") {
		t.Fatalf("expected the typed query in the view:\n%s", v)
	}
	if !strings.Contains(v, "Cancel") || !strings.Contains(v, "Apply") {
		t.Fatalf("expected search footer hints while typing:\n%s", v)
	}
}

func TestViewCommittedShowsFilterBadge(t *testing.T) {
	t.Parallel()
	m := typeRunes(enterSearch(t, withTitledPRs("Fix cache", "Add docs")), "cache")
	ent, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	v := ent.(Model).View()
	if !strings.Contains(v, `filter: "cache"`) {
		t.Fatalf("expected filter badge, got:\n%s", v)
	}
	if !strings.Contains(v, "(1/2)") {
		t.Fatalf("expected filter count (1/2), got:\n%s", v)
	}
}

func TestViewNoResultsMessage(t *testing.T) {
	t.Parallel()
	m := typeRunes(enterSearch(t, withTitledPRs("Fix cache", "Add docs")), "zzzzz")
	if v := m.View(); !strings.Contains(v, "No PRs match") {
		t.Fatalf("expected a no-results message, got:\n%s", v)
	}
}

func TestViewFooterHasSearchHint(t *testing.T) {
	t.Parallel()
	if v := withTitledPRs("Fix cache", "Add docs").View(); !strings.Contains(v, "[/] Search") {
		t.Fatalf("expected [/] Search hint in footer, got:\n%s", v)
	}
}

func TestViewCommittedZeroMatchesShowsBadgeAndNoResults(t *testing.T) {
	t.Parallel()
	m := typeRunes(enterSearch(t, withTitledPRs("Fix cache", "Add docs")), "zzz")
	ent, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	v := ent.(Model).View()
	if !strings.Contains(v, `filter: "zzz" (0/2)`) {
		t.Fatalf("expected badge with 0/2, got:\n%s", v)
	}
	if !strings.Contains(v, "No PRs match") {
		t.Fatalf("expected no-results message with badge, got:\n%s", v)
	}
}

// prNumbers returns the PR numbers currently visible, in display order.
func prNumbers(m Model) []int {
	out := make([]int, len(m.filtered))
	for i, idx := range m.filtered {
		out[i] = m.ctx.PRs[idx].Number
	}
	return out
}

func TestFilterMine(t *testing.T) {
	t.Parallel()
	m := withPRs(0)
	m.ctx.PRs = []ghClient.PullRequest{
		{Number: 1, Title: "a", User: ghClient.User{Login: "octocat"}},
		{Number: 2, Title: "b", User: ghClient.User{Login: "hubot"}},
		{Number: 3, Title: "c", User: ghClient.User{Login: "OctoCat"}},
	}
	m.currentUser = "octocat"
	m.filter = filterMine
	m.recompute()
	if got := prNumbers(m); !slices.Equal(got, []int{1, 3}) {
		t.Fatalf("filtered = %v, want [1 3] (case-insensitive author match)", got)
	}
}

func TestFilterNeedsReview(t *testing.T) {
	t.Parallel()
	m := withPRs(0)
	m.ctx.PRs = []ghClient.PullRequest{
		{Number: 1, Title: "a", RequestedReviewers: []ghClient.User{{Login: "octocat"}}},
		{Number: 2, Title: "b", RequestedReviewers: []ghClient.User{{Login: "hubot"}}},
		{Number: 3, Title: "c"},
	}
	m.currentUser = "octocat"
	m.filter = filterNeedsReview
	m.recompute()
	if got := prNumbers(m); !slices.Equal(got, []int{1}) {
		t.Fatalf("filtered = %v, want [1]", got)
	}
}

func TestFilterDependabot(t *testing.T) {
	t.Parallel()
	m := withPRs(0)
	m.ctx.PRs = []ghClient.PullRequest{
		{Number: 1, Title: "a", User: ghClient.User{Login: "dependabot[bot]"}},
		{Number: 2, Title: "b", User: ghClient.User{Login: "octocat"}},
	}
	m.filter = filterDependabot
	m.recompute()
	if got := prNumbers(m); !slices.Equal(got, []int{1}) {
		t.Fatalf("filtered = %v, want [1]", got)
	}
}

func TestFilterUserDependentEmptyWhenLoginUnresolved(t *testing.T) {
	t.Parallel()
	m := withPRs(0)
	m.ctx.PRs = []ghClient.PullRequest{{Number: 1, User: ghClient.User{Login: "octocat"}}}
	m.currentUser = ""
	m.filter = filterMine
	m.recompute()
	if got := prNumbers(m); len(got) != 0 {
		t.Fatalf("filtered = %v, want empty when login unresolved", got)
	}
}

func TestFilterComposesWithSearch(t *testing.T) {
	t.Parallel()
	m := withPRs(0)
	m.ctx.PRs = []ghClient.PullRequest{
		{Number: 1, Title: "add auth", User: ghClient.User{Login: "octocat"}},
		{Number: 2, Title: "fix auth bug", User: ghClient.User{Login: "octocat"}},
		{Number: 3, Title: "fix auth bug", User: ghClient.User{Login: "hubot"}},
	}
	m.currentUser = "octocat"
	m.filter = filterMine
	m.query = "bug"
	m.recompute()
	if got := prNumbers(m); !slices.Equal(got, []int{2}) {
		t.Fatalf("filtered = %v, want [2] (filter ∩ search)", got)
	}
}

func TestFilterAllUnchanged(t *testing.T) {
	t.Parallel()
	m := withPRs(3)
	m.filter = filterAll
	m.recompute()
	if got := len(m.filtered); got != 3 {
		t.Fatalf("filtered len = %d, want 3", got)
	}
}

func TestCurrentUserMsgStoresLogin(t *testing.T) {
	t.Parallel()
	m := withPRs(2)
	updated, _ := m.Update(currentUserMsg("octocat"))
	if got := updated.(Model).currentUser; got != "octocat" {
		t.Fatalf("currentUser = %q, want octocat", got)
	}
}

func TestCurrentUserMsgReappliesActiveFilter(t *testing.T) {
	t.Parallel()
	m := withPRs(0)
	m.ctx.PRs = []ghClient.PullRequest{
		{Number: 1, Title: "a", User: ghClient.User{Login: "octocat"}},
		{Number: 2, Title: "b", User: ghClient.User{Login: "hubot"}},
	}
	m.filter = filterMine
	m.recompute() // currentUser still "" → nothing matches
	if len(m.filtered) != 0 {
		t.Fatalf("precondition: want 0 filtered before login resolves, got %d", len(m.filtered))
	}
	updated, _ := m.Update(currentUserMsg("octocat"))
	if got := prNumbers(updated.(Model)); !slices.Equal(got, []int{1}) {
		t.Fatalf("filtered = %v, want [1] after login resolves", got)
	}
}

func TestCurrentUserMsgSkipsRecomputeWhenFilterNotUserDependent(t *testing.T) {
	t.Parallel()
	m := withPRs(1)
	m.filter = filterAll
	m.ctx.PRs[0].Body = strings.Repeat("line\n", 200) // tall body so it can scroll
	m.updateViewportContent()
	m.viewport.ScrollDown(50)
	if m.viewport.AtTop() {
		t.Fatal("precondition: expected viewport scrolled away from top")
	}
	updated, _ := m.Update(currentUserMsg("octocat"))
	um := updated.(Model)
	if um.currentUser != "octocat" {
		t.Fatalf("currentUser = %q, want octocat (login still stored)", um.currentUser)
	}
	if um.viewport.AtTop() {
		t.Fatal("filterAll: currentUserMsg must not recompute (viewport scroll should be preserved)")
	}
}

func TestCurrentUserMsgTargetView(t *testing.T) {
	t.Parallel()
	if currentUserMsg("x").TargetView() != screen.ViewPR {
		t.Fatal("currentUserMsg must target the PR view")
	}
}

func TestInitEmitsCurrentUserCmd(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{currentUser: "octocat"}, "octocat", "hello", 100, 40)
	msg := m.Init()()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("Init() msg = %T, want tea.BatchMsg", msg)
	}
	var sawCurrentUser bool
	for _, c := range batch {
		if _, ok := c().(currentUserMsg); ok {
			sawCurrentUser = true
		}
	}
	if !sawCurrentUser {
		t.Fatal("Init did not emit a command resolving the current user")
	}
}

func TestCurrentUserCmdReturnsLogin(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{currentUser: "octocat"}, "octocat", "hello", 100, 40)
	msg := m.currentUserCmd()()
	cu, ok := msg.(currentUserMsg)
	if !ok {
		t.Fatalf("got %T, want currentUserMsg", msg)
	}
	if string(cu) != "octocat" {
		t.Fatalf("login = %q, want octocat", string(cu))
	}
}

func TestCurrentUserCmdEmptyOnError(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{currentUserErr: errors.New("boom")}, "octocat", "hello", 100, 40)
	msg := m.currentUserCmd()()
	cu, ok := msg.(currentUserMsg)
	if !ok {
		t.Fatalf("got %T, want currentUserMsg", msg)
	}
	if string(cu) != "" {
		t.Fatalf("login = %q, want empty on error", string(cu))
	}
}

// withWideFilteredPRs builds a loaded, very wide PR screen (so badges/messages
// are not truncated in the narrow left pane) with the given PRs and current user,
// then recomputes.
func withWideFilteredPRs(currentUser string, prs ...ghClient.PullRequest) Model {
	m := New(fakeBackend{}, "octocat", "hello", 300, 40)
	m.loading = false
	m.currentUser = currentUser
	m.ctx.PRs = prs
	m.recompute()
	return m
}

func TestFilterKeyAppliesAndToggles(t *testing.T) {
	t.Parallel()
	m := withWideFilteredPRs("octocat",
		ghClient.PullRequest{Number: 1, Title: "a", User: ghClient.User{Login: "octocat"}},
		ghClient.PullRequest{Number: 2, Title: "b", User: ghClient.User{Login: "hubot"}},
	)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	um := updated.(Model)
	if um.filter != filterMine {
		t.Fatalf("filter = %v, want filterMine", um.filter)
	}
	if got := prNumbers(um); !slices.Equal(got, []int{1}) {
		t.Fatalf("filtered = %v, want [1]", got)
	}
	updated2, _ := um.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	um2 := updated2.(Model)
	if um2.filter != filterAll {
		t.Fatalf("filter = %v, want filterAll after toggle", um2.filter)
	}
	if len(um2.filtered) != 2 {
		t.Fatalf("filtered len = %d, want 2 after clear", len(um2.filtered))
	}
}

func TestFilterKeySwitchesDirectly(t *testing.T) {
	t.Parallel()
	m := withWideFilteredPRs("octocat",
		ghClient.PullRequest{Number: 1, Title: "a", User: ghClient.User{Login: "dependabot[bot]"}},
		ghClient.PullRequest{Number: 2, Title: "b", User: ghClient.User{Login: "octocat"}},
	)
	m.filter = filterMine
	m.recompute()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	um := updated.(Model)
	if um.filter != filterDependabot {
		t.Fatalf("filter = %v, want filterDependabot", um.filter)
	}
	if got := prNumbers(um); !slices.Equal(got, []int{1}) {
		t.Fatalf("filtered = %v, want [1]", got)
	}
}

func TestFilterKeyResetsCursor(t *testing.T) {
	t.Parallel()
	m := withWideFilteredPRs("octocat",
		ghClient.PullRequest{Number: 1, Title: "a", User: ghClient.User{Login: "octocat"}},
		ghClient.PullRequest{Number: 2, Title: "b", User: ghClient.User{Login: "octocat"}},
		ghClient.PullRequest{Number: 3, Title: "c", User: ghClient.User{Login: "octocat"}},
	)
	m.cursor = 2
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if got := updated.(Model).cursor; got != 0 {
		t.Fatalf("cursor = %d, want 0 after filter change", got)
	}
}

func TestFilterKeyIgnoredWhileLoading(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{}, "octocat", "hello", 100, 40) // loading == true
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if updated.(Model).filter != filterAll {
		t.Fatal("filter must not change while loading")
	}
}

func TestFilterBadgeRendered(t *testing.T) {
	t.Parallel()
	m := withWideFilteredPRs("octocat",
		ghClient.PullRequest{Number: 1, Title: "a", User: ghClient.User{Login: "octocat"}},
		ghClient.PullRequest{Number: 2, Title: "b", User: ghClient.User{Login: "hubot"}},
	)
	m.filter = filterMine
	m.recompute()
	if v := m.View(); !strings.Contains(v, "filter: My PRs (1/2)") {
		t.Fatalf("expected filter badge, got:\n%s", v)
	}
}

func TestFilterAndSearchBadgeRendered(t *testing.T) {
	t.Parallel()
	m := withWideFilteredPRs("octocat",
		ghClient.PullRequest{Number: 1, Title: "add auth", User: ghClient.User{Login: "octocat"}},
		ghClient.PullRequest{Number: 2, Title: "fix bug", User: ghClient.User{Login: "octocat"}},
	)
	m.filter = filterMine
	m.query = "auth"
	m.recompute()
	if v := m.View(); !strings.Contains(v, `filter: My PRs · "auth" (1/2)`) {
		t.Fatalf("expected combined badge, got:\n%s", v)
	}
}

func TestFilterFooterHint(t *testing.T) {
	t.Parallel()
	m := withPRs(2)
	if v := m.View(); !strings.Contains(v, "[m/v/d] Filter") {
		t.Fatalf("expected filter footer hint, got:\n%s", v)
	}
	m.filter = filterMine
	m.recompute()
	if v := m.View(); !strings.Contains(v, "again clears") {
		t.Fatalf("expected clear hint when a filter is active, got:\n%s", v)
	}
}

func TestFilterEmptyStateMessage(t *testing.T) {
	t.Parallel()
	m := withWideFilteredPRs("octocat",
		ghClient.PullRequest{Number: 1, Title: "a", User: ghClient.User{Login: "hubot"}},
	)
	m.filter = filterMine
	m.recompute()
	if v := m.View(); !strings.Contains(v, "No PRs match filter: My PRs") {
		t.Fatalf("expected filter empty-state, got:\n%s", v)
	}
}

func TestFilterEmptyStateUnresolvedLogin(t *testing.T) {
	t.Parallel()
	m := withWideFilteredPRs("",
		ghClient.PullRequest{Number: 1, Title: "a", User: ghClient.User{Login: "hubot"}},
	)
	m.filter = filterNeedsReview
	m.recompute()
	if v := m.View(); !strings.Contains(v, "couldn't determine your GitHub login") {
		t.Fatalf("expected unresolved-login hint, got:\n%s", v)
	}
}

func TestFilterReviewKeyApplies(t *testing.T) {
	t.Parallel()
	m := withWideFilteredPRs("octocat",
		ghClient.PullRequest{Number: 1, Title: "a", RequestedReviewers: []ghClient.User{{Login: "octocat"}}},
		ghClient.PullRequest{Number: 2, Title: "b", User: ghClient.User{Login: "octocat"}},
	)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	um := updated.(Model)
	if um.filter != filterNeedsReview {
		t.Fatalf("filter = %v, want filterNeedsReview", um.filter)
	}
	if got := prNumbers(um); !slices.Equal(got, []int{1}) {
		t.Fatalf("filtered = %v, want [1]", got)
	}
}

func TestFilterKeyIgnoredOnFatalErrorScreen(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{}, "octocat", "hello", 100, 40)
	fe, _ := m.Update(screen.FetchErrMsg{View: screen.ViewPR, Err: errors.New("boom")})
	m = fe.(Model) // fatal error screen: loading cleared, fetchErr set, no PRs
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if updated.(Model).filter != filterAll {
		t.Fatal("filter must not change on the fatal error screen")
	}
}

func TestFilterDependabotBadgeRendered(t *testing.T) {
	t.Parallel()
	m := withWideFilteredPRs("octocat",
		ghClient.PullRequest{Number: 1, Title: "a", User: ghClient.User{Login: "dependabot[bot]"}},
		ghClient.PullRequest{Number: 2, Title: "b", User: ghClient.User{Login: "octocat"}},
	)
	m.filter = filterDependabot
	m.recompute()
	if v := m.View(); !strings.Contains(v, "filter: Dependabot (1/2)") {
		t.Fatalf("expected Dependabot badge, got:\n%s", v)
	}
}

func TestFilterAndSearchEmptyStateUnresolvedLogin(t *testing.T) {
	t.Parallel()
	m := withWideFilteredPRs("",
		ghClient.PullRequest{Number: 1, Title: "add auth", User: ghClient.User{Login: "hubot"}},
	)
	m.filter = filterMine
	m.query = "zzz"
	m.recompute()
	if v := m.View(); !strings.Contains(v, "couldn't determine your GitHub login") {
		t.Fatalf("expected unresolved-login hint in combined filter+query empty state, got:\n%s", v)
	}
}

func TestListPaneKeysDoNotScrollDetailViewport(t *testing.T) {
	t.Parallel()
	tall := strings.Repeat("line\n", 300)
	// Keys the bubbles viewport binds to downward scrolling (d=half-page, j=line,
	// f=page). In the LIST pane these must NOT scroll the detail viewport. PR #1 is
	// authored by dependabot[bot] (and stays first/selected under the cursor) so
	// that pressing 'd' — which also toggles the Dependabot quick filter — still
	// leaves it visible with its tall body; otherwise the filter would empty the
	// viewport and mask the very scroll bug this test targets.
	for _, r := range []rune{'d', 'j', 'f'} {
		m := withWideFilteredPRs("octocat",
			ghClient.PullRequest{Number: 1, Title: "a", Body: tall, User: ghClient.User{Login: "dependabot[bot]"}},
			ghClient.PullRequest{Number: 2, Title: "b", Body: tall, User: ghClient.User{Login: "octocat"}},
		)
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if got := updated.(Model); !got.viewport.AtTop() {
			t.Errorf("key %q in list focus scrolled the detail viewport (YOffset=%d); want top", r, got.viewport.YOffset)
		}
	}
}

func TestDetailPaneStillScrollsWhenFocused(t *testing.T) {
	t.Parallel()
	tall := strings.Repeat("line\n", 300)
	// Authored by dependabot[bot] so toggling the Dependabot filter (also bound to
	// 'd') keeps this PR visible instead of emptying the viewport, letting the
	// scroll this test checks for actually be observable.
	m := withWideFilteredPRs("octocat",
		ghClient.PullRequest{Number: 1, Title: "a", Body: tall, User: ghClient.User{Login: "dependabot[bot]"}},
	)
	m.focus = focusDetails
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}) // half-page-down when details focused
	if got := updated.(Model); got.viewport.AtTop() {
		t.Fatal("detail pane focused: 'd' should scroll the detail viewport")
	}
}

func TestFilterKeysIgnoredWhenDetailFocused(t *testing.T) {
	t.Parallel()
	m := withWideFilteredPRs("octocat",
		ghClient.PullRequest{Number: 1, Title: "a", User: ghClient.User{Login: "dependabot[bot]"}},
		ghClient.PullRequest{Number: 2, Title: "b", User: ghClient.User{Login: "octocat"}},
	)
	m.focus = focusDetails
	// Filters are a list-pane action; in the detail pane these keys drive viewport
	// scrolling (e.g. "d" = half-page-down) and must not change the active filter.
	for _, r := range []rune{'m', 'v', 'd'} {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if got := updated.(Model).filter; got != filterAll {
			t.Fatalf("key %q in detail focus changed filter to %v; want filterAll (filters are list-pane only)", r, got)
		}
	}
}
