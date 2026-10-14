package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"github.com/kalelc/go-investment/internal/fintual"
)

type goalMode int

const (
	goalMonthly goalMode = iota
	goalDaily
)

type goalView struct {
	goal    fintual.Goal
	points  []fintual.BalancePoint // chronological
	months  []monthSummary         // chronological
	mode    goalMode
	offsets [2]int // first visible row per mode; 0 is the most recent
}

func newGoalView(goal fintual.Goal, points []fintual.BalancePoint) goalView {
	return goalView{goal: goal, points: points, months: monthlySummaries(points)}
}

func (g goalView) rows() int {
	if g.mode == goalMonthly {
		return len(g.months)
	}
	return len(g.points)
}

func (g *goalView) scroll(delta int) {
	g.offsets[g.mode] = min(max(0, g.rows()-1), max(0, g.offsets[g.mode]+delta))
}

func (g *goalView) toggleMode() {
	g.mode = 1 - g.mode
}

// totalGain includes realized gains so that selling shares (which moves gain
// from unrealized to realized) doesn't show up as a daily loss.
func totalGain(p fintual.BalancePoint) float64 { return p.UnrealizedGain + p.RealizedGain }

func (g goalView) render(width, height int) string {
	if len(g.points) == 0 {
		return mutedStyle.Render("Todavía no hay historial para este objetivo. Presiona r en el panel para sincronizar.")
	}
	first, last := g.points[0], g.points[len(g.points)-1]

	summary := fmt.Sprintf("Desde %s · %d días · Saldo %s\n", first.Date, len(g.points), CLP(last.Valuation))
	summary += currentLabelStyle.Render(colCurrentGain) + " " + highlightCurrent(signStyle(ptr(last.UnrealizedGain)).UnsetPadding().UnsetAlign()).Render(optSigned(ptr(last.UnrealizedGain))) +
		" · " + colRealizedGain + " " + colorSigned(last.RealizedGain) +
		" · " + colTotalGain + " " + colorSigned(totalGain(last))

	tabs := []string{"Mensual", "Diario"}
	for i := range tabs {
		if goalMode(i) == g.mode {
			tabs[i] = currentLabelStyle.Render("[" + tabs[i] + "]")
		} else {
			tabs[i] = mutedStyle.Render(" " + tabs[i] + " ")
		}
	}

	var body string
	if g.mode == goalMonthly {
		body = g.renderMonthly(height)
	} else {
		body = g.renderDaily(height)
	}
	return summary + "\n\n" + strings.Join(tabs, " ") + "\n" + body
}

func (g goalView) visible(height, used int) (start, visible int) {
	visible = max(3, height-used)
	start = min(g.offsets[g.mode], max(0, g.rows()-visible))
	return start, visible
}

func (g goalView) renderMonthly(height int) string {
	years := yearSummaries(g.months)
	start, visible := g.visible(height, 22+len(years))

	maxAbs := 0.0
	for _, m := range g.months {
		maxAbs = max(maxAbs, math.Abs(m.Result))
	}
	const barWidth = 16

	t := newTable("Mes", "Saldo cierre", "Aportes netos", "Resultado", "Rent.", "")
	var shown []monthSummary
	for i := len(g.months) - 1 - start; i >= 0 && len(shown) < visible; i-- {
		m := g.months[i]
		t.Row(m.Month, CLP(m.Close), optSigned(ptr(m.NetDeposits)), optSigned(ptr(m.Result)), pct(m.Return), bar(m.Result, maxAbs, barWidth))
		shown = append(shown, m)
	}
	t.StyleFunc(func(r, c int) lipgloss.Style {
		if r == table.HeaderRow {
			return headerStyle
		}
		m := shown[r]
		switch c {
		case 0:
			return cellStyle
		case 3:
			return highlightCurrent(signStyle(ptr(m.Result)))
		case 4:
			return signStyle(m.Return)
		case 5:
			return signStyle(ptr(m.Result)).UnsetAlign().Width(barWidth + 2)
		default:
			return numStyle
		}
	})
	position := mutedStyle.Render(fmt.Sprintf("meses %d–%d de %d", start+1, start+len(shown), len(g.months)))

	var yearLines []string
	for _, y := range years {
		yearLines = append(yearLines, fmt.Sprintf("%s %s %s · Mejor mes %s · Peor mes %s",
			currentLabelStyle.Render("Año "+y.Year+":"), colorSigned(y.Result),
			signStyle(y.Return).UnsetPadding().UnsetAlign().Render("("+pct(y.Return)+")"), y.Best, y.Worst))
	}

	legend := mutedStyle.Render("Aportes netos: depósitos − retiros del mes.") + "\n" +
		currentLabelStyle.Render("Resultado") + mutedStyle.Render(": lo que ganaste o perdiste en el mes, sin contar tus aportes.") + "\n" +
		mutedStyle.Render("Rent.: resultado sobre el saldo inicial más la mitad de los aportes; el año compone los meses.")

	return t.Render() + "\n" + position + "\n\n" + strings.Join(yearLines, "\n") + "\n\n" + legend
}

func (g goalView) renderDaily(height int) string {
	start, visible := g.visible(height, 24)
	t := newTable("Fecha", "Saldo", "Var. saldo", "Resultado día", colCurrentGain, colRealizedGain, colTotalGain)
	var styles [][5]lipgloss.Style
	for i := len(g.points) - 1 - start; i >= 0 && len(styles) < visible; i-- {
		p := g.points[i]
		date, varSaldo, resultado := p.Date, "—", "—"
		row := [5]lipgloss.Style{numStyle, numStyle,
			highlightCurrent(signStyle(ptr(p.UnrealizedGain))), signStyle(ptr(p.RealizedGain)), signStyle(ptr(totalGain(p)))}
		if i > 0 {
			prev := g.points[i-1]
			if days := daysBetween(prev.Date, p.Date); days > 1 {
				date += fmt.Sprintf(" (%d días)", days)
			}
			dv, dg := p.Valuation-prev.Valuation, totalGain(p)-totalGain(prev)
			varSaldo, resultado = optSigned(&dv), optSigned(&dg)
			row[0], row[1] = signStyle(&dv), signStyle(&dg)
		}
		t.Row(date, CLP(p.Valuation), varSaldo, resultado,
			optSigned(ptr(p.UnrealizedGain)), optSigned(ptr(p.RealizedGain)), optSigned(ptr(totalGain(p))))
		styles = append(styles, row)
	}
	t.StyleFunc(func(r, c int) lipgloss.Style {
		switch {
		case r == table.HeaderRow && c == 4:
			return currentHeaderStyle
		case r == table.HeaderRow:
			return headerStyle
		case c == 0:
			return cellStyle
		case c == 1:
			return numStyle
		default:
			return styles[r][c-2]
		}
	})
	position := mutedStyle.Render(fmt.Sprintf("días %d–%d de %d", start+1, start+len(styles), len(g.points)))
	note := mutedStyle.Render("Fintual entrega un punto por día solo para el último año; antes, uno por semana.\n" +
		"En las filas marcadas con (N días), la variación y el resultado cubren ese período.")
	return t.Render() + "\n" + position + "\n\n" + gainLegend() + "\n\n" + note
}

func daysBetween(from, to string) int {
	a, errA := time.Parse(time.DateOnly, from)
	b, errB := time.Parse(time.DateOnly, to)
	if errA != nil || errB != nil {
		return 0
	}
	return int(b.Sub(a).Hours() / 24)
}

func ptr(v float64) *float64 { return &v }

func colorSigned(v float64) string {
	return signStyle(&v).UnsetPadding().UnsetAlign().Render(optSigned(&v))
}
