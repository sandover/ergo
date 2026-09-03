package main

import (
	"errors"

	"github.com/sandover/ergo/v4/internal/ergo"
	"github.com/spf13/cobra"
)

func batchShowCmd(app func() *ergo.Application, streams Streams, options *ergo.RepositoryOptions, noServer *bool, color *colorMode) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch-show",
		Short: "Show many tasks as one JSON document",
		Args:  cobra.MinimumNArgs(1),
	}
	cmd.Flags().Bool("json", true, "Write JSON (always on for batch-show)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		req := ergo.BatchShowRequest{IDs: args, WithBody: true}
		if handled, err := tryProxy(cmd, streams, options, noServer, color, "batch_show", req, ""); handled {
			return err
		}
		out, err := app().BatchShow(req)
		if err != nil {
			return err
		}
		if warnings := ergo.FormatBatchShowWarnings(out.Missing); warnings != "" {
			if _, err := cmd.ErrOrStderr().Write([]byte(warnings)); err != nil {
				return err
			}
		}
		return ergo.RenderBatchShowJSON(cmd.OutOrStdout(), out)
	}
	return cmd
}

func validateListJSONFlags(jsonOutput, withMeta, withBody bool) error {
	if (withMeta || withBody) && !jsonOutput {
		return errors.New("usage: --with-meta and --with-body require --json")
	}
	return nil
}
