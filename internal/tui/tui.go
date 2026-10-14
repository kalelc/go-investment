// Package tui is the goinv terminal app.
package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kalelc/go-investment/internal/app"
	"github.com/kalelc/go-investment/internal/fintual"
	"github.com/kalelc/go-investment/internal/store"
)

func Run(a *app.App) error {
	_, err := tea.NewProgram(newModel(a, a.LastEmail())).Run()
	return err
}

type screen int

const (
	screenLoading screen = iota
	screenLogin
	screenCode
	screenDashboard
	screenHistory
	screenGoal
)

const historyDays = 30

type model struct {
	app    *app.App
	screen screen

	spinner  spinner.Model
	loading  string
	email    textinput.Model
	password textinput.Model
	code     textinput.Model
	login    *app.Login

	snapshot *app.Snapshot
	cursor   int
	history  []store.Row
	goal     goalView
	syncing  bool

	info          string
	err           error
	width, height int
	updatedAt     time.Time
}

type (
	refreshedMsg    struct{ snap *app.Snapshot }
	loginStartedMsg struct{ login *app.Login }
	loginDoneMsg    struct{}
	historyMsg      struct{ rows []store.Row }
	syncedMsg       struct{ points int }
	syncErrMsg      struct{ err error }
	goalHistoryMsg  struct {
		goal   fintual.Goal
		points []fintual.BalancePoint
	}
	loggedOutMsg struct{}
	errMsg       struct{ err error }
)

func newModel(a *app.App, lastEmail string) model {
	email := textinput.New()
	email.Prompt = "Email      "
	email.Placeholder = "tu@email.cl"
	email.SetValue(lastEmail)

	password := textinput.New()
	password.Prompt = "Contraseña "
	password.EchoMode = textinput.EchoPassword

	code := textinput.New()
	code.Prompt = "Código     "
	code.Placeholder = "llegó a tu email"

	return model{
		app:      a,
		screen:   screenLoading,
		loading:  "Consultando saldos en Fintual…",
		spinner:  spinner.New(spinner.WithSpinner(spinner.Dot)),
		email:    email,
		password: password,
		code:     code,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.refresh())
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Minute)
}

func (m model) refresh() tea.Cmd {
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		snap, err := m.app.Refresh(c)
		if err != nil {
			return errMsg{err}
		}
		return refreshedMsg{snap}
	}
}

func (m model) startLogin(email, password string) tea.Cmd {
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		l, err := m.app.StartLogin(c, email, password)
		if err != nil {
			return errMsg{err}
		}
		return loginStartedMsg{l}
	}
}

func (m model) finishLogin(code string) tea.Cmd {
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		if err := m.app.FinishLogin(c, m.login, code); err != nil {
			return errMsg{err}
		}
		return loginDoneMsg{}
	}
}

func (m model) loadHistory() tea.Cmd {
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		rows, err := m.app.History(c, historyDays)
		if err != nil {
			return errMsg{err}
		}
		return historyMsg{rows}
	}
}

func (m model) syncHistory() tea.Cmd {
	goals := m.snapshot.Goals
	return func() tea.Msg {
		// The first sync fetches years of data per goal.
		c, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		n, err := m.app.SyncHistory(c, goals)
		if err != nil {
			return syncErrMsg{err}
		}
		return syncedMsg{n}
	}
}

func (m model) loadGoal(g fintual.Goal) tea.Cmd {
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		points, err := m.app.GoalHistory(c, g.ID)
		if err != nil {
			return errMsg{err}
		}
		return goalHistoryMsg{g, points}
	}
}

