package main

import (
	"strings"
	"testing"
)

// TestTheCallersArgumentsSurvive. This command's whole job is "bootstrap, plus
// one flag", and the failure mode of getting it wrong is invisible until
// production: replacing the slice instead of extending it would drop the
// program name and every operator-supplied flag.
func TestTheCallersArgumentsSurvive(t *testing.T) {
	in := []string{"wss", "--config=/etc/wa.yaml", "-v"}
	got := withStdioMode(in)

	for _, want := range in {
		if !contains(got, want) {
			t.Fatalf("argument %q was lost: %v", want, got)
		}
	}
	if got[0] != "wss" {
		t.Fatalf("the program name moved to %q; argv[0] is not an ordinary flag", got[0])
	}
	if !contains(got, modeStdio) {
		t.Fatalf("the command did not request stdio at all: %v", got)
	}
}

// TestAskingForStdioTwiceDoesNotRepeatTheFlag: an operator who already passes
// --mode=stdio is not wrong, and a flag parser handed the same flag twice is
// entitled to reject it. Appending unconditionally would turn a redundant but
// correct invocation into a startup failure.
func TestAskingForStdioTwiceDoesNotRepeatTheFlag(t *testing.T) {
	got := withStdioMode([]string{"wss", modeStdio})
	n := 0
	for _, a := range got {
		if a == modeStdio {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("the flag appears %d times: %v", n, got)
	}
}

// TestTheModeIsStdioAndNotSomethingElse pins the value, because this command
// and cmd/core differ by exactly this string and nothing else guards it.
func TestTheModeIsStdioAndNotSomethingElse(t *testing.T) {
	if !strings.HasPrefix(modeStdio, "--mode=") {
		t.Fatalf("%q is not a mode flag", modeStdio)
	}
	if !strings.HasSuffix(modeStdio, "=stdio") {
		t.Fatalf("%q does not select stdio; this command is the stdio entry point", modeStdio)
	}
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}
