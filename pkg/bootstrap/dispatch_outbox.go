package bootstrap

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"

	"wa-api/pkg/infra/db"
)

// Fiação do outbox de webhook (ADR-0005, D3).
//
// O repositório (pkg/infra/db/webhook_outbox.go) responde "o que está pendente
// e quem pode pegar". Este arquivo decide QUANDO cada coisa acontece, e é aqui
// que mora a parte perigosa.
//
// # O outbox é o ÚNICO agendador
//
// Manter os dois — o `time.AfterFunc` da F88 e a varredura — criaria corrida:
// os dois disparariam perto de `due_at` e o cliente receberia em duplicata.
// Quando há outbox, o timer não é usado; quando não há, o timer continua sendo
// o mecanismo, exatamente como hoje.
//
// # Degradação é alta, não silenciosa (D7)
//
// Se o outbox não estiver disponível, a entrega NÃO para: cai no caminho em
// memória de sempre, com aviso. Perde durabilidade, não função — que é o
// princípio do ADR inteiro. E as duas nunca cuidam da mesma entrega, então a
// corrida acima não reaparece pela porta dos fundos: a varredura só enxerga o
// que está na tabela.

const (
	// outboxSweepInterval é de quanto em quanto tempo a varredura procura
	// entregas vencidas.
	//
	// Um segundo é folgado para o que ela serve: a menor espera de retentativa
	// é a base do backoff, hoje dezenas de segundos. O intervalo não precisa
	// ser menor que o menor prazo — precisa ser MUITO menor, e é.
	outboxSweepInterval = time.Second

	// outboxOpTimeout limita cada operação de banco do outbox. Sem ele, um
	// banco pendurado congelaria a varredura em vez de deixá-la tentar de novo
	// no tique seguinte.
	outboxOpTimeout = 5 * time.Second
)

// outboxRuntime guarda o que a fiação do outbox precisa.
//
// O banco vem JUNTO do repositório, e não de um global separado, porque a
// retomada relê a chave HMAC do usuário — e dois globais que precisam estar
// ambos preenchidos é um estado a mais para ficar inconsistente. Um ponteiro
// só: ou o outbox subiu inteiro, ou não subiu.
type outboxRuntime struct {
	repo *db.WebhookOutboxRepository
	db   *sqlx.DB
}

// outboxAtual é o runtime em uso, ou nil quando o outbox não subiu.
//
// Ponteiro atômico e não variável comum: a varredura roda numa goroutine e o
// caminho de entrega roda em várias, então ler e escrever isto sem sincronizar
// seria corrida de dados.
var outboxAtual atomic.Pointer[outboxRuntime]

// outboxDisponivel devolve o runtime, ou nil.
func outboxDisponivel() *outboxRuntime { return outboxAtual.Load() }

// setupWebhookOutbox instala o repositório e informa o que ficou valendo.
//
// Nunca é fatal: sem outbox o processo entrega igual, só não sobrevive a
// restart. Derrubar o processo por causa disso trocaria "perde o pendente num
// deploy" por "não entrega nada", que é pior.
func setupWebhookOutbox(s *server) {
	if s.DB == nil {
		log.Warn().
			Str("fallback", "in-memory").
			Bool("survives_restart", false).
			Msg("sem banco para o outbox de webhook; o retry continua so em memoria")
		return
	}
	outboxAtual.Store(&outboxRuntime{repo: db.NewWebhookOutboxRepository(s.DB), db: s.DB})
	log.Info().
		Dur("sweep_interval", outboxSweepInterval).
		Msg("outbox de webhook ativo; entregas pendentes sobrevivem a restart")
}

// outboxEnqueue grava a intenção de entregar e devolve o id da linha.
//
// Devolve "" quando não há outbox ou quando a gravação falha — e "" é o sinal,
// para o resto do caminho, de que aquela entrega é do tipo antigo: sem
// durabilidade, reagendada em memória. Um id vazio nunca vira operação de
// banco.
func outboxEnqueue(userID, url string, payload map[string]string, scope db.HMACScope) string {
	rt := outboxDisponivel()
	if rt == nil {
		return ""
	}

	id := uuid.NewString()
	ctx, cancel := context.WithTimeout(context.Background(), outboxOpTimeout)
	defer cancel()

	if err := rt.repo.Enqueue(ctx, db.OutboxEntry{
		ID:      id,
		UserID:  userID,
		URL:     url,
		Payload: payload,
		Scope:   scope,
	}); err != nil {
		// A entrega SEGUE. Falhar em registrar durabilidade não pode custar a
		// tentativa que ia acontecer agora — seria trocar "pode perder num
		// restart" por "perdeu já".
		log.Error().Err(err).Str("url", url).Str("userid", userID).
			Msg("falha ao registrar a entrega no outbox; ela segue sem durabilidade")
		return ""
	}
	return id
}

