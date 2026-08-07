// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import "time"

// Constantes usadas pela cola de app state na raiz do pacote (appstate.go,
// appstate_dispatch.go, appstate_keys.go, appstate_send.go). Os valores sao
// exatamente os literais que estavam embutidos antes da Fase E do ADR-0004 —
// nomear nao mudou nenhum deles.
//
// O que ja e' exportado por `internal/wa-noise/appstate` (nomes de patch
// `WAPatch*` e nomes de indice `Index*`) e' reusado de la', nao reescrito
// aqui.

// Namespace e nos XML da consulta de sincronizacao de app state
// (`<iq><sync><collection>`), usada tanto no fetch quanto no send de patches.
const (
	appStateNamespace     = "w:sync:app:state"
	appStateSyncTag       = "sync"
	appStateCollectionTag = "collection"
	appStatePatchTag      = "patch"
	appStateErrorTag      = "error"
)

// Atributos do no `<collection>` e do `<error>` da resposta.
const (
	appStateAttrName           = "name"
	appStateAttrVersion        = "version"
	appStateAttrReturnSnapshot = "return_snapshot"
	appStateAttrType           = "type"
	appStateAttrCode           = "code"
)

const (
	// appStateRespTypeError e' o valor do atributo `type` que marca a
	// `<collection>` de resposta como falha em vez de confirmacao.
	appStateRespTypeError = "error"
	// appStateConflictCode e' o codigo de erro que o servidor devolve quando
	// a versao enviada esta' desatualizada: os patches conflitantes vem junto
	// na resposta e o envio pode ser refeito uma vez apos aplica-los.
	appStateConflictCode = 409
)

// Contextos usados no campo `In` de `ElementMissingError`.
const (
	appStateFetchErrContext = "app state patch response"
	appStateSendErrContext  = "app state send response"
)

// Namespace e no da consulta que marca uma colecao como "nao suja"
// (`Client.MarkNotDirty`). E' um protocolo separado do de app state, apesar de
// ser disparado pelas mesmas notificacoes.
const (
	dirtyNamespace          = "urn:xmpp:whatsapp:dirty"
	dirtyCleanTag           = "clean"
	dirtyCleanAttrType      = "type"
	dirtyCleanAttrTimestamp = "timestamp"
)

// appStateKeyRequestInterval e' o intervalo minimo entre dois pedidos da mesma
// chave de app state ao dispositivo primario. Sem esse limite, um patch que
// nunca decodifica geraria um pedido por tentativa de sync.
const appStateKeyRequestInterval = 24 * time.Hour

// Valores booleanos serializados como string dentro dos indices de mutacao. O
// protocolo representa flags como "0"/"1" dentro do array do indice.
//
// Duplicam `indexBoolFalse`/`indexBoolTrue` de `appstate/constants.go` de
// proposito: aqueles sao nao exportados e exporta-los so' para esta leitura
// alargaria a superficie publica do subpacote.
const (
	// appStateIndexTrue e' o "verdadeiro" das flags de indice (`isFromMe`,
	// `deleteMedia`).
	appStateIndexTrue = "1"
	// appStateIndexSelfSender e' a sentinela usada na posicao de sender JID
	// quando o remetente e' o proprio dono do chat.
	appStateIndexSelfSender = "0"
)

// Tamanhos minimos do array de indice de cada tipo de mutacao. O indice vem do
// servidor, entao todo acesso posicional precisa ser guardado: um indice curto
// demais causaria panico no handler de nos, que roda numa goroutine sem
// recover (`client_events.go`, handlerQueueLoop).
const (
	// appStateIndexMinLenLabelEdit cobre [tipo, labelID].
	appStateIndexMinLenLabelEdit = 2
	// appStateIndexMinLenDeleteChatMedia cobre [tipo, jid, deleteMedia].
	appStateIndexMinLenDeleteChatMedia = 3
	// appStateIndexMinLenLabelAssocChat cobre [tipo, labelID, jid].
	appStateIndexMinLenLabelAssocChat = 3
	// appStateIndexMinLenClearChatMedia cobre [tipo, jid, ?, deleteMedia].
	appStateIndexMinLenClearChatMedia = 4
	// appStateIndexMinLenStar cobre [tipo, chatJID, msgID, isFromMe, sender].
	appStateIndexMinLenStar = 5
	// appStateIndexMinLenDeleteForMe tem o mesmo layout de star.
	appStateIndexMinLenDeleteForMe = 5
	// appStateIndexMinLenLabelAssocMessage cobre
	// [tipo, labelID, jid, msgID, isFromMe, sender].
	appStateIndexMinLenLabelAssocMessage = 6
)
