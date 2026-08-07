// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import "time"

// Parametros de geracao de ID de mensagem (message_id.go).
//
// O ID "web" e' `WebMessageIDPrefix` seguido do SHA-256, em hex maiusculo, de
// timestamp || jid || aleatorio. Os tamanhos abaixo sao formato de fio: mudar
// qualquer um deles muda o ID que o servidor ve.
const (
	// webMessageIDTimestampLength e' o numero de bytes do unix time big-endian
	// que abre o material do hash.
	webMessageIDTimestampLength = 8
	// webMessageIDRandomLength e' quanto material aleatorio entra no hash.
	webMessageIDRandomLength = 16
	// webMessageIDHashLength e' quantos bytes do digest SHA-256 viram o sufixo
	// hex do ID (9 bytes => 18 caracteres hex).
	webMessageIDHashLength = 9
	// webMessageIDJIDSuffix e' o sufixo de servidor legado concatenado ao user
	// do proprio JID dentro do material do hash. E' `@c.us` (e nao
	// `@s.whatsapp.net`) porque e' o que o WhatsApp Web usa.
	webMessageIDJIDSuffix = "@c.us"
	// legacyMessageIDRandomLength e' o tamanho do aleatorio da funcao
	// GenerateMessageID depreciada, que nao passa por hash.
	legacyMessageIDRandomLength = 8
)

// facebookMessageIDRandomBits e' quantos bits baixos do ID de mensagem do
// Messenger sao aleatorios; os bits acima disso sao o unix time em
// milissegundos deslocado para a esquerda.
const facebookMessageIDRandomBits = 22

// historySyncLoopIdleTimeout e' quanto tempo handleHistorySyncNotificationLoop
// espera por uma nova notificacao antes de encerrar. Ao encerrar ele zera
// historySyncHandlerStarted, e o proximo protocol message religa o loop.
const historySyncLoopIdleTimeout = 1 * time.Minute

// decryptedBufferClearInterval e' o intervalo minimo entre duas limpezas de
// hashes antigos do buffer de eventos decriptados (EnableDecryptedEventBuffer).
// Nao e' um agendamento: a limpeza so' e' disparada por uma mensagem decriptada
// com sucesso depois desse intervalo.
const decryptedBufferClearInterval = 12 * time.Hour

// Parametros das chaves derivadas de message secret (msgsecret*.go).
const (
	// msgSecretKeyLength e' o tamanho em bytes da chave AES-GCM derivada por
	// HKDF-SHA256 a partir do message secret original, tanto em
	// generateMsgSecretKey quanto em applyBotMessageHKDF.
	msgSecretKeyLength = 32
	// msgSecretIVSize e' o tamanho do nonce AES-GCM usado por encryptMsgSecret
	// (96 bits, o padrao do modo).
	msgSecretIVSize = 12
)

// encTypeMsgSecret e' o valor do atributo `type` do <enc> de mensagem de bot,
// cujo conteudo e' um waE2E.MessageSecretMessage e nao um ciphertext Signal.
// Ao contrario de encTypeMsg/encTypePreKeyMsg/encTypeSenderKey
// (send_constants.go), este so' aparece no caminho de recepcao.
const encTypeMsgSecret = "msmsg"

// Separadores de dominio do hash de ciphertext usado como chave do buffer de
// eventos decriptados (bufferedDecrypt, message_decrypt_session.go).
//
// ATENCAO: estes valores sao entrada de hash persistida. Mudar qualquer um
// deles invalida silenciosamente todo o buffer ja' gravado — as entradas
// antigas viram inalcancaveis e mensagens ja' processadas poderiam ser
// reprocessadas. Sao constantes justamente para que ninguem os altere por
// engano.
const (
	ciphertextHashDomainPreKey    = "prekey"
	ciphertextHashDomainNormal    = "normal"
	ciphertextHashDomainSenderKey = "senderkey"
)
