// Package database owns NBTerminal's embedded SQLite schema and transactional
// storage primitives. Callers encrypt product payloads before passing them here;
// this package never receives plaintext connection secrets or command history.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/0xdevelop/NBTerminal/internal/security"
	"github.com/george012/gtbox/gtbox_orm/gtbox_orm_sqlite"
)

const (
	FileName                    = "nbterminal.db"
	connectionMigrationMarker   = "legacy-connections-v1"
	historyMigrationMarker      = "legacy-history-v1"
	activeConnectionMetadataKey = "active_connection_id"
)

type DB struct {
	sql  *sql.DB
	path string
}

type ConnectionRow struct {
	ID         string
	PayloadEnc string
	Position   int
}

type HistoryRow struct {
	RecordedAtNS  int64
	ConnectionID  string
	PayloadEnc    string
	LegacySource  string
	LegacyOrdinal int64
}

func Open(dataDir string) (*DB, error) {
	if dataDir == "" {
		return nil, errors.New("database data directory is required")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	if err := security.SecurePrivateDir(dataDir); err != nil {
		return nil, fmt.Errorf("secure database directory: %w", err)
	}
	path := filepath.Join(dataDir, FileName)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return nil, fmt.Errorf("inspect sqlite file: %w", statErr)
		}
		if !info.Mode().IsRegular() {
			return nil, errors.New("sqlite path is not a regular file")
		}
	} else if err != nil {
		return nil, fmt.Errorf("securely create sqlite file: %w", err)
	} else if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close precreated sqlite file: %w", err)
	}
	if err := security.SecurePrivateFile(path); err != nil {
		return nil, fmt.Errorf("secure sqlite file before open: %w", err)
	}
	orm := &gtbox_orm_sqlite.GTORMSqlite{}
	orm.OpenSqlite(path)
	if orm.SqliteError != nil {
		return nil, fmt.Errorf("open sqlite through gtbox gorm: %w", orm.SqliteError)
	}
	if orm.SqliteDB == nil {
		return nil, errors.New("gtbox gorm returned an empty sqlite handle")
	}
	raw, err := orm.SqliteDB.DB()
	if err != nil {
		return nil, fmt.Errorf("load sqlite connection pool from gtbox gorm: %w", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := &DB{sql: raw, path: path}
	if err := db.initialize(); err != nil {
		_ = raw.Close()
		return nil, err
	}
	if err := security.SecurePrivateFile(path); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("secure sqlite file: %w", err)
	}
	return db, nil
}

func (d *DB) initialize() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, statement := range []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA synchronous = FULL`,
		`PRAGMA journal_mode = DELETE`,
		`PRAGMA locking_mode = EXCLUSIVE`,
	} {
		if _, err := d.sql.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure sqlite: %w", err)
		}
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite schema migration: %w", err)
	}
	rollback := func(err error) error {
		_ = tx.Rollback()
		return fmt.Errorf("initialize sqlite schema: %w", err)
	}
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at_ns INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS app_metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS connections (
			id TEXT PRIMARY KEY,
			payload_enc TEXT NOT NULL,
			position INTEGER NOT NULL,
			updated_at_ns INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS command_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			recorded_at_ns INTEGER NOT NULL,
			connection_id TEXT NOT NULL,
			payload_enc TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_command_history_connection
			ON command_history(connection_id, id DESC)`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return rollback(err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at_ns) VALUES(1, ?)`,
		time.Now().UTC().UnixNano()); err != nil {
		return rollback(err)
	}
	var schemaVersion int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&schemaVersion); err != nil {
		return rollback(err)
	}
	if schemaVersion < 2 {
		for _, statement := range []string{
			`ALTER TABLE command_history ADD COLUMN legacy_source TEXT`,
			`ALTER TABLE command_history ADD COLUMN legacy_ordinal INTEGER`,
			`CREATE UNIQUE INDEX idx_command_history_legacy_source
				ON command_history(legacy_source, legacy_ordinal)
				WHERE legacy_source IS NOT NULL`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return rollback(err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations(version, applied_at_ns) VALUES(2, ?)`,
			time.Now().UTC().UnixNano()); err != nil {
			return rollback(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite schema migration: %w", err)
	}
	return d.sql.PingContext(ctx)
}

