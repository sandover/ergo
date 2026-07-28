//go:build unix

package ergo

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestValidateResultPathRejectsFIFOWithoutBlocking(t *testing.T) {
	repoDir := t.TempDir()
	fifo := filepath.Join(repoDir, "stream")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := validateResultPath(repoDir, "stream")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("FIFO validation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("FIFO validation blocked")
	}
}

func TestValidateResultPathRejectsOtherSpecialFiles(t *testing.T) {
	repoDir := t.TempDir()
	socket := filepath.Join(repoDir, "socket")
	listener, err := netListenUnix(socket)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	defer listener.Close()

	if _, err := validateResultPath(repoDir, "socket"); err == nil ||
		!strings.Contains(err.Error(), "regular file") {
		t.Fatalf("socket validation error = %v", err)
	}
}

func netListenUnix(path string) (*os.File, error) {
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}
	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: path}); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
