// Package combo recognises and orders the 牌型 of 开心十三张.
//
// The game accepts three play sizes: a single card, a 对子, or exactly five
// cards. The five-card 牌型 follow the ladder defined by the rules:
//
//	四带一 > 三带二 > 五张同花顺 > 五张同花 > 五张顺子
//
// A play may only answer a previous play of the same size, so 单张 and 对子 are
// each decided on their own and the ladder never mixes with them.
package combo

import (
	"errors"
	"fmt"

	"github.com/kuaile13/kuaile13/internal/cards"
)

// Kind identifies a 牌型.
type Kind int

const (
	Invalid Kind = iota
	Single
	Pair
	Straight
	Flush
	StraightFlush
	TriplePair
	QuadSingle
)

// Tier is the ladder position used to compare two plays of the same size.
// Higher beats lower; 单张 and 对子 sit at the bottom and are decided by card
// order alone.
func (k Kind) Tier() int {
	switch k {
	case Single:
		return 0
	case Pair:
		return 1
	case Straight:
		return 2
	case Flush:
		return 3
	case StraightFlush:
		return 4
	case TriplePair:
		return 5
	case QuadSingle:
		return 6
	}
	return -1
}

func (k Kind) String() string {
	switch k {
	case Single:
		return "单张"
	case Pair:
		return "对子"
	case Straight:
		return "五张顺子"
	case Flush:
		return "五张同花"
	case StraightFlush:
		return "五张同花顺"
	case TriplePair:
		return "三带二"
	case QuadSingle:
		return "四带一"
	}
	return "非法牌型"
}

// Combo is a recognised play.
type Combo struct {
	Kind  Kind       `json:"kind"`
	Name  string     `json:"name"`
	Cards cards.Hand `json:"cards"`
	// Strength and SuitDeciders are aligned comparison vectors, strongest first.
	// Strength 1 appears only in the wheel A-2-3-4-5, where the Ace plays low.
	Strength     []int `json:"strength"`
	SuitDeciders []int `json:"suitDeciders"`
}

// Size is the number of cards in the play.
func (c *Combo) Size() int { return len(c.Cards) }

// Clone returns an independent copy so callers cannot mutate shared state.
func (c *Combo) Clone() *Combo {
	if c == nil {
		return nil
	}
	out := *c
	out.Cards = append(cards.Hand(nil), c.Cards...)
	out.Strength = append([]int(nil), c.Strength...)
	out.SuitDeciders = append([]int(nil), c.SuitDeciders...)
	return &out
}

func (c *Combo) String() string {
	if c == nil {
		return "-"
	}
	return fmt.Sprintf("%s %s", c.Name, c.Cards)
}

// Compare orders two plays. Plays of a different size are incomparable and
// reported as equal, which callers must reject before allowing a beat.
func Compare(a, b *Combo) int {
	if a == nil || b == nil {
		switch {
		case a == b:
			return 0
		case a == nil:
			return -1
		default:
			return 1
		}
	}
	if a.Size() != b.Size() {
		return 0
	}
	if ta, tb := a.Kind.Tier(), b.Kind.Tier(); ta != tb {
		return ta - tb
	}
	for i := range a.Strength {
		if a.Strength[i] != b.Strength[i] {
			return a.Strength[i] - b.Strength[i]
		}
	}
	for i := range a.SuitDeciders {
		if a.SuitDeciders[i] != b.SuitDeciders[i] {
			return a.SuitDeciders[i] - b.SuitDeciders[i]
		}
	}
	return 0
}

// Beats reports whether a is a legal answer to prev.
func Beats(a, prev *Combo) bool {
	if a == nil || prev == nil {
		return false
	}
	return a.Size() == prev.Size() && Compare(a, prev) > 0
}

// slot pairs one comparison strength with the suit that breaks its tie.
type slot struct {
	strength int
	suit     int
}

func assemble(kind Kind, h cards.Hand, slots []slot) *Combo {
	strength := make([]int, len(slots))
	suits := make([]int, len(slots))
	for i, s := range slots {
		strength[i] = s.strength
		suits[i] = s.suit
	}
	return &Combo{
		Kind:         kind,
		Name:         kind.String(),
		Cards:        append(cards.Hand(nil), h...),
		Strength:     strength,
		SuitDeciders: suits,
	}
}

// topSuit is the strongest suit carrying rank r, used as the tie breaker when
// two players hold the same rank in the same slot.
func topSuit(h cards.Hand, r cards.Rank) int {
	best := -1
	for _, c := range h {
		if c.Rank == r && int(c.Suit) > best {
			best = int(c.Suit)
		}
	}
	return best
}

