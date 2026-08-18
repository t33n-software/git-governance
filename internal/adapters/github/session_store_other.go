//go:build !windows && !darwin && !linux

package github

import (
	"context"
	"errors"
)

type unavailableSessionStore struct{}

func newPlatformSessionStore() SessionStore {
	return unavailableSessionStore{}
}

func (unavailableSessionStore) LoadActive(context.Context, string, string) (Session, error) {
	return Session{}, errors.New("no supported native GitHub App secret store is available on this platform")
}

func (unavailableSessionStore) LoadActiveForHost(context.Context, string) (Session, error) {
	return Session{}, errors.New("no supported native GitHub App secret store is available on this platform")
}

func (unavailableSessionStore) LoadActiveForRepository(context.Context, string, string, string) (Session, error) {
	return Session{}, errors.New("no supported native GitHub App secret store is available on this platform")
}

func (unavailableSessionStore) ListForHost(context.Context, string) ([]Session, error) {
	return nil, errors.New("no supported native GitHub App secret store is available on this platform")
}

func (unavailableSessionStore) BindRepository(context.Context, string, string, string, string) error {
	return errors.New("no supported native GitHub App secret store is available on this platform")
}

func (unavailableSessionStore) SaveActive(context.Context, Session) error {
	return errors.New("no supported native GitHub App secret store is available on this platform")
}

func (unavailableSessionStore) DeleteActive(context.Context, string, string) error {
	return errors.New("no supported native GitHub App secret store is available on this platform")
}
