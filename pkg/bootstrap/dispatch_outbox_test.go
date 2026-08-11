package bootstrap

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	dbpkg "wa-api/pkg/infra/db"
)

// ADR-0005 D3, fiação. O repositório já é testado em pkg/infra/db; o que estes
// testes travam é a DECISÃO: quem executa a próxima tentativa, e o que acontece
// com a linha em cada desfecho.

// instalarOutboxDeTeste liga um outbox real sobre SQLite temporário e o desliga
// ao fim. `outboxAtual` é estado de pacote: sem restaurar, um teste contamina
// os seguintes — mesmo cuidado que prepararRetry já toma com appCtx.
func instalarOutboxDeTeste(t *testing.T) *dbpkg.WebhookOutboxRepository {
	t.Helper()

	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "outbox.db"))
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("fechar banco: %v", err)
		}
	})
	if err := dbpkg.InitializeSchema(db); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}

	anterior := outboxAtual.Load()
	t.Cleanup(func() { outboxAtual.Store(anterior) })

	repo := dbpkg.NewWebhookOutboxRepository(db)
	outboxAtual.Store(&outboxRuntime{repo: repo, db: db})
	return repo
}

func semOutbox(t *testing.T) {
	t.Helper()
	anterior := outboxAtual.Load()
	t.Cleanup(func() { outboxAtual.Store(anterior) })
	outboxAtual.Store(nil)
}

