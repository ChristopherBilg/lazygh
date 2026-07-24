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

// maxBlockquoteNesting caps blockquote nesting before glamour renders it. It is
// compared against maxBlockquoteMarkers (an upper bound on real depth), so a body
// is only rendered when every line has at most this many '>' — keeping glamour's
// worst case well under a second. Far above any realistic description (a few
// levels), so legitimate content is never affected in practice.
const maxBlockquoteNesting = 24

// maxBlockquoteMarkers returns the largest number of '>' characters on any single
// line of md. It is a deliberate UPPER BOUND on blockquote nesting depth: opening
// a blockquote nested D levels deep requires D '>' markers on that line, so this
// can never under-count real nesting — regardless of how the markers are spaced
// (space/tab), interleaved with other container markers (list items, definition
// lists), or otherwise arranged. It may over-count a line that contains literal
// '>' in prose or code, which only causes a safe raw fallback. (Fenced code is
// intentionally NOT special-cased: a fence-detection mismatch could under-count,
// and over-counting is always safe.) glamour v1.0.0's blockquote rendering cost
// grows roughly exponentially with real nesting depth — a hang, not a panic, so
// recover() can't help and there is no timeout — so Render falls back to raw text
// when this exceeds maxBlockquoteNesting.
func maxBlockquoteMarkers(md string) int {
	maxCount := 0
	for line := range strings.SplitSeq(md, "\n") {
		if n := strings.Count(line, ">"); n > maxCount {
			maxCount = n
		}
	}
	return maxCount
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
// an error: on a renderer-build error, a render error, or a panic (glamour/chroma
// can panic on pathological input) it logs and returns the sanitized raw text.
// A width below 1, or Markdown nested deeper than maxBlockquoteNesting (see its
// doc comment for why), skips glamour and returns the sanitized raw text.
//
// mu is held across the whole call, including r.Render: the UI is
// single-threaded in prod, and holding the lock for the full call keeps this
// race-free under go test -race without needing glamour's *TermRenderer to be
// safe for concurrent use.
func Render(md string, width int) (out string) {
	clean := sanitize(md)
	if width < 1 {
		return clean
	}
	if maxBlockquoteMarkers(clean) > maxBlockquoteNesting {
		slog.Warn("markdown: blockquote nesting too deep; showing raw", "depth_limit", maxBlockquoteNesting)
		return clean
	}

	mu.Lock()
	defer mu.Unlock()

	defer func() {
		if rec := recover(); rec != nil {
			slog.Warn("markdown: render panicked; showing raw", "err", rec)
			out = clean
		}
	}()

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
