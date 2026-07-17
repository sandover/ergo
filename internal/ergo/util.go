// Purpose: Provide shared helpers for IDs, locking, and time formatting.
// Exports: none (package-internal helpers).
// Role: Low-level utility layer used across command and storage logic.
// Invariants: Lock acquisition is briefly bounded; IDs are 6 characters.
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

const defaultLockTimeout = 10 * time.Second

func withLock(path string, opts GlobalOptions, fn func() error) error {
	_ = opts
	lockFile, err := os.Open(path)
	if err != nil && os.IsNotExist(err) {
		// The lock file is not state; it's just the synchronization primitive.
		// If it's missing, recreate it on demand so normal commands can proceed.
		if err := ensureFileExists(path, 0644); err != nil {
			return err
		}
		lockFile, err = os.Open(path)
	}
	if err != nil {
		return err
	}
	defer lockFile.Close()

	deadline := time.Now().Add(defaultLockTimeout)
	for {
		locked, err := tryFileLock(lockFile)
		if err != nil {
			return err
		}
		if locked {
			break
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%w after %s", ErrLockBusy, defaultLockTimeout)
		}
		time.Sleep(lockRetryDelay(deadline))
	}
	defer func() {
		_ = unlockFile(lockFile)
	}()
	return fn()
}

func lockRetryDelay(deadline time.Time) time.Duration {
	delay := 10*time.Millisecond + time.Duration(time.Now().UnixNano()%int64(20*time.Millisecond))
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return 0
	}
	if delay > remaining {
		return remaining
	}
	return delay
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
