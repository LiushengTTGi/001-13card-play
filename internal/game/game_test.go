package game_test

import (
	"math/rand"
	"testing"

	"github.com/kuaile13/kuaile13/internal/ai"
	"github.com/kuaile13/kuaile13/internal/cards"
	"github.com/kuaile13/kuaile13/internal/game"
)

var names = [4]string{"你", "王五", "张三", "李四"}

func hands(spec ...string) [4]cards.Hand {
	var out [4]cards.Hand
	for i, s := range spec {
		out[i] = cards.HandOf(s)
	}
	return out
}

func newRound(spec ...string) *game.Round {
	return game.NewWithHands(1, 1, names, [4]float64{}, hands(spec...), rand.New(rand.NewSource(7)))
}

func playOut(t *testing.T, r *game.Round, script ...string) {
	t.Helper()
	for _, step := range script {
		seat := int(step[0] - '0')
		if err := r.Play(seat, cards.HandOf(step[2:])); err != nil {
			t.Fatalf("seat %d play %q: %v", seat, step, err)
		}
	}
}

func TestDealHandsOutThirteenEachAndSeatsTheDiamondTwoHolder(t *testing.T) {
	r := game.New(1, 1, names, [4]float64{}, rand.New(rand.NewSource(11)))
	seen := map[cards.Card]bool{}
	for i, p := range r.Players {
		if len(p.Hand) != 13 {
			t.Fatalf("seat %d holds %d cards, want 13", i, len(p.Hand))
		}
		for _, c := range p.Hand {
			if seen[c] {
				t.Fatalf("card %v dealt twice", c)
			}
			seen[c] = true
		}
	}
	if len(seen) != 52 {
		t.Fatalf("dealt %d distinct cards, want 52", len(seen))
	}
	holder := -1
	for _, p := range r.Players {
		if p.Hand.Has(cards.Hand{cards.Must("d2")}) {
			holder = p.Seat
		}
	}
	if r.Leader() != holder || r.Turn() != holder {
		t.Fatalf("leader = %d turn = %d, but ♦2 sits with %d", r.Leader(), r.Turn(), holder)
	}
}

func TestTrickClosesWhenTheOtherThreePass(t *testing.T) {
	// seat 2 opens because it holds ♦2.
	r := newRound("h5 s9 dJ", "c6 sA dQ", "d2 s3 h7", "d8 cK sJ")
	if r.Turn() != 2 || !r.MustLead(2) || r.CanPass(2) {
		t.Fatalf("seat 2 should lead: turn=%d", r.Turn())
	}
	if err := r.Play(2, cards.HandOf("s3")); err != nil {
		t.Fatal(err)
	}
	if r.OnTable() == nil || r.TableSeat() != 2 {
		t.Fatal("the table should carry the ♠3")
	}
	for i, wantSeat := range []int{3, 0, 1} {
		if got := r.Turn(); got != wantSeat {
			t.Fatalf("actor %d: turn = %d, want %d (counter-clockwise)", i, got, wantSeat)
		}
		if !r.CanPass(wantSeat) {
			t.Fatalf("seat %d should be allowed to pass", wantSeat)
		}
		if err := r.Pass(wantSeat); err != nil {
			t.Fatal(err)
		}
	}
	// Everyone else passed, so seat 2 wins the trick and leads again.
	if r.Turn() != 2 || r.OnTable() != nil || !r.MustLead(2) {
		t.Fatalf("trick should return to seat 2: turn=%d table=%v", r.Turn(), r.OnTable())
	}
}

func TestPlayMustAnswerWithTheSameCardCount(t *testing.T) {
	r := newRound("d2 h5", "s6 d6 c6 h6 s3", "c9 h9 s9 dJ cJ", "sA dK cK hK sK")
	if r.Turn() != 0 {
		t.Fatalf("seat 0 holds ♦2 and should lead, turn=%d", r.Turn())
	}
	if err := r.Play(0, cards.HandOf("h5")); err != nil {
		t.Fatal(err)
	}
	if err := r.Play(1, cards.HandOf("d6 c6 h6 s6 s3")); err == nil {
		t.Fatal("五张牌型不该能管单张")
	}
	if err := r.Play(1, cards.HandOf("c6 h6 s3")); err == nil {
		t.Fatal("三张不是合法牌型")
	}
	if err := r.Play(1, cards.HandOf("sA")); err == nil {
		t.Fatal("不该能出别人手上的牌")
	}
	if err := r.Play(1, cards.HandOf("s6")); err != nil {
		t.Fatalf("♠6 应当管得上 ♥5: %v", err)
	}
	if r.Turn() != 2 || r.TableSeat() != 1 {
		t.Fatalf("table should move on: turn=%d owner=%d", r.Turn(), r.TableSeat())
	}
}

