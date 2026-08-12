package spa

// Classifying what a WhatsApp page currently is.
//
// The classes are the ones the product's WaAuthStatus needs to distinguish, and
// the rule that shapes this file comes from phase 6: a target that exists, is
// attached, and whose process is alive tells you NOTHING about whether the
// application is running. The page measured there answered once and then went
// silent for over four minutes with every structural signal reporting healthy.
//
// So UNRESPONSIVE is not "we found no elements". It is "the evaluation did not
// come back inside its budget", and it can only be decided by the caller that
// held the deadline.
//
// Study origin: scripts/chromium-study/p4c_target.go, classify.

import "strings"

// PageClass is what a page currently is.
type PageClass string

const (
	// ClassAppReady means the application is loaded and usable.
	ClassAppReady PageClass = "APP_READY"
	// ClassLoginRequired means a QR code is on screen: the session is gone and
	// only a human with a phone can restore it.
	ClassLoginRequired PageClass = "LOGIN_REQUIRED"
	// ClassSessionConflict means WhatsApp took the session elsewhere. Phase 4C
	// measured this as SESSION_MIGRATES_TO_NEWEST_TAB: one active session per
	// profile, and the newest tab wins.
	ClassSessionConflict PageClass = "SESSION_CONFLICT"
	// ClassErrorPage is WhatsApp refusing the browser itself — the
	// "update Chrome" screen the study hit by launching without a user agent.
	ClassErrorPage PageClass = "ERROR_PAGE"
	// ClassRedirect means the page is no longer on web.whatsapp.com.
	ClassRedirect PageClass = "REDIRECT"
	// ClassPairingLoading is the pairing screen with no code on it yet.
	//
	// Measured, not imagined: at t+9s the real SPA has the pairing screen
	// mounted — 340 DOM nodes, its help/hint/terms markers present, a loading
	// spinner — and no canvas at all. The QR arrives around t+15s.
	//
	// It is a class of its own because folding it into OTHER makes "wait, the
	// code is coming" indistinguishable from "we do not recognise this page",
	// and those ask opposite things of the caller.
	ClassPairingLoading PageClass = "PAIRING_LOADING"
	// ClassUnresponsive means the page did not answer within its budget.
	//
	// NEVER inferred from missing elements. A timeout is a class of its own,
	// and phase 4B got a whole conclusion wrong by reading one as evidence of
	// something else.
	ClassUnresponsive PageClass = "UNRESPONSIVE"
	// ClassOther is a page that answered and matched nothing. It is not a
	// failure to classify away: an unknown state that gets folded into a known
	// one is how a wrong diagnosis becomes permanent.
	ClassOther PageClass = "OTHER"
)

// Terminal reports whether a class means the session is over. Callers use it to
// decide whether waiting longer could help.
func (c PageClass) Terminal() bool {
	return c == ClassLoginRequired || c == ClassSessionConflict
}

// PageSnapshot is the structural state of a page. Shape, never content —
// with one bounded exception, guarded below.
type PageSnapshot struct {
	URL        string `json:"url"`
	Title      string `json:"title"`
	ReadyState string `json:"ready_state"`
	HasPane    bool   `json:"has_pane_side"`
	HasQR      bool   `json:"has_qr"`
	// HasQRLoading is the pairing screen without a code yet.
	HasQRLoading bool `json:"has_qr_loading"`
	DOMNodes     int  `json:"dom_nodes"`
	TextLength   int  `json:"text_length"`
	// Markers is the subset of OUR OWN marker strings that the page reported
	// finding in its body text. It replaced a free-text sample in 2026-08-12
	// (H6), and the difference is the whole PII argument.
	//
	// The old field carried up to 200 characters of body.innerText, guarded by
	// one selector: if #pane-side was absent, the text came back. On a loaded
	// application that text begins with the chat list — contact names and
	// message previews — so the day Meta renamed that selector, every snapshot
	// would have started carrying exactly what invariant 14 forbids.
	//
	// Now the page is handed a closed set of strings and answers with which of
	// them it saw. Anything not in the set we sent is dropped on arrival, so no
	// sequence of bytes originating in the page's own text can traverse the
	// boundary — regardless of which selector matched, or whether any did.
	//
	// It diverges from the study on purpose: p4c_target.go captured a sample
	// unconditionally, which was safe there only because nothing persisted it.
	Markers []string `json:"-"`
}

// conflictMarkers are WhatsApp's own words for "this session moved".
//
// Matching on text is unpleasant and it is what the study had to do: the
// conflict screen carries no id, class or data attribute to key on. Both
// languages the product serves are listed, and a phrase that stops matching
// degrades to ClassOther — visibly — rather than to a wrong class.
//
// They are lowercase because the comparison is done on lowercased text, in the
// page. Adding a marker with an uppercase letter makes it unmatchable.
var conflictMarkers = []string{
	"open in another",
	"another window",
	"only be used in one",
	"outra janela",
	"aberto em outro",
	"outra aba",
}

