package tabs

import (
	"strings"
	"testing"
)

func TestNextWrapsAround(t *testing.T) {
	t.Parallel()
	tests := []struct{ from, want Tab }{
		{Description, FilesChanged},
		{FilesChanged, Comments},
		{Comments, Description}, // wraps
	}
	for _, tt := range tests {
		if got := tt.from.Next(); got != tt.want {
			t.Errorf("%d.Next() = %d, want %d", tt.from, got, tt.want)
		}
	}
}

func TestPrevWrapsAround(t *testing.T) {
	t.Parallel()
	tests := []struct{ from, want Tab }{
		{Comments, FilesChanged},
		{FilesChanged, Description},
		{Description, Comments}, // wraps
	}
	for _, tt := range tests {
		if got := tt.from.Prev(); got != tt.want {
			t.Errorf("%d.Prev() = %d, want %d", tt.from, got, tt.want)
		}
	}
}

func TestBarContainsAllLabels(t *testing.T) {
	t.Parallel()
	bar := Bar(Description)
	for _, label := range []string{"Description", "Files Changed", "Comments"} {
		if !strings.Contains(bar, label) {
			t.Errorf("Bar() missing label %q:\n%s", label, bar)
		}
	}
}

func TestBarVariesByActiveTab(t *testing.T) {
	t.Parallel()
	// Non-TTY tests strip ANSI, so highlighting shows only as a padding shift;
	// assert each active tab yields a distinct bar rather than checking color codes.
	d, f, c := Bar(Description), Bar(FilesChanged), Bar(Comments)
	if d == f || d == c || f == c {
		t.Fatalf("expected distinct bars per active tab; got description=%q filesChanged=%q comments=%q", d, f, c)
	}
}
