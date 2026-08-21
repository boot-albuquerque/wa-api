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

// Marking a conversation read.
//
// THE CALL TAKES ONE OBJECT, read from the app's own invocation rather than
// guessed:
//
//	sendConversationSeen({chat, key, threadId, unreadDelta})
//
// The `key` is the chat's lastReceivedKey — the message being acknowledged.
// Acknowledging without naming what was read is not something the protocol
// offers, so a call that omitted it would be asking for something that does not
// exist.
//
// IT HAS A REAL POSTCONDITION, which is rarer here than it sounds: unreadCount
// must reach zero. Most outward operations in this module can only be verified
// from the other account; this one changes something observable on this side,
// and so it is verified rather than assumed.

// Bounds for the read acknowledgement. Var, not const, so tests can compress
// the clock.
var (
	markBudget = 20 * time.Second
	markTick   = 500 * time.Millisecond
)

var (
	// ErrMarkRead is the page refusing or throwing.
	ErrMarkRead = fmt.Errorf("chats: the page refused to mark the conversation read")
	// ErrNoSuchChat is a conversation this account does not have.
	ErrNoSuchChat = fmt.Errorf("chats: no such conversation")
	// ErrStillUnread is the postcondition: the call returned and the
	// conversation is still unread. It is the failure a caller cannot see, and
	// the reason this verifies at all.
	ErrStillUnread = fmt.Errorf("chats: the conversation is still unread after being marked")
)

// MarkResult is what marking a conversation read did.
type MarkResult struct {
	// Before is the unread count when the call started. Zero means there was
	// nothing to acknowledge, which is a legitimate outcome and not a failure.
	Before int
	// After is the unread count once the page settled.
	After int
	// Waited is how long the acknowledgement took to be reflected.
	Waited time.Duration
}

// Changed reports whether anything was actually acknowledged.
func (r MarkResult) Changed() bool { return r.Before != r.After }

func (r MarkResult) String() string {
	return fmt.Sprintf("chats.MarkResult(before=%d after=%d changed=%t waited=%s)",
		r.Before, r.After, r.Changed(), r.Waited.Round(time.Millisecond))
}

const markStateKey = "__waHeadlessMarkRead"

// MarkRead acknowledges a conversation and returns only after the page agrees.
//
// A conversation with nothing unread returns successfully with Changed() false:
// acknowledging an empty inbox is not an error, and treating it as one would
// make every idle chat look broken.
func (l *Lister) MarkRead(ctx context.Context, jid, label string) (MarkResult, error) {
	if strings.TrimSpace(jid) == "" {
		return MarkResult{}, ErrNoSuchChat
	}
	start := time.Now()

	var kicked string
	if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return l.eval(ctx, markScript(jid), &kicked)
	}); err != nil {
		return MarkResult{}, fmt.Errorf("%w: %v", ErrMarkRead, err)
	}

	var out struct {
		Stage  string `json:"stage"`
		OK     bool   `json:"ok"`
		Why    string `json:"why"`
		Before int    `json:"before"`
		After  int    `json:"after"`
	}
	deadline := time.Now().Add(markBudget)
	for {
		var raw string
		if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return l.eval(ctx, markResultScript, &raw)
		}); err != nil {
			return MarkResult{}, fmt.Errorf("%w: %v", ErrMarkRead, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return MarkResult{}, fmt.Errorf("chats: unexpected answer: %w", err)
		}
		if out.Stage != "pending" {
			break
		}
		if !time.Now().Before(deadline) {
			return MarkResult{}, fmt.Errorf("%w: the page never settled within %s", ErrMarkRead, markBudget)
		}
		time.Sleep(markTick)
	}
	switch {
	case out.Stage == "find" && !out.OK:
		return MarkResult{}, fmt.Errorf("%w (%s)", ErrNoSuchChat, out.Why)
	case !out.OK:
		return MarkResult{}, fmt.Errorf("%w at %s (%s)", ErrMarkRead, out.Stage, out.Why)
	}

	res := MarkResult{Before: out.Before, After: out.After, Waited: time.Since(start)}
	// THE POSTCONDITION. A conversation that had unread messages and still has
	// them was not acknowledged, whatever the call reported.
	if res.Before > 0 && res.After > 0 {
		return MarkResult{}, fmt.Errorf("%w: %d before, %d after", ErrStillUnread, res.Before, res.After)
	}
	return res, nil
}

func markScript(jid string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(markStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(markStateKey) + `] = v; };
		const resolveIdentity = ` + spa.ResolveIdentityExpr + `;
		(async () => {
		let stage = 'find';
		try {
			const Chats = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			// Resolve first: this build files chats under the identity the
			// server assigns, so a lookup by the number a caller typed finds
			// nothing (H34, H39).
			const r = await resolveIdentity(` + strconv.Quote(jid) + `);
			if (!r.ok) { park({ stage, ok: false, why: r.why }); return; }
			const chat = Chats.get(r.wid);
			if (!chat) { park({ stage, ok: false, why: 'NO_CHAT' }); return; }

			const before = (typeof chat.unreadCount === 'number' && chat.unreadCount > 0)
				? chat.unreadCount : 0;
			if (before === 0) {
				// Nothing to acknowledge. Calling anyway would send a receipt
				// for a message the account may not have, and the honest answer
				// is that there was nothing to do.
				park({ stage: 'done', ok: true, why: '', before: 0, after: 0 });
				return;
			}

			stage = 'seen';
			const Seen = window.require('` + string(spa.ModuleSendConversationSeen) + `');
			// ONE OBJECT, and the key names WHAT is being acknowledged. Read
			// from the app's own call.
			await Seen.sendConversationSeen({
				chat: chat,
				key: chat.lastReceivedKey,
				unreadDelta: before
			});

			stage = 'verify';
			const after = (typeof chat.unreadCount === 'number' && chat.unreadCount > 0)
				? chat.unreadCount : 0;
			park({ stage: 'done', ok: true, why: '', before: before, after: after });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

const markResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + markStateKey + `"` + `];
	if (!s) { return { stage: 'seen', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`
