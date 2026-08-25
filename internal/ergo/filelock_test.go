// Purpose: Verify platform file locks distinguish readers from writers and can be released.
// Exports: none (tests only).
// Role: Cross-platform coverage for the locking primitive used by withLock.
// Invariants: Readers may overlap; writers exclude readers and other writers.
package ergo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileLockModesAndRelease(t *testing.T) {
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

	locked, err := tryFileLock(first, repositoryLockShared)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	if !locked {
		t.Fatal("first lock unexpectedly contended")
	}

	locked, err = tryFileLock(second, repositoryLockShared)
	if err != nil {
		t.Fatalf("contended lock: %v", err)
	}
	if !locked {
		t.Fatal("second reader did not acquire a shared lock")
	}
	if err := unlockFile(second); err != nil {
		t.Fatalf("unlock second reader: %v", err)
	}

	locked, err = tryFileLock(second, repositoryLockExclusive)
	if err != nil {
		t.Fatalf("writer behind reader: %v", err)
	}
	if locked {
		t.Fatal("writer acquired a lock while a reader held one")
	}

	if err := unlockFile(first); err != nil {
		t.Fatalf("unlock first handle: %v", err)
	}

	locked, err = tryFileLock(second, repositoryLockExclusive)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	if !locked {
		t.Fatal("second handle did not acquire the released lock")
	}
	locked, err = tryFileLock(first, repositoryLockExclusive)
	if err != nil {
		t.Fatalf("second writer behind writer: %v", err)
	}
	if locked {
		t.Fatal("second writer acquired an active exclusive lock")
	}
	if err := unlockFile(second); err != nil {
		t.Fatalf("unlock second handle: %v", err)
	}
}

func TestExclusiveFileLockBlocksReader(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "lock")
	if err := os.WriteFile(lockPath, nil, 0644); err != nil {
		t.Fatal(err)
	}

	writer, err := os.Open(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	reader, err := os.Open(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	locked, err := tryFileLock(writer, repositoryLockExclusive)
	if err != nil || !locked {
		t.Fatalf("exclusive lock: locked=%v err=%v", locked, err)
	}
	locked, err = tryFileLock(reader, repositoryLockShared)
	if err != nil {
		t.Fatalf("reader behind writer: %v", err)
	}
	if locked {
		t.Fatal("reader acquired a lock while a writer held one")
	}
	if err := unlockFile(writer); err != nil {
		t.Fatalf("unlock writer: %v", err)
	}
}
