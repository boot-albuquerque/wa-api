// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/notification"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/tctoken"
)

// handleBlocklist e' fachada: a logica vive em internal/wa-noise/notification
// (Fase F/G, lote 5).
func (cli *Client) handleBlocklist(ctx context.Context, node *waBinary.Node) {
	if cli == nil {
		return
	}
	notification.HandleBlocklist(cli.notifT(), node)
}

// handlePrivacyTokenNotification NAO foi extraido junto: ele grava no
// Store.PrivacyTokens e depende de resolveTCTokenStorageLID, ou seja, e' o
// dominio de tctoken chegando por uma notificacao, nao parsing de notificacao.
// Ver PATCHES.md, lote 5, "O que NAO foi extraido, e por que".

func (cli *Client) handlePrivacyTokenNotification(ctx context.Context, node *waBinary.Node) {
	if cli.getOwnID().IsEmpty() {
		cli.Log.Debugf("Ignoring privacy token notification, session was deleted")
		return
	}
	tokens := node.GetChildByTag("tokens")
	if tokens.Tag != "tokens" {
		cli.Log.Warnf("privacy_token notification didn't contain <tokens> tag")
		return
	}
	parentAG := node.AttrGetter()
	sender := parentAG.JID("from").ToNonAD()
	senderLID := parentAG.OptionalJIDOrEmpty("sender_lid").ToNonAD()
	if senderLID.IsEmpty() {
		senderLID = cli.resolveTCTokenStorageLID(ctx, sender)
	}
	if !parentAG.OK() {
		cli.Log.Warnf("privacy_token notification didn't have a sender (%v)", parentAG.Error())
		return
	}
	for _, child := range tokens.GetChildren() {
		ag := child.AttrGetter()
		if child.Tag != "token" {
			cli.Log.Warnf("privacy_token notification contained unexpected <%s> tag", child.Tag)
			continue
		}
		if tokenType := ag.String("type"); tokenType != tctoken.TokenType {
			cli.Log.Warnf("privacy_token notification contained unexpected token type %s", tokenType)
			continue
		}
		token, ok := child.Content.([]byte)
		if !ok {
			cli.Log.Warnf("privacy_token notification contained non-binary token")
			continue
		}
		timestamp := ag.UnixTime("t")
		if !ag.OK() {
			cli.Log.Warnf("privacy_token notification is missing some fields: %v", ag.Error())
		}
		err := cli.Store.PrivacyTokens.PutPrivacyTokens(ctx, store.PrivacyToken{
			User:      senderLID,
			Token:     token,
			Timestamp: timestamp,
		})
		if err != nil {
			cli.Log.Errorf("Failed to save privacy token from %s: %v", senderLID, err)
		} else {
			cli.Log.Debugf("Received privacy token from %s (ts: %v)", senderLID, timestamp)
		}
	}
}
