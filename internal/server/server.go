// Package server exposes a table over HTTP and keeps the three bots moving.
package server

import (
	"encoding/json"
	"io/fs"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/kuaile13/kuaile13/internal/ai"
	"github.com/kuaile13/kuaile13/internal/cards"
	"github.com/kuaile13/kuaile13/internal/combo"
	"github.com/kuaile13/kuaile13/internal/game"
)

// HumanSeat is the seat the browser plays.
const HumanSeat = 0

// Names are the four seats, ordered counter-clockwise from the human player.
var Names = [4]string{"你", "下家·王五", "对家·张三", "上家·李四"}

// Table owns the live round and the bot clock.
type Table struct {
	mu      sync.Mutex
	round   *game.Round
	roundNo int
	bet     float64
	totals  [4]float64
	rng     *rand.Rand
	think   time.Duration
	lastAct time.Time
}

// NewTable starts scoring from zero with the given stake and bot pace.
func NewTable(bet float64, think time.Duration) *Table {
	if bet <= 0 {
		bet = 1
	}
	if think <= 0 {
		think = 700 * time.Millisecond
	}
	return &Table{bet: bet, think: think, rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

// NewTableWithSeed is the deterministic constructor used by the tests.
func NewTableWithSeed(bet float64, seed int64) *Table {
	t := NewTable(bet, time.Millisecond)
	t.rng = rand.New(rand.NewSource(seed))
	return t
}

// deal opens the next round, banking whatever score the previous one reached.
// Callers must hold t.mu.
func (t *Table) deal() {
	if t.round != nil {
		for _, p := range t.round.Players {
			t.totals[p.Seat] = p.Total
		}
	}
	t.roundNo++
	t.round = game.New(t.roundNo, t.bet, Names, t.totals, t.rng)
	t.lastAct = time.Now()
}

// ensureRound deals the very first round; later rounds wait for a restart so
// that the settlement stays on screen.
func (t *Table) ensureRound() {
	if t.round == nil {
		t.deal()
	}
}

// Step advances one bot seat and reports whether the table changed.
func (t *Table) Step() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureRound()
	if t.round.Phase != game.Playing || t.round.Turn() == HumanSeat {
		return false
	}
	if time.Since(t.lastAct) < t.think {
		return false
	}
	seat := t.round.Turn()
	action := ai.Decide(t.round, seat, t.rng)
	err := t.applyBot(seat, action)
	if err != nil {
		// Reaching here means the bot policy proposed something the engine
		// refuses, so redeal instead of wedging the turn clock.
		log.Printf("kuaile13: bot seat %d failed: %v — redealing", seat, err)
		t.deal()
		return true
	}
	t.lastAct = time.Now()
	return true
}

func (t *Table) applyBot(seat int, action ai.Action) error {
	if action.Pass {
		return t.round.Pass(seat)
	}
	err := t.round.Play(seat, action.Cards)
	if err == nil {
		return nil
	}
	log.Printf("kuaile13: bot %d play rejected: %v", seat, err)
	return t.round.Pass(seat)
}

// Run drives the bot clock until the returned stop function is called.
func (t *Table) Run() (stop func()) {
	done := make(chan struct{})
	go func() {
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				for t.Step() {
				}
			}
		}
	}()
	return func() { close(done) }
}

// View is the redacted snapshot a client renders.
type View struct {
	Phase   game.Phase        `json:"phase"`
	RoundNo int               `json:"roundNo"`
	Bet     float64           `json:"bet"`
	Turn    int               `json:"turn"`
	Leader  int               `json:"leader"`
	Table   *TableCell        `json:"table"`
	Seats   []Seat            `json:"seats"`
	Result  *game.RoundResult `json:"result,omitempty"`
	Seq     int               `json:"seq"`
	CanPass bool              `json:"canPass"`
}

// TableCell is the play that must be beaten.
type TableCell struct {
	Seat  int        `json:"seat"`
	Name  string     `json:"name"`
	Cards cards.Hand `json:"cards"`
}

// Seat is one player as seen from the human seat.
type Seat struct {
	Seat   int        `json:"seat"`
	Name   string     `json:"name"`
	IsBot  bool       `json:"isBot"`
	Count  int        `json:"count"`
	Place  int        `json:"place"`
	Bet    float64    `json:"bet"`
	Delta  float64    `json:"delta"`
	Total  float64    `json:"total"`
	Points float64    `json:"points"` // bankroll in points, shown instead of Total
	Hand   cards.Hand `json:"hand,omitempty"`
}

// Snapshot returns the table state plus every event newer than since.
func (t *Table) Snapshot(since int) (View, []game.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureRound()
	return t.view(), t.round.EventsSince(since)
}

