// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/user"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// userTransport adapta *Client a user.Transport. Existe para que o pacote
// internal/wa-noise/user possa operar sobre uma interface estreita sem importar
// o pacote raiz (o que fecharia um ciclo) e sem que *Client precise ganhar
// metodos exportados novos so' para satisfazer a interface.
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 7".
type userTransport struct {
	cli *Client
}

var _ user.Transport = userTransport{}

// userT devolve o adaptador de usuario deste cliente.
func (cli *Client) userT() user.Transport {
	return userTransport{cli}
}

// SendIQ traduz user.IQ para o infoQuery da raiz. Os campos que este dominio
// nunca preenche (ID, SMaxID, Timeout, NoRetry) ficam no zero, exatamente como
// ficavam quando os nos eram montados na raiz.
func (t userTransport) SendIQ(ctx context.Context, query user.IQ) (*waBinary.Node, error) {
	return t.cli.sendIQ(ctx, infoQuery{
		Namespace: query.Namespace,
		Type:      infoQueryType(query.Type),
		To:        query.To,
		Target:    query.Target,
		Content:   query.Content,
	})
}

func (t userTransport) Store() *store.Device {
	return t.cli.Store
}

// DeviceCache devolve o ponteiro para o cache de dispositivos do cliente. O
// ponteiro precisa ser estavel — userDevicesCache e' campo de *Client e
// userTransport embrulha o ponteiro do cliente, entao o mutex de dentro nunca
// e' copiado por valor.
func (t userTransport) DeviceCache() *user.DeviceCache {
	return &t.cli.userDevicesCache
}

func (t userTransport) Log() waLog.Logger {
	return t.cli.Log
}

// GenerateRequestID mantem generateRequestID em request.go, onde ele e'
// definido: e' do substrato de requisicao do fork inteiro, nao deste dominio.
func (t userTransport) GenerateRequestID() string {
	return t.cli.generateRequestID()
}

func (t userTransport) DispatchEvent(evt any) {
	t.cli.dispatchEvent(evt)
}

// ParticipantListHash mantem participantListHashV2 em send_transport.go: ele
// hasheia listas de participantes de grupo tanto quanto listas de dispositivo,
// e e' do dominio de envio. So' a operacao atravessa a interface.
func (t userTransport) ParticipantListHash(jids []types.JID) string {
	return participantListHashV2(jids)
}

// ElementMissing devolve o tipo concreto historico. ElementMissingError e' erro
// generico de parsing de XML do fork inteiro (usado por group, usync, appstate,
// pair-code, ...), nao do dominio de usuario, entao continua definido na raiz;
// so' a construcao atravessa a interface.
func (t userTransport) ElementMissing(tag, in string) error {
	return &ElementMissingError{Tag: tag, In: in}
}

// WrapIQError preserva o *wrappedIQError historico, cujo Is() casa contra o
// erro humano e cujo Unwrap() devolve o erro de IQ.
func (t userTransport) WrapIQError(human, iq error) error {
	return wrapIQError(human, iq)
}

// IQErrors entrega os MESMOS ponteiros de sentinela da raiz. Ver o doc de
// user.IQErrors para por que isso importa.
func (t userTransport) IQErrors() user.IQErrors {
	return user.IQErrors{
		NotAuthorized: ErrIQNotAuthorized,
		NotFound:      ErrIQNotFound,
	}
}
