// Package security manages local secrets that must not be stored in the SQLite
// database. It deliberately returns keys only in memory and never logs them.
package security

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const MasterKeyFileName = ".nbterminal-master.key"

func LoadOrCreateMasterKey(configDir string, protectedDataPaths ...string) (string, error) {
	if configDir == "" {
		return "", errors.New("master key directory is required")
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return "", fmt.Errorf("create master key directory: %w", err)
	}
	if err := SecurePrivateDir(configDir); err != nil {
		return "", fmt.Errorf("secure master key directory: %w", err)
	}
	path := filepath.Join(configDir, MasterKeyFileName)
	if key, err := readMasterKey(path); err == nil {
		return key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	for _, protectedPath := range protectedDataPaths {
		info, err := os.Lstat(protectedPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("inspect protected data before creating master key: %w", err)
		}
		if !info.Mode().IsRegular() {
			return "", errors.New("protected data path is not a regular file")
		}
		if info.Size() > 0 {
			return "", errors.New("master key is missing while encrypted application data already exists")
		}
	}

	raw := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", fmt.Errorf("generate master key: %w", err)
	}
	key := hex.EncodeToString(raw)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return readMasterKey(path)
	}
	if err != nil {
		return "", fmt.Errorf("create master key: %w", err)
	}
	writeErr := func() error {
		if _, err := file.WriteString(key + "\n"); err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			return err
		}
		return file.Close()
	}()
	if writeErr != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("persist master key: %w", writeErr)
	}
	if err := SecurePrivateFile(path); err != nil {
		return "", fmt.Errorf("secure master key: %w", err)
	}
	if err := syncDirectory(configDir); err != nil {
		return "", fmt.Errorf("sync master key directory: %w", err)
	}
	return key, nil
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func readMasterKey(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if err := SecurePrivateFile(path); err != nil {
		return "", fmt.Errorf("secure master key: %w", err)
	}
	key := strings.TrimSpace(string(payload))
	raw, err := hex.DecodeString(key)
	if err != nil || len(raw) != 32 {
		return "", errors.New("master key file is invalid")
	}
	return key, nil
}

func PurposeKey(masterKey, purpose string) (string, error) {
	if strings.TrimSpace(masterKey) == "" {
		return "", errors.New("master key is required")
	}
	if strings.TrimSpace(purpose) == "" {
		return "", errors.New("encryption purpose is required")
	}
	return masterKey + ":" + purpose, nil
}
