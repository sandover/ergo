package ergo

import (
	"fmt"
	"os"
	"time"
)

const defaultLockTimeout = 10 * time.Second

func repositoryWithLock(path string, opts GlobalOptions, fn func() error) error {
	_ = opts
	lockFile, err := os.Open(path)
	if err != nil && os.IsNotExist(err) {
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
	defer func() { _ = unlockFile(lockFile) }()
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
