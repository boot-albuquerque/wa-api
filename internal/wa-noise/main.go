// A Fase H reorganizou a arvore em camadas:
//
//	core/          o nucleo (Client, conexao, request) — implementacao
//	capabilities/  group user media message newsletter notification pairing
//	               prekeys retry send tctoken appstatesync
//	protocol/ security/ persistence/ runtime/ observability/
//
// Este arquivo e' a fachada dessa arvore: e' o unico caminho de import que
// consumidores fora do fork (pkg/bootstrap/, pkg/infra/wa-noise/*,
// pkg/infra/{history,media}/) devem usar. Nenhum deles importa `core/`
// diretamente — `core/` e' detalhe de implementacao, e a direcao de dependencia
// declarada no inventario da Fase H
// (docs/FASE_H_INVENTORY.md) e' explicita: **nada de fora importa `core`**.
//
// A fachada e' fina de proposito. Ela NAO reexporta os ~732 simbolos de topo do
// nucleo um a um: `type Client = core.Client` e' um *alias de tipo*, entao o
// method set inteiro de `*Client` — inclusive os 178 wrappers de
// `DangerousInternals` — vem junto em uma linha. O que sobra sao os poucos
// tipos, constantes, erros e funcoes que os consumidores nomeiam
// explicitamente, listados abaixo. Se um consumidor novo precisar de um simbolo
// que ainda nao esta aqui, a correcao e' **adicionar a linha nesta fachada**,
// nunca importar `core/` direto.
//
// Regra de escrita: este arquivo so' contem aliases e delegacao. Nenhuma logica
// nova mora aqui — se algo precisa de corpo, mora em `core/` ou em
// `capabilities/`.
package wanoise

import (
	core "wa-api/internal/wa-noise/core"
)

// Cliente e construtor.
type (
	// Client e' o cliente WhatsApp. Alias de tipo: preserva o method set
	// completo de core.Client, inclusive DangerousInternals().
	Client = core.Client

	// EventHandler e' a assinatura de handler registrada em AddEventHandler.
	EventHandler = core.EventHandler
)

// NewClient constroi um Client. E' uma var, e nao um wrapper com corpo, para
// que a assinatura acompanhe core.NewClient sem reescrita manual.
var NewClient = core.NewClient

// Envio de mensagens e requests.
type (
	SendResponse     = core.SendResponse
	SendRequestExtra = core.SendRequestExtra
)

// Pareamento por QR.
type (
	QRChannelItem = core.QRChannelItem
)

const (
	QRChannelEventCode = core.QRChannelEventCode
)

// Proxy.
type (
	SetProxyOptions = core.SetProxyOptions
)

// Grupos.
type (
	ParticipantChange        = core.ParticipantChange
	ParticipantRequestChange = core.ParticipantRequestChange
	ReqCreateGroup           = core.ReqCreateGroup
)

const (
	ParticipantChangeAdd    = core.ParticipantChangeAdd
	ParticipantChangeRemove = core.ParticipantChangeRemove

	ParticipantChangeApprove = core.ParticipantChangeApprove
	ParticipantChangeReject  = core.ParticipantChangeReject
)

// Newsletters (canais).
//
// Reexportados no levantamento de paridade de 2026-08-20, quando as onze
// capacidades de newsletter entraram na fachada estreita: o consumidor
// (pkg/infra/wa-noise/adapters/misc) precisa de NOMEAR estes tipos para montar
// os parametros, e o gate waclient-facade proibe-o de importar core direto.
type (
	CreateNewsletterParams      = core.CreateNewsletterParams
	GetNewsletterMessagesParams = core.GetNewsletterMessagesParams
	GetNewsletterUpdatesParams  = core.GetNewsletterUpdatesParams
)

// Perfil e midia.
type (
	GetProfilePictureParams = core.GetProfilePictureParams
	DownloadableMessage     = core.DownloadableMessage

	// MediaType e UploadResponse expostos para CAP-02 (pkg/infra/wa-noise/
	// adapters/chat): consumidor precisa nomear o tipo de midia ao chamar
	// Client.Upload (alias de metodo, ja' presente via Client = core.Client)
	// e ler os campos de UploadResponse para montar a mensagem protobuf.
	MediaType      = core.MediaType
	UploadResponse = core.UploadResponse
)

const (
	MediaImage          = core.MediaImage
	MediaVideo          = core.MediaVideo
	MediaAudio          = core.MediaAudio
	MediaDocument       = core.MediaDocument
	MediaLinkThumbnail  = core.MediaLinkThumbnail
)

// Erros nomeados pelos consumidores. Sao as *mesmas* variaveis de core, entao
// errors.Is/== contra elas continua valendo.
var (
	ErrProfilePictureUnauthorized = core.ErrProfilePictureUnauthorized
	ErrProfilePictureNotSet       = core.ErrProfilePictureNotSet
	ErrQRStoreContainsID          = core.ErrQRStoreContainsID

	// Recusas do servidor do WhatsApp a um info query. Sao as sentinelas de
	// core/errors.go, entao errors.Is contra elas casa por codigo e texto (ver
	// IQError.Is) e nao por string formatada.
	//
	// Estao aqui porque a F204 mediu um 400 bad-request de montante a chegar ao
	// cliente como 500 internal server error, e a traducao para o vocabulario
	// de apperr precisa de as nomear. Sem esta reexportacao o unico caminho
	// seria casar o texto de err.Error(), que e' o que a entrada do HOUSEKEEP
	// assumia — desnecessariamente.
	ErrIQBadRequest          = core.ErrIQBadRequest
	ErrIQNotAuthorized       = core.ErrIQNotAuthorized
	ErrIQForbidden           = core.ErrIQForbidden
	ErrIQNotFound            = core.ErrIQNotFound
	ErrIQNotAllowed          = core.ErrIQNotAllowed
	ErrIQNotAcceptable       = core.ErrIQNotAcceptable
	ErrIQGone                = core.ErrIQGone
	ErrIQResourceLimit       = core.ErrIQResourceLimit
	ErrIQLocked              = core.ErrIQLocked
	ErrIQRateOverLimit       = core.ErrIQRateOverLimit
	ErrIQInternalServerError = core.ErrIQInternalServerError
	ErrIQServiceUnavailable  = core.ErrIQServiceUnavailable
	ErrIQPartialServerError  = core.ErrIQPartialServerError
	ErrIQTimedOut            = core.ErrIQTimedOut
)

// IQError e' a recusa estruturada de um info query. Alias de tipo: o campo
// Code (numerico) e' o que a traducao para apperr consulta, em vez de casar o
// texto formatado por Error().
type IQError = core.IQError
