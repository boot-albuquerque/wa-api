// Package avatar answers the product's fetchContactAvatar by asking the server
// for a contact's profile picture.
//
// ABSENCE IS NOT AN ERROR, and that is the whole contract. Measured 2026-08-20
// over twelve live contacts:
//
//	12  asked
//	10  came back carrying eurl,previewEurl,filehash,fullDirectPath,
//	    previewDirectPath,id,tag,timestamp,stale,eurlStale
//	 2  came back carrying id,tag,timestamp,stale,eurlStale  <- no url fields
//	 0  threw
//	 0  returned null
//
// So a person with no picture produces a NORMAL result whose url fields are
// simply absent. A capability that mapped that to an error would be wrong for
// two of every twelve people, and the caller would learn "the fetch failed"
// where the truth is "there is no picture".
//
// IT ASKS THE SERVER rather than reading the page's cache. The local
// ProfilePicThumbCollection held 68 models against a roster of 545 people, and
// only 33 of those carried a url — serving from it would answer "no avatar" for
// most of the address book while the server has one.
package avatar

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// Bounds for the page-side request, which goes to WhatsApp and so is the slow
// part. They are var, not const, so tests can compress the clock; production
// never assigns them.
var (
	requestBudget = 30 * time.Second
	requestTick   = 250 * time.Millisecond
)

var (
	// ErrRequest is the page refusing or throwing. It is deliberately NOT what
	// a person without a picture produces — see the package comment.
	ErrRequest = fmt.Errorf("avatar: the page refused the request")
	// ErrBadJID is a recipient the page could not turn into an identity.
	ErrBadJID = fmt.Errorf("avatar: the recipient is not a usable identity")
)

// Avatar is what the server knows about one contact's picture.
type Avatar struct {
	// Present is false when the person has no picture. It is a normal answer,
	// and it is the field callers must branch on rather than on URL != "".
	Present bool
	// URL is the full-size picture. Empty when Present is false.
	URL string
	// PreviewURL is the thumbnail.
	PreviewURL string
	// Tag changes when the picture changes, so a caller can skip a download it
	// already has.
	Tag string
	// Timestamp is when the server says the picture was set.
	Timestamp time.Time
	// Stale is the page's own opinion that its copy needs refreshing. Measured
	// false on all twelve sampled contacts, so it is carried rather than
	// interpreted.
	Stale bool
}

// String redacts the urls. A profile-picture url identifies a person AND
// carries an access token, which makes it two kinds of secret at once.
func (a Avatar) String() string {
	if !a.Present {
		return "avatar.Avatar(present=false)"
	}
	return fmt.Sprintf("avatar.Avatar(present=true url=<redacted> preview=%t tag=%s at=%s stale=%t)",
		a.PreviewURL != "", a.Tag, a.Timestamp.UTC().Format(time.RFC3339), a.Stale)
}

// Fetcher asks for avatars on one session.
type Fetcher struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Fetcher.
func New(runner *engine.Runner, eval spa.Evaluator) *Fetcher {
	return &Fetcher{runner: runner, eval: eval}
}

// stateKey is where the page parks the outcome. Evaluate does not await
// promises, so an async call has to be started and then drained.
const stateKey = "__waHeadlessAvatarResult"

type wire struct {
	Stage      string `json:"stage"`
	OK         bool   `json:"ok"`
	Why        string `json:"why"`
	Present    bool   `json:"present"`
	URL        string `json:"url"`
	PreviewURL string `json:"preview_url"`
	Tag        string `json:"tag"`
	Timestamp  int64  `json:"t"`
	Stale      bool   `json:"stale"`
}

func kickScript(jid string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(stateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(stateKey) + `] = v; };
		(async () => {
			let stage = 'resolve';
			try {
				const WidFactory = window.require('` + string(spa.ModuleWidFactory) + `');
				const wid = WidFactory.createWid(` + strconv.Quote(jid) + `);
				if (!wid) { park({ stage, ok: false, why: 'WID_NULL' }); return; }
				stage = 'request';
				const B = window.require('` + string(spa.ModuleContactProfilePicThumbBridge) + `');
				// The argument carries .id; it is NOT the wid itself. Passing
				// the wid throws on 'isNewsletter'.
				const r = await B.requestProfilePicFromServer({ id: wid });
				if (r === null || r === undefined) {
					// Never observed in twelve samples, but a null here is the
					// page declining to answer, which is not the same as a
					// person having no picture. Kept distinguishable.
					park({ stage, ok: false, why: 'NULL_RESULT' });
					return;
				}
				const str = (v) => (typeof v === 'string' ? v : '');
				const url = str(r.eurl);
				park({
					stage: 'done', ok: true, why: '',
					present: url !== '',
					url: url, preview_url: str(r.previewEurl), tag: str(r.tag),
					t: (typeof r.timestamp === 'number') ? r.timestamp : 0,
					stale: !!r.stale
				});
			} catch (e) {
				park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 140) });
			}
		})();
		return { started: true };
	})())`
}

const pollScript = `JSON.stringify((() => {
	const s = window[` + `"` + stateKey + `"` + `];
	if (!s) { return { stage: 'request', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`

// Fetch asks the server for jid's picture.
//
// A person with no picture returns (Avatar{Present: false}, nil). Only a page
// that refused returns an error.
func (f *Fetcher) Fetch(ctx context.Context, jid, label string) (Avatar, error) {
	if strings.TrimSpace(jid) == "" {
		return Avatar{}, ErrBadJID
	}
	var kicked string
	if err := f.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return f.eval(ctx, kickScript(jid), &kicked)
	}); err != nil {
		return Avatar{}, fmt.Errorf("%w: %v", ErrRequest, err)
	}

	deadline := time.Now().Add(requestBudget)
	var w wire
	for {
		var raw string
		if err := f.runner.Do(ctx, engine.OpStateProbe, label+"/poll", func(ctx context.Context) error {
			return f.eval(ctx, pollScript, &raw)
		}); err != nil {
			return Avatar{}, fmt.Errorf("%w: %v", ErrRequest, err)
		}
		if err := json.Unmarshal([]byte(raw), &w); err != nil {
			return Avatar{}, fmt.Errorf("avatar: unexpected answer: %w", err)
		}
		if w.Stage != "pending" {
			break
		}
		if !time.Now().Before(deadline) {
			return Avatar{}, fmt.Errorf("%w: the page never settled within %s", ErrRequest, requestBudget)
		}
		time.Sleep(requestTick)
	}
	if !w.OK {
		if w.Stage == "resolve" {
			return Avatar{}, fmt.Errorf("%w (%s)", ErrBadJID, w.Why)
		}
		return Avatar{}, fmt.Errorf("%w at %s (%s)", ErrRequest, w.Stage, w.Why)
	}
	out := Avatar{
		Present:    w.Present,
		URL:        w.URL,
		PreviewURL: w.PreviewURL,
		Tag:        w.Tag,
		Stale:      w.Stale,
	}
	if w.Timestamp > 0 {
		out.Timestamp = time.Unix(w.Timestamp, 0).UTC()
	}
	return out, nil
}
