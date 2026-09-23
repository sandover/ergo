//go:build windows

// Purpose: Complete the common event-log replacement flow on Windows hosts.
// Exports: none (package-internal helper).
// Role: No-op after replaceLogFile requests Windows write-through.
// Invariants: Does not claim parent-directory metadata durability.
package ergo

func syncDir(string) error {
	return nil
}
