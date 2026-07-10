package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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
