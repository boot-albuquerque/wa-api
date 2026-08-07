// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waMsgApplication"
	"wa-api/internal/wa-noise/retry"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

// Fachadas do cache de mensagens recentes. A logica e o estado vivem em
// internal/wa-noise/retry (Fase F/G, lote 5).

// RecentMessage e recentMessageKey sao APELIDOS de tipo, e nao tipos novos,
// porque internals.go (gerado, F29, fora do escopo) cita RecentMessage na
// assinatura de dois metodos de DangerousInternalClient e precisa compilar sem
// ser tocado.
//
// Os campos do tipo passaram de wa/fb para WA/FB. Isso e' alargamento da
// superficie de internals.go (o tipo antes so' tinha campos nao exportados,
// logo era inutilizavel de fora), nao quebra.
type (
	RecentMessage    = retry.RecentMessage
	recentMessageKey = retry.RecentKey
)

func (cli *Client) addRecentMessage(ctx context.Context, to types.JID, id types.MessageID, wa *waE2E.Message, fb *waMsgApplication.MessageApplication) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return retry.AddRecent(ctx, cli.retryT(), to, id, wa, fb)
}

func (cli *Client) getRecentMessage(to types.JID, id types.MessageID) RecentMessage {
	if cli == nil {
		return RecentMessage{}
	}
	return retry.GetRecent(cli.retryT(), to, id)
}

func (cli *Client) getMessageForRetry(ctx context.Context, receipt *events.Receipt, messageID types.MessageID) (*RecentMessage, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return retry.GetForRetry(ctx, cli.retryT(), receipt, messageID)
}

func parseRecentMessage(format string, buf []byte) (*RecentMessage, error) {
	return retry.ParseRecent(format, buf)
}
