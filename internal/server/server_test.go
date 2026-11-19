package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kuaile13/kuaile13/internal/cards"
	"github.com/kuaile13/kuaile13/internal/game"
	"github.com/kuaile13/kuaile13/internal/server"
)

// envelope mirrors the wire contract of every endpoint in this package.
type envelope struct {
	OK     bool         `json:"ok"`
	Error  string       `json:"error"`
	View   *server.View `json:"view"`
	Events []game.Event `json:"events"`
	Hint   struct {
		Cards cards.Hand `json:"cards"`
		Text  string     `json:"text"`
	} `json:"hint"`
}

var testWeb = fstest.MapFS{
	"index.html": &fstest.MapFile{Data: []byte(`<!doctype html><title>开心十三张</title>`)},
}

func start(t *testing.T, think time.Duration) (*httptest.Server, *server.Table) {
	t.Helper()
	table := server.NewTable(1, think)
	stop := table.Run()
	srv := httptest.NewServer(server.Handlers{Table: table, Web: testWeb}.Mux())
	t.Cleanup(func() { srv.Close(); stop() })
	return srv, table
}

func get(t *testing.T, url string) envelope {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	return decode(t, res)
}

func post(t *testing.T, url, body string) (int, envelope) {
	t.Helper()
	res, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	return res.StatusCode, decode(t, res)
}

func decode(t *testing.T, res *http.Response) envelope {
	t.Helper()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	var out envelope
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return out
}

func TestFirstStateDealsThirteenToTheHumanAndHidesTheBots(t *testing.T) {
	srv, _ := start(t, time.Hour)
	s := get(t, srv.URL+"/api/state?since=0")
	if len(s.View.Seats) != 4 {
		t.Fatalf("seats = %d", len(s.View.Seats))
	}
	if got := len(s.View.Seats[0].Hand); got != 13 {
		t.Fatalf("human hand = %d cards, want 13", got)
	}
	for _, seat := range s.View.Seats[1:] {
		if seat.Hand != nil {
			t.Fatalf("seat %d leaked its hand to the client", seat.Seat)
		}
		if seat.Count != 13 {
			t.Fatalf("seat %d count = %d, want 13", seat.Seat, seat.Count)
		}
	}
	if s.View.Phase != game.Playing {
		t.Fatalf("phase = %s", s.View.Phase)
	}
	if len(s.Events) == 0 || s.Events[0].Type != "deal" {
		t.Fatalf("events should open with the deal, got %+v", s.Events)
	}
}

func TestOpeningLeaderCannotPassAndASoloPlayIsRefused(t *testing.T) {
	srv, _ := start(t, time.Hour)
	s := get(t, srv.URL+"/api/state?since=0")
	if code, reply := post(t, srv.URL+"/api/pass", `{}`); code != http.StatusBadRequest || reply.Error == "" {
		t.Fatalf("pass as %d-th actor should be refused: %d %+v", s.View.Turn, code, reply)
	}
	if code, reply := post(t, srv.URL+"/api/play", `{"cards":[{"rank":9,"suit":0}]}`); code != http.StatusBadRequest {
		t.Fatalf("an off-turn play should be refused, got %d %+v", code, reply)
	}
}

func TestStakeCarriesThroughRestart(t *testing.T) {
	srv, _ := start(t, time.Hour)
	before := get(t, srv.URL+"/api/state?since=0")
	if code, reply := post(t, srv.URL+"/api/restart", `{"bet":3}`); code != http.StatusOK || !reply.OK {
		t.Fatalf("restart reply = %d %+v", code, reply)
	}
	after := get(t, srv.URL+"/api/state?since=0")
	if after.View.Bet != 3 {
		t.Fatalf("bet = %v, want 3", after.View.Bet)
	}
	if after.View.RoundNo != before.View.RoundNo+1 {
		t.Fatalf("round = %d, want %d", after.View.RoundNo, before.View.RoundNo+1)
	}
	for _, seat := range after.View.Seats {
		if seat.Bet != 3 {
			t.Fatalf("seat %d bet = %v, want 3", seat.Seat, seat.Bet)
		}
	}
}

func TestResetScoresClearsTheLedger(t *testing.T) {
	srv, _ := start(t, time.Hour)
	if code, reply := post(t, srv.URL+"/api/restart", `{"bet":1,"resetScores":true}`); code != http.StatusOK || !reply.OK {
		t.Fatalf("restart reply = %d %+v", code, reply)
	}
	s := get(t, srv.URL+"/api/state?since=0")
	for _, seat := range s.View.Seats {
		if seat.Total != 0 {
			t.Fatalf("seat %d total = %v after a reset", seat.Seat, seat.Total)
		}
	}
}

// TestPlaysARoundThroughTheAPI drives the human seat with the hint endpoint and
// lets the clock run the bots, which covers the full browser path: poll, act,
// poll again, settle.
func TestPlaysARoundThroughTheAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("需要跑完整一局")
	}
	srv, _ := start(t, 2*time.Millisecond)
	deadline := time.Now().Add(30 * time.Second)
	cursor := 0
	seenEvents := 0
	for time.Now().Before(deadline) {
		s := get(t, srv.URL+"/api/state?since="+strconv.Itoa(cursor))
		v := s.View
		cursor = v.Seq
		seenEvents += len(s.Events)
		if v.Phase == game.Over {
			break
		}
		if v.Turn != server.HumanSeat {
			time.Sleep(6 * time.Millisecond)
			continue
		}
		_, hint := post(t, srv.URL+"/api/hint", `{}`)
		if len(hint.Hint.Cards) > 0 {
			payload, err := json.Marshal(map[string]any{"cards": hint.Hint.Cards})
			if err != nil {
				t.Fatal(err)
			}
			if code, reply := post(t, srv.URL+"/api/play", string(payload)); code != http.StatusOK || !reply.OK {
				t.Fatalf("playing the hint failed: %d %+v (%s)", code, reply, hint.Hint.Text)
			}
		} else if v.CanPass {
			if code, reply := post(t, srv.URL+"/api/pass", `{}`); code != http.StatusOK || !reply.OK {
				t.Fatalf("passing on the hint failed: %d %+v (%s)", code, reply, hint.Hint.Text)
			}
		} else {
			// 提示建议不出，但轮到我们先出时必须出牌，改用最小的单张。
			hand := v.Seats[server.HumanSeat].Hand
			if code, reply := post(t, srv.URL+"/api/play", cardPayload(hand[0])); code != http.StatusOK {
				t.Fatalf("forced lead failed: %d %+v (%s)", code, reply, hint.Hint.Text)
			}
		}
	}
	final := get(t, srv.URL+"/api/state?since=0")
	if final.View.Phase != game.Over {
		t.Fatalf("round never finished, seats=%+v", final.View.Seats)
	}
	if final.View.Result == nil || len(final.View.Result.Ranks) != 4 {
		t.Fatalf("missing settlement: %+v", final.View.Result)
	}
	if seenEvents == 0 {
		t.Fatal("the event feed never delivered anything")
	}
	for _, rank := range final.View.Result.Ranks {
		if rank.Place < 1 || rank.Place > 4 {
			t.Fatalf("broken place for seat %d: %+v", rank.Seat, rank)
		}
	}
}

func cardPayload(c cards.Card) string {
	return `{"cards":[{"rank":` + strconv.Itoa(int(c.Rank)) + `,"suit":` + strconv.Itoa(int(c.Suit)) + `}]}`
}
