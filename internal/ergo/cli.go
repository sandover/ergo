// CLI option and flag parsing helpers.
package ergo

import (
	"fmt"
	"os"
)

func debugf(opts GlobalOptions, format string, args ...any) {
	if !opts.Verbose {
		return
	}
	fmt.Fprintf(os.Stderr, "debug: "+format+"\n", args...)
}
