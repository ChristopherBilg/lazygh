package markdown

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMain(m *testing.M) {
	// Silence slog by default so passing runs stay quiet; tests that assert on
	// log output install their own buffer-backed logger.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

// maxLineWidth returns the widest line's display width in s (ANSI-strip first).
func maxLineWidth(s string) int {
	w := 0
	for line := range strings.SplitSeq(s, "\n") {
		if n := ansi.StringWidth(line); n > w {
			w = n
		}
	}
	return w
}

func TestRenderTransformsMarkdown(t *testing.T) {
	out := Render("# Heading\n\n- item one\n- item two\n", 80)
	stripped := ansi.Strip(out)
	for _, want := range []string{"Heading", "item one", "item two"} {
		if !strings.Contains(stripped, want) {
			t.Errorf("rendered output missing %q; got:\n%s", want, stripped)
		}
	}
	if strings.Contains(stripped, "# Heading") {
		t.Errorf("heading marker '#' should be rendered away; got:\n%s", stripped)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI escapes in rendered output; got none:\n%q", out)
	}
}

func TestRenderHighlightsCodeFence(t *testing.T) {
	out := Render("```go\nfunc main() {}\n```\n", 80)
	if s := ansi.Strip(out); !strings.Contains(s, "func main()") {
		t.Errorf("rendered code fence missing code text; got:\n%s", s)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI escapes from code highlighting; got none:\n%q", out)
	}
}

func TestRenderEmptyReturnsBlank(t *testing.T) {
	if out := Render("", 80); strings.TrimSpace(ansi.Strip(out)) != "" {
		t.Errorf("Render(\"\") = %q, want blank", out)
	}
}

func TestRenderTinyWidthReturnsRaw(t *testing.T) {
	const raw = "# Heading"
	out := Render(raw, 0)
	if out != raw {
		t.Errorf("Render(raw, 0) = %q, want raw %q", out, raw)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("tiny-width fallback must not add ANSI; got:\n%q", out)
	}
}

func TestRenderStripsControlBytes(t *testing.T) {
	// Hostile body: an OSC 52 clipboard-write sequence plus a stray CSI.
	out := Render("code\x1b]52;c;ZXZpbA==\x07more\x1b[31m", 80)
	if strings.Contains(out, "\x1b]") {
		t.Errorf("output must not contain an OSC introducer from input:\n%q", out)
	}
	if strings.ContainsRune(out, '\x07') {
		t.Errorf("output must not contain a BEL from input:\n%q", out)
	}
	if s := ansi.Strip(out); !strings.Contains(s, "code") || !strings.Contains(s, "more") {
		t.Errorf("visible text should survive sanitization; got:\n%s", s)
	}
}

func TestRenderReflowsWithWidth(t *testing.T) {
	body := "This is a reasonably long paragraph of prose that must wrap differently " +
		"at a narrow width than it does at a wide width so we can prove reflow works."
	wide := maxLineWidth(ansi.Strip(Render(body, 100)))
	narrow := maxLineWidth(ansi.Strip(Render(body, 40)))
	if narrow >= wide {
		t.Errorf("expected narrower wrap at width 40 (%d) than at width 100 (%d)", narrow, wide)
	}
}

func TestRenderDeepBlockquotesFallBackToRaw(t *testing.T) {
	var buf bytes.Buffer
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))

	// Deeper than the guard threshold: must fall back to raw instantly (no hang).
	body := strings.Repeat("> ", 60) + "deep quote text"
	out := Render(body, 80)
	if !strings.Contains(ansi.Strip(out), "deep quote text") {
		t.Fatalf("deep-blockquote fallback lost the text:\n%s", ansi.Strip(out))
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("deep-blockquote fallback should be raw (no ANSI):\n%q", out)
	}
	if !strings.Contains(buf.String(), "container nesting too deep") {
		t.Fatalf("expected a warning for deep blockquote nesting; got:\n%s", buf.String())
	}
}

func TestRenderShallowBlockquoteStillStyled(t *testing.T) {
	// A normal shallow quote must still render styled (guard must not over-trigger).
	if out := Render("> quoted line\n", 80); !strings.Contains(out, "\x1b[") {
		t.Fatalf("shallow blockquote should render styled (ANSI); got raw:\n%q", out)
	}
}

func TestRenderHandlesUnusualInputQuickly(t *testing.T) {
	// Non-blockquote oddities go through glamour and must not panic or hang.
	in := strings.Repeat("#", 50) + " heading-ish\n\n" + strings.Repeat("*", 100) + "\n"
	if out := Render(in, 60); out == "" {
		t.Error("expected non-empty output for unusual input")
	}
}

