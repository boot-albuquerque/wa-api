package message

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/noise/capabilities/send"
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/protocol/types/events"
)

func fromMeInfo() *types.MessageInfo {
	return &types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat: testGroupJID, Sender: testOwnJID, IsFromMe: true, IsGroup: true,
		},
		ID: "MSG1",
	}
}

// waitFor espera ate' `cond` virar true, com teto. Os handlers de protocol
// message disparam goroutines de PRODUCAO (recibos, app state key share), entao
// o teste precisa esperar por elas — nao e' artefato do duble.
func waitFor(t *testing.T, f *fakeTransport, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		ok := cond()
		f.mu.Unlock()
		if ok {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

// --- HandleProtocolMessage ---

// Protocol message de OUTRO usuario e' ignorado inteiro: history sync, migracao
// LID e app state key share so' podem vir do proprio dispositivo primario.
func TestHandleProtocolMessageIgnoresNotFromMe(t *testing.T) {
	f := newFakeTransport()
	info := fromMeInfo()
	info.IsFromMe = false
	msg := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		HistorySyncNotification: &waE2E.HistorySyncNotification{},
	}}

	if ok := HandleProtocolMessage(context.Background(), f, info, msg); !ok {
		t.Fatalf("ok = false")
	}
	if f.histSync.Len() != 0 {
		t.Fatalf("enfileirou history sync de mensagem alheia")
	}
}

// A notificacao de history sync entra na fila e o recibo sai. Com
// ManualHistorySyncDownload, a fila NAO e' alimentada (quem baixa e' o
// chamador), mas o recibo continua saindo — a menos que
// DisableManualHistorySyncReceipt tambem esteja ligado.
func TestHandleProtocolMessageHistorySyncModes(t *testing.T) {
	tests := []struct {
		name         string
		manual       bool
		disableRcpt  bool
		wantQueued   int
		wantReceipts int
	}{
		{"automatico", false, false, 1, 1},
		{"manual com recibo", true, false, 0, 1},
		{"manual sem recibo", true, true, 0, 0},
		// DisableManualHistorySyncReceipt sozinho nao desliga nada: a condicao
		// e' a conjuncao dos dois.
		{"disable sem manual", false, true, 1, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeTransport()
			f.manualHistorySync = tc.manual
			f.disableManualRcpt = tc.disableRcpt
			// Impede que o loop de download consuma a fila e apague a evidencia.
			f.downloadErr = errors.New("sem rede")

			msg := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
				HistorySyncNotification: &waE2E.HistorySyncNotification{},
			}}
			HandleProtocolMessage(context.Background(), f, fromMeInfo(), msg)

			if tc.wantReceipts > 0 && !waitFor(t, f, func() bool { return len(f.sentNodes) >= tc.wantReceipts }) {
				t.Fatalf("recibo de protocol message nao saiu")
			}
			if tc.wantReceipts == 0 {
				time.Sleep(50 * time.Millisecond)
				f.mu.Lock()
				n := len(f.sentNodes)
				f.mu.Unlock()
				if n != 0 {
					t.Fatalf("mandou %d recibos, queria nenhum", n)
				}
			}
			if tc.wantQueued == 0 && f.histSync.handlerActive.Load() {
				t.Errorf("ligou o loop de history sync em modo manual")
			}
		})
	}
}

// Categoria "peer" gera um recibo proprio, independente do resto.
func TestHandleProtocolMessagePeerCategoryReceipt(t *testing.T) {
	f := newFakeTransport()
	info := fromMeInfo()
	info.Category = send.MsgCategoryPeer
	msg := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{}}

	HandleProtocolMessage(context.Background(), f, info, msg)
	if !waitFor(t, f, func() bool { return len(f.sentNodes) == 1 }) {
		t.Fatal("recibo de peer nao saiu")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	node := f.sentNodes[0]
	if node.Tag != "receipt" || node.Attrs["type"] != string(types.ReceiptTypePeerMsg) {
		t.Fatalf("no' = %+v", node)
	}
}

// A resposta de placeholder resend e a de recuperacao de app state so' sao
// lidas quando vem do dispositivo PRIMARIO (Device == 0). De um companion,
// seriam eco de uma requisicao que nao fizemos.
func TestHandleProtocolMessagePeerDataOnlyFromPrimaryDevice(t *testing.T) {
	f := newFakeTransport()
	f.appStateRecoveryRet = false
	info := fromMeInfo()
	info.Sender.Device = 3
	msg := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		PeerDataOperationRequestResponseMessage: &waE2E.PeerDataOperationRequestResponseMessage{
			PeerDataOperationRequestType: waE2E.PeerDataOperationRequestType_COMPANION_SYNCD_SNAPSHOT_FATAL_RECOVERY.Enum(),
		},
	}}
	if ok := HandleProtocolMessage(context.Background(), f, info, msg); !ok {
		t.Fatal("ok = false: entrou no ramo de peer data vindo de companion")
	}
}

