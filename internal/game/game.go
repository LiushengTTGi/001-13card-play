// Package game runs a 开心十三张 round: four seats of thirteen cards, the holder
// of ♦2 opening, and a scoring table settled by the order in which hands empty.
package game

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/kuaile13/kuaile13/internal/cards"
	"github.com/kuaile13/kuaile13/internal/combo"
)

// Phase is the lifecycle of a round.
type Phase string

const (
	// Playing means a seat still has to act.
	Playing Phase = "playing"
	// Over means every seat has a place and the round is settled.
	Over Phase = "over"
)

const (
	seatCount    = 4
	handSize     = 13
	openingScore = 0.5 // 第 2、3 名各输自己下注的 2/4
	lastScore    = 1.0 // 第 4 名输光另外三家的下注
)

// Player is one seat at the table.
type Player struct {
	Seat  int        `json:"seat"`
	Name  string     `json:"name"`
	IsBot bool       `json:"isBot"`
	Hand  cards.Hand `json:"hand,omitempty"`
	Bet   float64    `json:"bet"`
	Place int        `json:"place"`
	Delta float64    `json:"delta"`
	Total float64    `json:"total"`
}

// Points is what the seat holds: the starting bankroll plus every settled round.
func (p *Player) Points() float64 { return PointsOf(p.Total) }

// Round is one deal of 开心十三张.
type Round struct {
	Players []*Player
	Phase   Phase
	RoundNo int
	Bet     float64

	turn      int
	leader    int
	tableSeat int
	onTable   *combo.Combo
	pending   map[int]bool
	finished  []*Player
	events    []Event
	seq       int
	rng       *rand.Rand
}

// Event is one entry of the action feed the UI replays.
type Event struct {
	Seq   int          `json:"seq"`
	Type  string       `json:"type"`
	Seat  int          `json:"seat"`
	Cards cards.Hand   `json:"cards,omitempty"`
	Kind  string       `json:"kind,omitempty"`
	Text  string       `json:"text,omitempty"`
	Data  *RoundResult `json:"result,omitempty"`
}

// RoundResult settles a finished round.
type RoundResult struct {
	RoundNo int       `json:"roundNo"`
	Bet     float64   `json:"bet"`
	Ranks   []Ranking `json:"ranks"`
}

// Ranking is one seat's settlement line.
type Ranking struct {
	Seat   int     `json:"seat"`
	Name   string  `json:"name"`
	Place  int     `json:"place"`
	Bet    float64 `json:"bet"`
	Delta  float64 `json:"delta"`
	Total  float64 `json:"total"`
	Points float64 `json:"points"` // bankroll in points: StartPoints plus Total
}

// New deals a fresh round. seat 0 is the human player, seats 1..3 are bots and
// turn order runs counter-clockwise: 0 → 1 → 2 → 3. totals carries the running
// score of each seat over from previous rounds.
func New(roundNo int, bet float64, names [seatCount]string, totals [seatCount]float64, rng *rand.Rand) *Round {
	deck := cards.NewDeck()
	deck.Shuffle(rng)
	hands := [seatCount]cards.Hand{}
	for seat := range hands {
		hands[seat] = deck[seat*handSize : (seat+1)*handSize]
	}
	return NewWithHands(roundNo, bet, names, totals, hands, rng)
}

// NewWithHands lays out known hands instead of shuffling. The ♦2 opening rule
// still applies, which lets the tests script exact trick and scoring paths.
func NewWithHands(roundNo int, bet float64, names [seatCount]string, totals [seatCount]float64, hands [seatCount]cards.Hand, rng *rand.Rand) *Round {
	if bet <= 0 {
		bet = 1
	}
	r := &Round{
		Phase:   Playing,
		RoundNo: roundNo,
		Bet:     bet,
		rng:     rng,
		pending: map[int]bool{},
	}
	for seat := 0; seat < seatCount; seat++ {
		hand := append(cards.Hand(nil), hands[seat]...)
		hand.Sort()
		r.Players = append(r.Players, &Player{
			Seat:  seat,
			Name:  names[seat],
			IsBot: seat != 0,
			Hand:  hand,
			Bet:   bet,
			Total: totals[seat],
		})
	}
	r.open()
	return r
}

