package repolist

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	ghClient "github.com/ChristopherBilg/lazygh/internal/github"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
)

// fakeBackend is a repolist.Backend test double; the repo-list tests never
// execute the fetch command, so a trivial success is enough.
type fakeBackend struct{}

func (fakeBackend) Repositories(_ context.Context, _ bool) ([]ghClient.Repository, error) {
	return nil, nil
}

func repo(owner, name string) ghClient.Repository {
	r := ghClient.Repository{Name: name}
	r.Owner.Login = owner
	return r
}

// loaded builds a loaded repo-list screen (spinner initialized via New) with n
// synthetic repositories and a sized window.
func loaded(n int) Model {
	m := New(fakeBackend{})
	m.loading = false
	m.width = 80
	m.height = 24
	repos := make([]ghClient.Repository, n)
	for i := range repos {
		repos[i] = repo("o", fmt.Sprintf("r%d", i))
	}
	m.repos = repos
	return m
}

func TestUpdateCursorNavigation(t *testing.T) {
	t.Parallel()
	repos := []ghClient.Repository{repo("o", "a"), repo("o", "b"), repo("o", "c")}

	tests := []struct {
		name       string
		startCur   int
		key        tea.KeyMsg
		wantCursor int
	}{
		{"up at top stays", 0, tea.KeyMsg{Type: tea.KeyUp}, 0},
		{"down moves", 0, tea.KeyMsg{Type: tea.KeyDown}, 1},
		{"k at top stays", 0, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, 0},
		{"j moves", 1, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, 2},
		{"down at bottom stays", 2, tea.KeyMsg{Type: tea.KeyDown}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := Model{repos: repos, cursor: tt.startCur}
			updated, _ := m.Update(tt.key)
			if got := updated.(Model).cursor; got != tt.wantCursor {
				t.Fatalf("cursor = %d, want %d", got, tt.wantCursor)
			}
		})
	}
}

func TestEnterEmitsRepoSelectedMsg(t *testing.T) {
	t.Parallel()
	repos := []ghClient.Repository{repo("octocat", "hello"), repo("octocat", "world")}
	m := Model{repos: repos, cursor: 1}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	msg := cmd()
	sel, ok := msg.(RepoSelectedMsg)
	if !ok {
		t.Fatalf("expected RepoSelectedMsg, got %T", msg)
	}
	if sel.Owner != "octocat" || sel.Name != "world" {
		t.Fatalf("got %+v, want {octocat world}", sel)
	}
}

func TestEnterEmptyListNoOp(t *testing.T) {
	t.Parallel()
	m := Model{}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("expected nil command for empty list")
	}
}

func TestReposMsgPopulates(t *testing.T) {
	t.Parallel()
	m := Model{loading: true}
	updated, _ := m.Update(reposMsg([]ghClient.Repository{repo("o", "a")}))
	um := updated.(Model)
	if um.loading {
		t.Fatal("expected loading=false after reposMsg")
	}
	if len(um.repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(um.repos))
	}
}

func TestRefreshEmitsForceFetch(t *testing.T) {
	t.Parallel()
	m := Model{repos: []ghClient.Repository{repo("o", "a")}}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatal("expected a refresh command, got nil")
	}
	um := updated.(Model)
	if um.loading {
		t.Fatal("refresh must not re-enter the loading state (existing list stays visible)")
	}
	if !um.refreshing {
		t.Fatal("expected refreshing=true after 'r' with existing repos")
	}
}

func TestReposMsgClampsCursorWhenListShrinks(t *testing.T) {
	t.Parallel()
	m := Model{
		repos:  []ghClient.Repository{repo("o", "a"), repo("o", "b"), repo("o", "c")},
		cursor: 2,
	}
	updated, _ := m.Update(reposMsg([]ghClient.Repository{repo("o", "a")}))
	um := updated.(Model)
	if um.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 after list shrank to 1", um.cursor)
	}
	_, cmd := um.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a selection command after refresh")
	}
	sel, ok := cmd().(RepoSelectedMsg)
	if !ok {
		t.Fatalf("expected RepoSelectedMsg, got %T", cmd())
	}
	if sel.Name != "a" {
		t.Fatalf("selected %q, want a", sel.Name)
	}
}

func TestReposMsgClearsFetchErr(t *testing.T) {
	t.Parallel()
	m := loaded(1)
	m.fetchErr = errors.New("old")
	updated, _ := m.Update(reposMsg([]ghClient.Repository{repo("o", "a")}))
	if updated.(Model).fetchErr != nil {
		t.Fatal("expected fetchErr cleared after successful reposMsg")
	}
}

func TestReposMsgTargetView(t *testing.T) {
	t.Parallel()
	if (reposMsg{}).TargetView() != screen.ViewRepoList {
		t.Fatal("reposMsg must target the repo-list view")
	}
}

