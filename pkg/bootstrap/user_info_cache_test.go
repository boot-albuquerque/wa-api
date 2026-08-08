package bootstrap

import (
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
	_ "modernc.org/sqlite"

	"wa-api/pkg/infra/db"
)

// schemaDB devolve um sqlite de memória com o schema REAL do projeto.
//
// Contra o schema real, e não contra um CREATE TABLE escrito no teste: a F71
// era exatamente uma query pedindo uma coluna que o schema nunca teve, e um
// teste que cria a própria tabela combinaria com o erro em vez de expô-lo.
func schemaDB(t *testing.T) *sqlx.DB {
	t.Helper()
	sqlDB, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.InitializeSchema(sqlDB); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return sqlDB
}

// seedUser insere um usuário e devolve seu id.
func seedUser(t *testing.T, sqlDB *sqlx.DB, id, token, webhook string) string {
	t.Helper()
	_, err := sqlDB.Exec(
		`INSERT INTO users (id,name,token,webhook,events,jid,proxy_url,history)
		 VALUES (?,?,?,?,?,?,?,?)`,
		id, "Teste", token, webhook, "All", "5511999@s.whatsapp.net", "", 7)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return id
}

// withCache troca o UserInfoCache global por um vazio durante o teste. O
// cache é global de pacote; sem isolar, um teste contamina o outro.
func withCache(t *testing.T) {
	t.Helper()
	anterior := appCtx.UserInfoCache
	appCtx.UserInfoCache = cache.New(0, 0)
	t.Cleanup(func() { appCtx.UserInfoCache = anterior })
}

// TestEnsureUserInfoCached_CarregaDoBanco é o teste da F70: um usuário que
// existe no banco mas não está no cache passa a estar.
//
// É o cenário exato do onboarding normal — usuário criado por
// POST /admin/users depois da subida do servidor, portanto nunca visto por
// connectOnStartup.
func TestEnsureUserInfoCached_CarregaDoBanco(t *testing.T) {
	withCache(t)
	sqlDB := schemaDB(t)
	seedUser(t, sqlDB, "u-1", "tok-1", "https://exemplo.test/hook")

	if _, found := appCtx.UserInfoCache.Get("tok-1"); found {
		t.Fatal("o cache deveria começar vazio")
	}

	if err := ensureUserInfoCached(sqlDB, "u-1", "tok-1"); err != nil {
		t.Fatalf("ensureUserInfoCached: %v", err)
	}

	v, found := appCtx.UserInfoCache.Get("tok-1")
	if !found {
		t.Fatal("a entrada não foi criada")
	}
	info := v.(Values)
	// O webhook é o campo que a F70 tornava inalcançável: getUserWebhookUrl
	// lê só do cache e devolvia "" no miss, ignorando o que estava no banco.
	if got := info.Get("Webhook"); got != "https://exemplo.test/hook" {
		t.Errorf("Webhook = %q, queria o valor do banco", got)
	}
	if got := info.Get("Id"); got != "u-1" {
		t.Errorf("Id = %q", got)
	}
	if got := info.Get("Events"); got != "All" {
		t.Errorf("Events = %q", got)
	}
	if got := info.Get("History"); got != "7" {
		t.Errorf("History = %q, queria \"7\"", got)
	}
}

// TestEnsureUserInfoCached_NaoSobrescreve: com entrada presente, a função não
// toca no banco nem substitui o que já está lá. O handler de PairSuccess
// atualiza o Jid da entrada; recarregar do banco por cima desfaria isso.
func TestEnsureUserInfoCached_NaoSobrescreveEntradaExistente(t *testing.T) {
	withCache(t)
	sqlDB := schemaDB(t)
	seedUser(t, sqlDB, "u-2", "tok-2", "https://do-banco.test")

	appCtx.UserInfoCache.Set("tok-2", Values{M: map[string]string{
		"Id": "u-2", "Token": "tok-2", "Webhook": "https://ja-no-cache.test",
	}}, cache.NoExpiration)

	if err := ensureUserInfoCached(sqlDB, "u-2", "tok-2"); err != nil {
		t.Fatalf("ensureUserInfoCached: %v", err)
	}

	v, _ := appCtx.UserInfoCache.Get("tok-2")
	if got := v.(Values).Get("Webhook"); got != "https://ja-no-cache.test" {
		t.Errorf("Webhook = %q; a entrada existente foi sobrescrita", got)
	}
}

// TestEnsureUserInfoCached_UsuarioInexistente: erro, e nada é gravado. Uma
// entrada vazia no cache seria pior que a ausência — os leitores a tratariam
// como válida.
func TestEnsureUserInfoCached_UsuarioInexistenteNaoGravaNada(t *testing.T) {
	withCache(t)
	sqlDB := schemaDB(t)

	err := ensureUserInfoCached(sqlDB, "nao-existe", "tok-x")
	if err == nil {
		t.Fatal("queria erro para usuário inexistente")
	}
	if _, found := appCtx.UserInfoCache.Get("tok-x"); found {
		t.Error("gravou entrada para um usuário que não existe")
	}
}

// TestQueryDeHistoricoCasaComOSchema é o teste da F71.
//
// Executa a query EXATA de handlePairSuccess (eventhandler_session.go) contra
// o schema real. Antes da correção ela pedia `days_to_sync_history`, coluna
// que nunca existiu: falhava em todo banco, o sync automático de histórico
// após pareamento nunca rodava, e como a falha era só um Warn ninguém viu.
//
// Se alguém renomear a coluna de novo, é aqui que aparece — e não num log de
// produção seis meses depois.
func TestQueryDeHistoricoCasaComOSchema(t *testing.T) {
	sqlDB := schemaDB(t)
	seedUser(t, sqlDB, "u-3", "tok-3", "")

	var dias int
	// A query de PRODUÇÃO, não uma cópia: uma cópia deixaria o teste verde
	// enquanto o handler pede outra coluna.
	query := sqlDB.Rebind(historyDaysQuery)
	if err := sqlDB.Get(&dias, query, "u-3"); err != nil {
		t.Fatalf("a query de histórico não casa com o schema: %v", err)
	}
	if dias != 7 {
		t.Errorf("history = %d, queria 7", dias)
	}
}

// TestUserInfoColumns_UsadaPelosDoisCaminhos trava a constante compartilhada.
// connectOnStartup e ensureUserInfoCached preenchem a MESMA estrutura; se um
// deles voltar a ter a lista de colunas copiada inline, a entrada passa a
// nascer diferente dependendo do caminho — defeito que só aparece num dos
// dois fluxos, em produção.
func TestUserInfoColumns_ContemOsCamposDaEntrada(t *testing.T) {
	for _, col := range []string{"id", "name", "token", "jid", "webhook", "events", "proxy_url", "history", "hmac_key"} {
		if !strings.Contains(userInfoColumns, col) {
			t.Errorf("userInfoColumns não tem %q", col)
		}
	}
	// A query tem de rodar contra o schema real, não só parecer certa.
	sqlDB := schemaDB(t)
	seedUser(t, sqlDB, "u-4", "tok-4", "")
	if _, err := sqlDB.Queryx("SELECT " + userInfoColumns + " FROM users WHERE connected=1"); err != nil {
		t.Fatalf("userInfoColumns não casa com o schema: %v", err)
	}
}
