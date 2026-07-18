// Package tabs renders the pull-request detail pane's tab bar (Description,
// Files Changed, Comments) and models tab selection with wrap-around cycling.
package tabs

import (
	"strings"

	"github.com/ChristopherBilg/lazygh/internal/tui/styles"
)

// Tab identifies a tab in the PR detail pane, in display order.
type Tab int

// Tab identifiers for the PR detail pane, in display order.
const (
	Description Tab = iota
	FilesChanged
	Comments
)

// labels are the human-readable tab names, indexed by Tab.
var labels = [...]string{
	Description:  "Description",
	FilesChanged: "Files Changed",
	Comments:     "Comments",
}

// Next returns the following tab, wrapping from the last back to the first.
func (t Tab) Next() Tab { return Tab((int(t) + 1) % len(labels)) }

// Prev returns the preceding tab, wrapping from the first to the last.
func (t Tab) Prev() Tab { return Tab((int(t) + len(labels) - 1) % len(labels)) }

// Bar renders the tab bar with the active tab highlighted via styles.Title,
// mirroring nav.Bar. Sub-tabs are not numbered (digits are the top-level nav).
// The caller truncates the result to the pane width.
func Bar(active Tab) string {
	segments := make([]string, len(labels))
	for i, label := range labels {
		if Tab(i) == active {
			segments[i] = styles.Title.Render(label)
		} else {
			segments[i] = label
		}
	}
	return strings.Join(segments, "   ")
}
