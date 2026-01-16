package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

func printVersion() {
	fmt.Println("ergo " + version)
}

func exitErr(err error, opts *GlobalOptions) {
	fmt.Fprintln(os.Stderr, "error:", err)
	if opts == nil || !opts.Quiet {
		if strings.HasPrefix(err.Error(), "usage:") {
			fmt.Fprintln(os.Stderr, "hint: run `ergo --help`")
		} else if errors.Is(err, errNoErgoDir) {
			fmt.Fprintln(os.Stderr, "hint: run `ergo init` in your repo")
		} else if errors.Is(err, errLockBusy) {
			fmt.Fprintln(os.Stderr, "hint: another process is writing; retry or pass `--lock-timeout 30s`")
		} else if errors.Is(err, errLockTimeout) {
			fmt.Fprintln(os.Stderr, "hint: lock wait timed out; retry or increase `--lock-timeout`")
		} else if strings.Contains(err.Error(), "require human") {
			fmt.Fprintln(os.Stderr, "hint: run `ergo ready --as human` and ask the human to handle decision tasks")
		} else if strings.HasPrefix(err.Error(), "readonly:") {
			fmt.Fprintln(os.Stderr, "hint: remove `--readonly` (or switch to a read-only command)")
		}
	}
	os.Exit(1)
}
