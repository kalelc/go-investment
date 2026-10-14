package tui

import (
	"math"
	"strconv"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"github.com/kalelc/go-investment/internal/app"
	"github.com/kalelc/go-investment/internal/store"
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Cyan).Padding(0, 1)
	cellStyle   = lipgloss.NewStyle().Padding(0, 1)
	numStyle    = cellStyle.Align(lipgloss.Right)
	upStyle     = numStyle.Foreground(lipgloss.Green)
	downStyle   = numStyle.Foreground(lipgloss.Red)
	mutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Red)
	borderStyle = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)

	currentColor       = lipgloss.Yellow
	currentHeaderStyle = headerStyle.Foreground(currentColor)
	currentLabelStyle  = lipgloss.NewStyle().Bold(true).Foreground(currentColor)
)

const (
	colCurrentGain  = "Ganancia actual"
	colRealizedGain = "Realizada"
	colTotalGain    = "Total"
)

// gainLegend explains the gain columns; the current gain is the highlighted one.
func gainLegend() string {
	return currentLabelStyle.Render(colCurrentGain) + mutedStyle.Render(": de las cuotas que tienes hoy; es la que muestra Fintual.") + "\n" +
		mutedStyle.Render(colRealizedGain+": de cuotas ya vendidas (retiros o rebalanceos).") + "\n" +
		mutedStyle.Render(colTotalGain+": actual + realizada = saldo − depósitos netos.")
}

// highlightCurrent marks the current-gain column, keeping its sign color.
func highlightCurrent(st lipgloss.Style) lipgloss.Style { return st.Bold(true) }

// sumGains adds unrealized and realized gains; nil only if both are missing.
func sumGains(unrealized, realized *float64) *float64 {
	if unrealized == nil && realized == nil {
		return nil
	}
	var total float64
	for _, v := range []*float64{unrealized, realized} {
		if v != nil {
			total += *v
		}
	}
	return &total
}

func newTable(headers ...string) *table.Table {
	return table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(borderStyle).
		Headers(headers...)
}

func signStyle(v *float64) lipgloss.Style {
	switch {
	case v != nil && *v > 0:
		return upStyle
	case v != nil && *v < 0:
		return downStyle
	default:
		return numStyle
	}
}

// signedColumn collects optional signed amounts plus their total.
type signedColumn struct {
	values []*float64
	sum    float64
	any    bool
}

func (c *signedColumn) add(v *float64) string {
	c.values = append(c.values, v)
	if v != nil {
		c.sum += *v
		c.any = true
	}
	return optSigned(v)
}

func (c *signedColumn) addTotal() string { return c.add(ptrIf(c.sum, c.any)) }

// RenderSnapshot highlights row `selected`; pass -1 for none.
func RenderSnapshot(s *app.Snapshot, selected int) string {
	var total float64
	var current, realized, totalGain, change signedColumn

	t := newTable("Objetivo", "Saldo", colCurrentGain, colRealizedGain, colTotalGain, "Var. vs foto anterior")
	for _, g := range s.Goals {
		total += g.NAV
		var v *float64
		if prev, ok := s.Prev[g.ID]; ok {
			v = ptr(g.NAV - prev)
		}
		t.Row(g.Name, CLP(g.NAV), current.add(g.Profit), realized.add(g.RealizedProfit),
			totalGain.add(sumGains(g.Profit, g.RealizedProfit)), change.add(v))
	}
	t.Row("TOTAL", CLP(total), current.addTotal(), realized.addTotal(), totalGain.addTotal(), change.addTotal())

	signed := map[int]*signedColumn{2: &current, 3: &realized, 4: &totalGain, 5: &change}
	last := len(s.Goals)
	t.StyleFunc(func(row, col int) lipgloss.Style {
		var st lipgloss.Style
		switch {
		case row == table.HeaderRow && col == 2:
			return currentHeaderStyle
		case row == table.HeaderRow:
			return headerStyle
		case col == 0:
			st = cellStyle
		case signed[col] != nil:
			st = signStyle(signed[col].values[row])
		default:
			st = numStyle
		}
		if col == 2 {
			st = highlightCurrent(st)
		}
		if row == selected {
			st = st.Reverse(true)
		}
		return st.Bold(row == last || col == 2)
	})
	return t.Render() + "\n" + gainLegend()
}

// RenderHistory expects rows sorted by date descending.
func RenderHistory(rows []store.Row) string {
	if len(rows) == 0 {
		return mutedStyle.Render("Todavía no hay fotos guardadas.")
	}

	deltas := make([]*float64, len(rows))
	for i, r := range rows {
		for _, older := range rows[i+1:] {
			if older.GoalID == r.GoalID {
				d := r.NAV - older.NAV
				deltas[i] = &d
				break
			}
		}
	}

	t := newTable("Fecha", "Objetivo", "Saldo", "Variación")
	for i, r := range rows {
		t.Row(r.Date, r.GoalName, CLP(r.NAV), optSigned(deltas[i]))
	}
	t.StyleFunc(func(row, col int) lipgloss.Style {
		switch {
		case row == table.HeaderRow:
			return headerStyle
		case col == 3:
			return signStyle(deltas[row])
		case col == 2:
			return numStyle
		default:
			return cellStyle
		}
	})
	return t.Render()
}

func ptrIf(v float64, ok bool) *float64 {
	if !ok {
		return nil
	}
	return &v
}

func optSigned(v *float64) string {
	switch {
	case v == nil:
		return "—"
	case *v > 0:
		return "+" + CLP(*v)
	default:
		return CLP(*v)
	}
}

// CLP formats an amount as $1.234.567.
func CLP(v float64) string {
	s := strconv.FormatInt(int64(math.Round(v)), 10)
	neg := s[0] == '-'
	if neg {
		s = s[1:]
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	if neg {
		return "-$" + string(out)
	}
	return "$" + string(out)
}
