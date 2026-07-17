package pr

import (
	"context"
	"errors"
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

// fakeBackend is a pr.Backend test double. RepoPRs/OpenPRInBrowser succeed
// trivially; CheckoutPR returns checkoutErr so checkout result handling can be
// exercised without spawning `gh` or requiring a local git repository.
type fakeBackend struct {
	checkoutErr error
}

func (fakeBackend) RepoPRs(_ context.Context, owner, name string, _ bool) (ghClient.RepoContext, error) {
	return ghClient.RepoContext{Owner: owner, Name: name}, nil
}

func (f fakeBackend) CheckoutPR(_ context.Context, _, _ string, _ int) error { return f.checkoutErr }

func (fakeBackend) OpenPRInBrowser(_ context.Context, _, _ string, _ int) error { return nil }

// withPRs builds a loaded PR screen with n synthetic pull requests.
func withPRs(n int) Model {
	m := New(fakeBackend{}, "octocat", "hello", 100, 40)
	m.loading = false
	prs := make([]ghClient.PullRequest, n)
	for i := range prs {
		prs[i] = ghClient.PullRequest{Number: i + 1, Title: "Title", State: "open"}
	}
	m.ctx.PRs = prs
	m.updateViewportContent()
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
