package ergo

import (
	"fmt"
	"io"
)

func RenderVersion(w io.Writer, outcome VersionOutcome) {
	fmt.Fprintln(w, "ergo "+outcome.Version)
}
