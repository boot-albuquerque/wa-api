package db

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

// ADR-0005 D3. Estes testes rodam contra SQLite REAL, como os de
// message_history e pelo mesmo motivo: o que decide aqui é SQL — quem casa com
// `due_at <= now`, o que o UPDATE de reivindicação alcança, o round-trip do
// payload. Um dublê de driver validaria a string da query, não a consulta.
//
// O ramo `FOR UPDATE SKIP LOCKED` do Postgres fica descoberto por estes testes,
// e isso está registrado, não escondido: ele muda VAZÃO sob concorrência, não
// semântica. A propriedade que importa — reivindicar antes de entregar — é
// exercitada nas duas pontas, e é ela que os testes abaixo travam.

func newOutboxDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db := openTestDB(t)
	if err := InitializeSchema(db); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	return db
}

func sampleEntry(id string) OutboxEntry {
	return OutboxEntry{
		ID:      id,
		UserID:  "user-1",
		URL:     "https://example.invalid/hook",
		Payload: map[string]string{"jsonData": `{"event":"Message"}`, "type": "Message"},
	}
}

func TestOutbox_EnqueueThenClaim(t *testing.T) {
	db := newOutboxDB(t)
	repo := NewWebhookOutboxRepository(db)
	ctx := context.Background()

	if err := repo.Enqueue(ctx, sampleEntry("e1")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	claimed, err := repo.ClaimDue(ctx)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("reivindicadas = %d, want 1", len(claimed))
	}

	got := claimed[0]
	if got.ID != "e1" || got.UserID != "user-1" || got.URL != "https://example.invalid/hook" {
		t.Errorf("entrada veio corrompida: %+v", got)
	}
	// O payload é o que efetivamente será entregue: se ele não sobrevive ao
	// round-trip, a durabilidade não vale nada — a linha existiria e a entrega
	// sairia errada.
	if got.Payload["jsonData"] != `{"event":"Message"}` || got.Payload["type"] != "Message" {
		t.Errorf("payload não sobreviveu ao round-trip: %+v", got.Payload)
	}
}

