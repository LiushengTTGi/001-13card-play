package combo

import (
	"strings"
	"testing"

	"github.com/kuaile13/kuaile13/internal/cards"
)

func h(spec ...string) cards.Hand { return cards.HandOf(spec...) }

func kindOf(t *testing.T, spec ...string) *Combo {
	t.Helper()
	c, err := Classify(h(spec...))
	if err != nil {
		t.Fatalf("classify %v: %v", spec, err)
	}
	return c
}

func TestRecognisesEveryPlaySize(t *testing.T) {
	cases := []struct {
		spec string
		want Kind
	}{
		{"sA", Single},
		{"d2", Single},
		{"d2 h2", Pair},
		{"sA hA", Pair},
		{"d3 h4 c5 s6 d7", Straight},
		{"dA h2 s3 c4 d5", Straight},
		{"c2 c3 c4 c6 c8", Flush},
		{"h9 h10 hJ hQ hK", StraightFlush},
		{"d10 cJ hQ sK dA", Straight},
		{"d7 c7 h7 s2 s2", TriplePair},
		{"d9 c9 h9 s9 dK", QuadSingle},
		{"d5 d6 d7 d8 d9", StraightFlush},
		{"dA d2 d3 d4 d5", StraightFlush}, // 同花色的 A-2-3-4-5 升格为同花顺
	}
	for _, tc := range cases {
		got := kindOf(t, strings.Fields(tc.spec)...)
		if got.Kind != tc.want {
			t.Errorf("%s = %s, want %s", tc.spec, got.Kind, tc.want)
		}
	}
}

func TestRejectsIllegalGroups(t *testing.T) {
	cases := [][]string{
		{"d2", "h3"},                         // 两张必须同点数
		{"d3", "h3", "s3"},                   // 三张必须带一对
		{"d2", "h3", "s4"},                   // 三张不算牌型
		{"d2", "h3", "s4", "d5", "c6", "h7"}, // 六张
		{"d2", "h3", "s5", "d7", "c9"},       // 五张但不成顺不成花
		{"d2", "h3", "s4", "d5", "c8"},       // 五张但不连续
		{"d4", "h4", "s4", "c4"},             // 四张不能单出
		{"d2", "h2", "s2", "c3", "c4"},       // 三带二的带牌必须是对子
	}
	for _, spec := range cases {
		if _, err := Classify(h(spec...)); err == nil {
			t.Errorf("%v should not classify as a play", spec)
		}
	}
}

func TestLadderOrdersTheFiveCardPlays(t *testing.T) {
	straight := kindOf(t, "d3", "h4", "c5", "s6", "d7")
	flush := kindOf(t, "c2", "c3", "c5", "c7", "c9")
	straightFlush := kindOf(t, "h5", "h6", "h7", "h8", "h9")
	triplePair := kindOf(t, "d4", "c4", "h4", "sK", "dK")
	quadSingle := kindOf(t, "d6", "c6", "h6", "s6", "d2")

	order := []*Combo{straight, flush, straightFlush, triplePair, quadSingle}
	names := []string{"顺子", "同花", "同花顺", "三带二", "四带一"}
	for i := 0; i < len(order)-1; i++ {
		if Compare(order[i], order[i+1]) >= 0 {
			t.Errorf("%s 应当小于 %s", names[i], names[i+1])
		}
		if !Beats(order[i+1], order[i]) {
			t.Errorf("%s 应当大过 %s", names[i+1], names[i])
		}
	}
	// The ladder is absolute: the weakest 四带一 still beats the strongest 三带二.
	weakQuad := kindOf(t, "d3", "c3", "h3", "s3", "d9")
	strongTriple := kindOf(t, "dA", "cA", "hA", "sK", "dK")
	if !Beats(weakQuad, strongTriple) {
		t.Errorf("四带一 %s 应当大过三带二 %s", weakQuad, strongTriple)
	}
}

func TestSinglesCompareByRankThenSuit(t *testing.T) {
	weakest := kindOf(t, "d2")
	for _, spec := range []string{"c2", "h2", "s2", "d3", "dK", "dA", "hA", "sA"} {
		if !Beats(kindOf(t, spec), weakest) {
			t.Errorf("%s 应当大过 ♦2", spec)
		}
	}
	if !Beats(kindOf(t, "sA"), kindOf(t, "hA")) {
		t.Error("♠A 应当最大")
	}
	if Compare(kindOf(t, "sA"), kindOf(t, "sA")) != 0 {
		t.Error("同一张牌不能大过自己")
	}
}

func TestWheelIsTheWeakestStraight(t *testing.T) {
	wheel := kindOf(t, "sA", "d2", "h3", "c4", "s5")
	run := kindOf(t, "d3", "h4", "c5", "d6", "s7")
	if wheel.Kind != Straight {
		t.Fatalf("A-2-3-4-5 应是顺子，实际 %s", wheel.Kind)
	}
	if !Beats(run, wheel) {
		t.Errorf("%s 应当大过顺子 A-2-3-4-5", run)
	}
	broadway := kindOf(t, "d10", "cJ", "hQ", "sK", "dA")
	if !Beats(broadway, run) {
		t.Errorf("10-J-Q-K-A 应当大过 %s", run)
	}
}

