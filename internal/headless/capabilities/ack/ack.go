// Package ack reads how far a message got: sent, delivered, read.
//
// IT READS msg.ack AND NOT THE INFO DRAWER, and that is measured rather than
// chosen for simplicity. WAWebMsgInfoCollection was empty in a fresh session —
// zero models against 368 sent messages — and get() returned nothing for every
// recent one. The per-participant detail the app shows in its "message info"
// drawer is fetched when that drawer opens; a headless session that never opens
// it has msg.ack and nothing else.
//
// THE NUMBERS COME FROM THE PAGE. WAWebAck names the states, and this package
// reads the mapping out of it rather than hard-coding integers: a constant
// copied from somebody's blog is a constant nobody can check, and this build is
// the only authority on its own enum.
package ack

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

var (
	// ErrAck is the page refusing or throwing.
	ErrAck = fmt.Errorf("ack: the page refused")
	// ErrNoMessage is an id that is not in the loaded collection.
	ErrNoMessage = fmt.Errorf("ack: no such message in the loaded collection")
)

// State is how far a message got. The zero value is Unknown on purpose: a
// missing ack must not read as "pending", which is a claim.
type State int

// The states, ordered by progress. Their NUMERIC values are this package's own;
// the page's numbers are looked up and mapped, so a build that renumbers its
// enum changes Status.Raw and not this.
const (
	Unknown State = iota
	Error
	Pending
	Sent
	Delivered
	Read
	Played
)

func (s State) String() string {
	switch s {
	case Error:
		return "error"
	case Pending:
		return "pending"
	case Sent:
		return "sent"
	case Delivered:
		return "delivered"
	case Read:
		return "read"
	case Played:
		return "played"
	}
	return "unknown"
}

// Status is a message's delivery state.
type Status struct {
	State State
	// Raw is the page's own number, kept so a caller looking at a build that
	// renumbered its enum can see what actually came back.
	Raw int
	// FromMe says whose message it is. For an incoming message the ack is about
	// what THIS account has acknowledged, which is a different question from
	// "did they read mine" — and conflating the two is easy enough to be worth
	// naming here.
	FromMe bool
	// EnumSource says where the mapping came from, or "fallback" when the page
	// did not expose one.
	EnumSource string
	Waited     time.Duration
}

func (s Status) String() string {
	return fmt.Sprintf("ack.Status(state=%s raw=%d fromMe=%t enum=%s waited=%s)",
		s.State, s.Raw, s.FromMe, s.EnumSource, s.Waited.Round(time.Millisecond))
}

// Reader reads delivery states on one session.
type Reader struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Reader.
func New(runner *engine.Runner, eval spa.Evaluator) *Reader {
	return &Reader{runner: runner, eval: eval}
}

// Of reads one message's delivery state.
func (r *Reader) Of(ctx context.Context, msgID, label string) (Status, error) {
	if strings.TrimSpace(msgID) == "" {
		return Status{}, ErrNoMessage
	}
	start := time.Now()

	var raw string
	if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/read", func(ctx context.Context) error {
		return r.eval(ctx, ackScript(msgID), &raw)
	}); err != nil {
		return Status{}, fmt.Errorf("%w: %v", ErrAck, err)
	}
	var out struct {
		OK     bool   `json:"ok"`
		Why    string `json:"why"`
		Ack    int    `json:"ack"`
		Name   string `json:"name"`
		FromMe bool   `json:"fromMe"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return Status{}, fmt.Errorf("ack: unexpected answer: %w", err)
	}
	if !out.OK {
		if out.Why == "NOT_LOADED" {
			return Status{}, ErrNoMessage
		}
		return Status{}, fmt.Errorf("%w (%s)", ErrAck, out.Why)
	}
	return Status{State: fromName(out.Name), Raw: out.Ack, FromMe: out.FromMe,
		EnumSource: out.Source, Waited: time.Since(start)}, nil
}

// fromName maps the page's own name for a state onto this package's. Mapping by
// NAME rather than by number is what makes a renumbered enum harmless.
func fromName(name string) State {
	switch strings.ToUpper(name) {
	case "ERROR":
		return Error
	case "PENDING", "CLOCK":
		return Pending
	case "SENT", "SERVER":
		return Sent
	case "DELIVERED", "DEVICE":
		return Delivered
	case "READ":
		return Read
	case "PLAYED":
		return Played
	}
	return Unknown
}

func ackScript(msgID string) string {
	return `JSON.stringify((() => {
		try {
			const MC = window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection;
			let msg = null;
			for (const m of MC.getModelsArray()) {
				try { if (m.id && m.id.id === ` + strconv.Quote(msgID) + `) { msg = m; break; } } catch (e) {}
			}
			if (!msg) { return { ok: false, why: 'NOT_LOADED' }; }
			const ack = (typeof msg.ack === 'number') ? msg.ack : -99;

			// THE MAPPING IS READ FROM THE PAGE, by looking for the enum entry
			// whose VALUE equals this message's ack and reporting its NAME. A
			// build that renumbers its states stays readable; a hard-coded table
			// would silently mislabel every message.
			let name = '', source = 'fallback';
			try {
				const A = window.require('` + string(spa.ModuleAck) + `');
				for (const key of ['ACK', 'Ack', 'default']) {
					const e = A && A[key];
					if (!e || typeof e !== 'object') { continue; }
					for (const k of Object.keys(e)) {
						if (e[k] === ack) { name = k; source = 'WAWebAck.' + key; break; }
					}
					if (name) { break; }
				}
			} catch (e) {}
			if (!name) {
				// The fallback is named as such in the result, so a caller can
				// tell a measured label from a guessed one.
				name = ({'-1': 'ERROR', '0': 'PENDING', '1': 'SENT',
				         '2': 'DELIVERED', '3': 'READ', '4': 'PLAYED'})[String(ack)] || '';
			}
			return { ok: true, why: '', ack: ack, name: name,
				fromMe: !!(msg.id && msg.id.fromMe), source: source };
		} catch (e) {
			return { ok: false, why: String((e && e.message) || e).slice(0, 160) };
		}
	})())`
}
