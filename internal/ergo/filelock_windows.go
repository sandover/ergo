//go:build windows

// Purpose: Provide non-blocking advisory file locks on Windows hosts.
// Exports: none (package-internal helpers).
// Role: Platform implementation used by withLock.
// Invariants: Locks are exclusive and cover the first byte of the lock file.
package ergo

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func tryFileLock(file *os.File) (bool, error) {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&overlapped,
	)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return false, err
}

func unlockFile(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
}
