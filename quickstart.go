// quickstart command handler.
package main

import (
	"errors"
	"fmt"
)

func runQuickstart(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: ergo quickstart")
	}
	fmt.Println(quickstartText(stdoutIsTTY()))
	return nil
}
