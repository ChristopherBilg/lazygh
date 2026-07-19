package diff

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestClassify(t *testing.T) {
	t.Parallel()
	tests := []struct {
		line string
		want lineKind
	}{
		{"@@ -1,2 +1,3 @@", kindHunk},
		{"diff --git a/x b/x", kindFileHeader},
		{"index abc..def 100644", kindFileHeader},
		{"--- a/x", kindFileHeader},
		{"+++ b/x", kindFileHeader},
		{"Binary files a/x and b/x differ", kindFileHeader},
		{`\ No newline at end of file`, kindFileHeader},
		{"+added", kindAdded},
		{"-removed", kindRemoved},
		{" context", kindContext},
		{"", kindContext},
		{"----", kindRemoved},
		{"++++", kindAdded},
		{"---foo", kindRemoved},
		{"+++bar", kindAdded},
	}
	for _, tt := range tests {
		if got := classify(tt.line); got != tt.want {
			t.Errorf("classify(%q) = %d, want %d", tt.line, got, tt.want)
		}
	}
}

func TestPathFromHeader(t *testing.T) {
	t.Parallel()
	tests := []struct{ line, want string }{
		{"+++ b/internal/foo.go", "internal/foo.go"},
		{"--- a/internal/foo.go", "internal/foo.go"},
		{"+++ /dev/null", ""},
		{"---", ""},
	}
	for _, tt := range tests {
		if got := pathFromHeader(tt.line); got != tt.want {
			t.Errorf("pathFromHeader(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestHighlightEmpty(t *testing.T) {
	t.Parallel()
	if got := Highlight("   \n  "); got != "" {
		t.Fatalf("Highlight(blank) = %q, want empty", got)
	}
}

func TestHighlightPreservesStructureAndGutters(t *testing.T) {
	t.Parallel()
	raw := "diff --git a/main.go b/main.go\n" +
		"--- a/main.go\n" +
		"+++ b/main.go\n" +
		"@@ -1 +1,2 @@\n" +
		" package main\n" +
		"+var x = 1\n" +
		"-var y = 2\n"
	out := ansi.Strip(Highlight(raw))
	for _, want := range []string{
		"diff --git a/main.go", "@@ -1 +1,2 @@",
		" package main", "+var x = 1", "-var y = 2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Highlight output missing %q; got:\n%s", want, out)
		}
	}
}

func TestHighlightBinaryDoesNotPanic(t *testing.T) {
	t.Parallel()
	raw := "diff --git a/img.png b/img.png\n" +
		"Binary files a/img.png and b/img.png differ\n"
	if out := ansi.Strip(Highlight(raw)); !strings.Contains(out, "Binary files a/img.png and b/img.png differ") {
		t.Fatalf("binary diff not rendered:\n%s", out)
	}
}

func TestHighlightNoTrailingBlankLine(t *testing.T) {
	t.Parallel()
	// gh pr diff output ends with a trailing newline; it must not become a
	// spurious extra output line.
	raw := "diff --git a/main.go b/main.go\n@@ -1 +1 @@\n-old\n+new\n"
	out := Highlight(raw)
	if got := len(strings.Split(out, "\n")); got != 4 {
		t.Fatalf("Highlight produced %d lines, want 4 (no trailing blank line): %q", got, strings.Split(out, "\n"))
	}
}

func TestHighlightEmitsANSIOnContent(t *testing.T) {
	t.Parallel()
	raw := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -0,0 +1 @@\n+func main() {}\n"
	if out := Highlight(raw); !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected chroma ANSI escapes in highlighted output, got none:\n%q", out)
	}
}

func TestHighlightDeletionUsesOldPathLexer(t *testing.T) {
	t.Parallel()
	// Deleted .go file: new side is /dev/null, so the lexer must fall back to the
	// old path. The removed Go line should still be highlighted (ANSI present).
	raw := "diff --git a/main.go b/main.go\ndeleted file mode 100644\n--- a/main.go\n+++ /dev/null\n@@ -1 +0,0 @@\n-func main() {}\n"
	if out := Highlight(raw); !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected the deleted Go line to be highlighted via the old-path lexer:\n%q", out)
	}
}
