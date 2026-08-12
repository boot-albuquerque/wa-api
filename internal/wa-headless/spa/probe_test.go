package spa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// fakePage answers the two scripts the way a browser would, and RECORDS which
// ones were asked for. The recording is the point: "no text was fetched" is the
// PII claim, and it cannot be checked by looking at the result.
type fakePage struct {
	structure PageSnapshot
	text      string
	asked     []string
	failOn    map[string]error
	// paneAppearsLate simulates the application finishing its load between the
	// two probes — the race the marker script has to refuse.
	paneAppearsLate bool
	// reportExtra is what a page says that it was never asked about. Real pages
	// do not do this; the boundary has to survive one that does.
	reportExtra []string
}

func (f *fakePage) eval(ctx context.Context, expression string, out *string) error {
	switch {
	case strings.Contains(expression, "has_pane_side"):
		f.asked = append(f.asked, "structure")
		if err := f.failOn["structure"]; err != nil {
			return err
		}
		b, _ := json.Marshal(f.structure)
		*out = string(b)
		return nil
	case strings.Contains(expression, "markers.filter"):
		f.asked = append(f.asked, "markers")
		if err := f.failOn["markers"]; err != nil {
			return err
		}
		// This mirrors markerScript in probe.go, and it mirrors it EXACTLY on
		// purpose: lowercase the body once, keep the markers whose text occurs
		// in it, answer with nothing else. A double that were more permissive
		// than the page — returning the sample, say — would hide the very
		// defect these tests exist to catch.
		hits := []string{}
		if !f.paneAppearsLate {
			low := strings.ToLower(f.text)
			for _, m := range textMarkers() {
				if strings.Contains(low, m) {
					hits = append(hits, m)
				}
			}
		}
		hits = append(hits, f.reportExtra...)
		b, _ := json.Marshal(hits)
		*out = string(b)
		return nil
	}
	return errors.New("unexpected expression")
}

func (f *fakePage) askedFor(what string) bool {
	for _, a := range f.asked {
		if a == what {
			return true
		}
	}
	return false
}

func testRunner() *engine.Runner {
	p := engine.DefaultDeadlines
	p.StateProbe = 200 * time.Millisecond
	return &engine.Runner{Policy: p}
}

// The claim invariant 14 rests on: a working session never has its text read.
func TestProbeNeverReadsTextFromAReadyPage(t *testing.T) {
	page := &fakePage{
		structure: PageSnapshot{URL: "https://web.whatsapp.com/", HasPane: true, TextLength: 48000},
		text:      "Mum: see you at 8  ·  Work: the deploy is out",
	}

	snap, cls := Probe(context.Background(), testRunner(), page.eval, "boot")

	if cls != ClassAppReady {
		t.Fatalf("class = %q, want %q", cls, ClassAppReady)
	}
	if page.askedFor("markers") {
		t.Fatal("a READY page had its text scanned. On a loaded application the " +
			"body text begins with the chat list — contact names and message " +
			"previews — which is the PII invariant 14 forbids")
	}
	if len(snap.Markers) != 0 {
		t.Fatalf("markers survived into the snapshot of a structural match: %v", snap.Markers)
	}
}

// The QR screen is the other class a structural probe settles, and it is the
// one where the page genuinely has nothing to hide — but the rule is the same.
func TestProbeNeverReadsTextFromAQRPage(t *testing.T) {
	page := &fakePage{structure: PageSnapshot{URL: "https://web.whatsapp.com/", HasQR: true}}

	_, cls := Probe(context.Background(), testRunner(), page.eval, "boot")

	if cls != ClassLoginRequired {
		t.Fatalf("class = %q, want %q", cls, ClassLoginRequired)
	}
	if page.askedFor("markers") {
		t.Error("text was scanned for a page that structure already classified")
	}
}

// The screens that need text carry no structural marker at all, so the second
// probe has to happen there — and only there.
func TestProbeReadsTextOnlyWhenStructureMatchedNothing(t *testing.T) {
	page := &fakePage{
		structure: PageSnapshot{URL: "https://web.whatsapp.com/", TextLength: 120},
		text:      "WhatsApp is open in another window. Click Use Here to use it here.",
	}

	snap, cls := Probe(context.Background(), testRunner(), page.eval, "boot")

	if !page.askedFor("markers") {
		t.Fatal("no text was scanned for a page with no structural marker; the " +
			"conflict screen would be unclassifiable")
	}
	if cls != ClassSessionConflict {
		t.Fatalf("class = %q, want %q (markers %v)", cls, ClassSessionConflict, snap.Markers)
	}
}

