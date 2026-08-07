// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import "time"

// Constantes do fluxo de pareamento (QR e codigo de telefone) e do canal de QR.
const (
	// pairQRDataFormat monta o payload do QR code lido pelo celular:
	// ref, chave Noise publica, chave de identidade publica, adv secret e tipo de cliente.
	pairQRDataFormat = "https://wa.me/settings/linked_devices#%s,%s,%s,%s,%s"

	// pairMainDeviceID e o ID de dispositivo do aparelho principal (o celular)
	// dentro de um LID: o companion sempre tem device != 0.
	pairMainDeviceID = 0

	// pairIdentityKeyLength e o tamanho de uma chave publica Curve25519 de identidade.
	pairIdentityKeyLength = 32
)

// Codigos e textos do no <error> devolvido ao servidor quando o pareamento falha.
// Os codigos espelham status HTTP por convencao do protocolo do WhatsApp.
const (
	pairErrCodeInternal     = 500
	pairErrCodeUnauthorized = 401

	pairErrTextInternal          = "internal-error"
	pairErrTextHMACMismatch      = "hmac-mismatch"
	pairErrTextSignatureMismatch = "signature-mismatch"
)

// Parametros criptograficos do pareamento por codigo de telefone (link code).
const (
	// pairCodePBKDF2Iterations e o custo do PBKDF2 que deriva a chave de cifragem
	// a partir do codigo de 8 caracteres. Escrito como 2<<16 no upstream.
	pairCodePBKDF2Iterations = 2 << 16

	// pairCodeSaltLength, pairCodeIVLength e pairCodeKeyLength descrevem o blob
	// de 80 bytes trocado nos dois sentidos: salt || IV || pubkey cifrada.
	pairCodeSaltLength = 32
	pairCodeIVLength   = 16
	pairCodeKeyLength  = 32

	// Offsets derivados dos comprimentos acima, para que o fatiamento do blob
	// continue sendo [0:32], [32:48], [48:80] sem literal solto.
	pairCodeSaltEnd       = pairCodeSaltLength
	pairCodeIVEnd         = pairCodeSaltEnd + pairCodeIVLength
	pairCodeWrappedKeyEnd = pairCodeIVEnd + pairCodeKeyLength

	// pairCodeRawLength e o numero de bytes aleatorios que viram o codigo de
	// pareamento; 5 bytes = 8 caracteres no base32 customizado do WhatsApp.
	pairCodeRawLength = 5
	// pairCodeGroupLength e onde o codigo exibido ao usuario recebe o hifen.
	pairCodeGroupLength = 4

	// pairCodeAdvSecretRandomLength e pairCodeKeyBundleSaltLength sao entradas
	// aleatorias do bundle de chaves enviado ao aparelho principal.
	pairCodeAdvSecretRandomLength = 32
	pairCodeKeyBundleSaltLength   = 32
	// pairCodeKeyBundleNonceLength e o nonce do AES-GCM que cifra o bundle.
	pairCodeKeyBundleNonceLength = 12
	// pairCodeKeyBundleKeyLength e o tamanho da chave AES derivada por HKDF.
	pairCodeKeyBundleKeyLength = 32
	// pairCodeAdvSecretLength e o tamanho do adv secret derivado por HKDF.
	pairCodeAdvSecretLength = 32

	// pairCodePhoneMinLength e o menor comprimento aceito para o numero de
	// telefone ja limpo de nao-digitos (o upstream recusa `len <= 6`).
	pairCodePhoneMinLength = 7
	// pairCodePhoneTrunkPrefix indica numero nacional, nao internacional.
	pairCodePhoneTrunkPrefix = "0"
)

// Rotulos HKDF do pareamento por codigo. Sao strings de dominio do protocolo:
// mudar qualquer byte quebra a compatibilidade com o aparelho principal.
const (
	pairCodeKeyBundleHKDFInfo = "link_code_pairing_key_bundle_encryption_key"
	pairCodeAdvSecretHKDFInfo = "adv_secret"
)

// pairCodeBase32Alphabet e o alfabeto base32 customizado do WhatsApp para o
// codigo de pareamento: sem 0/O, 1/I e U, para evitar leitura ambigua.
const pairCodeBase32Alphabet = "123456789ABCDEFGHJKLMNPQRSTVWXYZ"

// Politica de emissao de QR codes por GetQRChannel.
const (
	// qrChannelBuffer e a capacidade do canal devolvido ao chamador.
	qrChannelBuffer = 8
	// qrCodeTimeout e a validade de cada QR code apos o primeiro.
	qrCodeTimeout = 20 * time.Second
	// qrCodeFirstTimeout e a validade do primeiro QR code, identificado por
	// ainda haver qrCodeFirstBatchSize codigos na fila.
	qrCodeFirstTimeout   = 60 * time.Second
	qrCodeFirstBatchSize = 6
)

// Nomes dos eventos emitidos no QRChannelItem.Event.
const (
	QRChannelEventCode  = "code"
	QRChannelEventError = "error"

	qrChannelEventSuccess                   = "success"
	qrChannelEventTimeout                   = "timeout"
	qrChannelEventUnexpectedState           = "err-unexpected-state"
	qrChannelEventClientOutdated            = "err-client-outdated"
	qrChannelEventScannedWithoutMultidevice = "err-scanned-without-multidevice"
)
