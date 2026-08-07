package whatsmeow

import (
	"context"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/capabilities/retry"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// Fachadas do dominio de retry. A logica vive em internal/wa-noise/retry
// (Fase F/G, lote 5); aqui ficam so' as delegacoes que preservam as
// assinaturas usadas pelo caminho de recibo e por DangerousInternalClient.

// incomingRetryKey e' apelido de tipo, e nao tipo novo, porque internals.go
// (gerado, F29, fora do escopo) nao pode ser tocado.
type incomingRetryKey = retry.IncomingKey

func (cli *Client) shouldRecreateSession(ctx context.Context, retryCount int, jid types.JID) (reason string, recreate bool) {
	if cli == nil {
		return "", false
	}
	return retry.ShouldRecreateSession(ctx, cli.retryT(), retryCount, jid)
}

func (cli *Client) tryHandleRetryReceipt(ctx context.Context, receipt *events.Receipt, node *waBinary.Node) {
	if cli == nil {
		return
	}
	retry.TryHandleReceipt(ctx, cli.retryT(), receipt, node)
}

// handleRetryReceipt handles an incoming retry receipt for an outgoing message.
func (cli *Client) handleRetryReceipt(ctx context.Context, receipt *events.Receipt, node *waBinary.Node) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return retry.HandleReceipt(ctx, cli.retryT(), receipt, node)
}

// SetMaxParallelRetryReceiptHandling sets how many retry receipts can be handled in parallel.
// Defaults to unlimited. This should only be set before connecting, changing it afterwards can cause data races.
func (cli *Client) SetMaxParallelRetryReceiptHandling(n int64) {
	if cli == nil {
		return
	}
	cli.retryState.SetMaxParallel(n)
}
