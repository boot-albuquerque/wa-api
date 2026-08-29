// Package fetchmessages answers the product's fetchMessages: the metadata of
// messages already in a chat.
//
// SAME CONTRACT AS onMessageMeta, and deliberately the SAME CODE for the part
// that matters: the metadata allow-list and the decoding come from
// capabilities/messagemeta rather than being written again here. A second
// allow-list is how a body eventually ships — two lists drift, someone adds a
// field to one, and the invariant holds where it is tested and fails where it
// is not.
//
// WHAT IT CAN AND CANNOT SEE, stated up front because the name promises more
// than the mechanism delivers. It reads WAWebMsgCollection, which holds what
// the SPA has already loaded — 384 models on the lab profile when this was
// measured, against a chat list of 917. It does NOT ask the server for more.
// whatsapp-web.js's fetchMessages can trigger a load; this cannot, and a caller
// that asks for 100 messages and gets 12 is being told what is loaded, not what
// exists.
//
// That gap is declared rather than hidden because the alternative — returning a
// short list as if it were the whole history — is the silent-incompleteness
// failure this module keeps meeting.
package fetchmessages

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"wa-api/internal/headless/capabilities/messagemeta"
	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
)

// DefaultLimit bounds a fetch that does not ask for a size.
const DefaultLimit = 50

// Result is what a fetch found.
type Result struct {
	// Messages is the metadata, OLDEST FIRST, matching the order the page
	// stores them in.
	Messages []messagemeta.Meta
	// Loaded is how many models the collection held in total when asked,
	// across all chats. It is the denominator that makes a short result
	// readable: 12 of 384 loaded is a chat with little traffic, 12 of 12 is a
	// page that has barely loaded anything.
	Loaded int
	// Matched is how many models belonged to the requested chat before the
	// limit was applied. Matched > len(Messages) means the limit truncated.
	Matched int
}

// Truncated reports whether the limit cut the result short.
func (r Result) Truncated() bool { return r.Matched > len(r.Messages) }

// ErrNoCollection is a page where the message store did not resolve.
var ErrNoCollection = fmt.Errorf("fetchmessages: the message collection is not available")

// Fetcher reads message metadata for one session.
type Fetcher struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Fetcher.
func New(runner *engine.Runner, eval spa.Evaluator) *Fetcher {
	return &Fetcher{runner: runner, eval: eval}
}

// script builds the page-side read.
//
// The chat jid is passed through strconv.Quote rather than concatenated raw:
// the value comes from a caller and lands inside a JavaScript expression, and
// an unescaped quote there is a page-side injection with this session's
// privileges. Quoting is not defensive style here, it is the boundary.
func script(chatJID string, limit int) string {
	return `JSON.stringify((() => {
		const mod = window.require('` + string(spa.ModuleMsgCollection) + `');
		const coll = mod && mod.MsgCollection;
		if (!coll || typeof coll.getModelsArray !== 'function') {
			return { ok: false };
		}
		const want = ` + strconv.Quote(chatJID) + `;
		const limit = ` + strconv.Itoa(limit) + `;
		const meta = ` + messagemeta.MetaExpr + `;
		const all = coll.getModelsArray();
		const out = { ok: true, loaded: all.length, matched: 0, events: [] };
		const hit = [];
		for (let i = 0; i < all.length; i++) {
			let m;
			try { m = meta(all[i]); } catch (e) { continue; }
			if (want !== "" && m.id.remote_jid !== want) { continue; }
			out.matched++;
			hit.push(m);
		}
		// Keep the NEWEST when truncating — a caller asking for 50 of 400 wants
		// the recent ones — but hand them back oldest-first so the sequence
		// reads forward.
		out.events = limit > 0 && hit.length > limit ? hit.slice(hit.length - limit) : hit;
		return out;
	})())`
}

// Fetch reads the metadata of messages loaded for chatJID.
//
// An empty chatJID reads every loaded chat, which is what the product's own
// "fetch everything the page knows" path needs; it is not a wildcard match on
// the jid, it is the absence of a filter.
func (f *Fetcher) Fetch(ctx context.Context, chatJID string, limit int, label string) (Result, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}

	var raw string
	if err := f.runner.Do(ctx, engine.OpStateProbe, label, func(ctx context.Context) error {
		return f.eval(ctx, script(chatJID, limit), &raw)
	}); err != nil {
		return Result{}, fmt.Errorf("fetchmessages: reading the collection: %w", err)
	}

	ok, loaded, matched, eventsJSON, err := splitWire(raw)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{}, ErrNoCollection
	}

	msgs, err := messagemeta.DecodeWire(eventsJSON)
	if err != nil {
		return Result{}, err
	}
	return Result{Messages: msgs, Loaded: loaded, Matched: matched}, nil
}

// splitWire pulls the envelope apart without decoding the events, so the event
// decoding stays in messagemeta and cannot drift from it.
func splitWire(raw string) (ok bool, loaded, matched int, events []byte, err error) {
	var env struct {
		OK      bool            `json:"ok"`
		Loaded  int             `json:"loaded"`
		Matched int             `json:"matched"`
		Events  json.RawMessage `json:"events"`
	}
	if uerr := json.Unmarshal([]byte(raw), &env); uerr != nil {
		return false, 0, 0, nil, fmt.Errorf("fetchmessages: unexpected answer shape: %w", uerr)
	}
	if len(env.Events) == 0 {
		env.Events = json.RawMessage("[]")
	}
	return env.OK, env.Loaded, env.Matched, env.Events, nil
}
