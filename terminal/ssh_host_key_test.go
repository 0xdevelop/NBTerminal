package terminal

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func testSSHHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func withTestHostKeyVerifier(conn Connection) Connection {
	conn.HostKeyCallback = func(string, net.Addr, ssh.PublicKey) error { return nil }
	return conn
}

func TestSSHHostKeyStoreTrustsUnknownKeyWithHashedPrivatePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	store := NewSSHHostKeyStore(path)
	callback := store.Callback()
	remote := &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 2200}
	key := testSSHHostKey(t)

	err := callback("[synthetic.example]:2200", remote, key)
	var hostErr *SSHHostKeyError
	if !errors.As(err, &hostErr) || hostErr.Kind() != SSHHostKeyUnknown {
		t.Fatalf("unknown key error = %T %v", err, err)
	}
	if fingerprint := hostErr.Fingerprint(); !strings.HasPrefix(fingerprint, "SHA256:") {
		t.Fatalf("fingerprint = %q", fingerprint)
	}
	if err := store.Trust(hostErr); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	if err := callback("[synthetic.example]:2200", remote, key); err != nil {
		t.Fatalf("trusted key rejected: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("known_hosts mode = %o", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "synthetic.example") || !strings.Contains(string(data), "|1|") {
		t.Fatalf("known_hosts must hash host identity, content=%q", string(data))
	}
}

func TestSSHHostKeyStoreRejectsChangedKeyAndRefusesReplacement(t *testing.T) {
	store := NewSSHHostKeyStore(filepath.Join(t.TempDir(), "known_hosts"))
	callback := store.Callback()
	remote := &net.TCPAddr{IP: net.ParseIP("192.0.2.20"), Port: 22}
	first := testSSHHostKey(t)

	err := callback("synthetic.example:22", remote, first)
	var unknown *SSHHostKeyError
	if !errors.As(err, &unknown) {
		t.Fatalf("first key error = %T %v", err, err)
	}
	if err := store.Trust(unknown); err != nil {
		t.Fatal(err)
	}

	err = callback("synthetic.example:22", remote, testSSHHostKey(t))
	var changed *SSHHostKeyError
	if !errors.As(err, &changed) || changed.Kind() != SSHHostKeyChanged {
		t.Fatalf("changed key error = %T %v", err, err)
	}
	if err := store.Trust(changed); !errors.Is(err, ErrSSHHostKeyChanged) {
		t.Fatalf("changed key trust error = %v", err)
	}
}

func TestSSHHostKeyStoreRejectsStaleUnknownTrustAfterAnotherKeyWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	store := NewSSHHostKeyStore(path)
	callback := store.Callback()
	remote := &net.TCPAddr{IP: net.ParseIP("192.0.2.21"), Port: 22}
	first := testSSHHostKey(t)
	second := testSSHHostKey(t)

	var firstUnknown, secondUnknown *SSHHostKeyError
	if !errors.As(callback("synthetic.example:22", remote, first), &firstUnknown) {
		t.Fatal("first unknown key did not produce a typed trust request")
	}
	if !errors.As(callback("synthetic.example:22", remote, second), &secondUnknown) {
		t.Fatal("second unknown key did not produce a typed trust request")
	}
	if err := store.Trust(firstUnknown); err != nil {
		t.Fatalf("trust first key: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Trust(secondUnknown); !errors.Is(err, ErrSSHHostKeyChanged) {
		t.Fatalf("stale trust error = %v, want ErrSSHHostKeyChanged", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("stale trust request modified the host-key store")
	}
}

func TestSSHHostKeyStoreTreatsRepeatedTrustOfSameKeyAsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	store := NewSSHHostKeyStore(path)
	remote := &net.TCPAddr{IP: net.ParseIP("192.0.2.22"), Port: 22}
	key := testSSHHostKey(t)
	var unknown *SSHHostKeyError
	if !errors.As(store.Callback()("synthetic.example:22", remote, key), &unknown) {
		t.Fatal("unknown key did not produce a typed trust request")
	}
	if err := store.Trust(unknown); err != nil {
		t.Fatalf("first trust: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Trust(unknown); err != nil {
		t.Fatalf("repeated trust should be idempotent: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("repeated trust duplicated the known-host entry")
	}
}

func TestSSHHostKeyStoreRefusesSymlinkTrustFile(t *testing.T) {
	temp := t.TempDir()
	legitimate := NewSSHHostKeyStore(filepath.Join(temp, "legitimate_known_hosts"))
	remote := &net.TCPAddr{IP: net.ParseIP("192.0.2.30"), Port: 22}
	err := legitimate.Callback()("synthetic.example:22", remote, testSSHHostKey(t))
	var unknown *SSHHostKeyError
	if !errors.As(err, &unknown) {
		t.Fatalf("unknown key error = %T %v", err, err)
	}

	target := filepath.Join(temp, "target")
	if err := os.WriteFile(target, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(temp, "known_hosts")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := NewSSHHostKeyStore(link).Trust(unknown); err == nil {
		t.Fatal("Trust unexpectedly followed known_hosts symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "preserve" {
		t.Fatalf("symlink target was modified: %q", data)
	}
}

func TestSSHClientConfigRequiresExplicitHostKeyVerifier(t *testing.T) {
	conn := Connection{ID: "remote", Name: "Remote", Type: ConnectionTypeSSH, Host: "synthetic.example", Port: 22, Username: "tester", Password: "synthetic-secret"}
	if _, err := sshClientConfig(conn); !errors.Is(err, ErrSSHHostKeyVerifierRequired) {
		t.Fatalf("missing host key verifier error = %v", err)
	}
	conn = withTestHostKeyVerifier(conn)
	cfg, err := sshClientConfig(conn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HostKeyCallback == nil {
		t.Fatal("explicit host key callback was not preserved")
	}
}
