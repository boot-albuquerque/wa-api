// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Este arquivo e o decrypt_loop.go/decrypt_session.go sao o caminho de
// decifragem Signal de TODA mensagem que entra. Foram extraidos de forma
// MECANICA: cada `cli.X` virou `t.X` (ou a free function correspondente) e nada
// mais. Nem a ordem das operacoes, nem o tratamento de erro, nem quais erros
// sao ignorados em silencio, nem quando o ack sai. Ver PATCHES.md, lote 9.
package message

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// HandleEncrypted e' a porta de entrada de um stanza <message> vindo do socket.
func HandleEncrypted(ctx context.Context, t Transport, node *waBinary.Node) {
	info, err := ParseInfo(t, node)
	if err != nil {
		t.Log().Warnf("Failed to parse message: %v", err)
	} else {
		if !info.SenderAlt.IsEmpty() {
			t.StoreLIDPNMapping(ctx, info.SenderAlt, info.Sender)
		} else if !info.RecipientAlt.IsEmpty() {
			t.StoreLIDPNMapping(ctx, info.RecipientAlt, info.Chat)
		}
		if info.VerifiedName != nil && len(info.VerifiedName.Details.GetVerifiedName()) > 0 {
			go t.UpdateBusinessName(ctx, info.Sender, info.SenderAlt, info, info.VerifiedName.Details.GetVerifiedName())
		}
		if len(info.PushName) > 0 && info.PushName != "-" && (!t.IsMessenger() || info.PushName != "username") {
			go t.UpdatePushName(ctx, info.Sender, info.SenderAlt, info, info.PushName)
		}
		if info.Sender.Server == types.NewsletterServer {
			var cancelled bool
			defer t.MaybeDeferredAck(ctx, node)(&cancelled)
			cancelled = HandlePlaintext(ctx, t, info, node)
		} else {
			DecryptMessages(ctx, t, info, node)
		}
	}
}

// HandlePlaintext trata a mensagem de newsletter, que vem em claro.
func HandlePlaintext(ctx context.Context, t Transport, info *types.MessageInfo, node *waBinary.Node) (handlerFailed bool) {
	// TODO edits have an additional <meta msg_edit_t="1696321271735" original_msg_t="1696321248"/> node
	plaintext, ok := node.GetOptionalChildByTag("plaintext")
	if !ok {
		// 3:
		return
	}
	plaintextBody, ok := plaintext.Content.([]byte)
	if !ok {
		t.Log().Warnf("Plaintext message from %s doesn't have byte content", info.SourceString())
		return
	}

	var msg waE2E.Message
	err := proto.Unmarshal(plaintextBody, &msg)
	if err != nil {
		t.Log().Warnf("Error unmarshaling plaintext message from %s: %v", info.SourceString(), err)
		return
	}
	StoreSecret(ctx, t, info, &msg)
	evt := &events.Message{
		Info:       *info,
		RawMessage: &msg,
	}
	meta, ok := node.GetOptionalChildByTag("meta")
	if ok {
		evt.NewsletterMeta = &events.NewsletterMessageMeta{
			EditTS:     meta.AttrGetter().UnixMilli("msg_edit_t"),
			OriginalTS: meta.AttrGetter().UnixTime("original_msg_t"),
		}
	}
	return t.DispatchEvent(evt.UnwrapRaw())
}

// MigrateSessionStore move a sessao Signal de um JID de telefone para o LID
// correspondente. Falha e' apenas logada — era assim antes da extracao, e
// abortar a decifragem por causa disso perderia a mensagem.
func MigrateSessionStore(ctx context.Context, t Transport, pn, lid types.JID) {
	err := t.Store().Sessions.MigratePNToLID(ctx, pn, lid)
	if err != nil {
		t.Log().Errorf("Failed to migrate signal store from %s to %s: %v", pn, lid, err)
	}
}

// ClearUntrustedIdentity apaga identidade e sessao de um alvo cuja identidade
// mudou, e despacha o evento de mudanca de identidade.
func ClearUntrustedIdentity(ctx context.Context, t Transport, target types.JID) error {
	err := t.Store().Identities.DeleteIdentity(ctx, target.SignalAddress().String())
	if err != nil {
		return fmt.Errorf("failed to delete identity: %w", err)
	}
	err = t.Store().Sessions.DeleteSession(ctx, target.SignalAddress().String())
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	go t.DispatchEvent(&events.IdentityChange{JID: target, Timestamp: time.Now(), Implicit: true})
	return nil
}
