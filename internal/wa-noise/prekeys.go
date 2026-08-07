// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	"go.mau.fi/libsignal/keys/prekey"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/capabilities/prekeys"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/security/keys"
)

// A logica deste dominio vive em internal/wa-noise/prekeys/. O que sobra aqui
// sao fachadas: elas guardam o contrato historico (nomes, assinaturas e o
// receptor *Client) e delegam. Ver PATCHES.md, "Fase F/G — lote 4".
const (
	// WantedPreKeyCount is the number of prekeys that the client should upload to the WhatsApp servers in a single batch.
	WantedPreKeyCount = prekeys.WantedCount
	// MinPreKeyCount is the number of prekeys when the client will upload a new batch of prekeys to the WhatsApp servers.
	MinPreKeyCount = prekeys.MinCount
)

// preKeyResp e' apelido de tipo, e nao um tipo novo, porque internals.go
// (gerado, fora do escopo deste lote) cita o nome antigo na assinatura de
// DangerousInternalClient.FetchPreKeys.
type preKeyResp = prekeys.Resp

func (cli *Client) getServerPreKeyCount(ctx context.Context) (int, error) {
	if cli == nil {
		return 0, ErrClientIsNil
	}
	return prekeys.GetServerCount(ctx, cli.preKeyT())
}

func (cli *Client) uploadPreKeys(ctx context.Context, initialUpload bool) {
	if cli == nil {
		return
	}
	prekeys.Upload(ctx, cli.preKeyT(), initialUpload)
}

func (cli *Client) fetchPreKeysNoError(ctx context.Context, retryDevices []types.JID) map[types.JID]*prekey.Bundle {
	if cli == nil {
		return nil
	}
	return prekeys.FetchNoError(ctx, cli.preKeyT(), retryDevices)
}

func (cli *Client) fetchPreKeys(ctx context.Context, users []types.JID) (map[types.JID]preKeyResp, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return prekeys.Fetch(ctx, cli.preKeyT(), users)
}

func preKeyToNode(key *keys.PreKey) waBinary.Node {
	return prekeys.ToNode(key)
}

func nodeToPreKeyBundle(deviceID uint32, node waBinary.Node) (*prekey.Bundle, error) {
	return prekeys.NodeToBundle(deviceID, node)
}
