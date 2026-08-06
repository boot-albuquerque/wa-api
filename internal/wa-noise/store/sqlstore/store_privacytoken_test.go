// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package sqlstore

import (
	"bytes"
	"context"
	"testing"
	"time"

	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
)

func privacyToken(user string, token string, ts time.Time) store.PrivacyToken {
	return store.PrivacyToken{
		User:      types.JID{User: user, Server: types.DefaultUserServer},
		Token:     []byte(token),
		Timestamp: ts,
	}
}

func TestGetPrivacyTokenMissingReturnsNil(t *testing.T) {
	token, err := newTestStore(t).GetPrivacyToken(context.Background(), contactJID("1"))
	if err != nil {
		t.Fatalf("GetPrivacyToken: %v", err)
	}
	if token != nil {
		t.Fatalf("token inexistente deveria devolver nil, veio %+v", token)
	}
}

func TestPutGetPrivacyTokenRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)
	if err := s.PutPrivacyTokens(ctx, privacyToken("1", "tok", now)); err != nil {
		t.Fatalf("PutPrivacyTokens: %v", err)
	}
	got, err := s.GetPrivacyToken(ctx, contactJID("1"))
	if err != nil {
		t.Fatalf("GetPrivacyToken: %v", err)
	}
	if got == nil {
		t.Fatal("GetPrivacyToken devolveu nil")
	}
	if !bytes.Equal(got.Token, []byte("tok")) {
		t.Fatalf("Token = %q", got.Token)
	}
	if !got.Timestamp.Equal(now) {
		t.Fatalf("Timestamp = %v, esperava %v", got.Timestamp, now)
	}
	if !got.SenderTimestamp.IsZero() {
		t.Fatalf("SenderTimestamp deveria ser zero (NULL no banco), veio %v", got.SenderTimestamp)
	}
}

func TestPutPrivacyTokensPersistsSenderTimestamp(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)
	token := privacyToken("1", "tok", now)
	token.SenderTimestamp = now.Add(-time.Minute)
	if err := s.PutPrivacyTokens(ctx, token); err != nil {
		t.Fatalf("PutPrivacyTokens: %v", err)
	}
	got, err := s.GetPrivacyToken(ctx, contactJID("1"))
	if err != nil {
		t.Fatalf("GetPrivacyToken: %v", err)
	}
	if !got.SenderTimestamp.Equal(token.SenderTimestamp) {
		t.Fatalf("SenderTimestamp = %v, esperava %v", got.SenderTimestamp, token.SenderTimestamp)
	}
}

// O ON CONFLICT tem WHERE EXCLUDED.timestamp >= ...: um token mais VELHO nao
// substitui o vigente.
func TestPutPrivacyTokensIgnoresOlderTimestamp(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)
	if err := s.PutPrivacyTokens(ctx, privacyToken("1", "novo", now)); err != nil {
		t.Fatalf("PutPrivacyTokens novo: %v", err)
	}
	if err := s.PutPrivacyTokens(ctx, privacyToken("1", "velho", now.Add(-time.Hour))); err != nil {
		t.Fatalf("PutPrivacyTokens velho: %v", err)
	}
	got, err := s.GetPrivacyToken(ctx, contactJID("1"))
	if err != nil {
		t.Fatalf("GetPrivacyToken: %v", err)
	}
	if !bytes.Equal(got.Token, []byte("novo")) {
		t.Fatalf("um token mais velho sobrescreveu o vigente: %q", got.Token)
	}
}

func TestPutPrivacyTokensAcceptsNewerTimestamp(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)
	if err := s.PutPrivacyTokens(ctx, privacyToken("1", "velho", now.Add(-time.Hour))); err != nil {
		t.Fatalf("PutPrivacyTokens velho: %v", err)
	}
	if err := s.PutPrivacyTokens(ctx, privacyToken("1", "novo", now)); err != nil {
		t.Fatalf("PutPrivacyTokens novo: %v", err)
	}
	got, err := s.GetPrivacyToken(ctx, contactJID("1"))
	if err != nil {
		t.Fatalf("GetPrivacyToken: %v", err)
	}
	if !bytes.Equal(got.Token, []byte("novo")) {
		t.Fatalf("o token mais novo deveria ter substituido: %q", got.Token)
	}
}

