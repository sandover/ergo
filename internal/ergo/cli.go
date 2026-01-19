// CLI option and flag parsing helpers.
package ergo

import (
	"errors"
	"fmt"
	"os"
	"strings"
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

func parseFlagValue(args []string, name string) (string, error) {
	for i := 0; i < len(args); i++ {
		if args[i] == name {
			if i+1 >= len(args) {
				return "", fmt.Errorf("missing value for %s", name)
			}
			return args[i+1], nil
		}
	}
	return "", nil
}

type outputFormat string

const (
	outputFormatText outputFormat = "text"
	outputFormatJSON outputFormat = "json"
)

func parseOutputFormat(args []string, fallback outputFormat) (outputFormat, error) {
	formatValue, err := parseFlagValue(args, "--format")
	if err != nil {
		return "", err
	}
	if hasFlag(args, "--format") || formatValue != "" {
		return "", errors.New("unsupported flag --format (use --json)")
	}
	if hasFlag(args, "--json") {
		return outputFormatJSON, nil
	}
	return fallback, nil
}

func parseOutputFormatAndArgs(args []string, fallback outputFormat) (outputFormat, []string, error) {
	format, err := parseOutputFormat(args, fallback)
	if err != nil {
		return "", nil, err
	}
	remaining, err := stripOutputFlags(args)
	if err != nil {
		return "", nil, err
	}
	return format, remaining, nil
}

func parseShowArgs(args []string) (string, outputFormat, bool, error) {
	format, err := parseOutputFormat(args, outputFormatText)
	if err != nil {
		return "", "", false, err
	}

	var id string
	short := false
	for _, arg := range args {
		switch arg {
		case "--json":
			continue
		case "--short":
			short = true
			continue
		case "--format":
			return "", "", false, errors.New("unsupported flag --format (use --json)")
		}
		if strings.HasPrefix(arg, "-") {
			return "", "", false, fmt.Errorf("unknown flag %s", arg)
		}
		if id != "" {
			return "", "", false, errors.New("usage: ergo show <id> [--short] [--json]")
		}
		id = arg
	}
	if id == "" {
		return "", "", false, errors.New("usage: ergo show <id> [--short] [--json]")
	}
	if short && format == outputFormatJSON {
		return "", "", false, errors.New("conflicting flags: --short and --json")
	}
	return id, format, short, nil
}

func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}

func stripOutputFlags(args []string) ([]string, error) {
	stripped := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			continue
		case "--format":
			return nil, errors.New("unsupported flag --format (use --json)")
		default:
			stripped = append(stripped, args[i])
		}
	}
	return stripped, nil
}
