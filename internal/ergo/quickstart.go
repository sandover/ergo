// Purpose: Serve quickstart command output.
// Exports: RunQuickstart.
// Role: CLI handler for quickstart.
// Invariants: Rejects any args; outputs the full guide.
// Notes: Uses QuickstartText for formatting.
package ergo

import (
	"errors"
	"fmt"
)

func RunQuickstart(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: ergo quickstart")
	}
	fmt.Println(QuickstartText(stdoutIsTTY()))
	return nil
}