// The race: the application finishes loading between the two probes. The text
// script refuses, and a snapshot that would have carried the chat list carries
// nothing.
func TestProbeRefusesTextWhenThePaneAppearsBetweenProbes(t *testing.T) {
	page := &fakePage{
		structure:       PageSnapshot{URL: "https://web.whatsapp.com/", TextLength: 4},
		text:            "Mum: see you at 8",
		paneAppearsLate: true,
	}

	snap, _ := Probe(context.Background(), testRunner(), page.eval, "boot")

	if len(snap.Markers) != 0 {
		t.Fatalf("the race produced markers from a loaded application: %v", snap.Markers)
	}
}

// Phase 6's lesson, in the place it has to live: a page that stops answering is
// UNRESPONSIVE, and nothing about the structure it failed to report may soften
// that into something more optimistic.
func TestProbeReportsUnresponsiveWhenTheStructureProbeTimesOut(t *testing.T) {
	page := &fakePage{failOn: map[string]error{"structure": context.DeadlineExceeded}}

	_, cls := Probe(context.Background(), testRunner(), page.eval, "boot")

	if cls != ClassUnresponsive {
		t.Fatalf("class = %q, want %q", cls, ClassUnresponsive)
	}
}

// A page that answers the first probe and not the second is going silent
// mid-snapshot. Reporting the OTHER we already had would hide a session on its
// way out.
func TestProbeReportsUnresponsiveWhenTheTextProbeTimesOut(t *testing.T) {
	page := &fakePage{
		structure: PageSnapshot{URL: "https://web.whatsapp.com/", TextLength: 10},
		failOn:    map[string]error{"markers": context.DeadlineExceeded},
	}

	_, cls := Probe(context.Background(), testRunner(), page.eval, "boot")

	if cls != ClassUnresponsive {
		t.Fatalf("class = %q, want %q", cls, ClassUnresponsive)
	}
}

// A malformed answer is still an answer. Calling it unresponsive would send a
// recycler after a live session.
func TestProbeDoesNotCallGarbageUnresponsive(t *testing.T) {
	page := &fakePage{}
	eval := func(ctx context.Context, expression string, out *string) error {
		*out = "not json at all"
		return nil
	}

	_, cls := Probe(context.Background(), testRunner(), eval, "boot")

	if cls == ClassUnresponsive {
		t.Fatal("a page that answered with garbage was reported as not answering")
	}
	if cls != ClassOther {
		t.Fatalf("class = %q, want %q", cls, ClassOther)
	}
	_ = page
}

// The budget must come from the policy, or a silent page hangs the caller.
func TestProbeIsBoundedByTheStateProbeBudget(t *testing.T) {
	eval := func(ctx context.Context, expression string, out *string) error {
		<-ctx.Done()
		return ctx.Err()
	}

	start := time.Now()
	_, cls := Probe(context.Background(), testRunner(), eval, "boot")
	elapsed := time.Since(start)

	if cls != ClassUnresponsive {
		t.Fatalf("class = %q, want %q", cls, ClassUnresponsive)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("Probe took %v on a 200ms StateProbe budget", elapsed)
	}
}

// The measured intermediate state: pairing screen mounted, code not there yet.
//
// At t+9s against the real SPA there were 340 DOM nodes, the link-device
// markers and a loading spinner, and no canvas at all. Folding that into OTHER
// makes "wait, the code is coming" read the same as "we do not recognise this
// page" — and those ask opposite things of the caller.
func TestPairingScreenWithoutACodeIsItsOwnClass(t *testing.T) {
	page := &fakePage{structure: PageSnapshot{
		URL: "https://web.whatsapp.com/", HasQRLoading: true, TextLength: 900,
	}}

	snap, cls := Probe(context.Background(), testRunner(), page.eval, "boot")

	if cls != ClassPairingLoading {
		t.Fatalf("class = %q, want %q", cls, ClassPairingLoading)
	}
	if cls == ClassOther {
		t.Error("the pairing screen was reported as an unrecognised page")
	}
	// It matched on structure, so no text may have been scanned.
	if page.askedFor("markers") || len(snap.Markers) != 0 {
		t.Error("text was read from a page that structure already classified")
	}
}

// A code on screen outranks the loading markers: both are present at t+15s, and
// reporting PAIRING_LOADING then would mean waiting for something already there.
func TestACodeOnScreenOutranksTheLoadingMarkers(t *testing.T) {
	page := &fakePage{structure: PageSnapshot{
		URL: "https://web.whatsapp.com/", HasQR: true, HasQRLoading: true,
	}}

	if _, cls := Probe(context.Background(), testRunner(), page.eval, "boot"); cls != ClassLoginRequired {
		t.Fatalf("class = %q, want %q", cls, ClassLoginRequired)
	}
}

