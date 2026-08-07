package newsletter

import (
	"context"

	waLog "wa-api/internal/wa-noise/observability/log"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waWa6"
	"wa-api/internal/wa-noise/protocol/types"
)

// IQType e' o atributo "type" de um <iq>. Espelha o infoQueryType do pacote
// raiz sem depender dele.
type IQType string

const (
	IQSet IQType = "set"
	IQGet IQType = "get"
)

// IQ e' a fatia de infoQuery (pacote raiz) que o dominio de newsletter usa.
// Existe para que Transport nao precise expor o infoQuery da raiz, o que
// arrastaria a raiz para dentro deste pacote e refaria o ciclo de import. O
// adaptador da raiz traduz IQ -> infoQuery campo a campo; os campos que este
// dominio nunca preenche (Target, ID, SMaxID, Timeout, NoRetry) ficam no zero,
// exatamente como ficavam antes da extracao.
type IQ struct {
	Namespace string
	Type      IQType
	To        types.JID
	Content   any
}

// Transport e' a fatia do cliente de que o dominio de newsletter precisa.
//
// Deliberadamente nao expoe nada do *wa-noise.Client alem disso — e' o que
// permite que este pacote nao importe o pacote raiz e que os testes usem um
// duble em vez de um cliente real com socket e sessao Noise.
type Transport interface {
	// SendIQ manda um <iq> e espera a resposta.
	SendIQ(ctx context.Context, query IQ) (*waBinary.Node, error)
	// SendNode manda um no cru, sem esperar resposta.
	SendNode(ctx context.Context, node waBinary.Node) error
	// GenerateRequestID gera o id de uma requisicao com resposta assincrona.
	GenerateRequestID() string
	// WaitResponse registra o canal de resposta de reqID. Precisa ser chamado
	// ANTES do envio, senao a resposta pode chegar sem ouvinte.
	WaitResponse(reqID string) chan *waBinary.Node
	// CancelResponse desfaz o registro feito por WaitResponse.
	CancelResponse(reqID string, ch chan *waBinary.Node)
	// GenerateMessageID gera um ID de mensagem quando o chamador nao fornece.
	GenerateMessageID() types.MessageID
	// ParseMessages interpreta um no <messages> em eventos de newsletter. A
	// implementacao vive na raiz (notification_newsletter.go), junto do resto
	// do parsing de notificacao; expo-la aqui evita que este pacote precise
	// importar a raiz.
	ParseMessages(node *waBinary.Node) []*types.NewsletterMessage
	// ClientPayload devolve o payload de cliente da sessao, usado para decidir
	// entre as query IDs web e desktop do MEX.
	ClientPayload() *waWa6.ClientPayload
	// ElementMissing monta o erro de elemento XML ausente. O tipo concreto
	// (*wa-noise.ElementMissingError) e' generico do fork inteiro, nao do
	// dominio de newsletter, entao continua definido na raiz; a construcao
	// passa por aqui para preservar o tipo exato que os chamadores historicos
	// recebem em um type assert.
	ElementMissing(tag, in string) error
	// Log e' o logger do cliente.
	Log() waLog.Logger
}
