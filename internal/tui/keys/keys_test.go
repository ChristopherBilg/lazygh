package keys

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ChristopherBilg/lazygh/internal/config"
)

// runeMsg builds a KeyMsg for a single-rune key like "c" or "x".
func runeMsg(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestDefaultBindings(t *testing.T) {
	if !key.Matches(runeMsg('c'), Map.Checkout) {
		t.Error("default Checkout should match 'c'")
	}
	if !key.Matches(runeMsg('k'), Map.Up) {
		t.Error("default Up should match 'k'")
	}
	if !key.Matches(tea.KeyMsg{Type: tea.KeyEnter}, Map.Select) {
		t.Error("default Select should match enter")
	}
	if !key.Matches(tea.KeyMsg{Type: tea.KeyTab}, Map.TogglePane) {
		t.Error("default TogglePane should match tab")
	}
}

func TestConfigureOverridesCheckout(t *testing.T) {
	t.Cleanup(func() { Configure(config.Default().Keys) })
	kc := config.Default().Keys
	kc.Checkout = []string{"x"}
	Configure(kc)

	if !key.Matches(runeMsg('x'), Map.Checkout) {
		t.Error("after Configure, Checkout should match 'x'")
	}
	if key.Matches(runeMsg('c'), Map.Checkout) {
		t.Error("after remapping to 'x', 'c' should no longer match Checkout")
	}
}
