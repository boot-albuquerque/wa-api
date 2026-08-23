package db

import (
	"context"
	"fmt"
	"testing"
	"time"
	"wa-api/pkg/domain"

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

// --- F97 etapa 2: apagar o texto claro das linhas existentes ---------------

func novoBancoMigrado(t *testing.T) *sqlx.DB {
	t.Helper()
	db := openTestDB(t)
	if err := InitializeSchema(db); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	return db
}

// rodarMigracao16 executa a migração sobre um banco JÁ migrado.
//
// Chama a função direto, numa transação, em vez de applyMigration: o
// InitializeSchema acima já aplicou a 16 e registrou o id, então reaplicá-la
// falha na chave primária de `migrations` — o que, de quebra, é a evidência de
// que ela está fiada na lista e roda de verdade na subida do processo.
//
// As linhas legadas precisam existir DEPOIS disso, porque o repositório não
// grava mais texto claro: o estado que a migração conserta não é mais
// produzível pelo caminho normal.
func rodarMigracao16(t *testing.T, db *sqlx.DB) error {
	t.Helper()

	tx, err := db.Beginx()
	if err != nil {
		t.Fatalf("abrir transacao: %v", err)
	}
	if err := applyBlankPlaintextTokenMigration(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return nil
}

// TestMigracao16_ApagaOTextoClaroPreservandoOAcesso é o teste da F97 etapa 2.
//
// As duas metades, e uma sem a outra é inútil ou destrutiva: o texto claro sai
// do disco E o usuário continua conseguindo autenticar.
func TestMigracao16_ApagaOTextoClaroPreservandoOAcesso(t *testing.T) {
	db := novoBancoMigrado(t)

	// Linha no estado ANTIGO: texto claro presente, gravada antes da etapa 1.
	// Escrita direto, e não pelo repositório, porque o repositório já não grava
	// texto claro — o cenário que a migração conserta não é mais produzível
	// pelo caminho normal.
	const token = "token-legado"
	if _, err := db.Exec(
		`INSERT INTO users (id,name,token,token_hash,webhook,jid,qrcode,events,proxy_url,history)
		 VALUES ('u-legado','legado',?,?, '','','','All','',0)`,
		token, domain.HashToken(token)); err != nil {
		t.Fatalf("inserir linha legada: %v", err)
	}

	if err := rodarMigracao16(t, db); err != nil {
		t.Fatalf("migracao 16: %v", err)
	}

	var linha struct {
		Token     string `db:"token"`
		TokenHash string `db:"token_hash"`
	}
	if err := db.Get(&linha, "SELECT token, token_hash FROM users WHERE id = 'u-legado'"); err != nil {
		t.Fatalf("reler linha: %v", err)
	}

	if linha.Token != "" {
		t.Errorf("token = %q, want vazio: a credencial continua legivel em disco", linha.Token)
	}
	if want := domain.HashToken(token); linha.TokenHash != want {
		t.Fatalf("token_hash = %q, want %q: o usuario perdeu a forma de autenticar", linha.TokenHash, want)
	}
}

// TestMigracao16_PreencheOHashQueFalta cobre a linha que tem texto claro e NÃO
// tem hash — a que a migração precisa salvar antes de apagar.
func TestMigracao16_PreencheOHashQueFalta(t *testing.T) {
	db := novoBancoMigrado(t)

	const token = "sem-hash"
	if _, err := db.Exec(
		`INSERT INTO users (id,name,token,token_hash,webhook,jid,qrcode,events,proxy_url,history)
		 VALUES ('u-sem-hash','sem hash',?,NULL,'','','','All','',0)`, token); err != nil {
		t.Fatalf("inserir linha sem hash: %v", err)
	}

	if err := rodarMigracao16(t, db); err != nil {
		t.Fatalf("migracao 16: %v", err)
	}

	var linha struct {
		Token     string `db:"token"`
		TokenHash string `db:"token_hash"`
	}
	if err := db.Get(&linha, "SELECT token, token_hash FROM users WHERE id = 'u-sem-hash'"); err != nil {
		t.Fatalf("reler linha: %v", err)
	}
	if want := domain.HashToken(token); linha.TokenHash != want {
		t.Errorf("token_hash = %q, want %q: o hash nao foi preenchido antes de apagar", linha.TokenHash, want)
	}
	if linha.Token != "" {
		t.Errorf("token = %q, want vazio", linha.Token)
	}
}

// TestMigracao16_PassaComBancoEmOrdem é o controle na direção oposta dos dois
// acima: uma migração que "protege" recusando sempre não protege nada.
func TestMigracao16_PassaComBancoEmOrdem(t *testing.T) {
	db := novoBancoMigrado(t)

	if _, err := db.Exec(
		`INSERT INTO users (id,name,token,token_hash,webhook,jid,qrcode,events,proxy_url,history)
		 VALUES ('u-ok','ok','',?, '','','','All','',0)`, domain.HashToken("tok-ok")); err != nil {
		t.Fatalf("inserir: %v", err)
	}

	if err := rodarMigracao16(t, db); err != nil {
		t.Fatalf("migracao 16 recusou um banco em ordem: %v", err)
	}
}

// TestMigracao16_FalhaEmVezDeDestruir prova que a migração NÃO apaga quando
// não consegue garantir o acesso.
//
// O cenário é duas linhas com o MESMO token em claro e sem hash — estado
// possível porque o índice único é sobre `token_hash`, e ele era NULL nas duas.
// O preenchimento calcula o mesmo hash para ambas e a segunda viola o índice.
//
// O que importa é o DESFECHO: a transação inteira volta atrás, e o texto claro
// continua lá. Uma migração que apagasse primeiro e falhasse depois deixaria
// dois usuários sem acesso e sem como recuperar — o valor não existe em
// nenhum outro lugar.
//
// O que este teste NÃO cobre, e vale dizer em vez de deixar implícito: a
// verificação `semHash > 0` da migração. Aqui o UPDATE do preenchimento falha
// ANTES dela, no índice único. Desligar a verificação não muda o resultado
// deste teste — confirmado por controle negativo. Ela é defesa em profundidade
// sobre uma operação irreversível, e está documentada como tal no código.
func TestMigracao16_FalhaEmVezDeDestruir(t *testing.T) {
	db := novoBancoMigrado(t)

	for _, id := range []string{"u-a", "u-b"} {
		if _, err := db.Exec(
			`INSERT INTO users (id,name,token,token_hash,webhook,jid,qrcode,events,proxy_url,history)
			 VALUES (?,?,'token-repetido',NULL,'','','','All','',0)`, id, id); err != nil {
			t.Fatalf("inserir %s: %v", id, err)
		}
	}

	if err := rodarMigracao16(t, db); err == nil {
		t.Fatal("a migracao passou com duas linhas de token repetido; uma delas ficaria sem hash")
	}

	// E o texto claro continua lá: a transação voltou atrás por inteiro.
	var restantes int
	if err := db.Get(&restantes, `SELECT COUNT(*) FROM users WHERE token = 'token-repetido'`); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if restantes != 2 {
		t.Errorf("linhas com texto claro = %d, want 2: a migracao apagou antes de garantir o acesso", restantes)
	}
}

// TestMigracao16_ReportaFalhaDeLeitura cobre o caminho em que o banco não
// responde: a migração precisa PROPAGAR o erro, e não seguir para o apagamento
// como se não houvesse linha pendente nenhuma.
//
// Uma leitura que falha e é tratada como "nada a preencher" levaria direto ao
// UPDATE que apaga — destruindo credencial com base numa consulta que nunca
// respondeu.
func TestMigracao16_ReportaFalhaDeLeitura(t *testing.T) {
	db := novoBancoMigrado(t)

	// Sem a tabela, toda consulta da migração falha.
	if _, err := db.Exec(`DROP TABLE users`); err != nil {
		t.Fatalf("dropar users: %v", err)
	}

	if err := rodarMigracao16(t, db); err == nil {
		t.Fatal("a migracao seguiu com o banco sem a tabela users")
	}
}

// TestMigracao16_BancoVazioNaoQuebra: instalação nova não tem linha legada
// nenhuma, e a migração roda em toda subida. Um erro aqui apareceria como
// falha de arranque num sistema que não tem o problema.
func TestMigracao16_BancoVazioNaoQuebra(t *testing.T) {
	if err := rodarMigracao16(t, novoBancoMigrado(t)); err != nil {
		t.Fatalf("migracao 16 falhou num banco vazio: %v", err)
	}
}

// F106. O defeito era de cabeça de fila: uma linha com payload ilegível tem o
// `due_at` mais antigo, então `ORDER BY due_at LIMIT 64` a punha em TODO lote,
// toda `ClaimDue` falhava no scan, e nenhuma entrega saudável atrás dela era
// reivindicada — permanentemente.
//
// O teste fixa o que importa para o cliente: as entregas boas SAEM, apesar da
// ruim. Não fixa a mensagem de log nem o formato do erro.
func TestOutbox_PayloadIlegivelNaoDerrubaOLote(t *testing.T) {
	db := newOutboxDB(t)
	repo := NewWebhookOutboxRepository(db)
	ctx := context.Background()

	// A ruim entra PRIMEIRO e com `due_at` mais antigo — é a posição que
	// causava o travamento. Inserida por SQL cru porque Enqueue serializa o
	// payload corretamente e nunca produziria isto; o cenário real é corrupção
	// no disco, restauração parcial ou escrita truncada.
	antigo := time.Now().UTC().Add(-time.Hour)
	if _, err := db.ExecContext(ctx, db.Rebind(
		`INSERT INTO webhook_outbox (id, user_id, url, payload, attempt, due_at, hmac_scope, created_at)
		 VALUES (?, ?, ?, ?, 0, ?, ?, ?)`),
		"corrompida", "user-1", "https://example.invalid/hook",
		`{"jsonData": NAO_E_JSON`, antigo, string(HMACScopeUser), antigo,
	); err != nil {
		t.Fatalf("inserir a linha corrompida: %v", err)
	}

	const boas = 3
	for i := 0; i < boas; i++ {
		if err := repo.Enqueue(ctx, sampleEntry(fmt.Sprintf("boa-%d", i))); err != nil {
			t.Fatalf("Enqueue boa-%d: %v", i, err)
		}
	}

	entries, err := repo.ClaimDue(ctx)
	if err != nil {
		t.Fatalf("ClaimDue devolveu erro por causa de UMA linha ilegivel; o lote inteiro morreu: %v", err)
	}

	if len(entries) != boas {
		t.Fatalf("reivindicadas %d de %d entregas saudaveis; a linha corrompida levou as outras junto",
			len(entries), boas)
	}
	for _, e := range entries {
		if e.ID == "corrompida" {
			t.Error("a linha ilegivel foi devolvida como entrega valida; ela iria para o webhook")
		}
	}

	// A ilegível tem de ter o `due_at` EMPURRADO junto com as boas. Sem isso
	// ela volta na cabeça de todo lote, para sempre — ocupando vaga e gerando
	// uma linha de log por varredura.
	segunda, err := repo.ClaimDue(ctx)
	if err != nil {
		t.Fatalf("segunda ClaimDue: %v", err)
	}
	if len(segunda) != 0 {
		t.Errorf("segunda varredura devolveu %d entregas; nada deveria estar vencido ainda", len(segunda))
	}

	var due time.Time
	if err := db.GetContext(ctx, &due, db.Rebind(`SELECT due_at FROM webhook_outbox WHERE id = ?`), "corrompida"); err != nil {
		t.Fatalf("ler o due_at da corrompida: %v", err)
	}
	if !due.After(antigo) {
		t.Errorf("o due_at da linha ilegivel nao foi empurrado (%s); ela volta na cabeca de todo lote", due)
	}
}
