package chatstate

import (
	"strings"
	"testing"
)

// TWO CALLS MUST NOT SHARE A PAGE GLOBAL (H177).
//
// Every reader here parked on ONE global. Two concurrent calls on the same
// session overwrote each other and each polled until non-empty, so one could
// take the other's answer — measured in capabilities/message at 12 crossings in
// 12 rounds, with a well-formed wrong answer that nothing detected.
func TestTwoCallsDoNotShareAStateKey(t *testing.T) {
	a, b := nextStateKey(), nextStateKey()
	if a == b {
		t.Fatalf("two calls got the same key %q; concurrent callers would "+
			"overwrite each other's answers", a)
	}
	if !strings.HasPrefix(a, stateKeyPrefix) || !strings.HasPrefix(b, stateKeyPrefix) {
		t.Fatalf("the keys left the module's namespace: %q %q", a, b)
	}
}
