package auth

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Bookmark is a saved load-test configuration ("favorite"). It persists
// independently of whether the test is currently running, and is editable.
type Bookmark struct {
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	Method      string            `json:"method"`
	Concurrence int               `json:"concurrence"`
	Body        string            `json:"body"`
	Headers     map[string]string `json:"headers"`
	Verbose     bool              `json:"verbose"`
	UpdatedAt   int64             `json:"updated_at"`
}

// ListBookmarks returns all saved tests, most-recently-updated first.
func (s *Store) ListBookmarks() ([]Bookmark, error) {
	rows, err := s.db.Query(
		`SELECT name,url,method,concurrence,body,headers,verbose,updated_at FROM bookmarks ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Bookmark{}
	for rows.Next() {
		var b Bookmark
		var headers string
		var verbose int
		if err := rows.Scan(&b.Name, &b.URL, &b.Method, &b.Concurrence, &b.Body, &headers, &verbose, &b.UpdatedAt); err != nil {
			return nil, err
		}
		b.Verbose = verbose != 0
		b.Headers = map[string]string{}
		_ = json.Unmarshal([]byte(headers), &b.Headers)
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetBookmark returns a single saved test, or ErrNotFound.
func (s *Store) GetBookmark(name string) (*Bookmark, error) {
	var b Bookmark
	var headers string
	var verbose int
	err := s.db.QueryRow(
		`SELECT name,url,method,concurrence,body,headers,verbose,updated_at FROM bookmarks WHERE name=?`, name).
		Scan(&b.Name, &b.URL, &b.Method, &b.Concurrence, &b.Body, &headers, &verbose, &b.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	b.Verbose = verbose != 0
	b.Headers = map[string]string{}
	_ = json.Unmarshal([]byte(headers), &b.Headers)
	return &b, nil
}

// SaveBookmark upserts a saved test by name (create on first favorite, update on edit).
func (s *Store) SaveBookmark(b *Bookmark) error {
	hb, _ := json.Marshal(b.Headers)
	if len(hb) == 0 {
		hb = []byte("{}")
	}
	v := 0
	if b.Verbose {
		v = 1
	}
	now := time.Now().Unix()
	_, err := s.db.Exec(`
INSERT INTO bookmarks(name,url,method,concurrence,body,headers,verbose,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?)
ON CONFLICT(name) DO UPDATE SET
  url=excluded.url, method=excluded.method, concurrence=excluded.concurrence,
  body=excluded.body, headers=excluded.headers, verbose=excluded.verbose, updated_at=excluded.updated_at`,
		b.Name, b.URL, b.Method, b.Concurrence, b.Body, string(hb), v, now, now)
	return err
}

// DeleteBookmark removes a saved test (un-favorite).
func (s *Store) DeleteBookmark(name string) error {
	_, err := s.db.Exec(`DELETE FROM bookmarks WHERE name=?`, name)
	return err
}