func (m model) logout() tea.Cmd {
	return func() tea.Msg {
		if err := m.app.Logout(); err != nil {
			return errMsg{err}
		}
		return loggedOutMsg{}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case refreshedMsg:
		m.snapshot, m.err = msg.snap, nil
		m.cursor = min(m.cursor, max(0, len(msg.snap.Goals)-1))
		m.updatedAt = time.Now()
		m.info = fmt.Sprintf("Foto del %s guardada", msg.snap.Date)
		m.screen = screenDashboard
		if m.syncing {
			return m, nil
		}
		m.syncing = true
		return m, tea.Batch(m.spinner.Tick, m.syncHistory())

	case syncedMsg:
		m.syncing = false
		m.info = fmt.Sprintf("Historial al día (%d puntos actualizados) · %s", msg.points, time.Now().Format("15:04"))
		return m, nil

	case syncErrMsg:
		m.syncing = false
		if errors.Is(msg.err, app.ErrNeedLogin) {
			return m.handleError(msg.err)
		}
		// The dashboard is still useful without history.
		m.err = fmt.Errorf("sincronizando historial: %w", msg.err)
		return m, nil

	case goalHistoryMsg:
		m.goal, m.err = newGoalView(msg.goal, msg.points), nil
		m.screen = screenGoal
		return m, nil

	case loginStartedMsg:
		m.login, m.err = msg.login, nil
		m.info = fmt.Sprintf("Fintual envió un código a %s.", m.email.Value())
		m.code.SetValue("")
		m.screen = screenCode
		return m, m.code.Focus()

	case loginDoneMsg:
		m.login, m.err = nil, nil
		m.password.SetValue("")
		return m.startLoading("Sesión iniciada. Consultando saldos…", m.refresh())

	case historyMsg:
		m.history, m.err = msg.rows, nil
		m.screen = screenHistory
		return m, nil

	case loggedOutMsg:
		m.snapshot, m.info = nil, "Sesión cerrada."
		return m.showLogin(nil)

	case errMsg:
		return m.handleError(msg.err)

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m.handleKey(msg)
	}

	return m.updateInputs(msg)
}

func (m model) handleError(err error) (tea.Model, tea.Cmd) {
	switch {
	case errors.Is(err, app.ErrNeedLogin):
		// Not an error to show in red: the user just needs to log in.
		m.info = ""
		if msg := err.Error(); msg != app.ErrNeedLogin.Error() {
			m.info = strings.TrimPrefix(msg, app.ErrNeedLogin.Error()+": ")
		}
		return m.showLogin(nil)
	case m.screen == screenLoading && m.login != nil:
		// Wrong code: ask for it again.
		m.screen = screenCode
		m.err = err
		return m, m.code.Focus()
	case m.screen == screenLoading && m.snapshot == nil:
		return m.showLogin(err)
	case m.screen == screenLoading:
		m.screen = screenDashboard
	}
	m.err = err
	return m, nil
}

func (m model) showLogin(err error) (tea.Model, tea.Cmd) {
	m.screen, m.err, m.login = screenLogin, err, nil
	m.password.SetValue("")
	m.code.Blur()
	if m.email.Value() == "" {
		m.password.Blur()
		return m, m.email.Focus()
	}
	m.email.Blur()
	return m, m.password.Focus()
}

