// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package message

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/appstate"
	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waHistorySync"
	"wa-api/internal/wa-noise/protocol/proto/waWeb"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

func zlibBlob(t *testing.T, hs *waHistorySync.HistorySync) []byte {
	t.Helper()
	raw, err := proto.Marshal(hs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err = w.Write(raw); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err = w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes()
}

// --- DownloadHistorySync ---

// O payload inline dispensa o download: e' o caminho do bootstrap inicial.
func TestDownloadHistorySyncInlinePayloadSkipsDownload(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{})
	f.downloadErr = errors.New("nao devia baixar")
	blob := zlibBlob(t, &waHistorySync.HistorySync{
		SyncType: waHistorySync.HistorySync_INITIAL_BOOTSTRAP.Enum(),
		NctSalt:  []byte("salt"),
	})

	hs, err := DownloadHistorySync(context.Background(), f, &waE2E.HistorySyncNotification{
		InitialHistBootstrapInlinePayload: blob,
	}, true)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if hs.GetSyncType() != waHistorySync.HistorySync_INITIAL_BOOTSTRAP {
		t.Errorf("tipo = %s", hs.GetSyncType())
	}
	if string(f.nctSalt) != "salt" {
		t.Errorf("nctSalt = %q", f.nctSalt)
	}
}

