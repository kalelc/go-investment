package fintual

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeFintual mimics the two-step login and the GraphQL API.
func fakeFintual(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		switch r.URL.Path {
		case "/auth/sessions/initiate_login":
			if body["password"] != "secret" {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"message":"Email or password incorrect"}`))
			}
		case "/auth/sessions/finalize_login_web":
			if body["code"] != "123456" {
				w.WriteHeader(http.StatusUnprocessableEntity)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "_session", Value: "abc", Path: "/"})
		case "/gql/":
			if ck, err := r.Cookie("_session"); err != nil || ck.Value != "abc" {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"errors":[{"message":"ApiError::Unauthorized"}]}`))
				return
			}
			query, _ := body["query"].(string)
			switch {
			case strings.Contains(query, "clGoals"):
				w.Write([]byte(`{"data":{"goals":[{"id":"7","name":"Casa"}],"reserves":[],
					"apvAGoal":{"id":"7","name":"APV"},"apvBGoal":null,"apvDcGoal":null,
					"customAllocatedInvestments":[{"id":"9","name":"107"}]}}`))
			case strings.Contains(query, "clGoalLatestAccruedBalance"):
				w.Write([]byte(`{"data":{"balance":{"accruedValuationAmount":"1500000.4","accruedNetDepositsAmount":"1400000"},
					"gain":{"unrealizedGainOrLossAmount":"60000","realizedGainOrLossAmount":"40000.4"}}}`))
			case strings.Contains(query, "clCustomAllocatedInvestmentLatestAccruedBalance"):
				w.Write([]byte(`{"data":{"balance":{"accruedValuationAmount":300000,"accruedNetDepositsAmount":280000},"gain":null}}`))
			case strings.Contains(query, "clApvAGoalLatestAccruedBalance"):
				w.Write([]byte(`{"data":{"balance":{"accruedValuationAmount":820000,"accruedNetDepositsAmount":null},"gain":null}}`))
			default:
				t.Errorf("query inesperada: %s", query)
			}
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestLoginAndGoals(t *testing.T) {
	srv := fakeFintual(t)
	defer srv.Close()
	ctx := context.Background()

	c := New()
	c.BaseURL = srv.URL
	if _, err := c.Goals(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("sin sesión esperaba ErrUnauthorized, obtuve %v", err)
	}
	if err := c.StartLogin(ctx, "a@b.cl", "mala"); err == nil {
		t.Fatal("esperaba error con contraseña incorrecta")
	}
	if err := c.StartLogin(ctx, "a@b.cl", "secret"); err != nil {
		t.Fatal(err)
	}
	if err := c.FinishLogin(ctx, "a@b.cl", "secret", "123456"); err != nil {
		t.Fatal(err)
	}

	goals, err := c.Goals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(goals) != 3 {
		t.Fatalf("esperaba 3 objetivos, obtuve %+v", goals)
	}
	casa, custom, apv := goals[0], goals[1], goals[2]
	if custom.ID != "custom:9" || custom.Name != "107" || custom.NAV != 300000 {
		t.Errorf("inversión personalizada mal parseada: %+v", custom)
	}
	if casa.ID != "goal:7" || casa.NAV != 1500000.4 || casa.Profit == nil || *casa.Profit != 60000 ||
		casa.RealizedProfit == nil || *casa.RealizedProfit != 40000.4 {
		t.Errorf("objetivo Casa mal parseado: %+v", casa)
	}
	if apv.ID != "apv_a:7" || apv.NAV != 820000 || apv.Deposited != nil || apv.Profit != nil {
		t.Errorf("objetivo APV mal parseado: %+v", apv)
	}
}

func TestSessionRestoresFromCookies(t *testing.T) {
	srv := fakeFintual(t)
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
	c.SetCookies([]Cookie{{Name: "_session", Value: "abc"}})
	if _, err := c.Goals(context.Background()); err != nil {
		t.Fatalf("la sesión restaurada debería funcionar: %v", err)
	}
}