func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}

func (d *DB) Path() string {
	if d == nil {
		return ""
	}
	return d.path
}

func (d *DB) LoadConnections(ctx context.Context) ([]ConnectionRow, string, error) {
	if d == nil || d.sql == nil {
		return nil, "", errors.New("sqlite database is not open")
	}
	rows, err := d.sql.QueryContext(ctx, `SELECT id, payload_enc, position FROM connections ORDER BY position, id`)
	if err != nil {
		return nil, "", fmt.Errorf("query connections: %w", err)
	}
	defer rows.Close()
	var result []ConnectionRow
	for rows.Next() {
		var row ConnectionRow
		if err := rows.Scan(&row.ID, &row.PayloadEnc, &row.Position); err != nil {
			return nil, "", fmt.Errorf("scan connection: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate connections: %w", err)
	}
	active, err := d.metadata(ctx, activeConnectionMetadataKey)
	if err != nil {
		return nil, "", err
	}
	return result, active, nil
}

func (d *DB) SaveConnections(ctx context.Context, rows []ConnectionRow, activeID string) error {
	return d.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM connections`); err != nil {
			return err
		}
		statement, err := tx.PrepareContext(ctx, `INSERT INTO connections(id, payload_enc, position, updated_at_ns) VALUES(?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer statement.Close()
		now := time.Now().UTC().UnixNano()
		for _, row := range rows {
			if row.ID == "" || row.PayloadEnc == "" {
				return errors.New("encrypted connection row is incomplete")
			}
			if _, err := statement.ExecContext(ctx, row.ID, row.PayloadEnc, row.Position, now); err != nil {
				return err
			}
		}
		return setMetadataTx(ctx, tx, activeConnectionMetadataKey, activeID)
	})
}

func (d *DB) SetActiveConnection(ctx context.Context, activeID string) error {
	if d == nil || d.sql == nil {
		return errors.New("sqlite database is not open")
	}
	_, err := d.sql.ExecContext(ctx, `INSERT INTO app_metadata(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, activeConnectionMetadataKey, activeID)
	if err != nil {
		return fmt.Errorf("set active connection: %w", err)
	}
	return nil
}

func (d *DB) MigrateConnections(ctx context.Context, rows []ConnectionRow, activeID string) (bool, error) {
	applied := false
	err := d.withTx(ctx, func(tx *sql.Tx) error {
		statement, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO connections(id, payload_enc, position, updated_at_ns) VALUES(?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer statement.Close()
		now := time.Now().UTC().UnixNano()
		var maxPosition int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) FROM connections`).Scan(&maxPosition); err != nil {
			return err
		}
		for _, row := range rows {
			if row.ID == "" || row.PayloadEnc == "" {
				return errors.New("encrypted migration connection row is incomplete")
			}
			result, err := statement.ExecContext(ctx, row.ID, row.PayloadEnc, maxPosition+1+row.Position, now)
			if err != nil {
				return err
			}
			if count, err := result.RowsAffected(); err == nil && count > 0 {
				applied = true
			}
		}
		currentActive, err := metadataTx(ctx, tx, activeConnectionMetadataKey)
		if err != nil {
			return err
		}
		if currentActive == "" && activeID != "" {
			if err := setMetadataTx(ctx, tx, activeConnectionMetadataKey, activeID); err != nil {
				return err
			}
		}
		return setMetadataTx(ctx, tx, connectionMigrationMarker, "done")
	})
	if err != nil {
		return false, fmt.Errorf("migrate legacy connections: %w", err)
	}
	return applied, nil
}

