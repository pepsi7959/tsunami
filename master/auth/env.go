package auth

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Env is a named set of key/value variables ("environment preset"). Its values
// are substituted into a test's URL, headers and body via {{key}} tokens at
// start time. It persists independently of any test and is shared across users
// of this ocean.
type Env struct {
	Name      string            `json:"name"`
	Vars      map[string]string `json:"vars"`
	UpdatedAt int64             `json:"updated_at"`
}

// ListEnvs returns all environments, most-recently-updated first.
func (s *Store) ListEnvs() ([]Env, error) {
	rows, err := s.db.Query(`SELECT name,vars,updated_at FROM envs ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Env{}
	for rows.Next() {
		var e Env
		var vars string
		if err := rows.Scan(&e.Name, &vars, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.Vars = map[string]string{}
		_ = json.Unmarshal([]byte(vars), &e.Vars)
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetEnv returns a single environment, or ErrNotFound.
func (s *Store) GetEnv(name string) (*Env, error) {
	var e Env
	var vars string
	err := s.db.QueryRow(`SELECT name,vars,updated_at FROM envs WHERE name=?`, name).
		Scan(&e.Name, &vars, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	e.Vars = map[string]string{}
	_ = json.Unmarshal([]byte(vars), &e.Vars)
	return &e, nil
}

// SaveEnv upserts an environment by name (create on first save, update on edit).
func (s *Store) SaveEnv(e *Env) error {
	vb, _ := json.Marshal(e.Vars)
	if len(vb) == 0 {
		vb = []byte("{}")
	}
	now := time.Now().Unix()
	_, err := s.db.Exec(`
INSERT INTO envs(name,vars,created_at,updated_at) VALUES(?,?,?,?)
ON CONFLICT(name) DO UPDATE SET vars=excluded.vars, updated_at=excluded.updated_at`,
		e.Name, string(vb), now, now)
	return err
}

// DeleteEnv removes an environment.
func (s *Store) DeleteEnv(name string) error {
	_, err := s.db.Exec(`DELETE FROM envs WHERE name=?`, name)
	return err
}
