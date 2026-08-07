// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package prekeys

import (
	"context"
	"fmt"

	"go.mau.fi/libsignal/keys/prekey"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// Resp e' o resultado da busca de prekey de um unico dispositivo: ou o bundle,
// ou o erro de parsing daquele <user>. Era preKeyResp na raiz, e continua
// visivel de la' por apelido de tipo, porque internals.go (gerado) cita o nome
// antigo numa assinatura.
type Resp struct {
	Bundle *prekey.Bundle
	Err    error
}

// FetchNoError busca prekeys e devolve so' os bundles que deram certo, logando
// os que falharam. Devolve nil quando a lista de dispositivos e' vazia — sem
// tocar no socket.
func FetchNoError(ctx context.Context, t Transport, retryDevices []types.JID) map[types.JID]*prekey.Bundle {
	if len(retryDevices) == 0 {
		return nil
	}
	bundlesResp, err := Fetch(ctx, t, retryDevices)
	if err != nil {
		t.Log().Warnf("Failed to fetch prekeys for %v with no existing session: %v", retryDevices, err)
		return nil
	}
	bundles := make(map[types.JID]*prekey.Bundle, len(retryDevices))
	for _, jid := range retryDevices {
		resp := bundlesResp[jid]
		if resp.Err != nil {
			t.Log().Warnf("Failed to fetch prekey for %s: %v", jid, resp.Err)
			continue
		}
		bundles[jid] = resp.Bundle
	}
	return bundles
}

// Fetch pede ao servidor as prekeys dos dispositivos dados.
//
// O erro devolvido e' o da requisicao inteira; os erros por dispositivo vao no
// campo Err de cada Resp.
func Fetch(ctx context.Context, t Transport, users []types.JID) (map[types.JID]Resp, error) {
	requests := make([]waBinary.Node, len(users))
	for i, user := range users {
		requests[i].Tag = "user"
		requests[i].Attrs = waBinary.Attrs{
			"jid":    user,
			"reason": "identity",
		}
	}
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: "encrypt",
		Type:      IQGet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag:     "key",
			Content: requests,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send prekey request: %w", err)
	} else if len(resp.GetChildren()) == 0 {
		return nil, fmt.Errorf("got empty response to prekey request")
	}
	list := resp.GetChildByTag("list")
	respData := make(map[types.JID]Resp)
	for _, child := range list.GetChildren() {
		if child.Tag != "user" {
			continue
		}
		jid := child.AttrGetter().JID("jid")
		bundle, err := NodeToBundle(uint32(jid.Device), child)
		respData[jid] = Resp{bundle, err}
	}
	return respData, nil
}
