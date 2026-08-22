package chats

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

// Marking a conversation UNREAD. NOT PROVEN AGAINST THE LIVE BUILD — see H78.
//
// TWO PRIMITIVES WERE MEASURED AND NEITHER MARKS THE CHAT. Cross-session, the
// count went 0 -> 0 both times. This file is kept because it encodes those
// measurements and its tests lock them; it is not wired to anything.
//
// IT IS NOT THE SAME PRIMITIVE AS MARKING READ, and believing it was cost two
// live runs. sendConversationSeen({chat, key, threadId, unreadDelta}) with
// unreadDelta -1 does NOTHING: measured across sessions, the count went 0 -> 0.
//
// The real one is a Cmd verb, found from the app's own caller:
//
//	Cmd.markChatUnread(chat, wanted)      // and the caller toggles !chat.markedUnread
//
// AND THAT DID NOT WORK EITHER, which taught the thing worth keeping: Cmd is an
// EVENT BUS, not an action surface. Its body is
//
//	i.markChatUnread = function(t, n) { this.trigger("mark_chat_unread", …) }
//
// so a verb only does something when a listener is bound to it. Starring works
// because its listener is in the always-loaded core; this one's listener lives
// in a UI chunk a headless session never loads. That explains why some Cmd verbs
// have worked here and others silently do nothing — and it is a warning for
// every future Cmd call, not a fact about unread.
//
// IT DID NOT APPEAR IN MY OWN SURVEY OF Cmd, and the reason is worth keeping: I
// listed Cmd's keys filtered by /^send/, because everything I had driven there
// so far was named sendSomething. The filter encoded an assumption about naming
// and hid the answer. A survey narrowed by a guess is a guess.
//
// THE FIELD IS chat.markedUnread. An earlier probe reported it "absent", which
// was true and misleading: it is undefined until something sets it.
//
// AND IT CANNOT BE CONFIRMED IN-SESSION, which is measured and not assumed. The
// first live run reported the lab chat at 22 unread before and after; a FRESH
// session read the same chat as 0. The count this session sees is not what the
// account has, and it does not move when this session changes it — the same
// wall H58 hit with group metadata, in a second place.
//
// So Verified is false for a real change, and the honest proof is across
// sessions. There is no markedUnread field on the chat to consult either: it was
// measured absent.
//
// A CONSEQUENCE FOR MarkRead, recorded rather than hidden: its postcondition
// asserts the count moved IN-SESSION, which is the check this measurement just
// showed unreliable. H52 proved MarkRead against a chat where the count did
// move; whether that generalises is now an open question and has a HOUSEKEEP
// entry.

// UnreadMark is what marking a conversation unread did.
type UnreadMark struct {
	// Before is the unread count when the call started.
	Before int
	// After is what this session could still see, which after a real change is
	// the OLD value: the count is stale with respect to this session's own
	// writes. It is reported anyway so a caller can see that it did not move.
	After int
	// NoOp is true when the conversation was already marked.
	NoOp bool
	// Verified says whether the change was CONFIRMED. It is true only for a
	// no-op, because a real change is invisible to the session that made it —
	// measured, not assumed. Reporting a change as confirmed here would be a
	// lie the caller cannot detect.
	Verified bool
	Waited   time.Duration
}

func (u UnreadMark) String() string {
	return fmt.Sprintf("chats.UnreadMark(before=%d after=%d noop=%t verified=%t waited=%s)",
		u.Before, u.After, u.NoOp, u.Verified, u.Waited.Round(time.Millisecond))
}

// UnreadCount reports what this session sees for a chat.
//
// IT IS CORRECT AT SESSION START AND STALE AFTER A CHANGE THIS SESSION MADE,
// which is what makes cross-session the only honest proof: mark in one session,
// count in the next.
func (l *Lister) UnreadCount(ctx context.Context, jid, label string) (int, error) {
	if strings.TrimSpace(jid) == "" {
		return 0, ErrNoSuchChat
	}
	var raw string
	if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/unread-count", func(ctx context.Context) error {
		return l.eval(ctx, unreadCountScript(jid), &raw)
	}); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrLifecycle, err)
	}
	t := strings.TrimSpace(raw)
	if t == "NO_CHAT" {
		return 0, ErrNoSuchChat
	}
	n, err := strconv.Atoi(t)
	if err != nil {
		return 0, fmt.Errorf("chats: unexpected unread count: %w", err)
	}
	return n, nil
}

