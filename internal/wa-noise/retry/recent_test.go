// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package retry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waMsgApplication"
	"wa-api/internal/wa-noise/types"
)

// --- RecentMessage ---

func TestRecentMessageIsEmpty(t *testing.T) {
	if !(RecentMessage{}).IsEmpty() {
		t.Error("RecentMessage zerada deveria ser vazia")
	}
	if (RecentMessage{WA: &waE2E.Message{}}).IsEmpty() {
		t.Error("RecentMessage com payload wa nao e' vazia")
	}
	if (RecentMessage{FB: &waMsgApplication.MessageApplication{}}).IsEmpty() {
		t.Error("RecentMessage com payload fb nao e' vazia")
	}
}

// --- ParseRecent ---

func TestParseRecentWAFormat(t *testing.T) {
	buf, err := proto.Marshal(waMessage("oi"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rm, err := ParseRecent(StoreFormatWA, buf)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if rm.FB != nil {
		t.Error("formato wa nao deveria preencher fb")
	}
	if rm.WA.GetConversation() != "oi" {
		t.Errorf("Conversation = %q, esperado oi", rm.WA.GetConversation())
	}
}

func TestParseRecentFBFormat(t *testing.T) {
	buf, err := proto.Marshal(&waMsgApplication.MessageApplication{
		Metadata: &waMsgApplication.MessageApplication_Metadata{
			FrankingKey: []byte("chave"),
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rm, err := ParseRecent(StoreFormatFB, buf)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if rm.WA != nil {
		t.Error("formato fb nao deveria preencher wa")
	}
	if string(rm.FB.GetMetadata().GetFrankingKey()) != "chave" {
		t.Errorf("FrankingKey = %q", rm.FB.GetMetadata().GetFrankingKey())
	}
}

func TestParseRecentUnknownFormat(t *testing.T) {
	rm, err := ParseRecent("json", []byte("{}"))
	if err == nil {
		t.Fatal("esperado erro para formato desconhecido")
	}
	if rm != nil {
		t.Error("nao deveria devolver mensagem junto com erro")
	}
	if !strings.Contains(err.Error(), "json") {
		t.Errorf("erro deveria nomear o formato recebido: %v", err)
	}
}

// O buffer vem do banco: se estiver corrompido, o parser precisa devolver erro
// em vez de deixar o protobuf meio preenchido escapar.
func TestParseRecentInvalidPayload(t *testing.T) {
	if _, err := ParseRecent(StoreFormatWA, []byte{0xFF, 0xFF, 0xFF}); err == nil {
		t.Error("esperado erro de unmarshal no formato wa")
	}
	if _, err := ParseRecent(StoreFormatFB, []byte{0xFF, 0xFF, 0xFF}); err == nil {
		t.Error("esperado erro de unmarshal no formato fb")
	}
}

// Formato vazio (o que GetOutgoingEvent devolve quando nao acha nada) cai no
// default e vira erro, nao mensagem vazia.
func TestParseRecentEmptyFormat(t *testing.T) {
	if _, err := ParseRecent("", nil); err == nil {
		t.Error("esperado erro para formato vazio")
	}
}

// --- buffer circular ---

func TestAddAndGetRecent(t *testing.T) {
	tr := newFakeTransport()
	msg := waMessage("oi")
	if err := AddRecent(context.Background(), tr, testPeerJID, "MSG1", msg, nil); err != nil {
		t.Fatalf("AddRecent: %v", err)
	}
	got := GetRecent(tr, testPeerJID, "MSG1")
	if got.WA != msg {
		t.Errorf("GetRecent devolveu %v, esperado a mesma mensagem", got.WA)
	}
	// Chave e' (destinatario, ID): trocar qualquer um dos dois nao acha.
	if !GetRecent(tr, testOwnJID, "MSG1").IsEmpty() {
		t.Error("JID diferente nao deveria bater")
	}
	if !GetRecent(tr, testPeerJID, "MSG2").IsEmpty() {
		t.Error("ID diferente nao deveria bater")
	}
}

// O buffer e' circular de RecentMessagesSize posicoes: a entrada
// RecentMessagesSize+1 sobrescreve a primeira, e a primeira tem que sair
// tambem do mapa — senao o mapa cresce sem limite (contraste com F36).
func TestAddRecentEvictsOldestAfterFullCircle(t *testing.T) {
	tr := newFakeTransport()
	ctx := context.Background()
	for i := 0; i < RecentMessagesSize; i++ {
		id := types.MessageID(fmt.Sprintf("MSG%d", i))
		if err := AddRecent(ctx, tr, testPeerJID, id, &waE2E.Message{}, nil); err != nil {
			t.Fatalf("AddRecent %d: %v", i, err)
		}
	}
	if n := len(tr.state.recentMap); n != RecentMessagesSize {
		t.Fatalf("mapa tem %d entradas, esperado %d", n, RecentMessagesSize)
	}
	if tr.state.recentPtr != 0 {
		t.Errorf("ponteiro = %d, esperado 0 apos a volta completa", tr.state.recentPtr)
	}
	if GetRecent(tr, testPeerJID, "MSG0").IsEmpty() {
		t.Fatal("MSG0 deveria estar viva ate' a proxima insercao")
	}

	if err := AddRecent(ctx, tr, testPeerJID, "EXTRA", &waE2E.Message{}, nil); err != nil {
		t.Fatalf("AddRecent extra: %v", err)
	}
	if !GetRecent(tr, testPeerJID, "MSG0").IsEmpty() {
		t.Error("MSG0 deveria ter sido despejada pela volta do buffer")
	}
	if GetRecent(tr, testPeerJID, "MSG1").IsEmpty() {
		t.Error("MSG1 nao deveria ter sido despejada ainda")
	}
	if n := len(tr.state.recentMap); n != RecentMessagesSize {
		t.Errorf("mapa tem %d entradas, esperado que ficasse em %d", n, RecentMessagesSize)
	}
}

// Sem UseMessageStore o store nao e' tocado, e o caminho fb tambem entra no
// cache em memoria.
func TestAddRecentWithoutStoreDoesNotTouchIt(t *testing.T) {
	tr := newFakeTransport()
	tr.useMessageStore = false
	err := AddRecent(context.Background(), tr, testPeerJID, "MSG1", nil, &waMsgApplication.MessageApplication{})
	if err != nil {
		t.Fatalf("AddRecent: %v", err)
	}
	if GetRecent(tr, testPeerJID, "MSG1").IsEmpty() {
		t.Error("mensagem fb deveria estar no cache")
	}
	if tr.stores.addOutgoingCall != 0 {
		t.Errorf("store foi tocado %d vezes, esperado 0", tr.stores.addOutgoingCall)
	}
}

// --- store persistente ---

func TestAddRecentWithStore(t *testing.T) {
	tr := newFakeTransport()
	tr.useMessageStore = true
	if err := AddRecent(context.Background(), tr, testPeerJID, "MSG1", waMessage("oi"), nil); err != nil {
		t.Fatalf("AddRecent: %v", err)
	}
	if tr.stores.outgoing["MSG1"][0] != StoreFormatWA {
		t.Errorf("formato gravado = %v", tr.stores.outgoing["MSG1"][0])
	}
	// F52: o throttle de StoreClearInterval e' codigo morto (lastStoreClear
	// nunca e' escrito), entao o expurgo roda em TODA gravacao. Travado aqui
	// para que uma correcao futura seja consciente.
	if tr.stores.deleteOldCalls != 1 {
		t.Errorf("expurgos = %d, esperado 1 (throttle morto, F52)", tr.stores.deleteOldCalls)
	}
	if err := AddRecent(context.Background(), tr, testPeerJID, "MSG2", waMessage("oi"), nil); err != nil {
		t.Fatalf("AddRecent 2: %v", err)
	}
	if tr.stores.deleteOldCalls != 2 {
		t.Errorf("expurgos = %d, esperado 2 (F52)", tr.stores.deleteOldCalls)
	}
}

func TestAddRecentStoreErrors(t *testing.T) {
	boom := errors.New("boom")
	for name, setup := range map[string]func(*fakeTransport){
		"falha ao gravar":  func(tr *fakeTransport) { tr.stores.addOutErr = boom },
		"falha no expurgo": func(tr *fakeTransport) { tr.stores.deleteOldErr = boom },
	} {
		t.Run(name, func(t *testing.T) {
			tr := newFakeTransport()
			tr.useMessageStore = true
			setup(tr)
			err := AddRecent(context.Background(), tr, testPeerJID, "MSG1", waMessage("oi"), nil)
			if !errors.Is(err, boom) {
				t.Fatalf("erro = %v, esperado embrulhar boom", err)
			}
			// Erro no store aborta ANTES do cache em memoria.
			if !GetRecent(tr, testPeerJID, "MSG1").IsEmpty() {
				t.Error("a mensagem nao deveria ter entrado no cache")
			}
		})
	}
}

// Com o store ligado mas sem payload nenhum (wa e fb nil), buf fica nil e o
// bloco do store inteiro e' pulado — a entrada vazia ainda entra no cache.
func TestAddRecentWithStoreAndNoPayload(t *testing.T) {
	tr := newFakeTransport()
	tr.useMessageStore = true
	if err := AddRecent(context.Background(), tr, testPeerJID, "MSG1", nil, nil); err != nil {
		t.Fatalf("AddRecent: %v", err)
	}
	if tr.stores.addOutgoingCall != 0 {
		t.Error("sem payload, o store nao deveria ser tocado")
	}
	if _, ok := tr.state.recentMap[RecentKey{testPeerJID, "MSG1"}]; !ok {
		t.Error("a entrada vazia deveria estar no cache")
	}
}

// --- GetForRetry ---

func TestGetForRetryFromCache(t *testing.T) {
	tr := newFakeTransport()
	msg := waMessage("oi")
	tr.state.AddRecent(RecentKey{testPeerJID, "MSG1"}, RecentMessage{WA: msg})
	got, err := GetForRetry(context.Background(), tr, dmReceipt("MSG1"), "MSG1")
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if got.WA != msg {
		t.Error("deveria ter achado no cache pelo JID do chat")
	}
}

// Segundo caminho: o cache tem a mensagem sob o OUTRO lado do par LID/PN.
func TestGetForRetryFromAlternateJID(t *testing.T) {
	for name, tc := range map[string]struct {
		chat, alt types.JID
		setup     func(*fakeLIDs)
	}{
		"chat e' PN, cache e' LID": {testPeerJID, testPeerLID, func(l *fakeLIDs) {
			l.pnToLID[testPeerJID] = testPeerLID
		}},
		"chat e' LID, cache e' PN": {testPeerLID, testPeerJID, func(l *fakeLIDs) {
			l.lidToPN[testPeerLID] = testPeerJID
		}},
	} {
		t.Run(name, func(t *testing.T) {
			tr := newFakeTransport()
			tc.setup(tr.lids)
			msg := waMessage("oi")
			tr.state.AddRecent(RecentKey{tc.alt, "MSG1"}, RecentMessage{WA: msg})
			receipt := dmReceipt("MSG1")
			receipt.Chat = tc.chat
			got, err := GetForRetry(context.Background(), tr, receipt, "MSG1")
			if err != nil {
				t.Fatalf("erro: %v", err)
			}
			if got == nil || got.WA != msg {
				t.Errorf("nao achou pelo JID alternativo: %+v", got)
			}
		})
	}
}

func TestGetForRetryAlternateJIDError(t *testing.T) {
	tr := newFakeTransport()
	tr.lids.err = errors.New("boom")
	got, err := GetForRetry(context.Background(), tr, dmReceipt("MSG1"), "MSG1")
	if err == nil {
		t.Fatal("esperado erro")
	}
	if got != nil {
		t.Error("nao deveria devolver mensagem com erro")
	}
}

// Terceiro caminho: o store persistente. E' TERMINAL — com UseMessageStore
// ligado, o callback GetMessageForRetry nao e' consultado nem quando o store
// nao acha nada.
func TestGetForRetryFromStoreIsTerminal(t *testing.T) {
	tr := newFakeTransport()
	tr.useMessageStore = true
	tr.messageForRetry = waMessage("do callback")

	buf, err := proto.Marshal(waMessage("do store"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	tr.stores.outgoing["MSG1"] = [2]any{StoreFormatWA, buf}
	got, err := GetForRetry(context.Background(), tr, dmReceipt("MSG1"), "MSG1")
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if got.WA.GetConversation() != "do store" {
		t.Errorf("Conversation = %q", got.WA.GetConversation())
	}

	// Nada no store: cai no default de ParseRecent (formato vazio) e vira erro,
	// sem consultar o callback.
	if _, err := GetForRetry(context.Background(), tr, dmReceipt("MSG9"), "MSG9"); err == nil {
		t.Error("esperado erro de formato vazio, nao fallback para o callback")
	}
}

func TestGetForRetryStoreError(t *testing.T) {
	tr := newFakeTransport()
	tr.useMessageStore = true
	tr.stores.getOutErr = errors.New("boom")
	if _, err := GetForRetry(context.Background(), tr, dmReceipt("MSG1"), "MSG1"); err == nil {
		t.Error("esperado erro do store")
	}
}

// Quarto caminho: o callback do usuario.
func TestGetForRetryFromCallback(t *testing.T) {
	tr := newFakeTransport()
	tr.messageForRetry = waMessage("do callback")
	got, err := GetForRetry(context.Background(), tr, dmReceipt("MSG1"), "MSG1")
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if got.WA.GetConversation() != "do callback" {
		t.Errorf("Conversation = %q", got.WA.GetConversation())
	}
}

// Nada em lugar nenhum: (nil, nil), que NAO e' erro.
func TestGetForRetryNotFound(t *testing.T) {
	tr := newFakeTransport()
	got, err := GetForRetry(context.Background(), tr, dmReceipt("MSG1"), "MSG1")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != nil {
		t.Errorf("esperado nil, veio %+v", got)
	}
}

// Servidor que nao e' nem PN nem LID (grupo, por exemplo) pula o passo do JID
// alternativo sem erro.
func TestGetForRetryUnknownServerSkipsAlternate(t *testing.T) {
	tr := newFakeTransport()
	receipt := dmReceipt("MSG1")
	receipt.Chat = testGroupJID
	if _, err := GetForRetry(context.Background(), tr, receipt, "MSG1"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
}

// Mensagem FB com o store ligado e' serializada no formato fb.
func TestAddRecentWithStoreFBFormat(t *testing.T) {
	tr := newFakeTransport()
	tr.useMessageStore = true
	err := AddRecent(context.Background(), tr, testPeerJID, "MSG1", nil, &waMsgApplication.MessageApplication{})
	if err != nil {
		t.Fatalf("AddRecent: %v", err)
	}
	if got := tr.stores.outgoing["MSG1"][0]; got != StoreFormatFB {
		t.Errorf("formato gravado = %v, esperado %v", got, StoreFormatFB)
	}
}
