// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package notification

// Taxonomia do atributo `type` do no <notification>. Diferente das tags e
// atributos usados uma unica vez no ponto onde o no e' lido — que por
// convencao dos lotes 1-4 continuam literais — estes sao a tabela de
// roteamento do handleNotification da raiz: o conjunto fechado de dominios que
// o servidor pode nos empurrar. Nomea-los deixa a lista legivel como taxonomia
// e impede que um handler novo entre com a string escrita a mao errada.
//
// Tipos que o upstream documenta mas nao trata (business, disappearing_mode,
// server, pay, psa) nao entram aqui de proposito: sem handler, a constante
// seria codigo morto.
//
// As constantes vivem neste pacote e nao na raiz mesmo o roteador tendo ficado
// na raiz: elas sao a taxonomia do dominio, e o pacote que a define e' o que
// se chama notification. A raiz cita Type* diretamente no switch.
const (
	TypeEncrypt              = "encrypt"
	TypeServerSync           = "server_sync"
	TypeAccountSync          = "account_sync"
	TypeDevices              = "devices"
	TypeFBIDDevices          = "fbid:devices"
	TypeGroup                = "w:gp2"
	TypePicture              = "picture"
	TypeMediaRetry           = "mediaretry"
	TypePrivacyToken         = "privacy_token"
	TypeLinkCodeCompanionReg = "link_code_companion_reg"
	TypeNewsletter           = "newsletter"
	TypeMex                  = "mex"
	TypeStatus               = "status"
)
