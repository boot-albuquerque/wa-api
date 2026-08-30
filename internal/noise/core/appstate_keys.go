package core

import (
	"context"

	"wa-api/internal/noise/capabilities/appstatesync"
	"wa-api/internal/noise/protocol/appstate"
)

// Fachada do pedido de chaves de app state ao dispositivo primario. A logica
// vive em internal/noise/appstatesync/keys.go.

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
