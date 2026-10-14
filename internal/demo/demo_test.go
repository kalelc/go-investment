package demo

import (
	"context"
	"math"
	"testing"

	"github.com/kalelc/go-investment/internal/fintual"
)

func TestDemoEndToEnd(t *testing.T) {
	ctx := context.Background()
	a, cleanup, err := Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	snap, err := a.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Goals) != len(specs) {
		t.Fatalf("esperaba %d objetivos, obtuve %d", len(specs), len(snap.Goals))
	}
	for _, g := range snap.Goals {
		if g.Profit == nil || g.RealizedProfit == nil || g.Deposited == nil {
			t.Fatalf("faltan ganancias en %+v", g)
		}
		// Same identity as real Fintual data: current + realized = balance - net deposits.
		if diff := *g.Profit + *g.RealizedProfit - (g.NAV - *g.Deposited); math.Abs(diff) > 5 {
			t.Errorf("%s: las ganancias no cuadran (diferencia %.0f)", g.Name, diff)
		}
		if _, ok := snap.Prev[g.ID]; !ok {
			t.Errorf("%s: falta la foto de ayer", g.Name)
		}
	}

	if _, err := a.SyncHistory(ctx, snap.Goals); err != nil {
		t.Fatal(err)
	}
	casa, err := a.GoalHistory(ctx, "goal:101")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range monthlyDeposits(casa) {
		if m != 300_000 && m != -1_200_000 && m != 2_000_000 && m != 2_300_000 {
			t.Errorf("aporte mensual inesperado %.0f: deberían ser montos exactos", m)
		}
	}
	last := casa[len(casa)-1]
	if len(casa) < 400 || last.RealizedGain == 0 {
		t.Errorf("Casa debería tener historia larga y ganancia realizada: %d puntos, realizada %.0f", len(casa), last.RealizedGain)
	}
}

func monthlyDeposits(pts []fintual.BalancePoint) []float64 {
	var out []float64
	var open fintual.BalancePoint
	for i, p := range pts {
		if i+1 < len(pts) && pts[i+1].Date[:7] == p.Date[:7] {
			continue
		}
		gain := func(p fintual.BalancePoint) float64 { return p.UnrealizedGain + p.RealizedGain }
		out = append(out, p.Valuation-open.Valuation-(gain(p)-gain(open)))
		open = p
	}
	return out
}
