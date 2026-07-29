package ergo

import (
	"path/filepath"
	"testing"
)

func TestInfoReportsResolvedExecutableAndRepositoryPaths(t *testing.T) {
	project := t.TempDir()
	if _, err := InitializeRepository(project); err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	executable := filepath.Join(t.TempDir(), "ergo")
	outcome, err := NewApplication(RepositoryOptions{StartDir: project}).Info(InfoRequest{
		Executable: executable,
		Version:    "4.2.0",
	})
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if outcome.Executable != executable {
		t.Errorf("Executable = %q, want %q", outcome.Executable, executable)
	}
	if outcome.Version != "4.2.0" {
		t.Errorf("Version = %q", outcome.Version)
	}
	if outcome.Project != project {
		t.Errorf("Project = %q, want %q", outcome.Project, project)
	}
	wantBacklog := filepath.Join(project, ".ergo", "backlog.jsonl")
	if outcome.Backlog != wantBacklog {
		t.Errorf("Backlog = %q, want %q", outcome.Backlog, wantBacklog)
	}
}

func TestInfoRequiresAnActiveBacklog(t *testing.T) {
	_, err := NewApplication(RepositoryOptions{StartDir: t.TempDir()}).Info(InfoRequest{
		Executable: "/usr/local/bin/ergo",
		Version:    "dev",
	})
	if err == nil {
		t.Fatal("Info succeeded without an active backlog")
	}
}
