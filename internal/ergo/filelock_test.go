// Purpose: Verify platform file locks serialize writers and can be released.
// Exports: none (tests only).
// Role: Cross-platform coverage for the locking primitive used by withLock.
// Invariants: A second handle cannot acquire an active lock and can acquire it after unlock.
package ergo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileLockContentionAndRelease(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "lock")
	if err := os.WriteFile(lockPath, nil, 0644); err != nil {
		t.Fatal(err)
	}

	first, err := os.Open(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	second, err := os.Open(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	locked, err := tryFileLock(first)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	if !locked {
		t.Fatal("first lock unexpectedly contended")
	}

	locked, err = tryFileLock(second)
	if err != nil {
		t.Fatalf("contended lock: %v", err)
	}
	if locked {
		t.Fatal("second handle acquired an active exclusive lock")
	}

	if err := unlockFile(first); err != nil {
		t.Fatalf("unlock first handle: %v", err)
	}

	locked, err = tryFileLock(second)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	if !locked {
		t.Fatal("second handle did not acquire the released lock")
	}
	if err := unlockFile(second); err != nil {
		t.Fatalf("unlock second handle: %v", err)
	}
}
