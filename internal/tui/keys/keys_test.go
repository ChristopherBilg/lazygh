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

func TestDefaultSearchBinding(t *testing.T) {
	if !key.Matches(runeMsg('/'), Map.Search) {
		t.Error("default Search should match '/'")
	}
}

func TestConfigureOverridesSearch(t *testing.T) {
	t.Cleanup(func() { Configure(config.Default().Keys) })
	kc := config.Default().Keys
	kc.Search = []string{"f"}
	Configure(kc)

	if !key.Matches(runeMsg('f'), Map.Search) {
		t.Error("after Configure, Search should match 'f'")
	}
	if key.Matches(runeMsg('/'), Map.Search) {
		t.Error("after remapping to 'f', '/' should no longer match Search")
	}
}

func TestDefaultTabBindings(t *testing.T) {
	if !key.Matches(runeMsg('['), Map.PrevTab) {
		t.Error("default PrevTab should match '['")
	}
	if !key.Matches(runeMsg(']'), Map.NextTab) {
		t.Error("default NextTab should match ']'")
	}
}

func TestConfigureOverridesTabKeys(t *testing.T) {
	t.Cleanup(func() { Configure(config.Default().Keys) })
	kc := config.Default().Keys
	kc.PrevTab = []string{"p"}
	kc.NextTab = []string{"n"}
	Configure(kc)

	if !key.Matches(runeMsg('p'), Map.PrevTab) {
		t.Error("after Configure, PrevTab should match 'p'")
	}
	if !key.Matches(runeMsg('n'), Map.NextTab) {
		t.Error("after Configure, NextTab should match 'n'")
	}
	if key.Matches(runeMsg('['), Map.PrevTab) {
		t.Error("after remap, '[' should no longer match PrevTab")
	}
	if key.Matches(runeMsg(']'), Map.NextTab) {
		t.Error("after remap, ']' should no longer match NextTab")
	}
}
