// Purpose: Wire cobra subcommands to internal ergo.RunX implementations.
// Exports: none.
// Role: CLI composition layer for user-facing commands.
// Invariants: Flags and command names align with help/quickstart docs.
// Notes: init functions register commands and their flags.
package main

import (
	"errors"
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
	newCmd.AddCommand(newEpicCmd)
	// ergo list
	rootCmd.AddCommand(listCmd)
	// ergo show
	rootCmd.AddCommand(showCmd)
	// ergo claim
	rootCmd.AddCommand(claimCmd)
	rootCmd.AddCommand(newLifecycleCmd("done", "Mark a task done"))
	rootCmd.AddCommand(newLifecycleCmd("block", "Mark a task blocked"))
	rootCmd.AddCommand(newLifecycleCmd("cancel", "Cancel a task"))
	rootCmd.AddCommand(newLifecycleCmd("release", "Return unfinished work to todo"))
	rootCmd.AddCommand(titleCmd)
	rootCmd.AddCommand(bodyCmd)
	rootCmd.AddCommand(moveCmd)
	// ergo sequence
	rootCmd.AddCommand(sequenceCmd)
	rootCmd.AddCommand(unsequenceCmd)
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
	Short: "Initialize an Ergo graph",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 {
			return errors.New("usage: ergo init [dir]")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunInit(args, globalOpts)
	},
}

// -- new --
var newCmd = &cobra.Command{
	Use:   "new",
	Short: "Create tasks and epics",
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
	Use:   `task "<title>"`,
	Short: "Create a task",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return errors.New(ergo.NewTaskUsage)
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if keys := ergo.LegacyCreationKeys(args[0]); len(keys) > 0 {
			guidance := `creation JSON is not accepted; use ergo new task "<title>"`
			if hasString(keys, "epic") {
				guidance += " --epic <id>"
			}
			if hasAnyString(keys, "state", "claim", "result") {
				guidance += ", then use claim, done, block, cancel, or release for lifecycle data"
			}
			return errors.New(guidance)
		}
		epicID, _ := cmd.Flags().GetString("epic")
		return ergo.RunNewTask(args[0], epicID, globalOpts)
	},
}

var newEpicCmd = &cobra.Command{
	Use:   `epic "<title>" --file <path>`,
	Short: "Create an epic and its tasks from Markdown",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return errors.New(ergo.NewEpicUsage)
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if keys := ergo.LegacyCreationKeys(args[0]); len(keys) > 0 {
			return errors.New(`creation JSON is not accepted; use ergo new epic "<title>" --file <path>`)
		}
		return ergo.RunNewEpic(args[0], epicTaskFile, globalOpts)
	},
}

var epicTaskFile string

func hasString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func hasAnyString(values []string, targets ...string) bool {
	for _, target := range targets {
		if hasString(values, target) {
			return true
		}
	}
	return false
}

// -- list --
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return errors.New("usage: ergo list [--epic <id>] [--ready | --all]")
		}
		return nil
	},
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
	listCmd.Flags().String("epic", "", "Filter by epic ID")
	listCmd.Flags().Bool("ready", false, "Show only ready tasks")
	listCmd.Flags().Bool("all", false, "Show all tasks (including canceled/done)")
}

// -- show --
var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show task details",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return errors.New("usage: ergo show <id>")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunShow(args[0], globalOpts)
	},
}

// -- claim --
var claimCmd = &cobra.Command{
	Use:   "claim [<id>]",
	Short: "Claim a task (or oldest ready task)",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 {
			return fmt.Errorf("usage: ergo claim [<id>] --agent <identity>")
		}
		return nil
	},
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

func newLifecycleCmd(kind, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   kind + " <id>",
		Short: short,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("usage: ergo %s <id> [-m <message>] [--result <path>]", kind)
			}
			return nil
		},
	}
	cmd.Flags().String("result", "", "Attach an existing project-relative result file")
	cmd.Flags().StringArrayP("message", "m", nil, "Append a lifecycle message (repeatable)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		resultPath, _ := cmd.Flags().GetString("result")
		messages, _ := cmd.Flags().GetStringArray("message")
		return ergo.RunLifecycle(kind, args[0], ergo.LifecycleOptions{
			ResultPath: resultPath,
			ResultSet:  cmd.Flags().Changed("result"),
			Messages:   messages,
		}, globalOpts)
	}
	return cmd
}

var titleCmd = &cobra.Command{
	Use:   "title <id> <title>",
	Short: "Replace a task title",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return fmt.Errorf("usage: ergo title <id> <title>")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunTitle(args[0], args[1], globalOpts)
	},
}

var bodyCmd = &cobra.Command{
	Use:   "body <id>",
	Short: "Replace a task body from stdin",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("usage: printf '%%s\\n' '<body>' | ergo body <id>")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunBody(args[0], globalOpts)
	},
}

var moveCmd = &cobra.Command{
	Use:   "move <id> <epic-id> | move <id> --root",
	Short: "Move a task into an epic or to root",
	Args: func(cmd *cobra.Command, args []string) error {
		toRoot, _ := cmd.Flags().GetBool("root")
		if toRoot && len(args) == 2 {
			return fmt.Errorf("move destination and --root are mutually exclusive")
		}
		if (toRoot && len(args) != 1) || (!toRoot && len(args) != 2) {
			return fmt.Errorf("usage: ergo move <id> <epic-id> | ergo move <id> --root")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		toRoot, _ := cmd.Flags().GetBool("root")
		destination := ""
		if !toRoot {
			destination = args[1]
		}
		return ergo.RunMove(args[0], destination, toRoot, globalOpts)
	},
}

func init() {
	moveCmd.Flags().Bool("root", false, "Move the task out of its epic")
}

func init() {
	newEpicCmd.Flags().StringVar(&epicTaskFile, "file", "", "Markdown file with # Title chunks separated by ---")
	newTaskCmd.Flags().String("epic", "", "Create the task in this epic")
}

// -- sequence --
var sequenceCmd = &cobra.Command{
	Use:   "sequence <A> <B> [<C>...]",
	Short: "Enforce task order (A then B then C)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunSequence(args, globalOpts)
	},
}

var unsequenceCmd = &cobra.Command{
	Use:   "unsequence <A> <B> [<C>...]",
	Short: "Remove task order (A then B then C)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunUnsequence(args, globalOpts)
	},
}

// -- where --
var whereCmd = &cobra.Command{
	Use:   "where",
	Short: "Show ergo directory path",
	Args:  noArgs("where"),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunWhere(globalOpts)
	},
}

// -- compact --
var compactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Compact the event log",
	Args:  noArgs("compact"),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunCompact(globalOpts)
	},
}

// -- prune --
var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Prune closed work (dry-run by default)",
	Args:  noArgs("prune [--yes]"),
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
	Args:  noArgs("quickstart"),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunQuickstart(args)
	},
}

// -- version --
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version",
	Args:  noArgs("version"),
	Run: func(cmd *cobra.Command, args []string) {
		printVersion()
	},
}

func noArgs(usage string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return fmt.Errorf("usage: ergo %s", usage)
		}
		return nil
	}
}