func (t *Table) view() View {
	v := View{
		Phase:   t.round.Phase,
		RoundNo: t.roundNo,
		Bet:     t.round.Bet,
		Turn:    t.round.Turn(),
		Leader:  t.round.Leader(),
		Seq:     len(t.round.Events()),
		CanPass: t.round.CanPass(HumanSeat),
	}
	if onTable := t.round.OnTable(); onTable != nil {
		v.Table = &TableCell{
			Seat:  t.round.TableSeat(),
			Name:  onTable.Name,
			Cards: onTable.Cards,
		}
	}
	for _, p := range t.round.Players {
		s := Seat{
			Seat: p.Seat, Name: p.Name, IsBot: p.IsBot,
			Count: len(p.Hand), Place: p.Place, Bet: p.Bet,
			Delta: p.Delta, Total: p.Total, Points: p.Points(),
		}
		if p.Seat == HumanSeat {
			s.Hand = p.Hand.Sorted()
		}
		v.Seats = append(v.Seats, s)
	}
	v.Result = t.round.Result()
	return v
}

// Play commits the human selection.
func (t *Table) Play(chosen cards.Hand) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureRound()
	if err := t.round.Play(HumanSeat, chosen); err != nil {
		return err
	}
	t.lastAct = time.Now()
	return nil
}

// Pass declines the current trick.
func (t *Table) Pass() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureRound()
	if err := t.round.Pass(HumanSeat); err != nil {
		return err
	}
	t.lastAct = time.Now()
	return nil
}

// Hint suggests a play for the human seat. It always names a playable option:
// the bot policy may prefer to hold its cards back, but the player deserves to
// see what is available.
func (t *Table) Hint() (*combo.Combo, string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureRound()
	if t.round.Phase != game.Playing || t.round.Turn() != HumanSeat {
		return nil, "还没轮到你出牌"
	}
	legal := t.round.LegalPlays(HumanSeat)
	if len(legal) == 0 {
		return nil, "管不上，只能不出"
	}
	action := ai.Decide(t.round, HumanSeat, t.rng)
	if !action.Pass {
		c, err := combo.Classify(action.Cards)
		if err == nil {
			return c, "建议出 " + c.Name + "：" + action.Why
		}
	}
	return legal[0], "可以出 " + legal[0].Name + "（电脑会选择先不出，把牌留着）"
}

// Restart abandons the current round and deals a new one.
func (t *Table) Restart(bet float64, resetScores bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if bet > 0 {
		t.bet = bet
	}
	if resetScores {
		t.totals = [4]float64{}
		t.round = nil
	}
	t.deal()
}

// Handlers wires the JSON API and the embedded UI onto a mux.
type Handlers struct {
	Table *Table
	Web   fs.FS
}

// Mux builds the HTTP routes.
func (h Handlers) Mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", h.state)
	mux.HandleFunc("POST /api/play", h.play)
	mux.HandleFunc("POST /api/pass", h.pass)
	mux.HandleFunc("POST /api/hint", h.hint)
	mux.HandleFunc("POST /api/restart", h.restart)
	mux.Handle("/", http.FileServer(http.FS(h.Web)))
	return mux
}

type reply struct {
	OK     bool         `json:"ok"`
	Error  string       `json:"error,omitempty"`
	View   *View        `json:"view,omitempty"`
	Events []game.Event `json:"events,omitempty"`
	Hint   *hintReply   `json:"hint,omitempty"`
}

// hintReply is the suggestion for the 提示 button. A suggestion of "不出"
// carries no cards, so the UI can disable 出牌 and highlight 不出.
type hintReply struct {
	Cards cards.Hand `json:"cards,omitempty"`
	Text  string     `json:"text"`
}

func (h Handlers) state(w http.ResponseWriter, r *http.Request) {
	since, _ := strconv.Atoi(r.URL.Query().Get("since"))
	view, events := h.Table.Snapshot(since)
	writeJSON(w, http.StatusOK, reply{OK: true, View: &view, Events: events})
}

type playRequest struct {
	Cards cards.Hand `json:"cards"`
}

type restartRequest struct {
	Bet         float64 `json:"bet"`
	ResetScores bool    `json:"resetScores"`
}

func (h Handlers) play(w http.ResponseWriter, r *http.Request) {
	var req playRequest
	if !decode(w, r, &req) {
		return
	}
	if err := h.Table.Play(req.Cards); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reply{OK: true})
}

func (h Handlers) pass(w http.ResponseWriter, r *http.Request) {
	if err := h.Table.Pass(); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reply{OK: true})
}

func (h Handlers) hint(w http.ResponseWriter, r *http.Request) {
	c, text := h.Table.Hint()
	out := &hintReply{Text: text}
	if c != nil {
		out.Cards = c.Cards
	}
	writeJSON(w, http.StatusOK, reply{OK: true, Hint: out})
}

func (h Handlers) restart(w http.ResponseWriter, r *http.Request) {
	var req restartRequest
	if !decode(w, r, &req) {
		return
	}
	h.Table.Restart(req.Bet, req.ResetScores)
	writeJSON(w, http.StatusOK, reply{OK: true})
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		fail(w, err)
		return false
	}
	return true
}

func fail(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, reply{Error: err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
