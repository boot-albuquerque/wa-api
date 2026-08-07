// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package retry

import "time"

// Politica de retry: quantas vezes cada lado do protocolo pode insistir e por
// quanto tempo o estado auxiliar sobrevive. Os limites sao contadores de
// protocolo, nao ajustes de performance — mudar qualquer um deles muda quantas
// mensagens o servidor consegue nos fazer reprocessar.
const (
	// MaxIncomingRequests e' o teto de pedidos de retry que aceitamos do
	// mesmo par (remetente + ID de mensagem) antes de parar de responder. O
	// contador e' nosso, interno, e nao o `count` que o servidor manda — que e'
	// justamente o ponto: sem ele um par malicioso pediria retry da mesma
	// mensagem indefinidamente, e cada retry custa uma cifragem.
	MaxIncomingRequests = 10

	// MinCountForSessionRecreate e' a partir de qual `count` do recibo de
	// retry consideramos recriar a sessao Signal quando ja' temos uma. Abaixo
	// disso assume-se que a sessao existente ainda serve.
	MinCountForSessionRecreate = 2

	// MaxOutgoingReceipts e' quantos recibos de retry mandamos para a mesma
	// mensagem recebida antes de desistir de decifra-la.
	MaxOutgoingReceipts = 5

	// ReceiptVersion e' o atributo `v` do no <retry> que enviamos.
	ReceiptVersion = 1
)

// Formato de serializacao do buffer de mensagens recentes persistido
// (Client.UseRetryMessageStore). O valor e' gravado por AddRecent e lido de
// volta por ParseRecent — os dois precisam concordar, entao sao constantes e
// nao literais.
const (
	StoreFormatWA = "wa"
	StoreFormatFB = "fb"

	// StoreClearInterval e' de quanto em quanto tempo, no maximo, as mensagens
	// velhas seriam apagadas do store de retry.
	//
	// ATENCAO: o throttle que usa esta constante esta' morto no upstream — o
	// carimbo comparado contra ela (State.LastStoreClear) nunca e' escrito,
	// entao a comparacao e' sempre verdadeira. Ver F52 em HOUSEKEEP.md; a
	// extracao preservou o comportamento em vez de corrigi-lo.
	StoreClearInterval = 12 * time.Hour
)

// RecentMessagesSize e' quantas mensagens enviadas ficam em cache na memoria
// para atender recibos de retry. O buffer e' circular justamente para nao
// crescer sem limite (contraste com os dois contadores de F36).
const RecentMessagesSize = 256

// recreateSessionTimeout e' quanto tempo esperamos antes de recriar a sessao
// Signal com o mesmo par de novo.
const recreateSessionTimeout = 1 * time.Hour
