// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package message reune o caminho de ENTRADA de mensagens do fork: parsing do
// stanza <message>, decifragem Signal (DM, grupo e bot), o tratamento das
// partes de protocolo da mensagem ja' decifrada, o history sync, os segredos de
// mensagem (reaction/comment/poll/event) e a geracao de IDs de mensagem.
//
// # Por que `message` e nao `decrypt`
//
// O nome segue a convencao ja' acordada para a Fase H (`capabilities/message/`)
// e a regra que a acompanha: so' se chamaria `decrypt` um componente que
// fizesse *exclusivamente* parsing criptografico. Este faz muito mais —
// `parse.go` nao decifra nada, `id.go` e `builders.go` sao do caminho de
// composicao, `history_sync.go` baixa e descomprime blob, `secrets_store.go`
// grava mapeamentos LID/PN. Decifrar e' UMA das responsabilidades, nao a
// unica.
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 9".
package message

import (
	"context"

	"wa-api/internal/wa-noise/appstate"
	"wa-api/internal/wa-noise/appstatesync"
	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/proto/waHistorySync"
	"wa-api/internal/wa-noise/proto/waWeb"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// Errors reune os sentinelas de erro do pacote RAIZ de que este dominio
// precisa.
//
// ErrNotLoggedIn nao migrou para message/errors.go, ao contrario dos seis
// sentinelas exclusivos deste caminho, pelo mesmo racional de send.Errors
// (lote 8): 15 arquivos da raiz o devolvem, entao ele e' do fork inteiro, nao
// deste dominio. E' o MESMO ponteiro da raiz, nao uma copia, de modo que
// errors.Is continua casando exatamente como antes da extracao.
type Errors struct {
	NotLoggedIn error
}

// Nacks reune os tres codigos de nack que o caminho de recepcao usa ao acusar
// um <message> que nao deu para processar.
//
// Eles NAO foram redeclarados aqui: sao valores de fio (488/491/495) e a tabela
// inteira dos treze vive em receipt.go, na raiz, que e' o dominio de recibo e
// esta' fora do escopo do lote. Redeclarar tres deles deste lado da fronteira
// e' exatamente a divergencia silenciosa que a Fase F/G existe para impedir —
// entao eles atravessam como DADO, pelo mesmo racional de group.IQErrors
// (lote 6) e user.IQErrors (lote 7).
type Nacks struct {
	UnrecognizedStanza   int
	InvalidProtobuf      int
	MissingMessageSecret int
}

// Transport e' a fatia do cliente de que o dominio de recepcao de mensagem
// precisa.
//
// E' largo — comparavel ao de send/ (lote 8) — e pelo mesmo motivo estrutural,
// espelhado: receber uma mensagem e' o caminho que TOCA TODOS OS OUTROS
// DOMINIOS. Parsear consulta o proprio JID; decifrar consulta a sessao Signal e
// o buffer de eventos; falhar em decifrar dispara recibo de retry e nack; a
// mensagem decifrada atualiza push name e nome business (usuario), distribui
// sender key (grupo), religa o app state (appstatesync) e baixa history sync
// (media). Cada metodo abaixo corresponde a uma chamada que o codigo fazia em
// *Client antes da extracao — NENHUM foi inventado para conveniencia.
//
// # Fachada da raiz, e nao import direto, para usuario/appstate/media
//
// Mesma decisao do lote 8, e pelas mesmas razoes. `UpdatePushName`,
// `UpdateBusinessName`, `HandleHistoricalPushNames` (dominio de usuario),
// `FetchAppState`/`HandleAppStateRecovery` (appstate/appstatesync),
// `DownloadHistorySyncBlob`/`DeleteHistorySyncMedia` (media/download) passam
// pela fachada da raiz porque as free functions daqueles pacotes recebem os
// transportes DELES: chama-las direto obrigaria Transport a expor tres
// interfaces alheias inteiras para alcancar duas ou tres funcoes de cada, e
// obrigaria o duble de teste deste pacote a saber montar um <iq> de usync ou um
// download de media. Alem disso, as fachadas sao EXATAMENTE o que o codigo
// original chamava (`cli.updatePushName`, `cli.FetchAppState`, ...), o que
// mantem a extracao literal.
//
// # Import direto, nas excecoes, todas por serem DADO ou funcao PURA
//
//   - `user.ParseVerifiedNameContent` — funcao pura (Node -> *types.VerifiedName),
//     sem transporte nem estado. Isso fecha a divida que o lote 7 deixou
//     documentada em user.go ("usado por message_parse.go, que continua na raiz
//     ... lote 9, pendente").
//   - `send.*` — as constantes de fio do <enc> e o tamanho do segredo de
//     mensagem, importadas em vez de redeclaradas, para que entrada e saida nao
//     possam divergir. Mesma razao pela qual o lote 8 importou
//     `retry.FBApplicationVersion`.
//   - `msgpad.Unpad`, `media.EncryptRetryReceipt`, `gcmutil`, `hkdfutil` —
//     funcoes puras de pacotes folha.
//   - `appstate.AllPatchNames` — dado (lista de nomes de patch).
//   - `appstatesync.State` — estado com mutex proprio, alcancado por ponteiro
//     estavel, mesmo racional de `group.Cache`/`user.DeviceCache` nos lotes 6/7.
//   - `store.SignalProtobufSerializer` — usado direto, e nao pelo `pbSerializer`
//     da raiz, porque sao o MESMO valor e `store` ja' e' folha (o lote 8 fez
//     igual).
type Transport interface {
	// Store e' o device store da sessao: sessoes Signal, identidades, segredos
	// de mensagem, buffer de eventos e mapeamentos LID/PN.
	Store() *store.Device
	// Log e' o logger do cliente.
	Log() waLog.Logger
	// OwnID e' o JID de telefone da sessao; OwnLID, o JID oculto.
	OwnID() types.JID
	OwnLID() types.JID
	// IsMessenger diz se a sessao e' Messenger (MessengerConfig != nil). Muda a
	// geracao de ID de mensagem e o filtro de push name "username".
	IsMessenger() bool
	// Errors entrega os sentinelas da raiz. Ver o doc de Errors.
	Errors() Errors
	// Nacks entrega os codigos de nack da raiz. Ver o doc de Nacks.
	Nacks() Nacks

	// DispatchEvent entrega um evento aos handlers e devolve true se algum
	// falhou.
	DispatchEvent(evt any) (handlerFailed bool)
	// SendNode escreve um no' no socket.
	SendNode(ctx context.Context, node waBinary.Node) error

	// AutoTrustIdentity liga a limpeza automatica de identidade nao confiavel
	// no caminho de prekey.
	AutoTrustIdentity() bool
	// EnableDecryptedEventBuffer liga o buffer de eventos decifrados.
	EnableDecryptedEventBuffer() bool
	// SynchronousAck faz o ack sair so' depois dos handlers.
	SynchronousAck() bool
	// ManualHistorySyncDownload e DisableManualHistorySyncReceipt controlam se
	// o loop de history sync baixa sozinho e se o recibo sai mesmo assim.
	ManualHistorySyncDownload() bool
	DisableManualHistorySyncReceipt() bool
	// BackgroundEventCtx e' o contexto de fundo em que o loop de history sync
	// roda.
	BackgroundEventCtx() context.Context

	// BackgroundIfAsyncAck roda fn na hora se SynchronousAck, senao em
	// goroutine. Substrato de recibo (receipt.go), generico do fork.
	BackgroundIfAsyncAck(fn func())
	// MaybeDeferredAck devolve o fechamento de ack diferido de um no'.
	MaybeDeferredAck(ctx context.Context, node *waBinary.Node) func(cancelled ...*bool)
	// SendAck acusa um no' com o codigo de erro dado (0 = sem erro).
	SendAck(ctx context.Context, node *waBinary.Node, errorCode int)
	// SendMessageReceipt manda o recibo de entrega da mensagem.
	SendMessageReceipt(ctx context.Context, info *types.MessageInfo, node *waBinary.Node)
	// SendRetryReceipt pede o reenvio de uma mensagem que nao deu para decifrar.
	SendRetryReceipt(ctx context.Context, node *waBinary.Node, info *types.MessageInfo, forceIncludeIdentity bool)
	// ImmediateRequestMessageFromPhone / CancelDelayedRequestFromPhone sao o par
	// de pedido de mensagem indisponivel ao telefone (retry_request_from_phone.go).
	ImmediateRequestMessageFromPhone(ctx context.Context, info *types.MessageInfo)
	CancelDelayedRequestFromPhone(id types.MessageID)

	// UpdateBusinessName / UpdatePushName / HandleHistoricalPushNames sao o
	// dominio de usuario (lote 7), alcancado pela fachada da raiz.
	UpdateBusinessName(ctx context.Context, jid, jidAlt types.JID, info *types.MessageInfo, name string)
	UpdatePushName(ctx context.Context, jid, jidAlt types.JID, info *types.MessageInfo, name string)
	HandleHistoricalPushNames(ctx context.Context, names []*waHistorySync.Pushname)
	// StoreLIDPNMapping grava um par LID/PN visto no stanza.
	StoreLIDPNMapping(ctx context.Context, first, second types.JID)

	// AppStateSync e' o estado de sincronizacao de app state (lote 3/appstatesync).
	// O ponteiro precisa ser estavel: State contem um RWMutex e nunca pode ser
	// copiado por valor.
	AppStateSync() *appstatesync.State
	// FetchAppState dispara a busca de um patch de app state.
	FetchAppState(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error
	// HandleAppStateRecovery trata a resposta de recuperacao fatal de snapshot.
	HandleAppStateRecovery(ctx context.Context, reqID types.MessageID, result []*waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult) bool

	// HandleDecryptedArmadillo trata o payload v3 ja' decifrado
	// (armadillomessage.go, nao extraido).
	HandleDecryptedArmadillo(ctx context.Context, info *types.MessageInfo, decrypted []byte, retryCount int) (handlerFailed, protobufFailed bool)
	// ParseWebMessage converte um WebMessageInfo em evento (client_session.go).
	ParseWebMessage(chatJID types.JID, webMsg *waWeb.WebMessageInfo) (*events.Message, error)

	// DownloadHistorySyncBlob baixa o blob apontado pela notificacao. E' um
	// metodo especifico, e nao o `Download` generico, para que Transport nao
	// precise expor o tipo `media.Downloadable`.
	DownloadHistorySyncBlob(ctx context.Context, notif *waE2E.HistorySyncNotification) ([]byte, error)
	// DeleteHistorySyncMedia apaga do servidor a media de history sync ja'
	// consumida (`DeleteMedia` com MediaHistory).
	DeleteHistorySyncMedia(ctx context.Context, directPath string, encFileHash []byte, encHandle string) error
	// StoreNCTSalt grava o salt de cstoken vindo do history sync (cstoken.go).
	StoreNCTSalt(ctx context.Context, salt []byte) error

	// HistorySync e' a fila de notificacoes de history sync deste cliente. Ver
	// o doc de HistorySyncQueue: o ponteiro precisa ser estavel.
	HistorySync() *HistorySyncQueue
	// DecryptBuffer e' o estado de limpeza do buffer de eventos decifrados. Ver
	// o doc de DecryptBufferState.
	DecryptBuffer() *DecryptBufferState
}