func TestIllegalCardSetAndWrongTurnAreRejected(t *testing.T) {
	r := newRound("d2 h5", "c6 sA", "d8 s9", "dK cK")
	if err := r.Play(1, cards.HandOf("c6")); err == nil {
		t.Fatal("seat 1 不该能抢着出牌")
	}
	if err := r.Pass(1); err == nil {
		t.Fatal("seat 1 不该能在自己的回合之外过牌")
	}
	if err := r.Play(0, cards.HandOf("d2 c6")); err == nil {
		t.Fatal("不该能出掉不属于手牌的牌")
	}
	if err := r.Play(0, cards.HandOf("d2 h5")); err == nil {
		t.Fatal("对子不是合法牌型")
	}
	if err := r.Pass(0); err == nil {
		t.Fatal("先出的一方必须出牌，不能过")
	}
}

func TestLastPlayerInAKnockoutOrderPaysTheFullLadder(t *testing.T) {
	// Tiny hands fix the finishing order: seats 1, 2 and 3 each empty their
	// hand on their turn, leaving seat 0 last.
	r := newRound("d2 h5 s9", "c3", "d6", "sJ")
	playOut(t, r, "0 d2", "1 c3", "2 d6", "3 sJ")
	if r.Phase != game.Over {
		t.Fatalf("round should be settled, phase=%s", r.Phase)
	}
	res := r.Result()
	if res == nil {
		t.Fatal("missing result")
	}
	wantPlace := map[int]int{0: 4, 1: 1, 2: 2, 3: 3}
	wantDelta := map[int]float64{0: -3, 1: 4, 2: -0.5, 3: -0.5}
	for _, rank := range res.Ranks {
		if rank.Place != wantPlace[rank.Seat] {
			t.Errorf("seat %d place = %d, want %d", rank.Seat, rank.Place, wantPlace[rank.Seat])
		}
		if rank.Delta != wantDelta[rank.Seat] {
			t.Errorf("seat %d delta = %v, want %v", rank.Seat, rank.Delta, wantDelta[rank.Seat])
		}
	}
}

func TestScoringScalesWithTheStake(t *testing.T) {
	r := game.NewWithHands(1, 3, names, [4]float64{}, hands("d2 h5 s9", "c3", "d6", "sJ"), rand.New(rand.NewSource(3)))
	playOut(t, r, "0 d2", "1 c3", "2 d6", "3 sJ")
	deltas := map[int]float64{}
	for _, rank := range r.Result().Ranks {
		deltas[rank.Seat] = rank.Delta
	}
	want := map[int]float64{0: -9, 1: 12, 2: -1.5, 3: -1.5}
	for seat, w := range want {
		if deltas[seat] != w {
			t.Errorf("3 倍下注 seat %d delta = %v, want %v", seat, deltas[seat], w)
		}
	}
}

func TestSeatsStartWithTheBankrollAndItStaysZeroSum(t *testing.T) {
	// Every seat holds the starting bankroll before the first round is settled.
	r := newRound("d2 h5 s9", "c3", "d6", "sJ")
	for _, p := range r.Players {
		if p.Points() != game.StartPoints {
			t.Fatalf("seat %d points = %v, want the starting bankroll %d",
				p.Seat, p.Points(), game.StartPoints)
		}
	}
	playOut(t, r, "0 d2", "1 c3", "2 d6", "3 sJ")
	// A point is one bet: the winner collects 4 bets = 4 points, second and third
	// pay half a bet = 0.5 each, last place pays 3 bets = 3 points.
	want := map[int]float64{0: 997, 1: 1004, 2: 999.5, 3: 999.5}
	sum := 0.0
	for _, rank := range r.Result().Ranks {
		if rank.Points != want[rank.Seat] {
			t.Errorf("seat %d points = %v, want %v", rank.Seat, rank.Points, want[rank.Seat])
		}
		sum += rank.Points
	}
	if sum != 4*game.StartPoints {
		t.Errorf("seats hold %v points in total, want the bankroll back at %d",
			sum, 4*game.StartPoints)
	}

	// The same ranking at 3x scales by the stake and still sums to 4000.
	r3 := game.NewWithHands(1, 3, names, [4]float64{},
		hands("d2 h5 s9", "c3", "d6", "sJ"), rand.New(rand.NewSource(3)))
	playOut(t, r3, "0 d2", "1 c3", "2 d6", "3 sJ")
	if got := r3.Players[1].Points(); got != game.StartPoints+12 {
		t.Errorf("3x winner points = %v, want %v", got, game.StartPoints+12)
	}
	if got := r3.Players[2].Points(); got != game.StartPoints-1.5 {
		t.Errorf("3x second place points = %v, want %v", got, game.StartPoints-1.5)
	}
	sum = 0.0
	for _, rank := range r3.Result().Ranks {
		sum += rank.Points
	}
	if sum != 4*game.StartPoints {
		t.Errorf("3x: seats hold %v points, want %d", sum, 4*game.StartPoints)
	}
}

