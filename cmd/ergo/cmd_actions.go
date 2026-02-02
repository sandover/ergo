// Purpose: Wire cobra subcommands to internal ergo.RunX implementations.
// Exports: none.
// Role: CLI composition layer for user-facing commands.
// Invariants: Flags and command names align with help/quickstart docs.
// Notes: init functions register commands and their flags.
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
	Short: "Create a new task or epic",
}

var newTaskCmd = &cobra.Command{
	Use:   "task",
	Short: "Create a new task (JSON stdin)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := globalOpts
		opts.BodyStdin = newTaskBodyStdin
		opts.TitleFlag = newTaskTitle
		opts.BodyFlag = newTaskBody
		opts.EpicFlag = newTaskEpic
		opts.StateFlag = newTaskState
		opts.ClaimFlag = newTaskClaim
		return ergo.RunNewTask(opts)
	},
}

var (
	newTaskBodyStdin bool
	newTaskTitle     string
	newTaskBody      string
	newTaskEpic      string
	newTaskState     string
	newTaskClaim     string
)

func init() {
	newTaskCmd.Flags().BoolVar(&newTaskBodyStdin, "body-stdin", false, "Read body from stdin (raw text); metadata via flags")
	newTaskCmd.Flags().StringVar(&newTaskTitle, "title", "", "Task title (required with --body-stdin)")
	newTaskCmd.Flags().StringVar(&newTaskBody, "body", "", "Inline body text (mutually exclusive with --body-stdin)")
	newTaskCmd.Flags().StringVar(&newTaskEpic, "epic", "", "Epic ID to assign this task to")
	newTaskCmd.Flags().StringVar(&newTaskState, "state", "", "Initial state (todo|doing|done|blocked|canceled|error)")
	newTaskCmd.Flags().StringVar(&newTaskClaim, "claim", "", "Initial claim identity (agent id)")
}

var newEpicCmd = &cobra.Command{
	Use:   "epic",
	Short: "Create a new epic (JSON stdin)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := globalOpts
		opts.BodyStdin = newEpicBodyStdin
		opts.TitleFlag = newEpicTitle
		opts.BodyFlag = newEpicBody
		return ergo.RunNewEpic(opts)
	},
}

var (
	newEpicBodyStdin bool
	newEpicTitle     string
	newEpicBody      string
)

func init() {
	newEpicCmd.Flags().BoolVar(&newEpicBodyStdin, "body-stdin", false, "Read body from stdin (raw text); metadata via flags")
	newEpicCmd.Flags().StringVar(&newEpicTitle, "title", "", "Epic title (required with --body-stdin)")
	newEpicCmd.Flags().StringVar(&newEpicBody, "body", "", "Inline body text (mutually exclusive with --body-stdin)")
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
		opts := globalOpts
		opts.BodyStdin = setBodyStdin
		opts.TitleFlag = setTitle
		opts.BodyFlag = setBody
		opts.EpicFlag = setEpic
		opts.StateFlag = setState
		opts.ClaimFlag = setClaim
		opts.ResultPathFlag = setResultPath
		opts.ResultSummaryFlag = setResultSummary
		return ergo.RunSet(args[0], opts)
	},
}

var (
	setBodyStdin     bool
	setTitle         string
	setBody          string
	setEpic          string
	setState         string
	setClaim         string
	setResultPath    string
	setResultSummary string
)

func init() {
	setCmd.Flags().BoolVar(&setBodyStdin, "body-stdin", false, "Read body from stdin (raw text); other fields via flags")
	setCmd.Flags().StringVar(&setTitle, "title", "", "New title")
	setCmd.Flags().StringVar(&setBody, "body", "", "Inline body text (mutually exclusive with --body-stdin)")
	setCmd.Flags().StringVar(&setEpic, "epic", "", "Epic ID to assign this task to (\"\" unassign only via JSON)")
	setCmd.Flags().StringVar(&setState, "state", "", "Set state (todo|doing|done|blocked|canceled|error)")
	setCmd.Flags().StringVar(&setClaim, "claim", "", "Set claim identity (\"\" unclaim only via JSON)")
	setCmd.Flags().StringVar(&setResultPath, "result-path", "", "Attach result file path (requires --result-summary)")
	setCmd.Flags().StringVar(&setResultSummary, "result-summary", "", "Attach one-line result summary (requires --result-path)")
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
