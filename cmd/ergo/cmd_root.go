package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/sandover/ergo/internal/ergo"
	"github.com/spf13/cobra"
)

const commandInputHelp = "ergo_command_input"

type Streams struct {
	In             io.Reader
	Out            io.Writer
	Err            io.Writer
	StdinTerminal  bool
	StdoutTerminal bool
	Width          int
}

func NewRootCommand(app *ergo.Application, streams Streams, buildVersion string) *cobra.Command {
	if app == nil {
		panic("NewRootCommand requires an application")
	}
	if streams.In == nil || streams.Out == nil || streams.Err == nil {
		panic("NewRootCommand requires input, output, and error streams")
	}
	options := ergo.RepositoryOptions{}
	root := &cobra.Command{
		Use: "ergo", Short: "A dependency-aware backlog for coding agents.",
		Long:         "Ergo manages a repository-local backlog shared by agents and humans.\nTasks and dependencies persist across sessions and remain safe under\nconcurrent work.",
		SilenceUsage: true, SilenceErrors: true, Version: buildVersion,
	}
	root.SetIn(streams.In)
	root.SetOut(streams.Out)
	root.SetErr(streams.Err)
	root.PersistentFlags().StringVar(&options.StartDir, "dir", "", "Run in a specific directory")
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		if cmd == root {
			usage := ergo.UsageText(streams.StdoutTerminal)
			fmt.Fprint(cmd.OutOrStdout(), usage)
			if !strings.HasSuffix(usage, "\n") {
				fmt.Fprintln(cmd.OutOrStdout())
			}
			return
		}
		fmt.Fprint(cmd.OutOrStdout(), cmd.UsageString())
		if input := cmd.Annotations[commandInputHelp]; input != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "\nInput:\n  %s\n", input)
		}
	})
	addCommands(root, app, streams, &options, buildVersion)
	return root
}

func runCommand(root *cobra.Command, args []string, streams Streams) int {
	if err := removedArgumentError(args); err != nil {
		writeCLIError(streams.Err, err, args)
		return 1
	}
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		writeCLIError(streams.Err, err, args)
		return 1
	}
	return 0
}

type removedCommandError struct {
	command string
	err     error
}

func (e *removedCommandError) Error() string {
	return e.err.Error()
}

func (e *removedCommandError) Unwrap() error {
	return e.err
}

func removedArgumentError(args []string) error {
	for _, arg := range args {
		switch {
		case arg == "--json" || strings.HasPrefix(arg, "--json="):
			return errors.New("--json is not accepted; Ergo prints readable text")
		case arg == "--summary" || strings.HasPrefix(arg, "--summary="):
			return errors.New("--summary is not accepted; use -m <message>")
		}
	}
	switch command := rootInvocation(args); command {
	case "plan":
		return errors.New(`plan is not accepted; use ergo new epic "<title>" --file <path>`)
	case "set", "reopen":
		return &removedCommandError{
			command: command,
			err:     fmt.Errorf("unknown command %q for %q", command, "ergo"),
		}
	}
	return nil
}

func rootInvocation(args []string) string {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--dir" {
			index++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return arg
	}
	return ""
}
