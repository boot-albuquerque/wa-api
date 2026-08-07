// Package whatsmeow e' a porta de entrada de internal/wa-noise/ (fork ativo do
// whatsmeow, ADR-0004).
//
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
package whatsmeow

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

// Perfil e midia.
type (
	GetProfilePictureParams = core.GetProfilePictureParams
	DownloadableMessage     = core.DownloadableMessage
)

// Erros nomeados pelos consumidores. Sao as *mesmas* variaveis de core, entao
// errors.Is/== contra elas continua valendo.
var (
	ErrProfilePictureUnauthorized = core.ErrProfilePictureUnauthorized
	ErrProfilePictureNotSet       = core.ErrProfilePictureNotSet
	ErrQRStoreContainsID          = core.ErrQRStoreContainsID
)
