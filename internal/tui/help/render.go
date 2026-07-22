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

// Render draws the full-screen contextual help page for a view: a title, the
// grouped keybindings (key column aligned to the widest key), and a close hint.
// The block is left-aligned — so the key/description columns stay aligned rather
// than drifting per row — and vertically centered when it fits; it scrolls in a
// viewport when it is taller than the screen.
func Render(view screen.ViewID, width, height int) string {
	width = max(width, 1)
	height = max(height, 1)

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

	content := b.String()

	// Scroll rather than clip when the content is taller than the screen.
	if lipgloss.Height(content) > height {
		vp := viewport.New(width, height)
		vp.SetContent(content)
		return vp.View()
	}
	return lipgloss.Place(width, height, lipgloss.Left, lipgloss.Center, content)
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
