// goinv keeps a daily record of a Fintual portfolio.
//
// Usage:
//
//	goinv                   terminal app (login, balances, history)
//	goinv snapshot          save today's snapshot with the stored session (for cron)
//	goinv history [days]    list stored snapshots (default 30 days)
//	goinv status            show the stored session and its age
//	goinv logout            delete the stored session
//
// The session lives in the Keychain; the password is never stored.
//
// Environment:
//
//	GOINV_DEBUG     log Fintual response details (the TUI logs to ~/.local/share/goinv/debug.log)
//	GOINV_DB        SQLite path (default ~/.local/share/goinv/goinv.db)
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/kalelc/go-investment/internal/app"
	"github.com/kalelc/go-investment/internal/session"
	"github.com/kalelc/go-investment/internal/store"
	"github.com/kalelc/go-investment/internal/tui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}

	switch cmd {
	case "status":
		return status()
	case "logout":
		if err := session.Delete(); err != nil {
			return err
		}
		fmt.Println("Sesión borrada.")
		return nil
	}

	dataDir, err := dataDir()
	if err != nil {
		return err
	}
	dbPath := os.Getenv("GOINV_DB")
	if dbPath == "" {
		dbPath = filepath.Join(dataDir, "goinv.db")
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	a := &app.App{Store: st}

	// Generous: the first history sync fetches years of data per goal.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	switch cmd {
	case "":
		// The TUI owns the screen, so debug output goes to a file.
		if os.Getenv("GOINV_DEBUG") != "" {
			f, err := os.OpenFile(filepath.Join(dataDir, "debug.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			defer f.Close()
			a.Debug = f
		}
		return tui.Run(a)

	case "snapshot":
		a.Debug = debugWriter()
		snap, err := a.Refresh(ctx)
		if errors.Is(err, app.ErrNeedLogin) {
			return fmt.Errorf("%w; abre la app con `make run` para iniciar sesión", err)
		}
		if err != nil {
			return err
		}
		lipgloss.Println(tui.RenderSnapshot(snap, -1))
		fmt.Printf("Foto del %s guardada (%d objetivos)\n", snap.Date, len(snap.Goals))
		n, err := a.SyncHistory(ctx, snap.Goals)
		if err != nil {
			return fmt.Errorf("sincronizando historial: %w", err)
		}
		fmt.Printf("Historial al día (%d puntos actualizados)\n", n)
		return nil

	case "history":
		days := 30
		if len(args) > 1 {
			if days, err = strconv.Atoi(args[1]); err != nil {
				return fmt.Errorf("días inválidos: %q", args[1])
			}
		}
		rows, err := a.History(ctx, days)
		if err != nil {
			return err
		}
		lipgloss.Println(tui.RenderHistory(rows))
		return nil

	default:
		return fmt.Errorf("comando desconocido: %q (uso: goinv [snapshot | history [días] | status | logout])", cmd)
	}
}

func status() error {
	s, err := session.Load()
	if errors.Is(err, session.ErrNotFound) {
		fmt.Println("No hay sesión guardada. Abre la app con: make run")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Printf("Email:           %s\n", s.Email)
	fmt.Printf("Sesión iniciada: %s (hace %s)\n", s.LoggedInAt.Format("2006-01-02 15:04"), app.Age(s.LoggedInAt))
	fmt.Printf("Último uso OK:   %s (hace %s)\n", s.LastUsedAt.Format("2006-01-02 15:04"), app.Age(s.LastUsedAt))
	return nil
}

func dataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".local", "share", "goinv")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	// MkdirAll keeps the permissions of an existing directory.
	return dir, os.Chmod(dir, 0o700)
}

func debugWriter() io.Writer {
	if os.Getenv("GOINV_DEBUG") != "" {
		return os.Stderr
	}
	return nil
}
