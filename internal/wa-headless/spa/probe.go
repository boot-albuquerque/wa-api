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
//	2. text, ONLY if structure matched nothing, because the screens that need
//	   text (conflict, "update Chrome") carry no structural marker at all.
//
// The common path therefore never fetches text. The extra round trip is paid
// only where the session is already broken.

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

// textScript is the second, guarded evaluation. It runs only on a page that
// matched no structure, and it REFUSES to return anything if the application
// turns out to be loaded after all — the page can finish loading between the
// two probes, and a race must not be the way PII escapes.
const textScript = `JSON.stringify((() => {
	if (document.querySelector('` + paneSideSelector + `')) return '';
	const t = document.body ? document.body.innerText : '';
	return t.slice(0, 200);
})())`

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

	var sample string
	if err := r.Do(ctx, engine.OpStateProbe, label+"/text", func(ctx context.Context) error {
		return eval(ctx, textScript, &sample)
	}); err != nil {
		// The page answered the first probe and not the second. It is going
		// unresponsive, and saying so beats reporting the OTHER we had before.
		return snap, ClassifyProbe(snap, err)
	}
	// The page script returns JSON.stringify(<string>), so what arrives here is
	// a quoted, escaped JSON string — measured against a real browser, not
	// assumed. A decode failure means the contract changed; swallowing it would
	// leave an empty sample and a page classified by what it lacks.
	if unquoteErr := json.Unmarshal([]byte(sample), &snap.TextSample); unquoteErr != nil {
		snap.TextSample = ""
		return snap, ClassOther
	}
	return snap, Classify(snap)
}
