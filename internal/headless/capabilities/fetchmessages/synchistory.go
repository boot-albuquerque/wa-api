package fetchmessages

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wa-api/internal/headless/engine"
)

const syncStateKey = "__headlessSyncHistory"

// Budgets. Var so a test can compress them.
var (
	syncBudget = 30 * time.Second
	syncTick   = 250 * time.Millisecond
)

var (
	// ErrNoChat is a jid this session has no conversation for.
	ErrNoChat = fmt.Errorf("fetchmessages: no such conversation")
	// ErrSync is the page refusing or throwing.
	ErrSync = fmt.Errorf("fetchmessages: the page refused the history request")
)

// SyncRequest is what asking the phone for history did.
//
// IT REPORTS A REQUEST, NOT A SYNC, and the name says so on purpose. The history
// arrives later, over the socket, into the message collection — there is nothing
// this call can read back to prove it worked, and a type called SyncResult would
// invite exactly the silent success invariant 14 forbids.
type SyncRequest struct {
	// Requested says the request left. False is not a failure: it is the guard
	// answering that this conversation has nothing left to fetch.
	Requested bool
	// TransferType is the page's own endOfHistoryTransferType for the chat, and
	// it is carried because it is the ONLY thing that explains a false. Measured
	// across 389 chats: 378 read 0 (there is history to ask for), 5 read 1, and
	// 6 carry no such field at all.
	TransferType int
	// HasTransferType separates "the page says 0" from "the page says nothing" —
	// 6 of 389 chats had no field, and merging them into 0 would report that a
	// request is possible for a conversation the page never described.
	HasTransferType bool
}

func (s SyncRequest) String() string {
	return fmt.Sprintf("fetchmessages.SyncRequest(requested=%t hasType=%t type=%d)",
		s.Requested, s.HasTransferType, s.TransferType)
}

// SyncHistory asks the PHONE to send this conversation's older history.
//
// IT IS A DIFFERENT ACT FROM Fetch, and that distinction is the whole reason the
// ledger row stayed open: Fetch reads what this session already holds, and this
// asks the paired phone to send more. A caller who wanted the second and got the
// first would see an empty answer and conclude the conversation is empty.
//
// THE GUARD IS THE REFERENCE'S AND IT IS NOT DECORATION. It only requests when
// endOfHistoryTransferType is 0; any other value means the phone has already sent
// everything it is going to, and asking again is asking for nothing. Skipping it
// would turn "there is nothing left" into an indistinguishable success.
//
// NOTHING CONFIRMS IT IN THIS SESSION. The history arrives asynchronously, and
// this returns what it DID rather than what it achieved.
func (f *Fetcher) SyncHistory(ctx context.Context, chatJID, label string) (SyncRequest, error) {
	if strings.TrimSpace(chatJID) == "" {
		return SyncRequest{}, ErrNoChat
	}
	// O SCRIPT ESTACIONA A RESPOSTA E O GO ESPERA POR ELA. O `Evaluate` deste
	// modulo nao aguarda promessa (invariante 6), entao ler o retorno do kick
	// leria a string "kicked" — que e' o defeito que a primeira versao disto
	// tinha, e que so' apareceu porque o JSON nao decodificou.
	var kicked string
	if err := f.runner.Do(ctx, engine.OpStateProbe, label+"/synchistory/kick", func(ctx context.Context) error {
		return f.eval(ctx, syncScript(chatJID), &kicked)
	}); err != nil {
		return SyncRequest{}, fmt.Errorf("%w: %v", ErrSync, err)
	}
	var raw string
	deadline := time.Now().Add(syncBudget)
	for {
		if err := f.runner.Do(ctx, engine.OpStateProbe, label+"/synchistory/read", func(ctx context.Context) error {
			return f.eval(ctx, `window.`+syncStateKey+` || ""`, &raw)
		}); err != nil {
			return SyncRequest{}, fmt.Errorf("%w: %v", ErrSync, err)
		}
		if raw != "" {
			break
		}
		if !time.Now().Before(deadline) {
			return SyncRequest{}, fmt.Errorf("%w: the page never settled within %s", ErrSync, syncBudget)
		}
		select {
		case <-ctx.Done():
			return SyncRequest{}, ctx.Err()
		case <-time.After(syncTick):
		}
	}
	var out struct {
		OK       bool   `json:"ok"`
		Why      string `json:"why"`
		NotFound bool   `json:"notFound"`
		Sent     bool   `json:"sent"`
		HasType  bool   `json:"hasType"`
		Type     int    `json:"type"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return SyncRequest{}, fmt.Errorf("fetchmessages: unexpected answer: %w", err)
	}
	if out.NotFound {
		return SyncRequest{}, ErrNoChat
	}
	if !out.OK {
		return SyncRequest{}, fmt.Errorf("%w (%s)", ErrSync, out.Why)
	}
	return SyncRequest{Requested: out.Sent, TransferType: out.Type, HasTransferType: out.HasType}, nil
}

func syncScript(chatJID string) string {
	return `(() => {
	window.` + syncStateKey + ` = null;
	const park = v => { window.` + syncStateKey + ` = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 140);
	(async () => {
	try {
		const CC = window.require("WAWebChatCollection").ChatCollection;
		const chat = CC.get(` + strconv.Quote(chatJID) + `);
		if (!chat) { park({ ok: true, notFound: true }); return; }

		const t = chat.endOfHistoryTransferType;
		const hasType = (typeof t === "number");
		// A GUARDA VEM ANTES DO PEDIDO. Pedir historico a uma conversa que ja'
		// entregou tudo gasta um pedido e devolve "sucesso" indistinguivel de um
		// pedido util.
		if (!hasType || t !== 0) {
			park({ ok: true, notFound: false, sent: false, hasType: hasType, type: hasType ? t : 0 });
			return;
		}
		// A referencia passa DOIS argumentos a uma funcao de aridade 3; o
		// terceiro tem padrao.
		await window.require("WAWebSendNonMessageDataRequest")
			.sendPeerDataOperationRequest(3, { chatId: chat.id });
		park({ ok: true, notFound: false, sent: true, hasType: true, type: 0 });
	} catch (e) {
		park({ ok: false, why: safe(e) });
	}
	})();
	return "kicked";
	})()`
}
