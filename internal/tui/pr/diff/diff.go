// Package diff renders a unified diff (as produced by `gh pr diff`) into a
// syntax-highlighted, gutter-marked string for the PR detail viewport. Code
// content is highlighted per file with chroma using the file's detected lexer;
// added/removed/context lines are distinguished by a colored gutter rather than a
// full-line background, which chroma's ANSI resets would break.
//
// Lines are highlighted one at a time, so language constructs that span multiple
// lines (block comments, multi-line strings) may be imperfectly highlighted — an
// accepted v1 limitation.
package diff

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

// lineKind classifies a single unified-diff line.
type lineKind int

const (
	kindContext    lineKind = iota // " "-prefixed or unrecognized
	kindAdded                      // "+" (but not "+++")
	kindRemoved                    // "-" (but not "---")
	kindHunk                       // "@@ ... @@"
	kindFileHeader                 // diff --git / index / --- / +++ / mode / rename / Binary / no-newline
)

// Gutter/header styles. ANSI palette colors adapt to the user's terminal theme.
var (
	addedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))            // green
	removedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))            // red
	hunkStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))            // cyan
	headerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true) // gray
)

// Fixed syntax theme + 256-color terminal formatter for v1 (making the theme
// configurable is a follow-up).
var (
	chromaStyle     = styles.Get("monokai")
	chromaFormatter = formatters.Get("terminal256")
)

// classify returns the kind of a single unified-diff line. Header prefixes are
// tested before the bare "+"/"-" cases so "+++"/"---" are headers, not content.
func classify(line string) lineKind {
	switch {
	case strings.HasPrefix(line, "@@"):
		return kindHunk
	case strings.HasPrefix(line, "+++ "),
		strings.HasPrefix(line, "--- "),
		strings.HasPrefix(line, "diff --git"),
		strings.HasPrefix(line, "index "),
		strings.HasPrefix(line, "old mode"),
		strings.HasPrefix(line, "new mode"),
		strings.HasPrefix(line, "new file mode"),
		strings.HasPrefix(line, "deleted file mode"),
		strings.HasPrefix(line, "rename from"),
		strings.HasPrefix(line, "rename to"),
		strings.HasPrefix(line, "copy from"),
		strings.HasPrefix(line, "copy to"),
		strings.HasPrefix(line, "similarity index"),
		strings.HasPrefix(line, "dissimilarity index"),
		strings.HasPrefix(line, "Binary files"),
		strings.HasPrefix(line, "GIT binary patch"),
		strings.HasPrefix(line, `\ No newline`):
		return kindFileHeader
	case strings.HasPrefix(line, "+"):
		return kindAdded
	case strings.HasPrefix(line, "-"):
		return kindRemoved
	default:
		return kindContext
	}
}

// pathFromHeader extracts the file path from a "--- a/path" or "+++ b/path" line,
// stripping the leading "a/"/"b/". It returns "" for /dev/null and malformed lines.
func pathFromHeader(line string) string {
	fields := strings.SplitN(line, " ", 2)
	if len(fields) < 2 {
		return ""
	}
	p := strings.TrimSpace(fields[1])
	if p == "" || p == "/dev/null" {
		return ""
	}
	if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
		p = p[2:]
	}
	return p
}

// lexerFor returns the chroma lexer for path, falling back to the plaintext lexer
// when the language is unknown.
func lexerFor(path string) chroma.Lexer {
	if path == "" {
		return lexers.Fallback
	}
	if l := lexers.Match(path); l != nil {
		return l
	}
	return lexers.Fallback
}

// highlightCode returns code highlighted with lexer as an ANSI string (trailing
// newline trimmed). On any chroma error — or panic (chroma can panic on
// pathological input, e.g. panic("unknown state ...")) — it returns code
// unchanged, so a diff always renders.
func highlightCode(code string, lexer chroma.Lexer) (result string) {
	if code == "" {
		return ""
	}
	defer func() {
		if recover() != nil {
			result = code
		}
	}()
	it, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}
	var b strings.Builder
	if err := chromaFormatter.Format(&b, chromaStyle, it); err != nil {
		return code
	}
	return strings.TrimRight(b.String(), "\n")
}

// Highlight renders a unified diff into a syntax-highlighted, gutter-marked string
// for the detail viewport. It returns "" for a blank diff.
func Highlight(rawDiff string) string {
	if strings.TrimSpace(rawDiff) == "" {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(rawDiff, "\n"), "\n")
	var (
		b       strings.Builder
		oldPath string
		lexer   = lexers.Fallback
	)
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		switch classify(line) {
		case kindFileHeader:
			switch {
			case strings.HasPrefix(line, "--- "):
				oldPath = pathFromHeader(line)
			case strings.HasPrefix(line, "+++ "):
				p := pathFromHeader(line)
				if p == "" {
					p = oldPath
				}
				lexer = lexerFor(p)
			}
			b.WriteString(headerStyle.Render(line))
		case kindHunk:
			b.WriteString(hunkStyle.Render(line))
		case kindAdded:
			b.WriteString(addedStyle.Render("+") + highlightCode(line[1:], lexer))
		case kindRemoved:
			b.WriteString(removedStyle.Render("-") + highlightCode(line[1:], lexer))
		default: // kindContext
			code := line
			if strings.HasPrefix(line, " ") {
				code = line[1:]
			}
			b.WriteString(" " + highlightCode(code, lexer))
		}
	}
	return b.String()
}
