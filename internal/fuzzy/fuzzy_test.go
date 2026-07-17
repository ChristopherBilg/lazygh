package fuzzy

import (
	"reflect"
	"testing"
)

func TestRankEmptyQueryReturnsAllInOrder(t *testing.T) {
	items := []string{"beta", "alpha", "gamma"}
	if got := Rank("", items); !reflect.DeepEqual(got, []int{0, 1, 2}) {
		t.Fatalf("Rank(\"\") = %v, want [0 1 2]", got)
	}
}

func TestRankFiltersNonMatches(t *testing.T) {
	items := []string{"Fix login bug", "Add dark mode", "Refactor cache"}
	if got := Rank("cache", items); !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("Rank(cache) = %v, want [2]", got)
	}
}

func TestRankIsCaseInsensitive(t *testing.T) {
	items := []string{"Fix RACE condition"}
	if got := Rank("race", items); !reflect.DeepEqual(got, []int{0}) {
		t.Fatalf("Rank(race) = %v, want [0]", got)
	}
}

func TestRankSubstringBeatsSubsequence(t *testing.T) {
	// "abc": scattered subsequence in item 0, contiguous substring in item 1.
	items := []string{"a-b-c-x", "xxabc"}
	if got := Rank("abc", items); !reflect.DeepEqual(got, []int{1, 0}) {
		t.Fatalf("Rank(abc) = %v, want [1 0]", got)
	}
}

func TestRankPrefersWordBoundary(t *testing.T) {
	// Both contain "cache"; item 1 has it at a word boundary (index 0).
	items := []string{"precache warmup", "cache refresh"}
	if got := Rank("cache", items); !reflect.DeepEqual(got, []int{1, 0}) {
		t.Fatalf("Rank(cache) = %v, want [1 0]", got)
	}
}

func TestRankTieBreakByIndex(t *testing.T) {
	items := []string{"cache", "cache"}
	if got := Rank("cache", items); !reflect.DeepEqual(got, []int{0, 1}) {
		t.Fatalf("Rank(cache) = %v, want [0 1] (stable)", got)
	}
}

func TestRankEmptyItems(t *testing.T) {
	if got := Rank("x", nil); len(got) != 0 {
		t.Fatalf("Rank on empty items = %v, want empty", got)
	}
}
