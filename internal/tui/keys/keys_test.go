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

func TestDefaultFilterBindings(t *testing.T) {
	if !key.Matches(runeMsg('m'), Map.FilterMine) {
		t.Error("default FilterMine should match 'm'")
	}
	if !key.Matches(runeMsg('v'), Map.FilterReview) {
		t.Error("default FilterReview should match 'v'")
	}
	if !key.Matches(runeMsg('d'), Map.FilterDependabot) {
		t.Error("default FilterDependabot should match 'd'")
	}
}

func TestConfigureOverridesFilterMine(t *testing.T) {
	t.Cleanup(func() { Configure(config.Default().Keys) })
	kc := config.Default().Keys
	kc.FilterMine = []string{"M"}
	Configure(kc)
	if !key.Matches(runeMsg('M'), Map.FilterMine) {
		t.Error("after Configure, FilterMine should match 'M'")
	}
	if key.Matches(runeMsg('m'), Map.FilterMine) {
		t.Error("after remap, 'm' should no longer match FilterMine")
	}
}

func TestDefaultActionBindings(t *testing.T) {
	if !key.Matches(runeMsg('a'), Map.Approve) {
		t.Error("default Approve should match 'a'")
	}
	if !key.Matches(runeMsg('M'), Map.Merge) {
		t.Error("default Merge should match 'M'")
	}
	if !key.Matches(runeMsg('D'), Map.Close) {
		t.Error("default Close should match 'D'")
	}
}

func TestConfigureOverridesMerge(t *testing.T) {
	t.Cleanup(func() { Configure(config.Default().Keys) })
	kc := config.Default().Keys
	kc.Merge = []string{"g"}
	Configure(kc)
	if !key.Matches(runeMsg('g'), Map.Merge) {
		t.Error("after Configure, Merge should match 'g'")
	}
	if key.Matches(runeMsg('M'), Map.Merge) {
		t.Error("after remap, 'M' should no longer match Merge")
	}
}

func TestDefaultHelpBinding(t *testing.T) {
	if !key.Matches(runeMsg('?'), Map.Help) {
		t.Error("default Help should match '?'")
	}
}

func TestConfigureOverridesHelp(t *testing.T) {
	t.Cleanup(func() { Configure(config.Default().Keys) })
	kc := config.Default().Keys
	kc.Help = []string{"H"}
	Configure(kc)
	if !key.Matches(runeMsg('H'), Map.Help) {
		t.Error("after Configure, Help should match 'H'")
	}
	if key.Matches(runeMsg('?'), Map.Help) {
		t.Error("after remap, '?' should no longer match Help")
	}
}
