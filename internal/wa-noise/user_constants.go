// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

// Namespaces dos IQs de usuário.
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

// USync — o transporte de consulta em lote de informação de usuário.
//
// As tags são escritas por `usync` e lidas de volta na resposta (`usyncNodeTag`
// + `usyncListTag` em `GetOptionalChildByTag`, `usyncUserTag` no `!=` que filtra
// os filhos da lista em `IsOnWhatsApp`, `GetUserInfo`, `GetUserDevices` e
// `GetBotProfiles`). Como literais, um typo em qualquer um dos dois lados não é
// erro de compilação — a lista simplesmente sai vazia.
const (
	usyncNodeTag  = "usync"
	usyncQueryTag = "query"
	usyncListTag  = "list"
	usyncUserTag  = "user"

	usyncModeQuery = "query"
	usyncModeFull  = "full"

	usyncContextInteractive = "interactive"
	usyncContextBackground  = "background"
	usyncContextMessage     = "message"

	// usyncLastValue/usyncIndexValue: este fork nunca pagina a consulta usync —
	// manda tudo num lote só e declara ser a última página.
	usyncLastValue  = "true"
	usyncIndexValue = "0"
)

// Lista de dispositivos.
const (
	devicesNodeTag    = "devices"
	deviceListNodeTag = "device-list"
	deviceNodeTag     = "device"
	usersNodeTag      = "users"

	// deviceListVersion é o valor do atributo `version` de `<devices>`, que
	// seleciona o formato de resposta do servidor. O parser deste arquivo
	// (`parseDeviceList`) só entende o formato 2.
	deviceListVersion = "2"

	// fbIDDeviceChunkSize é o número de usuários por IQ `fbid:devices`.
	fbIDDeviceChunkSize = 15
)

// Contato / IsOnWhatsApp.
const (
	contactNodeTag = "contact"
	// contactTypeIn marca, na resposta de `IsOnWhatsApp`, que o número
	// consultado está na agenda do usuário.
	contactTypeIn = "in"

	lidNodeTag    = "lid"
	statusNodeTag = "status"

	pictureNodeTag  = "picture"
	picturesNodeTag = "pictures"
)

// Bots.
const (
	botSectionTag = "section"
	// botSectionTypeAll é a única seção de `GetBotListV2` cujos filhos viram
	// entradas da lista; o servidor manda outras seções (destaques etc.).
	botSectionTypeAll = "all"

	botCommandsTag = "commands"
	botCommandTag  = "command"
	botPromptsTag  = "prompts"
	botPromptTag   = "prompt"

	botListVersion    = "2"
	botProfileVersion = "1"
)

// profileNodeTag é a tag do nó `<profile>`, compartilhada pelo perfil business
// (escrita por `GetBusinessProfile`, lida por `parseBusinessProfile`) e pelo
// perfil de bot (escrita por `GetBotProfiles` e por `usync`, lida na resposta).
const profileNodeTag = "profile"

// Perfil business.
const (
	businessProfileNodeTag = "business_profile"
	businessNodeTag        = "business"
	verifiedNameNodeTag    = "verified_name"
	// businessProfileVersion é o valor do atributo `v` de `<business_profile>`,
	// a versão de esquema que o parser deste fork acompanha.
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

	// Os dois status HTTP que o servidor devolve dentro do nó `<picture>` em vez
	// de como erro de IQ.
	profilePictureStatusNotModified = 304
	profilePictureStatusNotSet      = 204
)

// Links de QR / mensagem business.
const (
	qrNodeTag = "qr"
	// qrTypeContact e as duas ações são o que `GetContactQRLink` escreve.
	qrTypeContact  = "contact"
	qrActionGet    = "get"
	qrActionRevoke = "revoke"
)
