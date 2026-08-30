package core

import (
	"context"

	"go.mau.fi/libsignal/keys/prekey"

	"wa-api/internal/noise/capabilities/prekeys"
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/security/keys"
)

// A logica deste dominio vive em internal/noise/prekeys/. O que sobra aqui
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
