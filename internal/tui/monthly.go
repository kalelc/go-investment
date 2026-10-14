package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/kalelc/go-investment/internal/fintual"
)

type monthSummary struct {
	Month       string // YYYY-MM
	Close       float64
	NetDeposits float64
	Result      float64
	Return      *float64
}

type yearSummary struct {
	Year        string
	Result      float64
	Return      *float64
	Best, Worst string

	bestResult, worstResult float64
}

// monthlySummaries expects chronological points and returns chronological months.
// The first month starts from zero, so its deposits are everything put in.
func monthlySummaries(points []fintual.BalancePoint) []monthSummary {
	var months []monthSummary
	var open fintual.BalancePoint
	for i, p := range points {
		if i+1 < len(points) && points[i+1].Date[:7] == p.Date[:7] {
			continue
		}
		result := totalGain(p) - totalGain(open)
		deposits := p.Valuation - open.Valuation - result
		m := monthSummary{Month: p.Date[:7], Close: p.Valuation, NetDeposits: deposits, Result: result}
		// Modified Dietz: deposits count as invested for half the month.
		if base := open.Valuation + deposits/2; base > 0 {
			m.Return = ptr(result / base)
		}
		months = append(months, m)
		open = p
	}
	return months
}

// yearSummaries compounds monthly returns; it returns the most recent year first.
func yearSummaries(months []monthSummary) []yearSummary {
	var years []yearSummary
	for _, m := range months {
		year := m.Month[:4]
		if len(years) == 0 || years[0].Year != year {
			years = append([]yearSummary{{Year: year, Return: ptr(0)}}, years...)
		}
		y := &years[0]
		y.Result += m.Result
		if m.Return != nil && y.Return != nil {
			y.Return = ptr((1+*y.Return)*(1+*m.Return) - 1)
		}
		if y.Best == "" || m.Result > y.bestResult {
			y.Best, y.bestResult = m.Month, m.Result
		}
		if y.Worst == "" || m.Result < y.worstResult {
			y.Worst, y.worstResult = m.Month, m.Result
		}
	}
	return years
}

// pct formats a fraction as a Chilean-style signed percentage: +1,16%.
func pct(v *float64) string {
	if v == nil {
		return "—"
	}
	return strings.Replace(fmt.Sprintf("%+.2f%%", *v*100), ".", ",", 1)
}

var barEighths = []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// bar draws |v| relative to maxAbs in at most width cells, with eighth-cell precision.
func bar(v, maxAbs float64, width int) string {
	if maxAbs == 0 || v == 0 {
		return ""
	}
	eighths := int(math.Round(math.Abs(v) / maxAbs * float64(width*8)))
	eighths = max(1, eighths)
	return strings.Repeat("█", eighths/8) + barEighths[eighths%8]
}
