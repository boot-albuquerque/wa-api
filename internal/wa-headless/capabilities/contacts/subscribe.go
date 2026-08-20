package contacts

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// This file answers the product's onContact.
//
// IT LISTENS FOR "change", NOT "add", and that is the finding rather than a
// preference. The message subscription in capabilities/messagemeta binds 'add'
// and works; copying that word here would install cleanly and deliver nothing,
// because a ROSTER changes by rows being updated, not by rows appearing.
//
// Measured 2026-08-20 over 90 seconds on the live lab roster, with named
// handlers bound for add/change/remove/reset/update/sort/sync/destroy:
//
//	change    7      <- the only named event that fired
//	add       0
//	remove    0
//	reset / update / sort / sync / destroy   0
//
//	35 distinct names seen through the catch-all, ALL of them change or
//	change:<field>, grouped as profilePicThumb 24, businessProfile 22, change 7
//
// A SELF-TEST made that measurement trustworthy. "Nothing fired" is ambiguous
// between "this build has no such event" and "the roster was quiet", so the
// probe triggers a private event on itself and checks the handler saw it. It
// did — so the vocabulary works, and the zeros above are real zeros.
//
// 'add' IS STILL BOUND. Ninety quiet seconds do not prove a brand-new contact
// would not arrive that way, and binding a second name costs nothing. The event
// name travels with each event so the counters can tell the truth later instead
// of this comment having to guess.

// EventAdd and EventChange are the two collection events this binds.
//
// They are constants and not literals because each appears in the install
// script and again in the decoding, and a name repeated across a producer and a
// consumer is the same bug waiting to diverge (ADR-0004).
const (
	EventAdd    = "add"
	EventChange = "change"
)

// ErrNotInstalled is a drain against a page with no subscription.
var ErrNotInstalled = fmt.Errorf("contacts: the subscription is not installed")

// DefaultBuffer bounds what the page holds between drains.
//
// The roster is chattier than it looks: a single picture refresh produced
// twelve field-level events for one contact. This subscription collapses those
// to one event per model change, but the buffer is still sized for bursts.
const DefaultBuffer = 256

// Event is one contact change the page observed.
type Event struct {
	// Name is EventAdd or EventChange. Carried rather than assumed, because
	// only 'change' was ever seen firing and a future 'add' must be
	// distinguishable rather than silently relabelled.
	Name string
	// Contact is the SINGLE-ROW view: a change event carries one row, so there
	// is no second row to merge. A caller that needs both identities of a
	// person should re-read the roster with List.
	Contact Contact
}

func (e Event) String() string {
	return fmt.Sprintf("contacts.Event(name=%s %s)", e.Name, e.Contact)
}

// Drain is what one drain collected.
type Drain struct {
	Events []Event
	// Dropped is what the buffer refused because it was full. A caller that
	// ignores this is reading a truncated sequence as a complete one.
	Dropped int
	// Seen is every event the handler received, including dropped ones.
	Seen int
	// Reinstalled says the subscription had vanished and was put back. The
	// events between the loss and the reinstall are GONE and uncountable, so
	// the sequence is broken rather than merely empty.
	Reinstalled bool
}

func (d Drain) String() string {
	return fmt.Sprintf("contacts.Drain(events=%d dropped=%d seen=%d reinstalled=%t)",
		len(d.Events), d.Dropped, d.Seen, d.Reinstalled)
}

// Subscription observes contact changes for one session.
type Subscription struct {
	runner *engine.Runner
	eval   spa.Evaluator
	size   int
}

// Subscribe builds a Subscription. A size of zero uses DefaultBuffer.
func Subscribe(runner *engine.Runner, eval spa.Evaluator, size int) *Subscription {
	if size <= 0 {
		size = DefaultBuffer
	}
	return &Subscription{runner: runner, eval: eval, size: size}
}

const subStateKey = "__waHeadlessContactSub"