func TestSuitedRunRanksAboveAnUnsuitedOne(t *testing.T) {
	plain := kindOf(t, "d4", "h5", "c6", "s7", "d8")
	suited := kindOf(t, "c4", "c5", "c6", "c7", "c8")
	if !Beats(suited, plain) {
		t.Errorf("%s 应当大过 %s", suited, plain)
	}
}

func TestPlaysOfDifferentSizesNeverCompare(t *testing.T) {
	single := kindOf(t, "sA")
	pair := kindOf(t, "d2", "c2")
	five := kindOf(t, "d2", "d3", "d4", "d5", "d6")
	if Beats(single, five) || Beats(five, single) {
		t.Error("单张管不了五张牌型，五张牌型也不能当单张出")
	}
	if Beats(single, pair) || Beats(pair, single) {
		t.Error("单张管不了对子，对子也不能当单张出")
	}
	if Beats(pair, five) || Beats(five, pair) {
		t.Error("对子管不了五张牌型")
	}
	if Compare(single, five) != 0 || Compare(pair, five) != 0 {
		t.Error("不同张数的牌应当不可比")
	}
}

func TestPairsCompareByRankThenSuit(t *testing.T) {
	weakest := kindOf(t, "d2", "c2")
	for _, spec := range [][2]string{{"h2", "s2"}, {"d3", "c3"}, {"dK", "cK"}, {"dA", "cA"}} {
		if !Beats(kindOf(t, spec[0], spec[1]), weakest) {
			t.Errorf("%s%s 应当大过 ♦2♣2", spec[0], spec[1])
		}
	}
	// Same rank: the stronger suit carried by the pair decides, ♦ ♣ ♥ ♠.
	if !Beats(kindOf(t, "h5", "s5"), kindOf(t, "d5", "c5")) {
		t.Error("♥5♠5 应当大过 ♦5♣5")
	}
	if Beats(kindOf(t, "d5", "c5"), kindOf(t, "h5", "s5")) {
		t.Error("♦5♣5 不该大过 ♥5♠5")
	}
	if Compare(kindOf(t, "dA", "cA"), kindOf(t, "dA", "cA")) != 0 {
		t.Error("同一对牌不能大过自己")
	}
	if Beats(kindOf(t, "sA", "hA"), kindOf(t, "sK", "hK", "dK", "sQ", "dQ")) {
		t.Error("对子管不了三带二")
	}
}

func TestAllEnumeratesOnlyLegalPlays(t *testing.T) {
	hand := h("d2", "c2", "h2", "s2", "dK", "hQ", "s5", "d6", "c7", "h8", "s9", "dJ", "cA")
	list := All(hand)
	for _, c := range list {
		if c.Size() != 1 && c.Size() != 2 && c.Size() != 5 {
			t.Fatalf("非法张数 %d: %v", c.Size(), c)
		}
		if !hand.Has(c.Cards) {
			t.Fatalf("%v 不是这手牌里的牌", c)
		}
	}
	if len(list) < len(hand) {
		t.Fatalf("至少每张单牌要各算一种出法，实际枚举 %d 种", len(list))
	}
	var pairs, quads int
	for _, c := range list {
		switch c.Kind {
		case Pair:
			pairs++
		case QuadSingle:
			quads++
		}
	}
	// 四张 2 能配出 C(4,2)=6 个对子，四条 2 带一张又能配出多种四带一。
	if pairs == 0 {
		t.Error("手牌有两对 2，应当能枚举出对子")
	}
	if quads == 0 {
		t.Error("手牌有四张 2，应当能枚举出四带一")
	}
}

func TestAllIsOrderedFromCheapestToDearest(t *testing.T) {
	list := All(h("d3", "c3", "h3", "s9", "d9", "hK", "sQ", "c7", "d8", "c10", "hJ", "sA", "d5"))
	for i := 1; i < len(list); i++ {
		if list[i].Size() == list[i-1].Size() && Compare(list[i], list[i-1]) < 0 {
			t.Fatalf("枚举顺序错乱: %v 排在 %v 前面", list[i-1], list[i])
		}
		if list[i].Size() < list[i-1].Size() {
			t.Fatalf("张数顺序错乱: %v 在 %v 前面", list[i], list[i-1])
		}
	}
}

func TestAnswersOnlyReturnBeats(t *testing.T) {
	hand := h("d9", "c9", "h9", "sJ", "dJ", "hK", "sQ", "c7", "d3", "c10", "hJ", "sA", "d2")
	prev := kindOf(t, "d5", "c5", "h5", "s8", "d8") // 三带二：5 带 8 对
	answers := Answers(hand, prev)
	if len(answers) == 0 {
		t.Fatal("手上有三张 9，应当能管上三带二 5")
	}
	for _, c := range answers {
		if !Beats(c, prev) {
			t.Errorf("%s 打不过 %s", c, prev)
		}
		if c.Kind != TriplePair {
			t.Errorf("%s 不该管得上三带二", c)
		}
	}
	if got := Answers(hand, kindOf(t, "d6", "c6", "h6", "s6", "cK")); len(got) != 0 {
		// 四带一只会被更大的四带一压，这手牌没有炸弹，应当无解。
		t.Errorf("不该有牌能压过四条 6: %v", got)
	}
}
