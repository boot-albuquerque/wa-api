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

// Emptying and removing conversations.
//
// NEITHER OF THESE HAS A LIVE PROOF, and the omission is deliberate and written
// down rather than quietly skipped. The lab account has exactly one peer
// conversation, and every other live test in this module reads or writes it:
// fetchmessages counts its history, edit and forward find messages in it, mute
// and markread act on it. Clearing or deleting it once would cost all of them,
// and neither act can be undone.
//
// So they are built from measured signatures, unit tested against a double that
// imitates the real protocol, and left unrun. A capability that exists and says
// it was never proven live is more honest than one that was proven by wrecking
// the fixture.
//
// THE SIGNATURES, from the functions' own synchronous bodies and confirmed at
// the app's call sites:
//
//	sendClear(chat, keepStarred)
//	sendDelete(chat, syncToDevices = true)
//
// The second argument of sendClear is named from the UI beside it: a checkbox
// under the words "the conversation will be empty but will stay in your list".

// Bounds. Var, not const, so tests can compress the clock.
var (
	lifecycleBudget = 30 * time.Second
	lifecycleTick   = 500 * time.Millisecond
)

var (
	// ErrLifecycle is the page refusing or throwing.
	ErrLifecycle = fmt.Errorf("chats: the page refused")
	// ErrNoSuchChat is declared in markread.go and reused here on purpose: a
	// jid with no loaded chat is the same condition whichever capability meets
	// it, and two errors for one condition make callers write two branches.
)

// Emptied is what a clear did.
type Emptied struct {
	// MessagesBefore is how many messages the chat held. A count, never
	// content, and it is reported because "the chat is now empty" means
	// something different for a chat that held two messages and one that held
	// two thousand.
	MessagesBefore int
	// KeptStarred says whether starred messages were spared.
	KeptStarred bool
	Waited      time.Duration
}

func (e Emptied) String() string {
	return fmt.Sprintf("chats.Emptied(messagesBefore=%d keptStarred=%t waited=%s)",
		e.MessagesBefore, e.KeptStarred, e.Waited.Round(time.Millisecond))
}

const lifecycleStateKey = "__waHeadlessChatLifecycle"

// Clear empties a conversation, leaving it in the list.
//
// keepStarred is a parameter rather than a constant because sparing starred
// messages and destroying them are different intentions, and choosing the
// destructive one on a caller's behalf is not a default this package will make.
func (l *Lister) Clear(ctx context.Context, chatJID string, keepStarred bool, label string) (Emptied, error) {
	if strings.TrimSpace(chatJID) == "" {
		return Emptied{}, ErrNoSuchChat
	}
	start := time.Now()
	out, err := l.lifecycle(ctx, clearScript(chatJID, keepStarred), label)
	if err != nil {
		return Emptied{}, err
	}
	return Emptied{MessagesBefore: out.Count, KeptStarred: keepStarred,
		Waited: time.Since(start)}, nil
}

// Delete removes the conversation itself.
//
// IT DOES NOT LEAVE GROUPS FIRST. The app's own flow calls sendExitGroup and
// then sendDelete, and a caller who wants that sequence must ask for both:
// deleting a group chat while still a member removes the conversation and
// leaves the account in the group, which is a real and sometimes wanted state.
// Doing the extra step silently would be deciding something the caller did not.
func (l *Lister) Delete(ctx context.Context, chatJID, label string) error {
	if strings.TrimSpace(chatJID) == "" {
		return ErrNoSuchChat
	}
	_, err := l.lifecycle(ctx, deleteScript(chatJID), label)
	return err
}

type lifecycleOut struct {
	Stage string `json:"stage"`
	OK    bool   `json:"ok"`
	Why   string `json:"why"`
	Count int    `json:"count"`
}

func (l *Lister) lifecycle(ctx context.Context, kick, label string) (lifecycleOut, error) {
	var out lifecycleOut
	var kicked string
	if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return l.eval(ctx, kick, &kicked)
	}); err != nil {
		return out, fmt.Errorf("%w: %v", ErrLifecycle, err)
	}
	deadline := time.Now().Add(lifecycleBudget)
	for {
		var raw string
		if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return l.eval(ctx, lifecycleResultScript, &raw)
		}); err != nil {
			return out, fmt.Errorf("%w: %v", ErrLifecycle, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return out, fmt.Errorf("chats: unexpected answer: %w", err)
		}
		if out.Stage != "pending" {
			break
		}
		if !time.Now().Before(deadline) {
			return out, fmt.Errorf("%w: the page never settled within %s", ErrLifecycle, lifecycleBudget)
		}
		time.Sleep(lifecycleTick)
	}
	switch {
	case out.Why == "NO_CHAT":
		return out, ErrNoSuchChat
	case !out.OK:
		return out, fmt.Errorf("%w at %s (%s)", ErrLifecycle, out.Stage, out.Why)
	}
	return out, nil
}

func clearScript(chatJID string, keepStarred bool) string {
	return lifecycleScript(chatJID, `
			stage = 'apply';
			const A = window.require('`+string(spa.ModuleSendClearChatAction)+`');
			// TWO POSITIONAL, and the second is keepStarred — named from the UI
			// checkbox that calls it.
			await A.sendClear(chat, `+strconv.FormatBool(keepStarred)+`);`)
}

func deleteScript(chatJID string) string {
	return lifecycleScript(chatJID, `
			stage = 'apply';
			const A = window.require('`+string(spa.ModuleDeleteChatAction)+`');
			// The second argument defaults to true in the app's own body; it is
			// passed explicitly so a future default change cannot silently alter
			// what this does.
			await A.sendDelete(chat, true);`)
}

func lifecycleScript(chatJID, apply string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(lifecycleStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(lifecycleStateKey) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const CC = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const chat = CC.get(` + strconv.Quote(chatJID) + `);
			if (!chat) { park({ stage, ok: false, why: 'NO_CHAT' }); return; }

			// COUNTED BEFORE, because after is meaningless: the point of both
			// acts is that there is nothing left to count.
			let count = 0;
			try {
				const MC = window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection;
				const key = chat.id.toString();
				for (const m of MC.getModelsArray()) {
					try {
						if (m.id && m.id.remote && m.id.remote.toString() === key) { count++; }
					} catch (e) {}
				}
			} catch (e) {}
` + apply + `
			park({ stage: 'done', ok: true, why: '', count: count });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

const lifecycleResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + lifecycleStateKey + `"` + `];
	if (!s) { return { stage: 'apply', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`
