// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package pairing

import (
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/util/keys"
)

// LinkingCache guarda a sessao de pareamento por codigo de telefone que esta'
// pendente: o par de chaves efemero gerado por PairPhone, o codigo mostrado ao
// usuario e a referencia devolvida pelo servidor. HandleCodeNotification
// precisa dos tres para fechar o pareamento.
//
// Era o tipo nao exportado phoneLinkingCache, em pair-code.go na raiz.
type LinkingCache struct {
	JID         types.JID
	KeyPair     *keys.KeyPair
	LinkingCode string
	PairingRef  string
}

// State e' o estado mutavel do pareamento. Substitui o campo solto
// `phoneLinkingCache *phoneLinkingCache` de *Client.
//
// ATENCAO — sem sincronizacao, de proposito. O campo original tambem nao tinha
// nenhuma: PairPhone escreve nele e HandleCodeNotification (chamado de um
// handler de notificacao, em outra goroutine) le'. Essa corrida e'
// PRE-EXISTENTE e vem do upstream; este lote e' extracao, e acrescentar um
// mutex aqui mudaria comportamento sob concorrencia em codigo de pareamento
// (criptografia de dispositivo). Registrada em HOUSEKEEP.md em vez de
// corrigida de graga.
//
// O zero value e' usavel.
type State struct {
	linking *LinkingCache
}

// Linking devolve a sessao de pareamento por codigo pendente, ou nil.
func (s *State) Linking() *LinkingCache { return s.linking }

// SetLinking registra a sessao de pareamento por codigo pendente.
func (s *State) SetLinking(c *LinkingCache) { s.linking = c }
