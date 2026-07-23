package action

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestViewShowsPlaceholderAndTabs(t *testing.T) {
	m := New(100, 40)
	v := m.View()
	for _, want := range []string{"Actions", "Coming soon", "Pull Requests", "Issues", "[?] help"} {
		if !strings.Contains(v, want) {
			t.Fatalf("view missing %q:\n%s", want, v)
		}
	}
}

func TestUpdateStoresWindowSize(t *testing.T) {
	m := New(10, 10)
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	if cmd != nil {
		t.Fatal("expected no command from placeholder update")
	}
	um := updated.(Model)
	if um.width != 120 || um.height != 50 {
		t.Fatalf("dims = %dx%d, want 120x50", um.width, um.height)
	}
}

func TestInitReturnsNil(t *testing.T) {
	if New(1, 1).Init() != nil {
		t.Fatal("expected placeholder Init to return nil")
	}
}
