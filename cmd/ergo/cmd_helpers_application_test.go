package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/sandover/ergo/internal/ergo"
)

func TestApplicationErrorHintUsesTypedUsageClassification(t *testing.T) {
	var output bytes.Buffer
	err := &ergo.ApplicationError{
		Kind: ergo.ErrorUsage,
		Err:  errors.New("a semantic validation message without a usage prefix"),
	}
	if !writeApplicationErrorHint(&output, err, []string{"done", "TASK"}) {
		t.Fatal("typed error was not handled")
	}
	if !strings.Contains(output.String(), "ergo done --help") {
		t.Fatalf("hint = %q", output.String())
	}
}

func TestApplicationErrorHintDoesNotTextMatchTypedConflict(t *testing.T) {
	var output bytes.Buffer
	err := &ergo.ApplicationError{
		Kind: ergo.ErrorConflict,
		Err:  errors.New(`unknown command "set"`),
	}
	if !writeApplicationErrorHint(&output, err, nil) {
		t.Fatal("typed error was not handled")
	}
	if output.Len() != 0 {
		t.Fatalf("conflict hint = %q, want none", output.String())
	}
}