// outboxSettle apaga a linha: entregue ou esgotada, não há o que retomar.
func outboxSettle(id string) {
	rt := outboxDisponivel()
	if rt == nil || id == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), outboxOpTimeout)
	defer cancel()

	if err := rt.repo.Delete(ctx, id); err != nil {
		// Não é fatal, mas é grave o suficiente para Error: a linha sobrevivente
		// será reentregue quando o prazo vencer, e o cliente recebe em
		// duplicata. Melhor duplicar que perder — mas o operador precisa saber.
		log.Error().Err(err).Str("outbox_id", id).
			Msg("falha ao apagar entrega concluida do outbox; ela sera reentregue quando o prazo vencer")
	}
}

// outboxDefer adia a linha para a próxima tentativa e devolve se conseguiu.
//
// False significa "não há retentativa durável para esta entrega", e quem chama
// segue para o caminho antigo ou para o terminal.
func outboxDefer(id string, proxima int, atraso time.Duration) bool {
	rt := outboxDisponivel()
	if rt == nil || id == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), outboxOpTimeout)
	defer cancel()

	if err := rt.repo.Reschedule(ctx, id, proxima, time.Now().UTC().Add(atraso)); err != nil {
		log.Error().Err(err).Str("outbox_id", id).Dur("delay", atraso).
			Msg("falha ao reagendar entrega no outbox; caindo para o reagendamento em memoria")
		return false
	}
	return true
}

// startOutboxSweeper sobe a varredura, quando há outbox.
func startOutboxSweeper(ctx context.Context) {
	if outboxDisponivel() == nil {
		return
	}
	safeGo("webhook-outbox-sweeper", func() { runOutboxSweeper(ctx) })
}

// runOutboxSweeper reivindica e reentrega, em laço, até o contexto acabar.
//
// A retomada de arranque NÃO precisa de código próprio: uma linha deixada por
// um processo morto está com `due_at` vencido, então a primeira varredura já a
// pega. Um caminho separado de "carregar pendentes na subida" seria um segundo
// mecanismo fazendo o mesmo trabalho, com o dobro das chances de divergir.
func runOutboxSweeper(ctx context.Context) {
	ticker := time.NewTicker(outboxSweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweepOutboxOnce(ctx)
		}
	}
}

// sweepOutboxOnce faz uma passada.
func sweepOutboxOnce(ctx context.Context) {
	rt := outboxDisponivel()
	if rt == nil {
		return
	}

	opCtx, cancel := context.WithTimeout(ctx, outboxOpTimeout)
	defer cancel()

	entries, err := rt.repo.ClaimDue(opCtx)
	if err != nil {
		log.Error().Err(err).Msg("falha ao reivindicar entregas vencidas do outbox")
		return
	}
	if len(entries) == 0 {
		return
	}

	log.Info().Int("entregas", len(entries)).Msg("retomando entregas pendentes do outbox")

	for _, entry := range entries {
		reentregar(entry)
	}
}

// reentregar devolve uma entrega reivindicada ao pool de despacho.
//
// Vai pelo pool, e não direto, pelo mesmo motivo da F88: a retentativa é uma
// entrega como qualquer outra e tem de respeitar o mesmo teto. Entregar aqui,
// em série dentro da varredura, faria uma fila de sessenta e quatro entregas
// lentas segurar a varredura inteira.
func reentregar(entry db.OutboxEntry) {
	chave, ok := resolverChaveHMAC(entry)
	if !ok {
		// Sem a chave a assinatura sairia errada e o cliente recusaria — o que
		// consumiria tentativas até esgotar, sem chance de sucesso. Melhor
		// desistir agora, com motivo.
		log.Error().Str("outbox_id", entry.ID).Str("userid", entry.UserID).
			Str("scope", string(entry.Scope)).
			Msg("nao foi possivel recuperar a chave HMAC da entrega pendente; descartando")
		outboxSettle(entry.ID)
		return
	}

	dispatchGo("outbox-retry", tamanhoDoPayload(entry.Payload), func() {
		tentarWebhook(entry.URL, entry.Payload, entry.UserID, chave, entry.Attempt, entry.ID)
	})
}

// resolverChaveHMAC relê a chave que assina a entrega.
//
// É por isso que a chave não é persistida no outbox: ela já existe, e duplicar
// segredo multiplica a superfície de vazamento. O escopo diz ONDE buscar.
//
// Ausência de chave é resultado VÁLIDO (nem todo usuário assina), e por isso o
// bool é separado do valor: `nil, true` significa "não assina", enquanto
// `nil, false` significa "deveria assinar e não consegui".
func resolverChaveHMAC(entry db.OutboxEntry) ([]byte, bool) {
	if entry.Scope == db.HMACScopeGlobal {
		return appCtx.GlobalHMACKeyEncrypted, true
	}

	rt := outboxDisponivel()
	if rt == nil || rt.db == nil {
		return nil, false
	}

	var chave []byte
	query := rt.db.Rebind(`SELECT hmac_key FROM users WHERE id = ?`)
	if err := rt.db.Get(&chave, query, entry.UserID); err != nil {
		log.Warn().Err(err).Str("userid", entry.UserID).
			Msg("falha ao reler a chave HMAC do usuario para uma entrega pendente")
		return nil, false
	}
	return chave, true
}
