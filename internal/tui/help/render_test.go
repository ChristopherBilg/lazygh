package help

import (
	"strings"
	"testing"

	"github.com/ChristopherBilg/lazygh/internal/config"
	"github.com/ChristopherBilg/lazygh/internal/tui/keys"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
)

func TestRenderShowsTitleAndBindings(t *testing.T) {
	out := Render(screen.ViewPR, 80, 40)
	for _, want := range []string{"Pull Requests", "checkout", "approve pr", "close pr", "quit"} {
		if !strings.Contains(out, want) {
			t.Errorf("PR help render missing %q:\n%s", want, out)
		}
	}
}

func TestRenderContextualRepoList(t *testing.T) {
	out := Render(screen.ViewRepoList, 80, 40)
	if strings.Contains(out, "checkout") {
		t.Errorf("repo-list help must not show 'checkout':\n%s", out)
	}
	if !strings.Contains(out, "refresh") {
		t.Errorf("repo-list help missing 'refresh':\n%s", out)
	}
}

func TestRenderReflectsRemappedKey(t *testing.T) {
	t.Cleanup(func() { keys.Configure(config.Default().Keys) })
	kc := config.Default().Keys
	kc.Checkout = []string{"x"}
	keys.Configure(kc)
	out := Render(screen.ViewPR, 80, 40)
	if !strings.Contains(out, "x") || !strings.Contains(out, "checkout") {
		t.Errorf("expected remapped checkout key in help:\n%s", out)
	}
}

func TestRenderScrollsWhenTooShort(t *testing.T) {
	if out := Render(screen.ViewPR, 80, 3); out == "" {
		t.Fatal("expected non-empty help render even when very short")
	}
}

// TestRenderBindingRowsShareLeftIndent guards the column alignment: two binding
// rows of very different total width must start at the same column. Horizontal
// Center would center each line individually and drift the key column per row;
// left-aligning the block keeps the columns lined up.
func TestRenderBindingRowsShareLeftIndent(t *testing.T) {
	out := Render(screen.ViewPR, 80, 40)
	short := leadingSpaces(t, out, "checkout")
	long := leadingSpaces(t, out, "needs my review")
	if short != long {
		t.Errorf("binding rows misaligned: 'checkout' indent=%d, 'needs my review' indent=%d", short, long)
	}
}

// TestRenderCloseHintReflectsRemappedKeys guards that the close hint is built
// from the configured help/back keys, not a hardcoded literal — otherwise it
// would go stale on the very screen meant to show correct bindings.
func TestRenderCloseHintReflectsRemappedKeys(t *testing.T) {
	t.Cleanup(func() { keys.Configure(config.Default().Keys) })
	kc := config.Default().Keys
	kc.Help = []string{"H"}
	kc.Back = []string{"B"}
	keys.Configure(kc)
	out := Render(screen.ViewPR, 80, 40)
	if !strings.Contains(out, "H / B  close") {
		t.Errorf("close hint should reflect remapped help/back keys:\n%s", out)
	}
}

// leadingSpaces returns the count of leading spaces on the first line of out
// that contains sub.
func leadingSpaces(t *testing.T, out, sub string) int {
	t.Helper()
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, sub) {
			return len(line) - len(strings.TrimLeft(line, " "))
		}
	}
	t.Fatalf("no line containing %q in:\n%s", sub, out)
	return -1
}
