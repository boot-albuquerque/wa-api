package wanoise

import (
	"context"

	"wa-api/internal/wa-noise/capabilities/appstatesync"
	"wa-api/internal/wa-noise/protocol/appstate"
)

// Fachada do pedido de chaves de app state ao dispositivo primario. A logica
// vive em internal/wa-noise/appstatesync/keys.go.

func (cli *Client) requestMissingAppStateKeys(ctx context.Context, patches *appstate.PatchList) {
	if cli == nil {
		return
	}
	appstatesync.RequestMissingKeys(ctx, cli.appStateT(), patches)
}

func (cli *Client) requestAppStateKeys(ctx context.Context, rawKeyIDs [][]byte) {
	if cli == nil {
		return
	}
	appstatesync.RequestKeys(ctx, cli.appStateT(), rawKeyIDs)
}
