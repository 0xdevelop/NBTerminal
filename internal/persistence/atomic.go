// Package persistence provides small, reusable primitives for durable local
// application state. It deliberately has no dependency on GUI or config code.
package persistence

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// AtomicWriteFile writes data to a temporary sibling, flushes it, and atomically
// replaces path. Keeping the temporary file in the destination directory avoids
// cross-filesystem rename failures. Existing files are never truncated before a
// complete replacement is ready.
func AtomicWriteFile(path string, data []byte, perm fs.FileMode) (err error) {
	if path == "" {
		return errors.New("atomic write path is required")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	if err = tmp.Chmod(perm); err != nil {
		return fmt.Errorf("set temporary file permissions: %w", err)
	}
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("flush temporary file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err = os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace destination file: %w", err)
	}
	// Rename normally preserves the temporary file mode. Chmod again so an
	// unusual platform implementation cannot retain a previous destination mode.
	if err = os.Chmod(path, perm); err != nil {
		return fmt.Errorf("set destination file permissions: %w", err)
	}
	return nil
}
