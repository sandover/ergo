package ergo

import "testing"

func TestFormatWireClient(t *testing.T) {
	got := formatWireClient(WireClient{Pid: 42, Ppid: 7, Argv: "ergo list --json"})
	want := `pid=42 ppid=7 argv="ergo list --json"`
	if got != want {
		t.Fatalf("formatWireClient() = %q, want %q", got, want)
	}
}

func TestBuildWireClientTruncatesLongArgv(t *testing.T) {
	args := []string{"ergo", "list"}
	for len(joinArgv(args)) <= maxWireClientArgv {
		args = append(args, "--flag")
	}
	client := buildWireClientFromArgs(args)
	if len(client.Argv) <= maxWireClientArgv {
		t.Fatalf("expected truncated argv, got len=%d", len(client.Argv))
	}
	if client.Argv[len(client.Argv)-3:] != "…" {
		t.Fatalf("expected ellipsis suffix, got %q", client.Argv)
	}
}

func joinArgv(args []string) string {
	return buildWireClientFromArgs(args).Argv
}

func buildWireClientFromArgs(args []string) WireClient {
	argv := ""
	for i, arg := range args {
		if i > 0 {
			argv += " "
		}
		argv += arg
	}
	if len(argv) > maxWireClientArgv {
		argv = argv[:maxWireClientArgv] + "…"
	}
	return WireClient{Pid: 1, Ppid: 2, Argv: argv}
}
