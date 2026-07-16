// Package keys holds the application key bindings as bubbles/key bindings,
// resolved from user config at startup and defaulted from config.Default().
package keys

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/ChristopherBilg/lazygh/internal/config"
)

// KeyMap is the set of bindings used across the TUI.
type KeyMap struct {
	Quit       key.Binding
	Back       key.Binding
	Up         key.Binding
	Down       key.Binding
	Select     key.Binding
	Refresh    key.Binding
	TogglePane key.Binding
	Checkout   key.Binding
	Open       key.Binding
	NavPRs     key.Binding
	NavIssues  key.Binding
	NavActions key.Binding
}

// Map is the process-wide key map. It starts at defaults so any code or test
// that never calls Configure still binds correctly.
var Map = newKeyMap(config.Default().Keys)

// Configure rebuilds Map from resolved user config. Call once at startup.
func Configure(kc config.KeysConfig) { Map = newKeyMap(kc) }

func newKeyMap(kc config.KeysConfig) KeyMap {
	// b is safe to call with each action's list: Default() supplies a non-empty
	// list for every action and applyKeys only overwrites with a non-empty list,
	// so keys[0] never panics. WithHelp labels feed a future help overlay.
	b := func(keys []string, help string) key.Binding {
		return key.NewBinding(key.WithKeys(keys...), key.WithHelp(keys[0], help))
	}
	return KeyMap{
		Quit:       b(kc.Quit, "quit"),
		Back:       b(kc.Back, "back"),
		Up:         b(kc.Up, "up"),
		Down:       b(kc.Down, "down"),
		Select:     b(kc.Select, "select"),
		Refresh:    b(kc.Refresh, "refresh"),
		TogglePane: b(kc.TogglePane, "toggle pane"),
		Checkout:   b(kc.Checkout, "checkout"),
		Open:       b(kc.Open, "open in browser"),
		NavPRs:     b(kc.NavPRs, "pull requests"),
		NavIssues:  b(kc.NavIssues, "issues"),
		NavActions: b(kc.NavActions, "actions"),
	}
}
