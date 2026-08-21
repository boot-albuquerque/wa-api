package contacts

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

// Reading the account's business labels, and which ones a chat carries.
//
// LABELS ONLY EXIST ON A BUSINESS ACCOUNT, and this one is (H66, measured via
// canSetMyPushname). On a personal account the collection is simply empty, which
// is why an empty list here is a successful answer and not an error: the caller
// cannot tell the two apart from the outside, and pretending to would be a
// claim about the account this package has no business making.
//
// A LABEL'S NAME IS SOMETHING THE OWNER WROTE. Label carries it for the caller
// and renders only a length, like every other piece of content in this module.

var (
	// ErrLabels is the page refusing or throwing.
	ErrLabels = fmt.Errorf("contacts: the page refused to read labels")
	// ErrNoSuchChatForLabels is a jid with no loaded chat.
	ErrNoSuchChatForLabels = fmt.Errorf("contacts: no such chat is loaded")
)

// Label is one business label.
type Label struct {
	// ID is the label's identity, a short string on this build ("1", "2", "3"
	// for the defaults). It is structure, not content.
	ID string
	// Name is what the owner called it.
	Name string
	// ColorIndex is the palette slot, not a colour value.
	ColorIndex int
	// Count is how many things the page says carry this label.
	Count int
}

// String renders the id and the NAME'S LENGTH.
func (l Label) String() string {
	return fmt.Sprintf("contacts.Label(id=%s nameLen=%d color=%d count=%d)",
		l.ID, len([]rune(l.Name)), l.ColorIndex, l.Count)
}

// Labels is the account's label set.
type Labels struct {
	All    []Label
	Waited time.Duration
}

func (l Labels) String() string {
	return fmt.Sprintf("contacts.Labels(count=%d waited=%s)",
		len(l.All), l.Waited.Round(time.Millisecond))
}

// ListLabels reads every label this account has.
func (l *Lister) ListLabels(ctx context.Context, label string) (Labels, error) {
	start := time.Now()
	var raw string
	if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/labels", func(ctx context.Context) error {
		return l.eval(ctx, listLabelsScript, &raw)
	}); err != nil {
		return Labels{}, fmt.Errorf("%w: %v", ErrLabels, err)
	}
	var out struct {
		OK   bool `json:"ok"`
		Why  string
		Rows []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Color int    `json:"color"`
			Count int    `json:"count"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return Labels{}, fmt.Errorf("contacts: unexpected label answer: %w", err)
	}
	if !out.OK {
		return Labels{}, fmt.Errorf("%w (%s)", ErrLabels, out.Why)
	}
	all := make([]Label, 0, len(out.Rows))
	for _, r := range out.Rows {
		all = append(all, Label{ID: r.ID, Name: r.Name, ColorIndex: r.Color, Count: r.Count})
	}
	// AN EMPTY SET IS A SUCCESSFUL ANSWER. A personal account has no labels, and
	// this package cannot tell that apart from a business account that made
	// none — so it reports what it sees instead of guessing which.
	return Labels{All: all, Waited: time.Since(start)}, nil
}

// LabelsOfChat reads the label ids a chat carries.
func (l *Lister) LabelsOfChat(ctx context.Context, chatJID, label string) ([]string, error) {
	if strings.TrimSpace(chatJID) == "" {
		return nil, ErrNoSuchChatForLabels
	}
	var raw string
	if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/chat-labels", func(ctx context.Context) error {
		return l.eval(ctx, chatLabelsScript(chatJID), &raw)
	}); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLabels, err)
	}
	var out struct {
		OK  bool     `json:"ok"`
		Why string   `json:"why"`
		IDs []string `json:"ids"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("contacts: unexpected chat-label answer: %w", err)
	}
	if !out.OK {
		if out.Why == "NO_CHAT" {
			return nil, ErrNoSuchChatForLabels
		}
		return nil, fmt.Errorf("%w (%s)", ErrLabels, out.Why)
	}
	// A CHAT WITH NO LABELS RETURNS AN EMPTY SLICE, never nil-with-an-error:
	// "this chat is not labelled" is an answer.
	if out.IDs == nil {
		return []string{}, nil
	}
	return out.IDs, nil
}

