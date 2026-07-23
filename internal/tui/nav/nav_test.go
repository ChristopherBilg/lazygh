package nav

import (
	"strings"
	"testing"

	"github.com/ChristopherBilg/lazygh/internal/config"
	"github.com/ChristopherBilg/lazygh/internal/tui/keys"
)

func TestBarContainsAllTabLabels(t *testing.T) {
	bar := Bar(TabPRs)
	for _, label := range []string{"Pull Requests", "Issues", "Actions"} {
		if !strings.Contains(bar, label) {
			t.Fatalf("bar %q missing label %q", bar, label)
		}
	}
}

func TestBarShowsNumberHints(t *testing.T) {
	bar := Bar(TabPRs)
	for _, want := range []string{"1 Pull Requests", "2 Issues", "3 Actions"} {
		if !strings.Contains(bar, want) {
			t.Fatalf("bar %q missing hint+label %q", bar, want)
		}
	}
}

func TestBarHighlightDependsOnActiveTab(t *testing.T) {
	// The active tab is rendered differently (highlighted via styles.Title,
	// which adds padding even when color is disabled), so changing which tab is
	// active must change the rendered bar.
	prs := Bar(TabPRs)
	issues := Bar(TabIssues)
	actions := Bar(TabActions)
	if prs == issues || prs == actions || issues == actions {
		t.Fatalf("expected distinct bars per active tab; got prs=%q issues=%q actions=%q", prs, issues, actions)
	}
}

func TestBarReflectsRemappedNavKey(t *testing.T) {
	// The tab-bar key hints must track the configured nav bindings, not a
	// hardcoded ordinal — remapping nav_prs should change the bar's PR hint.
	t.Cleanup(func() { keys.Configure(config.Default().Keys) })
	kc := config.Default().Keys
	kc.NavPRs = []string{"p"}
	keys.Configure(kc)

	bar := Bar(TabPRs)
	if !strings.Contains(bar, "p Pull Requests") {
		t.Fatalf("bar should show the remapped nav key, got: %q", bar)
	}
	if strings.Contains(bar, "1 Pull Requests") {
		t.Fatalf("bar should not show the old default nav key after remap, got: %q", bar)
	}
}
