// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

// Constantes dos varios tokens que acompanham mensagens enviadas:
// tctoken (trusted contact), cstoken (contact safety) e reporting token.

// Reporting token: HMAC truncado sobre um subconjunto canonico dos campos da
// mensagem, usado pelo servidor para correlacionar denuncias.
const (
	// reportingTokenNodeTag e reportingTokenChildTag sao as tags do no enviado.
	reportingTokenNodeTag  = "reporting"
	reportingTokenChildTag = "reporting_token"
	// reportingTokenVersionAttr / reportingTokenVersion identificam o esquema
	// de derivacao do token; o servidor recusa versoes que nao conhece.
	reportingTokenVersionAttr = "v"
	reportingTokenVersion     = "2"
	// reportingTokenLength e quantos bytes do HMAC-SHA256 vao no wire.
	reportingTokenLength = 16
)

// Wire types do protobuf, usados pelo extrator do reporting token para andar
// pelos campos da mensagem serializada sem desserializa-la.
const (
	wireVarint = 0
	wire64bit  = 1
	wireBytes  = 2
	wire32bit  = 5

	// wireTypeMask e wireFieldNumShift decompoem a tag varint de um campo:
	// os 3 bits baixos sao o wire type, o resto e o numero do campo.
	wireTypeMask      = 0x7
	wireFieldNumShift = 3

	// Tamanhos fixos dos wire types de largura fixa.
	wire64bitLength = 8
	wire32bitLength = 4
)

// A constante do atributo "type" do no <token> de trusted contact foi para
// internal/wa-noise/tctoken/ na Fase F/G lote 4, junto com o resto do dominio.

// pushMsgIDEncKeyLength e o tamanho da chave que cifra os message IDs nas
// notificacoes push da APNs.
const pushMsgIDEncKeyLength = 32
