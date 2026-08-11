package engine

import (
	"context"
	"testing"
	"time"
)

// What can be asserted here without a browser is narrow, and saying so is part
// of the test: priming a context that was never a tab must REPORT that, not
// pretend it worked.
//
// The failure mode being closed is a caller that primes the wrong context —
// the session context, say, instead of the tab's — and reads the nil back as
// "the target is ready". Every operation afterwards would then fail with
// "context canceled" and look like a dead browser.
//
// The rule that actually matters — never prime from inside a Runner.Do — is not
// observable from one call, so it is enforced statically in gate_test.go.
func TestPrimeTabRefusesAContextThatIsNotATab(t *testing.T) {
	err := PrimeTab(context.Background())
	if err == nil {
		t.Fatal("priming a plain context was reported as success; a caller would " +
			"then treat an unmaterialised target as ready")
	}
}

// Priming must not outlive a cancelled tab: if the tab is already gone, the
// call returns instead of blocking on a target that will never exist.
func TestPrimeTabReturnsOnACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() { done <- PrimeTab(ctx) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("priming a cancelled context was reported as success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("PrimeTab blocked on a cancelled context")
	}
}
