//go:build !windows

// Purpose: Identify an opened backlog file across normal appends on Unix.
// Role: Cheap cache invalidation for replacement and truncation detection.
// Invariants: Identity comes from the open descriptor, never its pathname.
package ergo

import (
	"fmt"
	"os"
	"syscall"
)

func sourceFileIdentity(_ *os.File, info os.FileInfo) (string, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("unix:%d:%d", uint64(stat.Dev), uint64(stat.Ino)), true
}
