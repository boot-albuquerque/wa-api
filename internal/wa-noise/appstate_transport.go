// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	"wa-api/internal/wa-noise/protocol/appstate"
	"wa-api/internal/wa-noise/capabilities/appstatesync"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waServerSync"
	"wa-api/internal/wa-noise/persistence/store"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// appStateTransport adapta *Client a appstatesync.Transport. Existe para que o
// pacote internal/wa-noise/appstatesync possa operar sobre uma interface
// estreita sem importar o pacote raiz (o que fecharia um ciclo) e sem que
// *Client precise ganhar metodos exportados novos so' para satisfazer a
// interface.
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 3".
type appStateTransport struct {
	cli *Client
}

var _ appstatesync.Transport = appStateTransport{}

// appStateT devolve o adaptador de app state deste cliente.
func (cli *Client) appStateT() appstatesync.Transport {
	return appStateTransport{cli}
}

func (t appStateTransport) Store() *store.Device {
	return t.cli.Store
}

func (t appStateTransport) Proc() *appstate.Processor {
	return t.cli.appStateProc
}

// State devolve o ponteiro para o estado de app state do cliente. O ponteiro
// precisa ser estavel — appStateSync e' campo de *Client e appStateTransport
// embrulha o ponteiro do cliente, entao os mutexes de dentro nunca sao copiados
// por valor.
func (t appStateTransport) State() *appstatesync.State {
	return &t.cli.appStateSync
}

func (t appStateTransport) Log() waLog.Logger {
	return t.cli.Log
}

func (t appStateTransport) DispatchEvent(evt any) bool {
	return t.cli.dispatchEvent(evt)
}

// SendIQ traduz appstatesync.IQ para o infoQuery da raiz. Os campos que este
// dominio nunca preenche (Target, ID, SMaxID, Timeout, NoRetry) ficam no zero,
// exatamente como ficavam quando os nos eram montados na raiz.
func (t appStateTransport) SendIQ(ctx context.Context, query appstatesync.IQ) (*waBinary.Node, error) {
	return t.cli.sendIQ(ctx, infoQuery{
		Namespace: query.Namespace,
		Type:      infoQueryType(query.Type),
		To:        query.To,
		Content:   query.Content,
	})
}

// SendPeerMessage descarta a SendResponse: o dominio de app state so' usa o
// erro, e expor SendResponse (tipo da raiz) na interface arrastaria a raiz para
// dentro do subpacote.
func (t appStateTransport) SendPeerMessage(ctx context.Context, msg *waE2E.Message) error {
	_, err := t.cli.SendPeerMessage(ctx, msg)
	return err
}

func (t appStateTransport) DownloadExternalBlob(ctx context.Context, ref *waServerSync.ExternalBlobReference) ([]byte, error) {
	return t.cli.downloadExternalAppStateBlob(ctx, ref)
}

func (t appStateTransport) EmitEventsOnFullSync() bool {
	return t.cli.EmitAppStateEventsOnFullSync
}

func (t appStateTransport) DebugLogs() bool {
	return t.cli.AppStateDebugLogs
}

func (t appStateTransport) StoreNCTSalt(ctx context.Context, salt []byte) error {
	return t.cli.storeNCTSalt(ctx, salt)
}

func (t appStateTransport) ClearNCTSalt(ctx context.Context) error {
	return t.cli.clearNCTSalt(ctx)
}

// ElementMissing devolve o tipo concreto historico. ElementMissingError e' erro
// generico de parsing de XML do fork inteiro (usado por group, usync, newsletter,
// pair-code, ...), nao deste dominio, entao continua definido na raiz; so' a
// construcao atravessa a interface.
func (t appStateTransport) ElementMissing(tag, in string) error {
	return &ElementMissingError{Tag: tag, In: in}
}
