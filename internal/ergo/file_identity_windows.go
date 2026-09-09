//go:build windows

// Purpose: Identify an opened backlog file across normal appends on Windows.
// Role: Cheap cache invalidation for replacement and truncation detection.
// Invariants: Identity comes from the open handle, never its pathname.
package ergo

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func sourceFileIdentity(file *os.File, _ os.FileInfo) (string, bool) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return "", false
	}
	return fmt.Sprintf("windows:%d:%d:%d", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), true
}
