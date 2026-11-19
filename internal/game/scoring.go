package game

// StartPoints is the bankroll every seat begins a session with. The unit is one
// bet, so at 1x the winner collects 4 points, second and third pay 0.5 each and
// last place pays 3: the four seats always sum to 4 × StartPoints.
const StartPoints = 1000

// PointsOf converts a running score in bet units into the bankroll shown to the
// player. One point is one bet, so halves must survive: rounding second and
// third place to whole points would break the 4 × StartPoints pool invariant.
func PointsOf(total float64) float64 {
	return StartPoints + total
}

// settle applies the payout ladder of 开心十三张.
//
// With everyone staking one bet:
//
//	第 1 名出完  赢家，收取另外三家输掉的分数
//	第 2 名出完  输掉自己下注的 2/4
//	第 3 名出完  输掉自己下注的 2/4
//	第 4 名出完  输掉另外三家的下注之和
//
// Bets may differ per seat, so the winner collects exactly what the other three
// paid and the table stays zero-sum.
func (r *Round) settle() {
	byPlace := map[int]*Player{}
	for _, p := range r.Players {
		byPlace[p.Place] = p
	}
	for _, p := range r.Players {
		p.Delta = 0
	}
	for place, p := range byPlace {
		switch place {
		case 2, 3:
			p.Delta = -openingScore * p.Bet
		case seatCount:
			others := 0.0
			for _, q := range r.Players {
				if q.Seat != p.Seat {
					others += q.Bet
				}
			}
			p.Delta = -lastScore * others
		}
	}
	winner := byPlace[1]
	if winner != nil {
		owed := 0.0
		for _, p := range r.Players {
			if p.Seat != winner.Seat {
				owed -= p.Delta
			}
		}
		winner.Delta = owed
	}
	for _, p := range r.Players {
		p.Total += p.Delta
	}
}
