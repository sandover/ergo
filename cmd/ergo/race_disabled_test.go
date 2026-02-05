// Purpose: Indicate whether tests run with the race detector.
// Exports: raceEnabled (test-only helper).
// Role: Allows performance guards to scale thresholds under race.
// Invariants: Returns false when the race build tag is absent.
// Notes: Build-tagged for compile-time selection.
//go:build !race

package main

func raceEnabled() bool {
	return false
}