func (m model) startLoading(label string, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.screen, m.loading, m.err = screenLoading, label, nil
	m.email.Blur()
	m.password.Blur()
	m.code.Blur()
	return m, tea.Batch(m.spinner.Tick, cmd)
}

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch m.screen {
	case screenLogin:
		switch key {
		case "esc":
			return m, tea.Quit
		case "tab", "shift+tab", "up", "down":
			return m.toggleLoginFocus()
		case "enter":
			if m.email.Focused() {
				return m.toggleLoginFocus()
			}
			email, password := strings.TrimSpace(m.email.Value()), m.password.Value()
			if email == "" || password == "" {
				m.err = errors.New("ingresa email y contraseña")
				return m, nil
			}
			m.info = ""
			return m.startLoading("Validando credenciales…", m.startLogin(email, password))
		}

	case screenCode:
		switch key {
		case "esc":
			m.info = ""
			return m.showLogin(nil)
		case "enter":
			code := strings.TrimSpace(m.code.Value())
			if code == "" {
				m.err = errors.New("ingresa el código que llegó a tu email")
				return m, nil
			}
			return m.startLoading("Verificando código…", m.finishLogin(code))
		}

	case screenDashboard:
		switch key {
		case "q", "esc":
			return m, tea.Quit
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			m.cursor = min(len(m.snapshot.Goals)-1, m.cursor+1)
		case "enter":
			if len(m.snapshot.Goals) > 0 {
				return m.startLoading("Cargando historial del objetivo…", m.loadGoal(m.snapshot.Goals[m.cursor]))
			}
		case "r":
			return m.startLoading("Consultando saldos en Fintual…", m.refresh())
		case "h":
			return m.startLoading("Cargando historial…", m.loadHistory())
		case "l":
			return m, m.logout()
		}
		return m, nil

	case screenHistory:
		switch key {
		case "q":
			return m, tea.Quit
		case "esc", "h":
			m.screen = screenDashboard
		}
		return m, nil

	case screenGoal:
		page := max(1, m.height/2)
		switch key {
		case "q":
			return m, tea.Quit
		case "esc", "backspace":
			m.screen = screenDashboard
		case "tab":
			m.goal.toggleMode()
		case "down", "j":
			m.goal.scroll(1)
		case "up", "k":
			m.goal.scroll(-1)
		case "pgdown", "space", "f":
			m.goal.scroll(page)
		case "pgup", "b":
			m.goal.scroll(-page)
		case "g", "home":
			m.goal.scroll(-m.goal.rows())
		case "G", "end":
			m.goal.scroll(m.goal.rows())
		}
		return m, nil

	case screenLoading:
		return m, nil
	}

	return m.updateInputs(msg)
}

func (m model) toggleLoginFocus() (tea.Model, tea.Cmd) {
	if m.email.Focused() {
		m.email.Blur()
		return m, m.password.Focus()
	}
	m.password.Blur()
	return m, m.email.Focus()
}

func (m model) updateInputs(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.screen {
	case screenLogin:
		if m.email.Focused() {
			m.email, cmd = m.email.Update(msg)
		} else {
			m.password, cmd = m.password.Update(msg)
		}
	case screenCode:
		m.code, cmd = m.code.Update(msg)
	}
	return m, cmd
}

func (m model) View() tea.View {
	var body, help string
	switch m.screen {
	case screenLoading:
		body = m.spinner.View() + " " + m.loading
	case screenLogin:
		body = m.email.View() + "\n" + m.password.View()
		help = "enter continuar · tab cambiar campo · esc salir"
	case screenCode:
		body = m.code.View()
		help = "enter verificar · esc volver"
	case screenDashboard:
		body = RenderSnapshot(m.snapshot, m.cursor)
		help = "↑/↓ elegir · enter ver historial del objetivo · r refrescar · h fotos · l cerrar sesión · q salir"
		if m.syncing {
			body += "\n\n" + m.spinner.View() + " Sincronizando historial de Fintual…"
		}
	case screenGoal:
		body = m.goal.render(m.width, m.height)
		help = "tab mensual/diario · ↑/↓ mover · pgup/pgdn página · g/G inicio/fin · esc volver · q salir"
	case screenHistory:
		body = fmt.Sprintf("Últimos %d días\n\n", historyDays) + RenderHistory(m.history)
		help = "h/esc volver · q salir"
	}

	var b strings.Builder
	b.WriteString(body + "\n")
	if m.err != nil {
		b.WriteString("\n" + errorStyle.Render("✗ "+m.err.Error()) + "\n")
	} else if m.info != "" && (m.screen == screenLogin || m.screen == screenCode || m.screen == screenDashboard) {
		b.WriteString("\n" + mutedStyle.Render(m.info) + "\n")
	}
	if help != "" {
		b.WriteString("\n" + mutedStyle.Render(help) + "\n")
	}

	pad := 2
	if m.width > 0 && lipgloss.Width(b.String())+2*pad > m.width {
		pad = 0
	}
	content := lipgloss.NewStyle().Padding(1, pad).Render(b.String())
	if m.width > 0 {
		content = lipgloss.PlaceHorizontal(m.width, lipgloss.Center, content)
	}
	v := tea.NewView(m.headerBar() + "\n" + content)
	v.AltScreen = true
	v.WindowTitle = "goinv"
	return v
}