// Waiting helps here, so it must not read as terminal — unlike LOGIN_REQUIRED,
// which needs a human with a phone.
func TestPairingLoadingIsNotTerminal(t *testing.T) {
	if ClassPairingLoading.Terminal() {
		t.Fatal("PAIRING_LOADING reported as terminal; a caller would give up on a " +
			"screen that resolves itself in about six seconds")
	}
	if !ClassLoginRequired.Terminal() {
		t.Fatal("LOGIN_REQUIRED must stay terminal: only a human with a phone resolves it")
	}
}

// The structure script must carry BOTH QR selectors. One of them changing is
// then not an outage, which is the whole reason for keeping two.
func TestStructureScriptCarriesBothQRSelectors(t *testing.T) {
	for _, want := range []string{qrTestIDSelector, qrAriaSelector, qrLoadingSelector, paneSideSelector} {
		if !strings.Contains(structureScript, want) {
			t.Errorf("the structure probe does not ask for %s", want)
		}
	}
	// The QR payload is a credential. The probe must not even look at it.
	if strings.Contains(structureScript, "data-ref") {
		t.Error("the structure probe reads [data-ref], which carries the QR payload")
	}
}

// H6, reproduced: Meta renames #pane-side and the chat list is on screen.
//
// This is the exact state the old guard could not survive. Structure reports no
// pane — because the selector no longer matches anything — so the second probe
// runs against a loaded application whose body text is the chat list. Under the
// old script that returned 200 characters of innerText.
//
// The assertion is deliberately made against the WHOLE snapshot rather than
// against one field: a test that checked a named field would keep passing if
// somebody added a second one.
func TestProbeCarriesNoPageTextWhenTheSelectorIsRenamed(t *testing.T) {
	private := []string{"Mum", "see you at 8", "the deploy is out", "+55 11"}

	page := &fakePage{
		// HasPane false is the rename: the element is there, our selector is not.
		structure: PageSnapshot{URL: "https://web.whatsapp.com/", TextLength: 48000},
		text: "Mum: see you at 8\nWork: the deploy is out\n" +
			"+55 11 99999-0000: are we still on?",
	}

	snap, cls := Probe(context.Background(), testRunner(), page.eval, "boot")

	// It genuinely reached the text path — otherwise this test proves nothing.
	if !page.askedFor("markers") {
		t.Fatal("the marker probe never ran, so this test did not exercise H6")
	}
	rendered := fmt.Sprintf("%+v", snap)
	for _, secret := range private {
		if strings.Contains(rendered, secret) {
			t.Errorf("page text reached the snapshot through a renamed selector: "+
				"%q is in %s", secret, rendered)
		}
	}
	// Nothing in a chat list matches our vocabulary, so it stays unrecognised.
	if cls != ClassOther {
		t.Errorf("class = %q, want %q", cls, ClassOther)
	}
}

// The boundary, against a page that answers with something it was never given.
//
// knownMarkers is what makes the PII claim structural instead of conventional:
// without it, the marker probe is just a differently-shaped way to return text.
func TestProbeDiscardsMarkersItNeverAskedAbout(t *testing.T) {
	page := &fakePage{
		structure:   PageSnapshot{URL: "https://web.whatsapp.com/", TextLength: 900},
		reportExtra: []string{"Mum: see you at 8", "outra aba but not really"},
	}

	snap, cls := Probe(context.Background(), testRunner(), page.eval, "boot")

	if len(snap.Markers) != 0 {
		t.Fatalf("the page smuggled strings we never sent: %v", snap.Markers)
	}
	// "outra aba but not really" CONTAINS a real marker. Dropping it whole,
	// rather than matching on it, is the difference between a whitelist and a
	// second substring search on the far side of the boundary.
	if cls == ClassSessionConflict {
		t.Fatal("a string the page invented was accepted as a conflict marker")
	}
}

// The script itself must not be able to return text, whatever the Go side does.
func TestMarkerScriptNeverReturnsPageText(t *testing.T) {
	if strings.Contains(markerScript, "slice(") {
		t.Error("the marker script slices the body text; that is the H6 defect returning")
	}
	if !strings.Contains(markerScript, "markers.filter") {
		t.Error("the marker script does not filter the marker list; it is returning something else")
	}
	// Every marker Go knows about must actually be asked for, or a class dies
	// silently the day somebody adds a phrase and forgets the wire.
	for _, m := range textMarkers() {
		if !strings.Contains(markerScript, m) {
			t.Errorf("marker %q is never sent to the page", m)
		}
	}
	// The pane check survives as a correctness guard: a chat list scanned for
	// these phrases can hit one by coincidence.
	if !strings.Contains(markerScript, paneSideSelector) {
		t.Error("the marker script no longer refuses a loaded application")
	}
}