func (s *Subscription) installScript() string {
	return `JSON.stringify((() => {
		const KEY = ` + strconv.Quote(subStateKey) + `;
		if (window[KEY] && window[KEY].installed) {
			return { installed: true, already: true };
		}
		const mod = window.require('` + string(spa.ModuleContactCollection) + `');
		const coll = mod && mod.ContactCollection;
		if (!coll || typeof coll.on !== 'function') {
			return { installed: false, reason: 'NO_COLLECTION' };
		}
		const row = ` + RowExpr + `;
		const state = { buf: [], dropped: 0, seen: 0, installed: false, cap: ` +
		strconv.Itoa(s.size) + ` };
		const push = function (name) {
			return function (m) {
				state.seen++;
				if (state.buf.length >= state.cap) { state.dropped++; return; }
				let r = null;
				try { r = row(m); } catch (e) { state.dropped++; return; }
				if (!r) { state.dropped++; return; }
				state.buf.push({ name: name, row: r });
			};
		};
		// 'change' is the one measured to fire for a roster; 'add' is bound
		// because ninety quiet seconds do not prove it never will.
		coll.on('` + EventChange + `', push('` + EventChange + `'));
		coll.on('` + EventAdd + `', push('` + EventAdd + `'));
		state.installed = true;
		window[KEY] = state;
		return { installed: true, already: false };
	})())`
}

var subDrainScript = `JSON.stringify((() => {
	const s = window[` + strconv.Quote(subStateKey) + `];
	if (!s || !s.installed) { return { installed: false }; }
	const out = { installed: true, events: s.buf, dropped: s.dropped, seen: s.seen };
	s.buf = [];
	s.dropped = 0;
	return out;
})())`

type wireEvent struct {
	Name string `json:"name"`
	Row  row    `json:"row"`
}

type wireSubDrain struct {
	Installed bool        `json:"installed"`
	Events    []wireEvent `json:"events"`
	Dropped   int         `json:"dropped"`
	Seen      int         `json:"seen"`
}

type wireSubInstall struct {
	Installed bool   `json:"installed"`
	Already   bool   `json:"already"`
	Reason    string `json:"reason"`
}

// Install puts the subscription in the page. It is idempotent, so a reinstall
// after a drain cannot leave two handlers counting the same change twice.
func (s *Subscription) Install(ctx context.Context, label string) error {
	var raw string
	if err := s.runner.Do(ctx, engine.OpStateProbe, label, func(ctx context.Context) error {
		return s.eval(ctx, s.installScript(), &raw)
	}); err != nil {
		return fmt.Errorf("contacts: installing: %w", err)
	}
	var got wireSubInstall
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return fmt.Errorf("contacts: unexpected install answer: %w", err)
	}
	if !got.Installed {
		return fmt.Errorf("%w: %s", ErrNotInstalled, got.Reason)
	}
	return nil
}

// Drain collects what the page buffered, reinstalling if the state vanished.
func (s *Subscription) Drain(ctx context.Context, label string) (Drain, error) {
	raw, err := s.read(ctx, label)
	if err != nil {
		return Drain{}, err
	}
	if !raw.Installed {
		if err := s.Install(ctx, label+"/reinstall"); err != nil {
			return Drain{}, err
		}
		again, err := s.read(ctx, label+"/after-reinstall")
		if err != nil {
			return Drain{}, err
		}
		out := decodeDrain(again)
		out.Reinstalled = true
		return out, nil
	}
	return decodeDrain(raw), nil
}

func (s *Subscription) read(ctx context.Context, label string) (wireSubDrain, error) {
	var raw string
	if err := s.runner.Do(ctx, engine.OpStateProbe, label, func(ctx context.Context) error {
		return s.eval(ctx, subDrainScript, &raw)
	}); err != nil {
		return wireSubDrain{}, fmt.Errorf("contacts: draining: %w", err)
	}
	var got wireSubDrain
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return wireSubDrain{}, fmt.Errorf("contacts: unexpected drain answer: %w", err)
	}
	return got, nil
}

func decodeDrain(w wireSubDrain) Drain {
	out := Drain{Dropped: w.Dropped, Seen: w.Seen}
	for _, e := range w.Events {
		// The SAME person filter as the listing. A change on the PSA sentinel
		// is still the sentinel, and letting it through here would put back
		// exactly what H41 took out — through the other door.
		if !isPerson(e.Row) {
			continue
		}
		name := strings.TrimSpace(e.Name)
		if name == "" {
			name = EventChange
		}
		out.Events = append(out.Events, Event{Name: name, Contact: contactOf(e.Row)})
	}
	return out
}
