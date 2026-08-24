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
	// ErrNotEmptied is Clear's postcondition failing: the page accepted and
	// messages the clear could have removed are still there.
	//
	// IT IS NOT "the conversation is not empty". A clear always leaves one system
	// notification behind (see residualKind), and treating that as failure would
	// make every correct clear report one.
	ErrNotEmptied = fmt.Errorf("chats: the page accepted the clear and it did not empty")
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
	// MessagesAfter is what the conversation still holds. It is NOT expected to
	// be zero: a clear provably leaves one system notification behind, measured
	// three times (see residualKind). Reporting it lets a caller see the residue
	// instead of wondering why the count did not reach zero.
	MessagesAfter int
	KeptStarred   bool
	Waited        time.Duration
}

func (e Emptied) String() string {
	return fmt.Sprintf("chats.Emptied(messagesBefore=%d messagesAfter=%d keptStarred=%t waited=%s)",
		e.MessagesBefore, e.MessagesAfter, e.KeptStarred, e.Waited.Round(time.Millisecond))
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
	// A POS-CONDICAO, e ela FALHA (decisao 67). Ate' hoje esta capacidade
	// devolvia o mesmo valor de sucesso tivesse apagado tudo ou nada, o que e' o
	// sucesso silencioso que a invariante 14 proibe.
	//
	// O criterio e' "nao sobrou nada limpavel", nao "diminuiu": limpar um chat ja'
	// limpo remove zero, corretamente, e a regra ingenua chamaria isso de falha
	// (H175).
	if out.Clearable > 0 {
		return Emptied{}, fmt.Errorf("%w: %d message(s) the clear could have removed are still there",
			ErrNotEmptied, out.Clearable)
	}
	return Emptied{MessagesBefore: out.Count, MessagesAfter: out.After,
		KeptStarred: keepStarred, Waited: time.Since(start)}, nil
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
	After int    `json:"after"`
	// Clearable is how many messages the clear could still have removed and did
	// not. -1 means the step did not run (Delete never verifies this way).
	Clearable int `json:"clearable"`
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

// residualKind is the message type a clear provably CANNOT remove.
//
// MEASURED, NOT ASSUMED (H175). Three clears on a throwaway group, and the
// residue was the same every time:
//
//	grupo virgem      2 (e2e_notification, gp2)  ->  1 (e2e_notification)
//	com 2 mensagens   3 (e2e_notification, chat) ->  1 (e2e_notification)
//	já limpo          1 (e2e_notification)       ->  1 (e2e_notification)
//
// The gp2 IS cleared; only this one survives. That third line is why the
// postcondition cannot be "after < before": clearing an already-clear chat
// removes nothing, correctly, and a naive rule would call that a failure.
//
// IF A FUTURE BUILD LEAVES ANOTHER TYPE BEHIND, this fails loudly rather than
// passing quietly — which is the direction that gets it re-measured.
const residualKind = "e2e_notification"

func clearScript(chatJID string, keepStarred bool) string {
	return lifecycleScript(chatJID, `
			stage = 'apply';
			const A = window.require('`+string(spa.ModuleSendClearChatAction)+`');
			// TWO POSITIONAL, and the second is keepStarred — named from the UI
			// checkbox that calls it.
			await A.sendClear(chat, `+strconv.FormatBool(keepStarred)+`);`,
		`
			// A POS-CONDICAO E' A AUSENCIA DO QUE E' LIMPAVEL, nao a reducao.
			// Ver residualKind para as tres medicoes que descartaram "after <
			// before" — a terceira, limpar o que ja' esta limpo, e' um no-op
			// correto que a regra ingenua reportaria como falha.
			after = 0; clearable = 0;
			try {
				const MC2 = window.require('`+string(spa.ModuleMsgCollection)+`').MsgCollection;
				const key2 = chat.id.toString();
				for (const m of MC2.getModelsArray()) {
					try {
						if (!(m.id && m.id.remote && m.id.remote.toString() === key2)) { continue; }
						after++;
						if (String(m.type || '') !== `+strconv.Quote(residualKind)+`) { clearable++; }
					} catch (e) {}
				}
			} catch (e) {}`)
}

func deleteScript(chatJID string) string {
	// SEM PASSO DE VERIFICACAO AQUI: o Delete tira a conversa inteira, entao
	// recontar mensagens dela nao tem o que ler. A pos-condicao do Delete e' o
	// chat sumir da colecao, provada por `chats.ByJID` na H166.
	return lifecycleScript(chatJID, `
			stage = 'apply';
			const A = window.require('`+string(spa.ModuleDeleteChatAction)+`');
			// The second argument defaults to true in the app's own body; it is
			// passed explicitly so a future default change cannot silently alter
			// what this does.
			await A.sendDelete(chat, true);`, "")
}

func lifecycleScript(chatJID, apply, verify string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(lifecycleStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(lifecycleStateKey) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const CC = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const chat = CC.get(` + strconv.Quote(chatJID) + `);
			if (!chat) { park({ stage, ok: false, why: 'NO_CHAT' }); return; }

			// CONTADO ANTES, e — para o Clear — TAMBEM DEPOIS.
			//
			// O comentario antigo aqui dizia que "after is meaningless: the point
			// of both acts is that there is nothing left to count". Isso e' falso
			// e foi medido (H175): um clear SEMPRE deixa uma notificacao de
			// sistema. Foi essa frase que manteve a capacidade sem pos-condicao,
			// e um Clear que nao apagasse nada devolvia o mesmo valor de sucesso.
			let count = 0, after = -1, clearable = -1;
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
` + verify + `
			park({ stage: 'done', ok: true, why: '', count: count, after: after, clearable: clearable });
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
