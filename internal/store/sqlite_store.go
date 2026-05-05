package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/agendash/AgenLeash/internal/model"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, errors.New("path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &SQLiteStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) init() error {
	stmts := []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA busy_timeout = 5000;`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			payload BLOB NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS workspaces (
			id TEXT PRIMARY KEY,
			payload BLOB NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			payload BLOB NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) Snapshot() Catalog {
	catalog := Catalog{}.normalize()
	catalog.Sessions = loadPayloadMap[model.Session](s.db, "sessions")
	catalog.Workspaces = loadPayloadMap[model.Workspace](s.db, "workspaces")
	catalog.Conversations = loadPayloadMap[model.Conversation](s.db, "conversations")
	if updatedAt, ok := s.metadataTime("updated_at"); ok {
		catalog.UpdatedAt = updatedAt
	}
	return catalog
}

func (s *SQLiteStore) Replace(c Catalog) error {
	c = c.normalize()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, table := range []string{"sessions", "workspaces", "conversations"} {
		if _, err = tx.Exec("DELETE FROM " + table); err != nil {
			return err
		}
	}

	for _, session := range c.Sessions {
		if err = upsertPayloadTx(tx, "sessions", session.ID, session); err != nil {
			return err
		}
	}
	for _, workspace := range c.Workspaces {
		if err = upsertPayloadTx(tx, "workspaces", workspace.ID, workspace); err != nil {
			return err
		}
	}
	for _, conversation := range c.Conversations {
		if err = upsertPayloadTx(tx, "conversations", conversation.ID, conversation); err != nil {
			return err
		}
	}

	updatedAt := c.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	if err = upsertMetadataTx(tx, "updated_at", updatedAt.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) UpsertSession(session model.Session) error {
	if session.ID == "" {
		return errors.New("session id is required")
	}
	changed, err := upsertPayload(s.db, "sessions", session.ID, session)
	if err != nil {
		return err
	}
	if changed {
		return s.touch()
	}
	return nil
}

func (s *SQLiteStore) UpsertWorkspace(workspace model.Workspace) error {
	if workspace.ID == "" {
		return errors.New("workspace id is required")
	}
	changed, err := upsertPayload(s.db, "workspaces", workspace.ID, workspace)
	if err != nil {
		return err
	}
	if changed {
		return s.touch()
	}
	return nil
}

func (s *SQLiteStore) UpsertConversation(conversation model.Conversation) error {
	if conversation.ID == "" {
		return errors.New("conversation id is required")
	}
	changed, err := upsertPayload(s.db, "conversations", conversation.ID, conversation)
	if err != nil {
		return err
	}
	if changed {
		return s.touch()
	}
	return nil
}

func (s *SQLiteStore) DeleteSession(id string) error {
	if id == "" {
		return errors.New("session id is required")
	}
	changed, err := deletePayload(s.db, "sessions", id)
	if err != nil {
		return err
	}
	if changed {
		return s.touch()
	}
	return nil
}

func (s *SQLiteStore) DeleteWorkspace(id string) error {
	if id == "" {
		return errors.New("workspace id is required")
	}
	changed, err := deletePayload(s.db, "workspaces", id)
	if err != nil {
		return err
	}
	if changed {
		return s.touch()
	}
	return nil
}

func (s *SQLiteStore) DeleteConversation(id string) error {
	if id == "" {
		return errors.New("conversation id is required")
	}
	changed, err := deletePayload(s.db, "conversations", id)
	if err != nil {
		return err
	}
	if changed {
		return s.touch()
	}
	return nil
}

func (s *SQLiteStore) GetSession(id string) (model.Session, bool) {
	var payload []byte
	if err := s.db.QueryRow(`SELECT payload FROM sessions WHERE id = ?`, id).Scan(&payload); err != nil {
		return model.Session{}, false
	}
	var session model.Session
	if err := json.Unmarshal(payload, &session); err != nil {
		return model.Session{}, false
	}
	return session, true
}

func (s *SQLiteStore) ListSessions() []model.Session {
	return loadPayloadList[model.Session](s.db, "sessions")
}

func (s *SQLiteStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) touch() error {
	_, err := s.db.Exec(
		`INSERT INTO metadata(key, value) VALUES('updated_at', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteStore) metadataTime(key string) (time.Time, bool) {
	var raw string
	if err := s.db.QueryRow(`SELECT value FROM metadata WHERE key = ?`, key).Scan(&raw); err != nil {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func upsertPayload(db *sql.DB, table, id string, value any) (bool, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	result, err := db.Exec(
		`INSERT INTO `+table+`(id, payload) VALUES(?, ?)
		 ON CONFLICT(id) DO UPDATE SET payload = excluded.payload
		 WHERE payload <> excluded.payload`,
		id,
		payload,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return true, nil
	}
	return rows > 0, nil
}

func upsertPayloadTx(tx *sql.Tx, table, id string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		`INSERT INTO `+table+`(id, payload) VALUES(?, ?)
		 ON CONFLICT(id) DO UPDATE SET payload = excluded.payload`,
		id,
		payload,
	)
	return err
}

func deletePayload(db *sql.DB, table, id string) (bool, error) {
	result, err := db.Exec(`DELETE FROM `+table+` WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return true, nil
	}
	return rows > 0, nil
}

func upsertMetadataTx(tx *sql.Tx, key, value string) error {
	_, err := tx.Exec(
		`INSERT INTO metadata(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key,
		value,
	)
	return err
}

func loadPayloadList[T any](db *sql.DB, table string) []T {
	rows, err := db.Query(`SELECT payload FROM ` + table)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := make([]T, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			continue
		}
		var value T
		if err := json.Unmarshal(payload, &value); err != nil {
			continue
		}
		out = append(out, value)
	}
	return out
}

func loadPayloadMap[T any](db *sql.DB, table string) map[string]T {
	rows, err := db.Query(`SELECT id, payload FROM ` + table)
	if err != nil {
		return map[string]T{}
	}
	defer rows.Close()

	out := make(map[string]T)
	for rows.Next() {
		var (
			id      string
			payload []byte
		)
		if err := rows.Scan(&id, &payload); err != nil {
			continue
		}
		var value T
		if err := json.Unmarshal(payload, &value); err != nil {
			continue
		}
		out[id] = value
	}
	return out
}
