package wanoise

import (
	"context"

	"wa-api/internal/wa-noise/capabilities/notification"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// Fachadas do dominio de notificacao de newsletter. A logica vive em
// internal/wa-noise/notification (Fase F/G, lote 5); aqui ficam so' as
// delegacoes que preservam as assinaturas usadas por handleNotification, por
// newsletter_transport.go e por DangerousInternalClient.

func (cli *Client) parseNewsletterMessages(node *waBinary.Node) []*types.NewsletterMessage {
	if cli == nil {
		return nil
	}
	return notification.ParseNewsletterMessages(cli.notifT(), node)
}

func (cli *Client) handleNewsletterNotification(ctx context.Context, node *waBinary.Node) {
	if cli == nil {
		return
	}
	notification.HandleNewsletter(cli.notifT(), node)
}

func (cli *Client) handleMexNotification(ctx context.Context, node *waBinary.Node) {
	if cli == nil {
		return
	}
	notification.HandleMex(cli.notifT(), node)
}
