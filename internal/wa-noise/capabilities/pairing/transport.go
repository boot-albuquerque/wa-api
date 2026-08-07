package pairing

import (
	"context"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// IQType e' o atributo "type" de um <iq>. Espelha o infoQueryType do pacote
// raiz sem depender dele.
type IQType string

// IQSet e' o unico tipo usado por este dominio: os dois estagios do pareamento
// por codigo sao `<iq type="set">`.
const IQSet IQType = "set"

// IQ e' a fatia de infoQuery (pacote raiz) que este dominio usa. Mesmo racional
// dos lotes 2 e 3: expor o infoQuery da raiz arrastaria a raiz para dentro
// deste pacote e refaria o ciclo de import. Os campos que este dominio nunca
// preenche (Target, ID, SMaxID, Timeout, NoRetry) ficam no zero.
type IQ struct {
	Namespace string
	Type      IQType
	To        types.JID
	Content   any
}

// Transport e' a fatia do cliente de que o pareamento precisa.
//
// Deliberadamente nao expoe nada do *whatsmeow.Client alem disso — e' o que
// permite que este pacote nao importe o pacote raiz e que os testes usem um
// duble em vez de um cliente real com socket e sessao Noise.
type Transport interface {
	// Store e' o device store da sessao. Aparece inteiro pelo mesmo racional
	// do lote 3: `store` ja' e' subpacote folha do fork.
	Store() *store.Device
	// State e' o estado mutavel deste dominio (a sessao de pareamento por
	// codigo pendente). O ponteiro precisa ser estavel.
	State() *State
	// Log e' o logger do cliente.
	Log() waLog.Logger

	// SendNode envia um no cru pelo socket.
	SendNode(ctx context.Context, node waBinary.Node) error
	// SendIQ envia um <iq> e espera a resposta.
	SendIQ(ctx context.Context, query IQ) (*waBinary.Node, error)
	// DispatchEvent entrega um evento aos handlers registrados.
	DispatchEvent(evt any)

	// ConfiguredClientType e' o Client.QRClientType configurado pelo usuario,
	// ou "" quando ele nao configurou nada (caso em que o tipo e' deduzido de
	// store.DeviceProps).
	ConfiguredClientType() ClientType
	// PrePairAllowed consulta o PrePairCallback do cliente. Devolve true
	// quando nao ha' callback configurado — a mesma semantica do
	// `cli.PrePairCallback != nil && !cli.PrePairCallback(...)` original.
	PrePairAllowed(jid types.JID, platform, businessName string) bool

	// StoreLIDPNMapping registra o par LID/PN do aparelho principal.
	StoreLIDPNMapping(ctx context.Context, lid, pn types.JID)
	// ExpectDisconnect avisa o cliente que a desconexao a seguir e' esperada
	// e nao deve virar evento de Disconnected.
	ExpectDisconnect()
	// Disconnect derruba a conexao.
	Disconnect()
	// SendUnifiedSession dispara o envio da unified session apos o pareamento.
	SendUnifiedSession()
	// SetServerTimeOffset registra a diferenca entre o relogio do servidor e o
	// local, em nanossegundos.
	SetServerTimeOffset(offset int64)

	// ElementMissing devolve o *whatsmeow.ElementMissingError historico. O
	// tipo e' erro generico de parsing de XML do fork inteiro (group, usync,
	// newsletter, ...), nao deste dominio, entao continua na raiz; so' a
	// construcao atravessa a interface. Mesmo racional dos lotes 1 a 3.
	ElementMissing(tag, in string) error
}
