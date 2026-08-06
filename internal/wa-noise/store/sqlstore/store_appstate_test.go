// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package sqlstore

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"wa-api/internal/wa-noise/store"
)

const testCollection = "regular"

func testHash(fill byte) [appStateHashLength]byte {
	var h [appStateHashLength]byte
	for i := range h {
		h[i] = fill
	}
	return h
}

func testMAC(fill byte) []byte {
	return bytes.Repeat([]byte{fill}, 32)
}

// whatsmeow_app_state_mutation_macs tem FK (jid, name) para
// whatsmeow_app_state_version: nao da' para gravar MAC de mutacao de uma
// colecao cuja versao ainda nao foi persistida.
func seedAppStateVersion(t *testing.T, s *SQLStore, name string) {
	t.Helper()
	if err := s.PutAppStateVersion(context.Background(), name, 1, testHash(0)); err != nil {
		t.Fatalf("PutAppStateVersion (seed %s): %v", name, err)
	}
}

func TestGetAppStateVersionUnknownIsZeroWithoutError(t *testing.T) {
	version, hash, err := newTestStore(t).GetAppStateVersion(context.Background(), testCollection)
	if err != nil {
		t.Fatalf("GetAppStateVersion: %v", err)
	}
	if version != 0 {
		t.Fatalf("versao inicial deveria ser 0, veio %d", version)
	}
	if hash != ([appStateHashLength]byte{}) {
		t.Fatal("hash inicial deveria ser todo zero")
	}
}

func TestPutGetAppStateVersionRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	want := testHash(0xAB)
	if err := s.PutAppStateVersion(ctx, testCollection, 42, want); err != nil {
		t.Fatalf("PutAppStateVersion: %v", err)
	}
	version, hash, err := s.GetAppStateVersion(ctx, testCollection)
	if err != nil {
		t.Fatalf("GetAppStateVersion: %v", err)
	}
	if version != 42 {
		t.Fatalf("versao = %d", version)
	}
	if hash != want {
		t.Fatal("o hash nao voltou igual")
	}
}

func TestPutAppStateVersionOverwrites(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.PutAppStateVersion(ctx, testCollection, 1, testHash(1)); err != nil {
		t.Fatalf("Put 1: %v", err)
	}
	if err := s.PutAppStateVersion(ctx, testCollection, 2, testHash(2)); err != nil {
		t.Fatalf("Put 2: %v", err)
	}
	version, hash, err := s.GetAppStateVersion(ctx, testCollection)
	if err != nil {
		t.Fatalf("GetAppStateVersion: %v", err)
	}
	if version != 2 || hash != testHash(2) {
		t.Fatalf("ON CONFLICT DO UPDATE nao substituiu: versao=%d", version)
	}
}

// Versao 0 GRAVADA (diferente de "nao existe") e' estado invalido: significa
// que o hash foi persistido sem a versao correspondente, e reaplicar patches a
// partir dele corromperia o app state.
func TestGetAppStateVersionRejectsStoredVersionZero(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.PutAppStateVersion(ctx, testCollection, 0, testHash(3)); err != nil {
		t.Fatalf("PutAppStateVersion: %v", err)
	}
	_, _, err := s.GetAppStateVersion(ctx, testCollection)
	if err == nil {
		t.Fatal("versao 0 gravada deveria devolver erro")
	}
}

func TestGetAppStateVersionRejectsWrongHashLength(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	// O schema tem CHECK(length(hash) = 128); provamos que o CHECK e' quem
	// impede a chegada de um hash curto (tornando ErrInvalidLength inalcancavel).
	_, err := s.db.Exec(ctx, putAppStateVersionQuery, s.JID, testCollection, 1, []byte{1, 2, 3})
	if err == nil {
		t.Fatal("o schema deveria recusar hash com tamanho != 128")
	}
}

func TestDeleteAppStateVersionResetsToInitialState(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.PutAppStateVersion(ctx, testCollection, 7, testHash(4)); err != nil {
		t.Fatalf("PutAppStateVersion: %v", err)
	}
	if err := s.DeleteAppStateVersion(ctx, testCollection); err != nil {
		t.Fatalf("DeleteAppStateVersion: %v", err)
	}
	version, hash, err := s.GetAppStateVersion(ctx, testCollection)
	if err != nil {
		t.Fatalf("GetAppStateVersion: %v", err)
	}
	if version != 0 || hash != ([appStateHashLength]byte{}) {
		t.Fatalf("apos o delete deveria voltar ao estado inicial, veio versao=%d", version)
	}
}

