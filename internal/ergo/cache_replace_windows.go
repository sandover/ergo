//go:build windows

// Purpose: Atomically replace a complete disposable cache on Windows.
// Role: Publication primitive; authoritative files use their durable writer.
// Invariants: The destination is never removed before replacement.
package ergo

import "golang.org/x/sys/windows"

func replaceCacheFile(source, destination string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING)
}
