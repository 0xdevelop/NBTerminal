//go:build !windows

package security

import "os"

func SecurePrivateDir(path string) error {
	return os.Chmod(path, 0o700)
}

func SecurePrivateFile(path string) error {
	return os.Chmod(path, 0o600)
}
