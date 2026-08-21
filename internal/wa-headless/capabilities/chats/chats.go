// Package chats lists conversations.
//
// THE TITLE COMES FROM formattedTitle, NOT getName, and that is the finding
// rather than a preference. Measured 2026-08-20 over all 384 chats on the lab
// account:
//
//	getName          1 of 384
//	formattedTitle 384 of 384
//	neither          0
//
// WAWebChatGetters exports getName, so reaching for it is the obvious move —
// and it would have produced a listing with a title for one conversation in
// every 384. Worse, it would have passed every unit test, because a double
// returns whatever it is told. It is the same trap the contact roster carries,
// where getName also answered for 1 of 944 (H39): on a companion device the
// address book lives on the phone, and formattedTitle is what the app renders.
//
// THE ORDER IS OURS, NOT THE PAGE'S. The collection was measured newest-first,
// but nothing documents that and a listing whose order changes underneath is
// one no caller can diff. Sorting here costs nothing and removes the dependency.
package chats

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// DefaultLimit bounds a listing that does not ask for a size.
const DefaultLimit = 100

// ErrNoCollection is a page where the chat store did not resolve.
var ErrNoCollection = fmt.Errorf("chats: the chat collection is not available")

// Chat is one conversation.
type Chat struct {
	// JID is the conversation's identity — a person's lid, or a group jid.
	JID string
	// Title is what the app itself renders for this conversation. It is a
	// person's name or a group's subject, so it is PII and never rendered.
	Title string
	// Timestamp is the last activity, and it is what the ordering uses.
	Timestamp time.Time
	// Unread is how many messages are waiting. Measured: 122 of 384 chats had
	// at least one.
	Unread  int
	IsGroup bool
	// Archived, Pinned and Muted are carried because a listing that ignored
	// them would put an archived conversation next to an active one and call
	// them the same thing.
	Archived bool
	Pinned   bool
	Muted    bool
	// ReadOnly marks a conversation this account cannot write to — an
	// announcement group, a broadcast. A caller that offered a reply box there
	// would be offering something that cannot work.
	ReadOnly bool
}

// String redacts the title. Everything else about a conversation is structure;
// the title is a person.
func (c Chat) String() string {
	return fmt.Sprintf("chats.Chat(jid=<redacted> title=%t group=%t unread=%d at=%s archived=%t pinned=%t muted=%t readonly=%t)",
		c.Title != "", c.IsGroup, c.Unread,
		c.Timestamp.UTC().Format(time.RFC3339), c.Archived, c.Pinned, c.Muted, c.ReadOnly)
}

// List is a page of conversations WITH the denominators that make it readable.
type List struct {
	// Chats are the conversations, most recent first.
	Chats []Chat
	// Total is how many the page held before any limit. Total > len(Chats)
	// means the listing was truncated, and a caller reading a short list as a
	// complete one is the silent-incompleteness failure this module keeps
	// meeting.
	Total int
	// WithUnread is how many of the TOTAL have unread messages — not how many
	// of the returned page do. It is the number a caller actually wants when
	// asking "is there anything waiting", and computing it from a truncated
	// page would answer a different question.
	WithUnread int
}

// Truncated reports whether the limit cut the listing short.
func (l List) Truncated() bool { return l.Total > len(l.Chats) }

func (l List) String() string {
	return fmt.Sprintf("chats.List(returned=%d total=%d withUnread=%d truncated=%t)",
		len(l.Chats), l.Total, l.WithUnread, l.Truncated())
}

// Lister reads conversations for one session.
type Lister struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Lister.
func New(runner *engine.Runner, eval spa.Evaluator) *Lister {
	return &Lister{runner: runner, eval: eval}
}

type wireChat struct {
	JID      string `json:"jid"`
	Title    string `json:"title"`
	T        int64  `json:"t"`
	Unread   int    `json:"unread"`
	IsGroup  bool   `json:"is_group"`
	Archived bool   `json:"archived"`
	Pinned   bool   `json:"pinned"`
	Muted    bool   `json:"muted"`
	ReadOnly bool   `json:"read_only"`
}

type wireList struct {
	OK         bool       `json:"ok"`
	Total      int        `json:"total"`
	WithUnread int        `json:"with_unread"`
	Chats      []wireChat `json:"chats"`
}

func script() string {
	return `JSON.stringify((() => {
		const mod = window.require('` + string(spa.ModuleChatCollection) + `');
		const coll = mod && mod.ChatCollection;
		if (!coll || typeof coll.getModelsArray !== 'function') { return { ok: false }; }
		const all = coll.getModelsArray();
		const out = { ok: true, total: all.length, with_unread: 0, chats: [] };
		for (const c of all) {
			try {
				const id = c.id;
				if (!id || !id._serialized) { continue; }
				const unread = (typeof c.unreadCount === 'number' && c.unreadCount > 0) ? c.unreadCount : 0;
				if (unread > 0) { out.with_unread++; }
				out.chats.push({
					jid: id._serialized,
					// formattedTitle, NOT getName: measured 384 of 384 against
					// 1 of 384.
					title: (typeof c.formattedTitle === 'string') ? c.formattedTitle : '',
					t: (typeof c.t === 'number') ? c.t : 0,
					unread: unread,
					is_group: id.server === 'g.us',
					archived: !!c.archive,
					pinned: !!c.pin,
					muted: !!c.muteExpiration,
					read_only: !!c.isReadOnly
				});
			} catch (e) {}
		}
		return out;
	})())`
}

// List returns the most recent conversations.
//
// The limit applies AFTER ordering, so a caller asking for ten gets the ten
// most recent rather than whichever ten the page happened to hold first.
func (l *Lister) List(ctx context.Context, limit int, label string) (List, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	var raw string
	if err := l.runner.Do(ctx, engine.OpStateProbe, label, func(ctx context.Context) error {
		return l.eval(ctx, script(), &raw)
	}); err != nil {
		return List{}, fmt.Errorf("chats: reading the collection: %w", err)
	}
	var w wireList
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		return List{}, fmt.Errorf("chats: unexpected answer: %w", err)
	}
	if !w.OK {
		return List{}, ErrNoCollection
	}

	out := List{Total: w.Total, WithUnread: w.WithUnread}
	all := make([]Chat, 0, len(w.Chats))
	for _, c := range w.Chats {
		ch := Chat{
			JID: c.JID, Title: c.Title, Unread: c.Unread, IsGroup: c.IsGroup,
			Archived: c.Archived, Pinned: c.Pinned, Muted: c.Muted, ReadOnly: c.ReadOnly,
		}
		if c.T > 0 {
			ch.Timestamp = time.Unix(c.T, 0).UTC()
		}
		all = append(all, ch)
	}

	// MOST RECENT FIRST, decided here. The collection was measured newest-first,
	// but nothing documents that, and ties must break somewhere stable or two
	// calls can disagree about a list nobody changed.
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].Timestamp.Equal(all[j].Timestamp) {
			return all[i].Timestamp.After(all[j].Timestamp)
		}
		return all[i].JID < all[j].JID
	})
	if len(all) > limit {
		all = all[:limit]
	}
	out.Chats = all
	return out, nil
}
