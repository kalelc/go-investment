package fintual

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

type Goal struct {
	ID        string // "<kind>:<id>": ids repeat across goal kinds
	Name      string
	NAV       float64
	Deposited *float64 // net of withdrawals
	// Profit is the unrealized gain, the figure Fintual shows as "ganancia".
	Profit         *float64
	RealizedProfit *float64
	Raw            json.RawMessage
}

const objectsQuery = `query GoinvObjects {
  goals: clGoals { id name }
  reserves: clReserves { id name }
  apvAGoal: clApvAGoal { id name }
  apvBGoal: clApvBGoal { id name }
  apvDcGoal: clApvDcGoal { id name }
  customAllocatedInvestments: clCustomAllocatedInvestments { id name }
}`

type kind struct {
	prefix string
	field  string // latest balance field
	gain   string // gain breakdown field
	graph  string // balance history field
	arg    string
}

var (
	kindGoal    = kind{"goal", "clGoalLatestAccruedBalance", "clGoalGainOrLossBreakdown", "clGoalBalanceGraphDataPoints", "goalId"}
	kindReserve = kind{"reserve", "clReserveLatestAccruedBalance", "clReserveGainOrLossBreakdown", "clReserveBalanceGraphDataPoints", "reserveId"}
	kindApvA    = kind{"apv_a", "clApvAGoalLatestAccruedBalance", "clApvAGoalGainOrLossBreakdown", "clApvAGoalBalanceGraphDataPoints", "apvAGoalId"}
	kindApvB    = kind{"apv_b", "clApvBGoalLatestAccruedBalance", "clApvBGoalGainOrLossBreakdown", "clApvBGoalBalanceGraphDataPoints", "apvBGoalId"}
	kindApvDc   = kind{"apv_dc", "clApvDcGoalLatestAccruedBalance", "clApvDcGoalGainOrLossBreakdown", "clApvDcGoalBalanceGraphDataPoints", "apvDcGoalId"}

	kindCustom = kind{"custom", "clCustomAllocatedInvestmentLatestAccruedBalance", "clCustomAllocatedInvestmentGainOrLossBreakdown", "clCustomAllocatedInvestmentBalanceGraphDataPoints", "customAllocatedInvestmentId"}

	kinds = []kind{kindGoal, kindReserve, kindApvA, kindApvB, kindApvDc, kindCustom}
)

type object struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Client) Goals(ctx context.Context) ([]Goal, error) {
	var objs struct {
		Goals     []object `json:"goals"`
		Reserves  []object `json:"reserves"`
		ApvAGoal  *object  `json:"apvAGoal"`
		ApvBGoal  *object  `json:"apvBGoal"`
		ApvDcGoal *object  `json:"apvDcGoal"`
		Custom    []object `json:"customAllocatedInvestments"`
	}
	if err := c.Query(ctx, "GoinvObjects", objectsQuery, nil, &objs); err != nil {
		return nil, err
	}

	type item struct {
		kind kind
		obj  object
	}
	var items []item
	for _, o := range objs.Goals {
		items = append(items, item{kindGoal, o})
	}
	for _, o := range objs.Reserves {
		items = append(items, item{kindReserve, o})
	}
	for _, o := range objs.Custom {
		items = append(items, item{kindCustom, o})
	}
	for _, apv := range []struct {
		kind kind
		obj  *object
	}{{kindApvA, objs.ApvAGoal}, {kindApvB, objs.ApvBGoal}, {kindApvDc, objs.ApvDcGoal}} {
		if apv.obj != nil {
			items = append(items, item{apv.kind, *apv.obj})
		}
	}

	goals := make([]Goal, 0, len(items))
	for _, it := range items {
		g, err := c.balance(ctx, it.kind, it.obj)
		if err != nil {
			return nil, err
		}
		goals = append(goals, g)
	}
	return goals, nil
}

func (c *Client) balance(ctx context.Context, k kind, o object) (Goal, error) {
	query := fmt.Sprintf(`query GoinvBalance($id: ID!) {
  balance: %[1]s(%[3]s: $id) {
    calculatedAt
    accruedValuationAmount
    accruedNetDepositsAmount
  }
  gain: %[2]s(%[3]s: $id) {
    date
    unrealizedGainOrLossAmount
    realizedGainOrLossAmount
  }
}`, k.field, k.gain, k.arg)

	var raw json.RawMessage
	if err := c.Query(ctx, "GoinvBalance", query, map[string]any{"id": o.ID}, &raw); err != nil {
		return Goal{}, fmt.Errorf("saldo de %q: %w", o.Name, err)
	}
	var resp struct {
		Balance *struct {
			Valuation   amount `json:"accruedValuationAmount"`
			NetDeposits amount `json:"accruedNetDepositsAmount"`
		} `json:"balance"`
		Gain *struct {
			Unrealized amount `json:"unrealizedGainOrLossAmount"`
			Realized   amount `json:"realizedGainOrLossAmount"`
		} `json:"gain"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return Goal{}, fmt.Errorf("saldo de %q: %w", o.Name, err)
	}

	g := Goal{ID: k.prefix + ":" + o.ID, Name: sanitize(o.Name), Raw: raw}
	if b := resp.Balance; b != nil {
		g.NAV = b.Valuation.Value
		g.Deposited = b.NetDeposits.ptr()
	}
	if gain := resp.Gain; gain != nil {
		g.Profit, g.RealizedProfit = gain.Unrealized.ptr(), gain.Realized.ptr()
	}
	return g, nil
}

// amount accepts numbers or strings: GraphQL decimal scalars often come as strings.
type amount struct {
	Value float64
	Valid bool
}

func (a amount) ptr() *float64 {
	if !a.Valid {
		return nil
	}
	return &a.Value
}

func (a *amount) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		b = []byte(s)
	}
	v, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return fmt.Errorf("monto inválido %s", b)
	}
	a.Value, a.Valid = v, true
	return nil
}
