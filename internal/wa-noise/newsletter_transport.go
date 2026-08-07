// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/newsletter"
	"wa-api/internal/wa-noise/protocol/proto/waWa6"
	"wa-api/internal/wa-noise/protocol/types"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// newsletterTransport adapta *Client a newsletter.Transport. Existe para que o
// pacote internal/wa-noise/newsletter possa operar sobre uma interface estreita
// sem importar o pacote raiz (o que fecharia um ciclo) e sem que *Client precise
// ganhar metodos exportados novos so' para satisfazer a interface.
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 2".
type newsletterTransport struct {
	cli *Client
}

var _ newsletter.Transport = newsletterTransport{}

// newsletterT devolve o adaptador de newsletter deste cliente.
func (cli *Client) newsletterT() newsletter.Transport {
	return newsletterTransport{cli}
}

// SendIQ traduz newsletter.IQ para o infoQuery da raiz. Os campos que o dominio
// de newsletter nunca preenche (Target, ID, SMaxID, Timeout, NoRetry) ficam no
// zero, exatamente como ficavam quando os nos eram montados na raiz.
func (t newsletterTransport) SendIQ(ctx context.Context, query newsletter.IQ) (*waBinary.Node, error) {
	return t.cli.sendIQ(ctx, infoQuery{
		Namespace: query.Namespace,
		Type:      infoQueryType(query.Type),
		To:        query.To,
		Content:   query.Content,
	})
}

func (t newsletterTransport) SendNode(ctx context.Context, node waBinary.Node) error {
	return t.cli.sendNode(ctx, node)
}

func (t newsletterTransport) GenerateRequestID() string {
	return t.cli.generateRequestID()
}

func (t newsletterTransport) WaitResponse(reqID string) chan *waBinary.Node {
	return t.cli.waitResponse(reqID)
}

func (t newsletterTransport) CancelResponse(reqID string, ch chan *waBinary.Node) {
	t.cli.cancelResponse(reqID, ch)
}

func (t newsletterTransport) GenerateMessageID() types.MessageID {
	return t.cli.GenerateMessageID()
}

func (t newsletterTransport) ParseMessages(node *waBinary.Node) []*types.NewsletterMessage {
	return t.cli.parseNewsletterMessages(node)
}

func (t newsletterTransport) ClientPayload() *waWa6.ClientPayload {
	return t.cli.Store.GetClientPayload()
}

// ElementMissing devolve o tipo concreto historico. ElementMissingError e' erro
// generico de parsing de XML do fork inteiro (usado por group, usync, appstate,
// pair-code, ...), nao do dominio de newsletter, entao continua definido aqui;
// so' a construcao atravessa a interface.
func (t newsletterTransport) ElementMissing(tag, in string) error {
	return &ElementMissingError{Tag: tag, In: in}
}

func (t newsletterTransport) Log() waLog.Logger {
	return t.cli.Log
}