func TestRunningScoreCarriesIntoTheNextRound(t *testing.T) {
	first := newRound("d2 h5 s9", "c3", "d6", "sJ")
	playOut(t, first, "0 d2", "1 c3", "2 d6", "3 sJ")
	totals := [4]float64{}
	for _, p := range first.Players {
		totals[p.Seat] = p.Total
	}
	second := game.NewWithHands(2, 1, names, totals, hands("d2 h5 s9", "c3", "d6", "sJ"), rand.New(rand.NewSource(5)))
	for _, p := range second.Players {
		if p.Total != totals[p.Seat] {
			t.Fatalf("seat %d total = %v, want the banked %v", p.Seat, p.Total, totals[p.Seat])
		}
	}
}

func TestEventsFeedRecordsEveryAction(t *testing.T) {
	r := newRound("d2 h5", "c6 sA", "d8 s9", "dK cK")
	playOut(t, r, "0 d2")
	events := r.Events()
	if len(events) < 2 {
		t.Fatalf("expected a deal plus a play, got %d events", len(events))
	}
	if events[0].Seq != 1 || events[0].Type != "deal" {
		t.Fatalf("first event = %+v", events[0])
	}
	if last := events[len(events)-1]; last.Type != "play" || last.Seat != 0 || len(last.Cards) != 1 {
		t.Fatalf("last event = %+v", last)
	}
	if got := r.EventsSince(events[0].Seq); len(got) != 1 {
		t.Fatalf("EventsSince returned %d events, want 1", len(got))
	}
}

// TestPlaysOutRealDeals drives complete rounds with the bot policy on all four
// seats: the end-to-end check that the engine always terminates and that the
// AI never proposes a play the engine rejects.
func TestPlaysOutRealDeals(t *testing.T) {
	seeds := int64(60)
	if testing.Short() {
		seeds = 12
	}
	for seed := int64(0); seed < seeds; seed++ {
		rng := rand.New(rand.NewSource(seed))
		r := game.New(1, 1, names, [4]float64{}, rng)
		for guard := 0; guard < 4000 && r.Phase == game.Playing; guard++ {
			seat := r.Turn()
			action := ai.Decide(r, seat, rng)
			var err error
			if action.Pass {
				err = r.Pass(seat)
			} else {
				err = r.Play(seat, action.Cards)
			}
			if err != nil {
				t.Fatalf("seed %d seat %d action %+v: %v", seed, seat, action, err)
			}
		}
		if r.Phase != game.Over {
			t.Fatalf("seed %d stalled without finishing", seed)
		}
		places := map[int]bool{}
		sum := 0.0
		for _, rank := range r.Result().Ranks {
			if rank.Place < 1 || rank.Place > 4 || places[rank.Place] {
				t.Fatalf("seed %d has broken places %+v", seed, r.Result().Ranks)
			}
			places[rank.Place] = true
			sum += rank.Delta
		}
		if len(places) != 4 {
			t.Fatalf("seed %d ranked %d seats", seed, len(places))
		}
		if sum != 0 {
			t.Fatalf("seed %d is not zero-sum: %v", seed, sum)
		}
		for _, p := range r.Players {
			if len(p.Hand) != 0 && p.Place != 4 {
				t.Fatalf("seed %d seat %d holds %d cards but placed %d", seed, p.Seat, len(p.Hand), p.Place)
			}
		}
	}
}

// TestOpeningPlayIsAlwaysAvailable makes sure no deal can strand the leader.
func TestOpeningPlayIsAlwaysAvailable(t *testing.T) {
	for seed := int64(0); seed < 300; seed++ {
		r := game.New(1, 1, names, [4]float64{}, rand.New(rand.NewSource(seed)))
		if len(r.LegalPlays(r.Turn())) == 0 {
			t.Fatalf("seed %d left the leader %d without a play", seed, r.Turn())
		}
	}
}
