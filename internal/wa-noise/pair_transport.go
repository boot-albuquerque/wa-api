// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/pairing"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// pairTransport adapta *Client a pairing.Transport. Existe para que o pacote
// internal/wa-noise/pairing possa operar sobre uma interface estreita sem
// importar o pacote raiz (o que fecharia um ciclo) e sem que *Client precise
// ganhar metodos exportados novos so' para satisfazer a interface.
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 4".
type pairTransport struct {
	cli *Client
}

var _ pairing.Transport = pairTransport{}

// pairT devolve o adaptador de pareamento deste cliente.
func (cli *Client) pairT() pairing.Transport {
	return pairTransport{cli}
}

func (t pairTransport) Store() *store.Device {
	return t.cli.Store
}

// State devolve o ponteiro para o estado de pareamento do cliente. O ponteiro
// precisa ser estavel — pairState e' campo de *Client e pairTransport embrulha
// o ponteiro do cliente.
func (t pairTransport) State() *pairing.State {
	return &t.cli.pairState
}

func (t pairTransport) Log() waLog.Logger {
	return t.cli.Log
}

func (t pairTransport) SendNode(ctx context.Context, node waBinary.Node) error {
	return t.cli.sendNode(ctx, node)
}

// SendIQ traduz pairing.IQ para o infoQuery da raiz. Os campos que este
// dominio nunca preenche (Target, ID, SMaxID, Timeout, NoRetry) ficam no zero,
// exatamente como ficavam quando os nos eram montados na raiz.
func (t pairTransport) SendIQ(ctx context.Context, query pairing.IQ) (*waBinary.Node, error) {
	return t.cli.sendIQ(ctx, infoQuery{
		Namespace: query.Namespace,
		Type:      infoQueryType(query.Type),
		To:        query.To,
		Content:   query.Content,
	})
}

// DispatchEvent descarta o retorno de handlerFailed: nenhum dos pontos de
// despacho deste dominio o consultava antes da extracao.
func (t pairTransport) DispatchEvent(evt any) {
	t.cli.dispatchEvent(evt)
}

func (t pairTransport) ConfiguredClientType() pairing.ClientType {
	return t.cli.QRClientType
}

// PrePairAllowed reproduz o `cli.PrePairCallback != nil && !cli.PrePairCallback(...)`
// original: sem callback configurado, o pareamento e' permitido.
func (t pairTransport) PrePairAllowed(jid types.JID, platform, businessName string) bool {
	if t.cli.PrePairCallback == nil {
		return true
	}
	return t.cli.PrePairCallback(jid, platform, businessName)
}

func (t pairTransport) StoreLIDPNMapping(ctx context.Context, lid, pn types.JID) {
	t.cli.StoreLIDPNMapping(ctx, lid, pn)
}

func (t pairTransport) ExpectDisconnect() {
	t.cli.expectDisconnect()
}

func (t pairTransport) Disconnect() {
	t.cli.Disconnect()
}

func (t pairTransport) SendUnifiedSession() {
	t.cli.sendUnifiedSession()
}

func (t pairTransport) SetServerTimeOffset(offset int64) {
	t.cli.serverTimeOffset.Store(offset)
}

// ElementMissing devolve o tipo concreto historico. ElementMissingError e' erro
// generico de parsing de XML do fork inteiro, nao deste dominio, entao continua
// definido na raiz; so' a construcao atravessa a interface.
func (t pairTransport) ElementMissing(tag, in string) error {
	return &ElementMissingError{Tag: tag, In: in}
}
