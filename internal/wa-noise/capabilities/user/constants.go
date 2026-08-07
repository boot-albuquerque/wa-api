// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package user

// Namespaces dos IQs de usuario.
const (
	statusIQNamespace         = "status"
	usyncIQNamespace          = "usync"
	blocklistIQNamespace      = "blocklist"
	botIQNamespace            = "bot"
	businessIQNamespace       = "w:biz"
	qrIQNamespace             = "w:qr"
	profilePictureIQNamespace = "w:profile:picture"
	fbidDevicesIQNamespace    = "fbid:devices"
)

// USync — o transporte de consulta em lote de informacao de usuario.
//
// As tags sao escritas por USync e lidas de volta na resposta (usyncNodeTag
// + usyncListTag em GetOptionalChildByTag, usyncUserTag no != que filtra
// os filhos da lista em IsOnWhatsApp, GetInfo, GetDevices e
// GetBotProfiles). Como literais, um typo em qualquer um dos dois lados nao e'
// erro de compilacao — a lista simplesmente sai vazia.
const (
	usyncNodeTag  = "usync"
	usyncQueryTag = "query"
	usyncListTag  = "list"
	usyncUserTag  = "user"

	// Os quatro valores de `mode`/`context` que este fork usa. Sao exportados
	// porque a fachada da raiz (`cli.usync`, citada por internals.go) recebe
	// mode e context como string do chamador, e os testes da raiz precisam
	// nomea-los.
	ModeQuery = "query"
	ModeFull  = "full"

	ContextInteractive = "interactive"
	ContextBackground  = "background"
	ContextMessage     = "message"

	// usyncLastValue/usyncIndexValue: este fork nunca pagina a consulta usync —
	// manda tudo num lote so' e declara ser a ultima pagina.
	usyncLastValue  = "true"
	usyncIndexValue = "0"
)

// Lista de dispositivos.
const (
	devicesNodeTag    = "devices"
	deviceListNodeTag = "device-list"
	deviceNodeTag     = "device"
	usersNodeTag      = "users"

	// deviceListVersion e' o valor do atributo `version` de <devices>, que
	// seleciona o formato de resposta do servidor. O parser deste pacote
	// (ParseDeviceList) so' entende o formato 2.
	deviceListVersion = "2"

	// fbIDDeviceChunkSize e' o numero de usuarios por IQ `fbid:devices`.
	fbIDDeviceChunkSize = 15
)

// Contato / IsOnWhatsApp.
const (
	contactNodeTag = "contact"
	// contactTypeIn marca, na resposta de IsOnWhatsApp, que o numero
	// consultado esta' na agenda do usuario.
	contactTypeIn = "in"

	lidNodeTag    = "lid"
	statusNodeTag = "status"

	pictureNodeTag  = "picture"
	picturesNodeTag = "pictures"
)

// Bots.
const (
	botSectionTag = "section"
	// botSectionTypeAll e' a unica secao de GetBotListV2 cujos filhos viram
	// entradas da lista; o servidor manda outras secoes (destaques etc.).
	botSectionTypeAll = "all"

	botCommandsTag = "commands"
	botCommandTag  = "command"
	botPromptsTag  = "prompts"
	botPromptTag   = "prompt"

	botListVersion    = "2"
	botProfileVersion = "1"
)

// profileNodeTag e' a tag do no <profile>, compartilhada pelo perfil business
// (escrita por GetBusinessProfile, lida por ParseBusinessProfile) e pelo
// perfil de bot (escrita por GetBotProfiles e por USync, lida na resposta).
const profileNodeTag = "profile"

// Perfil business.
const (
	businessProfileNodeTag = "business_profile"
	businessNodeTag        = "business"
	verifiedNameNodeTag    = "verified_name"
	// businessProfileVersion e' o valor do atributo `v` de <business_profile>,
	// a versao de esquema que o parser deste fork acompanha.
	businessProfileVersion = "244"

	businessHoursConfigTag = "business_hours_config"
	businessCategoryTag    = "category"
)

// Foto de perfil.
const (
	profilePictureQueryURL     = "url"
	profilePictureTypePreview  = "preview"
	profilePictureTypeImage    = "image"
	profilePictureTokenNodeTag = "tctoken"

	// Os dois status HTTP que o servidor devolve dentro do no <picture> em vez
	// de como erro de IQ.
	profilePictureStatusNotModified = 304
	profilePictureStatusNotSet      = 204
)

// Links de QR / mensagem business.
const (
	qrNodeTag = "qr"
	// qrTypeContact e as duas acoes sao o que GetContactQRLink escreve.
	qrTypeContact  = "contact"
	qrActionGet    = "get"
	qrActionRevoke = "revoke"
)

// Prefixos de link. A raiz os reexporta pelos nomes historicos
// (BusinessMessageLinkPrefix, ContactQRLinkPrefix, ...) por atribuicao.
const (
	BusinessMessageLinkPrefix       = "https://wa.me/message/"
	ContactQRLinkPrefix             = "https://wa.me/qr/"
	BusinessMessageLinkDirectPrefix = "https://api.whatsapp.com/message/"
	ContactQRLinkDirectPrefix       = "https://api.whatsapp.com/qr/"
)
