// Cobra command wiring for ergo subcommands.
// Purpose: bind CLI verbs/flags to internal ergo.RunX implementations.
// Exports: none.
// Role: CLI composition layer for user-facing commands.
// Invariants: flags must match help/quickstart documentation.
package main

import (
	"github.com/sandover/ergo/internal/ergo"
	"github.com/spf13/cobra"
)

// Wrapper functions to adapt Cobra commands to existing RunX functions
// or implement new Cobra logic while reusing existing business logic.

func init() {
	// ergo init
	rootCmd.AddCommand(initCmd)
	// ergo new
	rootCmd.AddCommand(newCmd)
	newCmd.AddCommand(newTaskCmd)
	newCmd.AddCommand(newEpicCmd)
	// ergo list
	rootCmd.AddCommand(listCmd)
	// ergo show
	rootCmd.AddCommand(showCmd)
	// ergo claim
	rootCmd.AddCommand(claimCmd)
	// ergo set
	rootCmd.AddCommand(setCmd)
	// ergo dep
	rootCmd.AddCommand(depCmd)
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
	Short: "Create a new task or epic",
}

var newTaskCmd = &cobra.Command{
	Use:   "task",
	Short: "Create a new task (JSON stdin)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunNewTask(globalOpts)
	},
}

var newEpicCmd = &cobra.Command{
	Use:   "epic",
	Short: "Create a new epic (JSON stdin)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunNewEpic(globalOpts)
	},
}

func init() {
}

// -- list --
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		epicID, _ := cmd.Flags().GetString("epic")
		readyOnly, _ := cmd.Flags().GetBool("ready")
		showEpics, _ := cmd.Flags().GetBool("epics")
		showAll, _ := cmd.Flags().GetBool("all")
		return ergo.RunList(ergo.ListOptions{
			EpicID:    epicID,
			ReadyOnly: readyOnly,
			ShowEpics: showEpics,
			ShowAll:   showAll,
		}, globalOpts)
	},
}

func init() {
	listCmd.Flags().String("epic", "", "Filter by epic ID")
	listCmd.Flags().Bool("ready", false, "Show only ready tasks")
	listCmd.Flags().Bool("epics", false, "Show only epics")
	listCmd.Flags().Bool("all", false, "Show all tasks (including canceled/done)")
}

// -- show --
var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show task details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		short, _ := cmd.Flags().GetBool("short")
		return ergo.RunShow(args[0], short, globalOpts)
	},
}

func init() {
	showCmd.Flags().Bool("short", false, "Short output format")
}

// -- claim --
var claimCmd = &cobra.Command{
	Use:   "claim [<id>]",
	Short: "Claim a task (or oldest ready task)",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		agentID, _ := cmd.Flags().GetString("agent")
		epicID, _ := cmd.Flags().GetString("epic")

		opts := globalOpts
		if agentID != "" {
			opts.AgentID = agentID
		}

		if len(args) == 0 {
			return ergo.RunClaimOldestReady(epicID, opts)
		}
		return ergo.RunClaim(args[0], opts)
	},
}

func init() {
	claimCmd.Flags().String("agent", "", "Claim identity (required; suggested: model@host)")
	claimCmd.Flags().String("epic", "", "Filter to tasks in this epic")
}

// -- set --
var setCmd = &cobra.Command{
	Use:   "set <id>",
	Short: "Update a task (JSON stdin)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunSet(args[0], globalOpts)
	},
}

func init() {
}

// -- dep --
var depCmd = &cobra.Command{
	Use:   "dep <A> <B> | dep rm <A> <B>",
	Short: "Manage dependencies (A depends on B)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunDep(args, globalOpts)
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

func init() {
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
