package tctoken

import (
	"context"

	sdklog "wa-api/internal/noise/observability/log"
	"wa-api/internal/noise/persistence/store"
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
)

// IQType e' o atributo "type" de um <iq>. Espelha o infoQueryType da raiz.
type IQType string

// IQSet e' o unico tipo usado por este dominio.
const IQSet IQType = "set"

// IQ e' a fatia de infoQuery (pacote raiz) que este dominio usa. Mesmo racional
// dos lotes 2 e 3.
type IQ struct {
	Namespace string
	Type      IQType
	To        types.JID
	Content   any
}

// Transport e' a fatia do cliente de que o dominio de tctoken precisa.
type Transport interface {
	// Store e' o device store da sessao. Pode devolver nil: ResolveStudioLID e
	// ResolveStorageLID checam, como o codigo original checava `cli.Store == nil`.
	Store() *store.Device
	// State e' o estado mutavel deste dominio (cache de timestamps e os dois
	// locks). O ponteiro precisa ser estavel.
	State() *State
	// Log e' o logger do cliente.
	Log() sdklog.Logger
	// BackgroundCtx e' o Client.BackgroundEventCtx: o contexto que sobrevive ao
	// fim da requisicao, usado pela poda assincrona e pela emissao em goroutine.
	BackgroundCtx() context.Context
	// SendIQ envia um <iq> e espera a resposta.
	SendIQ(ctx context.Context, query IQ) (*waBinary.Node, error)
}
