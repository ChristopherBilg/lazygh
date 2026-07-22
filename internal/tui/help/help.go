// Package help renders the contextual keybindings overlay and the shared
// key-hint formatting used by every screen's footer, so all on-screen key
// labels reflect the user's configured bindings from a single source.
package help

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"

	"github.com/ChristopherBilg/lazygh/internal/tui/keys"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
)

// Section is a titled group of bindings shown in the overlay.
type Section struct {
	Title    string
	Bindings []key.Binding
}

// Sections returns the bindings relevant to the given screen, grouped for the
// overlay. Only actions that actually work on that screen are listed, so e.g.
// the repo list never shows "checkout".
func Sections(view screen.ViewID) []Section {
	m := keys.Map
	switch view {
	case screen.ViewPR:
		return []Section{
			{"Navigate", []key.Binding{m.Up, m.Down, m.TogglePane, m.PrevTab, m.NextTab, m.NavPRs, m.NavIssues, m.NavActions, m.Back}},
			{"Actions", []key.Binding{m.Search, m.FilterMine, m.FilterReview, m.FilterDependabot, m.Checkout, m.Open, m.Approve, m.Merge, m.Close, m.Refresh}},
			{"General", []key.Binding{m.Help, m.Quit}},
		}
	case screen.ViewIssues, screen.ViewActions:
		return []Section{
			{"Navigate", []key.Binding{m.NavPRs, m.NavIssues, m.NavActions, m.Back}},
			{"General", []key.Binding{m.Help, m.Quit}},
		}
	default: // screen.ViewRepoList
		return []Section{
			{"Navigate", []key.Binding{m.Up, m.Down}},
			{"Actions", []key.Binding{m.Select, m.Refresh}},
			{"General", []key.Binding{m.Help, m.Quit}},
		}
	}
}

// Title returns the overlay heading for a screen.
func Title(view screen.ViewID) string {
	switch view {
	case screen.ViewPR:
		return "Keybindings · Pull Requests"
	case screen.ViewIssues:
		return "Keybindings · Issues"
	case screen.ViewActions:
		return "Keybindings · Actions"
	default:
		return "Keybindings · Repositories"
	}
}

// PrettyKey converts a Bubble Tea key name into a display glyph for the common
// named keys, leaving single characters and unknown names unchanged.
func PrettyKey(k string) string {
	switch k {
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	case "enter":
		return "⏎"
	default:
		return k
	}
}

// KeyHint formats a binding as "[<key>] <description>" from its primary key and
// WithHelp description — the single source of truth for on-screen labels.
func KeyHint(b key.Binding) string {
	h := b.Help()
	return fmt.Sprintf("[%s] %s", PrettyKey(h.Key), h.Desc)
}

// Footer joins the given bindings' hints with "  •  " and a leading space, for a
// screen's shrunken footer bar.
func Footer(bindings ...key.Binding) string {
	hints := make([]string, len(bindings))
	for i, b := range bindings {
		hints[i] = KeyHint(b)
	}
	return " " + strings.Join(hints, "  •  ")
}

// RetryHint is the "press <refresh-key> to retry" fragment shown in fatal and
// refresh-error messages, reflecting the configured refresh key.
func RetryHint() string {
	return fmt.Sprintf("press %s to retry", PrettyKey(keys.Map.Refresh.Help().Key))
}
