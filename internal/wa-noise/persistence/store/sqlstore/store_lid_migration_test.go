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

	"wa-api/internal/wa-noise/protocol/types"
)

func migrationJIDs(t *testing.T) (pn, lid types.JID) {
	t.Helper()
	return mustJID(t, "5511777777777", types.DefaultUserServer),
		mustJID(t, "222222222222222", types.HiddenUserServer)
}

// MigratePNToLID tem que mover as TRES tabelas com chave por endereco Signal
// juntas. Se movesse so' a sessao, o endereco novo ficaria sem identity key e a
// proxima mensagem falharia na verificacao de confianca.
func TestMigratePNToLIDMovesSessionsIdentitiesAndSenderKeys(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	pn, lid := migrationJIDs(t)
	pnAddr := pn.SignalAddressUser() + ":0"
	lidAddr := lid.SignalAddressUser() + ":0"

	if err := s.PutSession(ctx, pnAddr, []byte("sessao")); err != nil {
		t.Fatalf("PutSession: %v", err)
	}
	if err := s.PutIdentity(ctx, pnAddr, testKey(1)); err != nil {
		t.Fatalf("PutIdentity: %v", err)
	}
	if err := s.PutSenderKey(ctx, "grupo@g.us", pnAddr, []byte("sender-key")); err != nil {
		t.Fatalf("PutSenderKey: %v", err)
	}

	if err := s.MigratePNToLID(ctx, pn, lid); err != nil {
		t.Fatalf("MigratePNToLID: %v", err)
	}

	sess, err := s.GetSession(ctx, lidAddr)
	if err != nil {
		t.Fatalf("GetSession LID: %v", err)
	}
	if !bytes.Equal(sess, []byte("sessao")) {
		t.Fatalf("sessao nao migrou: %q", sess)
	}

	trusted, err := s.IsTrustedIdentity(ctx, lidAddr, testKey(1))
	if err != nil {
		t.Fatalf("IsTrustedIdentity LID: %v", err)
	}
	if !trusted {
		t.Fatal("identity key nao migrou para o endereco LID")
	}

	key, err := s.GetSenderKey(ctx, "grupo@g.us", lidAddr)
	if err != nil {
		t.Fatalf("GetSenderKey LID: %v", err)
	}
	if !bytes.Equal(key, []byte("sender-key")) {
		t.Fatalf("sender key nao migrou: %q", key)
	}
}

func TestMigratePNToLIDDeletesOldPNRows(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	pn, lid := migrationJIDs(t)
	pnAddr := pn.SignalAddressUser() + ":0"

	if err := s.PutSession(ctx, pnAddr, []byte("sessao")); err != nil {
		t.Fatalf("PutSession: %v", err)
	}
	if err := s.PutIdentity(ctx, pnAddr, testKey(2)); err != nil {
		t.Fatalf("PutIdentity: %v", err)
	}
	if err := s.PutSenderKey(ctx, "grupo@g.us", pnAddr, []byte("sk")); err != nil {
		t.Fatalf("PutSenderKey: %v", err)
	}

	if err := s.MigratePNToLID(ctx, pn, lid); err != nil {
		t.Fatalf("MigratePNToLID: %v", err)
	}

	if sess, sErr := s.GetSession(ctx, pnAddr); sErr != nil || sess != nil {
		t.Fatalf("a sessao antiga em PN deveria ter sido apagada (sess=%q err=%v)", sess, sErr)
	}
	if key, kErr := s.GetSenderKey(ctx, "grupo@g.us", pnAddr); kErr != nil || key != nil {
		t.Fatalf("a sender key antiga em PN deveria ter sido apagada (key=%q err=%v)", key, kErr)
	}
	// Identity apagada => endereco desconhecido => confiavel para qualquer chave.
	trusted, err := s.IsTrustedIdentity(ctx, pnAddr, testKey(99))
	if err != nil {
		t.Fatalf("IsTrustedIdentity PN: %v", err)
	}
	if !trusted {
		t.Fatal("a identity antiga em PN deveria ter sido apagada")
	}
}

// A segunda chamada e' curto-circuitada por migratedPNSessionsCache. Isso e'
// comportamento, nao otimizacao: sem o cache, uma sessao nova criada em PN
// depois da migracao seria movida de novo por cima da sessao LID em uso.
func TestMigratePNToLIDIsSkippedOnSecondCall(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	pn, lid := migrationJIDs(t)
	pnAddr := pn.SignalAddressUser() + ":0"
	lidAddr := lid.SignalAddressUser() + ":0"

	if err := s.PutSession(ctx, pnAddr, []byte("primeira")); err != nil {
		t.Fatalf("PutSession: %v", err)
	}
	if err := s.MigratePNToLID(ctx, pn, lid); err != nil {
		t.Fatalf("MigratePNToLID 1: %v", err)
	}

	// Uma sessao nova aparece em PN e a migracao e' pedida de novo.
	if err := s.PutSession(ctx, pnAddr, []byte("segunda")); err != nil {
		t.Fatalf("PutSession 2: %v", err)
	}
	if err := s.MigratePNToLID(ctx, pn, lid); err != nil {
		t.Fatalf("MigratePNToLID 2: %v", err)
	}

	sess, err := s.GetSession(ctx, lidAddr)
	if err != nil {
		t.Fatalf("GetSession LID: %v", err)
	}
	if !bytes.Equal(sess, []byte("primeira")) {
		t.Fatalf("a segunda migracao nao deveria ter rodado; LID = %q", sess)
	}
	if sess, err = s.GetSession(ctx, pnAddr); err != nil || !bytes.Equal(sess, []byte("segunda")) {
		t.Fatalf("a sessao nova em PN deveria estar intacta (sess=%q err=%v)", sess, err)
	}
}

func TestMigratePNToLIDWithNothingToMigrateSucceeds(t *testing.T) {
	pn, lid := migrationJIDs(t)
	if err := newTestStore(t).MigratePNToLID(context.Background(), pn, lid); err != nil {
		t.Fatalf("MigratePNToLID sem dados: %v", err)
	}
}

func TestMigratePNToLIDOverwritesExistingLIDRow(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	pn, lid := migrationJIDs(t)
	pnAddr := pn.SignalAddressUser() + ":0"
	lidAddr := lid.SignalAddressUser() + ":0"

	if err := s.PutSession(ctx, lidAddr, []byte("antiga-lid")); err != nil {
		t.Fatalf("PutSession LID: %v", err)
	}
	if err := s.PutSession(ctx, pnAddr, []byte("nova-pn")); err != nil {
		t.Fatalf("PutSession PN: %v", err)
	}
	if err := s.MigratePNToLID(ctx, pn, lid); err != nil {
		t.Fatalf("MigratePNToLID: %v", err)
	}
	sess, err := s.GetSession(ctx, lidAddr)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if !bytes.Equal(sess, []byte("nova-pn")) {
		t.Fatalf("o ON CONFLICT deveria ter sobrescrito a linha LID: %q", sess)
	}
}
