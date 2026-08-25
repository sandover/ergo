// Purpose: Verify repository-level shared/exclusive coordination and timeout behavior.
// Exports: none (tests only).
// Role: Deterministic coverage above the platform file-lock primitive.
// Invariants: readers overlap, writers wait, and expired callbacks never run.
package ergo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRepositoryLockReadersOverlapAndWriterWaits(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "lock")
	if err := os.WriteFile(lockPath, nil, 0644); err != nil {
		t.Fatal(err)
	}

	releaseReader := make(chan struct{})
	firstEntered := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- repositoryWithLock(lockPath, GlobalOptions{}, repositoryLockShared, func() error {
			close(firstEntered)
			<-releaseReader
			return nil
		})
	}()
	awaitSignal(t, firstEntered, "first reader")

	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- repositoryWithLock(lockPath, GlobalOptions{}, repositoryLockShared, func() error {
			close(secondEntered)
			return nil
		})
	}()
	awaitSignal(t, secondEntered, "second reader")
	if err := <-secondDone; err != nil {
		t.Fatalf("second reader: %v", err)
	}

	writerEntered := make(chan struct{})
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- repositoryWithLock(lockPath, GlobalOptions{}, repositoryLockExclusive, func() error {
			close(writerEntered)
			return nil
		})
	}()
	select {
	case <-writerEntered:
		t.Fatal("writer entered while a reader held the lock")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseReader)
	if err := <-firstDone; err != nil {
		t.Fatalf("first reader: %v", err)
	}
	awaitSignal(t, writerEntered, "writer after reader release")
	if err := <-writerDone; err != nil {
		t.Fatalf("writer: %v", err)
	}
}

func TestRepositoryLockTimeoutDoesNotRunCallback(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "lock")
	if err := os.WriteFile(lockPath, nil, 0644); err != nil {
		t.Fatal(err)
	}
	held, err := os.Open(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	locked, err := tryFileLock(held, repositoryLockExclusive)
	if err != nil || !locked {
		t.Fatalf("hold exclusive lock: locked=%v err=%v", locked, err)
	}
	defer func() {
		if err := unlockFile(held); err != nil {
			t.Errorf("unlock held lock: %v", err)
		}
	}()

	const timeout = 120 * time.Millisecond
	called := false
	started := time.Now()
	err = repositoryWithLockTimeout(lockPath, GlobalOptions{}, repositoryLockShared, timeout, func() error {
		called = true
		return nil
	})
	elapsed := time.Since(started)
	if !errors.Is(err, ErrLockBusy) {
		t.Fatalf("timeout error = %v, want ErrLockBusy", err)
	}
	if called {
		t.Fatal("protected callback ran after lock timeout")
	}
	if elapsed < timeout {
		t.Fatalf("returned before timeout: %s < %s", elapsed, timeout)
	}
	if elapsed > 4*timeout {
		t.Fatalf("returned too late: %s > %s", elapsed, 4*timeout)
	}
}

func awaitSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}
