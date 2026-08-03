// Package blog is the example app's shared code: SQLite storage, the
// template tree, and the view models every action renders.
//
// It exists because actions/ cannot hold shared code. The generator
// copies only <name>.<VERB>.go files into gen/actions/ and skips any file
// whose base name starts with "_", so a sibling helpers.go never reaches
// the generated tree. Ordinary packages travel fine: imports pass through
// the generator's package-clause rewrite unmodified.
package blog

import (
	"database/sql"
	"strings"
	"time"

	_ "modernc.org/sqlite" // the app opens its own handle; see Open
)

// migration is the app's whole schema: one additive, idempotent
// statement, applied at startup by Open.
const migration = `
CREATE TABLE IF NOT EXISTS posts (
  id         INTEGER PRIMARY KEY,
  title      TEXT    NOT NULL,
  body       TEXT    NOT NULL,
  published  INTEGER NOT NULL DEFAULT 0,
  created_at TEXT    NOT NULL,
  updated_at TEXT    NOT NULL
);`

// Post is one row of posts. Timestamps are stored as RFC3339 strings in
// UTC and parsed on scan, so formatting happens once, in Go, and no
// template ever formats a date.
type Post struct {
	ID        int64
	Title     string
	Body      string
	Published bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Open opens the app's SQLite database and applies the migration.
//
// The DSN reproduces rastrillo's own openDB by hand — busy_timeout before
// journal_mode=WAL, then SetMaxOpenConns(1) — because rastrillo.Serve
// never hands its opened *sql.DB back to the app, and every action here
// reads the database through Ctx.DB. See the README's friction log, F4.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(migration); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

const selectColumns = `id, title, body, published, created_at, updated_at`

// scanPost reads one row in selectColumns order, parsing both timestamps.
func scanPost(row interface{ Scan(...any) error }) (Post, error) {
	var (
		p         Post
		published int
		created   string
		updated   string
	)
	if err := row.Scan(&p.ID, &p.Title, &p.Body, &published, &created, &updated); err != nil {
		return Post{}, err
	}
	p.Published = published != 0
	var err error
	if p.CreatedAt, err = time.Parse(time.RFC3339, created); err != nil {
		return Post{}, err
	}
	if p.UpdatedAt, err = time.Parse(time.RFC3339, updated); err != nil {
		return Post{}, err
	}
	return p, nil
}

// Get returns one post by id. A missing id is sql.ErrNoRows, which every
// action turns into a 404.
func Get(db *sql.DB, id int64) (Post, error) {
	return scanPost(db.QueryRow(`SELECT `+selectColumns+` FROM posts WHERE id = ?`, id))
}

// ListPublished returns published posts, newest first.
func ListPublished(db *sql.DB, offset, limit int) ([]Post, error) {
	rows, err := db.Query(`SELECT `+selectColumns+` FROM posts WHERE published = 1
		ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	return collect(rows)
}

// CountPublished counts published posts.
func CountPublished(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM posts WHERE published = 1`).Scan(&n)
	return n, err
}

// List returns posts of any status, newest first, filtered by a title
// search when q is non-empty.
func List(db *sql.DB, q string, offset, limit int) ([]Post, error) {
	if q == "" {
		rows, err := db.Query(`SELECT `+selectColumns+` FROM posts
			ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, limit, offset)
		if err != nil {
			return nil, err
		}
		return collect(rows)
	}
	rows, err := db.Query(`SELECT `+selectColumns+` FROM posts WHERE title LIKE ? ESCAPE '\'
		ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, likePattern(q), limit, offset)
	if err != nil {
		return nil, err
	}
	return collect(rows)
}

// Count counts the posts List would page through.
func Count(db *sql.DB, q string) (int, error) {
	var n int
	var err error
	if q == "" {
		err = db.QueryRow(`SELECT COUNT(*) FROM posts`).Scan(&n)
	} else {
		err = db.QueryRow(`SELECT COUNT(*) FROM posts WHERE title LIKE ? ESCAPE '\'`, likePattern(q)).Scan(&n)
	}
	return n, err
}

// Create inserts a draft and returns its id.
func Create(db *sql.DB, title, body string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO posts (title, body, published, created_at, updated_at)
		VALUES (?, ?, 0, ?, ?)`, title, body, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Update rewrites a post's title and body and moves updated_at.
func Update(db *sql.DB, id int64, title, body string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`UPDATE posts SET title = ?, body = ?, updated_at = ? WHERE id = ?`,
		title, body, now, id)
	return err
}

// SetPublished flips a post's published flag and moves updated_at.
func SetPublished(db *sql.DB, id int64, v bool) error {
	n := 0
	if v {
		n = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`UPDATE posts SET published = ?, updated_at = ? WHERE id = ?`, n, now, id)
	return err
}

// Delete removes a post.
func Delete(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM posts WHERE id = ?`, id)
	return err
}

// collect drains a *sql.Rows into posts, always closing it.
func collect(rows *sql.Rows) ([]Post, error) {
	defer rows.Close()
	var out []Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// likePattern wraps a user's search string for a LIKE with ESCAPE '\'.
// The escaping is not about injection — the value is a bound parameter —
// but about meaning: someone searching for "100%" wants a literal match,
// not a wildcard.
func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}
