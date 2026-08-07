// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package notification

import (
	waLog "wa-api/internal/wa-noise/observability/log"
)

// Transport e' a fatia do cliente de que os handlers de <notification> deste
// pacote precisam.
//
// Sao dois metodos, e nao e' por acaso: este pacote so' absorveu os handlers de
// notificacao que **parseiam um no e despacham um evento**, sem tocar em
// estado do cliente, em store, em socket ou em outro dominio. Os handlers que
// fazem mais que isso (encrypt, server_sync, account_sync, devices,
// fbid:devices, privacy_token e o proprio roteador handleNotification)
// continuam na raiz — ver a secao "O que NAO foi extraido, e por que" do lote
// 5 em PATCHES.md.
//
// Deliberadamente nao expoe nada do *whatsmeow.Client alem disso: e' o que
// permite que este pacote nao importe o pacote raiz e que os testes usem um
// duble em vez de um cliente com socket e sessao Noise.
type Transport interface {
	// Log e' o logger do cliente.
	Log() waLog.Logger
	// DispatchEvent entrega um evento aos handlers registrados.
	//
	// Nao devolve nada de proposito: nenhum dos pontos de despacho absorvidos
	// por este pacote consultava o handlerFailed do dispatchEvent da raiz
	// antes da extracao.
	DispatchEvent(evt any)
}
