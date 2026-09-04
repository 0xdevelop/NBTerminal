package terminal

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/0xdevelop/NBTerminal/internal/database"
	"github.com/0xdevelop/NBTerminal/internal/security"
)

const historyOutputMaxBytes = 64 * 1024

// HistoryEntry is a compact, append-only command log record. It intentionally
// stores command/output metadata but never stores connection secrets.
type HistoryEntry struct {
	Time           time.Time      `json:"time"`
	ConnectionID   string         `json:"connection_id"`
	ConnectionName string         `json:"connection_name"`
	ConnectionType ConnectionType `json:"connection_type"`
	Command        string         `json:"command"`
	ExitCode       int            `json:"exit_code"`
	DurationMS     int64          `json:"duration_ms"`
	Interactive    bool           `json:"interactive,omitempty"`
	Stdout         string         `json:"stdout,omitempty"`
	Stderr         string         `json:"stderr,omitempty"`
}

// HistoryFromResult converts a command result into a persistable, secret-free
// history entry.
func HistoryFromResult(result CommandResult) HistoryEntry {
	when := result.FinishedAt
	if when.IsZero() {
		when = time.Now()
	}
	duration := result.FinishedAt.Sub(result.StartedAt).Milliseconds()
	if result.StartedAt.IsZero() || result.FinishedAt.IsZero() || duration < 0 {
		duration = 0
	}
	return HistoryEntry{
		Time:           when,
		ConnectionID:   normalizeUTF8(result.Connection.ID),
		ConnectionName: normalizeUTF8(result.Connection.Name),
		ConnectionType: result.Connection.Type,
		Command:        normalizeUTF8(result.Command),
		ExitCode:       result.ExitCode,
		DurationMS:     duration,
		Stdout:         truncateHistoryOutput(result.Stdout),
		Stderr:         truncateHistoryOutput(result.Stderr),
	}
}

func truncateHistoryOutput(s string) string {
	s = normalizeUTF8(s)
	if len(s) <= historyOutputMaxBytes {
		return s
	}
	cut := historyOutputMaxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	if cut == 0 {
		cut = historyOutputMaxBytes
	}
	return s[:cut] + fmt.Sprintf("\n[NBTerminal history output truncated: original %d bytes, kept %d bytes]", len(s), cut)
}

// HistoryStore appends and reads JSONL command history. The append-only format
// keeps GUI command logging simple and robust across crashes.
type HistoryStore struct {
	path          string
	db            *database.DB
	encryptionKey string
	mu            sync.Mutex
}

func NewHistoryStore(path string) *HistoryStore { return &HistoryStore{path: path} }

func NewSQLiteHistoryStore(db *database.DB, legacyPath, encryptionKey string) *HistoryStore {
	return &HistoryStore{path: legacyPath, db: db, encryptionKey: encryptionKey}
}

func (s *HistoryStore) Append(entry HistoryEntry) error {
	if s == nil || (s.path == "" && s.db == nil) {
		return errors.New("history store path is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.appendSQLiteLocked(entry)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	closeWithError := func(cause error) error {
		if closeErr := f.Close(); closeErr != nil && cause == nil {
			return closeErr
		}
		return cause
	}
	if err := f.Chmod(0o600); err != nil {
		return closeWithError(err)
	}
	if err := prepareHistoryAppend(f); err != nil {
		return closeWithError(err)
	}
	if err := json.NewEncoder(f).Encode(entry); err != nil {
		return closeWithError(err)
	}
	if err := f.Sync(); err != nil {
		return closeWithError(err)
	}
	return closeWithError(nil)
}

// prepareHistoryAppend positions f at its end and repairs only an incomplete
// final JSONL record, the sole record a process crash can leave half-written.
// A valid legacy final record without a newline receives a separator so the next
// append cannot concatenate two JSON objects.
func prepareHistoryAppend(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		_, err = f.Seek(0, io.SeekEnd)
		return err
	}
	const maxTailBytes = int64(maxTerminalLineBytes + 1)
	readSize := info.Size()
	if readSize > maxTailBytes {
		readSize = maxTailBytes
	}
	tail := make([]byte, readSize)
	start := info.Size() - readSize
	if _, err := f.ReadAt(tail, start); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if tail[len(tail)-1] == '\n' {
		_, err = f.Seek(0, io.SeekEnd)
		return err
	}
	lastNewline := bytes.LastIndexByte(tail, '\n')
	if lastNewline < 0 && start > 0 {
		return errors.New("history final record exceeds maximum size")
	}
	record := tail[lastNewline+1:]
	if json.Valid(record) {
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			return err
		}
		_, err = f.Write([]byte{'\n'})
		return err
	}
	truncateAt := start + int64(lastNewline+1)
	if err := f.Truncate(truncateAt); err != nil {
		return err
	}
	_, err = f.Seek(0, io.SeekEnd)
	return err
}

