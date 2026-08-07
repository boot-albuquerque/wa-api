// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import "time"

// Politica de retry: quantas vezes cada lado do protocolo pode insistir e por
// quanto tempo o estado auxiliar sobrevive. Os limites sao contadores de
// protocolo, nao ajustes de performance — mudar qualquer um deles muda quantas
// mensagens o servidor consegue nos fazer reprocessar.
const (
	// maxIncomingRetryRequests e' o teto de pedidos de retry que aceitamos do
	// mesmo par (remetente + ID de mensagem) antes de parar de responder. O
	// contador e' nosso, interno, e nao o `count` que o servidor manda — que e'
	// justamente o ponto: sem ele um par malicioso pediria retry da mesma
	// mensagem indefinidamente, e cada retry custa uma cifragem.
	maxIncomingRetryRequests = 10

	// minRetryCountForSessionRecreate e' a partir de qual `count` do recibo de
	// retry consideramos recriar a sessao Signal quando ja' temos uma. Abaixo
	// disso assume-se que a sessao existente ainda serve.
	minRetryCountForSessionRecreate = 2

	// maxOutgoingRetryReceipts e' quantos recibos de retry mandamos para a mesma
	// mensagem recebida antes de desistir de decifra-la.
	maxOutgoingRetryReceipts = 5

	// retryReceiptVersion e' o atributo `v` do no <retry> que enviamos.
	retryReceiptVersion = 1
)

// Formato de serializacao do buffer de mensagens recentes persistido
// (Client.UseRetryMessageStore). O valor e' gravado por addRecentMessage e lido
// de volta por parseRecentMessage — os dois precisam concordar, entao sao
// constantes e nao literais.
const (
	retryStoreFormatWA = "wa"
	retryStoreFormatFB = "fb"

	// retryStoreClearInterval e' de quanto em quanto tempo, no maximo, as
	// mensagens velhas sao apagadas do store de retry.
	retryStoreClearInterval = 12 * time.Hour
)
