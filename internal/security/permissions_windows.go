//go:build windows

package security

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func SecurePrivateDir(path string) error {
	return applyCurrentUserOnlyDACL(path)
}

func SecurePrivateFile(path string) error {
	return applyCurrentUserOnlyDACL(path)
}

func applyCurrentUserOnlyDACL(path string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("get current Windows user: %w", err)
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return fmt.Errorf("build current-user security descriptor: %w", err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return fmt.Errorf("read current-user DACL: %w", err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	); err != nil {
		return fmt.Errorf("apply current-user DACL: %w", err)
	}
	return nil
}
