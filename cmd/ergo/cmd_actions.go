package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/sandover/ergo/internal/ergo"
	"github.com/spf13/cobra"
)

func commandRender(cmd *cobra.Command, streams Streams) ergo.RenderOptions {
	width := streams.Width
	if width <= 0 {
		width = 80
	}
	return ergo.RenderOptions{Writer: cmd.OutOrStdout(), Color: streams.StdoutTerminal, Width: width}
}

func commandInput(cmd *cobra.Command, streams Streams, required bool, id string) (string, error) {
	if streams.StdinTerminal {
		if required {
			return "", errors.New("body requires piped stdin; example: printf '%s\\n' '## Goal' | ergo body " + id)
		}
		return "", nil
	}
	body, err := io.ReadAll(cmd.InOrStdin())
	return string(body), err
}

func addCommands(root *cobra.Command, base *ergo.Application, streams Streams, options *ergo.RepositoryOptions, buildVersion string) {
	app := func() *ergo.Application { return base.WithRepository(*options) }
	render := func(cmd *cobra.Command) ergo.RenderOptions { return commandRender(cmd, streams) }

	initCmd := &cobra.Command{Use: "init [dir]", Short: "Initialize an Ergo graph", Args: cobra.MaximumNArgs(1)}
	initCmd.Args = func(_ *cobra.Command, args []string) error {
		if len(args) > 1 {
			return errors.New("usage: ergo init [dir]")
		}
		return nil
	}
	initCmd.RunE = func(cmd *cobra.Command, args []string) error {
		dir := ""
		if len(args) == 1 {
			dir = args[0]
		}
		out, err := app().Initialize(ergo.InitializeRequest{Dir: dir})
		if err == nil {
			ergo.RenderInitialize(cmd.OutOrStdout(), out)
		}
		return err
	}

	newCmd := &cobra.Command{Use: "new", Short: "Create tasks and epics"}
	newCmd.Args = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}
		return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
	}
	newCmd.RunE = func(cmd *cobra.Command, _ []string) error { return cmd.Help() }

	newTaskCmd := &cobra.Command{Use: `task "<title>"`, Short: "Create a task", Args: exactArgs(1, ergo.NewTaskUsage),
		Annotations: map[string]string{commandInputHelp: "Optional piped stdin becomes the initial task body; no pipe creates an empty body."}}
	newTaskCmd.Flags().String("epic", "", "Create the task in this epic")
	newTaskCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if keys := legacyCreationKeys(args[0]); len(keys) > 0 {
			guidance := `creation JSON is not accepted; use ergo new task "<title>"`
			if hasString(keys, "epic") {
				guidance += " --epic <id>"
			}
			if hasAnyString(keys, "state", "claim", "result") {
				guidance += ", then use claim, done, block, cancel, or release for lifecycle data"
			}
			return errors.New(guidance)
		}
		epic, _ := cmd.Flags().GetString("epic")
		body, err := commandInput(cmd, streams, false, "")
		if err != nil {
			return err
		}
		out, err := app().CreateTask(ergo.CreateTaskRequest{Title: args[0], EpicID: epic, Body: body})
		if err == nil {
			ergo.RenderCreateTask(cmd.OutOrStdout(), out)
		}
		return err
	}

	newEpicCmd := &cobra.Command{Use: `epic "<title>" --file <path>`, Short: "Create an epic and its tasks from Markdown", Args: exactArgs(1, ergo.NewEpicUsage),
		Annotations: map[string]string{commandInputHelp: "Optional piped stdin becomes the epic body; --file supplies the child tasks."}}
	newEpicCmd.Flags().String("file", "", "Markdown file with # Title chunks separated by ---")
	newEpicCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if keys := legacyCreationKeys(args[0]); len(keys) > 0 {
			return errors.New(`creation JSON is not accepted; use ergo new epic "<title>" --file <path>`)
		}
		file, _ := cmd.Flags().GetString("file")
		body, err := commandInput(cmd, streams, false, "")
		if err != nil {
			return err
		}
		out, err := app().CreateEpic(ergo.CreateEpicRequest{Title: args[0], FilePath: file, Body: body})
		if err == nil {
			ergo.RenderCreateEpic(cmd.OutOrStdout(), out)
		}
		return err
	}
	newCmd.AddCommand(newTaskCmd, newEpicCmd)

	listCmd := &cobra.Command{Use: "list", Short: "List tasks", Args: noArgs("list [--epic <id>] [--ready | --all]")}
	listCmd.Flags().String("epic", "", "Filter by epic ID")
	listCmd.Flags().Bool("ready", false, "Show only ready tasks (conflicts with --all)")
	listCmd.Flags().Bool("all", false, "Show all tasks, including canceled/done (conflicts with --ready)")
	listCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		epic, _ := cmd.Flags().GetString("epic")
		ready, _ := cmd.Flags().GetBool("ready")
		all, _ := cmd.Flags().GetBool("all")
		out, err := app().List(ergo.ListRequest{EpicID: epic, ReadyOnly: ready, ShowAll: all})
		if err == nil {
			ergo.RenderList(cmd.OutOrStdout(), out, render(cmd).Color, render(cmd).Width)
		}
		return err
	}

	showCmd := &cobra.Command{Use: "show <id>", Short: "Show task details", Args: exactArgs(1, "usage: ergo show <id>")}
	showCmd.Flags().Bool("body", false, "Write only the stored body")
	showCmd.RunE = func(cmd *cobra.Command, args []string) error {
		bodyOnly, _ := cmd.Flags().GetBool("body")
		if bodyOnly {
			out, err := app().ShowBody(ergo.ShowBodyRequest{ID: args[0]})
			if err != nil {
				return err
			}
			return ergo.RenderShowBody(cmd.OutOrStdout(), out)
		}
		out, err := app().Show(ergo.ShowRequest{ID: args[0]})
		if err == nil {
			ergo.RenderShow(cmd.OutOrStdout(), out)
		}
		return err
	}

	claimCmd := &cobra.Command{Use: "claim [<id>]", Short: "Claim a task (or oldest ready task)"}
	claimCmd.Args = func(_ *cobra.Command, args []string) error {
		if len(args) > 1 {
			return errors.New("usage: ergo claim [<id>] --agent <identity>")
		}
		return nil
	}
	claimCmd.Flags().String("agent", "", "Claim identity (required; suggested: model@host)")
	claimCmd.RunE = func(cmd *cobra.Command, args []string) error {
		agent, _ := cmd.Flags().GetString("agent")
		id := ""
		if len(args) == 1 {
			id = args[0]
		}
		out, err := app().Claim(ergo.ClaimRequest{ID: id, AgentID: agent})
		if err == nil {
			ergo.RenderClaim(cmd.OutOrStdout(), out)
		}
		return err
	}

	lifecycle := func(kind, short string) *cobra.Command {
		cmd := &cobra.Command{
			Use:   kind + " <id>",
			Short: short,
			Args:  exactArgs(1, fmt.Sprintf("usage: ergo %s <id> [-m <message>] [--result <path>]", kind)),
		}
		cmd.Flags().String("result", "", "Attach an existing project-relative result file")
		cmd.Flags().StringArrayP("message", "m", nil, "Append a lifecycle message (repeatable)")
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			if !streams.StdinTerminal {
				return fmt.Errorf("%s does not read stdin; use ergo body %s to replace the body or -m <message> to add a lifecycle note", kind, args[0])
			}
			result, _ := cmd.Flags().GetString("result")
			messages, _ := cmd.Flags().GetStringArray("message")
			out, err := app().Lifecycle(ergo.LifecycleRequest{Kind: kind, ID: args[0], ResultPath: result, ResultSet: cmd.Flags().Changed("result"), Messages: messages})
			if err == nil {
				ergo.RenderLifecycle(cmd.OutOrStdout(), out)
			}
			return err
		}
		return cmd
	}

	titleCmd := &cobra.Command{Use: "title <id> <title>", Short: "Replace a task title", Args: exactArgs(2, "usage: ergo title <id> <title>")}
	titleCmd.RunE = func(cmd *cobra.Command, args []string) error {
		out, err := app().UpdateTitle(ergo.UpdateTitleRequest{ID: args[0], Title: args[1]})
		if err == nil {
			ergo.RenderTitle(cmd.OutOrStdout(), out)
		}
		return err
	}
	bodyCmd := &cobra.Command{Use: "body <id>", Short: "Replace a task body from stdin", Args: exactArgs(1, "usage: printf '%s\\n' '<body>' | ergo body <id>"),
		Annotations: map[string]string{commandInputHelp: "Piped stdin is required and replaces the body; an empty pipe clears it."}}
	bodyCmd.RunE = func(cmd *cobra.Command, args []string) error {
		body, err := commandInput(cmd, streams, true, args[0])
		if err != nil {
			return err
		}
		out, err := app().UpdateBody(ergo.UpdateBodyRequest{ID: args[0], Body: []byte(body)})
		if err == nil {
			ergo.RenderBody(cmd.OutOrStdout(), out)
		}
		return err
	}

	moveCmd := &cobra.Command{Use: "move <id> <epic-id> | ergo move <id> --root", Short: "Move a task into an epic or to root"}
	moveCmd.Flags().Bool("root", false, "Move the task out of its epic")
	moveCmd.Args = func(cmd *cobra.Command, args []string) error {
		toRoot, _ := cmd.Flags().GetBool("root")
		if toRoot && len(args) == 2 {
			return errors.New("move destination and --root are mutually exclusive")
		}
		if (toRoot && len(args) != 1) || (!toRoot && len(args) != 2) {
			return errors.New("usage: ergo move <id> <epic-id> | ergo move <id> --root")
		}
		return nil
	}
	moveCmd.RunE = func(cmd *cobra.Command, args []string) error {
		rootFlag, _ := cmd.Flags().GetBool("root")
		dest := ""
		if !rootFlag {
			dest = args[1]
		}
		out, err := app().Move(ergo.MoveRequest{ID: args[0], DestinationID: dest, ToRoot: rootFlag})
		if err == nil {
			ergo.RenderMove(cmd.OutOrStdout(), out)
		}
		return err
	}

	sequence := func(command, event, short string) *cobra.Command {
		cmd := &cobra.Command{Use: command + " <A> <B> [<C>...]", Short: short}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			out, err := app().Sequence(ergo.SequenceRequest{Command: command, EventType: event, IDs: args})
			if err == nil {
				ergo.RenderSequence(cmd.OutOrStdout(), out)
			}
			return err
		}
		return cmd
	}
	whereCmd := &cobra.Command{Use: "where", Short: "Show ergo directory path", Args: noArgs("where")}
	whereCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		out, err := app().Where()
		if err == nil {
			ergo.RenderWhere(cmd.OutOrStdout(), out)
		}
		return err
	}
	compactCmd := &cobra.Command{Use: "compact", Short: "Compact the event log", Args: noArgs("compact")}
	compactCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		out, err := app().Compact()
		if err == nil {
			ergo.RenderCompact(cmd.OutOrStdout(), out)
		}
		return err
	}
	pruneCmd := &cobra.Command{Use: "prune", Short: "Prune closed work (dry-run by default)", Args: noArgs("prune [--yes]")}
	pruneCmd.Flags().Bool("yes", false, "Apply prune (default is dry-run)")
	pruneCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		out, err := app().Prune(ergo.PruneRequest{Confirm: yes})
		if err == nil {
			ergo.RenderPrune(cmd.OutOrStdout(), out, render(cmd).Color, render(cmd).Width)
		}
		return err
	}
	quickCmd := &cobra.Command{Use: "quickstart", Short: "Show quickstart guide", Args: noArgs("quickstart")}
	quickCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		ergo.RenderQuickstart(cmd.OutOrStdout(), app().Quickstart(ergo.QuickstartRequest{Color: render(cmd).Color}))
		return nil
	}
	versionCmd := &cobra.Command{Use: "version", Short: "Show version", Args: noArgs("version")}
	versionCmd.Run = func(cmd *cobra.Command, _ []string) {
		ergo.RenderVersion(cmd.OutOrStdout(), app().Version(ergo.VersionRequest{Version: buildVersion}))
	}

	root.AddCommand(initCmd, newCmd, listCmd, showCmd, claimCmd,
		lifecycle("done", "Mark a task done"), lifecycle("block", "Mark a task blocked"), lifecycle("cancel", "Cancel a task"), lifecycle("release", "Return unfinished work to todo"),
		titleCmd, bodyCmd, moveCmd, sequence("sequence", "link", "Enforce task order (A then B then C)"), sequence("unsequence", "unlink", "Remove task order (A then B then C)"),
		whereCmd, compactCmd, pruneCmd, quickCmd, versionCmd)
}

func hasString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
func hasAnyString(values []string, targets ...string) bool {
	for _, v := range targets {
		if hasString(values, v) {
			return true
		}
	}
	return false
}
func noArgs(usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != 0 {
			return fmt.Errorf("usage: ergo %s", usage)
		}
		return nil
	}
}

func exactArgs(count int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != count {
			return errors.New(usage)
		}
		return nil
	}
}
