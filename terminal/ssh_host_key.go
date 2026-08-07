package terminal

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/0xdevelop/NBTerminal/internal/persistence"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var (
	ErrSSHHostKeyVerifierRequired = errors.New("SSH host key verifier is required")
	ErrSSHHostKeyChanged          = errors.New("SSH host key has changed")
)

type SSHHostKeyErrorKind uint8

const (
	SSHHostKeyUnknown SSHHostKeyErrorKind = iota + 1
	SSHHostKeyChanged
)

// SSHHostKeyError reports only the trust state and public-key fingerprint.
// Host identities and key material stay private so logs can safely include the
// error string without leaking saved connection details.
type SSHHostKeyError struct {
	kind        SSHHostKeyErrorKind
	fingerprint string
	hostname    string
	key         ssh.PublicKey
}

func (e *SSHHostKeyError) Error() string {
	if e != nil && e.kind == SSHHostKeyChanged {
		return ErrSSHHostKeyChanged.Error()
	}
	return "SSH host key is not trusted"
}

func (e *SSHHostKeyError) Unwrap() error {
	if e != nil && e.kind == SSHHostKeyChanged {
		return ErrSSHHostKeyChanged
	}
	return nil
}

func (e *SSHHostKeyError) Kind() SSHHostKeyErrorKind {
	if e == nil {
		return 0
	}
	return e.kind
}

func (e *SSHHostKeyError) Fingerprint() string {
	if e == nil {
		return ""
	}
	return e.fingerprint
}

// SSHHostKeyStore provides strict known_hosts verification with hashed host
// identities. Unknown keys require an explicit Trust call; changed keys can
// never be replaced through this API.
type SSHHostKeyStore struct {
	path string
	mu   sync.Mutex
}

func NewSSHHostKeyStore(path string) *SSHHostKeyStore {
	return &SSHHostKeyStore{path: filepath.Clean(path)}
}

func (s *SSHHostKeyStore) Callback() ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if s == nil || strings.TrimSpace(s.path) == "" {
			return ErrSSHHostKeyVerifierRequired
		}
		if key == nil {
			return errors.New("SSH host key is missing")
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.ensureFileLocked(); err != nil {
			return err
		}
		callback, err := knownhosts.New(s.path)
		if err != nil {
			return fmt.Errorf("open SSH host key store: %w", err)
		}
		if err := callback(hostname, remote, key); err == nil {
			return nil
		} else {
			var keyErr *knownhosts.KeyError
			if !errors.As(err, &keyErr) {
				return errors.New("SSH host key verification failed")
			}
			kind := SSHHostKeyUnknown
			if len(keyErr.Want) > 0 {
				kind = SSHHostKeyChanged
			}
			return &SSHHostKeyError{
				kind:        kind,
				fingerprint: ssh.FingerprintSHA256(key),
				hostname:    hostname,
				key:         key,
			}
		}
	}
}

func (s *SSHHostKeyStore) Trust(hostErr *SSHHostKeyError) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return ErrSSHHostKeyVerifierRequired
	}
	if hostErr == nil || hostErr.key == nil || strings.TrimSpace(hostErr.hostname) == "" {
		return errors.New("SSH host key trust request is incomplete")
	}
	if hostErr.kind == SSHHostKeyChanged {
		return ErrSSHHostKeyChanged
	}
	if hostErr.kind != SSHHostKeyUnknown {
		return errors.New("SSH host key trust request is invalid")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureFileLocked(); err != nil {
		return err
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read SSH host key store: %w", err)
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	hashedHost := knownhosts.HashHostname(knownhosts.Normalize(hostErr.hostname))
	data = append(data, knownhosts.Line([]string{hashedHost}, hostErr.key)...)
	data = append(data, '\n')
	if err := persistence.AtomicWriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("persist SSH host key: %w", err)
	}
	return nil
}

func (s *SSHHostKeyStore) ensureFileLocked() error {
	if info, err := os.Lstat(s.path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("SSH host key store must not be a symbolic link")
		}
		if !info.Mode().IsRegular() {
			return errors.New("SSH host key store is not a regular file")
		}
		if err := os.Chmod(s.path, 0o600); err != nil {
			return fmt.Errorf("secure SSH host key store: %w", err)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect SSH host key store: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create SSH host key directory: %w", err)
	}
	if err := persistence.AtomicWriteFile(s.path, nil, 0o600); err != nil {
		return fmt.Errorf("create SSH host key store: %w", err)
	}
	return nil
}
