package engine

// The guard of the degradation half of network.go.
//
// It asserts on the rule list handed to the protocol rather than on the
// method's effect, and that is the point: the property being protected is that
// a DEGRADATION can never be built as an OUTAGE, and that property lives in the
// struct, not in the browser. Asserting it here means the guard runs in every
// `make check`, with no target, no profile and no account.
//
// Why it matters is measured, not supposed. EVIDENCIA-SPA.md M5.4: with the
// outage announced, the SPA leaves CONNECTED in 1.4-3.1s; with the bytes merely
// stopped, in 33.2-34.2s. An emulation that announced an outage while claiming
// to emulate slowness would produce a finding about our emulation, and LOOP
// 04.3A already lost a measurement to exactly that.

import (
	"testing"
	"time"
)

// TestDegradedConditionsCannotExpressAnOutage is the guard the coordinator
// required for the engine addition, and it BITES: flipping Offline to true in
// degradedConditions fails it, with the failure output pasted in
// EVIDENCIA-SPA.md M7.
func TestDegradedConditionsCannotExpressAnOutage(t *testing.T) {
	// Every shape a caller can produce, the empty struct included, because the
	// zero value is the one a future caller reaches for by accident.
	for _, d := range []NetworkDegradation{
		{},
		{Latency: 900 * time.Millisecond},
		{DownloadBytesPerSecond: 200 << 10, UploadBytesPerSecond: 100 << 10},
		{Latency: 400 * time.Millisecond, DownloadBytesPerSecond: 500 << 10,
			UploadBytesPerSecond: 200 << 10},
		// Negative is not a caller's intent but is a caller's typo, and the
		// protocol reads a negative throughput as UNTHROTTLED. What must not
		// happen is for it to become an outage by another route.
		{DownloadBytesPerSecond: -5, UploadBytesPerSecond: -5},
	} {
		for _, c := range degradedConditions(d) {
			if c.Offline {
				t.Fatalf("degradedConditions(%+v) built an OUTAGE: Offline=true. A "+
					"degraded network is a network that is THERE — with the outage "+
					"announced the SPA reacts in ~3s and without it in ~34s "+
					"(EVIDENCIA-SPA.md M5.4), so this flag decides what the "+
					"measurement is about", d)
			}
		}
	}
}

// TestDegradedConditionsLeaveUnnamedAxesUnthrottled protects the OTHER way a
// degradation turns into an outage: a zero throughput is a hard zero-byte
// ceiling, which is an outage that never sets the flag.
func TestDegradedConditionsLeaveUnnamedAxesUnthrottled(t *testing.T) {
	for _, c := range degradedConditions(NetworkDegradation{Latency: time.Second}) {
		if c.DownloadThroughput != throughputUnthrottled {
			t.Errorf("an unnamed download axis became a %v-byte ceiling; %d is the "+
				"protocol's sentinel for untouched", c.DownloadThroughput, throughputUnthrottled)
		}
		if c.UploadThroughput != throughputUnthrottled {
			t.Errorf("an unnamed upload axis became a %v-byte ceiling; %d is the "+
				"protocol's sentinel for untouched", c.UploadThroughput, throughputUnthrottled)
		}
		if c.Latency != 1000 {
			t.Errorf("latency reached the protocol as %v; it is expressed in "+
				"milliseconds and 1s is 1000 of them", c.Latency)
		}
	}
}

// TestClearNetworkConditionsClearsWithAnEmptyRuleList guards the RESTORE
// contract, and it guards it on the list the restore path actually builds.
//
// The earlier version of this test called degradedConditions and measured the
// cardinality of the DEGRADATION path while its name announced the restore
// contract. The N2a-EVAL adversarial pass proved the gap by replacing the body
// of ClearNetworkConditions with a permanent outage rule: the test stayed
// GREEN. That is ARMADILHA 2 in pure form — the success path of the restore was
// exercised by no test at all — and the failure output of the mutation against
// THIS version is pasted in HOUSEKEEP.md H12.
func TestClearNetworkConditionsClearsWithAnEmptyRuleList(t *testing.T) {
	got := clearedConditions()
	if len(got) == 0 {
		return
	}
	// Dereferenced, because the list is of pointers and %+v on it prints
	// addresses — a failure message that names nothing is half a guard.
	for i, c := range got {
		t.Errorf("the restore path built rule %d of %d: %+v", i, len(got), *c)
	}
	t.Fatalf("the restore path built %d rule(s) and the contract is ZERO: the "+
		"rule list is replaced wholesale, so an EMPTY list is the only form that "+
		"leaves nothing behind — anything else is an emulation that outlives the "+
		"boot that asked for it", len(got))
}

// TestDegradationIsASingleGlobalRule is what the old test above was really
// measuring, kept because the property is worth keeping: a degradation is ONE
// rule matching every request, not one rule per axis and not a per-URL list.
func TestDegradationIsASingleGlobalRule(t *testing.T) {
	got := degradedConditions(NetworkDegradation{})
	if len(got) != 1 {
		t.Fatalf("a degradation is one global rule, got %d", len(got))
	}
	if got[0].URLPattern != allRequests {
		t.Errorf("the degradation rule matched %q; %q is the protocol's form for "+
			"a GLOBAL condition rather than a per-URL one",
			got[0].URLPattern, allRequests)
	}
}
