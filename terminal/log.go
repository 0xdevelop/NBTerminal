package terminal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"
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
	scanner := newLineScanner(f)
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

// LoadForConnection returns the most recent history entries for a single
// connection in chronological order. It is intentionally implemented on top of
// Load so GUI code can ask for per-connection history without learning the JSONL
// storage details.
func (s *HistoryStore) LoadForConnection(connectionID string, limit int) ([]HistoryEntry, error) {
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
