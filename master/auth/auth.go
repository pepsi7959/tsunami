package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"log"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Auth ties together the store, the identity providers, session issuance and
// the cookie policy. It is provider-agnostic: sessions reference a user id, not
// how the user proved their identity.
type Auth struct {
	store        *Store
	providers    map[string]Provider
	ttl          time.Duration
	cookieName   string
	cookieSecure bool
}

// New builds an Auth with the built-in local (password) provider registered.
func New(store *Store, ttl time.Duration, cookieSecure bool) *Auth {
	a := &Auth{
		store:        store,
		providers:    map[string]Provider{},
		ttl:          ttl,
		cookieName:   "tsu_session",
		cookieSecure: cookieSecure,
	}
	a.providers["local"] = &LocalProvider{store: store}
	// future: a.providers["google"] = &GoogleProvider{store: store, ...}
	return a
}

// Provider returns the named provider, or nil.
func (a *Auth) Provider(name string) Provider { return a.providers[name] }

// CookieName is the session cookie name.
func (a *Auth) CookieName() string { return a.cookieName }

// Store exposes the underlying store (for periodic cleanup, etc.).
func (a *Auth) Store() *Store { return a.store }

func genToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Issue creates a new session for the user and returns the raw token + expiry.
// Only the token's SHA-256 is stored, so a DB leak can't resurrect live sessions.
func (a *Auth) Issue(u *User, ip, ua string) (string, time.Time, error) {
	raw, err := genToken()
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now()
	exp := now.Add(a.ttl)
	if err := a.store.CreateSession(hashToken(raw), u.ID, now.Unix(), exp.Unix(), ip, ua); err != nil {
		return "", time.Time{}, err
	}
	return raw, exp, nil
}

// Validate returns the user for a live (non-expired) session token.
func (a *Auth) Validate(raw string) (*User, bool) {
	if raw == "" {
		return nil, false
	}
	u, err := a.store.GetValidSession(hashToken(raw), time.Now().Unix())
	if err != nil || u == nil {
		return nil, false
	}
	return u, true
}

// Revoke deletes the session for a token (logout).
func (a *Auth) Revoke(raw string) {
	if raw != "" {
		_ = a.store.DeleteSession(hashToken(raw))
	}
}

// Cookie builds the session Set-Cookie for a freshly issued token.
func (a *Auth) Cookie(raw string, exp time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     a.cookieName,
		Value:    raw,
		Path:     "/",
		Expires:  exp,
		MaxAge:   int(a.ttl / time.Second),
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
}

// ClearCookie builds the Set-Cookie that removes the session (logout).
func (a *Auth) ClearCookie() *http.Cookie {
	return &http.Cookie{
		Name:     a.cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
}

// SeedAdmin creates the initial local admin if there are no users yet. Idempotent
// (safe to call on every startup). A default "admin" password triggers a loud warning.
func (a *Auth) SeedAdmin(username, password string) error {
	n, err := a.store.CountUsers()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if username == "" {
		username = "admin"
	}
	if password == "" {
		password = "admin"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := a.store.CreateUser(&User{
		Email:     username,
		Name:      username,
		Provider:  "local",
		PasswordHash: string(hash),
		CreatedAt: time.Now().Unix(),
	}); err != nil {
		return err
	}
	if password == "admin" {
		log.Printf("[auth] WARNING: seeded admin %q with the DEFAULT password 'admin' — set ADMIN_PASS before exposing this ocean", username)
	} else {
		log.Printf("[auth] seeded initial admin user %q", username)
	}
	return nil
}
