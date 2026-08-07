// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package appstatesync

import (
	"context"

	"wa-api/internal/wa-noise/appstate"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waServerSync"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/protocol/types"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// IQType e' o atributo "type" de um <iq>. Espelha o infoQueryType do pacote
// raiz sem depender dele.
type IQType string

// IQSet e' o unico tipo usado por este dominio: tanto o fetch quanto o send de
// patches e o MarkNotDirty sao `<iq type="set">`.
const IQSet IQType = "set"

// IQ e' a fatia de infoQuery (pacote raiz) que este dominio usa. Existe pelo
// mesmo motivo que newsletter.IQ (lote 2): expor o infoQuery da raiz na
// interface arrastaria a raiz para dentro deste pacote e refaria o ciclo de
// import. O adaptador da raiz traduz IQ -> infoQuery campo a campo; os campos
// que este dominio nunca preenche (Target, ID, SMaxID, Timeout, NoRetry) ficam
// no zero, exatamente como ficavam antes da extracao.
type IQ struct {
	Namespace string
	Type      IQType
	To        types.JID
	Content   any
}

// Transport e' a fatia do cliente de que a sincronizacao de app state precisa.
//
// Deliberadamente nao expoe nada do *whatsmeow.Client alem disso — e' o que
// permite que este pacote nao importe o pacote raiz e que os testes usem um
// duble em vez de um cliente real com socket e sessao Noise.
type Transport interface {
	// Store e' o device store da sessao. Aparece inteiro (e nao fatiado em um
	// metodo por sub-store) porque `store` ja' e' um subpacote folha do fork:
	// expo-lo nao cria dependencia nova nem ciclo, e fatia-lo custaria oito
	// metodos de interface sem ganhar isolamento algum.
	Store() *store.Device
	// Proc e' o processador de patches (decodificacao, hash chain, chaves).
	// Mesmo racional de Store: `appstate` e' subpacote, nao a raiz.
	Proc() *appstate.Processor
	// State e' o estado mutavel deste dominio (lock de sync e historico de
	// pedidos de chave). Vive no cliente para durar entre chamadas; o ponteiro
	// precisa ser estavel (o mutex nunca pode ser copiado por valor).
	State() *State
	// Log e' o logger do cliente.
	Log() waLog.Logger
	// DispatchEvent entrega um evento aos handlers registrados. Devolve true se
	// algum handler falhou.
	DispatchEvent(evt any) (handlerFailed bool)
	// SendIQ manda um <iq> e espera a resposta.
	SendIQ(ctx context.Context, query IQ) (*waBinary.Node, error)
	// SendPeerMessage manda uma mensagem para o proprio dono da conta. So' o
	// erro interessa a este dominio, entao a SendResponse da raiz nao aparece
	// aqui (seria mais um tipo da raiz na interface).
	SendPeerMessage(ctx context.Context, msg *waE2E.Message) error
	// DownloadExternalBlob baixa o blob externo de um snapshot/patch grande
	// demais para vir inline.
	DownloadExternalBlob(ctx context.Context, ref *waServerSync.ExternalBlobReference) ([]byte, error)
	// EmitEventsOnFullSync espelha Client.EmitAppStateEventsOnFullSync.
	EmitEventsOnFullSync() bool
	// DebugLogs espelha Client.AppStateDebugLogs.
	DebugLogs() bool
	// StoreNCTSalt persiste o salt de NCT vindo de uma mutacao.
	StoreNCTSalt(ctx context.Context, salt []byte) error
	// ClearNCTSalt apaga o salt de NCT.
	ClearNCTSalt(ctx context.Context) error
	// ElementMissing monta o erro de elemento XML ausente. O tipo concreto
	// (*whatsmeow.ElementMissingError) e' generico do fork inteiro, nao deste
	// dominio, entao continua definido na raiz; so' a construcao passa por
	// aqui, para preservar o tipo exato que os chamadores historicos recebem em
	// um type assert. Mesma decisao do lote 2.
	ElementMissing(tag, in string) error
}