// open seats the ♦2 holder as the opening leader.
func (r *Round) open() {
	r.leader = r.openingSeat()
	r.turn = r.leader
	r.tableSeat = r.leader
	r.resetPending()
	r.push("deal", r.leader, nil, "", fmt.Sprintf("第 %d 局开始，%s 持有 ♦2，先出牌", r.RoundNo, r.Players[r.leader].Name))
}

// openingSeat finds the holder of ♦2, who leads the first trick.
func (r *Round) openingSeat() int {
	for _, p := range r.Players {
		for _, c := range p.Hand {
			if c.IsDiamondTwo() {
				return p.Seat
			}
		}
	}
	return 0
}

// Turn reports the seat that must act.
func (r *Round) Turn() int { return r.turn }

// PhaseValue reports the lifecycle state.
func (r *Round) PhaseValue() Phase { return r.Phase }

// OnTable returns the combo that must be beaten, if any.
func (r *Round) OnTable() *combo.Combo { return r.onTable.Clone() }

// TableSeat returns the owner of the combo on the table.
func (r *Round) TableSeat() int { return r.tableSeat }

// Leader returns the seat that opened the current trick.
func (r *Round) Leader() int { return r.leader }

// Player returns the seat's state.
func (r *Round) Player(seat int) *Player { return r.Players[seat] }

// Events returns the action feed.
func (r *Round) Events() []Event { return append([]Event(nil), r.events...) }

// EventsSince returns the feed entries newer than seq.
func (r *Round) EventsSince(seq int) []Event {
	out := make([]Event, 0, 8)
	for _, e := range r.events {
		if e.Seq > seq {
			out = append(out, e)
		}
	}
	return out
}

// Result returns the settlement once the round is over.
func (r *Round) Result() *RoundResult {
	if r.Phase != Over {
		return nil
	}
	res := &RoundResult{RoundNo: r.RoundNo, Bet: r.Bet}
	for _, p := range r.Players {
		res.Ranks = append(res.Ranks, Ranking{
			Seat: p.Seat, Name: p.Name, Place: p.Place,
			Bet: p.Bet, Delta: p.Delta, Total: p.Total,
			Points: PointsOf(p.Total),
		})
	}
	sortByPlace(res.Ranks)
	return res
}

// MustLead reports whether the seat opens the trick and therefore has to play.
func (r *Round) MustLead(seat int) bool {
	return r.Phase == Playing && r.turn == seat && r.onTable == nil
}

// CanPass reports whether the seat is allowed to decline the current trick.
func (r *Round) CanPass(seat int) bool {
	return r.Phase == Playing && r.turn == seat && r.onTable != nil
}

// LegalPlays returns every legal answer for a seat, cheapest first. A leading
// seat gets every 牌型 it can build.
func (r *Round) LegalPlays(seat int) []*combo.Combo {
	if r.Phase != Playing || r.turn != seat {
		return nil
	}
	return combo.Answers(r.Players[seat].Hand, r.onTable)
}

// Play commits a set of cards for a seat.
func (r *Round) Play(seat int, chosen cards.Hand) error {
	if r.Phase != Playing {
		return errors.New("game/phase: 本局已经结束")
	}
	if r.turn != seat {
		return fmt.Errorf("game/turn: 还没轮到 %s", r.Players[seat].Name)
	}
	p := r.Players[seat]
	if !p.Hand.Has(chosen) {
		return errors.New("game/hand: 选出的牌不在手牌里")
	}
	c, err := combo.Classify(chosen)
	if err != nil {
		return err
	}
	if r.onTable != nil && !combo.Beats(c, r.onTable) {
		return fmt.Errorf("game/beat: %s 打不过 %s 的%s", c.Name, r.Players[r.tableSeat].Name, r.onTable.Name)
	}

	hand, err := p.Hand.Remove(chosen)
	if err != nil {
		return err
	}
	p.Hand = hand
	r.onTable = c
	r.tableSeat = seat
	r.push("play", seat, chosen, c.Name, fmt.Sprintf("%s 出 %s（%s）", p.Name, chosen, c.Name))

	if len(p.Hand) == 0 {
		r.finish(seat)
	}
	if r.endRoundIfDecided() {
		return nil
	}
	r.resetPending()
	r.advance()
	return nil
}

