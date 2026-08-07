// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package tctoken implementa o trusted contact token: a emissao, o cache em
// memoria e a poda dos tokens que acompanham mensagens enviadas.
package tctoken

import (
	"context"
	"fmt"
	"strconv"
	"time"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

const (
	// BucketDuration e' a duracao de um bucket em segundos (7 dias).
	// Matches AB prop tctoken_duration.
	BucketDuration = 604800
	// NumBuckets e' o numero de buckets rolantes (4 = janela de ~28 dias).
	// Matches AB prop tctoken_num_buckets.
	NumBuckets = 4
	// DBPruneInterval e' o intervalo minimo entre podas no banco.
	DBPruneInterval = 24 * time.Hour

	// TokenType e' o valor do atributo "type" do no <token>. Exportada porque
	// notification_privacy.go (raiz) compara o atributo recebido contra ela.
	TokenType = "trusted_contact"
)

// CurrentCutoffTimestamp e' o inicio do bucket mais antigo ainda valido.
func CurrentCutoffTimestamp() time.Time {
	currentBucket := time.Now().Unix() / BucketDuration
	cutoffBucket := currentBucket - (NumBuckets - 1)
	return time.Unix(cutoffBucket*BucketDuration, 0)
}

// IsExpired diz se um token emitido em `timestamp` caiu fora da janela.
func IsExpired(timestamp time.Time) bool {
	if timestamp.IsZero() {
		return true
	}
	return timestamp.Before(CurrentCutoffTimestamp())
}

// ShouldSendNew returns true when the current bucket is newer than the last issuance bucket.
func ShouldSendNew(senderTimestamp time.Time) bool {
	if senderTimestamp.IsZero() {
		return true
	}
	now := time.Now().Unix()
	return now/BucketDuration > senderTimestamp.Unix()/BucketDuration
}

// ShouldSendInChatAction diz se o JID e' de um contato para o qual faz sentido
// mandar tctoken: usuario comum ou LID, nunca PSA nem bot.
func ShouldSendInChatAction(jid types.JID) bool {
	jid = jid.ToNonAD()
	return (jid.Server == types.DefaultUserServer || jid.Server == types.HiddenUserServer) &&
		jid.User != types.PSAJID.User &&
		!jid.IsBot()
}

// ResolveStorageLID traduz um JID de telefone para o LID sob o qual o token e'
// guardado; qualquer falha de resolucao mantem o JID original.
func ResolveStorageLID(ctx context.Context, t Transport, jid types.JID) types.JID {
	storageJID := jid.ToNonAD()
	if storageJID.Server != types.DefaultUserServer || t.Store() == nil || t.Store().LIDs == nil {
		return storageJID
	}
	lid, err := t.Store().LIDs.GetLIDForPN(ctx, storageJID)
	if err != nil {
		t.Log().Debugf("Failed to resolve LID for tctoken JID %s: %v", storageJID, err)
		return storageJID
	}
	if lid.IsEmpty() {
		return storageJID
	}
	return lid.ToNonAD()
}

// Ensure returns a stored non-expired tctoken for the given JID, if available.
func Ensure(ctx context.Context, t Transport, jid types.JID) (token []byte, err error) {
	DeleteExpired(t)
	storageJID := ResolveStorageLID(ctx, t, jid)
	existing, err := t.Store().PrivacyTokens.GetPrivacyToken(ctx, storageJID)
	if err != nil {
		return nil, fmt.Errorf("failed to get privacy token: %w", err)
	}
	if existing == nil {
		return nil, nil
	}
	t.State().ValidateAndSet(storageJID, existing.SenderTimestamp)
	if len(existing.Token) > 0 && !IsExpired(existing.Timestamp) {
		return existing.Token, nil
	}
	return nil, nil
}

// DeleteExpired dispara, no maximo uma vez por DBPruneInterval, a poda dos
// tokens expirados no banco.
//
// A poda roda numa goroutine e o lock so' e' liberado quando ela termina —
// mesma estrutura de Client.deleteExpiredPrivacyTokens.
func DeleteExpired(t Transport) {
	if !t.State().TryStartDBPrune(DBPruneInterval) {
		return
	}
	go func() {
		defer t.State().FinishDBPrune()
		deleted, err := t.Store().PrivacyTokens.DeleteExpiredPrivacyTokens(t.BackgroundCtx(), CurrentCutoffTimestamp())
		if err != nil {
			t.Log().Warnf("Failed to remove expired tctokens from DB: %v", err)
		} else if deleted > 0 {
			t.Log().Debugf("Removed %d expired tctokens from DB", deleted)
		}
	}()
}

// IssueAndSave emite um token novo e persiste o timestamp de emissao.
//
// Only called when a bucket boundary has been crossed since the last issuance.
func IssueAndSave(t Transport, jid types.JID, senderTimestamp time.Time) {
	ctx := t.BackgroundCtx()
	storageJID := jid.ToNonAD()
	_, err := Issue(ctx, t, storageJID, senderTimestamp)
	if err != nil {
		t.Log().Errorf("Failed to issue privacy token for %s: %v", jid, err)
		return
	}
	t.State().SetSenderTS(storageJID, senderTimestamp)
	// TODO replace with an UPDATE call instead of get+put
	existing, err := t.Store().PrivacyTokens.GetPrivacyToken(ctx, storageJID)
	if err != nil {
		t.Log().Errorf("Failed to load tctoken while persisting sender timestamp for %s: %v", jid, err)
		return
	}
	if existing == nil || len(existing.Token) == 0 {
		return
	}
	existing.SenderTimestamp = senderTimestamp
	if err = t.Store().PrivacyTokens.PutPrivacyTokens(ctx, *existing); err != nil {
		t.Log().Errorf("Failed to persist privacy token sender timestamp for %s: %v", jid, err)
	}
}

// Issue sends an IQ to the server to issue a privacy token for the given JID.
func Issue(ctx context.Context, t Transport, jid types.JID, timestamp time.Time) (*waBinary.Node, error) {
	return t.SendIQ(ctx, IQ{
		Namespace: "privacy",
		Type:      IQSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: "tokens",
			Content: []waBinary.Node{{
				Tag: "token",
				Attrs: waBinary.Attrs{
					"jid":  jid.ToNonAD(),
					"t":    strconv.FormatInt(timestamp.Unix(), 10),
					"type": TokenType,
				},
			}},
		}},
	})
}
