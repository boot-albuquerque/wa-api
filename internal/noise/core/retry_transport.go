package core

import (
	"context"
	"time"

	"go.mau.fi/libsignal/groups"
	"go.mau.fi/libsignal/keys/prekey"
	"go.mau.fi/libsignal/protocol"

	"wa-api/internal/noise/capabilities/prekeys"
	"wa-api/internal/noise/capabilities/retry"
	sdklog "wa-api/internal/noise/observability/log"
	"wa-api/internal/noise/persistence/store"
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/proto/waMsgTransport"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/protocol/types/events"
)

// retryTransport adapta *Client a retry.Transport. Existe para que o pacote
// internal/noise/retry possa operar sobre uma interface estreita sem
// importar o pacote raiz (o que fecharia um ciclo) e sem que *Client precise
// ganhar metodos exportados novos so' para satisfazer a interface.
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 5".
type retryTransport struct {
	cli *Client
}

var _ retry.Transport = retryTransport{}

// retryT devolve o adaptador de retry deste cliente.
func (cli *Client) retryT() retry.Transport {
	return retryTransport{cli}
}

func (t retryTransport) Store() *store.Device { return t.cli.Store }

// State devolve o ponteiro para o estado de retry do cliente. O ponteiro
// precisa ser estavel — retryState e' campo de *Client e retryTransport
// embrulha o ponteiro do cliente, entao os cinco mutexes de dentro nunca sao
// copiados por valor.
func (t retryTransport) State() *retry.State { return &t.cli.retryState }

func (t retryTransport) Log() sdklog.Logger { return t.cli.Log }

func (t retryTransport) SendNode(ctx context.Context, node waBinary.Node) error {
	return t.cli.sendNode(ctx, node)
}

func (t retryTransport) BackgroundCtx() context.Context { return t.cli.BackgroundEventCtx }

func (t retryTransport) OwnLID() types.JID { return t.cli.getOwnLID() }

func (t retryTransport) UseMessageStore() bool { return t.cli.UseRetryMessageStore }

func (t retryTransport) SynchronousAck() bool { return t.cli.SynchronousAck }

// RerequestFromPhoneEnabled junta as duas condicoes que o codigo original
// checava sempre em par, nos dois pontos de entrada do reenvio pelo telefone.
func (t retryTransport) RerequestFromPhoneEnabled() bool {
	return t.cli.AutomaticMessageRerequestFromPhone && t.cli.MessengerConfig == nil
}

// RerequestDelay le' a variavel publica a cada chamada, de proposito: ela e'
// API ajustavel em tempo de execucao.
func (t retryTransport) RerequestDelay() time.Duration { return RequestFromPhoneDelay }

// PreRetryAllowed absorve a checagem de nil do callback. Tabela-verdade
// identica a' do original: nil -> permite, true -> permite, false -> recusa.
func (t retryTransport) PreRetryAllowed(receipt *events.Receipt, id types.MessageID, retryCount int, msg *waE2E.Message) bool {
	if t.cli.PreRetryCallback == nil {
		return true
	}
	return t.cli.PreRetryCallback(receipt, id, retryCount, msg)
}

func (t retryTransport) GetMessageForRetry(requester, to types.JID, id types.MessageID) *waE2E.Message {
	if t.cli.GetMessageForRetry == nil {
		return nil
	}
	return t.cli.GetMessageForRetry(requester, to, id)
}

func (t retryTransport) FetchPreKeys(ctx context.Context, users []types.JID) (map[types.JID]prekeys.Resp, error) {
	return t.cli.fetchPreKeys(ctx, users)
}

func (t retryTransport) MigrateSessionStore(ctx context.Context, pn, lid types.JID) {
	t.cli.migrateSessionStore(ctx, pn, lid)
}

// CreateSKDM encapsula o groups.NewGroupSessionBuilder da raiz, que depende do
// pbSerializer (variavel de pacote da raiz) e por isso nao poderia atravessar
// a fronteira do subpacote como dado.
func (t retryTransport) CreateSKDM(ctx context.Context, chat types.JID) ([]byte, error) {
	builder := groups.NewGroupSessionBuilder(t.cli.Store, pbSerializer)
	senderKeyName := protocol.NewSenderKeyName(chat.String(), t.cli.getOwnLID().SignalAddress())
	signalSKDMessage, err := builder.Create(ctx, senderKeyName)
	if err != nil {
		return nil, err
	}
	return signalSKDMessage.Serialize(), nil
}

func (t retryTransport) EncryptForDevice(
	ctx context.Context,
	plaintext []byte,
	to types.JID,
	bundle *prekey.Bundle,
	extraAttrs waBinary.Attrs,
) (*waBinary.Node, bool, error) {
	return t.cli.encryptMessageForDevice(ctx, plaintext, to, bundle, extraAttrs, nil)
}

func (t retryTransport) EncryptForDeviceV3(
	ctx context.Context,
	payload *waMsgTransport.MessageTransport_Payload,
	skdm *waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage,
	dsm *waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage,
	to types.JID,
	bundle *prekey.Bundle,
	extraAttrs waBinary.Attrs,
) (*waBinary.Node, error) {
	return t.cli.encryptMessageForDeviceV3(ctx, payload, skdm, dsm, to, bundle, extraAttrs)
}

// MessageContent passa o zero de nodeExtraParams, que e' exatamente o que o
// call site de retry passava antes da extracao.
func (t retryTransport) MessageContent(
	baseNode waBinary.Node,
	message *waE2E.Message,
	msgAttrs waBinary.Attrs,
	includeIdentity bool,
) ([]waBinary.Node, error) {
	return t.cli.getMessageContent(baseNode, message, msgAttrs, includeIdentity, nodeExtraParams{})
}

func (t retryTransport) BuildBaseReceipt(id string, node *waBinary.Node) waBinary.Attrs {
	return buildBaseReceipt(id, node)
}

func (t retryTransport) RequestUnavailableMessage(ctx context.Context, chat, sender types.JID, id types.MessageID) error {
	_, err := t.cli.SendPeerMessage(ctx, t.cli.BuildUnavailableMessageRequest(chat, sender, id))
	return err
}

func (t retryTransport) ElementMissing(tag, in string) error {
	return &ElementMissingError{Tag: tag, In: in}
}
