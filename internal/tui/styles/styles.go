// Package styles holds the shared lipgloss styles used across the TUI screens.
package styles

import "github.com/charmbracelet/lipgloss"

var (
	Base         = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240"))
	Active       = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62"))
	SelectedItem = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	Title        = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1).Bold(true)
	Menu         = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(1, 2)
)
