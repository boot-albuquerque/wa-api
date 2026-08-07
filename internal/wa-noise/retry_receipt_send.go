// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/retry"
	"wa-api/internal/wa-noise/protocol/types"
)

// sendRetryReceipt sends a retry receipt for an incoming message.
//
// Fachada: a logica vive em internal/wa-noise/retry (Fase F/G, lote 5).
func (cli *Client) sendRetryReceipt(ctx context.Context, node *waBinary.Node, info *types.MessageInfo, forceIncludeIdentity bool) {
	if cli == nil {
		return
	}
	retry.SendReceipt(ctx, cli.retryT(), node, retryMessageRef(info), forceIncludeIdentity)
}
