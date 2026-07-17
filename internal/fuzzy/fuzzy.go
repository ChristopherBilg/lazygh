// Package fuzzy provides a small, dependency-free fuzzy string matcher and ranker
// used to filter the pull-request list by title. It intentionally has no
// third-party dependencies.
package fuzzy

import (
	"cmp"
	"slices"
	"strings"
)

// Score bands keep the two match kinds disjoint: any substring match outranks any
// pure-subsequence match, regardless of the within-tier bonuses.
const (
	substringBase  = 1_000_000
	boundaryBonus  = 8
	adjacencyBonus = 5
)

// Rank returns the indices of items that fuzzily match query, ordered best match
// first. A match is a case-insensitive subsequence of the item; a contiguous
// substring always outranks a scattered subsequence. Ties are broken by ascending
// original index (stable). An empty query returns every index in original order.
func Rank(query string, items []string) []int {
	if query == "" {
		out := make([]int, len(items))
		for i := range out {
			out[i] = i
		}
		return out
	}

	q := []rune(strings.ToLower(query))

	type scored struct {
		index int
		score int
	}
	var matches []scored
	for i, item := range items {
		if s, ok := score(q, []rune(strings.ToLower(item))); ok {
			matches = append(matches, scored{index: i, score: s})
		}
	}

	// Stable sort by score descending; equal scores keep ascending index order
	// because matches were appended in index order.
	slices.SortStableFunc(matches, func(a, b scored) int {
		return cmp.Compare(b.score, a.score)
	})

	out := make([]int, len(matches))
	for i, m := range matches {
		out[i] = m.index
	}
	return out
}

// score returns a match score for query q against target (both already
// lower-cased rune slices) and whether q matches at all. q must be non-empty.
func score(q, target []rune) (int, bool) {
	// Substring tier: q appears contiguously in target.
	if idx := indexRunes(target, q); idx >= 0 {
		s := substringBase - idx // earlier match ranks higher
		if isBoundary(target, idx) {
			s += boundaryBonus
		}
		return s, true
	}

	// Subsequence tier: greedy left-to-right alignment of every rune of q.
	positions, ok := subsequence(target, q)
	if !ok {
		return 0, false
	}
	s := 0
	for i, pos := range positions {
		if isBoundary(target, pos) {
			s += boundaryBonus
		}
		if i > 0 && pos == positions[i-1]+1 {
			s += adjacencyBonus
		}
	}
	s -= positions[0]                                              // earliness
	s -= (positions[len(positions)-1] - positions[0] + 1) - len(q) // compactness (gap count)
	return s, true
}

// indexRunes returns the first index at which sub appears contiguously in s, or
// -1. sub must be non-empty.
func indexRunes(s, sub []rune) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := range sub {
			if s[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// subsequence greedily matches every rune of q against s left to right, returning
// the matched positions and whether all of q was consumed.
func subsequence(s, q []rune) ([]int, bool) {
	positions := make([]int, 0, len(q))
	qi := 0
	for si := 0; si < len(s) && qi < len(q); si++ {
		if s[si] == q[qi] {
			positions = append(positions, si)
			qi++
		}
	}
	return positions, qi == len(q)
}

// isBoundary reports whether position pos in s begins a "word": index 0, or a rune
// immediately preceded by a separator.
func isBoundary(s []rune, pos int) bool {
	return pos == 0 || isSep(s[pos-1])
}

// isSep reports whether r separates words for boundary scoring.
func isSep(r rune) bool {
	switch r {
	case ' ', '-', '_', '/', '.', ':':
		return true
	default:
		return false
	}
}
