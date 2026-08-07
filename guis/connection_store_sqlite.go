package guis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/NBTerminal/internal/database"
	"github.com/0xdevelop/NBTerminal/internal/security"
)

func (s *connectionStore) loadSQLiteLocked() error {
	if s == nil || s.db == nil {
		return errors.New("sqlite connection store is not configured")
	}
	ctx := context.Background()
	rows, activeID, err := s.db.LoadConnections(ctx)
	if err != nil {
		return err
	}
	legacyExists, err := regularFileExists(s.path)
	if err != nil {
		return err
	}
	migrationComplete, err := s.db.ConnectionsMigrationComplete(ctx)
	if err != nil {
		return err
	}
	if legacyExists || (len(rows) == 0 && !migrationComplete) {
		legacy, err := s.legacyProfilesForMigrationLocked()
		if err != nil {
			return err
		}
		s.list = append([]connectionProfile(nil), legacy...)
		s.normalizeLocked()
		legacy = append([]connectionProfile(nil), s.list...)
		migrationRows, err := encryptedConnectionRows(legacy, s.encryptionKey)
		if err != nil {
			return err
		}
		legacyActiveID := normalizedActiveProfileID(legacy, configuredActiveProfileID())
		activeID, err = encryptedProfileStorageID(legacyActiveID, s.encryptionKey)
		if err != nil {
			return err
		}
		if _, err := s.db.MigrateConnections(ctx, migrationRows, activeID); err != nil {
			return err
		}
		migrationComplete, err = s.db.ConnectionsMigrationComplete(ctx)
		if err != nil {
			return err
		}
		if legacyExists && migrationComplete {
			if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove migrated legacy connections: %w", err)
			}
		}
		rows, activeID, err = s.db.LoadConnections(ctx)
		if err != nil {
			return err
		}
	}
	profiles, err := decryptConnectionRows(rows, s.encryptionKey)
	if err != nil {
		return err
	}
	s.list = profiles
	s.normalizeLocked()
	activeID, err = decryptedActiveProfileID(s.list, activeID, s.encryptionKey)
	if err != nil {
		return err
	}
	if config.GlobalConfig != nil {
		config.GlobalConfig.ActiveConnectionID = normalizedActiveProfileID(s.list, activeID)
	}
	return syncSQLiteConfig()
}

func syncSQLiteConfig() error {
	if config.GlobalConfig == nil {
		return nil
	}
	previousConnections := config.GlobalConfig.Connections
	previousActiveID := config.GlobalConfig.ActiveConnectionID
	config.GlobalConfig.Connections = nil
	config.GlobalConfig.ActiveConnectionID = ""
	if config.CurrentApp == nil || config.CurrentApp.AppConfigFilePath == "" {
		return nil
	}
	if err := config.SaveConfig(config.CurrentApp.AppConfigFilePath); err != nil {
		config.GlobalConfig.Connections = previousConnections
		config.GlobalConfig.ActiveConnectionID = previousActiveID
		return err
	}
	return nil
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect legacy connection store: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("legacy connection store is not a regular file")
	}
	return true, nil
}

func (s *connectionStore) legacyProfilesForMigrationLocked() ([]connectionProfile, error) {
	buf, err := os.ReadFile(s.path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read legacy connections: %w", err)
		}
		if profiles := profilesFromConfig(config.GlobalConfig); len(profiles) > 0 {
			return profiles, nil
		}
		return defaultConnections(), nil
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return nil, fmt.Errorf("secure legacy connections: %w", err)
	}
	if strings.TrimSpace(string(buf)) == "" {
		return defaultConnections(), nil
	}
	var profiles []connectionProfile
	if err := json.Unmarshal(buf, &profiles); err != nil {
		return nil, fmt.Errorf("decode legacy connections: %w", err)
	}
	return profiles, nil
}

