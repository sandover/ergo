// CLI option and flag parsing helpers.
package ergo

import (
	"fmt"
	"os"
)

func requireWritable(opts GlobalOptions, what string) error {
	if opts.ReadOnly {
		return fmt.Errorf("readonly: %s", what)
	}
	return nil
}

func debugf(opts GlobalOptions, format string, args ...any) {
	if !opts.Verbose {
		return
	}
	fmt.Fprintf(os.Stderr, "debug: "+format+"\n", args...)
}
