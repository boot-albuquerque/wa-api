// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package user

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

// IQ e' a fatia de infoQuery (pacote raiz) que o dominio de usuario usa. Existe
// para que Transport nao precise expor o infoQuery da raiz, o que arrastaria a
// raiz para dentro deste pacote e refaria o ciclo de import. O adaptador da
// raiz traduz IQ -> infoQuery campo a campo; os campos que este dominio nunca
// preenche (ID, SMaxID, Timeout, NoRetry) ficam no zero, exatamente como
// ficavam antes da extracao.
//
// Target so' e' usado por GetProfilePictureInfo, o unico <iq> deste dominio que
// preenche o alvo separado do destinatario. To fica no zero nos <iq> de w:qr,
// que o WhatsApp Android manda sem "to" — comportamento preservado literalmente.
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
// mesmo racional que manteve ElementMissingError la' nos lotes 2-6.
//
// Este dominio so' consulta dois deles (401 em GetProfilePictureInfo, 404 em
// GetProfilePictureInfo/ResolveBusinessMessageLink/ResolveContactQRLink); os
// outros tres ficam de fora da struct por nao terem chamador aqui.
type IQErrors struct {
	NotAuthorized error
	NotFound      error
}

// Transport e' a fatia do cliente de que o dominio de usuario precisa.
//
// Deliberadamente nao expoe nada do *whatsmeow.Client alem disso — e' o que
// permite que este pacote nao importe o pacote raiz e que os testes usem um
// duble em vez de um cliente real com socket e sessao Noise.
type Transport interface {
	// SendIQ manda um <iq> e espera a resposta.
	SendIQ(ctx context.Context, query IQ) (*waBinary.Node, error)
	// Store e' o device store da sessao. Este dominio grava push names, nomes
	// business e mapeamentos LID/PN, e le privacy tokens ao pedir foto.
	Store() *store.Device
	// DeviceCache e' o cache de listas de dispositivo da sessao. O ponteiro
	// precisa ser estavel: DeviceCache contem um mutex e nunca pode ser
	// copiado por valor.
	DeviceCache() *DeviceCache
	// Log e' o logger do cliente.
	Log() waLog.Logger
	// GenerateRequestID gera o "sid" da consulta usync. Vive em request.go, na
	// raiz, porque e' do substrato de requisicao do fork inteiro.
	GenerateRequestID() string
	// DispatchEvent publica um evento no barramento do cliente. UpdatePushName
	// e UpdateBusinessName emitem events.PushName / events.BusinessName.
	DispatchEvent(evt any)
	// ParticipantListHash calcula o dhash de uma lista de dispositivos. A
	// funcao (participantListHashV2) vive em send_transport.go, na raiz,
	// porque e' do dominio de ENVIO — ela hasheia listas de participantes de
	// grupo tanto quanto listas de dispositivo. So' a operacao atravessa a
	// interface; a definicao fica onde esta' ate' o lote de send.
	ParticipantListHash(jids []types.JID) string
	// ElementMissing monta o erro de elemento XML ausente. O tipo concreto
	// (*whatsmeow.ElementMissingError) e' generico do fork inteiro, nao do
	// dominio de usuario, entao continua definido na raiz; a construcao passa
	// por aqui para preservar o tipo exato que os chamadores historicos
	// recebem em um type assert.
	ElementMissing(tag, in string) error
	// WrapIQError embrulha um erro humano com o erro de IQ original,
	// preservando o *wrappedIQError historico da raiz.
	WrapIQError(human, iq error) error
	// IQErrors devolve os sentinelas de erro de <iq> da raiz. Ver IQErrors.
	IQErrors() IQErrors
}
