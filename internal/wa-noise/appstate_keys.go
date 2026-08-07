// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	"wa-api/internal/wa-noise/appstate"
	"wa-api/internal/wa-noise/appstatesync"
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
