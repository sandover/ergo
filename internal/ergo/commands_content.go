// Purpose: Implement direct title and body replacement commands.
// Exports: RunTitle and RunBody.
// Role: Map focused content edits onto the shared atomic mutation path.
// Invariants: titles are nonblank after trimming; bodies remain literal text.
// Invariants: body input must come from a pipe, including an empty pipe.
package ergo

import (
	"fmt"
	"io"
)

func RunTitle(id, title string, opts GlobalOptions, render RenderOptions) error {
	outcome, err := NewApplication(opts).UpdateTitle(UpdateTitleRequest{ID: id, Title: title})
	if err != nil {
		return err
	}
	RenderTitle(render.writer(), outcome)
	return nil
}

func RunBody(id string, body []byte, opts GlobalOptions, render RenderOptions) error {
	outcome, err := NewApplication(opts).UpdateBody(UpdateBodyRequest{ID: id, Body: body})
	if err != nil {
		return err
	}
	RenderBody(render.writer(), outcome)
	return nil
}

func RenderTitle(w io.Writer, outcome UpdateTitleOutcome) {
	if !outcome.Changed {
		fmt.Fprintf(w, "%s - %s (title unchanged)\n", outcome.ID, outcome.Title)
		return
	}
	fmt.Fprintf(w, "%s - %s\n", outcome.ID, outcome.Title)
}

func RenderBody(w io.Writer, outcome UpdateBodyOutcome) {
	if !outcome.Changed {
		fmt.Fprintf(w, "%s body unchanged\n", outcome.ID)
		return
	}
	fmt.Fprintf(w, "%s body: %d bytes\n", outcome.ID, outcome.Bytes)
}
