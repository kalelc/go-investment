// Package fintual talks to the private API behind fintual.cl: the emailed-code
// login and the GraphQL endpoint (/gql/). It is not a public API and may change.
package fintual

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
	"unicode"
)

const DefaultBaseURL = "https://fintual.cl"

var ErrUnauthorized = errors.New("sesión de Fintual inválida o expirada")

type Client struct {
	BaseURL string
	HTTP    *http.Client
	// Debug receives response details without secrets.
	Debug io.Writer
}

// maxResponseBytes caps how much of a response is read into memory.
var maxResponseBytes int64 = 32 << 20

func New() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		BaseURL: DefaultBaseURL,
		HTTP: &http.Client{
			Timeout:       30 * time.Second,
			Jar:           jar,
			CheckRedirect: sameHostRedirects,
		},
	}
}

// sameHostRedirects stops redirects to other hosts: on 307/308 Go resends the
// request body, which for the login endpoints includes the password.
func sameHostRedirects(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("demasiadas redirecciones")
	}
	if req.URL.Host != via[0].URL.Host {
		return fmt.Errorf("redirección a otro dominio bloqueada: %s", req.URL.Host)
	}
	return nil
}

type Cookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func (c *Client) Cookies() []Cookie {
	var out []Cookie
	for _, ck := range c.HTTP.Jar.Cookies(c.base()) {
		out = append(out, Cookie{Name: ck.Name, Value: ck.Value})
	}
	return out
}

func (c *Client) SetCookies(cookies []Cookie) {
	hc := make([]*http.Cookie, 0, len(cookies))
	for _, ck := range cookies {
		hc = append(hc, &http.Cookie{Name: ck.Name, Value: ck.Value, Path: "/"})
	}
	c.HTTP.Jar.SetCookies(c.base(), hc)
}

// StartLogin makes Fintual email a verification code.
func (c *Client) StartLogin(ctx context.Context, email, password string) error {
	_, err := c.postJSON(ctx, "/auth/sessions/initiate_login", map[string]string{
		"email": email, "password": password,
	})
	if err != nil {
		return fmt.Errorf("iniciando login: %w", err)
	}
	return nil
}

func (c *Client) FinishLogin(ctx context.Context, email, password, code string) error {
	_, err := c.postJSON(ctx, "/auth/sessions/finalize_login_web", map[string]string{
		"email": email, "password": password, "code": code,
	})
	if err != nil {
		return fmt.Errorf("verificando código: %w", err)
	}
	if len(c.Cookies()) == 0 {
		return errors.New("verificando código: Fintual no devolvió cookies de sesión")
	}
	return nil
}

func (c *Client) Query(ctx context.Context, operation, query string, vars map[string]any, out any) error {
	raw, err := c.postJSON(ctx, "/gql/", map[string]any{
		"operationName": operation,
		"query":         query,
		"variables":     vars,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	var resp struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("%s: respuesta inesperada: %s", operation, truncate(raw))
	}
	if len(resp.Errors) > 0 {
		if resp.Errors[0].Message == "ApiError::Unauthorized" {
			return ErrUnauthorized
		}
		return fmt.Errorf("%s: %s", operation, sanitize(resp.Errors[0].Message))
	}
	return json.Unmarshal(resp.Data, out)
}

type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Status, e.Body) }

func (c *Client) postJSON(ctx context.Context, path string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxResponseBytes {
		return nil, fmt.Errorf("respuesta de %s demasiado grande (más de %d bytes)", path, maxResponseBytes)
	}
	c.debugf("POST %s → HTTP %d, cookies en sesión: %d", path, res.StatusCode, len(c.Cookies()))
	if res.StatusCode == http.StatusUnauthorized && path == "/gql/" {
		return nil, ErrUnauthorized
	}
	if res.StatusCode >= 300 {
		return nil, &HTTPError{Status: res.StatusCode, Body: truncate(raw)}
	}
	return raw, nil
}

func (c *Client) base() *url.URL {
	u, _ := url.Parse(c.BaseURL + "/")
	return u
}

func (c *Client) debugf(format string, args ...any) {
	if c.Debug != nil {
		fmt.Fprintf(c.Debug, "[debug] "+format+"\n", args...)
	}
}

func truncate(b []byte) string {
	s := sanitize(string(b))
	if r := []rune(s); len(r) > 300 {
		return string(r[:300]) + "…"
	}
	return s
}

// sanitize drops control characters from server-provided text so it can't
// inject terminal escape sequences when displayed.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