// The "update Chrome" screen, which WhatsApp serves when it refuses the browser
// itself. It is a CONJUNCTION — the browser name plus a verb — because "chrome"
// alone appears on pages that are fine.
//
// The conjunction is evaluated here in Go, not in the page: the page is given
// the terms and reports which it saw, and this file decides what a combination
// means. Keeping the meaning on this side is what lets a unit test hold it.
const browserNameMarker = "chrome"

var browserUpdateMarkers = []string{"update", "atualize"}

// textMarkers is the closed set the page is allowed to answer with.
//
// Order does not matter — the answer is compared by value, not by index — but
// the contents do: this slice is simultaneously the question asked of the page
// and the whitelist applied to its answer.
func textMarkers() []string {
	m := make([]string, 0, len(conflictMarkers)+1+len(browserUpdateMarkers))
	m = append(m, conflictMarkers...)
	m = append(m, browserNameMarker)
	return append(m, browserUpdateMarkers...)
}

// knownMarkers drops anything the page reported that we did not ask about.
//
// This is the boundary that makes the PII claim structural rather than
// conventional. Without it, a page could answer the marker probe with its own
// text and the sample would be back — through a field whose name promises it
// cannot happen.
func knownMarkers(reported []string) []string {
	allowed := textMarkers()
	kept := make([]string, 0, len(reported))
	for _, r := range reported {
		for _, a := range allowed {
			if r == a {
				kept = append(kept, r)
				break
			}
		}
	}
	return kept
}

const whatsappHost = "web.whatsapp.com"

// Classify decides what the page is, from a snapshot that was actually taken.
//
// It takes no error, and that is deliberate. An earlier version accepted one
// and returned UNRESPONSIVE for it — while ClassifyProbe made the same decision
// a second time. With the rule in two places, deleting either left behaviour
// unchanged, so no test could hold either: a negative control removing the
// check passed. Redundant guards look like defence and act like camouflage.
//
// The "did it answer at all" question now lives in ClassifyProbe, once.
func Classify(s PageSnapshot) PageClass {
	// Structure first, and deliberately: it is both the cheapest signal and the
	// only one that never involves reading somebody's messages.
	if s.HasPane {
		return ClassAppReady
	}
	if s.HasQR {
		return ClassLoginRequired
	}
	if s.HasQRLoading {
		return ClassPairingLoading
	}

	if cls, ok := classifyByMarkers(s.Markers); ok {
		return cls
	}
	switch {
	case s.URL != "" && !strings.Contains(strings.ToLower(s.URL), whatsappHost):
		return ClassRedirect
	case s.TextLength == 0:
		// A page that answered, is on the right host, and has no text at all is
		// still loading or has nothing rendered. It is NOT unresponsive — it
		// answered — and calling it so would blame the target for a state the
		// caller could simply wait out.
		return ClassOther
	}
	return ClassOther
}

// ClassifyProbe is the ONLY place that decides what a failed probe means.
//
// Any error makes the page UNRESPONSIVE, and the merge is deliberate: a blown
// deadline, a dropped CDP connection and a script that threw are different
// causes with the same consequence — there is no valid snapshot, so the session
// is unusable. Classifying from the empty snapshot instead would report OTHER,
// which reads like "a page we do not recognise" rather than "no page answered".
//
// The distinction between the causes is not lost; it is in the OpLog, next to
// the budget the operation promised. That is where a cause belongs, not in a
// class the recycler acts on.
func ClassifyProbe(s PageSnapshot, probeErr error) PageClass {
	if probeErr != nil {
		return ClassUnresponsive
	}
	return Classify(s)
}

// classifyByMarkers matches the screens that carry no structural marker at all.
//
// It receives which of OUR strings the page reported seeing — never the text
// itself. That is the H6 correction: the substring search moved into the page,
// so what crosses the boundary is a verdict about our vocabulary instead of a
// slice of somebody's conversation.
//
// What did NOT move is the meaning. Which combination of markers constitutes a
// conflict screen or a refused browser is decided here, in Go, where a unit
// test can hold it and where changing it is a reviewable diff.
//
// Returns ok=false when nothing matched, so the caller can fall through to the
// signals that do not need text at all.
func classifyByMarkers(matched []string) (PageClass, bool) {
	if len(matched) == 0 {
		return ClassOther, false
	}
	seen := make(map[string]bool, len(matched))
	for _, m := range matched {
		seen[m] = true
	}

	for _, marker := range conflictMarkers {
		if seen[marker] {
			return ClassSessionConflict, true
		}
	}
	// WhatsApp refusing the browser itself, in both languages the product
	// serves. The study hit this by launching without a user agent.
	if seen[browserNameMarker] {
		for _, verb := range browserUpdateMarkers {
			if seen[verb] {
				return ClassErrorPage, true
			}
		}
	}
	return ClassOther, false
}
