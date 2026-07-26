// Package markdown renders Markdown to styled terminal text for the TUI using
// charmbracelet/glamour. It follows the app's global-Configure convention (see
// internal/tui/styles): Configure sets the active style once at startup and
// Render is called per view. Render never fails the caller — on any error or
// panic it logs and returns the sanitized raw input, so description content can
// never break the TUI.
package markdown

import (
	"log/slog"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/muesli/termenv"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// defaultStyle is glamour's built-in dark style — a dark-background palette
// consistent with the diff renderer's dark chroma theme (monokai).
const defaultStyle = "dark"

// knownStyles are the glamour built-in style names accepted for
// theme.markdown_style. "auto" is intentionally excluded — the app does no
// terminal light/dark detection (see design doc). Extend as glamour adds styles.
var knownStyles = map[string]bool{
	"ascii":       true,
	"dark":        true,
	"dracula":     true,
	"light":       true,
	"notty":       true,
	"pink":        true,
	"tokyo-night": true,
}

// maxContainerNesting caps block-container (list, block-quote, definition-list)
// nesting before glamour renders it, since glamour's per-level subtree re-wrap
// makes cost grow ~O(depth^2.5) — a hang, not a panic, so recover() can't help
// and there is no timeout. Bodies nested deeper than this render as raw text.
// Far above any realistic description (a handful of levels), so legitimate
// content is unaffected.
const maxContainerNesting = 24

// mdParser parses with the same extensions glamour enables (see NewTermRenderer),
// so the container nesting measured here matches what glamour will render.
// goldmark is safe for concurrent use.
var mdParser = goldmark.New(
	goldmark.WithExtensions(extension.GFM, extension.DefinitionList),
).Parser()

// maxContainerDepth parses md once and returns the deepest chain of nested
// block-container nodes (block-quote, list, definition-list) — the dimension
// that makes glamour's per-level subtree re-wrap blow up (~O(depth^2.5)).
// goldmark's parse is itself ~O(depth^2) (≈1.5s at depth ~32k, i.e. a ~64KB body
// of nesting markers), and the AST walk is O(nodes); both are bounded in practice
// because PR bodies are capped (~64K chars by GitHub), so this pre-check costs at
// most ~1-2s on a pathological max-size body and microseconds on real ones —
// acceptable and bounded, versus glamour's UNBOUNDED (many-seconds-to-forever)
// render hang that it prevents.
func maxContainerDepth(md string) int {
	doc := mdParser.Parse(text.NewReader([]byte(md)))
	depth, maxDepth := 0, 0
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		switch n.Kind() {
		case ast.KindBlockquote, ast.KindList, extast.KindDefinitionList:
			if entering {
				depth++
				if depth > maxDepth {
					maxDepth = depth
				}
			} else {
				depth--
			}
		}
		return ast.WalkContinue, nil
	})
	return maxDepth
}

type cacheKey struct {
	style string
	width int
}

var (
	mu    sync.Mutex
	style = defaultStyle
	cache = map[cacheKey]*glamour.TermRenderer{}
)

func init() { Configure(defaultStyle) }

// Configure sets the glamour style used by Render (from config.Theme.MarkdownStyle)
// and clears the renderer cache. An empty value uses the default; an unknown
// value logs a warning and falls back to the default, so a bad config value
// never breaks rendering. Call once at startup, before the TUI renders, like
// styles.Configure / keys.Configure.
func Configure(styleName string) {
	mu.Lock()
	defer mu.Unlock()
	switch styleName = strings.TrimSpace(styleName); {
	case styleName == "":
		styleName = defaultStyle
	case !knownStyles[styleName]:
		slog.Warn("markdown: unknown style; using default", "style", styleName, "default", defaultStyle)
		styleName = defaultStyle
	}
	style = styleName
	cache = map[cacheKey]*glamour.TermRenderer{}
}

// Render renders md as styled terminal text wrapped to width. It never returns
// an error: on a renderer-build error, a render error, or a panic — from the
// container-depth parse below, from building the glamour renderer, or from
// glamour/chroma's render itself, any of which can panic on pathological input —
// it logs and returns the sanitized raw text. A width below 1, or Markdown
// nested deeper than maxContainerNesting (see its doc comment for why), skips
// glamour and returns the sanitized raw text.
//
// mu is held across the whole locked section, including r.Render: the UI is
// single-threaded in prod, and holding the lock for the full call keeps this
// race-free under go test -race without needing glamour's *TermRenderer to be
// safe for concurrent use.
func Render(md string, width int) (out string) {
	clean := sanitize(md)
	// Registered first so it covers everything below — the goldmark parse in
	// maxContainerDepth as well as glamour's build/render. On any panic, fall
	// back to the sanitized raw text. On the locked path it runs after
	// mu.Unlock (LIFO), so the lock is always released first.
	defer func() {
		if rec := recover(); rec != nil {
			slog.Warn("markdown: render panicked; showing raw", "err", rec)
			out = clean
		}
	}()

	if width < 1 {
		return clean
	}
	if maxContainerDepth(clean) > maxContainerNesting {
		slog.Warn("markdown: container nesting too deep; showing raw", "depth_limit", maxContainerNesting)
		return clean
	}

	mu.Lock()
	defer mu.Unlock()

	key := cacheKey{style: style, width: width}
	r, ok := cache[key]
	if !ok {
		built, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle(style),
			glamour.WithWordWrap(width),
			glamour.WithColorProfile(termenv.ANSI256),
		)
		if err != nil {
			slog.Warn("markdown: renderer build failed; showing raw", "style", style, "width", width, "err", err)
			return clean
		}
		cache[key] = built
		r = built
	}

	rendered, err := r.Render(clean)
	if err != nil {
		slog.Warn("markdown: render failed; showing raw", "err", err)
		return clean
	}
	return strings.Trim(rendered, "\n")
}

// sanitize strips C0 control bytes (and DEL) from untrusted Markdown, keeping
// only newline and tab, so an attacker-controlled PR body cannot inject CSI/OSC
// escape sequences (e.g. OSC 52 clipboard writes) into the terminal. glamour
// re-adds its own legitimate color escapes afterward. Mirrors the defense in
// internal/tui/pr/diff.sanitize; kept local so this package stands alone.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}