func (s *HistoryStore) Load(limit int) ([]HistoryEntry, error) {
	if s == nil || (s.path == "" && s.db == nil) {
		return nil, errors.New("history store path is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.loadSQLiteLocked("", limit)
	}
	f, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	endsWithNewline := info.Size() == 0
	if info.Size() > 0 {
		var last [1]byte
		if _, err := f.ReadAt(last[:], info.Size()-1); err != nil {
			return nil, err
		}
		endsWithNewline = last[0] == '\n'
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	var entries []HistoryEntry
	appendLine := func(line []byte) error {
		var entry HistoryEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return err
		}
		entries = append(entries, entry)
		if limit > 0 && len(entries) > limit {
			copy(entries, entries[1:])
			entries = entries[:limit]
		}
		return nil
	}
	scanner := newLineScanner(f)
	var pending []byte
	for scanner.Scan() {
		if pending != nil {
			if err := appendLine(pending); err != nil {
				return nil, err
			}
		}
		pending = append(pending[:0], scanner.Bytes()...)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if pending != nil {
		if err := appendLine(pending); err != nil {
			if !endsWithNewline {
				return entries, nil
			}
			return nil, err
		}
	}
	return entries, nil
}

// LoadForConnection returns the most recent history entries for a single
// connection in chronological order. It is intentionally implemented on top of
// Load so GUI code can ask for per-connection history without learning the JSONL
// storage details.
func (s *HistoryStore) LoadForConnection(connectionID string, limit int) ([]HistoryEntry, error) {
	if s != nil && s.db != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.loadSQLiteLocked(connectionID, limit)
	}
	entries, err := s.Load(0)
	if err != nil {
		return nil, err
	}
	if connectionID == "" {
		if limit > 0 && len(entries) > limit {
			return append([]HistoryEntry(nil), entries[len(entries)-limit:]...), nil
		}
		return entries, nil
	}
	filtered := make([]HistoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.ConnectionID != connectionID {
			continue
		}
		filtered = append(filtered, entry)
		if limit > 0 && len(filtered) > limit {
			copy(filtered, filtered[1:])
			filtered = filtered[:limit]
		}
	}
	return filtered, nil
}

func (s *HistoryStore) MigrateLegacy() (bool, error) {
	if s == nil || s.db == nil {
		return false, nil
	}
	legacySource, err := historyLegacySource(s.path, s.encryptionKey)
	if err != nil {
		return false, err
	}
	legacy := NewHistoryStore(s.path)
	entries, err := legacy.Load(0)
	if err != nil {
		return false, fmt.Errorf("load legacy command history: %w", err)
	}
	rows := make([]database.HistoryRow, 0, len(entries))
	for index, entry := range entries {
		row, err := encryptedHistoryRow(entry, s.encryptionKey)
		if err != nil {
			return false, err
		}
		row.LegacySource = legacySource
		row.LegacyOrdinal = int64(index + 1)
		rows = append(rows, row)
	}
	appliedNow, err := s.db.MigrateHistory(context.Background(), rows)
	if err != nil {
		return false, err
	}
	migrationComplete, err := s.db.HistoryMigrationComplete(context.Background())
	if err != nil {
		return false, err
	}
	if legacySource != "" && migrationComplete {
		if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("remove migrated legacy command history: %w", err)
		}
	}
	return appliedNow, nil
}

