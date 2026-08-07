// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"time"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/tctoken"
	"wa-api/internal/wa-noise/protocol/types"
)

// A logica deste dominio vive em internal/wa-noise/tctoken/. O que sobra aqui
// sao fachadas: elas guardam o contrato historico (nomes, assinaturas e o
// receptor *Client) e delegam. Ver PATCHES.md, "Fase F/G — lote 4".

func shouldSendTCTokenInChatAction(jid types.JID) bool {
	return tctoken.ShouldSendInChatAction(jid)
}

func shouldSendNewTCToken(senderTimestamp time.Time) bool {
	return tctoken.ShouldSendNew(senderTimestamp)
}

func (cli *Client) resolveTCTokenStorageLID(ctx context.Context, jid types.JID) types.JID {
	if cli == nil {
		return jid.ToNonAD()
	}
	return tctoken.ResolveStorageLID(ctx, cli.tcTokenT(), jid)
}

// getTCTokenSenderTS reads the in-memory sender timestamp for a JID.
func (cli *Client) getTCTokenSenderTS(jid types.JID) time.Time {
	if cli == nil {
		return time.Time{}
	}
	return cli.tcToken.SenderTS(jid)
}

func (cli *Client) validateAndSetTCTokenSenderTS(jid types.JID, storedSenderTimestamp time.Time) bool {
	if cli == nil {
		return false
	}
	return cli.tcToken.ValidateAndSet(jid, storedSenderTimestamp)
}

// setTCTokenSenderTS writes the in-memory sender timestamp for a JID.
func (cli *Client) setTCTokenSenderTS(jid types.JID, ts time.Time) {
	if cli == nil {
		return
	}
	cli.tcToken.SetSenderTS(jid, ts)
}

// ensureTCToken returns a stored non-expired tctoken for the given JID, if available.
func (cli *Client) ensureTCToken(ctx context.Context, jid types.JID) ([]byte, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return tctoken.Ensure(ctx, cli.tcTokenT(), jid)
}

func (cli *Client) deleteExpiredPrivacyTokens() {
	if cli == nil {
		return
	}
	tctoken.DeleteExpired(cli.tcTokenT())
}

// issuePrivacyTokenAndSave is only called when a bucket boundary has been
// crossed since the last issuance.
func (cli *Client) issuePrivacyTokenAndSave(jid types.JID, senderTimestamp time.Time) {
	if cli == nil {
		return
	}
	tctoken.IssueAndSave(cli.tcTokenT(), jid, senderTimestamp)
}

// issuePrivacyToken sends an IQ to the server to issue a privacy token for the given JID.
func (cli *Client) issuePrivacyToken(ctx context.Context, jid types.JID, timestamp time.Time) (*waBinary.Node, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return tctoken.Issue(ctx, cli.tcTokenT(), jid, timestamp)
}
