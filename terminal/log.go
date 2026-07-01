package terminal

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

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
		ConnectionID:   result.Connection.ID,
		ConnectionName: result.Connection.Name,
		ConnectionType: result.Connection.Type,
		Command:        result.Command,
		ExitCode:       result.ExitCode,
		DurationMS:     duration,
		Stdout:         result.Stdout,
		Stderr:         result.Stderr,
	}
}

// HistoryStore appends and reads JSONL command history. The append-only format
// keeps GUI command logging simple and robust across crashes.
type HistoryStore struct {
	path string
	mu   sync.Mutex
}

func NewHistoryStore(path string) *HistoryStore { return &HistoryStore{path: path} }

func (s *HistoryStore) Append(entry HistoryEntry) error {
	if s == nil || s.path == "" {
		return errors.New("history store path is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(entry)
}

func (s *HistoryStore) Load(limit int) ([]HistoryEntry, error) {
	if s == nil || s.path == "" {
		return nil, errors.New("history store path is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var entries []HistoryEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry HistoryEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
		if limit > 0 && len(entries) > limit {
			copy(entries, entries[1:])
			entries = entries[:limit]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
