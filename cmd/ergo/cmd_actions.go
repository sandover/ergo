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
	Long:  "Initialize an Ergo graph in the current directory or in dir. Existing graphs are left intact and missing required files are repaired.",
	Example: `  ergo init
  ergo init ./project`,
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
	Long:  "Create one task or an epic populated from a Markdown file.",
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
	Long: `Create an unclaimed todo task with the positional title.
If stdin is piped, its literal contents become the initial body. Use --epic to
place the task in an existing epic or promote a clean root todo task. Success
prints only the task ID.`,
	Example: `  ergo new task "Add login"
  printf '%s\n' 'Handle password login.' | ergo new task "Add login" --epic ABCDEF`,
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
	Long: `Create an epic and its child tasks atomically from a Markdown file.
The required file contains one or more '# Title' chunks separated by a line
that is exactly '---'. Optional piped stdin becomes the epic body. Epics are
derived from their children, so an empty epic cannot be created. Success names
the epic and every created task.`,
	Example: `  ergo new epic "Authentication" --file tasks.md
  printf '%s\n' 'Authentication release scope.' | ergo new epic "Authentication" --file tasks.md`,
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
	Long:  "List active work as a compact task tree. Filter to ready work, one epic, or all work including closed tasks.",
	Example: `  ergo list --ready
  ergo list --epic ABCDEF
  ergo list --all`,
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
	Use:     "show <id>",
	Short:   "Show task details",
	Long:    "Show the current task or epic, including its body, dependencies, lifecycle messages, and results.",
	Example: `  ergo show ABCDEF`,
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
	Long: `Claim a specific task, or omit the ID to claim the oldest ready task.
--agent is required unless supplied as a global flag. Claiming resumes todo,
blocked, or closed work as doing under the same ID.`,
	Example: `  ergo claim ABCDEF --agent model@host
  ergo claim --agent model@host`,
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
	long := fmt.Sprintf(`Set a task to %s and clear its claim. Use -m to append one
lifecycle note and --result to attach an existing project-relative file.
This command does not read stdin.`, lifecycleHelpState(kind))
	if kind == "release" {
		long += "\nRelease accepts unfinished work and rejects done or canceled tasks."
	}
	cmd := &cobra.Command{
		Use:     kind + " <id>",
		Short:   short,
		Long:    long,
		Example: lifecycleHelpExample(kind),
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

func lifecycleHelpState(kind string) string {
	if kind == "block" {
		return "blocked"
	}
	if kind == "release" {
		return "todo"
	}
	if kind == "cancel" {
		return "canceled"
	}
	return "done"
}

func lifecycleHelpExample(kind string) string {
	message := map[string]string{
		"done":    "Implemented and verified",
		"block":   "Waiting for an API key",
		"cancel":  "Requirement withdrawn",
		"release": "Ready for another agent",
	}[kind]
	return fmt.Sprintf("  ergo %s ABCDEF -m %q", kind, message)
}

var titleCmd = &cobra.Command{
	Use:     "title <id> <title>",
	Short:   "Replace a task title",
	Long:    "Replace the title of a task or epic. The title must not be blank.",
	Example: `  ergo title ABCDEF "Add password login"`,
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
	Long: `Replace the body of a task or epic with literal piped stdin.
An empty pipe clears the body. Interactive stdin is rejected.`,
	Example: `  printf '%s\n' '## Goal' '- Add login' | ergo body ABCDEF`,
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
	Long: `Move a leaf task into an epic or back to the root.
A clean root todo task can become an epic when it receives its first child.
Epics cannot be nested or moved.`,
	Example: `  ergo move ABCDEF GHIJKL
  ergo move ABCDEF --root`,
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
	Use:     "sequence <A> <B> [<C>...]",
	Short:   "Enforce task order (A then B then C)",
	Long:    "Add dependency edges so A must finish before B, B before C, and so on. Existing edges are reported as no changes.",
	Example: `  ergo sequence ABCDEF GHIJKL MNOPQR`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunSequence(args, globalOpts)
	},
}

var unsequenceCmd = &cobra.Command{
	Use:     "unsequence <A> <B> [<C>...]",
	Short:   "Remove task order (A then B then C)",
	Long:    "Remove dependency edges between each adjacent pair. Missing edges are reported as no changes.",
	Example: `  ergo unsequence ABCDEF GHIJKL`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunUnsequence(args, globalOpts)
	},
}

// -- where --
var whereCmd = &cobra.Command{
	Use:     "where",
	Short:   "Show ergo directory path",
	Long:    "Print the resolved .ergo directory selected by discovery or --dir.",
	Example: `  ergo where`,
	Args:    noArgs("where"),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunWhere(globalOpts)
	},
}

// -- compact --
var compactCmd = &cobra.Command{
	Use:     "compact",
	Short:   "Compact the event log",
	Long:    "Rewrite the event log to the minimum events representing the same current graph and report exact before and after counts.",
	Example: `  ergo compact`,
	Args:    noArgs("compact"),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunCompact(globalOpts)
	},
}

// -- prune --
var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Prune closed work (dry-run by default)",
	Long:  "Preview removal of done and canceled leaves and epics left empty. Pass --yes to append the prune events; run compact separately to remove history.",
	Example: `  ergo prune
  ergo prune --yes`,
	Args: noArgs("prune [--yes]"),
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
	Use:     "quickstart",
	Short:   "Show quickstart guide",
	Long:    "Show the complete Ergo guide, including the task model, workflows, concurrency, and maintenance.",
	Example: `  ergo quickstart`,
	Args:    noArgs("quickstart"),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ergo.RunQuickstart(args)
	},
}

// -- version --
var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Show version",
	Long:    "Print the Ergo version.",
	Example: `  ergo version`,
	Args:    noArgs("version"),
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
