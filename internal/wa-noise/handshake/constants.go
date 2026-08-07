// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package handshake implementa o handshake Noise_XX_25519_AESGCM_SHA256 da API
// web do WhatsApp e a verificacao da cadeia de certificados do servidor.
//
// O pacote nao conhece o *Client da raiz: Do recebe um *socket.FrameSocket ja'
// conectado mais um Config com o material de chave e os dois handlers, e
// devolve o *socket.NoiseSocket resultante. Quem guarda esse socket — sob
// socketLock — continua sendo a raiz (ver PATCHES.md, "Fase F/G — lote 10").
package handshake

import "time"

// ResponseTimeout e' quanto tempo Do espera pelo <ServerHello> antes de
// desistir. Era whatsmeow.NoiseHandshakeResponseTimeout.
const ResponseTimeout = 20 * time.Second

// WACertIssuerSerial e' o serial esperado do emissor do certificado
// intermediario da cadeia do servidor.
const WACertIssuerSerial = 0

// Tamanhos fixos do handshake Noise. Sao definidos pelas primitivas usadas
// (Curve25519 e Ed25519 sobre a curva Djb), nao escolhas nossas — as conversoes
// de fatia para array em handshake.go/cert.go so' sao seguras porque estes
// comprimentos sao conferidos antes.
const (
	// NoiseKeyLength e' o tamanho de uma chave publica/privada Curve25519.
	NoiseKeyLength = 32

	// CertSignatureLength e' o tamanho de uma assinatura da cadeia de
	// certificados do servidor.
	CertSignatureLength = 64
)

// WACertPubKey e' a chave publica raiz com que a assinatura do certificado
// intermediario do servidor e' conferida. E' dado de protocolo, fixado pelo
// WhatsApp.
var WACertPubKey = [...]byte{0x14, 0x23, 0x75, 0x57, 0x4d, 0xa, 0x58, 0x71, 0x66, 0xaa, 0xe7, 0x1e, 0xbe, 0x51, 0x64, 0x37, 0xc4, 0xa2, 0x8b, 0x73, 0xe3, 0x69, 0x5c, 0x6c, 0xe1, 0xf7, 0xf9, 0x54, 0x5d, 0xa8, 0xee, 0x6b}
