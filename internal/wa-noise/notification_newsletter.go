// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/notification"
	"wa-api/internal/wa-noise/types"
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
