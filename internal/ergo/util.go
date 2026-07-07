// Purpose: Provide shared helpers for IDs, locking, and time formatting.
// Exports: none (package-internal helpers).
// Role: Low-level utility layer used across command and storage logic.
// Invariants: Lock acquisition is bounded by opts; IDs are 6 characters.
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

const defaultLockTimeout = 30 * time.Second

func effectiveLockTimeout(opts GlobalOptions) time.Duration {
	if opts.LockTimeoutSet {
		return opts.LockTimeout
	}
	return defaultLockTimeout
}

type lockHolderInfo struct {
	PID       int    `json:"pid"`
	Command   string `json:"command,omitempty"`
	Agent     string `json:"agent,omitempty"`
	StartedAt string `json:"started_at"`
	Mode      string `json:"mode"`
}

func withLock(path string, lockType int, opts GlobalOptions, fn func() error) error {
	return withLockMetadata(path, lockType, opts, true, fn)
}

func withLockNoMetadata(path string, lockType int, opts GlobalOptions, fn func() error) error {
	return withLockMetadata(path, lockType, opts, false, fn)
}

func withLockMetadata(path string, lockType int, opts GlobalOptions, writeMetadata bool, fn func() error) error {
	fd, canWriteMetadata, err := openLockFile(path, writeMetadata)
	if err != nil && os.IsNotExist(err) {
		// The lock file is not state; it's just the synchronization primitive.
		// If it's missing, recreate it on demand so normal commands can proceed.
		if err := ensureFileExists(path, 0644); err != nil {
			return err
		}
		fd, canWriteMetadata, err = openLockFile(path, writeMetadata)
	}
	if err != nil {
		return err
	}
	defer syscall.Close(fd)

	timeout := effectiveLockTimeout(opts)
	deadline := time.Now().Add(timeout)
	for {
		if err := syscall.Flock(fd, lockType|syscall.LOCK_NB); err == nil {
			break
		} else {
			if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
				return err
			}
			if timeout == 0 || !time.Now().Before(deadline) {
				return lockBusyError(path, timeout)
			}
		}
		time.Sleep(lockRetryDelay(deadline))
	}
	if writeMetadata && canWriteMetadata {
		writeLockHolderMetadata(fd, opts, lockType)
	}
	defer func() {
		if writeMetadata && canWriteMetadata {
			clearLockHolderMetadata(fd)
		}
		_ = syscall.Flock(fd, syscall.LOCK_UN)
	}()
	return fn()
}

func openLockFile(path string, writeMetadata bool) (int, bool, error) {
	if !writeMetadata {
		fd, err := syscall.Open(path, syscall.O_RDONLY, 0)
		return fd, false, err
	}
	fd, err := syscall.Open(path, syscall.O_RDWR, 0)
	if err != nil && os.IsNotExist(err) {
		return -1, false, err
	}
	if err == nil {
		return fd, true, nil
	}
	fd, fallbackErr := syscall.Open(path, syscall.O_RDONLY, 0)
	if fallbackErr != nil {
		return -1, false, err
	}
	return fd, false, nil
}

func writeLockHolderMetadata(fd int, opts GlobalOptions, lockType int) {
	info := lockHolderInfo{
		PID:       os.Getpid(),
		Command:   opts.Command,
		Agent:     opts.AgentID,
		StartedAt: formatTime(time.Now().UTC()),
		Mode:      lockModeName(lockType),
	}
	data, err := json.Marshal(info)
	if err != nil {
		return
	}
	data = append(data, '\n')
	if _, err := syscall.Seek(fd, 0, 0); err != nil {
		return
	}
	if err := syscall.Ftruncate(fd, 0); err != nil {
		return
	}
	_, _ = syscall.Write(fd, data)
}

func clearLockHolderMetadata(fd int) {
	if _, err := syscall.Seek(fd, 0, 0); err != nil {
		return
	}
	_ = syscall.Ftruncate(fd, 0)
}

func lockModeName(lockType int) string {
	if lockType&syscall.LOCK_EX != 0 {
		return "exclusive"
	}
	return "unknown"
}

func lockBusyError(path string, timeout time.Duration) error {
	suffix := ""
	if timeout > 0 {
		suffix = fmt.Sprintf(" after %s", timeout)
	}
	if holder := readLockHolderSummary(path); holder != "" {
		suffix += "; " + holder
	}
	return fmt.Errorf("%w%s", ErrLockBusy, suffix)
}

func readLockHolderSummary(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		return ""
	}
	var info lockHolderInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return ""
	}
	var parts []string
	if info.PID != 0 {
		parts = append(parts, fmt.Sprintf("pid=%d", info.PID))
	}
	if info.Agent != "" {
		parts = append(parts, "agent="+info.Agent)
	}
	if info.Command != "" {
		parts = append(parts, "command="+info.Command)
	}
	if info.StartedAt != "" {
		parts = append(parts, "started_at="+info.StartedAt)
	}
	if info.Mode != "" {
		parts = append(parts, "mode="+info.Mode)
	}
	if len(parts) == 0 {
		return ""
	}
	return "holder " + strings.Join(parts, " ")
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
