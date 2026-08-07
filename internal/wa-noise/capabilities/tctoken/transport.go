// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package tctoken

import (
	"context"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	waLog "wa-api/internal/wa-noise/observability/log"
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
	Log() waLog.Logger
	// BackgroundCtx e' o Client.BackgroundEventCtx: o contexto que sobrevive ao
	// fim da requisicao, usado pela poda assincrona e pela emissao em goroutine.
	BackgroundCtx() context.Context
	// SendIQ envia um <iq> e espera a resposta.
	SendIQ(ctx context.Context, query IQ) (*waBinary.Node, error)
}
