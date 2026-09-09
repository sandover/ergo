//go:build !windows

// Purpose: Atomically replace a complete disposable cache on Unix.
// Role: Publication primitive; authoritative files use their durable writer.
// Invariants: The destination is never removed before replacement.
package ergo

import "os"

func replaceCacheFile(source, destination string) error {
	return os.Rename(source, destination)
}
