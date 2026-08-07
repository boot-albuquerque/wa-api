// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package sqlstore

import (
	"context"
	"testing"
	"time"

	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/protocol/types"
)

func chatJID(user string) types.JID {
	return types.JID{User: user, Server: types.GroupServer}
}

func TestGetChatSettingsUnknownIsNotFound(t *testing.T) {
	settings, err := newTestStore(t).GetChatSettings(context.Background(), chatJID("g1"))
	if err != nil {
		t.Fatalf("GetChatSettings: %v", err)
	}
	if settings.Found {
		t.Fatal("chat sem configuracao nao pode vir com Found=true")
	}
	if settings.Pinned || settings.Archived || !settings.MutedUntil.IsZero() {
		t.Fatalf("chat sem configuracao deveria vir zerado: %+v", settings)
	}
}

func TestPutPinnedAndArchived(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	chat := chatJID("g1")
	if err := s.PutPinned(ctx, chat, true); err != nil {
		t.Fatalf("PutPinned: %v", err)
	}
	if err := s.PutArchived(ctx, chat, true); err != nil {
		t.Fatalf("PutArchived: %v", err)
	}
	settings, err := s.GetChatSettings(ctx, chat)
	if err != nil {
		t.Fatalf("GetChatSettings: %v", err)
	}
	if !settings.Found || !settings.Pinned || !settings.Archived {
		t.Fatalf("GetChatSettings = %+v", settings)
	}
}

// As tres colunas sao gravadas por queries separadas sobre a MESMA linha
// (ON CONFLICT DO UPDATE de uma coluna so'): mexer em uma nao pode zerar as
// outras.
func TestChatSettingColumnsAreIndependent(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	chat := chatJID("g1")
	if err := s.PutPinned(ctx, chat, true); err != nil {
		t.Fatalf("PutPinned: %v", err)
	}
	if err := s.PutArchived(ctx, chat, false); err != nil {
		t.Fatalf("PutArchived: %v", err)
	}
	settings, err := s.GetChatSettings(ctx, chat)
	if err != nil {
		t.Fatalf("GetChatSettings: %v", err)
	}
	if !settings.Pinned {
		t.Fatal("PutArchived apagou o pinned")
	}
	if settings.Archived {
		t.Fatal("archived deveria continuar falso")
	}
}

func TestPutMutedUntilTimestampRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	chat := chatJID("g1")
	until := time.Now().Add(time.Hour).Truncate(time.Second)
	if err := s.PutMutedUntil(ctx, chat, until); err != nil {
		t.Fatalf("PutMutedUntil: %v", err)
	}
	settings, err := s.GetChatSettings(ctx, chat)
	if err != nil {
		t.Fatalf("GetChatSettings: %v", err)
	}
	if !settings.MutedUntil.Equal(until) {
		t.Fatalf("MutedUntil = %v, esperava %v", settings.MutedUntil, until)
	}
}

// Mute permanente nao cabe num timestamp Unix: e' gravado como o sentinela
// negativo mutedForeverDBValue e relido como store.MutedForever.
func TestPutMutedUntilForeverRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	chat := chatJID("g1")
	if err := s.PutMutedUntil(ctx, chat, store.MutedForever); err != nil {
		t.Fatalf("PutMutedUntil: %v", err)
	}
	settings, err := s.GetChatSettings(ctx, chat)
	if err != nil {
		t.Fatalf("GetChatSettings: %v", err)
	}
	if !settings.MutedUntil.Equal(store.MutedForever) {
		t.Fatalf("MutedUntil = %v, esperava MutedForever", settings.MutedUntil)
	}
}

func TestPutMutedUntilZeroUnmutes(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	chat := chatJID("g1")
	if err := s.PutMutedUntil(ctx, chat, store.MutedForever); err != nil {
		t.Fatalf("PutMutedUntil forever: %v", err)
	}
	if err := s.PutMutedUntil(ctx, chat, time.Time{}); err != nil {
		t.Fatalf("PutMutedUntil zero: %v", err)
	}
	settings, err := s.GetChatSettings(ctx, chat)
	if err != nil {
		t.Fatalf("GetChatSettings: %v", err)
	}
	if !settings.MutedUntil.IsZero() {
		t.Fatalf("time.Time zero deveria desmutar, veio %v", settings.MutedUntil)
	}
}

func TestChatSettingsAreScopedPerOurJID(t *testing.T) {
	ctx := context.Background()
	container, _, s := newTestDevice(t)
	if err := s.PutPinned(ctx, chatJID("g1"), true); err != nil {
		t.Fatalf("PutPinned: %v", err)
	}
	other := NewSQLStore(container, mustJID(t, "5511888888888", "s.whatsapp.net"))
	settings, err := other.GetChatSettings(ctx, chatJID("g1"))
	if err != nil {
		t.Fatalf("GetChatSettings outro our_jid: %v", err)
	}
	if settings.Found || settings.Pinned {
		t.Fatalf("a configuracao de chat de um our_jid vazou para outro: %+v", settings)
	}
}
