// Package appstatesync implementa o protocolo de sincronizacao de app state:
// buscar patches do servidor, aplica-los, traduzir cada mutacao em evento,
// enviar patches locais e pedir as chaves que faltam.
//
// O nome nao e' `appstate` de proposito: `internal/noise/appstate` ja' existe
// desde a Fase B e cuida da camada de baixo (decodificacao/codificacao de
// patches, hash chain, chaves). Este pacote e' a camada de cima, que era a
// "cola" na raiz do fork; ele importa aquele, nunca o contrario.
//
// Como os demais subpacotes extraidos na Fase F/G (media/, newsletter/), este
// pacote **nao importa o pacote raiz**: opera sobre a interface Transport, e a
// raiz mantem metodos-fachada em *Client que delegam para ca'. Ver ADR-0004 e
// PATCHES.md.
package appstatesync

import "time"

// Constantes do protocolo. Os valores sao exatamente os que estavam em
// `appstate_constants.go` na raiz antes desta extracao — mover nao mudou
// nenhum deles, so' caiu o prefixo `appState`, que agora e' o nome do pacote.

// Namespace e nos XML da consulta de sincronizacao de app state
// (`<iq><sync><collection>`), usada tanto no fetch quanto no send de patches.
const (
	namespace     = "w:sync:app:state"
	syncTag       = "sync"
	collectionTag = "collection"
	patchTag      = "patch"
	errorTag      = "error"
)

// Atributos do no `<collection>` e do `<error>` da resposta.
const (
	attrName           = "name"
	attrVersion        = "version"
	attrReturnSnapshot = "return_snapshot"
	attrType           = "type"
	attrCode           = "code"
)

const (
	// respTypeError e' o valor do atributo `type` que marca a `<collection>`
	// de resposta como falha em vez de confirmacao.
	respTypeError = "error"
	// conflictCode e' o codigo de erro que o servidor devolve quando a versao
	// enviada esta' desatualizada: os patches conflitantes vem junto na
	// resposta e o envio pode ser refeito uma vez apos aplica-los.
	conflictCode = 409
)

// Contextos usados no campo `In` do erro de elemento ausente.
const (
	fetchErrContext = "app state patch response"
	sendErrContext  = "app state send response"
)

// Namespace e no da consulta que marca uma colecao como "nao suja"
// (MarkNotDirty). E' um protocolo separado do de app state, apesar de ser
// disparado pelas mesmas notificacoes.
const (
	dirtyNamespace          = "urn:xmpp:whatsapp:dirty"
	dirtyCleanTag           = "clean"
	dirtyCleanAttrType      = "type"
	dirtyCleanAttrTimestamp = "timestamp"
)

// KeyRequestInterval e' o intervalo minimo entre dois pedidos da mesma chave de
// app state ao dispositivo primario. Sem esse limite, um patch que nunca
// decodifica geraria um pedido por tentativa de sync.
const KeyRequestInterval = 24 * time.Hour

// Valores booleanos serializados como string dentro dos indices de mutacao. O
// protocolo representa flags como "0"/"1" dentro do array do indice.
//
// Duplicam `indexBoolFalse`/`indexBoolTrue` de `appstate/constants.go` de
// proposito: aqueles sao nao exportados e exporta-los so' para esta leitura
// alargaria a superficie publica daquele subpacote.
const (
	// indexTrue e' o "verdadeiro" das flags de indice (`isFromMe`,
	// `deleteMedia`).
	indexTrue = "1"
	// indexSelfSender e' a sentinela usada na posicao de sender JID quando o
	// remetente e' o proprio dono do chat.
	indexSelfSender = "0"
)

// Tamanhos minimos do array de indice de cada tipo de mutacao. O indice vem do
// servidor, entao todo acesso posicional precisa ser guardado: um indice curto
// demais causaria panico no handler de nos, que roda numa goroutine sem
// recover (`client_events.go`, handlerQueueLoop).
const (
	// indexMinLenLabelEdit cobre [tipo, labelID].
	indexMinLenLabelEdit = 2
	// indexMinLenDeleteChatMedia cobre [tipo, jid, deleteMedia].
	indexMinLenDeleteChatMedia = 3
	// indexMinLenLabelAssocChat cobre [tipo, labelID, jid].
	indexMinLenLabelAssocChat = 3
	// indexMinLenClearChatMedia cobre [tipo, jid, ?, deleteMedia].
	indexMinLenClearChatMedia = 4
	// indexMinLenStar cobre [tipo, chatJID, msgID, isFromMe, sender].
	indexMinLenStar = 5
	// indexMinLenDeleteForMe tem o mesmo layout de star.
	indexMinLenDeleteForMe = 5
	// indexMinLenLabelAssocMessage cobre
	// [tipo, labelID, jid, msgID, isFromMe, sender].
	indexMinLenLabelAssocMessage = 6
)