// TestOutbox_ClaimIsExclusive é o teste central do desenho. Reivindicar tem de
// EMPURRAR `due_at`, senão duas réplicas (ou duas varreduras da mesma) pegam a
// mesma linha e o cliente recebe o mesmo webhook duas vezes.
//
// A segunda chamada é imediata, sem espera: é exatamente o que acontece quando
// duas réplicas varrem ao mesmo tempo.
func TestOutbox_ClaimIsExclusive(t *testing.T) {
	db := newOutboxDB(t)
	repo := NewWebhookOutboxRepository(db)
	ctx := context.Background()

	if err := repo.Enqueue(ctx, sampleEntry("e1")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	first, err := repo.ClaimDue(ctx)
	if err != nil {
		t.Fatalf("primeira ClaimDue: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("primeira reivindicação = %d, want 1", len(first))
	}

	second, err := repo.ClaimDue(ctx)
	if err != nil {
		t.Fatalf("segunda ClaimDue: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("a mesma entrega foi reivindicada duas vezes (%d); o cliente receberia o webhook em duplicata", len(second))
	}

	// E continua existindo: reivindicar não é entregar. Se o processo morrer
	// agora, a linha tem de voltar quando o prazo vencer.
	n, err := repo.PendingCount(ctx)
	if err != nil {
		t.Fatalf("PendingCount: %v", err)
	}
	if n != 1 {
		t.Errorf("pendentes = %d, want 1: reivindicar apagou a linha, e uma queda agora perderia a entrega", n)
	}
}

// TestOutbox_NotYetDueIsNotClaimed: o backoff só vale se a linha adiada for
// mesmo ignorada. Sem isto, reagendar não espaçaria nada e a tentativa sairia
// em rajada contra um destino que já está com problema.
func TestOutbox_NotYetDueIsNotClaimed(t *testing.T) {
	db := newOutboxDB(t)
	repo := NewWebhookOutboxRepository(db)
	ctx := context.Background()

	e := sampleEntry("e1")
	e.DueAt = time.Now().UTC().Add(time.Hour)
	if err := repo.Enqueue(ctx, e); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	claimed, err := repo.ClaimDue(ctx)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("entrega ainda não vencida foi reivindicada (%d)", len(claimed))
	}
}

func TestOutbox_RescheduleDelaysAndCountsTheAttempt(t *testing.T) {
	db := newOutboxDB(t)
	repo := NewWebhookOutboxRepository(db)
	ctx := context.Background()

	if err := repo.Enqueue(ctx, sampleEntry("e1")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := repo.Reschedule(ctx, "e1", 3, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("Reschedule: %v", err)
	}

	if claimed, err := repo.ClaimDue(ctx); err != nil {
		t.Fatalf("ClaimDue: %v", err)
	} else if len(claimed) != 0 {
		t.Fatalf("entrega reagendada para daqui a uma hora foi reivindicada agora")
	}

	// E quando o prazo vencer, volta com a tentativa gasta — senão o orçamento
	// de tentativas reiniciaria a cada retomada e um destino morto seria
	// tentado para sempre.
	if err := repo.Reschedule(ctx, "e1", 3, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("Reschedule (vencida): %v", err)
	}
	claimed, err := repo.ClaimDue(ctx)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("reivindicadas = %d, want 1", len(claimed))
	}
	if claimed[0].Attempt != 3 {
		t.Errorf("attempt = %d, want 3: o orçamento de tentativas reiniciaria a cada retomada", claimed[0].Attempt)
	}
}

func TestOutbox_DeleteRemoves(t *testing.T) {
	db := newOutboxDB(t)
	repo := NewWebhookOutboxRepository(db)
	ctx := context.Background()

	if err := repo.Enqueue(ctx, sampleEntry("e1")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := repo.Delete(ctx, "e1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	n, err := repo.PendingCount(ctx)
	if err != nil {
		t.Fatalf("PendingCount: %v", err)
	}
	if n != 0 {
		t.Errorf("pendentes = %d, want 0: a entrega concluída seria retomada para sempre", n)
	}
}

// TestOutbox_ClaimRespectsBatchCeiling: sem teto, um processo que sobe depois
// de uma indisponibilidade longa reivindica a fila inteira e a segura por um
// claimLease — mesmo sem vazão para entregá-la, e mesmo que outras réplicas
// estejam ociosas.
func TestOutbox_ClaimRespectsBatchCeiling(t *testing.T) {
	db := newOutboxDB(t)
	repo := NewWebhookOutboxRepository(db)
	ctx := context.Background()

	for i := 0; i < claimBatch+10; i++ {
		e := sampleEntry("e" + time.Duration(i).String())
		if err := repo.Enqueue(ctx, e); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	claimed, err := repo.ClaimDue(ctx)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(claimed) != claimBatch {
		t.Errorf("reivindicadas = %d, want %d (teto do lote)", len(claimed), claimBatch)
	}
}

// TestOutbox_EmptyOutboxClaimsNothing fecha o caso de repouso: a varredura roda
// periodicamente e, no estado normal, não encontra nada. Um erro aqui apareceria
// como ruído constante no log de um sistema saudável.
func TestOutbox_EmptyOutboxClaimsNothing(t *testing.T) {
	repo := NewWebhookOutboxRepository(newOutboxDB(t))

	claimed, err := repo.ClaimDue(context.Background())
	if err != nil {
		t.Fatalf("ClaimDue num outbox vazio devolveu erro: %v", err)
	}
	if len(claimed) != 0 {
		t.Errorf("reivindicadas = %d, want 0", len(claimed))
	}
}

// TestOutbox_ScopeSurvivesRoundTrip: sem isto, a retomada assinaria com a chave
// errada. O webhook global e o do usuário usam chaves DIFERENTES, e o
// discriminador é a única coisa que distingue os dois depois que o processo
// morreu — a URL não serve, porque a configuração pode ter mudado.
func TestOutbox_ScopeSurvivesRoundTrip(t *testing.T) {
	db := newOutboxDB(t)
	repo := NewWebhookOutboxRepository(db)
	ctx := context.Background()

	global := sampleEntry("e-global")
	global.Scope = HMACScopeGlobal
	if err := repo.Enqueue(ctx, global); err != nil {
		t.Fatalf("Enqueue global: %v", err)
	}

	user := sampleEntry("e-user")
	user.Scope = HMACScopeUser
	if err := repo.Enqueue(ctx, user); err != nil {
		t.Fatalf("Enqueue user: %v", err)
	}

	claimed, err := repo.ClaimDue(ctx)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("reivindicadas = %d, want 2", len(claimed))
	}

	got := map[string]HMACScope{}
	for _, e := range claimed {
		got[e.ID] = e.Scope
	}
	if got["e-global"] != HMACScopeGlobal {
		t.Errorf("scope de e-global = %q, want %q: a entrega global seria assinada com a chave do usuário", got["e-global"], HMACScopeGlobal)
	}
	if got["e-user"] != HMACScopeUser {
		t.Errorf("scope de e-user = %q, want %q", got["e-user"], HMACScopeUser)
	}
}

// TestOutbox_EmptyScopeDefaultsToUser fixa o padrão. Quem enfileira sem
// declarar o escopo tem a maioria dos casos — o webhook do usuário —, e o
// silêncio não pode virar "assine com a chave global".
func TestOutbox_EmptyScopeDefaultsToUser(t *testing.T) {
	repo := NewWebhookOutboxRepository(newOutboxDB(t))
	ctx := context.Background()

	if err := repo.Enqueue(ctx, sampleEntry("e1")); err != nil { // Scope zero
		t.Fatalf("Enqueue: %v", err)
	}

	claimed, err := repo.ClaimDue(ctx)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Scope != HMACScopeUser {
		t.Fatalf("scope = %q, want %q", claimed[0].Scope, HMACScopeUser)
	}
}
