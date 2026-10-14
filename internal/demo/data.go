// Package demo runs the app against a fake Fintual backend with generated
// data, for trying it out without an account and for README screenshots.
package demo

import (
	"math"
	"math/rand/v2"
	"time"

	"github.com/kalelc/go-investment/internal/fintual"
)

type goalSpec struct {
	id, name   string
	ageDays    int
	deposit    float64 // monthly, on day 5
	initial    float64
	drift, vol float64 // daily
	withdrawAt int     // days ago; 0 for none
	withdraw   float64
}

var specs = []goalSpec{
	{id: "goal:101", name: "🏠 Casa", ageDays: 900, initial: 2_000_000, deposit: 300_000,
		drift: 0.0004, vol: 0.004, withdrawAt: 200, withdraw: 1_500_000},
	{id: "goal:102", name: "🎓 Universidad", ageDays: 420, initial: 500_000, deposit: 150_000,
		drift: 0.0002, vol: 0.003},
	{id: "apv_b:201", name: "🐷 APV", ageDays: 1000, initial: 1_000_000, deposit: 100_000,
		drift: 0.0003, vol: 0.005},
	{id: "custom:301", name: "🚀 Acciones", ageDays: 240, initial: 800_000, deposit: 50_000,
		drift: 0.0004, vol: 0.011},
}

type series struct {
	spec        goalSpec
	points      []fintual.BalancePoint // chronological, daily
	netDeposits float64
}

// generate simulates each goal day by day up to today with a fixed seed, so
// the numbers are the same on every run; only the dates move with today.
func generate(today time.Time) []series {
	rng := rand.New(rand.NewPCG(2026, 10))
	out := make([]series, 0, len(specs))
	for _, sp := range specs {
		s := series{spec: sp}
		var valuation, costBasis, realized, withdrawn float64
		for ago := sp.ageDays; ago >= 0; ago-- {
			date := today.AddDate(0, 0, -ago)
			if ago < sp.ageDays {
				valuation *= 1 + sp.drift + sp.vol*rng.NormFloat64()
			}
			deposit := 0.0
			if ago == sp.ageDays {
				deposit = sp.initial
			} else if date.Day() == 5 {
				deposit = sp.deposit
			}
			valuation += deposit
			costBasis += deposit
			s.netDeposits += deposit

			if ago == sp.withdrawAt && sp.withdraw > 0 && valuation > sp.withdraw {
				// Whole pesos keep deposits exact: gains are derived from these.
				soldCost := math.Round(sp.withdraw / valuation * costBasis)
				realized += sp.withdraw - soldCost
				costBasis -= soldCost
				valuation -= sp.withdraw
				withdrawn += sp.withdraw
				s.netDeposits -= sp.withdraw
			}

			s.points = append(s.points, fintual.BalancePoint{
				Date:            date.Format(time.DateOnly),
				Valuation:       math.Round(valuation),
				CostBasis:       costBasis,
				UnrealizedGain:  math.Round(valuation) - costBasis,
				RealizedGain:    realized,
				WithdrawnAmount: withdrawn,
			})
		}
		out = append(out, s)
	}
	return out
}

// interval mimics Fintual: daily points for up to a year, weekly beyond.
func (s series) interval(code string) []fintual.BalancePoint {
	days := map[string]int{
		"last_month": 31, "last_three_months": 92, "last_six_months": 183,
		"last_year": 366, "last_three_years": 3 * 366,
	}[code]
	pts := s.points
	if code != "all_time" && days < len(pts) {
		pts = pts[len(pts)-days:]
	}
	if len(pts) <= 366 {
		return pts
	}
	var weekly []fintual.BalancePoint
	for i := len(pts) - 1; i >= 0; i -= 7 {
		weekly = append([]fintual.BalancePoint{pts[i]}, weekly...)
	}
	return weekly
}
