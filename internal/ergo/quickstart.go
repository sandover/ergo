// Purpose: Serve quickstart command output.
// Exports: RunQuickstart.
// Role: CLI handler for quickstart.
// Invariants: Rejects any args; outputs the full guide.
// Notes: Uses QuickstartText for formatting.
package ergo

import (
	"errors"
	"fmt"
	"io"
)

func RunQuickstart(args []string, render RenderOptions) error {
	if len(args) != 0 {
		return errors.New("usage: ergo quickstart")
	}
	outcome := NewApplication(RepositoryOptions{}).Quickstart(QuickstartRequest{Color: render.Color})
	RenderQuickstart(render.writer(), outcome)
	return nil
}

func RenderQuickstart(w io.Writer, outcome QuickstartOutcome) {
	fmt.Fprintln(w, outcome.Text)
}
