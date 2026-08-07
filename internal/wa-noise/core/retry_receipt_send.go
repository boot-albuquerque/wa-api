package wanoise

import (
	"context"

	"wa-api/internal/wa-noise/capabilities/retry"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
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
