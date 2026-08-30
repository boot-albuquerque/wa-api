package core

import (
	"context"

	"wa-api/internal/noise/capabilities/tctoken"
	sdklog "wa-api/internal/noise/observability/log"
	"wa-api/internal/noise/persistence/store"
	waBinary "wa-api/internal/noise/protocol/binary"
)

// tcTokenTransport adapta *Client a tctoken.Transport. Existe para que o pacote
// internal/noise/tctoken possa operar sobre uma interface estreita sem
// importar o pacote raiz (o que fecharia um ciclo).
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 4".
type tcTokenTransport struct {
	cli *Client
}

var _ tctoken.Transport = tcTokenTransport{}

// tcTokenT devolve o adaptador de tctoken deste cliente.
func (cli *Client) tcTokenT() tctoken.Transport {
	return tcTokenTransport{cli}
}

func (t tcTokenTransport) Store() *store.Device {
	return t.cli.Store
}

// State devolve o ponteiro para o estado de tctoken do cliente. O ponteiro
// precisa ser estavel — tcToken e' campo de *Client e tcTokenTransport embrulha
// o ponteiro do cliente, entao os dois mutexes de dentro nunca sao copiados por
// valor.
func (t tcTokenTransport) State() *tctoken.State {
	return &t.cli.tcToken
}

func (t tcTokenTransport) Log() sdklog.Logger {
	return t.cli.Log
}

func (t tcTokenTransport) BackgroundCtx() context.Context {
	return t.cli.BackgroundEventCtx
}

// SendIQ traduz tctoken.IQ para o infoQuery da raiz. Os campos que este dominio
// nunca preenche ficam no zero, como antes da extracao.
func (t tcTokenTransport) SendIQ(ctx context.Context, query tctoken.IQ) (*waBinary.Node, error) {
	return t.cli.sendIQ(ctx, infoQuery{
		Namespace: query.Namespace,
		Type:      infoQueryType(query.Type),
		To:        query.To,
		Content:   query.Content,
	})
}
