package help

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/ChristopherBilg/lazygh/internal/tui/keys"
	"github.com/ChristopherBilg/lazygh/internal/tui/screen"
	"github.com/ChristopherBilg/lazygh/internal/tui/styles"
)

// body builds the raw, left-aligned help text for a view: a title, the grouped
// keybindings (key column aligned to the widest key), and a close hint. The key
// and close-hint labels come from the live keys.Map, so remaps are reflected.
func body(view screen.ViewID) string {
	var b strings.Builder
	b.WriteString(styles.Title.Render(Title(view)))
	b.WriteString("\n")

	sections := Sections(view)
	keyWidth := widestKey(sections)
	for _, s := range sections {
		b.WriteString("\n")
		b.WriteString(styles.Title.Render(s.Title))
		b.WriteString("\n")
		for _, bind := range s.Bindings {
			h := bind.Help()
			fmt.Fprintf(&b, "  %-*s  %s\n", keyWidth, PrettyKey(h.Key), h.Desc)
		}
	}
	b.WriteString("\n")
	b.WriteString(styles.Title.Render(fmt.Sprintf("%s / %s  close",
		PrettyKey(keys.Map.Help.Help().Key), PrettyKey(keys.Map.Back.Help().Key))))
	return b.String()
}

// Render draws the full-screen contextual help page for a view at the given
// size. When the content fits, it is left-aligned (so the key/description
// columns stay aligned) and vertically centered; when it is taller than height,
// it is shown in a viewport scrolled by scroll rows (clamped by the viewport)
// so the lower sections remain reachable.
func Render(view screen.ViewID, width, height, scroll int) string {
	width = max(width, 1)
	height = max(height, 1)
	content := body(view)

	if lipgloss.Height(content) > height {
		vp := viewport.New(width, height)
		vp.SetContent(content)
		vp.SetYOffset(scroll)
		return vp.View()
	}
	return lipgloss.Place(width, height, lipgloss.Left, lipgloss.Center, content)
}

// MaxScroll returns the largest useful scroll offset for a view's help page at
// the given height: 0 when the content fits, else the number of lines beyond
// the last screenful.
func MaxScroll(view screen.ViewID, height int) int {
	return max(0, lipgloss.Height(body(view))-max(height, 1))
}

// widestKey returns the display width of the widest pretty key across sections,
// so the description column lines up.
func widestKey(sections []Section) int {
	w := 0
	for _, s := range sections {
		for _, b := range s.Bindings {
			if n := lipgloss.Width(PrettyKey(b.Help().Key)); n > w {
				w = n
			}
		}
	}
	return w
}
