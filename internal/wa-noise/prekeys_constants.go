// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import "time"

// Politica de upload de prekeys.
const (
	// initialPreKeyCount e quantas prekeys sao enviadas no primeiro upload apos
	// o pareamento. E deliberadamente muito maior que WantedPreKeyCount para
	// que a conta nasca com estoque suficiente para varias sessoes novas.
	initialPreKeyCount = 812

	// preKeyUploadDebounce e a janela em que um segundo pedido de upload
	// reconfere a contagem no servidor antes de gerar chaves de novo, para
	// evitar a corrida de dois uploads simultaneos.
	preKeyUploadDebounce = 10 * time.Minute
)

// Tamanhos de wire das chaves do protocolo Signal, conforme o servidor os envia
// e espera. Toda leitura de no de prekey valida contra estes valores.
const (
	// preKeyRegistrationIDLength e o registration ID em big-endian (uint32).
	preKeyRegistrationIDLength = 4

	// preKeyIDLength e o key ID no wire: 3 bytes, ou seja um uint32 truncado
	// para 24 bits. preKeyToNode corta o byte mais significativo ao escrever e
	// nodeToPreKey o reintroduz zerado ao ler.
	preKeyIDLength = 3
	// preKeyIDPadLength e quantos bytes zerados nodeToPreKey prefixa para
	// remontar o uint32 de 4 bytes a partir dos preKeyIDLength do wire.
	preKeyIDPadLength = preKeyRegistrationIDLength - preKeyIDLength

	// preKeyPubLength e o tamanho de uma chave publica Curve25519.
	preKeyPubLength = 32
	// preKeySignatureLength e o tamanho de uma assinatura Ed25519 da signed prekey.
	preKeySignatureLength = 64
)
