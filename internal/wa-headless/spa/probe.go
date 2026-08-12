package spa

// Taking a snapshot of the page, without reading anybody's messages.
//
// The study took one evaluation that gathered structure AND 200 characters of
// body text, then classified. That was safe there only because nothing
// persisted the result. Here it would put the top of the chat list — contact
// names and message previews — into every snapshot of a HEALTHY session, which
// is precisely what invariant 14 forbids.
//
// So the probe is two evaluations, and the order is the safety argument:
//
//	1. structure only — enough to answer APP_READY and LOGIN_REQUIRED, the two
//	   classes a working system spends all its time in;
//	2. marker matching, ONLY if structure matched nothing, because the screens
//	   that need text (conflict, "update Chrome") carry no structural marker.
//
// The common path therefore never looks at text at all. The extra round trip is
// paid only where the session is already broken.
//
// Step 2 does not return text (H6, 2026-08-12). It sends a closed set of our
// own strings into the page and receives back which of them were present, and
// the difference matters more than it looks: the previous version returned real
// body text withheld only when #pane-side was found, so a rename of that one
// selector by Meta would have converted every probe of a healthy session into a
// capture of the chat list. The privacy of this package no longer rests on any
// selector, on markup, or on an identity lookup — it rests on the fact that
// there is no path by which page text reaches a Go string.

import (
	"context"
	"encoding/json"

	"wa-api/internal/wa-headless/engine"
)

// Evaluator runs a JavaScript expression in the page and decodes its JSON
// result. It is the seam that keeps chromedp out of this package (ADR-0006 D1).
type Evaluator func(ctx context.Context, expression string, out *string) error

// Selectors, as constants: a literal at a call site is a contract nobody can
// find when Meta changes it.
//
// The QR is matched TWO ways, and the order is the argument:
//
//	[data-testid="link-device-qr-code"]   Meta's own hook, locale-independent
//	canvas[aria-label*="Scan"]            what wwebjs uses, English UI text
//
// Measured 2026-08-11 against the real SPA (EVIDENCIA-SPA.md M1): both are
// present, and they appear at the same instant. The aria-label one WORKS — that
// was checked, not assumed — but it is interface text in English, so the same
// element on a Portuguese account reads "Ler o código QR" and the selector
// stops matching. Keeping both means one of them changing is not an outage.
//
// The pairing-screen markers are separate from the QR itself. They appear about
// six seconds EARLIER, while the code is still loading, and that gap is a state
// of its own — see qrLoadingSelector.
const (
	paneSideSelector = "#pane-side"
	qrTestIDSelector = `[data-testid="link-device-qr-code"]`
	qrAriaSelector   = `canvas[aria-label*="Scan"]`
	// qrLoadingSelector marks the pairing screen before the code arrives.
	// Measured at t+9s with 340 DOM nodes and no canvas at all.
	qrLoadingSelector = `[data-testid^="link-device-qrcode-alt-linking"]`
)

// structureScript reads only shape: no innerText, no attribute values, nothing
// a person wrote. Every field is a boolean, a count, or a URL the caller
// already knows.
//
// It never reads the QR payload. The [data-ref] attribute that carries it is a
// credential for the seconds it lives, and this probe does not so much as look
// at its value.
const structureScript = `JSON.stringify((() => {
	const q = (s) => !!document.querySelector(s);
	return {
		url: location.href,
		title: document.title,
		ready_state: document.readyState,
		has_pane_side: q('` + paneSideSelector + `'),
		has_qr: q('` + qrTestIDSelector + `') || q('` + qrAriaSelector + `'),
		has_qr_loading: q('` + qrLoadingSelector + `'),
		dom_nodes: document.getElementsByTagName('*').length,
		text_length: document.body ? document.body.innerText.length : 0
	};
})())`

// markerScript is the second, guarded evaluation. It runs only on a page that
// matched no structure.
//
// It hands the page a closed list of OUR strings and asks which ones it saw.
// The body text is read inside the page, compared inside the page, and thrown
// away inside the page: what comes back is a subset of the list that went in.
//
// This replaced a script that returned 200 characters of body.innerText,
// withheld only when #pane-side was present (H6). That guard was real but it
// was ONE selector — the same selector this whole module treats as something
// Meta may rename without warning — and its failure mode was not a
// misclassification, it was contact names and message previews entering the
// process. A guard whose failure costs a class is worth having; a guard whose
// failure costs the PII invariant is worth replacing with a mechanism.
//
// The pane check survives, demoted to what it now actually protects:
// CORRECTNESS. The application can finish loading between the two probes, and a
// chat list scanned for these markers can hit one by coincidence — somebody
// writing "outra aba" in a message would otherwise turn a healthy session into
// SESSION_CONFLICT. It is no longer load-bearing for privacy.
var markerScript = buildMarkerScript()

func buildMarkerScript() string {
	// json.Marshal of []string is a JavaScript array literal, and it escapes
	// anything in a marker that would otherwise break out of the expression.
	list, err := json.Marshal(textMarkers())
	if err != nil {
		// Unreachable for []string, and a panic here would be a boot-time
		// failure in a package with no I/O. An empty list degrades to "no
		// markers ever match", which shows up as OTHER rather than as silence.
		list = []byte("[]")
	}
	return `JSON.stringify((() => {
	if (document.querySelector('` + paneSideSelector + `')) return [];
	const markers = ` + string(list) + `;
	const t = document.body ? document.body.innerText : '';
	const low = t.toLowerCase();
	return markers.filter((m) => low.indexOf(m) !== -1);
})())`
}

// Probe reads the page state and classifies it.
//
// Both evaluations run under the caller's Runner, so both carry the StateProbe
// budget: a page that stops executing JavaScript produces UNRESPONSIVE rather
// than a hang. That is the whole lesson of phase 6 in two lines.
func Probe(ctx context.Context, r *engine.Runner, eval Evaluator, label string) (PageSnapshot, PageClass) {
	var snap PageSnapshot

	var raw string
	err := r.Do(ctx, engine.OpStateProbe, label+"/structure", func(ctx context.Context) error {
		return eval(ctx, structureScript, &raw)
	})
	if err != nil {
		return snap, ClassifyProbe(snap, err)
	}
	if jsonErr := json.Unmarshal([]byte(raw), &snap); jsonErr != nil {
		// The page answered with something that is not the agreed shape. That
		// is not unresponsiveness — it answered — and reporting it as such
		// would send a recycler after a live session.
		return snap, ClassOther
	}

	// Structure was enough: no text is ever fetched on a healthy page.
	if cls := Classify(snap); cls != ClassOther {
		return snap, cls
	}

	var answer string
	if err := r.Do(ctx, engine.OpStateProbe, label+"/markers", func(ctx context.Context) error {
		return eval(ctx, markerScript, &answer)
	}); err != nil {
		// The page answered the first probe and not the second. It is going
		// unresponsive, and saying so beats reporting the OTHER we had before.
		return snap, ClassifyProbe(snap, err)
	}
	// The page script returns JSON.stringify(<string[]>), so what arrives here
	// is a JSON array — measured against a real browser, not assumed. A decode
	// failure means the contract changed; swallowing it would leave an empty
	// result and a page classified by what it lacks.
	var reported []string
	if unmarshalErr := json.Unmarshal([]byte(answer), &reported); unmarshalErr != nil {
		return snap, ClassOther
	}
	// Everything the page said that we did not ask about is discarded here, and
	// this line is the reason PageSnapshot cannot carry page text at all.
	snap.Markers = knownMarkers(reported)
	return snap, Classify(snap)
}
