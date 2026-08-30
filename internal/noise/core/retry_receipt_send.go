package core

import (
	"context"

	"wa-api/internal/noise/capabilities/retry"
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
)

// sendRetryReceipt sends a retry receipt for an incoming message.
//
// Fachada: a logica vive em internal/noise/retry (Fase F/G, lote 5).
func (cli *Client) sendRetryReceipt(ctx context.Context, node *waBinary.Node, info *types.MessageInfo, forceIncludeIdentity bool) {
	if cli == nil {
		return
	}
	retry.SendReceipt(ctx, cli.retryT(), node, retryMessageRef(info), forceIncludeIdentity)
}
