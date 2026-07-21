// Purpose: Implement direct title and body replacement commands.
// Exports: RunTitle and RunBody.
// Role: Map focused content edits onto the shared atomic mutation path.
// Invariants: titles are nonblank after trimming; bodies remain literal text.
// Invariants: body input must come from a pipe, including an empty pipe.
package ergo

import (
	"errors"
	"io"
	"os"
	"strings"
)

func RunTitle(id, title string, opts GlobalOptions) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title cannot be empty")
	}
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	outcome, err := applyTaskMutation(dir, opts, id, taskMutation{
		Kind: "title", Title: title, TitleSet: true,
	}, opts.JSON)
	if err != nil {
		return err
	}
	return writeMutationResult("title", id, outcome, opts.JSON)
}

func RunBody(id string, opts GlobalOptions) error {
	if !stdinIsPiped() {
		return errors.New("body requires piped stdin; example: printf '%s\\n' '## Goal' | ergo body " + id)
	}
	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	outcome, err := applyTaskMutation(dir, opts, id, taskMutation{
		Kind: "body", Body: string(body), BodySet: true,
	}, opts.JSON)
	if err != nil {
		return err
	}
	return writeMutationResult("body", id, outcome, opts.JSON)
}