// TestOutboxWiring_ReagendarUsaODuravelEnaoODaMemoria é o teste central da
// fiação.
//
// Com linha no outbox, a retentativa tem de ser DURÁVEL e o timer em memória
// NÃO pode ser armado. Se os dois fossem armados, ambos disparariam perto de
// `due_at` e o cliente receberia o mesmo webhook duas vezes — que é exatamente
// o defeito que a escolha centralizada em `reagendar` existe para impedir.
//
// A evidência de que o timer não foi armado é `retryBytesPendentes`: ele só sobe
// quando `agendarProximaTentativa` reserva orçamento.
func TestOutboxWiring_ReagendarUsaODuravelEnaoODaMemoria(t *testing.T) {
	prepararRetry(t, true, 5, 30)
	repo := instalarOutboxDeTeste(t)
	ctx := context.Background()

	if err := repo.Enqueue(ctx, dbpkg.OutboxEntry{
		ID: "e1", UserID: "u1", URL: "https://example.invalid/hook",
		Payload: payloadDeTeste(128),
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if !reagendar("https://example.invalid/hook", payloadDeTeste(128), "u1", nil, 1, "e1") {
		t.Fatal("reagendar devolveu false com outbox disponivel e orcamento restante")
	}

	if pendentes, _ := MetricasRetry(); pendentes != 0 {
		t.Errorf("o timer em memoria tambem foi armado (%d bytes pendentes); a entrega sairia em duplicata", pendentes)
	}

	// E a linha continua lá, adiada — se o processo cair agora, a varredura a
	// retoma quando o prazo vencer.
	if n, err := repo.PendingCount(ctx); err != nil || n != 1 {
		t.Errorf("pendentes = %d (err=%v), want 1", n, err)
	}
	if claimed, err := repo.ClaimDue(ctx); err != nil {
		t.Fatalf("ClaimDue: %v", err)
	} else if len(claimed) != 0 {
		t.Error("a entrega reagendada ficou imediatamente elegivel; o backoff nao espacaria nada")
	}
}

// TestOutboxWiring_SemOutboxCaiParaAMemoria é o controle na direção oposta e o
// que garante a promessa do D7: sem outbox a entrega NÃO para, degrada.
// Comportamento idêntico ao de hoje.
func TestOutboxWiring_SemOutboxCaiParaAMemoria(t *testing.T) {
	prepararRetry(t, true, 5, 30)
	semOutbox(t)

	if !reagendar("https://example.invalid/hook", payloadDeTeste(128), "u1", nil, 1, "") {
		t.Fatal("reagendar devolveu false sem outbox; a entrega seria descartada em vez de degradar")
	}

	if pendentes, _ := MetricasRetry(); pendentes == 0 {
		t.Error("o caminho em memoria nao foi usado; sem outbox nao sobra quem execute a retentativa")
	}
}

// TestOutboxWiring_OrcamentoEsgotadoNaoGravaPrazoNovo: esgotou é esgotado. Se a
// checagem viesse depois, uma entrega sem tentativas restantes ganharia prazo
// novo e a varredura a pegaria só para descobrir que não pode fazer nada — um
// laço de trabalho inútil por entrega morta.
func TestOutboxWiring_OrcamentoEsgotadoNaoGravaPrazoNovo(t *testing.T) {
	prepararRetry(t, true, 2, 30)
	repo := instalarOutboxDeTeste(t)
	ctx := context.Background()

	if err := repo.Enqueue(ctx, dbpkg.OutboxEntry{
		ID: "e1", UserID: "u1", URL: "https://example.invalid/hook",
		Payload: payloadDeTeste(64), Attempt: 2,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if reagendar("https://example.invalid/hook", payloadDeTeste(64), "u1", nil, 2, "e1") {
		t.Fatal("reagendou uma entrega sem tentativas restantes")
	}

	// A linha ainda existe: quem a apaga é o caminho terminal, depois de
	// entregá-la à fila de erro. Apagar aqui perderia o evento sem registro.
	if n, err := repo.PendingCount(ctx); err != nil || n != 1 {
		t.Errorf("pendentes = %d (err=%v), want 1", n, err)
	}
}

// TestOutboxWiring_RetryDesligadoNaoGravaPrazoNovo cobre o outro jeito de o
// orçamento acabar: a configuração desligada.
func TestOutboxWiring_RetryDesligadoNaoGravaPrazoNovo(t *testing.T) {
	prepararRetry(t, false, 5, 30)
	instalarOutboxDeTeste(t)

	if reagendar("https://example.invalid/hook", payloadDeTeste(64), "u1", nil, 1, "e1") {
		t.Fatal("reagendou com retry desligado")
	}
}

// TestOutboxWiring_SettleApagaEIdVazioNaoQuebra: `id` vazio é o sinal de
// "entrega sem durabilidade" e atravessa todo o caminho de entrega. Ele NUNCA
// pode virar operação de banco — nem erro.
func TestOutboxWiring_SettleApagaEIdVazioNaoQuebra(t *testing.T) {
	repo := instalarOutboxDeTeste(t)
	ctx := context.Background()

	if err := repo.Enqueue(ctx, dbpkg.OutboxEntry{
		ID: "e1", UserID: "u1", URL: "https://example.invalid/hook",
		Payload: payloadDeTeste(32),
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	outboxSettle("") // não pode tocar no banco
	if n, _ := repo.PendingCount(ctx); n != 1 {
		t.Fatalf("id vazio mexeu no outbox: pendentes = %d, want 1", n)
	}

	outboxSettle("e1")
	if n, _ := repo.PendingCount(ctx); n != 0 {
		t.Errorf("pendentes = %d, want 0: a entrega concluida seria retomada para sempre", n)
	}
}

// TestOutboxWiring_EnqueueSemOutboxDevolveIdVazio fixa o contrato do sinal: sem
// outbox não há id, e é isso que faz o resto do caminho escolher o timer.
func TestOutboxWiring_EnqueueSemOutboxDevolveIdVazio(t *testing.T) {
	semOutbox(t)

	if id := outboxEnqueue("u1", "https://example.invalid/hook", payloadDeTeste(16), dbpkg.HMACScopeUser); id != "" {
		t.Errorf("id = %q, want vazio", id)
	}
}

// TestOutboxWiring_EnqueueGeraIdUnico: dois eventos não podem colidir no mesmo
// id, senão o segundo INSERT falha por chave primária e aquela entrega perde a
// durabilidade em silêncio.
func TestOutboxWiring_EnqueueGeraIdUnico(t *testing.T) {
	repo := instalarOutboxDeTeste(t)

	primeiro := outboxEnqueue("u1", "https://example.invalid/hook", payloadDeTeste(16), dbpkg.HMACScopeUser)
	segundo := outboxEnqueue("u1", "https://example.invalid/hook", payloadDeTeste(16), dbpkg.HMACScopeUser)

	if primeiro == "" || segundo == "" {
		t.Fatalf("enqueue falhou: %q / %q", primeiro, segundo)
	}
	if primeiro == segundo {
		t.Fatalf("dois eventos receberam o mesmo id (%q)", primeiro)
	}
	if n, _ := repo.PendingCount(context.Background()); n != 2 {
		t.Errorf("pendentes = %d, want 2", n)
	}
}

// inserirUsuario cria a linha em `users` de onde a varredura relê a chave HMAC.
func inserirUsuario(t *testing.T, db *sqlx.DB, id string, chave []byte) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO users
		(id, name, token, webhook, jid, qrcode, events, proxy_url, history, s3_enabled, media_delivery, hmac_key)
		VALUES (?, ?, ?, '', '', '', 'All', '', 0, 0, 'base64', ?)`,
		id, "user-"+id, "tok-"+id, chave)
	if err != nil {
		t.Fatalf("inserir usuario %s: %v", id, err)
	}
}

// esperarOutboxVazio espera com PRAZO. Um laço sem deadline penduraria o teste
// em vez de falhá-lo, e teste pendurado não reporta nada (ARMADILHAS 16).
func esperarOutboxVazio(t *testing.T, repo *dbpkg.WebhookOutboxRepository, motivo string) {
	t.Helper()

	// F110. Drenar o pool ANTES de contar é o que torna esta espera
	// determinística. `sweepOutboxOnce` apenas DESPACHA a entrega
	// (`dispatchGo("outbox-retry", ...)`, dispatch_outbox.go:235); quem liquida
	// a linha é o worker. Contar antes de o worker terminar mede um estado
	// intermediário, e o prazo de 3s abaixo virava uma corrida contra a carga
	// da máquina — 2 falhas em ~6 execuções do `make check`, nenhuma
	// reproduzível isolada.
	//
	// O prazo continua como rede de segurança, não como mecanismo: se a
	// liquidação não acontecer nem depois de o pool drenar, é defeito de
	// verdade e o teste deve falhar.
	esperarDespachoDrenar(t)

	prazo := time.After(3 * time.Second)
	for {
		n, err := repo.PendingCount(context.Background())
		if err != nil {
			t.Fatalf("PendingCount: %v", err)
		}
		if n == 0 {
			return
		}
		select {
		case <-prazo:
			t.Fatal(motivo)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// TestOutboxWiring_VarreduraDescartaEntregaSemChave cobre o caminho em que a
// chave HMAC não pode ser relida — usuário apagado entre o enfileiramento e a
// retomada, por exemplo.
//
// Descartar é deliberado: sem a chave a assinatura sairia errada e o cliente
// recusaria, consumindo o orçamento de tentativas até esgotar, sem NENHUMA
// chance de sucesso. Melhor desistir agora, com motivo no log.
func TestOutboxWiring_VarreduraDescartaEntregaSemChave(t *testing.T) {
	prepararRetry(t, true, 5, 30)
	repo := instalarOutboxDeTeste(t)
	ctx := context.Background()

	// Sem inserirUsuario: `users` não tem a linha.
	if err := repo.Enqueue(ctx, dbpkg.OutboxEntry{
		ID: "e1", UserID: "fantasma", URL: "https://example.invalid/hook",
		Payload: payloadDeTeste(32),
		DueAt:   time.Now().UTC().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	sweepOutboxOnce(ctx)

	esperarOutboxVazio(t, repo,
		"entrega sem chave recuperavel ficou no outbox; voltaria a cada prazo, para sempre, sem poder ser assinada")
}

// TestOutboxWiring_VarreduraRetomaOVencido prova a retomada de verdade: a linha
// deixada por um processo morto está com o prazo vencido, a varredura a pega, a
// chave é relida e a entrega chega ao caminho de despacho.
//
// Este teste nasceu de um controle negativo que FALHOU EM FALHAR. A versão
// anterior não inseria o usuário, então a entrada era descartada por falta de
// chave antes de chegar ao despacho — e o teste passava mesmo com a liquidação
// removida do caminho de entrega. Ele descrevia um caminho que não percorria
// (ARMADILHAS 19).
func TestOutboxWiring_VarreduraRetomaOVencido(t *testing.T) {
	prepararRetry(t, true, 5, 30)
	repo := instalarOutboxDeTeste(t)
	ctx := context.Background()

	rt := outboxDisponivel()
	inserirUsuario(t, rt.db, "u1", []byte("chave-cifrada"))

	if err := repo.Enqueue(ctx, dbpkg.OutboxEntry{
		ID: "e1", UserID: "u1", URL: "https://example.invalid/hook",
		Payload: payloadDeTeste(32),
		DueAt:   time.Now().UTC().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	sweepOutboxOnce(ctx)

	// Sem cliente HTTP provisionado para `u1`, tentarWebhook desiste e liquida
	// a linha. O cliente nasce com a sessão: insistir numa entrega que não tem
	// como sair só gastaria tentativas do orçamento.
	esperarOutboxVazio(t, repo,
		"a varredura reivindicou mas nunca liquidou a entrega sem cliente HTTP; ela voltaria a cada prazo, para sempre")
}

// TestOutboxWiring_ChaveDoGlobalNaoVemDoBanco: a entrega do webhook global
// assina com a chave do PROCESSO, não com a do usuário. Confundir as duas faria
// o cliente global receber assinatura que não valida — e o erro seria atribuído
// à rede, não à chave.
func TestOutboxWiring_ChaveDoGlobalNaoVemDoBanco(t *testing.T) {
	instalarOutboxDeTeste(t)

	anterior := appCtx.GlobalHMACKeyEncrypted
	t.Cleanup(func() { appCtx.GlobalHMACKeyEncrypted = anterior })
	appCtx.GlobalHMACKeyEncrypted = []byte("chave-global")

	// Usuário inexistente de propósito: no escopo global o banco nem é
	// consultado, então a ausência não pode atrapalhar.
	chave, ok := resolverChaveHMAC(dbpkg.OutboxEntry{
		ID: "e1", UserID: "nao-existe", Scope: dbpkg.HMACScopeGlobal,
	})
	if !ok {
		t.Fatal("escopo global falhou ao resolver a chave; a entrega global seria descartada")
	}
	if string(chave) != "chave-global" {
		t.Errorf("chave = %q, want %q", chave, "chave-global")
	}
}

// TestOutboxWiring_SetupSemBancoNaoLiga: sem banco não há durabilidade, e o
// processo NÃO pode morrer por isso. Trocar "perde o pendente num deploy" por
// "não entrega nada" seria piorar o que se queria consertar.
func TestOutboxWiring_SetupSemBancoNaoLiga(t *testing.T) {
	anterior := outboxAtual.Load()
	t.Cleanup(func() { outboxAtual.Store(anterior) })
	outboxAtual.Store(nil)

	setupWebhookOutbox(&server{})

	if outboxDisponivel() != nil {
		t.Error("outbox ligou sem banco; as operacoes seguintes iriam desreferenciar nil")
	}
}

// TestOutboxWiring_SetupComBancoLiga é o par do teste acima: o caminho feliz
// tem de instalar o runtime, senão nada do resto acontece e o sistema fica
// silenciosamente sem durabilidade.
func TestOutboxWiring_SetupComBancoLiga(t *testing.T) {
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "setup.db"))
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("fechar banco: %v", err)
		}
	})

	anterior := outboxAtual.Load()
	t.Cleanup(func() { outboxAtual.Store(anterior) })
	outboxAtual.Store(nil)

	setupWebhookOutbox(&server{DB: db})

	rt := outboxDisponivel()
	if rt == nil {
		t.Fatal("outbox nao ligou com banco disponivel")
	}
	if rt.repo == nil || rt.db == nil {
		t.Error("runtime instalado pela metade; a retomada nao conseguiria reler a chave HMAC")
	}
}

// TestOutboxWiring_VarreduraParaComOContexto: a varredura roda até o fim do
// processo. Se ela não observasse o cancelamento, o desligamento gracioso
// esperaria por uma goroutine que nunca termina.
func TestOutboxWiring_VarreduraParaComOContexto(t *testing.T) {
	instalarOutboxDeTeste(t)

	ctx, cancel := context.WithCancel(context.Background())
	parou := make(chan struct{})
	go func() {
		runOutboxSweeper(ctx)
		close(parou)
	}()

	cancel()

	select {
	case <-parou:
	case <-time.After(3 * time.Second):
		// Prazo, nunca WaitGroup.Wait(): teste pendurado nao reporta nada
		// (ARMADILHAS 16).
		t.Fatal("a varredura ignorou o cancelamento; o desligamento gracioso ficaria esperando por ela")
	}
}

// TestOutboxWiring_SweeperNaoSobeSemOutbox: sem outbox não há o que varrer, e
// uma goroutine batendo num runtime nulo a cada segundo seria trabalho puro
// para produzir nada.
func TestOutboxWiring_SweeperNaoSobeSemOutbox(t *testing.T) {
	semOutbox(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startOutboxSweeper(ctx) // não deve subir goroutine nenhuma

	// Sem outbox, uma passada tem de ser inofensiva — é o que aconteceria se a
	// varredura tivesse subido por engano.
	sweepOutboxOnce(ctx)
}

// TestOutboxWiring_BancoFechadoDegradaSemDerrubar: com o banco indisponível,
// TODAS as operações do outbox precisam falhar para o lado seguro — enfileirar
// devolve id vazio (a entrega segue sem durabilidade), reagendar devolve false
// (cai para a memória) e a varredura reporta e volta no tique seguinte.
//
// Um panic aqui derrubaria o processo por causa de uma indisponibilidade
// temporária de banco, que é justamente o cenário em que ele precisa continuar
// entregando.
func TestOutboxWiring_BancoFechadoDegradaSemDerrubar(t *testing.T) {
	prepararRetry(t, true, 5, 30)

	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "fechado.db"))
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	if err := dbpkg.InitializeSchema(db); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}

	anterior := outboxAtual.Load()
	t.Cleanup(func() { outboxAtual.Store(anterior) })
	outboxAtual.Store(&outboxRuntime{repo: dbpkg.NewWebhookOutboxRepository(db), db: db})

	if err := db.Close(); err != nil {
		t.Fatalf("fechar banco: %v", err)
	}

	if id := outboxEnqueue("u1", "https://example.invalid/hook", payloadDeTeste(16), dbpkg.HMACScopeUser); id != "" {
		t.Errorf("enqueue devolveu id %q com o banco fechado; a entrega seria tratada como duravel sem estar", id)
	}
	if outboxDefer("e1", 1, time.Second) {
		t.Error("reagendou no outbox com o banco fechado; a retentativa se perderia entre os dois mecanismos")
	}
	outboxSettle("e1")                    // so pode logar
	sweepOutboxOnce(context.Background()) // idem

	// E o caminho de entrega continua funcionando, degradado para a memória.
	if !reagendar("https://example.invalid/hook", payloadDeTeste(16), "u1", nil, 1, "e1") {
		t.Error("com o outbox fora, a entrega deixou de ser reagendada em vez de degradar")
	}
}
