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
	NoColor        bool
	Term           string
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
	color := colorModeAuto
	root := &cobra.Command{
		Use: "ergo", Short: "A dependency-aware backlog for coding agents.",
		Long:         "Ergo manages a repository-local backlog shared by agents and humans.\nTasks and dependencies persist across sessions and remain safe under\nconcurrent work.",
		SilenceUsage: true, SilenceErrors: true, Version: buildVersion,
	}
	root.SetIn(streams.In)
	root.SetOut(streams.Out)
	root.SetErr(streams.Err)
	root.PersistentFlags().StringVar(&options.StartDir, "dir", "", "Run in a specific directory")
	root.PersistentFlags().Var(&color, "color", "Color output: auto, always, or never")
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		if cmd == root {
			usage := ergo.UsageText(resolveColor(color, streams))
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
	addCommands(root, app, streams, &options, &color, buildVersion)
	return root
}

type colorMode string

const (
	colorModeAuto   colorMode = "auto"
	colorModeAlways colorMode = "always"
	colorModeNever  colorMode = "never"
)

func (mode *colorMode) Set(value string) error {
	switch colorMode(value) {
	case colorModeAuto, colorModeAlways, colorModeNever:
		*mode = colorMode(value)
		return nil
	default:
		return fmt.Errorf("invalid color mode %q; expected auto, always, or never", value)
	}
}

func (mode *colorMode) String() string {
	if mode == nil || *mode == "" {
		return string(colorModeAuto)
	}
	return string(*mode)
}

func (mode *colorMode) Type() string {
	return "mode"
}

func resolveColor(mode colorMode, streams Streams) bool {
	switch mode {
	case colorModeAlways:
		return true
	case colorModeNever:
		return false
	default:
		return streams.StdoutTerminal && !streams.NoColor && streams.Term != "dumb"
	}
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
		if arg == "--dir" || arg == "--color" {
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
