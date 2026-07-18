//go:build !windows

// Purpose: Provide non-blocking advisory file locks on Unix hosts.
// Exports: none (package-internal helpers).
// Role: Platform implementation used by withLock.
// Invariants: Locks are exclusive and held for the lifetime of the open file.
package ergo

import (
	"errors"
	"os"
	"syscall"
)

func tryFileLock(file *os.File) (bool, error) {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return false, err
}

func unlockFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
