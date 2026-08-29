package core

import (
	"context"
	"strings"

	"wa-api/internal/noise/capabilities/group"
	sdklog "wa-api/internal/noise/observability/log"
	"wa-api/internal/noise/persistence/store"
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
)

// groupTransport adapta *Client a group.Transport. Existe para que o pacote
// internal/wa-noise/group possa operar sobre uma interface estreita sem
// importar o pacote raiz (o que fecharia um ciclo) e sem que *Client precise
// ganhar metodos exportados novos so' para satisfazer a interface.
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 6".
type groupTransport struct {
	cli *Client
}

var _ group.Transport = groupTransport{}

// groupT devolve o adaptador de grupo deste cliente.
func (cli *Client) groupT() group.Transport {
	return groupTransport{cli}
}

// SendIQ traduz group.IQ para o infoQuery da raiz. Os campos que este dominio
// nunca preenche (ID, SMaxID, Timeout, NoRetry) ficam no zero, exatamente como
// ficavam quando os nos eram montados na raiz.
func (t groupTransport) SendIQ(ctx context.Context, query group.IQ) (*waBinary.Node, error) {
	return t.cli.sendIQ(ctx, infoQuery{
		Namespace: query.Namespace,
		Type:      infoQueryType(query.Type),
		To:        query.To,
		Target:    query.Target,
		Content:   query.Content,
	})
}

func (t groupTransport) Store() *store.Device {
	return t.cli.Store
}

// Cache devolve o ponteiro para o cache de grupo do cliente. O ponteiro precisa
// ser estavel — groupCache e' campo de *Client e groupTransport embrulha o
// ponteiro do cliente, entao o mutex de dentro nunca e' copiado por valor.
func (t groupTransport) Cache() *group.Cache {
	return &t.cli.groupCache
}

func (t groupTransport) Log() sdklog.Logger {
	return t.cli.Log
}

func (t groupTransport) GenerateMessageID() types.MessageID {
	return t.cli.GenerateMessageID()
}

// TrimMessageIDPrefix mantem WebMessageIDPrefix em message_id.go, onde ele e'
// definido e testado; e' constante do dominio de ID de mensagem, nao do de
// grupo.
func (t groupTransport) TrimMessageIDPrefix(id types.MessageID) string {
	return strings.TrimPrefix(string(id), WebMessageIDPrefix)
}

// ElementMissing devolve o tipo concreto historico. ElementMissingError e' erro
// generico de parsing de XML do fork inteiro (usado por group, usync, appstate,
// pair-code, ...), nao do dominio de grupo, entao continua definido na raiz;
// so' a construcao atravessa a interface.
func (t groupTransport) ElementMissing(tag, in string) error {
	return &ElementMissingError{Tag: tag, In: in}
}

// WrapIQError preserva o *wrappedIQError historico, cujo Is() casa contra o
// erro humano e cujo Unwrap() devolve o erro de IQ.
func (t groupTransport) WrapIQError(human, iq error) error {
	return wrapIQError(human, iq)
}

// IQErrors entrega os MESMOS ponteiros de sentinela da raiz. Ver o doc de
// group.IQErrors para por que isso importa.
func (t groupTransport) IQErrors() group.IQErrors {
	return group.IQErrors{
		NotAuthorized: ErrIQNotAuthorized,
		Forbidden:     ErrIQForbidden,
		NotFound:      ErrIQNotFound,
		NotAcceptable: ErrIQNotAcceptable,
		Gone:          ErrIQGone,
	}
}