func TestHandleProtocolMessageAppStateRecoveryFailurePropagates(t *testing.T) {
	f := newFakeTransport()
	f.appStateRecoveryRet = false
	msg := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		PeerDataOperationRequestResponseMessage: &waE2E.PeerDataOperationRequestResponseMessage{
			PeerDataOperationRequestType: waE2E.PeerDataOperationRequestType_COMPANION_SYNCD_SNAPSHOT_FATAL_RECOVERY.Enum(),
		},
	}}
	if ok := HandleProtocolMessage(context.Background(), f, fromMeInfo(), msg); ok {
		t.Fatal("ok = true apesar da recuperacao de app state ter falhado")
	}
}

// --- ProcessProtocolParts ---

// A sender key so' e' processada em GRUPO; em DM e' aviso e nada mais.
func TestProcessProtocolPartsSenderKeyOnlyInGroup(t *testing.T) {
	f := newFakeTransport()
	info := fromMeInfo()
	info.IsGroup = false
	msg := &waE2E.Message{SenderKeyDistributionMessage: &waE2E.SenderKeyDistributionMessage{
		AxolotlSenderKeyDistributionMessage: []byte("nao e' uma SKDM"),
	}}
	if ok := ProcessProtocolParts(context.Background(), f, info, msg); !ok {
		t.Fatal("ok = false")
	}
}

// O DeviceSentMessage e' desembrulhado ANTES de procurar sender key e protocol
// message: o conteudo de verdade esta' dentro dele.
func TestProcessProtocolPartsUnwrapsDeviceSentMessage(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{})
	info := fromMeInfo()
	info.Category = send.MsgCategoryPeer
	msg := &waE2E.Message{DeviceSentMessage: &waE2E.DeviceSentMessage{
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{}},
	}}

	if ok := ProcessProtocolParts(context.Background(), f, info, msg); !ok {
		t.Fatal("ok = false")
	}
	if !waitFor(t, f, func() bool { return len(f.sentNodes) == 1 }) {
		t.Fatal("o protocol message de dentro do DeviceSentMessage nao foi tratado")
	}
}

// O segredo de mensagem e' gravado a partir do envelope EXTERNO, antes do
// desembrulho — e' de la' que ele vem.
func TestProcessProtocolPartsStoresSecretFromOuterMessage(t *testing.T) {
	stub := &stubMsgSecretStore{}
	f := newFakeTransport().withSecrets(stub)
	msg := &waE2E.Message{
		MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: testSecret},
		DeviceSentMessage:  &waE2E.DeviceSentMessage{Message: &waE2E.Message{}},
	}
	ProcessProtocolParts(context.Background(), f, fromMeInfo(), msg)
	if string(stub.putSecret) != string(testSecret) {
		t.Fatalf("segredo gravado = %X", stub.putSecret)
	}
}

// --- HandleDecrypted ---

func TestHandleDecryptedDispatchesMessage(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{})
	msg := &waE2E.Message{Conversation: proto.String("oi")}
	if failed := HandleDecrypted(context.Background(), f, fromMeInfo(), msg, 3); failed {
		t.Fatal("handlerFailed = true")
	}
	if len(f.events) != 1 {
		t.Fatalf("%d eventos", len(f.events))
	}
	evt := f.events[0].(*events.Message)
	if evt.RetryCount != 3 || evt.Message.GetConversation() != "oi" {
		t.Fatalf("evento = %+v", evt)
	}
}

// Handler que falha propaga o `true` para o chamador, que para o laco de <enc>.
func TestHandleDecryptedPropagatesHandlerFailure(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{})
	f.handlerFails = true
	if failed := HandleDecrypted(context.Background(), f, fromMeInfo(), &waE2E.Message{}, 0); !failed {
		t.Fatal("handlerFailed = false")
	}
}

// --- SendProtocolReceipt ---

// ID vazio e' no-op: nao ha' o que acusar, e mandar um <receipt> sem id seria
// rejeitado pelo servidor.
func TestSendProtocolReceiptEmptyIDIsNoop(t *testing.T) {
	f := newFakeTransport()
	if err := SendProtocolReceipt(context.Background(), f, "", types.ReceiptTypePeerMsg); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(f.sentNodes) != 0 {
		t.Fatalf("mandou %d nos para id vazio", len(f.sentNodes))
	}
}

func TestSendProtocolReceiptShape(t *testing.T) {
	f := newFakeTransport()
	if err := SendProtocolReceipt(context.Background(), f, "MSG1", types.ReceiptTypeHistorySync); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(f.sentNodes) != 1 {
		t.Fatalf("%d nos", len(f.sentNodes))
	}
	node := f.sentNodes[0]
	if node.Tag != "receipt" || node.Attrs["id"] != "MSG1" ||
		node.Attrs["type"] != string(types.ReceiptTypeHistorySync) {
		t.Fatalf("no' = %+v", node)
	}
	// O destinatario e' o proprio JID sem device: o recibo vai para o telefone.
	if node.Attrs["to"] != testOwnJID.ToNonAD() {
		t.Errorf("to = %v", node.Attrs["to"])
	}
	if node.Content != nil {
		t.Errorf("Content = %v, queria nil", node.Content)
	}
}

func TestSendProtocolReceiptPropagatesSendError(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("socket fechado")
	f.sendErr = sentinel
	if err := SendProtocolReceipt(context.Background(), f, "MSG1", types.ReceiptTypePeerMsg); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
	_ = waBinary.Node{}
}
