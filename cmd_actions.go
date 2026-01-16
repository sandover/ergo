package main

import (
	"github.com/spf13/cobra"
)

// Wrapper functions to adapt Cobra commands to existing runX functions
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
	// ergo next
	rootCmd.AddCommand(nextCmd)
	// ergo set
	rootCmd.AddCommand(setCmd)
	// ergo dep
	rootCmd.AddCommand(depCmd)
	// ergo where
	rootCmd.AddCommand(whereCmd)
	// ergo compact
	rootCmd.AddCommand(compactCmd)
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
		return runInit(args, globalOpts)
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
	RunE: func(cmd *cobra.Command, args []string) error {
		formatJSON, _ := cmd.Flags().GetBool("json")
		effectiveArgs := args
		if formatJSON {
			effectiveArgs = append(effectiveArgs, "--json")
		}
		return runNewTask(effectiveArgs, globalOpts)
	},
}

var newEpicCmd = &cobra.Command{
	Use:   "epic",
	Short: "Create a new epic (JSON stdin)",
	RunE: func(cmd *cobra.Command, args []string) error {
		formatJSON, _ := cmd.Flags().GetBool("json")
		effectiveArgs := args
		if formatJSON {
			effectiveArgs = append(effectiveArgs, "--json")
		}
		return runNewEpic(effectiveArgs, globalOpts)
	},
}

func init() {
	newTaskCmd.Flags().Bool("json", false, "Output JSON")
	newEpicCmd.Flags().Bool("json", false, "Output JSON")
	initCmd.Flags().Bool("json", false, "Output JSON")
}

// -- list --
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		var effectiveArgs []string
		if f, _ := cmd.Flags().GetBool("json"); f {
			effectiveArgs = append(effectiveArgs, "--json")
		}
		if f, _ := cmd.Flags().GetString("epic"); f != "" {
			effectiveArgs = append(effectiveArgs, "--epic", f)
		}
		if f, _ := cmd.Flags().GetBool("ready"); f {
			effectiveArgs = append(effectiveArgs, "--ready")
		}
		if f, _ := cmd.Flags().GetBool("blocked"); f {
			effectiveArgs = append(effectiveArgs, "--blocked")
		}
		if f, _ := cmd.Flags().GetBool("epics"); f {
			effectiveArgs = append(effectiveArgs, "--epics")
		}
		if f, _ := cmd.Flags().GetBool("all"); f {
			effectiveArgs = append(effectiveArgs, "--all")
		}
		return runList(effectiveArgs, globalOpts)
	},
}

func init() {
	listCmd.Flags().Bool("json", false, "Output JSON")
	listCmd.Flags().String("epic", "", "Filter by epic ID")
	listCmd.Flags().Bool("ready", false, "Show only ready tasks")
	listCmd.Flags().Bool("blocked", false, "Show only blocked tasks")
	listCmd.Flags().Bool("epics", false, "Show only epics")
	listCmd.Flags().Bool("all", false, "Show all tasks (including canceled/done)")
}

// -- show --
var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show task details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var effectiveArgs []string = args
		if f, _ := cmd.Flags().GetBool("json"); f {
			effectiveArgs = append(effectiveArgs, "--json")
		}
		if f, _ := cmd.Flags().GetBool("short"); f {
			effectiveArgs = append(effectiveArgs, "--short")
		}
		return runShow(effectiveArgs, globalOpts)
	},
}

func init() {
	showCmd.Flags().Bool("json", false, "Output JSON")
	showCmd.Flags().Bool("short", false, "Short output format")
}

// -- next --
var nextCmd = &cobra.Command{
	Use:   "next",
	Short: "Claim and show the next ready task",
	RunE: func(cmd *cobra.Command, args []string) error {
		var effectiveArgs []string
		if f, _ := cmd.Flags().GetBool("json"); f {
			effectiveArgs = append(effectiveArgs, "--json")
		}
		if f, _ := cmd.Flags().GetBool("peek"); f {
			effectiveArgs = append(effectiveArgs, "--peek")
		}
		if f, _ := cmd.Flags().GetString("epic"); f != "" {
			effectiveArgs = append(effectiveArgs, "--epic", f)
		}
		return runNext(effectiveArgs, globalOpts)
	},
}

func init() {
	nextCmd.Flags().Bool("json", false, "Output JSON")
	nextCmd.Flags().Bool("peek", false, "Peek at next task without claiming")
	nextCmd.Flags().String("epic", "", "Filter by epic ID")
}

// -- set --
var setCmd = &cobra.Command{
	Use:   "set <id>",
	Short: "Update a task (JSON stdin)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var effectiveArgs []string = args
		if f, _ := cmd.Flags().GetBool("json"); f {
			effectiveArgs = append(effectiveArgs, "--json")
		}
		return runSet(effectiveArgs, globalOpts)
	},
}

func init() {
	setCmd.Flags().Bool("json", false, "Output JSON")
}

// -- dep --
var depCmd = &cobra.Command{
	Use:   "dep <A> <B> | dep rm <A> <B>",
	Short: "Manage dependencies (A depends on B)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDep(args, globalOpts)
	},
}

// -- where --
var whereCmd = &cobra.Command{
	Use:   "where",
	Short: "Show ergo directory path",
	RunE: func(cmd *cobra.Command, args []string) error {
		var effectiveArgs []string
		if f, _ := cmd.Flags().GetBool("json"); f {
			effectiveArgs = append(effectiveArgs, "--json")
		}
		return runWhere(effectiveArgs, globalOpts)
	},
}

func init() {
	whereCmd.Flags().Bool("json", false, "Output JSON")
}

// -- compact --
var compactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Compact the event log",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCompact(args, globalOpts)
	},
}

// -- quickstart --
var quickstartCmd = &cobra.Command{
	Use:   "quickstart",
	Short: "Show quickstart guide",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runQuickstart(args)
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