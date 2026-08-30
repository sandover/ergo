package ergo

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

const maxWireClientArgv = 200

// WireClient identifies the CLI process that opened a proxied request.
type WireClient struct {
	Pid  int    `json:"pid,omitempty"`
	Ppid int    `json:"ppid,omitempty"`
	Argv string `json:"argv,omitempty"`
}

func BuildWireClient() WireClient {
	argv := strings.Join(os.Args, " ")
	if len(argv) > maxWireClientArgv {
		argv = argv[:maxWireClientArgv] + "…"
	}
	return WireClient{
		Pid:  os.Getpid(),
		Ppid: syscall.Getppid(),
		Argv: argv,
	}
}

func formatWireClient(client WireClient) string {
	if client.Pid == 0 && client.Argv == "" {
		return ""
	}
	var parts []string
	if client.Pid > 0 {
		parts = append(parts, fmt.Sprintf("pid=%d", client.Pid))
	}
	if client.Ppid > 0 {
		parts = append(parts, fmt.Sprintf("ppid=%d", client.Ppid))
	}
	if client.Argv != "" {
		parts = append(parts, fmt.Sprintf("argv=%q", client.Argv))
	}
	return strings.Join(parts, " ")
}
