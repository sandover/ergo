// Purpose: Provide shared helpers for IDs, locking, and time formatting.
// Exports: none (package-internal helpers).
// Role: Low-level utility layer used across command and storage logic.
// Invariants: Lock acquisition is non-blocking; IDs are 6 characters.
// Notes: Time formatting uses RFC3339Nano in UTC.
package ergo

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"
)

func sortedKeys(items map[string]struct{}) []string {
	if len(items) == 0 {
		return nil
	}
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedMapKeys(items map[string]map[string]struct{}) []string {
	if len(items) == 0 {
		return nil
	}
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func withLock(path string, lockType int, fn func() error) error {
	fd, err := syscall.Open(path, syscall.O_RDONLY, 0)
	if err != nil && os.IsNotExist(err) {
		// The lock file is not state; it's just the synchronization primitive.
		// If it's missing, recreate it on demand so normal commands can proceed.
		if err := ensureFileExists(path, 0644); err != nil {
			return err
		}
		fd, err = syscall.Open(path, syscall.O_RDONLY, 0)
	}
	if err != nil {
		return err
	}
	defer syscall.Close(fd)

	// Fail-fast: non-blocking lock attempt only
	if err := syscall.Flock(fd, lockType|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return ErrLockBusy
		}
		return err
	}
	defer func() {
		_ = syscall.Flock(fd, syscall.LOCK_UN)
	}()
	return fn()
}

func ensureFileExists(path string, mode os.FileMode) error {
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("%s is a directory", path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(path, []byte{}, mode); err != nil {
		return fmt.Errorf("cannot create %s: %w", path, err)
	}
	return nil
}

func newEvent(eventType string, ts time.Time, payload interface{}) (Event, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	return Event{Type: eventType, TS: formatTime(ts), Data: data}, nil
}

func newShortID(existing map[string]*Task) (string, error) {
	const maxAttempts = 64
	for i := 0; i < maxAttempts; i++ {
		id, err := shortID()
		if err != nil {
			return "", err
		}
		if _, exists := existing[id]; !exists {
			return id, nil
		}
	}
	return "", errors.New("failed to generate unique id")
}

func shortID() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	return strings.ToUpper(encoded[:6]), nil
}

func newUUID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4],
		buf[4:6],
		buf[6:8],
		buf[8:10],
		buf[10:16],
	), nil
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func prunedErr(id string) error {
	return fmt.Errorf("id %s is pruned", id)
}

func pickTime(candidate, fallback time.Time) time.Time {
	if !candidate.IsZero() {
		return candidate
	}
	return fallback
}

func maxTime(current, next time.Time) time.Time {
	if next.After(current) {
		return next
	}
	return current
}
