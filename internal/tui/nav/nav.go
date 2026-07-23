// Package nav renders the global navigation tab bar shared by the per-repo
// screens (Pull Requests, Issues, Actions).
package nav

import (
	"fmt"
	"strings"

	"github.com/ChristopherBilg/lazygh/internal/tui/keys"
	"github.com/ChristopherBilg/lazygh/internal/tui/styles"
)

// Tab identifies a top-level per-repo view in the navigation bar.
type Tab int

// Tab identifiers for the per-repo navigation bar, in display order.
const (
	TabPRs Tab = iota
	TabIssues
	TabActions
)

// labels are the human-readable names shown in the bar, indexed by Tab.
var labels = [...]string{
	TabPRs:     "Pull Requests",
	TabIssues:  "Issues",
	TabActions: "Actions",
}

// Bar renders the navigation tab bar with the active tab highlighted. Each
// per-repo screen calls Bar with its own Tab; because the router only ever
// renders the active screen, the highlighted tab always matches the visible
// view.
func Bar(active Tab) string {
	segments := make([]string, len(labels))
	for i, label := range labels {
		segment := fmt.Sprintf("%s %s", keyFor(Tab(i)), label)
		if Tab(i) == active {
			segment = styles.Title.Render(segment)
		}
		segments[i] = segment
	}
	return " " + strings.Join(segments, "   ")
}

// keyFor returns the configured key that switches to tab t, so the bar's hints
// track the user's nav bindings instead of a hardcoded ordinal.
func keyFor(t Tab) string {
	switch t {
	case TabIssues:
		return keys.Map.NavIssues.Help().Key
	case TabActions:
		return keys.Map.NavActions.Help().Key
	default: // TabPRs
		return keys.Map.NavPRs.Help().Key
	}
}