func TestDownloadHistorySyncDownloadError(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("404")
	f.downloadErr = sentinel
	if _, err := DownloadHistorySync(context.Background(), f, &waE2E.HistorySyncNotification{}, true); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

// Blob que nao e' zlib, ou que descomprime em algo que nao e' o protobuf
// esperado, tem que virar erro e nao panico.
func TestDownloadHistorySyncMalformedBlob(t *testing.T) {
	f := newFakeTransport()
	f.downloadData = []byte("nao e' zlib")
	if _, err := DownloadHistorySync(context.Background(), f, &waE2E.HistorySyncNotification{}, true); err == nil {
		t.Fatal("esperava erro de descompressao")
	}
}

// A gravacao derivada roda inteira: salt, segredos das conversas e mapeamentos
// PN/LID.
func TestDownloadHistorySyncStoresDerivedData(t *testing.T) {
	stub := &stubMsgSecretStore{}
	f := newFakeTransport().withSecrets(stub)
	f.downloadData = zlibBlob(t, &waHistorySync.HistorySync{
		SyncType: waHistorySync.HistorySync_FULL.Enum(),
		Conversations: []*waHistorySync.Conversation{
			historyConv(testOtherJID.String(), historyMsg("MSG1", true, "", "", testSecret)),
		},
		PhoneNumberToLidMappings: []*waHistorySync.PhoneNumberToLIDMapping{{
			PnJID:  proto.String(testOtherJID.String()),
			LidJID: proto.String(types.NewJID("55443322", types.HiddenUserServer).String()),
		}},
		GlobalSettings: &waHistorySync.GlobalSettings{},
	})

	if _, err := DownloadHistorySync(context.Background(), f, &waE2E.HistorySyncNotification{}, true); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(stub.putMany) != 1 || stub.putMany[0].ID != "MSG1" {
		t.Fatalf("segredos gravados = %+v", stub.putMany)
	}
}

// --- fila e loop de history sync ---

// EnqueueHistorySync poe na fila e liga o loop; o loop baixa, despacha o evento
// e apaga a media do servidor.
func TestHistorySyncLoopConsumesQueue(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{})
	f.downloadData = zlibBlob(t, &waHistorySync.HistorySync{
		SyncType: waHistorySync.HistorySync_RECENT.Enum(),
	})

	EnqueueHistorySync(f, &waE2E.HistorySyncNotification{})

	if !waitFor(t, f, func() bool { return f.deletedMedia == 1 }) {
		t.Fatal("o loop nao consumiu a notificacao")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var found bool
	for _, e := range f.events {
		if _, ok := e.(*events.HistorySync); ok {
			found = true
		}
	}
	if !found {
		t.Fatalf("nenhum events.HistorySync despachado: %#v", f.events)
	}
}

// Falha de download nao para o loop nem apaga a media: o blob continua no
// servidor para uma proxima tentativa.
func TestHistorySyncLoopDownloadFailureKeepsMedia(t *testing.T) {
	f := newFakeTransport()
	f.downloadErr = errors.New("sem rede")

	EnqueueHistorySync(f, &waE2E.HistorySyncNotification{})
	// Espera o loop drenar a fila.
	if !waitFor(t, f, func() bool { return f.histSync.Len() == 0 }) {
		t.Fatal("a fila nao foi drenada")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deletedMedia != 0 {
		t.Fatalf("apagou a media apesar do download ter falhado")
	}
}

// --- SendHistorySyncServerErrorReceipt ---

func TestSendHistorySyncServerErrorReceiptShape(t *testing.T) {
	f := newFakeTransport()
	mediaKey := bytes.Repeat([]byte{1}, 32)
	if err := SendHistorySyncServerErrorReceipt(context.Background(), f, "MSG1", mediaKey); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(f.sentNodes) != 1 {
		t.Fatalf("%d nos", len(f.sentNodes))
	}
	node := f.sentNodes[0]
	if node.Tag != "receipt" || node.Attrs["type"] != "server-error" || node.Attrs["category"] != "peer" {
		t.Fatalf("no' = %+v", node)
	}
	children := node.GetChildren()
	if len(children) != 1 || children[0].Tag != "encrypt" {
		t.Fatalf("filhos = %+v", children)
	}
	if len(children[0].GetChildren()) != 2 {
		t.Fatalf("o <encrypt> tem que levar enc_p e enc_iv: %+v", children[0])
	}
}

// Sem JID proprio nao ha' destinatario para o recibo.
func TestSendHistorySyncServerErrorReceiptNotLoggedIn(t *testing.T) {
	f := newFakeTransport()
	f.ownID = types.EmptyJID
	err := SendHistorySyncServerErrorReceipt(context.Background(), f, "MSG1", bytes.Repeat([]byte{1}, 32))
	if !errors.Is(err, errNotLoggedIn) {
		t.Fatalf("err = %v", err)
	}
}

// --- HandleAppStateSyncKeyShare ---

// Toda chave recebida e' gravada e, ao fim, TODOS os patches sao buscados.
func TestHandleAppStateSyncKeyShareFetchesAllPatches(t *testing.T) {
	f := newFakeTransport()
	HandleAppStateSyncKeyShare(context.Background(), f, &waE2E.AppStateSyncKeyShare{
		Keys: []*waE2E.AppStateSyncKey{{
			KeyID:   &waE2E.AppStateSyncKeyId{KeyID: []byte{1, 2, 3}},
			KeyData: &waE2E.AppStateSyncKeyData{KeyData: []byte("chave")},
		}},
	})
	if len(f.fetchedAppStates) != len(appstate.AllPatchNames) {
		t.Fatalf("buscou %d patches, queria %d", len(f.fetchedAppStates), len(appstate.AllPatchNames))
	}
}

// Erro de busca de patch e' apenas logado: os outros patches continuam sendo
// buscados.
func TestHandleAppStateSyncKeyShareFetchErrorDoesNotStop(t *testing.T) {
	f := newFakeTransport()
	f.fetchAppStateErr = errors.New("boom")
	HandleAppStateSyncKeyShare(context.Background(), f, &waE2E.AppStateSyncKeyShare{})
	if len(f.fetchedAppStates) != len(appstate.AllPatchNames) {
		t.Fatalf("buscou %d patches", len(f.fetchedAppStates))
	}
}

// --- HandlePlaceholderResendResponse ---

func TestHandlePlaceholderResendResponse(t *testing.T) {
	f := newFakeTransport()
	f.webMsgEvent = &events.Message{}
	raw, err := proto.Marshal(&waWeb.WebMessageInfo{
		Key: &waCommon.MessageKey{ID: proto.String("MSG1")},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	ok := HandlePlaceholderResendResponse(f, &waE2E.PeerDataOperationRequestResponseMessage{
		StanzaID: proto.String("REQ1"),
		PeerDataOperationResult: []*waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult{
			// item sem resposta -> so' aviso
			{},
			// protobuf invalido -> so' aviso
			{PlaceholderMessageResendResponse: &waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult_PlaceholderMessageResendResponse{
				WebMessageInfoBytes: []byte{0xFF, 0xFF, 0xFF},
			}},
			// item bom
			{PlaceholderMessageResendResponse: &waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult_PlaceholderMessageResendResponse{
				WebMessageInfoBytes: raw,
			}},
		},
	})
	if !ok {
		t.Fatal("ok = false")
	}
	if len(f.events) != 1 {
		t.Fatalf("%d eventos, queria 1 (so' o item bom)", len(f.events))
	}
	if f.webMsgEvent.UnavailableRequestID != "REQ1" {
		t.Errorf("UnavailableRequestID = %q", f.webMsgEvent.UnavailableRequestID)
	}
}

// Handler que falha faz a funcao devolver false, que sobe ate'
// HandleProtocolMessage e impede o ack de sucesso.
func TestHandlePlaceholderResendResponseHandlerFailure(t *testing.T) {
	f := newFakeTransport()
	f.handlerFails = true
	raw, err := proto.Marshal(&waWeb.WebMessageInfo{
		Key: &waCommon.MessageKey{ID: proto.String("MSG1")},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	ok := HandlePlaceholderResendResponse(f, &waE2E.PeerDataOperationRequestResponseMessage{
		PeerDataOperationResult: []*waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult{
			{PlaceholderMessageResendResponse: &waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult_PlaceholderMessageResendResponse{
				WebMessageInfoBytes: raw,
			}},
		},
	})
	if ok {
		t.Fatal("ok = true apesar do handler ter falhado")
	}
}

func TestHandlePlaceholderResendResponseParseError(t *testing.T) {
	f := newFakeTransport()
	f.webMsgErr = errors.New("nao parseou")
	raw, err := proto.Marshal(&waWeb.WebMessageInfo{
		Key: &waCommon.MessageKey{ID: proto.String("MSG1")},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	ok := HandlePlaceholderResendResponse(f, &waE2E.PeerDataOperationRequestResponseMessage{
		PeerDataOperationResult: []*waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult{
			{PlaceholderMessageResendResponse: &waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult_PlaceholderMessageResendResponse{
				WebMessageInfoBytes: raw,
			}},
		},
	})
	if !ok {
		t.Fatal("ok = false: falha de parse e' so' aviso")
	}
	if len(f.events) != 0 {
		t.Fatalf("despachou %d eventos", len(f.events))
	}
}

// --- HandleSenderKeyDistributionMessage ---

// SKDM malformada e' logada e ignorada: nao pode derrubar o processamento da
// mensagem que a carregava.
func TestHandleSenderKeyDistributionMessageMalformed(t *testing.T) {
	f := newFakeTransport()
	HandleSenderKeyDistributionMessage(context.Background(), f, testGroupJID, testOtherJID, []byte("lixo"))
}
