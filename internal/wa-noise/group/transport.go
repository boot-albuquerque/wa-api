// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package group

import (
	"context"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// IQType e' o atributo "type" de um <iq>. Espelha o infoQueryType do pacote
// raiz sem depender dele.
type IQType string

const (
	IQSet IQType = "set"
	IQGet IQType = "get"
)

// IQ e' a fatia de infoQuery (pacote raiz) que o dominio de grupo usa. Existe
// para que Transport nao precise expor o infoQuery da raiz, o que arrastaria a
// raiz para dentro deste pacote e refaria o ciclo de import. O adaptador da
// raiz traduz IQ -> infoQuery campo a campo; os campos que este dominio nunca
// preenche (ID, SMaxID, Timeout, NoRetry) ficam no zero, exatamente como
// ficavam antes da extracao.
//
// Target so' e' usado por SetPhoto, o unico <iq> deste dominio que nao vai
// para o namespace w:g2.
type IQ struct {
	Namespace string
	Type      IQType
	To        types.JID
	Target    types.JID
	Content   any
}

// IQErrors reune os sentinelas de erro de <iq> do pacote raiz de que este
// dominio precisa para mapear codigo de erro do servidor -> erro de dominio.
//
// Sao os MESMOS ponteiros da raiz, nao copias. Isso importa: IQError.Is compara
// Code **e** Text, entao um sentinela reconstruido aqui casaria por acidente
// hoje e divergiria em silencio se a raiz mudasse um Text. Passar os valores
// originais preserva a semantica de errors.Is exatamente como era antes da
// extracao.
//
// Os erros de IQ continuam definidos na raiz (errors.go, junto do substrato de
// request.go) porque sao genericos do fork inteiro, nao deste dominio — o
// mesmo racional que manteve ElementMissingError la' nos lotes 2, 3 e 4.
type IQErrors struct {
	NotAuthorized error
	Forbidden     error
	NotFound      error
	NotAcceptable error
	Gone          error
}

// Transport e' a fatia do cliente de que o dominio de grupo precisa.
//
// Deliberadamente nao expoe nada do *whatsmeow.Client alem disso — e' o que
// permite que este pacote nao importe o pacote raiz e que os testes usem um
// duble em vez de um cliente real com socket e sessao Noise.
type Transport interface {
	// SendIQ manda um <iq> e espera a resposta.
	SendIQ(ctx context.Context, query IQ) (*waBinary.Node, error)
	// Store e' o device store da sessao. Este dominio grava mapeamentos
	// LID/PN e telefones redigidos, e le privacy tokens ao criar grupo.
	Store() *store.Device
	// Cache e' o cache de metadados de grupo da sessao. O ponteiro precisa ser
	// estavel: Cache contem um mutex e nunca pode ser copiado por valor.
	Cache() *Cache
	// Log e' o logger do cliente.
	Log() waLog.Logger
	// GenerateMessageID gera um ID de mensagem quando o chamador nao fornece.
	GenerateMessageID() types.MessageID
	// TrimMessageIDPrefix remove o prefixo estatico dos IDs de mensagem "web"
	// (whatsmeow.WebMessageIDPrefix). O WhatsApp Web nao inclui esse prefixo na
	// chave de criacao de grupo. A constante continua em message_id.go, na
	// raiz, porque e' do dominio de ID de mensagem e nao deste.
	TrimMessageIDPrefix(id types.MessageID) string
	// ElementMissing monta o erro de elemento XML ausente. O tipo concreto
	// (*whatsmeow.ElementMissingError) e' generico do fork inteiro, nao do
	// dominio de grupo, entao continua definido na raiz; a construcao passa por
	// aqui para preservar o tipo exato que os chamadores historicos recebem em
	// um type assert.
	ElementMissing(tag, in string) error
	// WrapIQError embrulha um erro humano com o erro de IQ original,
	// preservando o *wrappedIQError historico da raiz.
	WrapIQError(human, iq error) error
	// IQErrors devolve os sentinelas de erro de <iq> da raiz. Ver IQErrors.
	IQErrors() IQErrors
}

// sendIQ e' o atalho interno para os <iq> do namespace w:g2, que sao todos os
// deste dominio menos SetPhoto. Substitui o cli.sendGroupIQ da raiz.
func sendIQ(ctx context.Context, t Transport, iqType IQType, jid types.JID, content waBinary.Node) (*waBinary.Node, error) {
	return t.SendIQ(ctx, IQ{
		Namespace: iqNamespace,
		Type:      iqType,
		To:        jid,
		Content:   []waBinary.Node{content},
	})
}

// SendIQ e' a forma exportada de sendIQ, usada pela fachada cli.sendGroupIQ da
// raiz (que por sua vez e' chamada por disappearing_timer.go e exposta em
// internals.go).
func SendIQ(ctx context.Context, t Transport, iqType IQType, jid types.JID, content waBinary.Node) (*waBinary.Node, error) {
	return sendIQ(ctx, t, iqType, jid, content)
}
