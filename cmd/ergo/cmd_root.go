// Purpose: Define the root command and global flags for the ergo CLI.
// Exports: none (package-private root command helpers).
// Role: CLI configuration and help plumbing.
// Invariants: Help text is sourced from internal/ergo UsageText.
// Notes: Global flags must match help/quickstart documentation.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/sandover/ergo/internal/ergo"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	// Root command flags
	globalOpts ergo.GlobalOptions
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "ergo",
	Short: "A dependency-aware backlog for coding agents.",
	Long: `Ergo manages a repository-local backlog shared by agents and humans.
Tasks and dependencies persist across sessions and remain safe under
concurrent work.`,
	SilenceUsage:  true, // Don't print usage on every error
	SilenceErrors: true, // We handle errors in main
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&globalOpts.StartDir, "dir", "", "Run in a specific directory")

	// Set the version to enable --version flag
	rootCmd.Version = version

	// Override default help to use our custom text
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd != rootCmd {
			fmt.Fprint(cmd.OutOrStdout(), cmd.UsageString())
			return
		}
		isTTY := term.IsTerminal(int(os.Stdout.Fd()))
		fmt.Println(ergo.UsageText(isTTY))
	})
}

func execute() {
	if err := removedArgumentError(os.Args[1:]); err != nil {
		exitErr(err, &globalOpts)
	}
	if err := rootCmd.Execute(); err != nil {
		exitErr(err, &globalOpts)
	}
}

func removedArgumentError(args []string) error {
	for _, arg := range args {
		switch {
		case arg == "--json" || strings.HasPrefix(arg, "--json="):
			return errors.New("--json is not accepted; Ergo prints readable text")
		case arg == "--summary" || strings.HasPrefix(arg, "--summary="):
			return errors.New("--summary is not accepted; use -m <message>")
		}
	}
	if rootInvocation(args) == "plan" {
		return errors.New(`plan is not accepted; use ergo new epic "<title>" --file <path>`)
	}
	return nil
}

func rootInvocation(args []string) string {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--dir" {
			index++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return arg
	}
	return ""
}
