// Purpose: Wire cobra subcommands to internal ergo.RunX implementations.
// Exports: none.
// Role: CLI composition layer for user-facing commands.
// Invariants: Flags and command names align with help/quickstart docs.
// Notes: init functions register commands and their flags.
package main

import (
	"fmt"

	"github.com/sandover/ergo/internal/ergo"
	"github.com/spf13/cobra"
)

func init() {
	// ergo init
	rootCmd.AddCommand(initCmd)
	// ergo new
	rootCmd.AddCommand(newCmd)
	newCmd.AddCommand(newTaskCmd)
	// ergo plan
	rootCmd.AddCommand(planCmd)
	// ergo list
	rootCmd.AddCommand(listCmd)
	// ergo show
	rootCmd.AddCommand(showCmd)
	// ergo claim
	rootCmd.AddCommand(claimCmd)
	// ergo set
	rootCmd.AddCommand(setCmd)
	// ergo sequence
	rootCmd.AddCommand(sequenceCmd)
	// ergo where
	rootCmd.AddCommand(whereCmd)
	// ergo compact
	rootCmd.AddCommand(compactCmd)
	// ergo prune
	rootCmd.AddCommand(pruneCmd)
	// ergo quickstart
	rootCmd.AddCommand(quickstartCmd)
	// ergo version
	rootCmd.AddCommand(versionCmd)
}

// -- init --
var initCmd = &cobra.Command{
	Use:   "init [dir]",
	Short: "Initialize ergo in the current (or specified) directory",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunInit(args, globalOpts)
	},
}

// -- new --
var newCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new task",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}
		return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var newTaskCmd = &cobra.Command{
	Use:   "task [json]",
	Short: "Create a new task (stdin = body)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunNewTask(args, globalOpts)
	},
}

// -- list --
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		epicID, _ := cmd.Flags().GetString("epic")
		readyOnly, _ := cmd.Flags().GetBool("ready")
		showAll, _ := cmd.Flags().GetBool("all")
		return ergo.RunList(ergo.ListOptions{
			EpicID:    epicID,
			ReadyOnly: readyOnly,
			ShowAll:   showAll,
		}, globalOpts)
	},
}

func init() {
	listCmd.Flags().String("epic", "", "Filter by container ID")
	listCmd.Flags().Bool("ready", false, "Show only ready tasks")
	listCmd.Flags().Bool("all", false, "Show all tasks (including canceled/done)")
}

// -- show --
var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show task details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunShow(args[0], globalOpts)
	},
}

// -- claim --
var claimCmd = &cobra.Command{
	Use:   "claim [<id>]",
	Short: "Claim a task (or oldest ready task)",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		agentID, _ := cmd.Flags().GetString("agent")

		opts := globalOpts
		if agentID != "" {
			opts.AgentID = agentID
		}

		if len(args) == 0 {
			return ergo.RunClaimOldestReady(opts)
		}
		return ergo.RunClaim(args[0], opts)
	},
}

func init() {
	claimCmd.Flags().String("agent", "", "Claim identity (required; suggested: model@host)")
}

// -- set --
var setCmd = &cobra.Command{
	Use:   "set <id> [json]",
	Short: "Update a task (stdin = new body)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunSet(args[0], args[1:], globalOpts)
	},
}

var (
	planFile string
)

var planCmd = &cobra.Command{
	Use:   "plan [json]",
	Short: "Create a container and children from a markdown file",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunPlan(planFile, args, globalOpts)
	},
}

func init() {
	planCmd.Flags().StringVar(&planFile, "file", "", "Markdown file with # Title chunks separated by ---")
}

// -- sequence --
var sequenceCmd = &cobra.Command{
	Use:   "sequence <A> <B> [<C>...] | sequence rm <A> <B>",
	Short: "Enforce task order (A then B then C)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunSequence(args, globalOpts)
	},
}

// -- where --
var whereCmd = &cobra.Command{
	Use:   "where",
	Short: "Show ergo directory path",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunWhere(globalOpts)
	},
}

// -- compact --
var compactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Compact the event log",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunCompact(globalOpts)
	},
}

// -- prune --
var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Prune closed work (dry-run by default)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		confirm, _ := cmd.Flags().GetBool("yes")
		return ergo.RunPrune(confirm, globalOpts)
	},
}

func init() {
	pruneCmd.Flags().Bool("yes", false, "Apply prune (default is dry-run)")
}

// -- quickstart --
var quickstartCmd = &cobra.Command{
	Use:   "quickstart",
	Short: "Show quickstart guide",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunQuickstart(args)
	},
}

// -- version --
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version",
	Run: func(cmd *cobra.Command, args []string) {
		printVersion()
	},
}
