package repolist

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	ghClient "github.com/ChristopherBilg/lazygh/internal/github"
)

func repo(owner, name string) ghClient.Repository {
	r := ghClient.Repository{Name: name}
	r.Owner.Login = owner
	return r
}

func TestUpdateCursorNavigation(t *testing.T) {
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
			m := Model{repos: repos, cursor: tt.startCur}
			updated, _ := m.Update(tt.key)
			if got := updated.(Model).cursor; got != tt.wantCursor {
				t.Fatalf("cursor = %d, want %d", got, tt.wantCursor)
			}
		})
	}
}

func TestEnterEmitsRepoSelectedMsg(t *testing.T) {
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
	m := Model{}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("expected nil command for empty list")
	}
}

func TestReposMsgPopulates(t *testing.T) {
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
	m := Model{repos: []ghClient.Repository{repo("o", "a")}}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatal("expected a refresh command, got nil")
	}
	if updated.(Model).loading {
		t.Fatal("refresh must not re-enter the loading state (existing list stays visible)")
	}
}

func TestReposMsgClampsCursorWhenListShrinks(t *testing.T) {
	m := Model{
		repos:  []ghClient.Repository{repo("o", "a"), repo("o", "b"), repo("o", "c")},
		cursor: 2,
	}
	updated, _ := m.Update(reposMsg([]ghClient.Repository{repo("o", "a")}))
	um := updated.(Model)
	if um.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 after list shrank to 1", um.cursor)
	}
	// enter must not panic and must select the valid remaining repo.
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
