package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/ChristopherBilg/lazygh/internal/config"
)

func TestTruncateLimitsWidth(t *testing.T) {
	if got := lipgloss.Width(Truncate(strings.Repeat("x", 100), 10)); got != 10 {
		t.Fatalf("Truncate width = %d, want 10", got)
	}
}

func TestTruncateShorterStringUnchanged(t *testing.T) {
	if got := Truncate("hi", 10); got != "hi" {
		t.Fatalf("Truncate(\"hi\", 10) = %q, want \"hi\"", got)
	}
}

func TestTruncateNegativeWidthUnchanged(t *testing.T) {
	if Truncate("hello", -3) != "hello" {
		t.Fatal("expected unchanged string for negative width")
	}
}

func TestTruncateNonPositiveWidthUnchanged(t *testing.T) {
	if Truncate("hello", 0) != "hello" {
		t.Fatal("expected unchanged string for width 0")
	}
}

func TestTruncateEllipsis(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		in    string
		width int
		want  string
	}{
		{"fits unchanged", "hello", 10, "hello"},
		{"truncates with ellipsis", "hello world", 5, "hell…"},
		{"zero width empty", "hello", 0, ""},
		{"negative width empty", "hello", -3, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := TruncateEllipsis(tt.in, tt.width); got != tt.want {
				t.Fatalf("TruncateEllipsis(%q, %d) = %q, want %q", tt.in, tt.width, got, tt.want)
			}
		})
	}
}

func TestConfigureAppliesTheme(t *testing.T) {
	t.Cleanup(func() { Configure(config.Default().Theme) })
	Configure(config.ThemeConfig{
		Accent:   "1",
		Border:   "2",
		Selected: "3",
		Title:    "4",
		Error:    "5",
	})

	cases := []struct {
		name string
		got  lipgloss.TerminalColor
		want lipgloss.TerminalColor
	}{
		{"Base border", Base.GetBorderTopForeground(), lipgloss.Color("2")},
		{"Active border", Active.GetBorderTopForeground(), lipgloss.Color("1")},
		{"Menu border", Menu.GetBorderTopForeground(), lipgloss.Color("1")},
		{"SelectedItem fg", SelectedItem.GetForeground(), lipgloss.Color("3")},
		{"Title fg", Title.GetForeground(), lipgloss.Color("4")},
		{"Title bg", Title.GetBackground(), lipgloss.Color("1")},
		{"Error fg", Error.GetForeground(), lipgloss.Color("5")},
		{"Divider fg", Divider.GetForeground(), lipgloss.Color("2")},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}
