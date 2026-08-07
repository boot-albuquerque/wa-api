// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/proto/waCommon"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/proto/waMsgApplication"
	"wa-api/internal/wa-noise/types"
)

// --- RecentMessage ---

func TestRecentMessageIsEmpty(t *testing.T) {
	if !(RecentMessage{}).IsEmpty() {
		t.Error("RecentMessage zerada deveria ser vazia")
	}
	if (RecentMessage{wa: &waE2E.Message{}}).IsEmpty() {
		t.Error("RecentMessage com payload wa nao e' vazia")
	}
	if (RecentMessage{fb: &waMsgApplication.MessageApplication{}}).IsEmpty() {
		t.Error("RecentMessage com payload fb nao e' vazia")
	}
}

// --- parseRecentMessage ---

func TestParseRecentMessageWAFormat(t *testing.T) {
	buf, err := proto.Marshal(&waE2E.Message{Conversation: proto.String("oi")})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rm, err := parseRecentMessage(retryStoreFormatWA, buf)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if rm.fb != nil {
		t.Error("formato wa nao deveria preencher fb")
	}
	if rm.wa.GetConversation() != "oi" {
		t.Errorf("Conversation = %q, esperado oi", rm.wa.GetConversation())
	}
}

func TestParseRecentMessageFBFormat(t *testing.T) {
	buf, err := proto.Marshal(&waMsgApplication.MessageApplication{
		Metadata: &waMsgApplication.MessageApplication_Metadata{
			FrankingKey: []byte("chave"),
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rm, err := parseRecentMessage(retryStoreFormatFB, buf)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if rm.wa != nil {
		t.Error("formato fb nao deveria preencher wa")
	}
	if string(rm.fb.GetMetadata().GetFrankingKey()) != "chave" {
		t.Errorf("FrankingKey = %q", rm.fb.GetMetadata().GetFrankingKey())
	}
}

func TestParseRecentMessageUnknownFormat(t *testing.T) {
	rm, err := parseRecentMessage("json", []byte("{}"))
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
func TestParseRecentMessageInvalidPayload(t *testing.T) {
	if _, err := parseRecentMessage(retryStoreFormatWA, []byte{0xFF, 0xFF, 0xFF}); err == nil {
		t.Error("esperado erro de unmarshal no formato wa")
	}
	if _, err := parseRecentMessage(retryStoreFormatFB, []byte{0xFF, 0xFF, 0xFF}); err == nil {
		t.Error("esperado erro de unmarshal no formato fb")
	}
}

// Formato vazio (o que GetOutgoingEvent devolve quando nao acha nada) cai no
// default e vira erro, nao mensagem vazia.
func TestParseRecentMessageEmptyFormat(t *testing.T) {
	if _, err := parseRecentMessage("", nil); err == nil {
		t.Error("esperado erro para formato vazio")
	}
}

// --- buffer circular de mensagens recentes ---

func recentMessagesTestClient() *Client {
	cli := notifTestClient()
	cli.recentMessagesMap = make(map[recentMessageKey]RecentMessage, recentMessagesSize)
	return cli
}

func TestAddAndGetRecentMessage(t *testing.T) {
	cli := recentMessagesTestClient()
	msg := &waE2E.Message{Conversation: proto.String("oi")}
	if err := cli.addRecentMessage(context.Background(), receiptTestPeerJID, "MSG1", msg, nil); err != nil {
		t.Fatalf("addRecentMessage: %v", err)
	}
	got := cli.getRecentMessage(receiptTestPeerJID, "MSG1")
	if got.wa != msg {
		t.Errorf("getRecentMessage devolveu %v, esperado a mesma mensagem", got.wa)
	}
	// Chave e' (destinatario, ID): trocar qualquer um dos dois nao acha.
	if !cli.getRecentMessage(receiptTestOwnJID, "MSG1").IsEmpty() {
		t.Error("JID diferente nao deveria bater")
	}
	if !cli.getRecentMessage(receiptTestPeerJID, "MSG2").IsEmpty() {
		t.Error("ID diferente nao deveria bater")
	}
}

// O buffer e' circular de recentMessagesSize posicoes: a entrada
// recentMessagesSize+1 sobrescreve a primeira, e a primeira tem que sair
// tambem do mapa — senao o mapa cresce sem limite.
func TestAddRecentMessageEvictsOldestAfterFullCircle(t *testing.T) {
	cli := recentMessagesTestClient()
	ctx := context.Background()
	for i := 0; i < recentMessagesSize; i++ {
		id := types.MessageID(fmt.Sprintf("MSG%d", i))
		if err := cli.addRecentMessage(ctx, receiptTestPeerJID, id, &waE2E.Message{}, nil); err != nil {
			t.Fatalf("addRecentMessage %d: %v", i, err)
		}
	}
	if len(cli.recentMessagesMap) != recentMessagesSize {
		t.Fatalf("mapa tem %d entradas, esperado %d", len(cli.recentMessagesMap), recentMessagesSize)
	}
	if cli.recentMessagesPtr != 0 {
		t.Errorf("ponteiro = %d, esperado 0 apos a volta completa", cli.recentMessagesPtr)
	}
	if cli.getRecentMessage(receiptTestPeerJID, "MSG0").IsEmpty() {
		t.Fatal("MSG0 deveria estar viva ate' a proxima insercao")
	}

	if err := cli.addRecentMessage(ctx, receiptTestPeerJID, "EXTRA", &waE2E.Message{}, nil); err != nil {
		t.Fatalf("addRecentMessage extra: %v", err)
	}
	if !cli.getRecentMessage(receiptTestPeerJID, "MSG0").IsEmpty() {
		t.Error("MSG0 deveria ter sido despejada pela volta do buffer")
	}
	if cli.getRecentMessage(receiptTestPeerJID, "MSG1").IsEmpty() {
		t.Error("MSG1 nao deveria ter sido despejada ainda")
	}
	if len(cli.recentMessagesMap) != recentMessagesSize {
		t.Errorf("mapa tem %d entradas, esperado que ficasse em %d", len(cli.recentMessagesMap), recentMessagesSize)
	}
}

// Sem UseRetryMessageStore, addRecentMessage nao toca no Store — o que este
// teste prova de fato e' que o caminho em memoria nao depende de banco, ja'
// que o Store do cliente de teste nao tem EventBuffer.
func TestAddRecentMessageWithoutStoreDoesNotTouchIt(t *testing.T) {
	cli := recentMessagesTestClient()
	cli.UseRetryMessageStore = false
	if err := cli.addRecentMessage(context.Background(), receiptTestPeerJID, "MSG1", nil, &waMsgApplication.MessageApplication{
		Payload: &waMsgApplication.MessageApplication_Payload{
			Content: &waMsgApplication.MessageApplication_Payload_SubProtocol{
				SubProtocol: &waMsgApplication.MessageApplication_SubProtocolPayload{
					FutureProof: waCommon.FutureProofBehavior_PLACEHOLDER.Enum(),
				},
			},
		},
	}); err != nil {
		t.Fatalf("addRecentMessage: %v", err)
	}
	if cli.getRecentMessage(receiptTestPeerJID, "MSG1").IsEmpty() {
		t.Error("mensagem fb deveria estar no cache")
	}
}

// --- pedido de mensagem ao celular ---

func phoneRerequestTestClient() *Client {
	cli := notifTestClient()
	cli.pendingPhoneRerequests = make(map[types.MessageID]context.CancelFunc)
	cli.BackgroundEventCtx = context.Background()
	cli.AutomaticMessageRerequestFromPhone = true
	return cli
}

// Com AutomaticMessageRerequestFromPhone desligado (o padrao) nada e' agendado
// — nem entrada no mapa de pendentes, nem espera.
func TestDelayedRequestMessageFromPhoneDisabled(t *testing.T) {
	cli := phoneRerequestTestClient()
	cli.AutomaticMessageRerequestFromPhone = false
	cli.delayedRequestMessageFromPhone(&types.MessageInfo{MessageSource: types.MessageSource{
		Chat: receiptTestPeerJID, Sender: receiptTestPeerJID,
	}, ID: "MSG1"})
	if len(cli.pendingPhoneRerequests) != 0 {
		t.Errorf("pendentes = %v, esperado vazio", cli.pendingPhoneRerequests)
	}
	// Cancelar algo que nunca foi agendado tambem nao pode explodir.
	cli.cancelDelayedRequestFromPhone("MSG1")
}

// O caminho que importa aqui e' o cancelamento: o pedido espera
// RequestFromPhoneDelay antes de ir a' rede, e cancelDelayedRequestFromPhone
// (chamado quando a mensagem finalmente chega) tem que interromper a espera
// antes disso. Sem o cancelamento, este teste levaria RequestFromPhoneDelay
// para terminar e ainda tentaria falar com o socket.
func TestCancelDelayedRequestFromPhoneInterruptsTheWait(t *testing.T) {
	cli := phoneRerequestTestClient()
	info := &types.MessageInfo{MessageSource: types.MessageSource{
		Chat: receiptTestPeerJID, Sender: receiptTestPeerJID,
	}, ID: "MSG1"}

	done := make(chan struct{})
	go func() {
		defer close(done)
		cli.delayedRequestMessageFromPhone(info)
	}()

	// Espera o agendamento aparecer no mapa antes de cancelar.
	deadline := time.Now().Add(2 * time.Second)
	for {
		cli.pendingPhoneRerequestsLock.RLock()
		_, scheduled := cli.pendingPhoneRerequests["MSG1"]
		cli.pendingPhoneRerequestsLock.RUnlock()
		if scheduled {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("o pedido nunca foi registrado como pendente")
		}
		time.Sleep(time.Millisecond)
	}

	cli.cancelDelayedRequestFromPhone("MSG1")
	select {
	case <-done:
	case <-time.After(RequestFromPhoneDelay):
		t.Fatal("o cancelamento nao interrompeu a espera")
	}
	if len(cli.pendingPhoneRerequests) != 0 {
		t.Errorf("pendentes = %v, esperado que o defer limpasse", cli.pendingPhoneRerequests)
	}
}

// clearDelayedMessageRequests cancela tudo de uma vez no disconnect.
func TestClearDelayedMessageRequests(t *testing.T) {
	cli := phoneRerequestTestClient()
	cancelled := make(chan string, 2)
	for _, id := range []types.MessageID{"MSG1", "MSG2"} {
		cli.pendingPhoneRerequests[id] = func() { cancelled <- string(id) }
	}
	cli.clearDelayedMessageRequests()
	if len(cancelled) != 2 {
		t.Errorf("cancelamentos = %d, esperado 2", len(cancelled))
	}
}

// --- coerencia das constantes de politica ---

func TestRetryPolicyConstantsAreCoherent(t *testing.T) {
	if minRetryCountForSessionRecreate < 1 {
		t.Errorf("minRetryCountForSessionRecreate = %d: abaixo de 1 recriaria sessao no primeiro retry",
			minRetryCountForSessionRecreate)
	}
	// Nos paramos de mandar recibo de retry em maxOutgoingRetryReceipts; o teto
	// do lado que *responde* precisa ser pelo menos tao grande, senao pararamos
	// de responder a retries que nos mesmos ainda estariamos pedindo.
	if maxIncomingRetryRequests < maxOutgoingRetryReceipts {
		t.Errorf("maxIncomingRetryRequests (%d) < maxOutgoingRetryReceipts (%d)",
			maxIncomingRetryRequests, maxOutgoingRetryReceipts)
	}
	if retryReceiptVersion != 1 {
		t.Errorf("retryReceiptVersion = %d: mudar a versao do <retry> muda o wire format",
			retryReceiptVersion)
	}
	if retryStoreFormatWA == retryStoreFormatFB {
		t.Error("os dois formatos do store de retry precisam ser distinguiveis")
	}
}
