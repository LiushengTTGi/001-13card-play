package cards

import "testing"

func TestDeckHasFiftyTwoCardsWithoutJokers(t *testing.T) {
	deck := NewDeck()
	if len(deck) != 52 {
		t.Fatalf("deck size = %d, want 52", len(deck))
	}
	seen := map[Card]bool{}
	for _, c := range deck {
		if seen[c] {
			t.Fatalf("duplicate card %v", c)
		}
		seen[c] = true
	}
}

func TestDiamondTwoIsWeakestAndSpadeAceStrongest(t *testing.T) {
	deck := NewDeck()
	deck.Sort()
	gotLow, gotHigh := deck[0], deck[51]
	if want := (Card{Rank: 2, Suit: Diamond}); gotLow != want {
		t.Errorf("weakest = %v, want ♦2", gotLow)
	}
	if want := (Card{Rank: 14, Suit: Spade}); gotHigh != want {
		t.Errorf("strongest = %v, want ♠A", gotHigh)
	}
}

func TestSuitOrderForTies(t *testing.T) {
	fives := HandOf("d5 c5 h5 s5")
	fives.Sort()
	want := "♦5 ♣5 ♥5 ♠5"
	if got := fives.String(); got != want {
		t.Errorf("sorted = %q, want %q", got, want)
	}
}

func TestParseAcceptsLettersAndSymbols(t *testing.T) {
	for _, code := range []string{"sA", "♠A", "h10", "♥10", "d2", "♦2"} {
		c, err := Parse(code)
		if err != nil {
			t.Fatalf("parse %q: %v", code, err)
		}
		if c.Suit == 0 && code != "d2" && code != "♦2" {
			t.Errorf("parse %q lost the suit", code)
		}
	}
	for _, code := range []string{"x9", "s1", "zA", ""} {
		if _, err := Parse(code); err == nil {
			t.Errorf("parse %q should fail", code)
		}
	}
}

func TestRemoveLeavesTheRestBehind(t *testing.T) {
	h := HandOf("d2 h7 sA")
	left, err := h.Remove(HandOf("h7"))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 2 || left.Has(HandOf("h7")) {
		t.Fatalf("left = %v", left)
	}
	if _, err := h.Remove(HandOf("c9")); err == nil {
		t.Fatal("removing a card that is not in the hand should fail")
	}
}
