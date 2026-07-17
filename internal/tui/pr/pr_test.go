package pr

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ChristopherBilg/lazygh/internal/config"
	ghClient "github.com/ChristopherBilg/lazygh/internal/github"
	"github.com/ChristopherBilg/lazygh/internal/tui/keys"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
)

// withPRs builds a loaded PR screen with n synthetic pull requests.
func withPRs(n int) Model {
	m := New("octocat", "hello", 100, 40)
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
	m := New("octocat", "hello", 120, 40)
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
	m := New("octocat", "hello", 100, 40)
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
	m := New("octocat", "hello", 100, 40) // loading == true
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
	if prDataMsg(ghClient.RepoContext{}).TargetView() != screen.ViewPR {
		t.Fatal("prDataMsg must target the PR view")
	}
}

func TestPRDataMsgClearsFetchErr(t *testing.T) {
	m := withPRs(1)
	m.fetchErr = errors.New("old")
	ctx := ghClient.RepoContext{Owner: "o", Name: "n", PRs: []ghClient.PullRequest{{Number: 1}}}
	updated, _ := m.Update(prDataMsg(ctx))
	if updated.(Model).fetchErr != nil {
		t.Fatal("expected fetchErr cleared after successful data")
	}
}

func TestFetchErrMsgWithNoDataShowsError(t *testing.T) {
	m := New("octocat", "hello", 100, 40) // loading, no PRs
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
	m := withPRs(1) // not loading, not refreshing
	_, cmd := m.Update(spinner.TickMsg{})
	if cmd != nil {
		t.Fatal("expected no tick command when idle (tick loop should stop)")
	}
}

func TestSpinnerTickContinuesWhileFetching(t *testing.T) {
	m := New("octocat", "hello", 100, 40) // loading == true
	_, cmd := m.Update(spinner.TickMsg{})
	if cmd == nil {
		t.Fatal("expected the tick loop to continue while loading")
	}
}

func TestRefreshingViewShowsSpinnerFooter(t *testing.T) {
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
	m := withPRs(2)
	m.message = "Opened PR #1 in browser"
	v := m.View()
	if !strings.Contains(v, "Opened PR #1 in browser") {
		t.Fatalf("expected footer to show the status message, got:\n%s", v)
	}
}

func TestCheckoutCmdUnavailableMessage(t *testing.T) {
	orig := checkoutPR
	t.Cleanup(func() { checkoutPR = orig })
	checkoutPR = func(owner, name string, prNumber int) error { return ghClient.ErrNotLocalRepo }

	msg := checkoutCmd("octocat", "other", 42)()
	status, ok := msg.(statusMsg)
	if !ok {
		t.Fatalf("expected statusMsg, got %T", msg)
	}
	if got := string(status); !strings.Contains(got, "Checkout unavailable") || !strings.Contains(got, "octocat/other") {
		t.Fatalf("status = %q, want 'Checkout unavailable' mentioning octocat/other", got)
	}
}

func TestCheckoutCmdSuccessMessage(t *testing.T) {
	orig := checkoutPR
	t.Cleanup(func() { checkoutPR = orig })
	checkoutPR = func(owner, name string, prNumber int) error { return nil }

	msg := checkoutCmd("octocat", "hello", 7)()
	status, ok := msg.(statusMsg)
	if !ok {
		t.Fatalf("expected statusMsg, got %T", msg)
	}
	if got := string(status); !strings.Contains(got, "Successfully checked out PR #7") {
		t.Fatalf("status = %q, want success message", got)
	}
}

func TestCheckoutCmdFailureMessage(t *testing.T) {
	orig := checkoutPR
	t.Cleanup(func() { checkoutPR = orig })
	checkoutPR = func(owner, name string, prNumber int) error { return errors.New("boom") }

	msg := checkoutCmd("octocat", "hello", 7)()
	status, ok := msg.(statusMsg)
	if !ok {
		t.Fatalf("expected statusMsg, got %T", msg)
	}
	if got := string(status); !strings.Contains(got, "Checkout failed") || !strings.Contains(got, "boom") {
		t.Fatalf("status = %q, want failure message", got)
	}
}

func TestCheckoutRemapTakesEffect(t *testing.T) {
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

func TestSlashEntersSearch(t *testing.T) {
	m := enterSearch(t, withTitledPRs("Fix cache", "Add docs"))
	if !m.searching {
		t.Fatal("expected searching=true after '/'")
	}
	if !m.CapturingInput() {
		t.Fatal("expected CapturingInput()=true while searching")
	}
}

func TestSlashIgnoredWhenDetailFocused(t *testing.T) {
	m := withTitledPRs("Fix cache", "Add docs")
	m.focus = focusDetails
	if enterSearch(t, m).searching {
		t.Fatal("'/' must not start search when the detail pane is focused")
	}
}

func TestTypingFiltersList(t *testing.T) {
	m := typeRunes(enterSearch(t, withTitledPRs("Fix cache race", "Add dark mode", "Refactor cache")), "cache")
	if m.query != "cache" {
		t.Fatalf("query = %q, want cache", m.query)
	}
	if len(m.filtered) != 2 {
		t.Fatalf("filtered len = %d, want 2", len(m.filtered))
	}
}

func TestArrowsNavigateAndLettersTypeWhileSearching(t *testing.T) {
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

func TestEnterCommitsFilter(t *testing.T) {
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
	m := typeRunes(enterSearch(t, withTitledPRs("Add docs", "Refactor cache", "Fix tests")), "cache")
	pr, ok := m.selectedPR()
	if !ok || pr.Number != 2 {
		t.Fatalf("selectedPR() = %+v ok=%v, want PR #2", pr, ok)
	}
}

func TestCheckoutActsOnFilteredSelection(t *testing.T) {
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
	m := typeRunes(enterSearch(t, withTitledPRs("Add docs", "Fix tests")), "zzz")
	ent, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = ent.(Model)
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}); cmd != nil {
		t.Fatal("expected no checkout command when the filter has no matches")
	}
}

func TestRefreshPreservesFilter(t *testing.T) {
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

func TestUpArrowNavigatesWhileSearching(t *testing.T) {
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

func TestViewSearchingShowsHintsAndQuery(t *testing.T) {
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
	m := typeRunes(enterSearch(t, withTitledPRs("Fix cache", "Add docs")), "zzzzz")
	if v := m.View(); !strings.Contains(v, "No PRs match") {
		t.Fatalf("expected a no-results message, got:\n%s", v)
	}
}

func TestViewFooterHasSearchHint(t *testing.T) {
	if v := withTitledPRs("Fix cache", "Add docs").View(); !strings.Contains(v, "[/] Search") {
		t.Fatalf("expected [/] Search hint in footer, got:\n%s", v)
	}
}