func TestFetchErrMsgWithNoDataShowsError(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{})
	m.width = 80
	updated, _ := m.Update(screen.FetchErrMsg{View: screen.ViewRepoList, Err: errors.New("boom")})
	um := updated.(Model)
	if um.loading {
		t.Fatal("expected loading cleared after fetch error")
	}
	v := um.View()
	if !strings.Contains(v, "Failed to load repositories") || !strings.Contains(v, "boom") {
		t.Fatalf("expected error view, got:\n%s", v)
	}
	if !strings.Contains(v, "press r to retry") {
		t.Fatalf("expected retry hint, got:\n%s", v)
	}
}

func TestFetchErrMsgWithDataKeepsListAndFooterError(t *testing.T) {
	t.Parallel()
	m := loaded(2)
	m.refreshing = true
	updated, _ := m.Update(screen.FetchErrMsg{View: screen.ViewRepoList, Err: errors.New("timeout")})
	um := updated.(Model)
	if um.refreshing {
		t.Fatal("expected refreshing cleared after fetch error")
	}
	if len(um.repos) != 2 {
		t.Fatal("stale repos must be retained on refresh error")
	}
	v := um.View()
	if !strings.Contains(v, "Refresh failed") || !strings.Contains(v, "timeout") {
		t.Fatalf("expected footer refresh error, got:\n%s", v)
	}
	if !strings.Contains(v, "Select a Repository") {
		t.Fatalf("expected repo list still rendered, got:\n%s", v)
	}
}

func TestSpinnerTickIgnoredWhenIdle(t *testing.T) {
	t.Parallel()
	m := loaded(1) // not loading, not refreshing
	_, cmd := m.Update(spinner.TickMsg{})
	if cmd != nil {
		t.Fatal("expected no tick command when idle (tick loop should stop)")
	}
}

func TestSpinnerTickContinuesWhileFetching(t *testing.T) {
	t.Parallel()
	m := New(fakeBackend{}) // loading == true, real spinner
	_, cmd := m.Update(spinner.TickMsg{})
	if cmd == nil {
		t.Fatal("expected the tick loop to continue while loading")
	}
}

func TestRefreshingViewShowsSpinnerFooter(t *testing.T) {
	t.Parallel()
	m := loaded(2)
	m.refreshing = true
	v := m.View()
	if !strings.Contains(v, "Refreshing...") {
		t.Fatalf("expected footer to show Refreshing..., got:\n%s", v)
	}
	if !strings.Contains(v, "Select a Repository") {
		t.Fatalf("expected the repo list to stay visible while refreshing, got:\n%s", v)
	}
}

func TestDefaultFooterShowsHints(t *testing.T) {
	t.Parallel()
	m := loaded(2)
	v := m.View()
	if !strings.Contains(v, "refresh") || !strings.Contains(v, "[?] help") {
		t.Fatalf("expected default footer hints, got:\n%s", v)
	}
}

func TestClampTop(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                               string
		top, cursor, total, capacity, want int
	}{
		{"list fits, cursor at top", 0, 0, 5, 12, 0},
		{"cursor within window, no scroll", 3, 5, 50, 12, 3},
		{"cursor above window pulls up", 10, 4, 50, 12, 4},
		{"cursor below window pulls down", 0, 20, 50, 12, 9},
		{"cursor at last pins window to end", 0, 49, 50, 12, 38},
		{"never scrolls past end", 45, 49, 50, 12, 38},
		{"total shorter than capacity clamps to 0", 5, 2, 3, 12, 0},
		{"capacity below 1 treated as 1", 0, 7, 50, 0, 7},
		{"negative top clamped to 0", -3, 0, 50, 12, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := clampTop(tt.top, tt.cursor, tt.total, tt.capacity); got != tt.want {
				t.Fatalf("clampTop(%d,%d,%d,%d) = %d, want %d",
					tt.top, tt.cursor, tt.total, tt.capacity, got, tt.want)
			}
		})
	}
}

func TestCapacityClampsToAtLeastOne(t *testing.T) {
	t.Parallel()
	tests := []struct{ height, want int }{
		{24, 12}, {13, 1}, {12, 1}, {0, 1}, {100, 88},
	}
	for _, tt := range tests {
		if got := (Model{height: tt.height}).capacity(); got != tt.want {
			t.Errorf("capacity(height=%d) = %d, want %d", tt.height, got, tt.want)
		}
	}
}

func TestScrollIndicator(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		top, end, total  int
		wantSub          string
		wantUp, wantDown bool
	}{
		{"at top has down only", 0, 12, 50, "1–12 of 50", false, true},
		{"in middle has both", 10, 22, 50, "11–22 of 50", true, true},
		{"at bottom has up only", 38, 50, 50, "39–50 of 50", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := scrollIndicator(tt.top, tt.end, tt.total)
			if !strings.Contains(got, tt.wantSub) {
				t.Fatalf("scrollIndicator = %q, want substring %q", got, tt.wantSub)
			}
			if up := strings.Contains(got, "↑"); up != tt.wantUp {
				t.Errorf("up arrow = %v, want %v (got %q)", up, tt.wantUp, got)
			}
			if down := strings.Contains(got, "↓"); down != tt.wantDown {
				t.Errorf("down arrow = %v, want %v (got %q)", down, tt.wantDown, got)
			}
		})
	}
}