func TestGetAppStateMutationMACMissingReturnsNil(t *testing.T) {
	mac, err := newTestStore(t).GetAppStateMutationMAC(context.Background(), testCollection, testMAC(1))
	if err != nil {
		t.Fatalf("GetAppStateMutationMAC: %v", err)
	}
	if mac != nil {
		t.Fatalf("MAC inexistente deveria devolver nil, veio %x", mac)
	}
}

func TestPutGetAppStateMutationMACRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedAppStateVersion(t, s, testCollection)
	index, value := testMAC(1), testMAC(2)
	err := s.PutAppStateMutationMACs(ctx, testCollection, 1, []store.AppStateMutationMAC{
		{IndexMAC: index, ValueMAC: value},
	})
	if err != nil {
		t.Fatalf("PutAppStateMutationMACs: %v", err)
	}
	got, err := s.GetAppStateMutationMAC(ctx, testCollection, index)
	if err != nil {
		t.Fatalf("GetAppStateMutationMAC: %v", err)
	}
	if !bytes.Equal(got, value) {
		t.Fatalf("value MAC = %x, esperava %x", got, value)
	}
}

// GetAppStateMutationMAC faz ORDER BY version DESC LIMIT 1: quando o mesmo
// index MAC foi gravado em varias versoes, vale o da versao mais alta.
func TestGetAppStateMutationMACReturnsHighestVersion(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedAppStateVersion(t, s, testCollection)
	index := testMAC(1)
	for version, fill := range map[uint64]byte{1: 0x10, 3: 0x30, 2: 0x20} {
		err := s.PutAppStateMutationMACs(ctx, testCollection, version, []store.AppStateMutationMAC{
			{IndexMAC: index, ValueMAC: testMAC(fill)},
		})
		if err != nil {
			t.Fatalf("Put versao %d: %v", version, err)
		}
	}
	got, err := s.GetAppStateMutationMAC(ctx, testCollection, index)
	if err != nil {
		t.Fatalf("GetAppStateMutationMAC: %v", err)
	}
	if !bytes.Equal(got, testMAC(0x30)) {
		t.Fatalf("esperava o MAC da versao 3, veio %x", got[:4])
	}
}

func TestPutAppStateMutationMACsEmptyIsNoOp(t *testing.T) {
	if err := newTestStore(t).PutAppStateMutationMACs(context.Background(), testCollection, 1, nil); err != nil {
		t.Fatalf("PutAppStateMutationMACs vazio: %v", err)
	}
}

// Mais mutacoes do que mutationBatchSize: prova que slices.Chunk fecha a
// transacao com TODOS os lotes gravados, nao so' o primeiro.
func TestPutAppStateMutationMACsChunksLargeBatches(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedAppStateVersion(t, s, testCollection)
	total := mutationBatchSize + 10
	mutations := make([]store.AppStateMutationMAC, total)
	for i := range mutations {
		index := bytes.Repeat([]byte{0}, 32)
		index[0] = byte(i % 256)
		index[1] = byte(i / 256)
		mutations[i] = store.AppStateMutationMAC{IndexMAC: index, ValueMAC: testMAC(9)}
	}
	if err := s.PutAppStateMutationMACs(ctx, testCollection, 1, mutations); err != nil {
		t.Fatalf("PutAppStateMutationMACs: %v", err)
	}
	// Confere o primeiro e o ultimo — um em cada lote.
	for _, i := range []int{0, total - 1} {
		got, err := s.GetAppStateMutationMAC(ctx, testCollection, mutations[i].IndexMAC)
		if err != nil {
			t.Fatalf("GetAppStateMutationMAC %d: %v", i, err)
		}
		if !bytes.Equal(got, testMAC(9)) {
			t.Fatalf("mutacao %d de %d nao foi gravada", i, total)
		}
	}
}

func TestDeleteAppStateMutationMACs(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedAppStateVersion(t, s, testCollection)
	indexA, indexB := testMAC(1), testMAC(2)
	err := s.PutAppStateMutationMACs(ctx, testCollection, 1, []store.AppStateMutationMAC{
		{IndexMAC: indexA, ValueMAC: testMAC(0xA)},
		{IndexMAC: indexB, ValueMAC: testMAC(0xB)},
	})
	if err != nil {
		t.Fatalf("PutAppStateMutationMACs: %v", err)
	}
	if err = s.DeleteAppStateMutationMACs(ctx, testCollection, [][]byte{indexA}); err != nil {
		t.Fatalf("DeleteAppStateMutationMACs: %v", err)
	}
	if got, gErr := s.GetAppStateMutationMAC(ctx, testCollection, indexA); gErr != nil || got != nil {
		t.Fatalf("indexA deveria ter sumido (got=%x err=%v)", got, gErr)
	}
	if got, gErr := s.GetAppStateMutationMAC(ctx, testCollection, indexB); gErr != nil || got == nil {
		t.Fatalf("indexB deveria continuar (got=%x err=%v)", got, gErr)
	}
}