func historyLegacySource(path, encryptionKey string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect legacy command history: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("legacy command history is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open legacy command history fingerprint: %w", err)
	}
	defer file.Close()
	mac := hmac.New(sha256.New, []byte(encryptionKey))
	if _, err := io.Copy(mac, file); err != nil {
		return "", fmt.Errorf("fingerprint legacy command history: %w", err)
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (s *HistoryStore) appendSQLiteLocked(entry HistoryEntry) error {
	row, err := encryptedHistoryRow(entry, s.encryptionKey)
	if err != nil {
		return err
	}
	return s.db.AppendHistory(context.Background(), row)
}

func encryptedHistoryRow(entry HistoryEntry, encryptionKey string) (database.HistoryRow, error) {
	if strings.TrimSpace(encryptionKey) == "" {
		return database.HistoryRow{}, errors.New("history encryption key is required")
	}
	entry.Time = entry.Time.UTC()
	if entry.Time.IsZero() {
		entry.Time = time.Now().UTC()
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return database.HistoryRow{}, fmt.Errorf("encode command history: %w", err)
	}
	ciphertext, err := security.EncryptPayloadGT(string(payload), encryptionKey)
	if err != nil {
		return database.HistoryRow{}, fmt.Errorf("encrypt command history: %w", err)
	}
	storageConnectionID, err := encryptedHistoryConnectionID(entry.ConnectionID, encryptionKey)
	if err != nil {
		return database.HistoryRow{}, err
	}
	return database.HistoryRow{RecordedAtNS: entry.Time.UnixNano(), ConnectionID: storageConnectionID, PayloadEnc: ciphertext}, nil
}

func (s *HistoryStore) loadSQLiteLocked(connectionID string, limit int) ([]HistoryEntry, error) {
	if strings.TrimSpace(s.encryptionKey) == "" {
		return nil, errors.New("history encryption key is required")
	}
	// GTEnc produces opaque ciphertext rather than a deterministic lookup token.
	// Keep connection associations encrypted in SQLite, then decrypt and filter
	// candidate rows in memory instead of comparing a fresh encryption result.
	databaseLimit := limit
	if connectionID != "" {
		databaseLimit = 0
	}
	rows, err := s.db.LoadHistory(context.Background(), "", databaseLimit)
	if err != nil {
		return nil, err
	}
	entries := make([]HistoryEntry, 0, len(rows))
	for _, row := range rows {
		plaintext, err := security.DecryptPayloadGT(row.PayloadEnc, s.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt command history: %w", err)
		}
		var entry HistoryEntry
		if err := json.Unmarshal([]byte(plaintext), &entry); err != nil {
			return nil, fmt.Errorf("decode command history: %w", err)
		}
		storedConnectionID, err := decryptedHistoryConnectionID(row.ConnectionID, s.encryptionKey)
		if err != nil || storedConnectionID != entry.ConnectionID {
			return nil, errors.New("decrypted command history connection id does not match its encrypted storage id")
		}
		if entry.Time.UTC().UnixNano() != row.RecordedAtNS {
			return nil, errors.New("decrypted command history timestamp does not match its storage index")
		}
		if connectionID != "" && entry.ConnectionID != connectionID {
			continue
		}
		entries = append(entries, entry)
		if limit > 0 && len(entries) > limit {
			copy(entries, entries[1:])
			entries = entries[:limit]
		}
	}
	return entries, nil
}

func encryptedHistoryConnectionID(connectionID, encryptionKey string) (string, error) {
	if connectionID == "" {
		return "", nil
	}
	storageID, err := security.EncryptPayloadGT(connectionID, encryptionKey)
	if err != nil {
		return "", fmt.Errorf("encrypt history connection id: %w", err)
	}
	return storageID, nil
}

func decryptedHistoryConnectionID(storageID, encryptionKey string) (string, error) {
	if storageID == "" {
		return "", nil
	}
	connectionID, err := security.DecryptPayloadGT(storageID, encryptionKey)
	if err != nil {
		return "", fmt.Errorf("decrypt history connection id: %w", err)
	}
	return connectionID, nil
}

// LastCommand returns the most recent non-empty command for a connection. An
// empty connectionID searches all history, which is useful for global GUI
// command recall. The boolean return is false when no reusable command exists.
func (s *HistoryStore) LastCommand(connectionID string) (string, bool, error) {
	entries, err := s.LoadForConnection(connectionID, 0)
	if err != nil {
		return "", false, err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Command != "" {
			return entries[i].Command, true, nil
		}
	}
	return "", false, nil
}
