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
)

func TestGetNCTSaltMissingReturnsNil(t *testing.T) {
	salt, err := newTestStore(t).GetNCTSalt(context.Background())
	if err != nil {
		t.Fatalf("GetNCTSalt: %v", err)
	}
	if salt != nil {
		t.Fatalf("sem salt gravado deveria devolver nil, veio %q", salt)
	}
}

func TestPutGetNCTSaltRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.PutNCTSalt(ctx, []byte("salt-1")); err != nil {
		t.Fatalf("PutNCTSalt: %v", err)
	}
	salt, err := s.GetNCTSalt(ctx)
	if err != nil {
		t.Fatalf("GetNCTSalt: %v", err)
	}
	if !bytes.Equal(salt, []byte("salt-1")) {
		t.Fatalf("GetNCTSalt = %q", salt)
	}
}

// A tabela tem PK (our_jid): so' existe um salt por conta, e regravar
// substitui.
func TestPutNCTSaltOverwrites(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.PutNCTSalt(ctx, []byte("v1")); err != nil {
		t.Fatalf("PutNCTSalt v1: %v", err)
	}
	if err := s.PutNCTSalt(ctx, []byte("v2")); err != nil {
		t.Fatalf("PutNCTSalt v2: %v", err)
	}
	salt, err := s.GetNCTSalt(ctx)
	if err != nil {
		t.Fatalf("GetNCTSalt: %v", err)
	}
	if !bytes.Equal(salt, []byte("v2")) {
		t.Fatalf("ON CONFLICT DO UPDATE nao substituiu: %q", salt)
	}
}

func TestDeleteNCTSalt(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.PutNCTSalt(ctx, []byte("salt")); err != nil {
		t.Fatalf("PutNCTSalt: %v", err)
	}
	if err := s.DeleteNCTSalt(ctx); err != nil {
		t.Fatalf("DeleteNCTSalt: %v", err)
	}
	salt, err := s.GetNCTSalt(ctx)
	if err != nil {
		t.Fatalf("GetNCTSalt: %v", err)
	}
	if salt != nil {
		t.Fatalf("o salt deveria ter sido apagado, veio %q", salt)
	}
}

func TestDeleteNCTSaltWithoutSaltIsNoOp(t *testing.T) {
	if err := newTestStore(t).DeleteNCTSalt(context.Background()); err != nil {
		t.Fatalf("DeleteNCTSalt sem salt: %v", err)
	}
}

func TestNCTSaltIsScopedPerOurJID(t *testing.T) {
	ctx := context.Background()
	container, _, s := newTestDevice(t)
	if err := s.PutNCTSalt(ctx, []byte("segredo")); err != nil {
		t.Fatalf("PutNCTSalt: %v", err)
	}
	other := NewSQLStore(container, mustJID(t, "5511888888888", "s.whatsapp.net"))
	salt, err := other.GetNCTSalt(ctx)
	if err != nil {
		t.Fatalf("GetNCTSalt outro our_jid: %v", err)
	}
	if salt != nil {
		t.Fatalf("o salt de um our_jid vazou para outro: %q", salt)
	}
}
