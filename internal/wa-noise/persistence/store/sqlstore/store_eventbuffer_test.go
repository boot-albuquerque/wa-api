// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package sqlstore

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func testCiphertextHash(fill byte) [ciphertextHashLength]byte {
	var h [ciphertextHashLength]byte
	for i := range h {
		h[i] = fill
	}
	return h
}

func TestGetBufferedEventMissingReturnsNil(t *testing.T) {
	buf, err := newTestStore(t).GetBufferedEvent(context.Background(), testCiphertextHash(1))
	if err != nil {
		t.Fatalf("GetBufferedEvent: %v", err)
	}
	if buf != nil {
		t.Fatalf("evento inexistente deveria devolver nil, veio %+v", buf)
	}
}

func TestPutGetBufferedEventRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	hash := testCiphertextHash(1)
	serverTime := time.Now().Add(-time.Minute).Truncate(time.Second)

	if err := s.PutBufferedEvent(ctx, hash, []byte("plaintext"), serverTime); err != nil {
		t.Fatalf("PutBufferedEvent: %v", err)
	}
	buf, err := s.GetBufferedEvent(ctx, hash)
	if err != nil {
		t.Fatalf("GetBufferedEvent: %v", err)
	}
	if buf == nil {
		t.Fatal("GetBufferedEvent devolveu nil")
	}
	if !bytes.Equal(buf.Plaintext, []byte("plaintext")) {
		t.Fatalf("Plaintext = %q", buf.Plaintext)
	}
	if !buf.ServerTime.Equal(serverTime) {
		t.Fatalf("ServerTime = %v, esperava %v", buf.ServerTime, serverTime)
	}
	if buf.InsertTime.IsZero() {
		t.Fatal("InsertTime deveria ser preenchido pelo store")
	}
}

// ClearBufferedEventPlaintext apaga o texto claro mas MANTEM a linha: o hash
// continua registrado como "ja' processado", o que e' o mecanismo de
// deduplicacao. Apagar a linha inteira faria o evento ser reprocessado.
func TestClearBufferedEventPlaintextKeepsTheRow(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	hash := testCiphertextHash(1)
	if err := s.PutBufferedEvent(ctx, hash, []byte("plaintext"), time.Now()); err != nil {
		t.Fatalf("PutBufferedEvent: %v", err)
	}
	if err := s.ClearBufferedEventPlaintext(ctx, hash); err != nil {
		t.Fatalf("ClearBufferedEventPlaintext: %v", err)
	}
	buf, err := s.GetBufferedEvent(ctx, hash)
	if err != nil {
		t.Fatalf("GetBufferedEvent: %v", err)
	}
	if buf == nil {
		t.Fatal("a linha deveria continuar existindo apos limpar o plaintext")
	}
	if buf.Plaintext != nil {
		t.Fatalf("o plaintext deveria ter sido zerado, veio %q", buf.Plaintext)
	}
}

func TestDeleteOldBufferedHashesRemovesOnlyExpired(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	recente := testCiphertextHash(1)
	antigo := testCiphertextHash(2)

	if err := s.PutBufferedEvent(ctx, recente, []byte("r"), time.Now()); err != nil {
		t.Fatalf("PutBufferedEvent recente: %v", err)
	}
	// Envelhece a linha para alem de bufferedEventRetention direto no banco:
	// PutBufferedEvent sempre usa time.Now() para insert_timestamp.
	expirado := time.Now().Add(-bufferedEventRetention - time.Hour).UnixMilli()
	_, err := s.db.Exec(ctx, putBufferedEventQuery, s.JID, antigo[:], []byte("a"), time.Now().Unix(), expirado)
	if err != nil {
		t.Fatalf("inserir evento antigo: %v", err)
	}

	if err = s.DeleteOldBufferedHashes(ctx); err != nil {
		t.Fatalf("DeleteOldBufferedHashes: %v", err)
	}
	if buf, gErr := s.GetBufferedEvent(ctx, antigo); gErr != nil || buf != nil {
		t.Fatalf("o evento expirado deveria ter sumido (buf=%v err=%v)", buf, gErr)
	}
	if buf, gErr := s.GetBufferedEvent(ctx, recente); gErr != nil || buf == nil {
		t.Fatalf("o evento recente deveria continuar (buf=%v err=%v)", buf, gErr)
	}
}

func TestDoDecryptionTxnCommitsOnSuccess(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	hash := testCiphertextHash(1)
	err := s.DoDecryptionTxn(ctx, func(ctx context.Context) error {
		return s.PutBufferedEvent(ctx, hash, []byte("dentro-da-txn"), time.Now())
	})
	if err != nil {
		t.Fatalf("DoDecryptionTxn: %v", err)
	}
	buf, err := s.GetBufferedEvent(ctx, hash)
	if err != nil {
		t.Fatalf("GetBufferedEvent: %v", err)
	}
	if buf == nil || !bytes.Equal(buf.Plaintext, []byte("dentro-da-txn")) {
		t.Fatalf("a transacao deveria ter comitado: %v", buf)
	}
}

func TestDoDecryptionTxnRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	hash := testCiphertextHash(1)
	boom := errors.New("boom")

	err := s.DoDecryptionTxn(ctx, func(ctx context.Context) error {
		if pErr := s.PutBufferedEvent(ctx, hash, []byte("x"), time.Now()); pErr != nil {
			return pErr
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("DoDecryptionTxn deveria propagar o erro do callback, veio %v", err)
	}
	buf, err := s.GetBufferedEvent(ctx, hash)
	if err != nil {
		t.Fatalf("GetBufferedEvent: %v", err)
	}
	if buf != nil {
		t.Fatalf("a transacao deveria ter feito rollback, mas gravou %v", buf)
	}
}

func TestAddGetOutgoingEventRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	chat := chatJID("g1")
	if err := s.AddOutgoingEvent(ctx, chat, "MSGID", "proto", []byte("payload")); err != nil {
		t.Fatalf("AddOutgoingEvent: %v", err)
	}
	format, result, err := s.GetOutgoingEvent(ctx, chat, chat, "MSGID")
	if err != nil {
		t.Fatalf("GetOutgoingEvent: %v", err)
	}
	if format != "proto" {
		t.Fatalf("format = %q", format)
	}
	if !bytes.Equal(result, []byte("payload")) {
		t.Fatalf("payload = %q", result)
	}
}

// GetOutgoingEvent aceita dois JIDs de chat (chat_jid=$2 OR chat_jid=$3) para
// que uma mensagem gravada em PN seja encontrada apos a migracao do chat para
// LID, e vice-versa.
func TestGetOutgoingEventMatchesEitherChatJID(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	principal := chatJID("g1")
	alternativo := chatJID("g2")
	if err := s.AddOutgoingEvent(ctx, alternativo, "MSGID", "proto", []byte("payload")); err != nil {
		t.Fatalf("AddOutgoingEvent: %v", err)
	}
	_, result, err := s.GetOutgoingEvent(ctx, principal, alternativo, "MSGID")
	if err != nil {
		t.Fatalf("GetOutgoingEvent: %v", err)
	}
	if !bytes.Equal(result, []byte("payload")) {
		t.Fatalf("o JID alternativo deveria casar, veio %q", result)
	}
}

func TestAddOutgoingEventOverwrites(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	chat := chatJID("g1")
	if err := s.AddOutgoingEvent(ctx, chat, "MSGID", "v1", []byte("p1")); err != nil {
		t.Fatalf("AddOutgoingEvent v1: %v", err)
	}
	if err := s.AddOutgoingEvent(ctx, chat, "MSGID", "v2", []byte("p2")); err != nil {
		t.Fatalf("AddOutgoingEvent v2: %v", err)
	}
	format, result, err := s.GetOutgoingEvent(ctx, chat, chat, "MSGID")
	if err != nil {
		t.Fatalf("GetOutgoingEvent: %v", err)
	}
	if format != "v2" || !bytes.Equal(result, []byte("p2")) {
		t.Fatalf("ON CONFLICT DO UPDATE nao substituiu: format=%q payload=%q", format, result)
	}
}

// Ao contrario de GetBufferedEvent, GetOutgoingEvent NAO engole sql.ErrNoRows —
// devolve o erro cru. Travamos isso para que uma mudanca nesse contrato apareca.
func TestGetOutgoingEventMissingReturnsError(t *testing.T) {
	_, _, err := newTestStore(t).GetOutgoingEvent(
		context.Background(), chatJID("g1"), chatJID("g1"), "NAO-EXISTE",
	)
	if err == nil {
		t.Fatal("GetOutgoingEvent de mensagem inexistente deveria devolver erro (sql.ErrNoRows)")
	}
}

func TestDeleteOldOutgoingEventsRemovesOnlyExpired(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	chat := chatJID("g1")
	if err := s.AddOutgoingEvent(ctx, chat, "RECENTE", "proto", []byte("r")); err != nil {
		t.Fatalf("AddOutgoingEvent recente: %v", err)
	}
	expirado := time.Now().Add(-outgoingEventRetention - time.Hour).UnixMilli()
	_, err := s.db.Exec(ctx, addOutgoingEventQuery, s.JID, chat, "ANTIGO", "proto", []byte("a"), expirado)
	if err != nil {
		t.Fatalf("inserir evento antigo: %v", err)
	}

	if err = s.DeleteOldOutgoingEvents(ctx); err != nil {
		t.Fatalf("DeleteOldOutgoingEvents: %v", err)
	}
	if _, _, gErr := s.GetOutgoingEvent(ctx, chat, chat, "ANTIGO"); gErr == nil {
		t.Fatal("o evento expirado deveria ter sumido")
	}
	if _, _, gErr := s.GetOutgoingEvent(ctx, chat, chat, "RECENTE"); gErr != nil {
		t.Fatalf("o evento recente deveria continuar: %v", gErr)
	}
}

func TestRetentionWindowsMatchDocumentedValues(t *testing.T) {
	// bufferedEventRetention espelha a janela de 14 dias em que os servidores do
	// WhatsApp guardam eventos; outgoingEventRetention e' a de 7 dias do buffer
	// de retry. Trocar um pelo outro seria silencioso sem esta trava.
	if bufferedEventRetention != 14*24*time.Hour {
		t.Fatalf("bufferedEventRetention = %v", bufferedEventRetention)
	}
	if outgoingEventRetention != 7*24*time.Hour {
		t.Fatalf("outgoingEventRetention = %v", outgoingEventRetention)
	}
}
