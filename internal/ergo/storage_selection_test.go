package ergo

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func isolateStorageEnvironment(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "config"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	return home
}

func writeStorageConfig(t *testing.T, config storageConfig) {
	t.Helper()
	path, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestExternalPersonalBacklogLifecycle(t *testing.T) {
	isolateStorageEnvironment(t)
	project, base := t.TempDir(), t.TempDir()
	app := NewApplication(RepositoryOptions{StartDir: project, DataDir: base, User: "aren"})
	if _, err := app.Where(); !errors.Is(err, ErrNoErgoDir) {
		t.Fatalf("uninitialized selection: %v", err)
	}
	initialized, err := app.Initialize(InitializeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := canonicalPath(filepath.Join(base, "users", "aren"))
	if initialized.Path != want {
		t.Fatalf("path = %s, want %s", initialized.Path, want)
	}
	if _, err := os.Stat(filepath.Join(project, dataDirName)); !os.IsNotExist(err) {
		t.Fatalf("source storage created: %v", err)
	}
	created, err := app.CreateTask(CreateTaskRequest{Title: "personal work"})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := app.Claim(ClaimRequest{ID: created.ID, AgentID: "test-agent"})
	if err != nil {
		t.Fatal(err)
	}
	source, _ := canonicalPath(project)
	if claim.ProjectDir != source {
		t.Fatalf("claim project = %s", claim.ProjectDir)
	}
	if err := os.WriteFile(filepath.Join(project, "evidence.txt"), []byte("source evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Result(ResultRequest{ID: created.ID, Text: "checked", FilePath: "evidence.txt", FileSet: true}); err != nil {
		t.Fatal(err)
	}
	shown, err := app.Show(ShowRequest{ID: created.ID})
	if err != nil {
		t.Fatal(err)
	}
	if shown.ProjectDir != source || len(shown.Journal) != 3 {
		t.Fatalf("show = %+v", shown)
	}
	if _, err := app.Lifecycle(LifecycleRequest{ID: created.ID, Kind: "done"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Compact(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Prune(PruneRequest{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	info, err := app.Info(InfoRequest{Executable: "ergo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Project != source || info.Owner != "aren" || info.Storage != want || info.Source != "--data-dir" {
		t.Fatalf("info = %+v", info)
	}
	ignored, err := os.ReadFile(filepath.Join(want, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{"/lock", "/cache.json", "/cache.tmp-*"} {
		if !hasIgnoreLine(ignored, rule) {
			t.Fatalf("missing ignore %s", rule)
		}
	}
	colleague := NewApplication(RepositoryOptions{StartDir: project, DataDir: base, User: "alice"})
	if _, err := colleague.Where(); !errors.Is(err, ErrNoErgoDir) {
		t.Fatalf("colleague fell back: %v", err)
	}
}

func TestPersonalDefaultsIsolateProjectsAndUsers(t *testing.T) {
	isolateStorageEnvironment(t)
	projects := []string{t.TempDir(), t.TempDir()}
	paths := map[string]bool{}
	for _, project := range projects {
		for _, user := range []string{"aren", "alice"} {
			app := NewApplication(RepositoryOptions{StartDir: project, User: user})
			out, err := app.Initialize(InitializeRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if paths[out.Path] {
				t.Fatalf("colliding storage: %s", out.Path)
			}
			paths[out.Path] = true
			if _, err := app.CreateTask(CreateTaskRequest{Title: user}); err != nil {
				t.Fatal(err)
			}
			where, err := app.Where()
			if err != nil || where.Path != out.Path {
				t.Fatalf("where = %+v, %v", where, err)
			}
		}
	}
}

func TestProjectMappingPrecedenceAndSharedOverride(t *testing.T) {
	isolateStorageEnvironment(t)
	root, outerData, innerData := t.TempDir(), t.TempDir(), t.TempDir()
	nested := filepath.Join(root, "nested")
	working := filepath.Join(nested, "src")
	if err := os.MkdirAll(working, 0755); err != nil {
		t.Fatal(err)
	}
	writeStorageConfig(t, storageConfig{Version: 1, DefaultUser: "aren", Projects: []projectStorage{
		{Root: root, DataDir: outerData}, {Root: nested, DataDir: innerData, DefaultBacklog: "personal"},
	}})
	for _, tc := range []struct {
		name                 string
		opts                 RepositoryOptions
		base, owner, project string
	}{
		{"personal", RepositoryOptions{StartDir: working}, innerData, "aren", nested},
		{"shared", RepositoryOptions{StartDir: working, Shared: true}, innerData, "shared", nested},
		{"colleague", RepositoryOptions{StartDir: working, User: "alice"}, innerData, "alice", nested},
		{"explicit base", RepositoryOptions{StartDir: working, DataDir: outerData}, outerData, "aren", nested},
		{"outer", RepositoryOptions{StartDir: root}, outerData, "shared", root},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected, err := resolveStorage(tc.opts, true)
			if err != nil {
				t.Fatal(err)
			}
			want := tc.base
			if tc.owner != "shared" {
				want = filepath.Join(want, "users", tc.owner)
			}
			want, _ = canonicalPath(want)
			project, _ := canonicalPath(tc.project)
			if selected.Dir != want || selected.Owner != tc.owner || selected.Project != project {
				t.Fatalf("selection = %+v", selected)
			}
		})
	}
	// A sibling with the same textual prefix must not match.
	sibling := root + "-other"
	if err := os.MkdirAll(sibling, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sibling) })
	selected, err := resolveStorage(RepositoryOptions{StartDir: sibling}, true)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Source != "repository" {
		t.Fatalf("prefix matched: %+v", selected)
	}
}

func TestStorageConfigurationValidation(t *testing.T) {
	isolateStorageEnvironment(t)
	path, _ := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		`{`, `null`, `{"version":2}`, `{"version":1,"sync":true}`,
		`{"version":1} {}`, `{"version":1,"projects":[{}]}`,
		`{"version":1,"default_user":"../escape"}`,
		`{"version":1,"projects":[{"root":".","default_backlog":"other"}]}`,
		`{"version":1,"projects":[{"root":"."},{"root":"."}]}`,
	} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadStorageConfig(); err == nil {
			t.Errorf("accepted %s", data)
		}
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"projects":[{"root":"source","ergo_data":"data"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := loadStorageConfig()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := canonicalPath(filepath.Join(filepath.Dir(path), "data"))
	if config.Projects[0].DataDir != want {
		t.Fatalf("relative data path = %s", config.Projects[0].DataDir)
	}
}

func TestExternalStorageUsesSourceGitEvidence(t *testing.T) {
	isolateStorageEnvironment(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	project, data := t.TempDir(), t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", project}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init")
	git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "fixture")
	head := git("rev-parse", "HEAD")
	nested := filepath.Join(project, "src")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "evidence.txt"), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	app := NewApplication(RepositoryOptions{StartDir: nested, DataDir: data})
	if _, err := app.Initialize(InitializeRequest{}); err != nil {
		t.Fatal(err)
	}
	created, err := app.CreateTask(CreateTaskRequest{Title: "source evidence"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Result(ResultRequest{ID: created.ID, Text: "checked", FilePath: "evidence.txt", FileSet: true}); err != nil {
		t.Fatal(err)
	}
	shown, err := app.Show(ShowRequest{ID: created.ID})
	if err != nil {
		t.Fatal(err)
	}
	last := shown.Journal[len(shown.Journal)-1]
	if last.File == nil || last.File.GitCommitAtAttach != head {
		t.Fatalf("evidence = %+v", last)
	}
	claim, err := app.Claim(ClaimRequest{AgentID: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := canonicalPath(project)
	if claim.ProjectDir != want {
		t.Fatalf("claim source = %s", claim.ProjectDir)
	}
}

func TestPortableUserIdentifiers(t *testing.T) {
	for _, user := range []string{"", "..", "../alice", "a/b", `a\b`, "Alice", "a.b", "a:b", "con", "lpt1", "álîce", strings.Repeat("a", 65)} {
		if err := validateUserID(user); err == nil {
			t.Errorf("accepted %q", user)
		}
	}
	for _, user := range []string{"alice", "user-42", "a_b"} {
		if err := validateUserID(user); err != nil {
			t.Errorf("rejected %q: %v", user, err)
		}
	}
}

func TestMappedInitializationPositionalRootAndSymlink(t *testing.T) {
	isolateStorageEnvironment(t)
	project, other, data := t.TempDir(), t.TempDir(), t.TempDir()
	writeStorageConfig(t, storageConfig{Version: 1, Projects: []projectStorage{{Root: project, DataDir: data}}})
	app := NewApplication(RepositoryOptions{StartDir: other, User: "alice"})
	initialized, err := app.Initialize(InitializeRequest{Dir: project})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := canonicalPath(filepath.Join(data, "users", "alice"))
	if initialized.Path != want {
		t.Fatalf("init = %+v", initialized)
	}
	if runtime.GOOS == "windows" {
		return
	}
	link := filepath.Join(t.TempDir(), "project-link")
	if err := os.Symlink(project, link); err != nil {
		t.Fatal(err)
	}
	selected, err := resolveStorage(RepositoryOptions{StartDir: link, User: "alice"}, false)
	if err != nil || selected.Dir != want {
		t.Fatalf("symlink selection = %+v, %v", selected, err)
	}
}
