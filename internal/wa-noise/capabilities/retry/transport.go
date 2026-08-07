package retry

import (
	"context"
	"time"

	"go.mau.fi/libsignal/keys/prekey"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/capabilities/prekeys"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waMsgTransport"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// Transport e' a fatia do cliente de que o dominio de retry precisa.
//
// E' o transporte mais largo dos cinco lotes, e o motivo e' o dominio: atender
// um recibo de retry significa reconstruir e RECIFRAR uma mensagem ja' enviada,
// entao o caminho toca sessao Signal, prekeys, montagem de no, envio e o
// caminho de reenvio pelo telefone. Cada metodo abaixo corresponde a uma
// chamada que o codigo fazia em *Client antes da extracao — nenhum foi
// inventado para conveniencia.
//
// Deliberadamente nao expoe nada do *whatsmeow.Client alem disso: e' o que
// permite que este pacote nao importe o pacote raiz (o que fecharia um ciclo) e
// que os testes usem um duble em vez de um cliente com socket e sessao Noise.
type Transport interface {
	// Store e' o device store da sessao. Aparece inteiro pelo mesmo racional
	// dos lotes 3 e 4: `store` ja' e' subpacote folha do fork, expo-lo nao cria
	// dependencia nova nem ciclo.
	Store() *store.Device
	// State e' o estado mutavel deste dominio. Vive no cliente para durar entre
	// chamadas; o ponteiro precisa ser estavel (os cinco mutexes de dentro
	// nunca podem ser copiados por valor).
	State() *State
	// Log e' o logger do cliente.
	Log() waLog.Logger
	// SendNode envia um no pelo socket.
	SendNode(ctx context.Context, node waBinary.Node) error
	// BackgroundCtx e' o contexto que sobrevive ao fim da requisicao
	// (Client.BackgroundEventCtx). O pedido adiado ao telefone deriva dele.
	BackgroundCtx() context.Context
	// OwnLID e' o LID deste dispositivo, usado como endereco Signal ao criar a
	// mensagem de distribuicao de chave de grupo.
	OwnLID() types.JID

	// --- configuracao do cliente ---

	// UseMessageStore espelha Client.UseRetryMessageStore.
	UseMessageStore() bool
	// SynchronousAck espelha Client.SynchronousAck.
	SynchronousAck() bool
	// RerequestFromPhoneEnabled combina Client.AutomaticMessageRerequestFromPhone
	// com a ausencia de MessengerConfig — as duas condicoes que o codigo
	// original checava juntas, sempre, nos dois pontos de entrada.
	RerequestFromPhoneEnabled() bool
	// RerequestDelay entrega o valor atual da variavel publica
	// whatsmeow.RequestFromPhoneDelay.
	//
	// Passa pela interface, e nao virou constante daqui, justamente porque a
	// variavel e' API publica ajustavel em tempo de execucao: le-la a cada
	// chamada preserva esse contrato.
	RerequestDelay() time.Duration

	// --- callbacks do usuario ---

	// PreRetryAllowed consulta Client.PreRetryCallback. Devolve true quando nao
	// ha callback registrado — a checagem de nil fica no adaptador, e a
	// tabela-verdade e' identica a' do original (nil -> permite, true ->
	// permite, false -> recusa).
	PreRetryAllowed(receipt *events.Receipt, id types.MessageID, retryCount int, msg *waE2E.Message) bool
	// GetMessageForRetry consulta o callback homonimo do cliente, usado quando
	// a mensagem nao esta' nem no cache nem no store.
	GetMessageForRetry(requester, to types.JID, id types.MessageID) *waE2E.Message

	// --- criptografia e montagem de no ---

	// FetchPreKeys busca bundles de prekey dos dispositivos dados.
	FetchPreKeys(ctx context.Context, users []types.JID) (map[types.JID]prekeys.Resp, error)
	// MigrateSessionStore move a sessao Signal de um JID de telefone para o LID.
	MigrateSessionStore(ctx context.Context, pn, lid types.JID)
	// CreateSKDM cria a mensagem de distribuicao de chave de remetente do grupo
	// dado, serializada. Encapsula o groups.NewGroupSessionBuilder da raiz, que
	// depende do pbSerializer de la'.
	CreateSKDM(ctx context.Context, chat types.JID) ([]byte, error)
	// EncryptForDevice cifra um payload E2E para um dispositivo.
	EncryptForDevice(
		ctx context.Context,
		plaintext []byte,
		to types.JID,
		bundle *prekey.Bundle,
		extraAttrs waBinary.Attrs,
	) (*waBinary.Node, bool, error)
	// EncryptForDeviceV3 cifra um payload de transporte FB para um dispositivo.
	EncryptForDeviceV3(
		ctx context.Context,
		payload *waMsgTransport.MessageTransport_Payload,
		skdm *waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage,
		dsm *waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage,
		to types.JID,
		bundle *prekey.Bundle,
		extraAttrs waBinary.Attrs,
	) (*waBinary.Node, error)
	// MessageContent monta o conteudo do <message> de retry. O nodeExtraParams
	// da raiz nao aparece aqui porque o call site sempre passava o zero dele.
	MessageContent(
		baseNode waBinary.Node,
		message *waE2E.Message,
		msgAttrs waBinary.Attrs,
		includeIdentity bool,
	) []waBinary.Node
	// BuildBaseReceipt monta os atributos comuns de um <receipt> de resposta.
	// Continua vivendo em receipt.go na raiz, que nao foi extraido — ver
	// PATCHES.md, lote 5.
	BuildBaseReceipt(id string, node *waBinary.Node) waBinary.Attrs
	// RequestUnavailableMessage pede ao proprio telefone o reenvio de uma
	// mensagem que nao conseguimos decifrar.
	RequestUnavailableMessage(ctx context.Context, chat, sender types.JID, id types.MessageID) error
	// ElementMissing constroi o erro de elemento XML ausente do fork. O tipo e'
	// generico do pacote raiz e continua la'; so' a construcao atravessa a
	// interface, mesmo racional dos lotes 1 a 4.
	ElementMissing(tag, in string) error
}
