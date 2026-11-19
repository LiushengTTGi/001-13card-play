package combo

import (
	"sort"

	"github.com/kuaile13/kuaile13/internal/cards"
)

// All enumerates every legal play a hand can make: each single card, each
// 对子, and every five-card subset that classifies as a 牌型.
//
// A hand holds at most 13 cards, so the C(13,2) and C(13,5) subset scans stay
// cheap and keep the recogniser as the single source of truth for legality.
func All(h cards.Hand) []*Combo {
	out := make([]*Combo, 0, 16)
	for _, c := range h {
		if combo, err := Classify(cards.Hand{c}); err == nil {
			out = append(out, combo)
		}
	}
	n := len(h)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if combo, err := Classify(cards.Hand{h[i], h[j]}); err == nil {
				out = append(out, combo)
			}
		}
	}
	idx := make([]int, 5)
	for i := range idx {
		idx[i] = i
	}
	var recurse func(start, depth int)
	recurse = func(start, depth int) {
		if depth == 5 {
			pick := make(cards.Hand, 5)
			for i, k := range idx {
				pick[i] = h[k]
			}
			if combo, err := Classify(pick); err == nil {
				out = append(out, combo)
			}
			return
		}
		for i := start; i <= n-5+depth; i++ {
			idx[depth] = i
			recurse(i+1, depth+1)
		}
	}
	if n >= 5 {
		recurse(0, 0)
	}
	Sort(out)
	return out
}

// Answers returns every legal play that beats prev.
func Answers(h cards.Hand, prev *Combo) []*Combo {
	if prev == nil {
		return All(h)
	}
	var out []*Combo
	for _, c := range All(h) {
		if Beats(c, prev) {
			out = append(out, c)
		}
	}
	return out
}

// Sort orders plays weakest to strongest, so the cheapest option comes first.
func Sort(list []*Combo) {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.Size() != b.Size() {
			return a.Size() < b.Size()
		}
		return Compare(a, b) < 0
	})
}

// Weakest returns the lowest-cost legal play, or nil when the hand cannot act.
func Weakest(list []*Combo) *Combo {
	if len(list) == 0 {
		return nil
	}
	sorted := append([]*Combo(nil), list...)
	Sort(sorted)
	return sorted[0]
}
