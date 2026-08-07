// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

// Taxonomia do atributo `type` do no <notification>. Diferente das tags e
// atributos usados uma unica vez no ponto onde o no e' lido — que por
// convencao dos lotes 1-4 continuam literais — estes sao a tabela de
// roteamento de handleNotification: o conjunto fechado de dominios que o
// servidor pode nos empurrar. Nomea-los deixa a lista legivel como taxonomia e
// impede que um handler novo entre com a string escrita a mao errada.
//
// Tipos que o upstream documenta mas nao trata (business, disappearing_mode,
// server, pay, psa) nao entram aqui de proposito: sem handler, a constante
// seria codigo morto.
const (
	notificationTypeEncrypt              = "encrypt"
	notificationTypeServerSync           = "server_sync"
	notificationTypeAccountSync          = "account_sync"
	notificationTypeDevices              = "devices"
	notificationTypeFBIDDevices          = "fbid:devices"
	notificationTypeGroup                = "w:gp2"
	notificationTypePicture              = "picture"
	notificationTypeMediaRetry           = "mediaretry"
	notificationTypePrivacyToken         = "privacy_token"
	notificationTypeLinkCodeCompanionReg = "link_code_companion_reg"
	notificationTypeNewsletter           = "newsletter"
	notificationTypeMex                  = "mex"
	notificationTypeStatus               = "status"
)
