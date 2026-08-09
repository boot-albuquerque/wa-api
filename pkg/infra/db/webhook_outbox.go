package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Outbox de entrega de webhook (ADR-0005, D3).
//
// O que ele resolve: hoje o reagendamento vive só em memória, num
// `time.AfterFunc` (dispatch_retry.go). Todo reinício do processo perde o que
// estava pendente, sem log e sem contagem — e sob k8s reinício é rotina, não
// exceção. Um deploy no meio de um pico descarta silenciosamente as entregas
// em voo.
//
// Por que outbox e não broker: durabilidade exige armazenamento TRANSACIONAL,
// não fila. SQLite é um. Isso é o que permite ao cenário catastrófico do ADR —
// um pod, SQLite, nada mais — manter a mesma GARANTIA que a configuração
// completa, mudando só a escala.

// driverPostgres é o nome que o sqlx devolve para o driver do Postgres. Nomeado
// porque a comparação decide se o `FOR UPDATE SKIP LOCKED` entra, e literal
// solto num `if` de dialeto é o tipo de coisa que ninguém revisa duas vezes.
const driverPostgres = "postgres"

const (
	// claimLease é por quanto tempo uma linha reivindicada deixa de ser
	// elegível para outra réplica.
	//
	// É teto de trabalho, não estimativa: precisa cobrir a tentativa mais lenta
	// que a entrega admite. Curto demais e duas réplicas entregam o mesmo
	// evento; longo demais e uma linha órfã (processo morto no meio) espera à
	// toa antes de alguém retomá-la.
	//
	// Não há estado "em processamento" separado, de propósito: um estado assim
	// fica PRESO quando o processo morre entre marcar e entregar, e alguém
	// precisa de um varredor para destravá-lo. Empurrar `due_at` faz o prazo
	// vencer sozinho.
	claimLease = 2 * time.Minute

	// claimBatch limita quantas linhas uma varredura reivindica de uma vez.
	// Sem teto, um processo que sobe depois de uma indisponibilidade longa
	// reivindicaria a fila inteira e a seguraria por um claimLease, mesmo sem
	// vazão para entregá-la.
	claimBatch = 64
)

// OutboxEntry é uma entrega pendente.
//
// A chave HMAC NÃO está aqui, e a ausência é deliberada: ela vive em
// `users.hmac_key` e é relida por UserID na retomada. Duplicar segredo em outra
// tabela multiplica a superfície de vazamento (backup, réplica, dump de
// suporte) sem comprar nada.
type OutboxEntry struct {
	ID      string
	UserID  string
	URL     string
	Payload map[string]string
	Attempt int
	DueAt   time.Time
}

// WebhookOutboxRepository persiste entregas pendentes.
type WebhookOutboxRepository struct {
	db *sqlx.DB
}

// NewWebhookOutboxRepository cria o repositório.
func NewWebhookOutboxRepository(db *sqlx.DB) *WebhookOutboxRepository {
	return &WebhookOutboxRepository{db: db}
}