func (d *DB) ConnectionsMigrationComplete(ctx context.Context) (bool, error) {
	value, err := d.metadata(ctx, connectionMigrationMarker)
	return value == "done", err
}

func (d *DB) AppendHistory(ctx context.Context, row HistoryRow) error {
	if d == nil || d.sql == nil {
		return errors.New("sqlite database is not open")
	}
	if row.PayloadEnc == "" {
		return errors.New("encrypted history payload is required")
	}
	if row.RecordedAtNS == 0 {
		row.RecordedAtNS = time.Now().UTC().UnixNano()
	}
	_, err := d.sql.ExecContext(ctx, `INSERT INTO command_history(recorded_at_ns, connection_id, payload_enc) VALUES(?, ?, ?)`, row.RecordedAtNS, row.ConnectionID, row.PayloadEnc)
	if err != nil {
		return fmt.Errorf("append command history: %w", err)
	}
	return nil
}

func (d *DB) LoadHistory(ctx context.Context, connectionID string, limit int) ([]HistoryRow, error) {
	if d == nil || d.sql == nil {
		return nil, errors.New("sqlite database is not open")
	}
	query := `SELECT recorded_at_ns, connection_id, payload_enc FROM command_history`
	args := make([]any, 0, 2)
	if connectionID != "" {
		query += ` WHERE connection_id = ?`
		args = append(args, connectionID)
	}
	if limit > 0 {
		query += ` ORDER BY id DESC LIMIT ?`
		args = append(args, limit)
	} else {
		query += ` ORDER BY id ASC`
	}
	rows, err := d.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query command history: %w", err)
	}
	defer rows.Close()
	var result []HistoryRow
	for rows.Next() {
		var row HistoryRow
		if err := rows.Scan(&row.RecordedAtNS, &row.ConnectionID, &row.PayloadEnc); err != nil {
			return nil, fmt.Errorf("scan command history: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate command history: %w", err)
	}
	if limit > 0 {
		for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
			result[left], result[right] = result[right], result[left]
		}
	}
	return result, nil
}

func (d *DB) MigrateHistory(ctx context.Context, rows []HistoryRow) (bool, error) {
	applied := false
	err := d.withTx(ctx, func(tx *sql.Tx) error {
		statement, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO command_history(recorded_at_ns, connection_id, payload_enc, legacy_source, legacy_ordinal) VALUES(?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer statement.Close()
		for _, row := range rows {
			if row.PayloadEnc == "" || row.LegacySource == "" || row.LegacyOrdinal <= 0 {
				return errors.New("encrypted migration history row is incomplete")
			}
			result, err := statement.ExecContext(ctx, row.RecordedAtNS, row.ConnectionID, row.PayloadEnc, row.LegacySource, row.LegacyOrdinal)
			if err != nil {
				return err
			}
			if count, err := result.RowsAffected(); err == nil && count > 0 {
				applied = true
			}
		}
		return setMetadataTx(ctx, tx, historyMigrationMarker, "done")
	})
	if err != nil {
		return false, fmt.Errorf("migrate legacy history: %w", err)
	}
	return applied, nil
}

func (d *DB) HistoryMigrationComplete(ctx context.Context) (bool, error) {
	value, err := d.metadata(ctx, historyMigrationMarker)
	return value == "done", err
}

func (d *DB) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if d == nil || d.sql == nil {
		return errors.New("sqlite database is not open")
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (d *DB) metadata(ctx context.Context, key string) (string, error) {
	var value string
	err := d.sql.QueryRowContext(ctx, `SELECT value FROM app_metadata WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load metadata: %w", err)
	}
	return value, nil
}

func metadataTx(ctx context.Context, tx *sql.Tx, key string) (string, error) {
	var value string
	err := tx.QueryRowContext(ctx, `SELECT value FROM app_metadata WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func setMetadataTx(ctx context.Context, tx *sql.Tx, key, value string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO app_metadata(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}
