package main

import (
	"errors"
	"strings"
	"testing"
)

func TestAMissingFormatterIsUnavailableAndNamesTheCommand(t *testing.T) {
	t.Parallel()
	_, err := formatter{name: "missing", cmd: "skillhub-no-such-formatter"}.format(t.TempDir(), "")
	if !errors.Is(err, errFormatterUnavailable) {
		t.Fatalf("err = %v, want errFormatterUnavailable", err)
	}
	if !strings.Contains(err.Error(), "skillhub-no-such-formatter") {
		t.Fatalf("err = %q, want it to name the missing command", err)
	}
}

func TestAFormatterThatExitsNonZeroReportsTheExitAndItsFirstStderrLine(t *testing.T) {
	t.Parallel()
	_, err := formatter{name: "go", cmd: "go", args: []string{"skillhub-no-such-subcommand"}}.format(t.TempDir(), "")
	if err == nil {
		t.Fatal("a formatter that exits non-zero was reported as a success")
	}
	if errors.Is(err, errFormatterUnavailable) || errors.Is(err, errNotFormattable) {
		t.Fatalf("err = %v, want an ordinary run failure", err)
	}
	if !strings.HasPrefix(err.Error(), "exit status ") || !strings.Contains(err.Error(), "skillhub-no-such-subcommand") {
		t.Fatalf("err = %q, want the exit status followed by the formatter's first stderr line", err)
	}
}
