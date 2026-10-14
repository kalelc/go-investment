package fintual

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Intervals are the timeIntervalCode values used by the web chart, longest first.
var Intervals = []string{"all_time", "last_three_years", "last_year", "last_six_months", "last_three_months", "last_month"}

type BalancePoint struct {
	Date            string // YYYY-MM-DD
	Valuation       float64
	CostBasis       float64 // cost of the shares currently held
	UnrealizedGain  float64
	RealizedGain    float64
	WithdrawnAmount float64
	Raw             json.RawMessage
}

// BalanceHistory takes a prefixed Goal.ID and one of Intervals.
func (c *Client) BalanceHistory(ctx context.Context, goalID, interval string) ([]BalancePoint, error) {
	k, id, err := parseGoalID(goalID)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`query GoinvBalanceHistory($id: ID!, $interval: String!) {
  points: %s(%s: $id, timeIntervalCode: $interval) {
    date
    sharesValuationAmount
    sharesCostBasisAmount
    unrealizedGainOrLossAmount
    realizedGainOrLossAmount
    withdrawnAmount
  }
}`, k.graph, k.arg)

	var resp struct {
		Points []json.RawMessage `json:"points"`
	}
	vars := map[string]any{"id": id, "interval": interval}
	if err := c.Query(ctx, "GoinvBalanceHistory", query, vars, &resp); err != nil {
		return nil, fmt.Errorf("historial de %s (%s): %w", goalID, interval, err)
	}

	points := make([]BalancePoint, 0, len(resp.Points))
	for _, raw := range resp.Points {
		var p struct {
			Date       string `json:"date"`
			Valuation  amount `json:"sharesValuationAmount"`
			CostBasis  amount `json:"sharesCostBasisAmount"`
			Unrealized amount `json:"unrealizedGainOrLossAmount"`
			Realized   amount `json:"realizedGainOrLossAmount"`
			Withdrawn  amount `json:"withdrawnAmount"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("historial de %s: %w", goalID, err)
		}
		if len(p.Date) < 10 {
			return nil, fmt.Errorf("historial de %s: fecha inválida %q", goalID, p.Date)
		}
		points = append(points, BalancePoint{
			Date:            p.Date[:10],
			Valuation:       p.Valuation.Value,
			CostBasis:       p.CostBasis.Value,
			UnrealizedGain:  p.Unrealized.Value,
			RealizedGain:    p.Realized.Value,
			WithdrawnAmount: p.Withdrawn.Value,
			Raw:             raw,
		})
	}
	return points, nil
}

func parseGoalID(goalID string) (kind, string, error) {
	prefix, id, ok := strings.Cut(goalID, ":")
	if ok {
		for _, k := range kinds {
			if k.prefix == prefix {
				return k, id, nil
			}
		}
	}
	return kind{}, "", fmt.Errorf("id de objetivo inválido: %q", goalID)
}
