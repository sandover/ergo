// Purpose: Prove automatic caching does not change user-visible read output.
// Coverage: list, ready/JSON projections, show, and literal body output before
// and after the same repository begins using its disposable cache.
// Invariants: cache presence is invisible at the CLI contract boundary.
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sandover/ergo/v6/internal/ergo"
)

func TestCacheDoesNotChangeReadCommandOutput(t *testing.T) {
	dir := t.TempDir()
	if _, err := ergo.WriteLargeBacklogFixture(dir, ergo.LargeBacklogFixtureOptions{TaskCount: 300}); err != nil {
		t.Fatal(err)
	}
	commands := [][]string{
		{"list"}, {"list", "--ready"}, {"list", "--json"},
		{"list", "--ready", "--json"}, {"show", "T00001"}, {"show", "T00001", "--body"},
	}
	cachePath := filepath.Join(dir, ".ergo", "cache.jsonl")
	for _, args := range commands {
		if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		firstOut, firstErr, firstCode := runErgo(t, dir, "", args...)
		if _, err := os.Stat(cachePath); err != nil {
			t.Fatalf("%v did not create cache: %v", args, err)
		}
		secondOut, secondErr, secondCode := runErgo(t, dir, "", args...)
		if firstOut != secondOut || firstErr != secondErr || firstCode != secondCode {
			t.Fatalf("%v changed with cache: first=(%q,%q,%d) second=(%q,%q,%d)",
				args, firstOut, firstErr, firstCode, secondOut, secondErr, secondCode)
		}
	}
}
