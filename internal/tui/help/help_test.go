package help

import (
	"strings"
	"testing"

	"github.com/ChristopherBilg/lazygh/internal/config"
	"github.com/ChristopherBilg/lazygh/internal/tui/keys"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
)

// descs returns the set of binding descriptions across all sections of a view.
func descs(view screen.ViewID) map[string]bool {
	out := map[string]bool{}
	for _, s := range Sections(view) {
		for _, b := range s.Bindings {
			out[b.Help().Desc] = true
		}
	}
	return out
}

func TestSectionsContextualPR(t *testing.T) {
	got := descs(screen.ViewPR)
	for _, want := range []string{"checkout", "approve pr", "merge pr", "search", "previous tab", "help", "quit"} {
		if !got[want] {
			t.Errorf("PR sections missing %q", want)
		}
	}
}

func TestSectionsRepoListOmitsPRActions(t *testing.T) {
	got := descs(screen.ViewRepoList)
	for _, absent := range []string{"checkout", "approve pr", "search"} {
		if got[absent] {
			t.Errorf("repo-list sections should not include %q", absent)
		}
	}
	for _, want := range []string{"select", "refresh", "help", "quit"} {
		if !got[want] {
			t.Errorf("repo-list sections missing %q", want)
		}
	}
}

func TestEveryActionAppearsInSomeView(t *testing.T) {
	seen := map[string]bool{}
	for _, v := range []screen.ViewID{screen.ViewRepoList, screen.ViewPR, screen.ViewIssues, screen.ViewActions} {
		for d := range descs(v) {
			seen[d] = true
		}
	}
	want := []string{
		"quit", "help", "back", "up", "down", "select", "refresh", "toggle pane",
		"checkout", "open in browser", "search", "pull requests", "issues", "actions",
		"filter: my PRs", "filter: needs my review", "filter: dependabot",
		"previous tab", "next tab", "approve pr", "merge pr", "close pr",
	}
	for _, d := range want {
		if !seen[d] {
			t.Errorf("action %q does not appear in any screen's help sections", d)
		}
	}
}

func TestKeyHintDefault(t *testing.T) {
	if got := KeyHint(keys.Map.Quit); got != "[q] quit" {
		t.Errorf("KeyHint(Quit) = %q, want [q] quit", got)
	}
}

func TestKeyHintReflectsRemap(t *testing.T) {
	t.Cleanup(func() { keys.Configure(config.Default().Keys) })
	kc := config.Default().Keys
	kc.Checkout = []string{"x"}
	keys.Configure(kc)
	if got := KeyHint(keys.Map.Checkout); got != "[x] checkout" {
		t.Errorf("KeyHint(Checkout) after remap = %q, want [x] checkout", got)
	}
}

func TestPrettyKey(t *testing.T) {
	cases := map[string]string{"up": "↑", "down": "↓", "enter": "⏎", "c": "c", "esc": "esc", "/": "/"}
	for in, want := range cases {
		if got := PrettyKey(in); got != want {
			t.Errorf("PrettyKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFooterJoinsHints(t *testing.T) {
	got := Footer(keys.Map.Help, keys.Map.Quit)
	if !strings.Contains(got, "[?] help") || !strings.Contains(got, "[q] quit") {
		t.Errorf("Footer = %q, want both hints", got)
	}
	if !strings.Contains(got, "•") {
		t.Errorf("Footer = %q, want a • separator", got)
	}
}

func TestRetryHintUsesRefreshKey(t *testing.T) {
	if got := RetryHint(); got != "press r to retry" {
		t.Errorf("RetryHint() = %q, want 'press r to retry'", got)
	}
}
