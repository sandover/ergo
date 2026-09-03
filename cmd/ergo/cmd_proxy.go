package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/sandover/ergo/v4/internal/ergo"
	"github.com/spf13/cobra"
)

func tryProxy(
	cmd *cobra.Command,
	streams Streams,
	options *ergo.RepositoryOptions,
	noServer *bool,
	color *colorMode,
	method string,
	payload any,
	stdin string,
) (handled bool, err error) {
	if noServer != nil && *noServer {
		return false, nil
	}
	if !ergo.SocketLive(*options) {
		return false, nil
	}
	projectDir, err := ergo.ResolveProjectDir(*options)
	if err != nil {
		return false, nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	render := commandRender(cmd, streams, *color)
	request := ergo.WireRequest{
		Method:  method,
		Dir:     projectDir,
		Color:   render.Color,
		Width:   render.Width,
		Stdin:   stdin,
		Client:  ergo.BuildWireClient(),
		Payload: raw,
	}
	response, err := ergo.ProxyRequest(*options, request)
	if err != nil {
		return false, nil
	}
	if response.OK {
		if response.Stderr != "" {
			if _, err := io.WriteString(cmd.ErrOrStderr(), response.Stderr); err != nil {
				return true, err
			}
		}
		if response.Stdout != "" {
			if _, err := io.WriteString(cmd.OutOrStdout(), response.Stdout); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	if response.Stderr != "" {
		return true, errors.New(response.Stderr)
	}
	return true, errors.New("server request failed")
}

func serveCmd(app *ergo.Application, options *ergo.RepositoryOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the resident command server for this backlog",
		Args:  noArgs("serve"),
		RunE: func(_ *cobra.Command, _ []string) error {
			session, err := app.WithRepository(*options).OpenSession()
			if err != nil {
				return err
			}
			defer session.Close()

			ergoDir := session.Repository().Dir()
			sockPath := ergo.SocketPath(ergoDir)
			pidPath := ergo.PidPath(ergoDir)

			if pid, ok := readPidFile(pidPath); ok && ergo.ProcessAlive(pid) && ergo.SocketLive(*options) {
				return fmt.Errorf("server already running on %s (pid %d)", sockPath, pid)
			}
			_ = os.Remove(sockPath)
			_ = os.Remove(pidPath)

			listener, err := ergo.ListenUnix(sockPath)
			if err != nil {
				return err
			}
			defer listener.Close()
			defer os.Remove(sockPath)

			if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0644); err != nil {
				return err
			}
			defer os.Remove(pidPath)

			fmt.Fprintf(os.Stderr, "ergo serve: listening on %s (pid %d, dir %s)\n",
				sockPath, os.Getpid(), session.ProjectDir())

			stop := make(chan os.Signal, 1)
			signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
			defer signal.Stop(stop)

			go func() {
				for {
					conn, err := listener.Accept()
					if err != nil {
						return
					}
					go ergo.ServeSession(session, conn)
				}
			}()

			<-stop
			listener.Close()
			return nil
		},
	}
}

func readPidFile(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	return pid, true
}
