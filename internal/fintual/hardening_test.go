package fintual

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginDoesNotFollowRedirectsToOtherHosts(t *testing.T) {
	var leaked string
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		leaked = string(b)
	}))
	defer evil.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
	err := c.StartLogin(context.Background(), "a@b.cl", "secret")
	if err == nil || !strings.Contains(err.Error(), "bloqueada") {
		t.Fatalf("esperaba bloquear la redirección, obtuve %v", err)
	}
	if strings.Contains(leaked, "secret") {
		t.Fatal("la contraseña llegó a otro dominio")
	}
}

func TestSameHostRedirectsAreFollowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/sessions/initiate_login" {
			http.Redirect(w, r, "/auth/sessions/initiate_login/", http.StatusPermanentRedirect)
		}
	}))
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
	if err := c.StartLogin(context.Background(), "a@b.cl", "secret"); err != nil {
		t.Fatalf("una redirección al mismo dominio debería funcionar: %v", err)
	}
}

func TestResponseSizeIsLimited(t *testing.T) {
	defer func(old int64) { maxResponseBytes = old }(maxResponseBytes)
	maxResponseBytes = 10

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("x", 11)))
	}))
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
	err := c.StartLogin(context.Background(), "a@b.cl", "secret")
	if err == nil || !strings.Contains(err.Error(), "demasiado grande") {
		t.Fatalf("esperaba rechazar la respuesta grande, obtuve %v", err)
	}
}

func TestSanitizeDropsControlCharacters(t *testing.T) {
	got := sanitize("💰 108\x1b[2J\x1b]0;hack\x07 - Ahorros\n")
	if got != "💰 108[2J]0;hack - Ahorros" {
		t.Fatalf("sanitize: %q", got)
	}
}
