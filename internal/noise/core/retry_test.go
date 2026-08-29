package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/noise/capabilities/retry"
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/protocol/types/events"
)

// A logica de retry vive em internal/wa-noise/retry (Fase F/G, lote 5) e e'
// testada la', com cobertura de 98,1%. O que a raiz precisa travar e' a
// tradução que o adaptador faz e as guardas de receptor nil.

// --- adaptador ---

func retryTestClient() *Client {
	cli := notifTestClient()
	cli.BackgroundEventCtx = context.Background()
	return cli
}

func TestRetryTransportTraduzConfiguracao(t *testing.T) {
	cli := retryTestClient()
	tr := cli.retryT()

	if tr.Store() != cli.Store {
		t.Error("Store nao e' o do cliente")
	}
	if tr.State() != &cli.retryState {
		t.Error("State tem que ser o ponteiro estavel do campo do cliente")
	}
	if tr.OwnLID() != receiptTestOwnLID {
		t.Errorf("OwnLID = %v", tr.OwnLID())
	}
	if tr.BackgroundCtx() != cli.BackgroundEventCtx {
		t.Error("BackgroundCtx nao e' o do cliente")
	}

	cli.UseRetryMessageStore = true
	cli.SynchronousAck = true
	if !tr.UseMessageStore() || !tr.SynchronousAck() {
		t.Error("as flags do cliente nao chegaram ao transporte")
	}

	// RerequestDelay le' a variavel publica a cada chamada — e' API ajustavel
	// em tempo de execucao, por isso nao virou constante do subpacote.
	original := RequestFromPhoneDelay
	defer func() { RequestFromPhoneDelay = original }()
	RequestFromPhoneDelay = 42 * time.Second
	if tr.RerequestDelay() != 42*time.Second {
		t.Errorf("RerequestDelay = %v, esperado 42s", tr.RerequestDelay())
	}
}

// RerequestFromPhoneEnabled junta as duas condicoes que o codigo original
// checava sempre em par.
func TestRetryTransportRerequestFromPhoneEnabled(t *testing.T) {
	for name, tc := range map[string]struct {
		automatic bool
		messenger bool
		want      bool
	}{
		"desligado":               {false, false, false},
		"ligado sem messenger":    {true, false, true},
		"ligado com messenger":    {true, true, false},
		"desligado com messenger": {false, true, false},
	} {
		t.Run(name, func(t *testing.T) {
			cli := retryTestClient()
			cli.AutomaticMessageRerequestFromPhone = tc.automatic
			if tc.messenger {
				cli.MessengerConfig = &MessengerConfig{}
			}
			if got := cli.retryT().RerequestFromPhoneEnabled(); got != tc.want {
				t.Errorf("= %v, esperado %v", got, tc.want)
			}
		})
	}
}

// A checagem de nil do PreRetryCallback vive no adaptador. Tabela-verdade
// identica a' do original: nil -> permite, true -> permite, false -> recusa.
func TestRetryTransportPreRetryAllowed(t *testing.T) {
	cli := retryTestClient()
	if !cli.retryT().PreRetryAllowed(nil, "M", 1, nil) {
		t.Error("sem callback deveria permitir")
	}
	for _, want := range []bool{true, false} {
		want := want
		cli.PreRetryCallback = func(*events.Receipt, types.MessageID, int, *waE2E.Message) bool { return want }
		if got := cli.retryT().PreRetryAllowed(nil, "M", 1, nil); got != want {
			t.Errorf("callback devolveu %v, adaptador devolveu %v", want, got)
		}
	}
}

// GetMessageForRetry tem default nao-nil em NewClient, mas um cliente montado
// a mao pode ter o campo nil; o adaptador protege.
func TestRetryTransportGetMessageForRetry(t *testing.T) {
	cli := retryTestClient()
	if cli.retryT().GetMessageForRetry(receiptTestPeerJID, receiptTestPeerJID, "M") != nil {
		t.Error("callback nil deveria devolver nil")
	}
	msg := &waE2E.Message{Conversation: proto.String("oi")}
	cli.GetMessageForRetry = func(_, _ types.JID, _ types.MessageID) *waE2E.Message { return msg }
	if cli.retryT().GetMessageForRetry(receiptTestPeerJID, receiptTestPeerJID, "M") != msg {
		t.Error("o callback nao foi consultado")
	}
}

func TestRetryTransportElementMissing(t *testing.T) {
	err := retryTestClient().retryT().ElementMissing("retry", "retry receipt")
	var missing *ElementMissingError
	if !errors.As(err, &missing) || missing.Tag != "retry" || missing.In != "retry receipt" {
		t.Errorf("erro = %v, esperado *ElementMissingError{retry, retry receipt}", err)
	}
}

// BuildBaseReceipt continua sendo o de receipt.go, que NAO foi extraido.
func TestRetryTransportBuildBaseReceipt(t *testing.T) {
	node := &waBinary.Node{Attrs: waBinary.Attrs{"from": receiptTestPeerJID}}
	attrs := retryTestClient().retryT().BuildBaseReceipt("M1", node)
	if attrs["id"] != "M1" || attrs["to"] != receiptTestPeerJID {
		t.Errorf("attrs = %v", attrs)
	}
}

// --- fachadas ---

