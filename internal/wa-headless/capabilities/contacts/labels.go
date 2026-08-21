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
