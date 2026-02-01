// Purpose: Define the root command and global flags for the ergo CLI.
// Exports: none (package-private root command helpers).
// Role: CLI configuration and help plumbing.
// Invariants: Help text is sourced from internal/ergo UsageText.
// Notes: Global flags must match help/quickstart documentation.
package main

import (
	"fmt"
	"os"

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
	rootCmd.PersistentFlags().StringVar(&globalOpts.AgentID, "agent", "", "Agent ID for claims (required for claim/implicit set; suggested: model@host)")
	rootCmd.PersistentFlags().BoolVarP(&globalOpts.Quiet, "quiet", "q", false, "Suppress output")
	rootCmd.PersistentFlags().BoolVarP(&globalOpts.Verbose, "verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().BoolVar(&globalOpts.JSON, "json", false, "Output JSON")

	// Set the version to enable --version flag
	rootCmd.Version = version

	// Override default help to use our custom text
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		isTTY := term.IsTerminal(int(os.Stdout.Fd()))
		fmt.Println(ergo.UsageText(isTTY))
	})
}

func execute() {
	if err := rootCmd.Execute(); err != nil {
		exitErr(err, &globalOpts)
	}
}
