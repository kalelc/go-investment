package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	barBackground  = lipgloss.Color("236")
	barStyle       = lipgloss.NewStyle().Background(barBackground).Foreground(lipgloss.BrightWhite)
	barBrandStyle  = barStyle.Bold(true).Foreground(lipgloss.Cyan)
	barMutedStyle  = barStyle.Foreground(lipgloss.BrightBlack)
	barValueStyle  = barStyle.Bold(true).Foreground(currentColor)
	demoBadgeStyle = lipgloss.NewStyle().Bold(true).Background(lipgloss.Magenta).Foreground(lipgloss.Black)
)

// headerBar renders a full-width bar with the breadcrumb on the left and the
// portfolio total on the right. Every segment carries the background color,
// since each Render resets the terminal style.
func (m model) headerBar() string {
	sep := barMutedStyle.Render(" › ")
	left := barStyle.Render(" ") + barBrandStyle.Render("Fintual")
	if m.app != nil && m.app.Demo {
		left += barStyle.Render(" ") + demoBadgeStyle.Render(" DEMO ")
	}
	for _, crumb := range m.breadcrumb() {
		left += sep + barStyle.Render(crumb)
	}
	left += barStyle.Render(" ")

	var right string
	if m.snapshot != nil {
		var total float64
		for _, g := range m.snapshot.Goals {
			total += g.NAV
		}
		right = barMutedStyle.Render("Patrimonio ") + barValueStyle.Render(CLP(total))
		if !m.updatedAt.IsZero() {
			right += barMutedStyle.Render(" · actualizado " + m.updatedAt.Format("15:04"))
		}
		right += barStyle.Render(" ")
	}

	width := m.width
	if width == 0 {
		width = lipgloss.Width(left) + lipgloss.Width(right) + 1
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		right, gap = "", max(0, width-lipgloss.Width(left))
	}
	return left + barStyle.Render(strings.Repeat(" ", gap)) + right
}

func (m model) breadcrumb() []string {
	switch m.screen {
	case screenLogin:
		return []string{"Iniciar sesión"}
	case screenCode:
		return []string{"Verificación"}
	case screenDashboard:
		return []string{"Panel"}
	case screenHistory:
		return []string{"Panel", "Fotos"}
	case screenGoal:
		mode := "Mensual"
		if m.goal.mode == goalDaily {
			mode = "Diario"
		}
		return []string{"Panel", m.goal.goal.Name, mode}
	default:
		return nil
	}
}
