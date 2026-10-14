// Package app holds the use cases shared by the TUI and the CLI.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/kalelc/go-investment/internal/fintual"
	"github.com/kalelc/go-investment/internal/session"
	"github.com/kalelc/go-investment/internal/store"
)

var ErrNeedLogin = errors.New("hay que iniciar sesión en Fintual")

type App struct {
	Store *store.Store
	Debug io.Writer
}

type Snapshot struct {
	Date  string
	Goals []fintual.Goal
	// Prev maps Goal.ID to the balance in its previous snapshot.
	Prev map[string]float64
}

func (a *App) client() *fintual.Client {
	c := fintual.New()
	c.Debug = a.Debug
	return c
}

func (a *App) sessionClient() (*fintual.Client, *session.Session, error) {
	sess, err := session.Load()
	if errors.Is(err, session.ErrNotFound) {
		return nil, nil, ErrNeedLogin
	}
	if err != nil {
		return nil, nil, err
	}
	client := a.client()
	client.SetCookies(sess.Cookies)
	return client, sess, nil
}

func expired(err error, sess *session.Session) error {
	if errors.Is(err, fintual.ErrUnauthorized) {
		return fmt.Errorf("%w: la sesión expiró (iniciada hace %s, último uso OK hace %s)",
			ErrNeedLogin, Age(sess.LoggedInAt), Age(sess.LastUsedAt))
	}
	return err
}

// touch persists the current cookies: Fintual may rotate them on any response.
func touch(client *fintual.Client, sess *session.Session) error {
	sess.Cookies, sess.LastUsedAt = client.Cookies(), time.Now()
	if err := session.Save(sess); err != nil {
		return fmt.Errorf("actualizando sesión: %w", err)
	}
	return nil
}

func (a *App) Refresh(ctx context.Context) (*Snapshot, error) {
	client, sess, err := a.sessionClient()
	if err != nil {
		return nil, err
	}
	goals, err := client.Goals(ctx)
	if err != nil {
		return nil, expired(err, sess)
	}
	if err := touch(client, sess); err != nil {
		return nil, err
	}

	today := time.Now().In(chile())
	if err := a.Store.SaveSnapshot(ctx, today, goals); err != nil {
		return nil, err
	}
	date := today.Format(time.DateOnly)
	prev, err := a.Store.PreviousNAV(ctx, date)
	if err != nil {
		return nil, err
	}
	return &Snapshot{Date: date, Goals: goals, Prev: prev}, nil
}

// SyncHistory fetches every interval the first time, longest first, so the
// finer-grained short intervals overwrite the long ones; afterwards it only
// refreshes the last month.
func (a *App) SyncHistory(ctx context.Context, goals []fintual.Goal) (int, error) {
	client, sess, err := a.sessionClient()
	if err != nil {
		return 0, err
	}
	total := 0
	for _, g := range goals {
		has, err := a.Store.HasBalanceHistory(ctx, g.ID)
		if err != nil {
			return total, err
		}
		intervals := fintual.Intervals
		if has {
			intervals = []string{"last_month"}
		}
		for _, interval := range intervals {
			points, err := client.BalanceHistory(ctx, g.ID, interval)
			if err != nil {
				return total, expired(err, sess)
			}
			if err := a.Store.SaveBalanceHistory(ctx, g.ID, points); err != nil {
				return total, err
			}
			total += len(points)
		}
		if err := a.Store.TrimLeadingEmptyHistory(ctx, g.ID); err != nil {
			return total, err
		}
	}
	return total, touch(client, sess)
}

func (a *App) GoalHistory(ctx context.Context, goalID string) ([]fintual.BalancePoint, error) {
	return a.Store.BalanceHistory(ctx, goalID)
}

type Login struct {
	client   *fintual.Client
	email    string
	password string
}

func (a *App) StartLogin(ctx context.Context, email, password string) (*Login, error) {
	client := a.client()
	if err := client.StartLogin(ctx, email, password); err != nil {
		return nil, err
	}
	return &Login{client: client, email: email, password: password}, nil
}

func (a *App) FinishLogin(ctx context.Context, l *Login, code string) error {
	if err := l.client.FinishLogin(ctx, l.email, l.password, code); err != nil {
		return err
	}
	now := time.Now()
	return session.Save(&session.Session{Email: l.email, Cookies: l.client.Cookies(), LoggedInAt: now, LastUsedAt: now})
}

func (a *App) LastEmail() string {
	if s, err := session.Load(); err == nil {
		return s.Email
	}
	return ""
}

func (a *App) Logout() error { return session.Delete() }

func (a *App) History(ctx context.Context, days int) ([]store.Row, error) {
	return a.Store.History(ctx, days)
}

// Age formats elapsed time as "3d 4h" or "2h 15m".
func Age(t time.Time) string {
	d := time.Since(t).Round(time.Minute)
	days, hours, mins := int(d.Hours())/24, int(d.Hours())%24, int(d.Minutes())%60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

func chile() *time.Location {
	loc, err := time.LoadLocation("America/Santiago")
	if err != nil {
		return time.Local
	}
	return loc
}
