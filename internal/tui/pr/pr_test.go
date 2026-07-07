package pr

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	ghClient "github.com/ChristopherBilg/lazygh/internal/github"
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
	m.updateViewportContent()
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