func TestDeleteAppStateMutationMACsEmptyIsNoOp(t *testing.T) {
	if err := newTestStore(t).DeleteAppStateMutationMACs(context.Background(), testCollection, nil); err != nil {
		t.Fatalf("DeleteAppStateMutationMACs vazio: %v", err)
	}
}

func TestAppStateMutationMACsAreScopedPerCollection(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedAppStateVersion(t, s, "critical_block")
	index := testMAC(1)
	err := s.PutAppStateMutationMACs(ctx, "critical_block", 1, []store.AppStateMutationMAC{
		{IndexMAC: index, ValueMAC: testMAC(5)},
	})
	if err != nil {
		t.Fatalf("PutAppStateMutationMACs: %v", err)
	}
	got, err := s.GetAppStateMutationMAC(ctx, testCollection, index)
	if err != nil {
		t.Fatalf("GetAppStateMutationMAC: %v", err)
	}
	if got != nil {
		t.Fatalf("o MAC de uma colecao vazou para outra: %x", got)
	}
}

func TestAppStateVersionIsScopedPerJID(t *testing.T) {
	ctx := context.Background()
	container, _, s := newTestDevice(t)
	if err := s.PutAppStateVersion(ctx, testCollection, 5, testHash(1)); err != nil {
		t.Fatalf("PutAppStateVersion: %v", err)
	}
	other := NewSQLStore(container, mustJID(t, "5511888888888", "s.whatsapp.net"))
	version, _, err := other.GetAppStateVersion(ctx, testCollection)
	if err != nil {
		t.Fatalf("GetAppStateVersion outro jid: %v", err)
	}
	if version != 0 {
		t.Fatalf("a versao de app state de um jid vazou para outro: %d", version)
	}
}

func TestPutAppStateMutationMACsUsesSQLitePlaceholderSyntax(t *testing.T) {
	// O driver de SQLite exige ?N para reusar o mesmo parametro em varias
	// linhas do VALUES; com $N o INSERT de multiplas mutacoes falharia. O teste
	// acima de chunking ja' exercita isso na pratica, aqui travamos o formato.
	got := fmt.Sprintf(mutationMACPlaceholderSQLite, 4, 5)
	if got != "(?1, ?2, ?3, ?4, ?5)" {
		t.Fatalf("placeholder SQLite = %q", got)
	}
	got = fmt.Sprintf(mutationMACPlaceholderPostgres, 4, 5)
	if got != "($1, $2, $3, $4, $5)" {
		t.Fatalf("placeholder Postgres = %q", got)
	}
}

// A FK (jid, name) tem ON DELETE CASCADE: apagar a versao de uma colecao leva
// junto todos os MACs de mutacao dela. E' isso que torna DeleteAppStateVersion
// um reset completo da colecao, e nao so' do contador.
func TestDeleteAppStateVersionCascadesToMutationMACs(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedAppStateVersion(t, s, testCollection)
	index := testMAC(1)
	err := s.PutAppStateMutationMACs(ctx, testCollection, 1, []store.AppStateMutationMAC{
		{IndexMAC: index, ValueMAC: testMAC(2)},
	})
	if err != nil {
		t.Fatalf("PutAppStateMutationMACs: %v", err)
	}
	if err = s.DeleteAppStateVersion(ctx, testCollection); err != nil {
		t.Fatalf("DeleteAppStateVersion: %v", err)
	}
	got, err := s.GetAppStateMutationMAC(ctx, testCollection, index)
	if err != nil {
		t.Fatalf("GetAppStateMutationMAC: %v", err)
	}
	if got != nil {
		t.Fatalf("o CASCADE deveria ter apagado o MAC junto com a versao: %x", got)
	}
}

// PutAppStateMutationMACs para uma colecao sem versao persistida falha na FK.
// Nao e' um bug: e' o schema impedindo MAC orfao, que ficaria invisivel para
// DeleteAppStateVersion e sobreviveria a um reset da colecao.
func TestPutAppStateMutationMACsRequiresExistingVersionRow(t *testing.T) {
	err := newTestStore(t).PutAppStateMutationMACs(
		context.Background(), "colecao_sem_versao", 1,
		[]store.AppStateMutationMAC{{IndexMAC: testMAC(1), ValueMAC: testMAC(2)}},
	)
	if err == nil {
		t.Fatal("esperava erro de foreign key para colecao sem versao persistida")
	}
}
