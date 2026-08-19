package db

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
)

// O adapter de users.history, users.proxy_url e users.webhook_use_proxy contra
// o schema de producao.
//
// As quatro instrucoes sao constantes do proprio arquivo de producao, entao o
// que este teste mede e' se elas CASAM com o schema real — e' o buraco da F71,
// em que uma query pedia uma coluna inexistente e nenhum teste a executava.

const sessionConfigRepoUserID = "user-session-config"

func newSessionConfigRepoDB(t *testing.T) *sqlx.DB {
	t.Helper()
	database := openTestDB(t)
	if err := InitializeSchema(database); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	if _, err := database.Exec(database.Rebind(
		`INSERT INTO users (id, name, token, token_hash) VALUES (?, ?, ?, ?)`),
		sessionConfigRepoUserID, "tenant", "tok-session-config", "hash-session-config"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return database
}

// historyColumn le users.history direto, sem passar pelo codigo sob teste.
func historyColumn(t *testing.T, database *sqlx.DB, userID string) int {
	t.Helper()
	var history int
	if err := database.QueryRowx(database.Rebind(
		`SELECT COALESCE(history, 0) FROM users WHERE id = ?`), userID).Scan(&history); err != nil {
		t.Fatalf("ler history: %v", err)
	}
	return history
}

// proxyColumns le as duas colunas de proxy direto.
func proxyColumns(t *testing.T, database *sqlx.DB, userID string) (string, bool) {
	t.Helper()
	var (
		proxyURL string
		useProxy bool
	)
	if err := database.QueryRowx(database.Rebind(
		`SELECT COALESCE(proxy_url, ''), COALESCE(webhook_use_proxy, true) FROM users WHERE id = ?`),
		userID).Scan(&proxyURL, &useProxy); err != nil {
		t.Fatalf("ler proxy: %v", err)
	}
	return proxyURL, useProxy
}

// TestSessionConfigRepository_SaveHistoryLimit — o valor chega a coluna, e
// gravar de novo SUBSTITUI. O zero e' um caso a parte de proposito: e' o
// estado "historico desligado", e um UPDATE que tratasse 0 como "nao mexer"
// tornaria o desligamento impossivel.
func TestSessionConfigRepository_SaveHistoryLimit(t *testing.T) {
	database := newSessionConfigRepoDB(t)
	repo := NewSessionConfigRepository(database)
	ctx := context.Background()

	if err := repo.SaveHistoryLimit(ctx, sessionConfigRepoUserID, 50); err != nil {
		t.Fatalf("SaveHistoryLimit: %v", err)
	}
	if got := historyColumn(t, database, sessionConfigRepoUserID); got != 50 {
		t.Fatalf("history = %d, quero 50", got)
	}

	if err := repo.SaveHistoryLimit(ctx, sessionConfigRepoUserID, 0); err != nil {
		t.Fatalf("SaveHistoryLimit(0): %v", err)
	}
	if got := historyColumn(t, database, sessionConfigRepoUserID); got != 0 {
		t.Fatalf("history = %d depois de desligar, quero 0", got)
	}
}

// TestSessionConfigRepository_SaveProxyConfig — as duas colunas na mesma
// operacao, e o ramo de desabilitacao gravando a string VAZIA (nao NULL, nao
// linha removida), como o UPDATE historico (`41bc8e2^:handlers.go:6121`).
func TestSessionConfigRepository_SaveProxyConfig(t *testing.T) {
	database := newSessionConfigRepoDB(t)
	repo := NewSessionConfigRepository(database)
	ctx := context.Background()

	const url = "socks5://user:pass@proxy.invalid:1080"
	if err := repo.SaveProxyConfig(ctx, sessionConfigRepoUserID, url, false); err != nil {
		t.Fatalf("SaveProxyConfig: %v", err)
	}
	gotURL, gotUseProxy := proxyColumns(t, database, sessionConfigRepoUserID)
	if gotURL != url {
		t.Errorf("proxy_url = %q, quero %q", gotURL, url)
	}
	if gotUseProxy {
		t.Error("webhook_use_proxy = true, quero false")
	}

	if err := repo.SaveProxyConfig(ctx, sessionConfigRepoUserID, "", true); err != nil {
		t.Fatalf("SaveProxyConfig(desabilitar): %v", err)
	}
	gotURL, gotUseProxy = proxyColumns(t, database, sessionConfigRepoUserID)
	if gotURL != "" {
		t.Errorf("proxy_url = %q depois de desabilitar, quero vazio", gotURL)
	}
	if !gotUseProxy {
		t.Error("webhook_use_proxy = false, quero true")
	}
}

// TestSessionConfigRepository_LoadWebhookUseProxy — os tres estados da coluna:
// nunca escrita (NULL, cai no COALESCE), escrita false, escrita true.
func TestSessionConfigRepository_LoadWebhookUseProxy(t *testing.T) {
	database := newSessionConfigRepoDB(t)
	repo := NewSessionConfigRepository(database)
	ctx := context.Background()

	got, err := repo.LoadWebhookUseProxy(ctx, sessionConfigRepoUserID)
	if err != nil {
		t.Fatalf("LoadWebhookUseProxy: %v", err)
	}
	if got != defaultWebhookUseProxy {
		t.Errorf("coluna nunca escrita = %v, quero %v (o default do COALESCE)", got, defaultWebhookUseProxy)
	}

	for _, want := range []bool{false, true} {
		if err := repo.SaveProxyConfig(ctx, sessionConfigRepoUserID, "", want); err != nil {
			t.Fatalf("SaveProxyConfig(%v): %v", want, err)
		}
		got, err := repo.LoadWebhookUseProxy(ctx, sessionConfigRepoUserID)
		if err != nil {
			t.Fatalf("LoadWebhookUseProxy: %v", err)
		}
		if got != want {
			t.Errorf("LoadWebhookUseProxy = %v, quero %v", got, want)
		}
	}
}

// TestSessionConfigRepository_LoadSemLinha_NaoEhErro — usuario inexistente
// reporta o default e NAO erro, pelo mesmo motivo pelo qual o handler
// historico ignorava o erro do Scan: esta leitura so' decide uma preferencia
// que o pedido nao mandou, e transformar a ausencia dela em falha recusaria a
// propria escrita do proxy.
func TestSessionConfigRepository_LoadSemLinha_NaoEhErro(t *testing.T) {
	repo := NewSessionConfigRepository(newSessionConfigRepoDB(t))

	got, err := repo.LoadWebhookUseProxy(context.Background(), "quem-nao-existe")
	if err != nil {
		t.Fatalf("linha ausente devolveu erro: %v", err)
	}
	if got != defaultWebhookUseProxy {
		t.Errorf("linha ausente = %v, quero %v", got, defaultWebhookUseProxy)
	}
}

// TestSessionConfigRepository_LoadHistoryLimit — a leitura de GET
// /webhook/history contra o schema REAL, e o round-trip com a escrita.
//
// A linha ausente le' 0, isto e', historico DESLIGADO, e nao erro: e' o mesmo
// fail-closed que ChatHistoryRepository.HistoryLimit da' ao gate
// (chat_history_repository.go:153), e responder outra coisa faria a rota
// anunciar um limite que ninguem grava.
func TestSessionConfigRepository_LoadHistoryLimit(t *testing.T) {
	database := newSessionConfigRepoDB(t)
	repo := NewSessionConfigRepository(database)
	ctx := context.Background()

	for _, quero := range []int{50, 0, 7} {
		if err := repo.SaveHistoryLimit(ctx, sessionConfigRepoUserID, quero); err != nil {
			t.Fatalf("SaveHistoryLimit(%d): %v", quero, err)
		}
		got, err := repo.LoadHistoryLimit(ctx, sessionConfigRepoUserID)
		if err != nil {
			t.Fatalf("LoadHistoryLimit: %v", err)
		}
		if got != quero {
			t.Errorf("LoadHistoryLimit = %d, quero %d", got, quero)
		}
	}

	got, err := repo.LoadHistoryLimit(ctx, "quem-nao-existe")
	if err != nil {
		t.Fatalf("linha ausente devolveu erro: %v", err)
	}
	if got != 0 {
		t.Errorf("linha ausente = %d, quero 0 (historico desligado)", got)
	}
}

// TestSessionConfigRepository_EscopoPorUsuario — escrever para um usuario nao
// pode tocar a linha do outro. E' a assercao que pega um WHERE esquecido, que
// e' o defeito que passa em todos os testes de um usuario so'.
func TestSessionConfigRepository_EscopoPorUsuario(t *testing.T) {
	database := newSessionConfigRepoDB(t)
	repo := NewSessionConfigRepository(database)
	ctx := context.Background()

	const outro = "outro-usuario"
	if _, err := database.Exec(database.Rebind(
		`INSERT INTO users (id, name, token, token_hash, history, proxy_url) VALUES (?, ?, ?, ?, ?, ?)`),
		outro, "tenant 2", "tok-2", "hash-2", 7, "http://outro.invalid:3128"); err != nil {
		t.Fatalf("seed outro usuario: %v", err)
	}

	if err := repo.SaveHistoryLimit(ctx, sessionConfigRepoUserID, 99); err != nil {
		t.Fatalf("SaveHistoryLimit: %v", err)
	}
	if err := repo.SaveProxyConfig(ctx, sessionConfigRepoUserID, "", true); err != nil {
		t.Fatalf("SaveProxyConfig: %v", err)
	}

	if got := historyColumn(t, database, outro); got != 7 {
		t.Errorf("o history do outro usuario virou %d, quero 7", got)
	}
	if gotURL, _ := proxyColumns(t, database, outro); gotURL != "http://outro.invalid:3128" {
		t.Errorf("o proxy_url do outro usuario virou %q", gotURL)
	}
}
