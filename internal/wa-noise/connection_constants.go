// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"net/http"
	"time"
)

// Codigos e tipos de <stream:error> tratados por handleStreamError. Sao valores
// de protocolo definidos pelo servidor, nao ajustes locais: mudar qualquer um
// deles muda quais erros de stream o cliente reconhece, e o que ele deixa cair
// no ramo "Unknown stream error".
const (
	// streamErrorRestartRequired ("515") e' o pedido de reinicio de conexao que
	// o servidor manda logo apos o pareamento. Nao e' falha: o caminho correto
	// e' desconectar e reconectar.
	streamErrorRestartRequired = "515"

	// streamErrorServiceUnavailable ("503") e' o servidor avisando que vai
	// reiniciar. O auto-reconnect resolve sozinho.
	streamErrorServiceUnavailable = "503"

	// streamErrorConflictTag e' o filho <conflict> do <stream:error>, cujo
	// atributo "type" refina o motivo do 401.
	streamErrorConflictTag = "conflict"
)

// O codigo "401" e os dois tipos de <conflict> (conflictTypeDeviceRemoved,
// conflictTypeReplaced) ja' existem em request.go como streamErrorAuthCode e
// companhia, extraidos por um lote anterior da Fase E. handleStreamError
// reutiliza aqueles nomes em vez de criar um segundo nome para o mesmo valor.

// Politica de reconexao automatica.
const (
	// autoReconnectDelayStep e' o passo do backoff linear do autoReconnect: a
	// espera antes da n-esima tentativa e' n * autoReconnectDelayStep.
	autoReconnectDelayStep = 2 * time.Second
)

// retryableConnectStatusCodes sao os status HTTP do handshake do websocket que
// isRetryableConnectError considera transitorios — vale a pena reconectar em
// background em vez de devolver erro ao chamador de ConnectContext. Qualquer
// outro status e' tratado como permanente (ex: 403, que e' banimento).
var retryableConnectStatusCodes = map[int]struct{}{
	http.StatusRequestTimeout:      {}, // 408
	http.StatusInternalServerError: {}, // 500
	http.StatusNotImplemented:      {}, // 501
	http.StatusBadGateway:          {}, // 502
	http.StatusServiceUnavailable:  {}, // 503
	http.StatusGatewayTimeout:      {}, // 504
}

// Temporizacao do loop que consome a fila de nos recebidos (handlerQueueLoop).
// Nenhum destes valores muda o resultado do tratamento — so' quando o cliente
// reclama de lentidao e quando desiste de esperar em primeiro plano.
const (
	// handlerQueueSlowNodeThreshold e' a partir de quanto tempo o tratamento de
	// um no ja' concluido vira um aviso no log.
	handlerQueueSlowNodeThreshold = 5 * time.Second

	// handlerQueueSlowNodeWarnInterval e' de quanto em quanto tempo o loop
	// avisa que um no ainda esta' sendo tratado.
	handlerQueueSlowNodeWarnInterval = 30 * time.Second

	// handlerQueueSlowNodeMaxWarnings e' quantos avisos o loop emite antes de
	// deixar o no terminar em segundo plano e seguir para o proximo. O tempo
	// total de espera e' handlerQueueSlowNodeMaxWarnings *
	// handlerQueueSlowNodeWarnInterval.
	handlerQueueSlowNodeMaxWarnings = 10
)

// Dimensionamento de buffers e identificadores do Client.
const (
	// uniqueIDPrefixLength e' quantos bytes aleatorios formam o prefixo de
	// Client.uniqueID, usado para que dois processos sobre o mesmo device nao
	// gerem os mesmos IDs de requisicao.
	uniqueIDPrefixLength = 2

	// historySyncNotificationBufferSize e' o buffer do canal que leva as
	// notificacoes de history sync ate' o loop consumidor. Ver F44 em
	// HOUSEKEEP.md: o envio para este canal e' bloqueante de proposito.
	historySyncNotificationBufferSize = 32

	// initialEventHandlerCapacity e' a capacidade inicial da lista de handlers
	// de evento. A maioria dos consumidores registra exatamente um.
	initialEventHandlerCapacity = 1
)

// Os tamanhos fixos do handshake Noise (noiseKeyLength, certSignatureLength)
// mudaram para internal/wa-noise/handshake, junto com o proprio handshake, no
// lote 10 da Fase F/G. Nenhum arquivo da raiz os le mais.

// Os parametros do dialer SOCKS5 (socksProxyDialTimeout, socksProxyKeepAlive)
// mudaram para internal/wa-noise/proxyconf no lote 10 da Fase F/G, junto com a
// montagem dos transports. Nenhum arquivo da raiz os le mais.
