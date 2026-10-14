// Package store persists snapshots and balance history in SQLite.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	"github.com/kalelc/go-investment/internal/fintual"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("aplicando schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// SaveSnapshot upserts by (date, goal), so running it twice a day is safe.
func (s *Store) SaveSnapshot(ctx context.Context, date time.Time, goals []fintual.Goal) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO snapshots (date, goal_id, goal_name, nav, deposited, profit, raw_json, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (date, goal_id) DO UPDATE SET
			goal_name = excluded.goal_name,
			nav = excluded.nav,
			deposited = excluded.deposited,
			profit = excluded.profit,
			raw_json = excluded.raw_json,
			fetched_at = excluded.fetched_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	day := date.Format(time.DateOnly)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, g := range goals {
		if _, err := stmt.ExecContext(ctx, day, g.ID, g.Name, g.NAV, g.Deposited, g.Profit, string(g.Raw), now); err != nil {
			return fmt.Errorf("guardando objetivo %s: %w", g.ID, err)
		}
	}
	return tx.Commit()
}

type Row struct {
	Date     string
	GoalID   string
	GoalName string
	NAV      float64
}

func (s *Store) History(ctx context.Context, days int) ([]Row, error) {
	since := time.Now().AddDate(0, 0, -days).Format(time.DateOnly)
	rows, err := s.db.QueryContext(ctx, `
		SELECT date, goal_id, goal_name, nav FROM snapshots
		WHERE date >= ? ORDER BY date DESC, goal_name`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.Date, &r.GoalID, &r.GoalName, &r.NAV); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PreviousNAV returns each goal's balance from its latest snapshot before date.
func (s *Store) PreviousNAV(ctx context.Context, date string) (map[string]float64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT goal_id, nav FROM snapshots s
		WHERE date = (SELECT MAX(date) FROM snapshots WHERE goal_id = s.goal_id AND date < ?)`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]float64{}
	for rows.Next() {
		var id string
		var nav float64
		if err := rows.Scan(&id, &nav); err != nil {
			return nil, err
		}
		out[id] = nav
	}
	return out, rows.Err()
}

func (s *Store) SaveBalanceHistory(ctx context.Context, goalID string, points []fintual.BalancePoint) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO balance_history
			(goal_id, date, valuation, cost_basis, unrealized_gain, realized_gain, withdrawn, raw_json, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (goal_id, date) DO UPDATE SET
			valuation = excluded.valuation,
			cost_basis = excluded.cost_basis,
			unrealized_gain = excluded.unrealized_gain,
			realized_gain = excluded.realized_gain,
			withdrawn = excluded.withdrawn,
			raw_json = excluded.raw_json,
			fetched_at = excluded.fetched_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, p := range points {
		if _, err := stmt.ExecContext(ctx, goalID, p.Date, p.Valuation, p.CostBasis,
			p.UnrealizedGain, p.RealizedGain, p.WithdrawnAmount, string(p.Raw), now); err != nil {
			return fmt.Errorf("guardando historial de %s (%s): %w", goalID, p.Date, err)
		}
	}
	return tx.Commit()
}

func (s *Store) BalanceHistory(ctx context.Context, goalID string) ([]fintual.BalancePoint, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT date, valuation, cost_basis, unrealized_gain, realized_gain, withdrawn
		FROM balance_history WHERE goal_id = ? ORDER BY date`, goalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []fintual.BalancePoint
	for rows.Next() {
		var p fintual.BalancePoint
		if err := rows.Scan(&p.Date, &p.Valuation, &p.CostBasis, &p.UnrealizedGain, &p.RealizedGain, &p.WithdrawnAmount); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) HasBalanceHistory(ctx context.Context, goalID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM balance_history WHERE goal_id = ?`, goalID).Scan(&n)
	return n > 0, err
}
