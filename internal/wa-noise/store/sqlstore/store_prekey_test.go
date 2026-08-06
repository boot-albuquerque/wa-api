// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package sqlstore

import (
	"context"
	"testing"
)

func TestGetOrGenPreKeysGeneratesRequestedCount(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	preKeys, err := s.GetOrGenPreKeys(ctx, 5)
	if err != nil {
		t.Fatalf("GetOrGenPreKeys: %v", err)
	}
	if len(preKeys) != 5 {
		t.Fatalf("esperava 5 prekeys, veio %d", len(preKeys))
	}
	seen := make(map[uint32]bool, len(preKeys))
	for i, k := range preKeys {
		if k == nil {
			t.Fatalf("prekey %d e' nil", i)
		}
		if seen[k.KeyID] {
			t.Fatalf("KeyID %d duplicado", k.KeyID)
		}
		seen[k.KeyID] = true
	}
}

// A segunda chamada tem que devolver as MESMAS chaves ainda nao enviadas ao
// servidor, nao gerar outras — senao o cliente publicaria prekeys que nao
// consegue mais usar para decifrar.
func TestGetOrGenPreKeysReusesUnuploadedKeys(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	first, err := s.GetOrGenPreKeys(ctx, 3)
	if err != nil {
		t.Fatalf("GetOrGenPreKeys 1: %v", err)
	}
	second, err := s.GetOrGenPreKeys(ctx, 3)
	if err != nil {
		t.Fatalf("GetOrGenPreKeys 2: %v", err)
	}
	for i := range first {
		if first[i].KeyID != second[i].KeyID {
			t.Fatalf("prekey %d mudou de ID entre chamadas: %d -> %d", i, first[i].KeyID, second[i].KeyID)
		}
	}
}

func TestGetOrGenPreKeysTopsUpAfterUpload(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	first, err := s.GetOrGenPreKeys(ctx, 3)
	if err != nil {
		t.Fatalf("GetOrGenPreKeys 1: %v", err)
	}
	if err = s.MarkPreKeysAsUploaded(ctx, first[len(first)-1].KeyID); err != nil {
		t.Fatalf("MarkPreKeysAsUploaded: %v", err)
	}
	second, err := s.GetOrGenPreKeys(ctx, 3)
	if err != nil {
		t.Fatalf("GetOrGenPreKeys 2: %v", err)
	}
	for _, k := range second {
		if k.KeyID <= first[len(first)-1].KeyID {
			t.Fatalf("apos o upload as prekeys deveriam ser novas, veio ID %d", k.KeyID)
		}
	}
}

func TestGenOnePreKeyIsMarkedUploaded(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	key, err := s.GenOnePreKey(ctx)
	if err != nil {
		t.Fatalf("GenOnePreKey: %v", err)
	}
	count, err := s.UploadedPreKeyCount(ctx)
	if err != nil {
		t.Fatalf("UploadedPreKeyCount: %v", err)
	}
	if count != 1 {
		t.Fatalf("GenOnePreKey deveria gravar ja' como uploaded; count=%d", count)
	}
	// E, por ser uploaded, nao deve reaparecer em GetOrGenPreKeys.
	batch, err := s.GetOrGenPreKeys(ctx, 2)
	if err != nil {
		t.Fatalf("GetOrGenPreKeys: %v", err)
	}
	for _, k := range batch {
		if k.KeyID == key.KeyID {
			t.Fatalf("prekey ja' enviada (%d) reapareceu no lote nao enviado", key.KeyID)
		}
	}
}

func TestGetPreKeyRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	generated, err := s.GenOnePreKey(ctx)
	if err != nil {
		t.Fatalf("GenOnePreKey: %v", err)
	}
	loaded, err := s.GetPreKey(ctx, generated.KeyID)
	if err != nil {
		t.Fatalf("GetPreKey: %v", err)
	}
	if loaded == nil {
		t.Fatal("GetPreKey devolveu nil para uma chave existente")
	}
	if loaded.KeyID != generated.KeyID {
		t.Fatalf("KeyID: %d != %d", loaded.KeyID, generated.KeyID)
	}
	if *loaded.Priv != *generated.Priv {
		t.Fatal("a chave privada nao voltou igual")
	}
	// A publica e' rederivada da privada em scanPreKey — deve bater tambem.
	if *loaded.Pub != *generated.Pub {
		t.Fatal("a chave publica rederivada nao bate com a original")
	}
}

func TestGetPreKeyMissingReturnsNil(t *testing.T) {
	key, err := newTestStore(t).GetPreKey(context.Background(), 999999)
	if err != nil {
		t.Fatalf("GetPreKey: %v", err)
	}
	if key != nil {
		t.Fatalf("prekey inexistente deveria devolver nil, veio %+v", key)
	}
}

func TestRemovePreKey(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	key, err := s.GenOnePreKey(ctx)
	if err != nil {
		t.Fatalf("GenOnePreKey: %v", err)
	}
	if err = s.RemovePreKey(ctx, key.KeyID); err != nil {
		t.Fatalf("RemovePreKey: %v", err)
	}
	loaded, err := s.GetPreKey(ctx, key.KeyID)
	if err != nil {
		t.Fatalf("GetPreKey: %v", err)
	}
	if loaded != nil {
		t.Fatal("a prekey removida ainda esta' la'")
	}
}

func TestMarkPreKeysAsUploadedIsInclusiveUpToID(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	batch, err := s.GetOrGenPreKeys(ctx, 4)
	if err != nil {
		t.Fatalf("GetOrGenPreKeys: %v", err)
	}
	if err = s.MarkPreKeysAsUploaded(ctx, batch[1].KeyID); err != nil {
		t.Fatalf("MarkPreKeysAsUploaded: %v", err)
	}
	count, err := s.UploadedPreKeyCount(ctx)
	if err != nil {
		t.Fatalf("UploadedPreKeyCount: %v", err)
	}
	if count != 2 {
		t.Fatalf("key_id<=$2 deveria marcar as 2 primeiras, marcou %d", count)
	}
}

func TestUploadedPreKeyCountStartsAtZero(t *testing.T) {
	count, err := newTestStore(t).UploadedPreKeyCount(context.Background())
	if err != nil {
		t.Fatalf("UploadedPreKeyCount: %v", err)
	}
	if count != 0 {
		t.Fatalf("esperava 0, veio %d", count)
	}
}

func TestPreKeyIDsAreScopedPerJID(t *testing.T) {
	ctx := context.Background()
	container, _, s := newTestDevice(t)
	if _, err := s.GetOrGenPreKeys(ctx, 3); err != nil {
		t.Fatalf("GetOrGenPreKeys: %v", err)
	}
	other := NewSQLStore(container, mustJID(t, "5511888888888", "s.whatsapp.net"))
	count, err := other.UploadedPreKeyCount(ctx)
	if err != nil {
		t.Fatalf("UploadedPreKeyCount outro jid: %v", err)
	}
	if count != 0 {
		t.Fatalf("as prekeys de um jid vazaram para outro (count=%d)", count)
	}
}
