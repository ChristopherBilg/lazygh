// Package styles holds the shared lipgloss styles and small rendering helpers
// used across the TUI screens.
package styles

import "github.com/charmbracelet/lipgloss"

var (
	Base         = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240"))
	Active       = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62"))
	SelectedItem = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	Title        = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1).Bold(true)
	Menu         = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(1, 2)
	Error        = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
)

// Truncate shortens s to fit within width columns (ANSI-aware) so a long message
// (e.g. an error string) cannot overflow and corrupt the layout. A non-positive
// width returns s unchanged.
func Truncate(s string, width int) string {
	if width <= 0 {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}
