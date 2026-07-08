package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ChristopherBilg/lazygh/internal/tui/repolist"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
)

func TestRepoSelectedSwitchesToPRView(t *testing.T) {
	m := NewModel()
	updated, cmd := m.Update(repolist.RepoSelectedMsg{Owner: "octocat", Name: "hello"})
	rm := updated.(Model)
	if rm.active != viewPR {
		t.Fatal("expected active view to be viewPR after RepoSelectedMsg")
	}
	if rm.perRepo[viewPR] == nil {
		t.Fatal("expected the PR screen to be built after RepoSelectedMsg")
	}
	if cmd == nil {
		t.Fatal("expected an init command (PR fetch) after selection")
	}
}

func TestEscReturnsToRepoList(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(repolist.RepoSelectedMsg{Owner: "o", Name: "n"})
	afterSelect := updated.(Model)
	if afterSelect.active != viewPR {
		t.Fatal("precondition: should be in PR view")
	}
	back, _ := afterSelect.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if back.(Model).active != viewRepoList {
		t.Fatal("expected esc to return to repo list")
	}
}

func TestErrMsgSetsError(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(screen.ErrMsg{Err: errors.New("boom")})
	if err := updated.(Model).err; err == nil || err.Error() != "boom" {
		t.Fatalf("expected err \"boom\", got %v", err)
	}
}

func TestWindowSizeStored(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 123, Height: 45})
	rm := updated.(Model)
	if rm.width != 123 || rm.height != 45 {
		t.Fatalf("dimensions = %dx%d, want 123x45", rm.width, rm.height)
	}
}

func TestQuitOnQ(t *testing.T) {
	m := NewModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected quit command on 'q'")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected command to be tea.Quit")
	}
}

func TestForwardRoutesNonGlobalKeyToRepoList(t *testing.T) {
	m := NewModel()
	// A non-global key (not q/ctrl+c/esc/backspace) must be forwarded to the
	// active screen via forward(); the router itself must not change views.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if updated.(Model).active != viewRepoList {
		t.Fatal("expected to stay on repo list after a non-global key")
	}
}

func TestForwardRoutesToActivePRScreen(t *testing.T) {
	m := NewModel()
	selected, _ := m.Update(repolist.RepoSelectedMsg{Owner: "o", Name: "n"})
	// A non-global key while in PR view is forwarded to the PR screen; the
	// router stays in PR view.
	updated, _ := selected.(Model).Update(tea.KeyMsg{Type: tea.KeyTab})
	if updated.(Model).active != viewPR {
		t.Fatal("expected to stay in PR view after forwarding a non-global key")
	}
}

func TestViewShowsErrorOverlay(t *testing.T) {
	m := NewModel()
	errored, _ := m.Update(screen.ErrMsg{Err: errors.New("boom")})
	if view := errored.(Model).View(); !strings.Contains(view, "Error: boom") {
		t.Fatalf("expected error overlay, got %q", view)
	}
}

func TestViewDelegatesToRepoList(t *testing.T) {
	m := NewModel()
	// No error, repo list active: View delegates to the repo-list screen,
	// which shows its loading text initially.
	if view := m.View(); !strings.Contains(view, "Fetching your repositories") {
		t.Fatalf("expected repo-list view, got %q", view)
	}
}

func TestViewDelegatesToPRScreen(t *testing.T) {
	m := NewModel()
	selected, _ := m.Update(repolist.RepoSelectedMsg{Owner: "o", Name: "hello"})
	// PR view active, no error: View delegates to the PR screen (loading text).
	if view := selected.(Model).View(); !strings.Contains(view, "Fetching PRs for hello") {
		t.Fatalf("expected PR screen view, got %q", view)
	}
}

func TestResizeAfterSelectionReachesPRScreen(t *testing.T) {
	m := NewModel()
	selected, _ := m.Update(repolist.RepoSelectedMsg{Owner: "o", Name: "n"})
	resized, _ := selected.(Model).Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	rm := resized.(Model)
	if rm.width != 120 || rm.height != 50 {
		t.Fatalf("dimensions = %dx%d, want 120x50", rm.width, rm.height)
	}
	if rm.perRepo[viewPR] == nil {
		t.Fatal("expected the PR screen to remain built after resize")
	}
}

func TestNumberKeysSwitchViewsAfterSelection(t *testing.T) {
	m := NewModel()
	sel, _ := m.Update(repolist.RepoSelectedMsg{Owner: "o", Name: "n"})
	cur := sel.(Model)

	tests := []struct {
		key  rune
		want view
	}{
		{'2', viewIssues},
		{'3', viewActions},
		{'1', viewPR},
	}
	for _, tt := range tests {
		updated, _ := cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tt.key}})
		cur = updated.(Model)
		if cur.active != tt.want {
			t.Fatalf("after key %q, active = %d, want %d", tt.key, cur.active, tt.want)
		}
	}
}

func TestNumberKeysIgnoredOnRepoList(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if updated.(Model).active != viewRepoList {
		t.Fatal("expected number keys to be ignored on the repo list")
	}
}

func TestSwitchingViewsDoesNotReinit(t *testing.T) {
	m := NewModel()
	sel, cmd := m.Update(repolist.RepoSelectedMsg{Owner: "o", Name: "n"})
	if cmd == nil {
		t.Fatal("expected an Init command when the per-repo screens are built")
	}
	// Switching to an already-built view must not re-init it (that is what
	// preserves its selection/scroll state).
	_, cmd = sel.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if cmd != nil {
		t.Fatal("expected no Init command when switching between held views")
	}
}

func TestAllPerRepoScreensBuiltOnSelection(t *testing.T) {
	m := NewModel()
	sel, _ := m.Update(repolist.RepoSelectedMsg{Owner: "o", Name: "n"})
	rm := sel.(Model)
	for _, v := range perRepoViews {
		if rm.perRepo[v] == nil {
			t.Fatalf("expected per-repo screen for view %d to be built", v)
		}
	}
}
