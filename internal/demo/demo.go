package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/kalelc/go-investment/internal/app"
	"github.com/kalelc/go-investment/internal/fintual"
	"github.com/kalelc/go-investment/internal/session"
	"github.com/kalelc/go-investment/internal/store"
)

const cookieName = "_demo_session"

// Start prepares an isolated environment: an in-memory keychain holding a
// fake session, a temporary database with yesterday's snapshot and a local
// fake Fintual server. Nothing touches the real keychain, database or Fintual.
func Start(ctx context.Context) (*app.App, func(), error) {
	keyring.MockInit()

	today := time.Now()
	data := generate(today)
	srv := httptest.NewServer(handler(data))

	dir, err := os.MkdirTemp("", "goinv-demo-*")
	if err != nil {
		srv.Close()
		return nil, nil, err
	}
	st, err := store.Open(filepath.Join(dir, "demo.db"))
	if err != nil {
		srv.Close()
		os.RemoveAll(dir)
		return nil, nil, err
	}
	cleanup := func() {
		st.Close()
		srv.Close()
		os.RemoveAll(dir)
	}

	var yesterday []fintual.Goal
	for _, s := range data {
		p := s.points[len(s.points)-2]
		yesterday = append(yesterday, fintual.Goal{ID: s.spec.id, Name: s.spec.name, NAV: p.Valuation, Raw: []byte(`{}`)})
	}
	if err := st.SaveSnapshot(ctx, today.AddDate(0, 0, -1), yesterday); err != nil {
		cleanup()
		return nil, nil, err
	}

	now := time.Now()
	err = session.Save(&session.Session{
		Email:      "demo@ejemplo.cl",
		Cookies:    []fintual.Cookie{{Name: cookieName, Value: "demo"}},
		LoggedInAt: now,
		LastUsedAt: now,
	})
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	return &app.App{Store: st, BaseURL: srv.URL, Demo: true}, cleanup, nil
}

func handler(data []series) http.Handler {
	byID := map[string]series{}
	for _, s := range data {
		byID[s.spec.id] = s
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/sessions/initiate_login", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/auth/sessions/finalize_login_web", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "demo", Path: "/"})
		w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/gql/", func(w http.ResponseWriter, r *http.Request) {
		if ck, err := r.Cookie(cookieName); err != nil || ck.Value != "demo" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"errors":[{"message":"ApiError::Unauthorized"}]}`))
			return
		}
		var req struct {
			OperationName string         `json:"operationName"`
			Query         string         `json:"query"`
			Variables     map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var resp any
		switch req.OperationName {
		case "GoinvObjects":
			resp = objects(data)
		case "GoinvBalance":
			s, ok := byID[goalID(req.Query, req.Variables)]
			if !ok {
				resp = map[string]any{"balance": nil, "gain": nil}
				break
			}
			last := s.points[len(s.points)-1]
			resp = map[string]any{
				"balance": map[string]any{
					"calculatedAt":             time.Now().Format(time.RFC3339),
					"accruedValuationAmount":   last.Valuation,
					"accruedNetDepositsAmount": s.netDeposits,
				},
				"gain": map[string]any{
					"date":                       last.Date,
					"unrealizedGainOrLossAmount": last.UnrealizedGain,
					"realizedGainOrLossAmount":   last.RealizedGain,
				},
			}
		case "GoinvBalanceHistory":
			interval, _ := req.Variables["interval"].(string)
			var points []map[string]any
			for _, p := range byID[goalID(req.Query, req.Variables)].interval(interval) {
				points = append(points, map[string]any{
					"date":                       p.Date,
					"sharesValuationAmount":      p.Valuation,
					"sharesCostBasisAmount":      p.CostBasis,
					"unrealizedGainOrLossAmount": p.UnrealizedGain,
					"realizedGainOrLossAmount":   p.RealizedGain,
					"withdrawnAmount":            p.WithdrawnAmount,
				})
			}
			resp = map[string]any{"points": points}
		default:
			json.NewEncoder(w).Encode(map[string]any{
				"errors": []map[string]string{{"message": "demo: operación no soportada " + req.OperationName}},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"data": resp})
	})
	return mux
}

func objects(data []series) map[string]any {
	resp := map[string]any{
		"goals": []any{}, "reserves": []any{}, "customAllocatedInvestments": []any{},
		"apvAGoal": nil, "apvBGoal": nil, "apvDcGoal": nil,
	}
	for _, s := range data {
		prefix, id, _ := strings.Cut(s.spec.id, ":")
		obj := map[string]string{"id": id, "name": s.spec.name}
		switch prefix {
		case "goal":
			resp["goals"] = append(resp["goals"].([]any), obj)
		case "custom":
			resp["customAllocatedInvestments"] = append(resp["customAllocatedInvestments"].([]any), obj)
		case "apv_b":
			resp["apvBGoal"] = obj
		}
	}
	return resp
}

// goalID rebuilds the app's "<kind>:<id>" from the GraphQL field being queried.
func goalID(query string, vars map[string]any) string {
	prefixes := []struct{ field, prefix string }{
		{"clApvAGoal", "apv_a"}, {"clApvBGoal", "apv_b"}, {"clApvDcGoal", "apv_dc"},
		{"clCustomAllocatedInvestment", "custom"}, {"clReserve", "reserve"}, {"clGoal", "goal"},
	}
	for _, p := range prefixes {
		if strings.Contains(query, p.field) {
			return p.prefix + ":" + fmt.Sprint(vars["id"])
		}
	}
	return ""
}