// PutPrivacyTokens e' variadico e monta o VALUES em runtime substituindo
// privacyTokenValuesTemplate — o caminho de multiplos tokens e' o unico em que
// essa substituicao faz diferenca.
func TestPutPrivacyTokensAcceptsMultipleTokensInOneStatement(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)
	err := s.PutPrivacyTokens(ctx,
		privacyToken("1", "t1", now),
		privacyToken("2", "t2", now),
		privacyToken("3", "t3", now),
	)
	if err != nil {
		t.Fatalf("PutPrivacyTokens: %v", err)
	}
	for user, want := range map[string]string{"1": "t1", "2": "t2", "3": "t3"} {
		got, gErr := s.GetPrivacyToken(ctx, contactJID(user))
		if gErr != nil {
			t.Fatalf("GetPrivacyToken %s: %v", user, gErr)
		}
		if got == nil || string(got.Token) != want {
			t.Fatalf("token de %s = %v, esperava %q", user, got, want)
		}
	}
}

func TestDeleteExpiredPrivacyTokens(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)
	if err := s.PutPrivacyTokens(ctx, privacyToken("1", "velho", now.Add(-2*time.Hour))); err != nil {
		t.Fatalf("PutPrivacyTokens velho: %v", err)
	}
	if err := s.PutPrivacyTokens(ctx, privacyToken("2", "novo", now)); err != nil {
		t.Fatalf("PutPrivacyTokens novo: %v", err)
	}

	deleted, err := s.DeleteExpiredPrivacyTokens(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("DeleteExpiredPrivacyTokens: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("esperava 1 token apagado, veio %d", deleted)
	}
	if got, gErr := s.GetPrivacyToken(ctx, contactJID("1")); gErr != nil || got != nil {
		t.Fatalf("o token velho deveria ter sumido (got=%v err=%v)", got, gErr)
	}
	if got, gErr := s.GetPrivacyToken(ctx, contactJID("2")); gErr != nil || got == nil {
		t.Fatalf("o token novo deveria continuar (got=%v err=%v)", got, gErr)
	}
}

func TestDeleteExpiredPrivacyTokensWithNothingExpired(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)
	if err := s.PutPrivacyTokens(ctx, privacyToken("1", "tok", now)); err != nil {
		t.Fatalf("PutPrivacyTokens: %v", err)
	}
	deleted, err := s.DeleteExpiredPrivacyTokens(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("DeleteExpiredPrivacyTokens: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("nao deveria apagar nada, apagou %d", deleted)
	}
}

func TestGetPrivacyTokenResolvesLIDToPN(t *testing.T) {
	ctx := context.Background()
	container, _, s := newTestDevice(t)
	pn := types.JID{User: "5511555555555", Server: types.DefaultUserServer}
	lid := types.JID{User: "333333333333333", Server: types.HiddenUserServer}
	if err := container.LIDMap.PutLIDMapping(ctx, lid, pn); err != nil {
		t.Fatalf("PutLIDMapping: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	err := s.PutPrivacyTokens(ctx, store.PrivacyToken{User: pn, Token: []byte("tok"), Timestamp: now})
	if err != nil {
		t.Fatalf("PutPrivacyTokens: %v", err)
	}
	got, err := s.GetPrivacyToken(ctx, lid)
	if err != nil {
		t.Fatalf("GetPrivacyToken por LID: %v", err)
	}
	if got == nil || !bytes.Equal(got.Token, []byte("tok")) {
		t.Fatalf("o CASE de LID->PN nao encontrou o token: %v", got)
	}
}

func TestPrivacyTokensAreScopedPerOurJID(t *testing.T) {
	ctx := context.Background()
	container, _, s := newTestDevice(t)
	now := time.Now().Truncate(time.Second)
	if err := s.PutPrivacyTokens(ctx, privacyToken("1", "tok", now)); err != nil {
		t.Fatalf("PutPrivacyTokens: %v", err)
	}
	other := NewSQLStore(container, mustJID(t, "5511888888888", "s.whatsapp.net"))
	got, err := other.GetPrivacyToken(ctx, contactJID("1"))
	if err != nil {
		t.Fatalf("GetPrivacyToken outro our_jid: %v", err)
	}
	if got != nil {
		t.Fatalf("o token de um our_jid vazou para outro: %+v", got)
	}
}
