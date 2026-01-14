// Text formatting: ANSI codes and marker substitution for embedded help/quickstart.
//
// DOCUMENTATION PHILOSOPHY:
//
// ergo has two documentation surfaces, each with a distinct purpose:
//
//   --help (help.txt):
//     The QUICK REFERENCE. A one-screen overview for users who already know ergo
//     or need a command reminder. Think: cheat sheet, not tutorial.
//     - Lists all commands with one-line descriptions
//     - Shows flag syntax without extended explanation
//     - Fits in a terminal without scrolling
//     - Agents use this for command syntax lookup
//
//   quickstart (quickstart.txt):
//     The COMPLETE REFERENCE MANUAL. A runnable tutorial that teaches by example.
//     Like a man page, but interactive—users run commands as they read.
//     - Walks through every feature with copy-paste examples
//     - Explains WHY things work the way they do
//     - Covers edge cases, state machines, and rules
//     - Agents consult this when learning ergo or debugging behavior
//
// When editing these files:
//   - help.txt: Brevity is paramount. One line per command. No prose.
//   - quickstart.txt: Completeness is paramount. If it's not here, it's undocumented.
//
// Together, --help + quickstart must provide 100% of what any user or agent needs.
// There is no separate man page, wiki, or external docs to maintain.
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
