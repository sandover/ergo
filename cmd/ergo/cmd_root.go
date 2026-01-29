// Root command configuration for the ergo CLI.
// Defines global flags, help output, and top-level command metadata.
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
	rootCmd.PersistentFlags().BoolVar(&globalOpts.ReadOnly, "readonly", false, "Run in read-only mode")
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