func unreadCountScript(jid string) string {
	return `(() => {
		try {
			const CC = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const chat = CC.get(` + strconv.Quote(jid) + `);
			if (!chat) { return 'NO_CHAT'; }
			// THE DELIBERATE MARK IS A FLAG, not a count, so it is reported as
			// a negative count: a caller asking "how many are waiting" and a
			// caller asking "did somebody leave this on purpose" want different
			// answers, and -1 is how this package says the second.
			if (chat.markedUnread === true) { return "-1"; }
			// STRING, because Evaluate unmarshals into one and a bare number is
			// a type error rather than a count.
			return String(typeof chat.unreadCount === 'number' ? chat.unreadCount : 0);
		} catch (e) { return 'NO_CHAT'; }
	})()`
}

const unreadStateKey = "__waHeadlessMarkUnread"

// MarkUnread leaves a conversation deliberately unread.
func (l *Lister) MarkUnread(ctx context.Context, jid, label string) (UnreadMark, error) {
	// IDEM ByJID (decisão 66): recusa explícita antes de agir. Medido, esta
	// função respondia "no such conversation" para o jid de telefone do par de
	// laboratório e EXECUTAVA para o mesmo par sob o lid.
	if spa.IsUnresolvedIdentity(jid) {
		return UnreadMark{}, ErrUnresolvedIdentity
	}
	if strings.TrimSpace(jid) == "" {
		return UnreadMark{}, ErrNoSuchChat
	}
	start := time.Now()

	var kicked string
	if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return l.eval(ctx, markUnreadScript(jid), &kicked)
	}); err != nil {
		return UnreadMark{}, fmt.Errorf("%w: %v", ErrLifecycle, err)
	}

	var out struct {
		Stage   string `json:"stage"`
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		Before  int    `json:"before"`
		After   int    `json:"after"`
		Already bool   `json:"already"`
	}
	deadline := time.Now().Add(lifecycleBudget)
	for {
		var raw string
		if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return l.eval(ctx, markUnreadResultScript, &raw)
		}); err != nil {
			return UnreadMark{}, fmt.Errorf("%w: %v", ErrLifecycle, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return UnreadMark{}, fmt.Errorf("chats: unexpected unread answer: %w", err)
		}
		if out.Stage != "pending" {
			break
		}
		if !time.Now().Before(deadline) {
			return UnreadMark{}, fmt.Errorf("%w: the page never settled within %s", ErrLifecycle, lifecycleBudget)
		}
		time.Sleep(lifecycleTick)
	}

	switch {
	case out.Why == "NO_CHAT":
		return UnreadMark{}, ErrNoSuchChat
	case !out.OK:
		return UnreadMark{}, fmt.Errorf("%w at %s (%s)", ErrLifecycle, out.Stage, out.Why)
	}
	return UnreadMark{Before: out.Before, After: out.After,
		NoOp: out.Already, Verified: out.Already, Waited: time.Since(start)}, nil
}

func markUnreadScript(jid string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(unreadStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(unreadStateKey) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const CC = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const chat = CC.get(` + strconv.Quote(jid) + `);
			if (!chat) { park({ stage, ok: false, why: 'NO_CHAT' }); return; }

			const before = (typeof chat.unreadCount === 'number' ? chat.unreadCount : 0);
			// ALREADY MARKED is a no-op, for the same reason a redundant archive
			// is (H55): the caller's intent is already satisfied.
			if (chat.markedUnread === true) {
				park({ stage: 'done', ok: true, why: 'ALREADY', already: true,
					before: before, after: before });
				return;
			}

			stage = 'apply';
			const Cmd = window.require('` + string(spa.ModuleCmd) + `').Cmd;
			// TWO POSITIONAL, and the second is the WANTED state — read from the
			// app's own caller, which passes !chat.markedUnread to toggle.
			Cmd.markChatUnread(chat, true);

			// NO POSTCONDITION HERE, and that is the measured conclusion. The
			// count this session reads is stale with respect to its own writes,
			// so waiting on it produced a wrong answer for 30 seconds.
			park({ stage: 'done', ok: true, why: '', already: false,
				before: before, after: before });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

// markUnreadResultScript returns what the kick parked. There is no settling
// branch: nothing this session can read reflects a mark it made.
const markUnreadResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + unreadStateKey + `"` + `];
	if (!s) { return { stage: 'apply', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`