const listLabelsScript = `JSON.stringify((() => {
	try {
		const LC = window.require('` + string(spa.ModuleLabelCollection) + `').LabelCollection;
		const arr = LC.getModelsArray ? LC.getModelsArray() : [];
		const rows = [];
		for (const l of arr) {
			try {
				rows.push({ id: String(l.id), name: String(l.name || ''),
					color: Number(l.colorIndex || 0), count: Number(l.count || 0) });
			} catch (e) {}
		}
		return { ok: true, why: '', rows: rows };
	} catch (e) {
		return { ok: false, why: String((e && e.message) || e).slice(0, 160), rows: [] };
	}
})())`

func chatLabelsScript(chatJID string) string {
	return `JSON.stringify((() => {
		try {
			const CC = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const chat = CC.get(` + strconv.Quote(chatJID) + `);
			if (!chat) { return { ok: false, why: 'NO_CHAT', ids: [] }; }
			// chat.labels is an array of ID STRINGS on this build — measured,
			// not assumed, and String() is applied because a build that stored
			// numbers there would otherwise produce a silently different type.
			const ls = chat.labels ? [].concat(chat.labels) : [];
			return { ok: true, why: '', ids: ls.map(String) };
		} catch (e) {
			return { ok: false, why: String((e && e.message) || e).slice(0, 160), ids: [] };
		}
	})())`
}

// Applying and removing a label on a chat.
//
// THE SHAPE CAME FROM THE ARGUMENT INSTRUMENT (H73). editLabelAssociation is an
// opaque wrapper with no readable call shape, and a plain recorder answered only
// ".forEach" — because a recorder answers the iteration itself and the callback
// never runs. Handing it a real array holding one recorder produced the answer:
//
//	editLabelAssociation([{id, type}], [chatModel])
//
// THE TYPE VOCABULARY IS THE ONE THING NOT READ. The app's mirror call is named
// addOrRemoveLabelsMD, so "add" and "remove" are the obvious strings — and this
// package does not rely on that being right: the postcondition reads chat.labels
// and fails if the label did not actually attach. A wrong verb produces a clean
// failure here rather than a silent no-op.

var (
	// ErrLabelUnchanged is the postcondition: the call returned and the chat's
	// labels are what they were.
	ErrLabelUnchanged = fmt.Errorf("contacts: the page accepted the label change and the chat's labels did not move")
	// ErrNoLabelGiven is a call with no label id.
	ErrNoLabelGiven = fmt.Errorf("contacts: no label id given")
)

// LabelChange is what applying or removing a label did.
type LabelChange struct {
	// Before and After are how many labels the chat carried.
	Before, After int
	// NoOp is true when the chat already had, or already lacked, the label.
	NoOp bool
	// Waited is how long the page took to reflect it.
	Waited time.Duration
}

func (c LabelChange) String() string {
	return fmt.Sprintf("contacts.LabelChange(before=%d after=%d noop=%t waited=%s)",
		c.Before, c.After, c.NoOp, c.Waited.Round(time.Millisecond))
}

const labelWriteStateKey = "__waHeadlessLabelWrite"

// AddLabel applies a label to a chat.
func (l *Lister) AddLabel(ctx context.Context, chatJID, labelID, label string) (LabelChange, error) {
	return l.setLabel(ctx, chatJID, labelID, true, label)
}

// RemoveLabel takes it off.
func (l *Lister) RemoveLabel(ctx context.Context, chatJID, labelID, label string) (LabelChange, error) {
	return l.setLabel(ctx, chatJID, labelID, false, label)
}