// Pass declines to answer the current combo.
func (r *Round) Pass(seat int) error {
	if r.Phase != Playing {
		return errors.New("game/phase: 本局已经结束")
	}
	if r.turn != seat {
		return fmt.Errorf("game/turn: 还没轮到 %s", r.Players[seat].Name)
	}
	if r.onTable == nil {
		return errors.New("game/lead: 轮到你先出，必须出牌")
	}
	delete(r.pending, seat)
	r.push("pass", seat, nil, "", fmt.Sprintf("%s 不出", r.Players[seat].Name))
	if r.endRoundIfDecided() {
		return nil
	}
	r.advance()
	return nil
}

// advance moves to the next seat still answering, or closes the trick.
func (r *Round) advance() {
	if len(r.pending) == 0 {
		r.closeTrick()
		return
	}
	r.turn = r.nextPendingFrom(r.tableSeat)
}

// closeTrick hands the lead back to the seat that last played, or to the next
// live seat when that player has already run out of cards.
func (r *Round) closeTrick() {
	leader := r.tableSeat
	if !r.isActive(leader) {
		leader = r.nextActiveFrom(leader)
	}
	r.leader = leader
	r.turn = leader
	r.tableSeat = leader
	r.onTable = nil
	r.resetPending()
	r.push("trick", leader, nil, "", fmt.Sprintf("%s 拿下这一轮，继续先出", r.Players[leader].Name))
}

func (r *Round) resetPending() {
	r.pending = map[int]bool{}
	for _, p := range r.Players {
		if r.isActive(p.Seat) && p.Seat != r.turn {
			r.pending[p.Seat] = true
		}
	}
}

func (r *Round) isActive(seat int) bool { return r.Players[seat].Place == 0 }

func (r *Round) nextActiveFrom(seat int) int {
	for i := 1; i <= seatCount; i++ {
		s := (seat + i) % seatCount
		if r.isActive(s) {
			return s
		}
	}
	return seat
}

func (r *Round) nextPendingFrom(seat int) int {
	for i := 1; i <= seatCount; i++ {
		s := (seat + i) % seatCount
		if r.pending[s] {
			return s
		}
	}
	return r.nextActiveFrom(seat)
}

func (r *Round) finish(seat int) {
	p := r.Players[seat]
	p.Place = len(r.finished) + 1
	r.finished = append(r.finished, p)
	r.push("finish", seat, nil, "", fmt.Sprintf("%s 出完了牌，第 %d 名", p.Name, p.Place))
}

// endRoundIfDecided settles the round once only one seat still holds cards.
// That seat takes last place without playing its hand out card by card.
func (r *Round) endRoundIfDecided() bool {
	var last *Player
	live := 0
	for _, p := range r.Players {
		if r.isActive(p.Seat) {
			live++
			last = p
		}
	}
	if live > 1 {
		return false
	}
	if live == 1 && last != nil {
		r.finish(last.Seat)
	}
	r.settle()
	r.Phase = Over
	r.push("over", lastSeat(r.finished), nil, "", "本局结束")
	return true
}

func (r *Round) push(eventType string, seat int, played cards.Hand, kind, text string) {
	r.seq++
	r.events = append(r.events, Event{
		Seq: r.seq, Type: eventType, Seat: seat,
		Cards: append(cards.Hand(nil), played...), Kind: kind, Text: text,
	})
}

func lastSeat(pl []*Player) int {
	if len(pl) == 0 {
		return 0
	}
	return pl[len(pl)-1].Seat
}

func sortByPlace(ranks []Ranking) {
	for i := 0; i < len(ranks); i++ {
		for j := i + 1; j < len(ranks); j++ {
			if ranks[j].Place < ranks[i].Place {
				ranks[i], ranks[j] = ranks[j], ranks[i]
			}
		}
	}
}
