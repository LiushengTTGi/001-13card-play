// Package ai plays the three bot seats.
//
// The policy is deliberately readable: shed cards cheaply, save the strong
// 牌型 for a fight, and always take the win when a play empties the hand.
package ai

import (
	"math/rand"

	"github.com/kuaile13/kuaile13/internal/cards"
	"github.com/kuaile13/kuaile13/internal/combo"
	"github.com/kuaile13/kuaile13/internal/game"
)

// Action is one bot decision.
type Action struct {
	Pass  bool       `json:"pass"`
	Cards cards.Hand `json:"cards,omitempty"`
	Why   string     `json:"why,omitempty"`
}

// Decide answers the current table state for a seat.
func Decide(r *game.Round, seat int, rng *rand.Rand) Action {
	if r.Phase != game.Playing || r.Turn() != seat {
		return Action{Pass: true, Why: "还没轮到"}
	}
	legal := r.LegalPlays(seat)
	if len(legal) == 0 {
		return Action{Pass: true, Why: "管不上"}
	}
	if r.OnTable() == nil {
		return pickLead(r, seat, legal, rng)
	}
	return pickAnswer(r, seat, legal)
}

// pickLead keeps the round moving: empty the hand when possible, otherwise
// dump the least valuable grouping of cards.
func pickLead(r *game.Round, seat int, legal []*combo.Combo, rng *rand.Rand) Action {
	var best *combo.Combo
	for _, c := range legal {
		if best == nil || leadCost(c) < leadCost(best) {
			best = c
		}
	}
	if rng != nil {
		// Spread equal-cost choices so repeated deals of the same shape do not
		// always play out identically.
		tied := make([]*combo.Combo, 0, 4)
		for _, c := range legal {
			if leadCost(c) == leadCost(best) {
				tied = append(tied, c)
			}
		}
		best = tied[rng.Intn(len(tied))]
	}
	return Action{Cards: best.Cards, Why: "先出最省牌的一组"}
}

// leadCost prefers five-card 牌型 over singles and weak 牌型 over strong ones.
func leadCost(c *combo.Combo) int {
	sizePenalty := 0
	if c.Size() == 1 {
		sizePenalty = 1000
	}
	return sizePenalty + c.Kind.Tier()*60 + topOf(c)*3
}

// pickAnswer only spends a 牌型 when it is worth the trick.
func pickAnswer(r *game.Round, seat int, legal []*combo.Combo) Action {
	var best *combo.Combo
	for _, c := range legal {
		if best == nil || answerCost(c) < answerCost(best) {
			best = c
		}
	}
	handSize := len(r.Player(seat).Hand)
	if best.Size() == handSize {
		return Action{Cards: best.Cards, Why: "一把出完"}
	}
	threat := len(r.Player(r.TableSeat()).Hand) <= 3

	budget := 26
	if threat {
		budget += 30
	}
	if handSize <= 6 {
		budget += 24
	}
	if answerCost(best) > budget {
		return Action{Pass: true, Why: "不值得用" + best.Name + "去管"}
	}
	return Action{Cards: best.Cards, Why: "用最小的" + best.Name + "管上"}
}

// answerCost prices a play by ladder tier and pip strength.
func answerCost(c *combo.Combo) int {
	return c.Kind.Tier()*20 + topOf(c)
}

func topOf(c *combo.Combo) int {
	if len(c.Strength) == 0 {
		return 0
	}
	return c.Strength[0]
}
