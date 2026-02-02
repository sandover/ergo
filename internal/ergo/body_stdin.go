// Purpose: Provide `--body-stdin` helpers for treating stdin as literal body text.
// Exports: readBodyFromStdinOrEmpty, validateBodyStdinExclusions, buildFlagUpdates.
// Role: Shared glue used by `new`/`set` command handlers for the body-stdin input mode.
// Invariants: `--body-stdin` forbids `--body`; stdin is never parsed as JSON in this mode.
// Notes: When stdin is a TTY (not piped), body is treated as empty to avoid blocking reads.
package ergo

import (
	"errors"
	"io"
	"os"
	"strings"
)

func readBodyFromStdinOrEmpty() (string, error) {
	if !stdinIsPiped() {
		return "", nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func validateBodyStdinExclusions(inlineBody string) error {
	if inlineBody == "" {
		return nil
	}
	return errors.New("--body and --body-stdin are mutually exclusive")
}

func buildFlagUpdates(opts GlobalOptions) map[string]string {
	updates := make(map[string]string)

	title := strings.TrimSpace(opts.TitleFlag)
	if title != "" {
		updates["title"] = title
	}
	if opts.EpicFlag != "" {
		updates["epic"] = opts.EpicFlag
	}
	if opts.StateFlag != "" {
		updates["state"] = opts.StateFlag
	}
	if opts.ClaimFlag != "" {
		updates["claim"] = opts.ClaimFlag
	}
	if opts.ResultPathFlag != "" {
		updates["result.path"] = opts.ResultPathFlag
	}
	if opts.ResultSummaryFlag != "" {
		updates["result.summary"] = opts.ResultSummaryFlag
	}

	return updates
}