// Classify recognises a set of cards.
func Classify(h cards.Hand) (*Combo, error) {
	switch len(h) {
	case 0:
		return nil, errors.New("combo/empty: 没有选出牌")
	case 1:
		c := h[0]
		return assemble(Single, h, []slot{{int(c.Rank), int(c.Suit)}}), nil
	case 2:
		return classifyPair(h)
	case 5:
		return classifyFive(h)
	default:
		return nil, fmt.Errorf("combo/size: 一次只能出 1 张、2 张或 5 张, 当前 %d 张", len(h))
	}
}

// classifyPair recognises 对子: two cards of the same rank, compared by rank
// and then by the stronger of its two suits.
func classifyPair(h cards.Hand) (*Combo, error) {
	if h[0].Rank != h[1].Rank {
		return nil, errors.New("combo/unknown: 两张必须是同点数的对子")
	}
	r := h[0].Rank
	return assemble(Pair, h, []slot{{int(r), topSuit(h, r)}}), nil
}

func classifyFive(h cards.Hand) (*Combo, error) {
	counts := h.CountByRank()

	if len(counts) == 2 {
		return classifyGroup(h, counts)
	}

	ranks := distinctRanksDesc(h, counts)
	suited := sameSuit(h)

	if len(counts) == 5 {
		if top, wheel, ok := straightTop(ranks); ok {
			kind := Straight
			if suited {
				kind = StraightFlush
			}
			slots := make([]slot, 0, 5)
			for i := 0; i < 5; i++ {
				r := cards.Rank(top - i)
				if wheel && r == 1 {
					// A-2-3-4-5: the Ace closes the run as the weakest card.
					slots = append(slots, slot{1, topSuit(h, cards.AceRank)})
					continue
				}
				slots = append(slots, slot{int(r), topSuit(h, r)})
			}
			return assemble(kind, h, slots), nil
		}
	}

	if suited {
		slots := make([]slot, 0, 5)
		for _, r := range ranks {
			slots = append(slots, slot{r, topSuit(h, cards.Rank(r))})
		}
		return assemble(Flush, h, slots), nil
	}

	return nil, errors.New("combo/unknown: 这五张牌构不成牌型")
}

// classifyGroup recognises 四带一 and 三带二, the only two 牌型 with a repeat.
func classifyGroup(h cards.Hand, counts map[cards.Rank]int) (*Combo, error) {
	var head, tail cards.Rank
	for r, n := range counts {
		if n >= 3 {
			head = r
		} else {
			tail = r
		}
	}
	switch counts[head] {
	case 4:
		return assemble(QuadSingle, h, []slot{{int(head), topSuit(h, head)}, {int(tail), topSuit(h, tail)}}), nil
	case 3:
		if counts[tail] != 2 {
			return nil, errors.New("combo/unknown: 三带二需要一对带牌")
		}
		return assemble(TriplePair, h, []slot{{int(head), topSuit(h, head)}, {int(tail), topSuit(h, tail)}}), nil
	}
	return nil, errors.New("combo/unknown: 这五张牌构不成牌型")
}

func sameSuit(h cards.Hand) bool {
	for _, c := range h {
		if c.Suit != h[0].Suit {
			return false
		}
	}
	return true
}

// distinctRanksDesc lists the hand's ranks from high to low, each once.
func distinctRanksDesc(h cards.Hand, counts map[cards.Rank]int) []int {
	ranks := make([]int, 0, len(counts))
	for r := range counts {
		ranks = append(ranks, int(r))
	}
	for i := 0; i < len(ranks); i++ {
		for j := i + 1; j < len(ranks); j++ {
			if ranks[j] > ranks[i] {
				ranks[i], ranks[j] = ranks[j], ranks[i]
			}
		}
	}
	return ranks
}

// straightTop validates a five-card run. The wheel A-2-3-4-5 is the weakest
// run and reports top 5 with wheel true; 10-J-Q-K-A is the strongest.
func straightTop(ranksDesc []int) (top int, wheel bool, ok bool) {
	if len(ranksDesc) != 5 {
		return 0, false, false
	}
	consecutive := true
	for i := 1; i < 5; i++ {
		if ranksDesc[i-1]-ranksDesc[i] != 1 {
			consecutive = false
			break
		}
	}
	if consecutive {
		return ranksDesc[0], false, true
	}
	if ranksDesc[0] == 14 && ranksDesc[1] == 5 && ranksDesc[2] == 4 && ranksDesc[3] == 3 && ranksDesc[4] == 2 {
		return 5, true, true
	}
	return 0, false, false
}
