package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kalelc/go-investment/internal/fintual"
)

func fakeHistory(days int) []fintual.BalancePoint {
	start, _ := time.Parse(time.DateOnly, "2025-01-01")
	var pts []fintual.BalancePoint
	v, gain := 1_000_000.0, 0.0
	for i := range days {
		if i == days/2 {
			v += 500_000 // deposit: raises the balance, not the gain
		}
		d := float64((i*37)%11-5) * 1000
		v, gain = v+d, gain+d
		pts = append(pts, fintual.BalancePoint{
			Date: start.AddDate(0, 0, i).Format(time.DateOnly), Valuation: v, UnrealizedGain: gain,
		})
	}
	return pts
}

func TestGoalViewShowsUnrealizedGainLikeFintual(t *testing.T) {
	pts := []fintual.BalancePoint{
		{Date: "2026-10-01", Valuation: 1000, UnrealizedGain: 100},
		// Selling shares moves 50 from unrealized to realized: no daily result.
		{Date: "2026-10-02", Valuation: 950, UnrealizedGain: 50, RealizedGain: 50},
	}
	g := newGoalView(fintual.Goal{Name: "108"}, pts)
	g.mode = goalDaily
	out := g.render(100, 40)
	for _, want := range []string{"Ganancia actual", "+$50", "Realizada", "Total", "+$100", "es la que muestra Fintual"} {
		if !strings.Contains(out, want) {
			t.Errorf("falta %q en:\n%s", want, out)
		}
	}
	// The balance drops 50, but the daily result must not.
	if n := strings.Count(out, "-$50"); n != 1 {
		t.Errorf("mover ganancia a realizada no debería verse como pérdida del día:\n%s", out)
	}
}

func TestGoalViewRender(t *testing.T) {
	g := newGoalView(fintual.Goal{Name: "Casa"}, fakeHistory(400))
	g.mode = goalDaily
	out := g.render(100, 40)
	for _, want := range []string{"Desde 2025-01-01", "400 días", "2026-02-04", "días 1–"} {
		if !strings.Contains(out, want) {
			t.Errorf("falta %q en:\n%s", want, out)
		}
	}
	g.offsets[goalDaily] = 199 // first visible row is the deposit day
	out = g.render(100, 40)
	for _, want := range []string{"+$503.000", "+$3.000"} {
		if !strings.Contains(out, want) {
			t.Errorf("la fila del depósito debería mostrar %q:\n%s", want, out)
		}
	}
	if os.Getenv("SHOW") != "" {
		g := newGoalView(fintual.Goal{Name: "Casa"}, fakeHistory(400))
		fmt.Println(g.render(100, 50))
	}
}

func TestMonthlySummaries(t *testing.T) {
	pts := []fintual.BalancePoint{
		{Date: "2026-01-15", Valuation: 1000, UnrealizedGain: 0},
		{Date: "2026-01-31", Valuation: 1010, UnrealizedGain: 10},
		{Date: "2026-02-10", Valuation: 1520, UnrealizedGain: 20}, // +500 deposit, +10 result
		{Date: "2026-02-28", Valuation: 1505, UnrealizedGain: 5},
	}
	months := monthlySummaries(pts)
	if len(months) != 2 {
		t.Fatalf("esperaba 2 meses, obtuve %+v", months)
	}
	jan, feb := months[0], months[1]
	if jan.Month != "2026-01" || jan.Close != 1010 || jan.NetDeposits != 1000 || jan.Result != 10 {
		t.Errorf("enero inesperado: %+v", jan)
	}
	if feb.Close != 1505 || feb.NetDeposits != 500 || feb.Result != -5 {
		t.Errorf("febrero inesperado: %+v", feb)
	}
	// -5 / (1010 + 500/2)
	if feb.Return == nil || pct(feb.Return) != "-0,40%" {
		t.Errorf("rentabilidad de febrero inesperada: %s", pct(feb.Return))
	}

	years := yearSummaries(months)
	if len(years) != 1 || years[0].Result != 5 || years[0].Best != "2026-01" || years[0].Worst != "2026-02" {
		t.Errorf("año inesperado: %+v", years)
	}
}

func TestDailyViewMarksMultiDayRows(t *testing.T) {
	pts := []fintual.BalancePoint{
		{Date: "2025-06-01", Valuation: 1000},
		{Date: "2025-06-08", Valuation: 1010, UnrealizedGain: 10},
		{Date: "2025-06-09", Valuation: 1015, UnrealizedGain: 15},
	}
	g := newGoalView(fintual.Goal{Name: "APV"}, pts)
	g.mode = goalDaily
	out := g.render(120, 50)
	if !strings.Contains(out, "2025-06-08 (7 días)") {
		t.Errorf("la fila semanal debería marcarse:\n%s", out)
	}
	if strings.Contains(out, "2025-06-09 (") {
		t.Errorf("una fila diaria no debería marcarse:\n%s", out)
	}
}

func TestGoalViewMonthlyIsDefault(t *testing.T) {
	g := newGoalView(fintual.Goal{Name: "Casa"}, fakeHistory(400))
	out := g.render(100, 50)
	for _, want := range []string{"[Mensual]", "Saldo cierre", "Aportes netos", "2026-02", "Año 2025:", "Mejor mes"} {
		if !strings.Contains(out, want) {
			t.Errorf("falta %q en:\n%s", want, out)
		}
	}
	g.toggleMode()
	if !strings.Contains(g.render(100, 50), "[Diario]") {
		t.Error("tab debería cambiar a la vista diaria")
	}
}
