// Body IO helpers and stdin/tty detection.
package main

import (
	"io"
	"os"
)

func readBodyFile(path string) (string, error) {
	if path == "-" {
		body, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return string(body), nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func stdinIsPiped() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) == 0
}

func stdoutIsTTY() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) == 0
}
