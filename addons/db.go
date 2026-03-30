// SQLite Session Persistence — stores sessions, messages, facts, and settings.
// Uses modernc.org/sqlite (pure Go, no CGO).

package addons

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/cuber-it/heinzel-ai-core-go/core"

	_ "modernc.org/sqlite"
)

type DBSession struct {
	ID         string
	Name       string
	Created    string
	LastActive string
}

type Fact struct {
	ID      int
	Key     string
	Value   string
	Source  string
	Created string
}

type DB struct {
	conn *sql.DB
	path string
}

func OpenDB(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	database := &DB{conn: conn, path: path}
	if err := database.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return database, nil
}

func (database *DB) Close() error {
	if database.conn != nil {
		return database.conn.Close()
	}
	return nil
}

func (database *DB) migrate() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			name TEXT,
			created TEXT,
			last_active TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT,
			role TEXT,
			content TEXT,
			timestamp TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS facts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT,
			value TEXT,
			source TEXT,
			created TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT
		)`,
	}

	for _, stmt := range statements {
		if _, err := database.conn.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:40], err)
		}
	}
	return nil
}

func (database *DB) CreateSession(name string) string {
	sessionID := fmt.Sprintf("session-%d", time.Now().UnixMilli())
	now := time.Now().Format(time.RFC3339)

	database.conn.Exec(
		"INSERT INTO sessions (id, name, created, last_active) VALUES (?, ?, ?, ?)",
		sessionID, name, now, now,
	)
	return sessionID
}

func (database *DB) ListSessions() []DBSession {
	rows, err := database.conn.Query(
		"SELECT id, name, created, last_active FROM sessions ORDER BY last_active DESC",
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var sessions []DBSession
	for rows.Next() {
		var session DBSession
		if err := rows.Scan(&session.ID, &session.Name, &session.Created, &session.LastActive); err != nil {
			continue
		}
		sessions = append(sessions, session)
	}
	return sessions
}

func (database *DB) SaveMessage(sessionID, role, content string) {
	now := time.Now().Format(time.RFC3339)
	database.conn.Exec(
		"INSERT INTO messages (session_id, role, content, timestamp) VALUES (?, ?, ?, ?)",
		sessionID, role, content, now,
	)
	database.conn.Exec(
		"UPDATE sessions SET last_active = ? WHERE id = ?",
		now, sessionID,
	)
}

func (database *DB) LoadMessages(sessionID string) []core.Message {
	rows, err := database.conn.Query(
		"SELECT role, content, timestamp FROM messages WHERE session_id = ? ORDER BY id ASC",
		sessionID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var messages []core.Message
	for rows.Next() {
		var role, content, timestamp string
		if err := rows.Scan(&role, &content, &timestamp); err != nil {
			continue
		}
		parsedTime, _ := time.Parse(time.RFC3339, timestamp)
		messages = append(messages, core.Message{
			Role:    role,
			Content: content,
			Time:    parsedTime,
		})
	}
	return messages
}

func (database *DB) SaveFact(key, value, source string) {
	now := time.Now().Format(time.RFC3339)
	database.conn.Exec(
		"INSERT INTO facts (key, value, source, created) VALUES (?, ?, ?, ?)",
		key, value, source, now,
	)
}

func (database *DB) QueryFacts(query string) []Fact {
	pattern := "%" + query + "%"
	rows, err := database.conn.Query(
		"SELECT id, key, value, source, created FROM facts WHERE key LIKE ? OR value LIKE ? ORDER BY created DESC",
		pattern, pattern,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var facts []Fact
	for rows.Next() {
		var fact Fact
		if err := rows.Scan(&fact.ID, &fact.Key, &fact.Value, &fact.Source, &fact.Created); err != nil {
			continue
		}
		facts = append(facts, fact)
	}
	return facts
}

func (database *DB) DeleteFact(key, value string) {
	database.conn.Exec(
		"DELETE FROM facts WHERE key = ? AND value = ?",
		key, value,
	)
}

func (database *DB) GetSetting(key string) string {
	var value string
	err := database.conn.QueryRow(
		"SELECT value FROM settings WHERE key = ?", key,
	).Scan(&value)
	if err != nil {
		return ""
	}
	return value
}

func (database *DB) SetSetting(key, value string) {
	database.conn.Exec(
		"INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)",
		key, value,
	)
}