func (s *connectionStore) saveActiveSQLiteLocked(list []connectionProfile, activeID string) error {
	ctx := context.Background()
	previous := append([]connectionProfile(nil), s.list...)
	s.list = append([]connectionProfile(nil), list...)
	s.normalizeLocked()
	activeID = normalizedActiveProfileID(s.list, activeID)
	rows, err := encryptedConnectionRows(s.list, s.encryptionKey)
	if err != nil {
		s.list = previous
		return err
	}
	activeStorageID, err := encryptedProfileStorageID(activeID, s.encryptionKey)
	if err != nil {
		s.list = previous
		return err
	}
	if err := s.db.SaveConnections(ctx, rows, activeStorageID); err != nil {
		s.list = previous
		return err
	}
	if err := syncSQLiteConfig(); err != nil {
		return fmt.Errorf("clean deprecated connection fields from app config: %w", err)
	}
	return nil
}

func (s *connectionStore) setActiveSQLiteLocked(activeID string) error {
	ctx := context.Background()
	activeID = normalizedActiveProfileID(s.list, activeID)
	activeStorageID, err := encryptedProfileStorageID(activeID, s.encryptionKey)
	if err != nil {
		return err
	}
	if err := s.db.SetActiveConnection(ctx, activeStorageID); err != nil {
		return err
	}
	if err := syncSQLiteConfig(); err != nil {
		return fmt.Errorf("clean deprecated active connection from app config: %w", err)
	}
	return nil
}

func encryptedConnectionRows(profiles []connectionProfile, encryptionKey string) ([]database.ConnectionRow, error) {
	if strings.TrimSpace(encryptionKey) == "" {
		return nil, errors.New("connection encryption key is required")
	}
	rows := make([]database.ConnectionRow, 0, len(profiles))
	for index, profile := range profiles {
		payload, err := json.Marshal(profile)
		if err != nil {
			return nil, fmt.Errorf("encode connection profile: %w", err)
		}
		ciphertext, err := security.EncryptPayloadGT(string(payload), encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("encrypt connection profile: %w", err)
		}
		storageID, err := encryptedProfileStorageID(profile.ID, encryptionKey)
		if err != nil {
			return nil, err
		}
		rows = append(rows, database.ConnectionRow{ID: storageID, PayloadEnc: ciphertext, Position: index})
	}
	return rows, nil
}

func decryptConnectionRows(rows []database.ConnectionRow, encryptionKey string) ([]connectionProfile, error) {
	if strings.TrimSpace(encryptionKey) == "" {
		return nil, errors.New("connection encryption key is required")
	}
	profiles := make([]connectionProfile, 0, len(rows))
	for _, row := range rows {
		plaintext, err := security.DecryptPayloadGT(row.PayloadEnc, encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt connection profile: %w", err)
		}
		var profile connectionProfile
		if err := json.Unmarshal([]byte(plaintext), &profile); err != nil {
			return nil, fmt.Errorf("decode connection profile %q: %w", row.ID, err)
		}
		if profile.ID == "" {
			return nil, errors.New("decrypted connection profile id is empty")
		}
		storageID, err := encryptedProfileStorageID(profile.ID, encryptionKey)
		if err != nil || storageID != row.ID {
			return nil, errors.New("decrypted connection profile id does not match its encrypted storage id")
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func encryptedProfileStorageID(profileID, encryptionKey string) (string, error) {
	if profileID == "" {
		return "", nil
	}
	storageID, err := security.EncryptPayloadGT(profileID, encryptionKey)
	if err != nil {
		return "", fmt.Errorf("encrypt connection profile id: %w", err)
	}
	return storageID, nil
}

func decryptedActiveProfileID(profiles []connectionProfile, activeStorageID, encryptionKey string) (string, error) {
	if activeStorageID == "" {
		return normalizedActiveProfileID(profiles, ""), nil
	}
	for _, profile := range profiles {
		storageID, err := encryptedProfileStorageID(profile.ID, encryptionKey)
		if err != nil {
			return "", err
		}
		if storageID == activeStorageID {
			return profile.ID, nil
		}
	}
	return normalizedActiveProfileID(profiles, ""), nil
}

func normalizedActiveProfileID(profiles []connectionProfile, activeID string) string {
	for _, profile := range profiles {
		if profile.ID == activeID {
			return activeID
		}
	}
	if len(profiles) > 0 {
		return profiles[0].ID
	}
	return ""
}

func configuredActiveProfileID() string {
	if config.GlobalConfig == nil {
		return ""
	}
	return config.GlobalConfig.ActiveConnectionID
}
