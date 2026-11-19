// Package cards models the 52-card French-suited deck used by 开心十三张.
package cards

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"unicode/utf8"
)

// Rank is the pip value of a card. 2 is the smallest and Ace is the largest.
type Rank int

const (
	MinRank Rank = 2
	MaxRank Rank = 14
	// AceRank is the Ace, which plays as the highest card everywhere except the
	// wheel A-2-3-4-5, where combo gives it the synthetic strength 1.
	AceRank Rank = 14
)

func (r Rank) String() string {
	switch r {
	case 14:
		return "A"
	case 13:
		return "K"
	case 12:
		return "Q"
	case 11:
		return "J"
	case 10:
		return "10"
	default:
		return fmt.Sprintf("%d", int(r))
	}
}

// Suit orders the four suits from weakest to strongest: ♦ ♣ ♥ ♠.
// Combined with the rank order it makes ♦2 the weakest card and ♠A the strongest.
type Suit int

const (
	Diamond Suit = iota
	Club
	Heart
	Spade
)

func (s Suit) String() string {
	return [4]string{"♦", "♣", "♥", "♠"}[s]
}

// IsRed reports whether the suit is drawn in red.
func (s Suit) IsRed() bool { return s == Diamond || s == Heart }

// Card is a single playing card.
type Card struct {
	Rank Rank `json:"rank"`
	Suit Suit `json:"suit"`
}

func (c Card) String() string { return c.Suit.String() + c.Rank.String() }

// IsDiamondTwo reports whether this is the card that opens the round.
func (c Card) IsDiamondTwo() bool { return c.Rank == 2 && c.Suit == Diamond }

// Cmp orders two cards by rank first and then by suit.
func Cmp(a, b Card) int {
	if a.Rank != b.Rank {
		return int(a.Rank) - int(b.Rank)
	}
	return int(a.Suit) - int(b.Suit)
}

// Hand is a mutable pile of cards.
type Hand []Card

// Sort orders the hand from weakest to strongest.
func (h Hand) Sort() {
	sort.Slice(h, func(i, j int) bool { return Cmp(h[i], h[j]) < 0 })
}

// Sorted returns a sorted copy of the hand.
func (h Hand) Sorted() Hand {
	out := append(Hand(nil), h...)
	out.Sort()
	return out
}

// String renders the hand as "♦2 ♥7 ...".
func (h Hand) String() string {
	var s string
	for i, c := range h {
		if i > 0 {
			s += " "
		}
		s += c.String()
	}
	return s
}

// Has reports whether every card of want is present in the hand.
func (h Hand) Has(want Hand) bool {
	remaining := append(Hand(nil), h...)
	for _, c := range want {
		found := -1
		for i, r := range remaining {
			if r == c {
				found = i
				break
			}
		}
		if found < 0 {
			return false
		}
		remaining = append(remaining[:found], remaining[found+1:]...)
	}
	return true
}

// Remove returns the hand without the given cards.
func (h Hand) Remove(want Hand) (Hand, error) {
	out := append(Hand(nil), h...)
	for _, c := range want {
		found := -1
		for i, r := range out {
			if r == c {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, errors.New("cards/not-enough: 手中没有 " + c.String())
		}
		out = append(out[:found], out[found+1:]...)
	}
	return out, nil
}

// CountByRank buckets the hand by rank.
func (h Hand) CountByRank() map[Rank]int {
	m := make(map[Rank]int, len(h))
	for _, c := range h {
		m[c.Rank]++
	}
	return m
}

// Parse reads a compact card code, accepting both the letter form "sA" and the
// symbol form "♠A" that String and the game log print.
func Parse(code string) (Card, error) {
	symbol, size := utf8.DecodeRuneInString(code)
	var suit Suit
	switch symbol {
	case 'd', '♦':
		suit = Diamond
	case 'c', '♣':
		suit = Club
	case 'h', '♥':
		suit = Heart
	case 's', '♠':
		suit = Spade
	default:
		return Card{}, fmt.Errorf("cards/parse: 无法识别的花色 %q", code)
	}
	switch label := code[size:]; label {
	case "A":
		return Card{Rank: AceRank, Suit: suit}, nil
	case "K":
		return Card{Rank: 13, Suit: suit}, nil
	case "Q":
		return Card{Rank: 12, Suit: suit}, nil
	case "J":
		return Card{Rank: 11, Suit: suit}, nil
	case "10":
		return Card{Rank: 10, Suit: suit}, nil
	case "2", "3", "4", "5", "6", "7", "8", "9":
		return Card{Rank: Rank(label[0] - '0'), Suit: suit}, nil
	default:
		return Card{}, fmt.Errorf("cards/parse: 无法识别的点数 %q", code)
	}
}

// Must is Parse for known-good literals in tests and fixtures.
func Must(code string) Card {
	c, err := Parse(code)
	if err != nil {
		panic(err)
	}
	return c
}

// HandOf builds a hand from compact codes.
func HandOf(codes ...string) Hand {
	out := make(Hand, 0, len(codes))
	for _, code := range codes {
		for _, part := range strings.Fields(code) {
			out = append(out, Must(part))
		}
	}
	return out
}

// NewDeck returns the 52 cards of the deck, jokers excluded.
func NewDeck() Hand {
	h := make(Hand, 0, 52)
	for s := Diamond; s <= Spade; s++ {
		for r := MinRank; r <= MaxRank; r++ {
			h = append(h, Card{Rank: r, Suit: s})
		}
	}
	return h
}

// Shuffle randomises the deck in place.
func (h Hand) Shuffle(rng *rand.Rand) {
	rng.Shuffle(len(h), func(i, j int) { h[i], h[j] = h[j], h[i] })
}