func TestFachadaDeRetryDelegaAoSubpacote(t *testing.T) {
	cli := retryTestClient()
	ctx := context.Background()
	msg := &waE2E.Message{Conversation: proto.String("oi")}
	if err := cli.addRecentMessage(ctx, receiptTestPeerJID, "MSG1", msg, nil); err != nil {
		t.Fatalf("addRecentMessage: %v", err)
	}
	if got := cli.getRecentMessage(receiptTestPeerJID, "MSG1"); got.WA != msg {
		t.Errorf("getRecentMessage = %+v", got)
	}
	got, err := cli.getMessageForRetry(ctx, &events.Receipt{MessageSource: types.MessageSource{
		Chat: receiptTestPeerJID, Sender: receiptTestPeerJID,
	}}, "MSG1")
	if err != nil || got == nil || got.WA != msg {
		t.Errorf("getMessageForRetry = (%+v, %v)", got, err)
	}

	// parseRecentMessage e' funcao livre, sem receptor.
	buf, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if rm, err := parseRecentMessage(retryStoreFormatWA, buf); err != nil || rm.WA.GetConversation() != "oi" {
		t.Errorf("parseRecentMessage = (%+v, %v)", rm, err)
	}
}

// O estado de retry mora num campo so' de *Client, e o ponteiro tem de ser
// estavel: os cinco mutexes de dentro nunca podem ser copiados por valor.
func TestRetryStateSharedThroughTransport(t *testing.T) {
	cli := retryTestClient()
	cli.retryState.SetMaxParallel(2)
	if cli.retryT().State().Sema() == nil {
		t.Error("o transporte nao enxergou o estado do cliente")
	}
	cli.SetMaxParallelRetryReceiptHandling(0)
	if cli.retryState.Sema() != nil {
		t.Error("a fachada nao chegou ao estado")
	}
}

func TestFachadaDeRetryRecusaClientNil(t *testing.T) {
	var cli *Client
	ctx := context.Background()
	if _, recreate := cli.shouldRecreateSession(ctx, 5, receiptTestPeerJID); recreate {
		t.Error("cliente nil nao deveria mandar recriar sessao")
	}
	cli.tryHandleRetryReceipt(ctx, nil, nil)
	if err := cli.handleRetryReceipt(ctx, nil, nil); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("handleRetryReceipt = %v, esperado ErrClientIsNil", err)
	}
	if err := cli.addRecentMessage(ctx, receiptTestPeerJID, "M", nil, nil); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("addRecentMessage = %v, esperado ErrClientIsNil", err)
	}
	if got := cli.getRecentMessage(receiptTestPeerJID, "M"); !got.IsEmpty() {
		t.Errorf("getRecentMessage = %+v, esperado zero", got)
	}
	if _, err := cli.getMessageForRetry(ctx, nil, "M"); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("getMessageForRetry = %v, esperado ErrClientIsNil", err)
	}
	cli.sendRetryReceipt(ctx, nil, nil, false)
	cli.cancelDelayedRequestFromPhone("M")
	cli.delayedRequestMessageFromPhone(&types.MessageInfo{})
	cli.immediateRequestMessageFromPhone(ctx, &types.MessageInfo{})
	cli.clearDelayedMessageRequests()
	cli.SetMaxParallelRetryReceiptHandling(1)
}

// retryMessageRef copia exatamente os cinco campos que o subpacote usa.
func TestRetryMessageRef(t *testing.T) {
	info := &types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     receiptTestPeerJID,
			Sender:   receiptTestOwnJID,
			IsFromMe: true,
		},
		ID:   "MSG1",
		Type: "peer_msg",
	}
	want := retry.MessageRef{
		Chat: receiptTestPeerJID, Sender: receiptTestOwnJID, ID: "MSG1",
		Type: "peer_msg", IsFromMe: true,
	}
	if got := retryMessageRef(info); got != want {
		t.Errorf("= %+v, esperado %+v", got, want)
	}
}

// --- compatibilidade de API ---

// As constantes da raiz sao apelidos das do subpacote: os mesmos valores, nao
// copias que podem divergir.
func TestConstantesDeRetrySaoApelidos(t *testing.T) {
	for name, pair := range map[string][2]int{
		"maxIncomingRetryRequests":        {maxIncomingRetryRequests, retry.MaxIncomingRequests},
		"minRetryCountForSessionRecreate": {minRetryCountForSessionRecreate, retry.MinCountForSessionRecreate},
		"maxOutgoingRetryReceipts":        {maxOutgoingRetryReceipts, retry.MaxOutgoingReceipts},
		"retryReceiptVersion":             {retryReceiptVersion, retry.ReceiptVersion},
		"recentMessagesSize":              {recentMessagesSize, retry.RecentMessagesSize},
		"FBMessageApplicationVersion":     {FBMessageApplicationVersion, retry.FBApplicationVersion},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s = %d, esperado %d", name, pair[0], pair[1])
		}
	}
	if retryStoreFormatWA != retry.StoreFormatWA || retryStoreFormatFB != retry.StoreFormatFB {
		t.Error("os formatos do store divergiram do subpacote")
	}
	if retryStoreClearInterval != retry.StoreClearInterval {
		t.Error("o intervalo de expurgo divergiu do subpacote")
	}
}

// RecentMessage tem de ser APELIDO de tipo, e nao tipo novo: internals.go
// (gerado, F29) cita o nome na assinatura de dois metodos e precisa compilar
// sem ser tocado.
func TestRecentMessageEhApelidoDeTipo(t *testing.T) {
	var fromRoot RecentMessage
	var fromPkg retry.RecentMessage = fromRoot // so' compila se for apelido
	_ = fromPkg
	var key recentMessageKey = retry.RecentKey{To: receiptTestPeerJID, ID: "M"}
	if key.ID != "M" {
		t.Errorf("chave = %+v", key)
	}
	var incoming incomingRetryKey = retry.IncomingKey{JID: receiptTestPeerJID, MessageID: "M"}
	if incoming.MessageID != "M" {
		t.Errorf("chave de entrada = %+v", incoming)
	}
}
