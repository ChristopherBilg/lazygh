// Package nav renders the global navigation tab bar shared by the per-repo
// screens (Pull Requests, Issues, Actions).
package nav

import (
	"fmt"
	"strings"

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
		segment := fmt.Sprintf("%d %s", i+1, label)
		if Tab(i) == active {
			segment = styles.Title.Render(segment)
		}
		segments[i] = segment
	}
	return " " + strings.Join(segments, "   ")
}
