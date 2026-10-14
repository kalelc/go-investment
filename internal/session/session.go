// Package session stores the Fintual session in the OS keychain.
package session

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/kalelc/go-investment/internal/fintual"
)

const (
	service = "goinv"
	account = "fintual-session"
)

var ErrNotFound = errors.New("no hay sesión guardada")

type Session struct {
	Email      string           `json:"email"`
	Cookies    []fintual.Cookie `json:"cookies"`
	LoggedInAt time.Time        `json:"logged_in_at"`
	LastUsedAt time.Time        `json:"last_used_at"`
}

func Load() (*Session, error) {
	data, err := keyring.Get(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal([]byte(data), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func Save(s *Session) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return keyring.Set(service, account, string(data))
}

func Delete() error {
	err := keyring.Delete(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
