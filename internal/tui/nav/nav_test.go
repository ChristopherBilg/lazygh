package nav

import (
	"strings"
	"testing"
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
