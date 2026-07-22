package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials is returned for any authentication failure (kept generic
// so callers don't leak whether the user exists).
var ErrInvalidCredentials = errors.New("invalid credentials")

// Provider authenticates a set of credentials and returns the matching User.
// This is the extension seam: LocalProvider (password) today; a GoogleProvider
// (verify an OAuth id_token, upsert by provider+subject) can be added later and
// registered alongside it — the session/cookie/middleware layers never change.
type Provider interface {
	Name() string
	Authenticate(creds map[string]string) (*User, error)
}

// LocalProvider checks a username/password against bcrypt hashes in the store.
type LocalProvider struct{ store *Store }

// Name identifies this provider ("local").
func (p *LocalProvider) Name() string { return "local" }

// Authenticate expects creds["username"] (the login identifier) and
// creds["password"]. Returns ErrInvalidCredentials on any mismatch.
func (p *LocalProvider) Authenticate(creds map[string]string) (*User, error) {
	u, err := p.store.GetUserByEmail(creds["username"])
	if err != nil || u == nil || u.Provider != "local" {
		return nil, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(creds["password"])) != nil {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}
