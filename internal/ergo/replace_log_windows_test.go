//go:build windows

package ergo

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceLogWindowsOverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, backlogFileName)
	if err := os.WriteFile(path, []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}

	want := []byte("replacement\n")
	if err := replaceLogAtomically(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("replacement = %q, want %q", got, want)
	}
}