// Enqueue grava a INTENÇÃO de entregar, antes da primeira tentativa.
//
// A ordem importa e é o ponto do mecanismo: gravar depois de tentar deixaria
// uma janela em que o processo morre com a entrega em voo e nenhum registro
// dela. Gravar antes admite o problema oposto — entregar duas vezes se o
// processo morrer entre entregar e apagar —, e essa é a troca correta: webhook
// é `at-least-once` por natureza, porque o cliente pode receber e não responder.
func (r *WebhookOutboxRepository) Enqueue(ctx context.Context, e OutboxEntry) error {
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return fmt.Errorf("webhook outbox: serializar payload: %w", err)
	}

	now := time.Now().UTC()
	dueAt := e.DueAt
	if dueAt.IsZero() {
		dueAt = now
	}

	query := r.db.Rebind(`INSERT INTO webhook_outbox
		(id, user_id, url, payload, attempt, due_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)

	if _, err := r.db.ExecContext(ctx, query,
		e.ID, e.UserID, e.URL, string(payload), e.Attempt, dueAt.UTC(), now); err != nil {
		return fmt.Errorf("webhook outbox: enfileirar %s: %w", e.ID, err)
	}
	return nil
}

// Reschedule adia a linha e registra a tentativa gasta.
//
// Usado quando uma tentativa falha e ainda há orçamento. Não recria a linha: o
// `id` é o mesmo do começo ao fim, então o mesmo evento nunca vira duas linhas
// por ter falhado.
func (r *WebhookOutboxRepository) Reschedule(ctx context.Context, id string, attempt int, dueAt time.Time) error {
	query := r.db.Rebind(`UPDATE webhook_outbox SET attempt = ?, due_at = ? WHERE id = ?`)
	if _, err := r.db.ExecContext(ctx, query, attempt, dueAt.UTC(), id); err != nil {
		return fmt.Errorf("webhook outbox: reagendar %s: %w", id, err)
	}
	return nil
}

// Delete remove a linha. É o passo final tanto do sucesso quanto do caminho
// terminal: entregue ou esgotado, não há mais o que retomar.
func (r *WebhookOutboxRepository) Delete(ctx context.Context, id string) error {
	query := r.db.Rebind(`DELETE FROM webhook_outbox WHERE id = ?`)
	if _, err := r.db.ExecContext(ctx, query, id); err != nil {
		return fmt.Errorf("webhook outbox: apagar %s: %w", id, err)
	}
	return nil
}

// ClaimDue reivindica até claimBatch entregas vencidas e as devolve.
//
// Reivindicar EMPURRA `due_at` para agora + claimLease, na mesma transação em
// que as linhas são lidas. É isso que impede duas réplicas de entregarem o
// mesmo evento sem manter transação aberta durante a entrega — que pode levar
// segundos e não pode segurar conexão do banco.
//
// No Postgres o `FOR UPDATE SKIP LOCKED` faz réplicas concorrentes pegarem
// conjuntos DISJUNTOS em vez de esperarem umas pelas outras. O SQLite não o
// conhece, e não precisa: `single` é um processo só, garantido pela trava de
// instância única (D1). A diferença é de VAZÃO, não de semântica — as duas
// pontas reivindicam antes de entregar.
func (r *WebhookOutboxRepository) ClaimDue(ctx context.Context) ([]OutboxEntry, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("webhook outbox: abrir transacao: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()

	selectSQL := `SELECT id, user_id, url, payload, attempt
		FROM webhook_outbox WHERE due_at <= ? ORDER BY due_at LIMIT ?`
	if r.db.DriverName() == driverPostgres {
		selectSQL += " FOR UPDATE SKIP LOCKED"
	}

	rows, err := tx.QueryxContext(ctx, tx.Rebind(selectSQL), now, claimBatch)
	if err != nil {
		return nil, fmt.Errorf("webhook outbox: selecionar vencidas: %w", err)
	}

	entries, err := scanOutboxRows(rows)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	if err := pushDueAt(ctx, tx, entries, now.Add(claimLease)); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("webhook outbox: confirmar reivindicacao: %w", err)
	}
	return entries, nil
}

// scanOutboxRows lê o resultset e SEMPRE fecha as linhas antes do UPDATE que
// vem depois. O SQLite não aceita escrever numa tabela com cursor de leitura
// aberto sobre ela na mesma conexão — deixar o Close para um defer no chamador
// funcionaria no Postgres e travaria no SQLite, que é a configuração do cenário
// catastrófico.
func scanOutboxRows(rows *sqlx.Rows) ([]OutboxEntry, error) {
	defer func() { _ = rows.Close() }()

	var entries []OutboxEntry
	for rows.Next() {
		var (
			e   OutboxEntry
			raw string
		)
		if err := rows.Scan(&e.ID, &e.UserID, &e.URL, &raw, &e.Attempt); err != nil {
			return nil, fmt.Errorf("webhook outbox: ler linha: %w", err)
		}
		if err := json.Unmarshal([]byte(raw), &e.Payload); err != nil {
			// Uma linha ilegível não pode derrubar o lote inteiro: ela ficaria
			// para sempre bloqueando entregas saudáveis atrás dela. Sai do lote
			// e continua vencida, para aparecer na varredura seguinte — e o erro
			// vai para quem chama, que decide se loga.
			return nil, fmt.Errorf("webhook outbox: payload ilegivel em %s: %w", e.ID, err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("webhook outbox: iterar linhas: %w", err)
	}
	return entries, nil
}

// pushDueAt empurra o prazo das linhas reivindicadas.
func pushDueAt(ctx context.Context, tx *sqlx.Tx, entries []OutboxEntry, until time.Time) error {
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ID)
	}

	query, args, err := sqlx.In(`UPDATE webhook_outbox SET due_at = ? WHERE id IN (?)`, until, ids)
	if err != nil {
		return fmt.Errorf("webhook outbox: montar update de reivindicacao: %w", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(query), args...); err != nil {
		return fmt.Errorf("webhook outbox: reivindicar: %w", err)
	}
	return nil
}

// PendingCount conta o que está no outbox. Existe para observabilidade e para
// os testes: sem ele, "o outbox esvaziou" só é verificável indiretamente.
func (r *WebhookOutboxRepository) PendingCount(ctx context.Context) (int, error) {
	var n int
	if err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM webhook_outbox`); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("webhook outbox: contar pendentes: %w", err)
	}
	return n, nil
}
