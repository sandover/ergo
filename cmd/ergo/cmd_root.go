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
	Short: "A task graph for coding agents.",
	Long: `ergo gives your AI agents a better place to plan.
Tasks and dependencies persist across sessions, stay visible to humans,
and are safe for concurrent agents. Data lives in the repo as plain text.`,
	SilenceUsage:  true, // Don't print usage on every error
	SilenceErrors: true, // We handle errors in main
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&globalOpts.StartDir, "dir", "", "Run in a specific directory")
	rootCmd.PersistentFlags().StringVar(&globalOpts.AgentID, "agent", "", "Agent ID for claims (suggested: model@host)")

	// Set the version to enable --version flag
	rootCmd.Version = version

	// Override default help to use our custom text
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
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
			return errors.New("--json was removed in Ergo 3; rerun without it")
		case arg == "--summary" || strings.HasPrefix(arg, "--summary="):
			return errors.New("--summary was removed in Ergo 3; use -m <message> instead")
		}
	}
	return nil
}