func (l *Lister) setLabel(ctx context.Context, chatJID, labelID string, add bool, label string) (LabelChange, error) {
	if strings.TrimSpace(chatJID) == "" {
		return LabelChange{}, ErrNoSuchChatForLabels
	}
	if strings.TrimSpace(labelID) == "" {
		return LabelChange{}, ErrNoLabelGiven
	}
	start := time.Now()

	var kicked string
	if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return l.eval(ctx, labelWriteScript(chatJID, labelID, add), &kicked)
	}); err != nil {
		return LabelChange{}, fmt.Errorf("%w: %v", ErrLabels, err)
	}

	var out struct {
		Stage   string `json:"stage"`
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		Before  int    `json:"before"`
		After   int    `json:"after"`
		Already bool   `json:"already"`
	}
	deadline := time.Now().Add(commonBudget)
	for {
		var raw string
		if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return l.eval(ctx, labelWriteResultScript, &raw)
		}); err != nil {
			return LabelChange{}, fmt.Errorf("%w: %v", ErrLabels, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return LabelChange{}, fmt.Errorf("contacts: unexpected label-write answer: %w", err)
		}
		if out.Stage != "pending" && out.Stage != "settling" {
			break
		}
		if !time.Now().Before(deadline) {
			if out.Stage == "settling" {
				return LabelChange{}, fmt.Errorf("%w within %s (before=%d after=%d)",
					ErrLabelUnchanged, commonBudget, out.Before, out.After)
			}
			return LabelChange{}, fmt.Errorf("%w: the page never settled within %s", ErrLabels, commonBudget)
		}
		time.Sleep(commonTick)
	}

	switch {
	case out.Why == "NO_CHAT":
		return LabelChange{}, ErrNoSuchChatForLabels
	case out.Why == "NO_LABEL":
		return LabelChange{}, ErrNoLabelGiven
	case !out.OK:
		return LabelChange{}, fmt.Errorf("%w at %s (%s)", ErrLabels, out.Stage, out.Why)
	}
	return LabelChange{Before: out.Before, After: out.After,
		NoOp: out.Already, Waited: time.Since(start)}, nil
}

func labelWriteScript(chatJID, labelID string, add bool) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(labelWriteStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(labelWriteStateKey) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const add = ` + strconv.FormatBool(add) + `;
			const want = ` + strconv.Quote(labelID) + `;
			const CC = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const chat = CC.get(` + strconv.Quote(chatJID) + `);
			if (!chat) { park({ stage, ok: false, why: 'NO_CHAT' }); return; }

			const LC = window.require('` + string(spa.ModuleLabelCollection) + `').LabelCollection;
			const known = (LC.getModelsArray ? LC.getModelsArray() : [])
				.map(l => String(l.id));
			// A LABEL THAT DOES NOT EXIST would otherwise be applied to nothing
			// and reported as a change that did not take.
			if (known.indexOf(want) === -1) { park({ stage, ok: false, why: 'NO_LABEL' }); return; }

			const current = () => (chat.labels ? [].concat(chat.labels).map(String) : []);
			const before = current();
			const has = before.indexOf(want) !== -1;
			if (has === add) {
				park({ stage: 'done', ok: true, why: 'ALREADY', already: true,
					before: before.length, after: before.length });
				return;
			}

			stage = 'apply';
			const B = window.require('` + string(spa.ModuleEditLabelAssociationBridge) + `');
			// [{id, type}] and [chatModel] — measured with the argument
			// instrument, because the wrapper shows nothing and a plain recorder
			// answers forEach itself.
			const mutations = [{ id: want, type: add ? 'add' : 'remove' }];
			const chats = [chat];
			await B.editLabelAssociation(mutations, chats);
			// THE MIRROR, which the app calls right after on its own call site.
			// Without it chat.labels does not move, and chat.labels is the only
			// postcondition available.
			if (LC.addOrRemoveLabelsMD) { LC.addOrRemoveLabelsMD(mutations, chats); }

			stage = 'verify';
			park({ stage: 'settling', ok: true, why: '', already: false,
				before: before.length, want: want, add: add, chat: chat });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 200) });
		}
		})();
		return { started: true };
	})())`
}

// labelWriteResultScript re-reads the parked chat's labels each round and never
// serialises the model.
const labelWriteResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + labelWriteStateKey + `"` + `];
	if (!s) { return { stage: 'apply', ok: false, why: 'STATE_MISSING' }; }
	if (s.stage === 'settling') {
		const now = s.chat && s.chat.labels ? [].concat(s.chat.labels).map(String) : [];
		if ((now.indexOf(s.want) !== -1) === s.add) {
			return { stage: 'done', ok: true, why: '', already: false,
				before: s.before, after: now.length };
		}
		return { stage: 'settling', ok: false, why: '', before: s.before, after: now.length };
	}
	return s;
})())`
