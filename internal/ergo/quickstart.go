// quickstart command handler.
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
