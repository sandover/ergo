package main

import (
	"github.com/spf13/cobra"
	"github.com/sandover/ergo/internal/ergo"
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
	rootCmd.PersistentFlags().DurationVar(&globalOpts.LockTimeout, "lock-timeout", ergo.DefaultLockTimeout, "Lock wait timeout")
	
	// --as flag needs custom parsing for validation, but for now we can bind to string and validate in PreRun
	var asStr string
	rootCmd.PersistentFlags().StringVar(&asStr, "as", "any", "Filter/act as worker type (any|agent|human)")
	
	// We need to hook into PreRun to parse 'as' into globalOpts.As
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		w, err := ergo.ParseWorker(asStr)
		if err != nil {
			return err
		}
		globalOpts.As = w
		return nil
	}

	rootCmd.PersistentFlags().StringVar(&globalOpts.AgentID, "agent", "", "Agent ID for claims (default: hostname)")
	rootCmd.PersistentFlags().BoolVarP(&globalOpts.Quiet, "quiet", "q", false, "Suppress output")
	rootCmd.PersistentFlags().BoolVarP(&globalOpts.Verbose, "verbose", "v", false, "Verbose output")
}

func execute() {
	if err := rootCmd.Execute(); err != nil {
		exitErr(err, &globalOpts)
	}
}