func TestMaxContainerDepth(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"plain", "plain text", 0},
		{"one quote", "> x", 1},
		{"two quotes", "> > x", 2},
		{"one list", "- x", 1},
		{"nested list", "- a\n  - b\n    - c\n", 3},
		{"list in quote", "> - x", 2},
	}
	for _, c := range cases {
		if got := maxContainerDepth(c.in); got != c.want {
			t.Errorf("%s: maxContainerDepth(%q) = %d, want %d", c.name, c.in, got, c.want)
		}
	}
}

func TestRenderListNestedBlockquotesFallBackToRaw(t *testing.T) {
	var buf bytes.Buffer
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))

	// Blockquotes nested inside list items — the bypass. Depth 30 > the limit,
	// so this must fall back to raw instantly instead of hanging glamour.
	body := strings.Repeat("- > ", 30) + "deep nested text"
	out := Render(body, 80)
	if !strings.Contains(ansi.Strip(out), "deep nested text") {
		t.Fatalf("list-nested-blockquote fallback lost the text:\n%s", ansi.Strip(out))
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("fallback should be raw (no ANSI):\n%q", out)
	}
	if !strings.Contains(buf.String(), "container nesting too deep") {
		t.Fatalf("expected a warning; got:\n%s", buf.String())
	}
}

func TestRenderTabSeparatedNestedBlockquotesFallBackToRaw(t *testing.T) {
	// The same list+blockquote nesting but with TAB separators must also fall
	// back to raw instead of reaching (and hanging) glamour.
	body := strings.Repeat("-\t>\t", 30) + "deep tab nested"
	out := Render(body, 80)
	if !strings.Contains(ansi.Strip(out), "deep tab nested") {
		t.Fatalf("tab-nested fallback lost the text:\n%s", ansi.Strip(out))
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("tab-nested fallback should be raw (no ANSI):\n%q", out)
	}
}

func TestRenderDefinitionListNestedBlockquotesFallBackToRaw(t *testing.T) {
	// Blockquotes nested via a definition-list ':' container (a goldmark
	// extension glamour enables) must also fall back to raw, not hang.
	body := "term\n: > " + strings.Repeat("- > ", 30) + "deep def nested"
	out := Render(body, 80)
	if !strings.Contains(ansi.Strip(out), "deep def nested") {
		t.Fatalf("def-list-nested fallback lost the text:\n%s", ansi.Strip(out))
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("def-list-nested fallback should be raw (no ANSI):\n%q", out)
	}
}

func TestRenderDeepDefinitionListFallsBackToRaw(t *testing.T) {
	// A PURE definition-list chain (no '>' or '-' at all): each level is a term
	// line followed by a ": " description indented 2 columns deeper than the
	// last, so the description's own text becomes the next level's term. This
	// is the third glamour hang vector — zero '>'/'-' markers, so it isn't
	// caught by anything that only looks for those.
	const depth = 30
	var b strings.Builder
	b.WriteString("t\n")
	for i := range depth {
		b.WriteString(strings.Repeat("  ", i))
		b.WriteString(": ")
		if i == depth-1 {
			b.WriteString("deep def text")
		} else {
			b.WriteString("t")
		}
		b.WriteString("\n")
	}
	body := b.String()

	if got := maxContainerDepth(body); got <= maxContainerNesting {
		t.Fatalf("test input not deep enough: depth=%d, want > %d", got, maxContainerNesting)
	}
	out := Render(body, 80)
	if !strings.Contains(ansi.Strip(out), "deep def text") {
		t.Fatalf("def-list fallback lost the text:\n%s", ansi.Strip(out))
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("def-list fallback should be raw (no ANSI):\n%q", out)
	}
}

func TestRenderDeepListFallsBackToRaw(t *testing.T) {
	// Deeply nested list (zero '>') is the second glamour hang vector — must fall
	// back to raw instead of hanging.
	body := strings.Repeat("- ", 60) + "deep list text"
	out := Render(body, 80)
	if !strings.Contains(ansi.Strip(out), "deep list text") {
		t.Fatalf("deep-list fallback lost the text:\n%s", ansi.Strip(out))
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("deep-list fallback should be raw (no ANSI):\n%q", out)
	}
}

func TestRenderShallowListStillStyled(t *testing.T) {
	// A normally nested list must still render styled (guard must not over-trigger).
	if out := Render("- a\n  - b\n", 80); !strings.Contains(out, "\x1b[") {
		t.Fatalf("shallow nested list should render styled (ANSI); got raw:\n%q", out)
	}
}

func TestConfigureUnknownStyleWarnsAndFallsBack(t *testing.T) {
	var buf bytes.Buffer
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig); Configure(defaultStyle) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))

	Configure("no-such-style")
	if !strings.Contains(buf.String(), "unknown style") {
		t.Fatalf("expected a warning for an unknown style; got:\n%s", buf.String())
	}
	if out := Render("# Heading", 80); !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected working render after fallback to default; got:\n%q", out)
	}
}
