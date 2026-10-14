package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kalelc/go-investment/internal/app"
	"github.com/kalelc/go-investment/internal/fintual"
)

// send applies a message without running the resulting commands, which would
// hit Fintual or the Keychain.
func send(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(model)
}

func typeText(t *testing.T, m model, s string) model {
	for _, r := range s {
		m = send(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

var enter = tea.KeyPressMsg{Code: tea.KeyEnter}

func TestLoginFlowToDashboard(t *testing.T) {
	m := newModel(&app.App{}, "")

	m = send(t, m, errMsg{app.ErrNeedLogin})
	if m.screen != screenLogin || !m.email.Focused() {
		t.Fatalf("esperaba login con foco en email, screen=%v", m.screen)
	}

	m = typeText(t, m, "a@b.cl")
	m = send(t, m, enter)
	if !m.password.Focused() {
		t.Fatal("enter en el email debería pasar a la contraseña")
	}
	m = typeText(t, m, "secret")
	m = send(t, m, enter)
	if m.screen != screenLoading {
		t.Fatalf("enviar credenciales debería mostrar carga, screen=%v", m.screen)
	}

	m = send(t, m, loginStartedMsg{&app.Login{}})
	if m.screen != screenCode || !strings.Contains(m.info, "a@b.cl") {
		t.Fatalf("esperaba pantalla de código, screen=%v info=%q", m.screen, m.info)
	}

	m = typeText(t, m, "000000")
	m = send(t, m, enter)
	m = send(t, m, errMsg{errors.New("código inválido")})
	if m.screen != screenCode || m.err == nil {
		t.Fatalf("un código inválido debería volver a la pantalla de código con error, screen=%v", m.screen)
	}

	m = send(t, m, loginDoneMsg{})
	if m.password.Value() != "" {
		t.Error("la contraseña no debería quedar en memoria tras el login")
	}
	profit, realized := 100.0, 25.0
	m = send(t, m, refreshedMsg{&app.Snapshot{
		Date:  "2026-10-13",
		Goals: []fintual.Goal{{ID: "goal:1", Name: "Casa", NAV: 1500, Profit: &profit, RealizedProfit: &realized}},
		Prev:  map[string]float64{"goal:1": 1400},
	}})
	if m.screen != screenDashboard {
		t.Fatalf("esperaba el panel, screen=%v", m.screen)
	}
	view := m.View().Content
	if !strings.Contains(view, "Patrimonio") || !strings.Contains(view, "Panel") {
		t.Errorf("la barra debería mostrar la ruta y el patrimonio:\n%s", view)
	}
	for _, want := range []string{"Casa", "$1.500", "+$100", "+$25", "+$125", "Ganancia actual", "Realizada", "TOTAL", "Foto del 2026-10-13 guardada"} {
		if !strings.Contains(view, want) {
			t.Errorf("el panel no muestra %q:\n%s", want, view)
		}
	}
}

func TestExpiredSessionPrefillsEmail(t *testing.T) {
	m := newModel(&app.App{}, "a@b.cl")
	m = send(t, m, errMsg{fmt.Errorf("%w: la sesión expiró (iniciada hace 2d 3h)", app.ErrNeedLogin)})
	if m.screen != screenLogin || !m.password.Focused() {
		t.Fatalf("con email conocido el foco debería ir a la contraseña, screen=%v", m.screen)
	}
	if !strings.Contains(m.View().Content, "la sesión expiró") {
		t.Error("debería explicar que la sesión expiró")
	}
}
