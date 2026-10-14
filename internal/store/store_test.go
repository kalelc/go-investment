package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kalelc/go-investment/internal/fintual"
)

func TestSaveSnapshotIsIdempotentPerDay(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	today := time.Now()

	g := fintual.Goal{ID: "1", Name: "APV", NAV: 100, Raw: []byte(`{}`)}
	if err := st.SaveSnapshot(ctx, today, []fintual.Goal{g}); err != nil {
		t.Fatal(err)
	}
	g.NAV = 200
	if err := st.SaveSnapshot(ctx, today, []fintual.Goal{g}); err != nil {
		t.Fatal(err)
	}

	rows, err := st.History(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].NAV != 200 {
		t.Fatalf("esperaba 1 fila con nav 200, obtuve %+v", rows)
	}
}

func TestPreviousNAV(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	day := func(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }
	save := func(date, id string, nav float64) {
		if err := st.SaveSnapshot(ctx, day(date), []fintual.Goal{{ID: id, Name: id, NAV: nav, Raw: []byte(`{}`)}}); err != nil {
			t.Fatal(err)
		}
	}
	save("2026-10-10", "a", 100)
	save("2026-10-12", "a", 120)
	save("2026-10-11", "b", 50)
	save("2026-10-13", "a", 130) // "today": excluded

	prev, err := st.PreviousNAV(ctx, "2026-10-13")
	if err != nil {
		t.Fatal(err)
	}
	if len(prev) != 2 || prev["a"] != 120 || prev["b"] != 50 {
		t.Fatalf("esperaba a=120 b=50, obtuve %v", prev)
	}
}

func TestBalanceHistoryUpsertAndOrder(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	if has, _ := st.HasBalanceHistory(ctx, "goal:1"); has {
		t.Fatal("no debería haber historial todavía")
	}
	pts := []fintual.BalancePoint{
		{Date: "2026-10-02", Valuation: 110, Raw: []byte(`{}`)},
		{Date: "2026-10-01", Valuation: 100, Raw: []byte(`{}`)},
	}
	if err := st.SaveBalanceHistory(ctx, "goal:1", pts); err != nil {
		t.Fatal(err)
	}
	// A shorter interval overwrites one day and adds another.
	pts = []fintual.BalancePoint{
		{Date: "2026-10-02", Valuation: 111, Raw: []byte(`{}`)},
		{Date: "2026-10-03", Valuation: 120, Raw: []byte(`{}`)},
	}
	if err := st.SaveBalanceHistory(ctx, "goal:1", pts); err != nil {
		t.Fatal(err)
	}

	got, err := st.BalanceHistory(ctx, "goal:1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Date != "2026-10-01" || got[1].Valuation != 111 || got[2].Date != "2026-10-03" {
		t.Fatalf("serie inesperada: %+v", got)
	}
	if has, _ := st.HasBalanceHistory(ctx, "goal:1"); !has {
		t.Fatal("debería haber historial")
	}
}

func TestTrimLeadingEmptyHistory(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	pts := []fintual.BalancePoint{
		{Date: "2023-10-13", Raw: []byte(`{}`)},
		{Date: "2023-10-20", Raw: []byte(`{}`)},
		{Date: "2026-03-10", Valuation: 100, Raw: []byte(`{}`)},
		{Date: "2026-04-01", Raw: []byte(`{}`)}, // emptied later: kept
		{Date: "2026-05-01", Valuation: 50, Raw: []byte(`{}`)},
	}
	if err := st.SaveBalanceHistory(ctx, "goal:1", pts); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveBalanceHistory(ctx, "goal:2", pts[:2]); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"goal:1", "goal:2"} {
		if err := st.TrimLeadingEmptyHistory(ctx, id); err != nil {
			t.Fatal(err)
		}
	}

	got, _ := st.BalanceHistory(ctx, "goal:1")
	if len(got) != 3 || got[0].Date != "2026-03-10" || got[1].Date != "2026-04-01" {
		t.Fatalf("esperaba conservar desde la apertura, obtuve %+v", got)
	}
	if got, _ := st.BalanceHistory(ctx, "goal:2"); len(got) != 0 {
		t.Fatalf("un objetivo sin saldo nunca no debería tener historial, obtuve %+v", got)
	}
}
