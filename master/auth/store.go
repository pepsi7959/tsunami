// Package auth provides user login/logout for the ocean HTTP API: SQLite-backed
// credentials + server-side sessions with a hard TTL. The session and middleware
// layers are identity-provider-agnostic so a Google (OAuth) provider can be added
// later without touching them — see provider.go.
package auth

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go driver (CGO_ENABLED=0 safe); registers "sqlite"
)

// ErrNotFound is returned when a user/session row does not exist.
var ErrNotFound = errors.New("not found")

// User is an account. auth_provider distinguishes how the user authenticates
// ("local" password now, "google" later); password_hash is set only for local,
// provider_subject only for external providers.
type User struct {
	ID              int64
	Email           string // login identifier (an email, or "admin" for the seeded local admin)
	Name            string
	Provider        string // "local" | "google" | ...
	PasswordHash    string // bcrypt; local only
	ProviderSubject string // external provider subject id; external only
	CreatedAt       int64
}

// Store is the SQLite persistence layer for users + sessions.
type Store struct{ db *sql.DB }

// Open opens (creating parent dirs + schema as needed) the SQLite auth database.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// single writer; keep the pool small and enable WAL for concurrent reads
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;`); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS users (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  email          TEXT NOT NULL UNIQUE,
  name           TEXT NOT NULL DEFAULT '',
  auth_provider  TEXT NOT NULL DEFAULT 'local',
  password_hash  TEXT NOT NULL DEFAULT '',
  provider_subject TEXT NOT NULL DEFAULT '',
  created_at     INTEGER NOT NULL,
  UNIQUE(auth_provider, provider_subject)
);
CREATE TABLE IF NOT EXISTS sessions (
  token_hash TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  ip         TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_sessions_user    ON sessions(user_id);`)
	return err
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// CountUsers returns the number of user rows (used for idempotent admin seeding).
func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateUser inserts a user; u.ID is populated on success.
func (s *Store) CreateUser(u *User) error {
	res, err := s.db.Exec(
		`INSERT INTO users(email,name,auth_provider,password_hash,provider_subject,created_at) VALUES(?,?,?,?,?,?)`,
		u.Email, u.Name, u.Provider, u.PasswordHash, u.ProviderSubject, u.CreatedAt)
	if err != nil {
		return err
	}
	u.ID, _ = res.LastInsertId()
	return nil
}

// GetUserByEmail returns the user with the given login identifier, or ErrNotFound.
func (s *Store) GetUserByEmail(email string) (*User, error) {
	u := &User{}
	err := s.db.QueryRow(
		`SELECT id,email,name,auth_provider,password_hash,provider_subject,created_at FROM users WHERE email=?`, email).
		Scan(&u.ID, &u.Email, &u.Name, &u.Provider, &u.PasswordHash, &u.ProviderSubject, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// CreateSession stores a session row (token already hashed by the caller).
func (s *Store) CreateSession(tokenHash string, userID, createdAt, expiresAt int64, ip, ua string) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions(token_hash,user_id,created_at,expires_at,ip,user_agent) VALUES(?,?,?,?,?,?)`,
		tokenHash, userID, createdAt, expiresAt, ip, ua)
	return err
}

// GetValidSession returns the user for a non-expired session, or ErrNotFound.
func (s *Store) GetValidSession(tokenHash string, now int64) (*User, error) {
	u := &User{}
	err := s.db.QueryRow(
		`SELECT u.id,u.email,u.name,u.auth_provider,u.password_hash,u.provider_subject,u.created_at
		   FROM sessions s JOIN users u ON u.id = s.user_id
		  WHERE s.token_hash=? AND s.expires_at > ?`, tokenHash, now).
		Scan(&u.ID, &u.Email, &u.Name, &u.Provider, &u.PasswordHash, &u.ProviderSubject, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// DeleteSession removes a single session (logout).
func (s *Store) DeleteSession(tokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash=?`, tokenHash)
	return err
}

// DeleteExpired purges sessions past their expiry.
func (s *Store) DeleteExpired(now int64) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, now)
	return err
}
