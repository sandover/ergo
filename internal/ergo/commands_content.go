// Purpose: Implement direct title and body replacement commands.
// Exports: RunTitle and RunBody.
// Role: Map focused content edits onto the shared atomic mutation path.
// Invariants: titles are nonblank after trimming; bodies remain literal text.
// Invariants: body input must come from a pipe, including an empty pipe.
package ergo

import (
	"errors"
	"fmt"
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
	_, err = applyTaskMutation(dir, opts, id, taskMutation{
		Kind: "title", Title: title, TitleSet: true,
	})
	if err != nil {
		return err
	}
	fmt.Printf("%s title: %s\n", id, title)
	return nil
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
	_, err = applyTaskMutation(dir, opts, id, taskMutation{
		Kind: "body", Body: string(body), BodySet: true,
	})
	if err != nil {
		return err
	}
	fmt.Printf("%s body updated\n", id)
	return nil
}
