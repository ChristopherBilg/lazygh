// Package styles holds the shared lipgloss styles and small rendering helpers
// used across the TUI screens. Styles are built from config via Configure().
package styles

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/ChristopherBilg/lazygh/internal/config"
)

var (
	Base         lipgloss.Style
	Active       lipgloss.Style
	SelectedItem lipgloss.Style
	Title        lipgloss.Style
	Menu         lipgloss.Style
	Error        lipgloss.Style
)

func init() { Configure(config.Default().Theme) }

// Configure rebuilds the shared styles from the resolved theme. Call once at
// startup, before the TUI renders.
func Configure(t config.ThemeConfig) {
	Base = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(t.Border))
	Active = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(t.Accent))
	SelectedItem = lipgloss.NewStyle().Foreground(lipgloss.Color(t.Selected)).Bold(true)
	Title = lipgloss.NewStyle().Foreground(lipgloss.Color(t.Title)).Background(lipgloss.Color(t.Accent)).Padding(0, 1).Bold(true)
	Menu = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(t.Accent)).Padding(1, 2)
	Error = lipgloss.NewStyle().Foreground(lipgloss.Color(t.Error)).Bold(true)
}

// Truncate shortens s to fit within width columns (ANSI-aware) so a long message
// (e.g. an error string) cannot overflow and corrupt the layout. A non-positive
// width returns s unchanged.
func Truncate(s string, width int) string {
	if width <= 0 {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}
