// Text formatting: ANSI codes and marker substitution for embedded help/quickstart.
package main

import (
	_ "embed"
	"strings"
)

const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiDim   = "\x1b[2m"
	ansiGreen = "\x1b[32m"
	ansiCyan  = "\x1b[36m"
)

//go:embed help.txt
var helpTextRaw string

//go:embed quickstart.txt
var quickstartTextRaw string

// usageText returns the help text, colorized if color is true.
func usageText(color bool) string {
	return applyMarkers(helpTextRaw, color)
}

// quickstartText returns the quickstart text, colorized if color is true.
func quickstartText(color bool) string {
	return applyMarkers(quickstartTextRaw, color)
}

// applyMarkers replaces {{MARKER}} tokens with ANSI codes or strips them.
func applyMarkers(text string, color bool) string {
	replacements := []struct {
		marker  string
		colored string
		plain   string
	}{
		{"{{BOLD}}", ansiBold, ""},
		{"{{CYAN}}", ansiCyan, ""},
		{"{{DIM}}", ansiDim, ""},
		{"{{GREEN}}", ansiGreen, ""},
		{"{{RESET}}", ansiReset, ""},
		{"{{HEADER}}", ansiBold + ansiCyan, ""},
		{"{{CMD}}", "  " + ansiGreen + "$" + ansiReset + " ", "  $ "},
		{"{{COMMENT}}", "    " + ansiDim + "# ", "    # "},
	}

	for _, r := range replacements {
		if color {
			text = strings.ReplaceAll(text, r.marker, r.colored)
		} else {
			text = strings.ReplaceAll(text, r.marker, r.plain)
		}
	}

	return strings.TrimSuffix(text, "\n")
}
