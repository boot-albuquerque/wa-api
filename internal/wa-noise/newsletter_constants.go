// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

// Nomes do wire format compartilhados pelo caminho de newsletter (canais do
// WhatsApp). Só entram aqui os que aparecem em mais de um lugar; os nomes de
// tag e atributo usados uma única vez, no ponto onde o nó é montado ou lido,
// continuam literais — ver PATCHES.md (Fase E, lote 2).
//
// As query IDs do MEX ficam em newsletter_mex.go, junto do mapeamento
// web -> desktop que é o único consumidor delas.
const (
	// newsletterNamespace é o namespace <iq> das consultas de newsletter que
	// não passam pelo MEX (live updates, listagem de mensagens e updates).
	newsletterNamespace = "newsletter"

	// newsletterLiveUpdatesTag é o nó de assinatura temporária de updates ao
	// vivo: vai na requisição e é relido na resposta para extrair a duração.
	newsletterLiveUpdatesTag = "live_updates"
	// newsletterLiveUpdatesDurationAttr carrega, em segundos, por quanto tempo
	// a assinatura vale.
	newsletterLiveUpdatesDurationAttr = "duration"

	// newsletterMessagesTag e newsletterMessageUpdatesTag são os nós de
	// resposta paginada de mensagens e de updates de mensagem.
	newsletterMessagesTag       = "messages"
	newsletterMessageUpdatesTag = "message_updates"
	// newsletterMessagesErrContext é o campo In dos ElementMissingError dos
	// dois getters paginados.
	newsletterMessagesErrContext = "newsletter messages response"
)
