package contacts

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
)

// Reading a contact's "about" text — what this build calls a text status.
//
// THE CALL TAKES A WID, and that was confirmed from MODULE-QUALIFIED call
// sites: o("WAWebTextStatusAction").getTextStatus(contact.id). Its neighbour
// findCommonGroups takes the MODEL. There is no rule that says which; there is
// only reading, and H69 is the entry that explains what assuming costs.
//
// THE FEATURE CAN BE OFF. receiveTextStatusEnabled() is checked before fetching,
// because a build with it disabled would otherwise be indistinguishable from a
// contact who has written nothing — and those are different answers.
//
// THE TEXT IS CONTENT. About is something a person wrote about themselves, so
// About carries it for the caller and renders only a LENGTH, the same way this
// module treats message bodies and display names.

var (
	// ErrAbout is the page refusing or throwing.
	ErrAbout = fmt.Errorf("contacts: the page refused to read the about text")
	// ErrAboutDisabled is the feature being off on this build. It is distinct
	// from an empty about on purpose: one means "this build cannot tell you",
	// the other means "they wrote nothing".
	ErrAboutDisabled = fmt.Errorf("contacts: this build has text status receiving disabled")
)

// About is a contact's about text.
type About struct {
	// Text is what they wrote. It may legitimately be empty.
	Text string
	// Fetched says whether this session had to ask the server, or already knew.
	Fetched bool
	Waited  time.Duration
}

// String renders a LENGTH. The text is something a person wrote about
// themselves.
func (a About) String() string {
	return fmt.Sprintf("contacts.About(len=%d fetched=%t waited=%s)",
		len([]rune(a.Text)), a.Fetched, a.Waited.Round(time.Millisecond))
}

const aboutStateKey = "__headlessAbout"

// AboutOf reads a contact's about text, fetching it if this session does not
// have it yet.
func (l *Lister) AboutOf(ctx context.Context, jid, label string) (About, error) {
	if strings.TrimSpace(jid) == "" {
		return About{}, ErrNoContact
	}
	start := time.Now()

	var kicked string
	if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return l.eval(ctx, aboutScript(jid), &kicked)
	}); err != nil {
		return About{}, fmt.Errorf("%w: %v", ErrAbout, err)
	}

	var out struct {
		Stage   string `json:"stage"`
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		Text    string `json:"text"`
		Fetched bool   `json:"fetched"`
	}
	deadline := time.Now().Add(commonBudget)
	for {
		var raw string
		if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return l.eval(ctx, aboutResultScript, &raw)
		}); err != nil {
			return About{}, fmt.Errorf("%w: %v", ErrAbout, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return About{}, fmt.Errorf("contacts: unexpected about answer: %w", err)
		}
		if out.Stage != "pending" {
			break
		}
		if !time.Now().Before(deadline) {
			return About{}, fmt.Errorf("%w: the page never settled within %s", ErrAbout, commonBudget)
		}
		time.Sleep(commonTick)
	}

	switch {
	case out.Why == "DISABLED":
		return About{}, ErrAboutDisabled
	case out.Why == "NOT_ON_WHATSAPP" || out.Why == "WID_NULL":
		return About{}, ErrNoContact
	case !out.OK:
		return About{}, fmt.Errorf("%w at %s (%s)", ErrAbout, out.Stage, out.Why)
	}
	// AN EMPTY ABOUT IS A SUCCESSFUL ANSWER. The refusal case is
	// ErrAboutDisabled, and keeping them apart is why the gate is consulted.
	return About{Text: out.Text, Fetched: out.Fetched, Waited: time.Since(start)}, nil
}

func aboutScript(jid string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(aboutStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(aboutStateKey) + `] = v; };
		(async () => {
		let stage = 'gate';
		try {
			const G = window.require('` + string(spa.ModuleTextStatusGatingUtils) + `');
			// ASKED FIRST: a build with the feature off is not a contact with
			// nothing written, and answering one with the other would be a lie
			// the caller cannot detect.
			if (G.receiveTextStatusEnabled && !G.receiveTextStatusEnabled()) {
				park({ stage, ok: false, why: 'DISABLED' }); return;
			}

			stage = 'resolve';
			const r = await (` + spa.ResolveIdentityExpr + `)(` + strconv.Quote(jid) + `);
			if (!r.ok) { park({ stage, ok: false, why: r.why }); return; }

			stage = 'read';
			const C = window.require('` + string(spa.ModuleTextStatusCollection) + `').TextStatusCollection;
			const read = () => {
				try {
					const m = C.find ? C.find(r.wid) : null;
					if (!m) { return null; }
					// The field has been seen under more than one name across
					// builds, so all the plausible ones are tried and the first
					// STRING wins.
					for (const f of ['status', 'text', 'textStatus']) {
						if (typeof m[f] === 'string') { return m[f]; }
					}
					return '';
				} catch (e) { return null; }
			};

			let text = read(), fetched = false;
			if (text === null) {
				// TAKES THE WID, not the model — confirmed from
				// module-qualified call sites.
				const A = window.require('` + string(spa.ModuleTextStatusAction) + `');
				await A.getTextStatus(r.wid);
				fetched = true;
				text = read();
			}
			park({ stage: 'done', ok: true, why: '',
				text: (text === null ? '' : text), fetched: fetched });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

const aboutResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + aboutStateKey + `"` + `];
	if (!s) { return { stage: 'read', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`
