package auth_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wa-api/pkg/domain"
	"wa-api/pkg/infra/auth"
	dbpkg "wa-api/pkg/infra/db"

	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
	_ "modernc.org/sqlite"
)

// newAuthTestDB abre um sqlite real e aplica o schema de produção, pela mesma
// via de pkg/infra/db/user_repository_test.go. LookupUser monta SQL na mão
// (token = $1 OR token_hash = $2); um fake de driver validaria a string, não a
// consulta — e é justamente a consulta que decide quem entra.
func newAuthTestDB(t *testing.T) *sql.DB {
	t.Helper()
	x, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := x.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	if err := dbpkg.InitializeSchema(x); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	return x.DB
}

// insertUser grava um usuário com token_hash preenchido (o formato pós-Fase 5b)
// e, opcionalmente, o token em claro na coluna legada.
func insertUser(t *testing.T, db *sql.DB, id, name, token string, alsoPlaintext bool) {
	t.Helper()
	plain := ""
	if alsoPlaintext {
		plain = token
	}
	_, err := db.Exec(
		`INSERT INTO users (id, name, token, token_hash, webhook, jid, events, proxy_url, qrcode)
		 VALUES ($1, $2, $3, $4, '', '', 'All', '', '')`,
		id, name, plain, domain.HashToken(token),
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func TestLookupUser_FindsUserByTokenHash(t *testing.T) {
	db := newAuthTestDB(t)
	insertUser(t, db, "u1", "alice", "alice-token", false)

	rec, err := auth.LookupUser(db, "alice-token")
	if err != nil {
		t.Fatalf("LookupUser: %v", err)
	}
	if rec == nil {
		t.Fatal("token valido (so token_hash gravado) nao autenticou — o OR token_hash da query nao esta funcionando")
	}
	if rec.ID != "u1" {
		t.Fatalf("id: got %q, want %q", rec.ID, "u1")
	}
	if rec.Name != "alice" {
		t.Fatalf("name: got %q, want %q", rec.Name, "alice")
	}
}

func TestLookupUser_FindsUserByLegacyPlaintextToken(t *testing.T) {
	// A coluna legada `token` continua aceita nesta release. Se o OR sumir da
	// query, usuários não migrados perdem acesso — este teste é o que avisa.
	db := newAuthTestDB(t)
	_, err := db.Exec(
		`INSERT INTO users (id, name, token, webhook, jid, events, proxy_url, qrcode)
		 VALUES ('u2', 'bob', 'bob-token', '', '', 'All', '', '')`)
	if err != nil {
		t.Fatalf("insert legacy user: %v", err)
	}

	rec, err := auth.LookupUser(db, "bob-token")
	if err != nil {
		t.Fatalf("LookupUser: %v", err)
	}
	if rec == nil {
		t.Fatal("token legado em claro deixou de autenticar")
	}
	if rec.ID != "u2" {
		t.Fatalf("id: got %q, want %q", rec.ID, "u2")
	}
}

// TestLookupUser_RejectsUnknownTokens é o teste de bypass. Cada candidato é uma
// mutação plausível da cláusula WHERE: remover o filtro, usar LIKE, comparar
// prefixo, comparar o hash contra o token em claro.
func TestLookupUser_RejectsUnknownTokens(t *testing.T) {
	db := newAuthTestDB(t)
	insertUser(t, db, "u1", "alice", "alice-token", true)

	rejected := map[string]string{
		"token inexistente":       "nao-existe",
		"token vazio":             "",
		"prefixo do token valido": "alice",
		"sufixo do token valido":  "token",
		"wildcard SQL":            "%",
		"wildcard SQL de 1 char":  "_",
		"token com sufixo extra":  "alice-tokenX",
		"hash do token valido":    domain.HashToken("alice-token"),
	}

	for name, candidate := range rejected {
		t.Run(name, func(t *testing.T) {
			rec, err := auth.LookupUser(db, candidate)
			if err != nil {
				t.Fatalf("LookupUser: %v", err)
			}
			if rec != nil {
				t.Fatalf("BYPASS: candidato %q autenticou como usuario %q", candidate, rec.ID)
			}
		})
	}
}

// TestLookupUser_TokenIsNotSQLInjectable fecha a outra metade do bypass: a
// query usa placeholders, então nenhum payload de injeção deve autenticar.
func TestLookupUser_TokenIsNotSQLInjectable(t *testing.T) {
	db := newAuthTestDB(t)
	insertUser(t, db, "u1", "alice", "alice-token", true)

	for _, payload := range []string{
		"' OR '1'='1",
		"' OR 1=1 --",
		"x' UNION SELECT id,name,webhook,jid,events,proxy_url,qrcode,history,1,'true','base64' FROM users --",
	} {
		rec, err := auth.LookupUser(db, payload)
		if err != nil {
			// Um erro é aceitável (a query não casou); autenticar não é.
			continue
		}
		if rec != nil {
			t.Fatalf("BYPASS por injecao: payload %q autenticou como %q", payload, rec.ID)
		}
	}
}

func TestLookupUser_ReportsErrorOnBrokenSchema(t *testing.T) {
	// Banco aberto sem schema: a query falha. O contrato é (nil, err), nunca
	// (nil, nil) — porque (nil, nil) significa "não autenticado" e um erro de
	// infraestrutura sendo lido como "não autenticado" mascara indisponibilidade.
	x, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() {
		if err := x.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	}()

	rec, err := auth.LookupUser(x.DB, "qualquer")
	if err == nil {
		t.Fatal("query contra banco sem tabela users nao reportou erro")
	}
	if rec != nil {
		t.Fatal("erro de infraestrutura devolveu um UserRecord")
	}
}

// TestLookupUser_ReportsErrorOnUnscannableRow cobre a falha de Scan, que é
// diferente da falha de Query: a linha existe e volta, mas um valor não cabe no
// destino. Aqui `history` (sql.NullInt64) recebe texto — sqlite é tipado
// dinamicamente e aceita gravar. O contrato continua sendo (nil, err): uma
// linha ilegível não pode virar "usuário não encontrado" nem autenticar
// parcialmente.
func TestLookupUser_ReportsErrorOnUnscannableRow(t *testing.T) {
	db := newAuthTestDB(t)
	insertUser(t, db, "u1", "alice", "alice-token", false)

	if _, err := db.Exec(`UPDATE users SET history = 'nao-e-numero' WHERE id = 'u1'`); err != nil {
		t.Fatalf("update history: %v", err)
	}

	rec, err := auth.LookupUser(db, "alice-token")
	if err == nil {
		t.Fatal("linha com valor inescaneavel nao reportou erro")
	}
	if rec != nil {
		t.Fatal("erro de scan devolveu um UserRecord")
	}
}

// rowsCloseFailDriver é um driver mínimo que entrega uma linha inescaneável e
// cujo Rows.Close falha. Serve para exercitar o defer de LookupUser: quando o
// Scan falha, o loop sai antes do fim do resultset, o database/sql ainda não
// fechou o Rows, e o Close do defer é quem propaga o erro do driver. Esse erro
// tem que ser apenas logado — o erro de Scan é o que vai para o chamador, e
// trocar um pelo outro apagaria a causa real.
type rowsCloseFailDriver struct{}

func (rowsCloseFailDriver) Open(string) (driver.Conn, error) { return rowsCloseFailConn{}, nil }

type rowsCloseFailConn struct{}

func (rowsCloseFailConn) Prepare(string) (driver.Stmt, error) { return rowsCloseFailStmt{}, nil }
func (rowsCloseFailConn) Close() error                        { return nil }
func (rowsCloseFailConn) Begin() (driver.Tx, error)           { return nil, errors.New("sem transacao") }

type rowsCloseFailStmt struct{}

func (rowsCloseFailStmt) Close() error  { return nil }
func (rowsCloseFailStmt) NumInput() int { return -1 }
func (rowsCloseFailStmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, errors.New("sem exec")
}
func (rowsCloseFailStmt) Query([]driver.Value) (driver.Rows, error) {
	return &rowsCloseFailRows{}, nil
}

type rowsCloseFailRows struct{ served bool }

func (*rowsCloseFailRows) Columns() []string {
	return []string{
		"id", "name", "webhook", "jid", "events", "proxy_url", "qrcode",
		"history", "has_hmac", "s3_enabled", "media_delivery",
	}
}

func (*rowsCloseFailRows) Close() error { return errors.New("falha ao fechar rows") }

func (r *rowsCloseFailRows) Next(dest []driver.Value) error {
	if r.served {
		return io.EOF
	}
	r.served = true
	for i := range dest {
		dest[i] = ""
	}
	// Índice 7 é `history`, escaneado em sql.NullInt64: texto não numérico
	// derruba o Scan e faz LookupUser sair do loop antes do fim do resultset.
	dest[7] = "nao-e-numero"
	dest[8] = false
	return nil
}

func init() { sql.Register("wa-auth-rows-close-fail", rowsCloseFailDriver{}) }

func TestLookupUser_ScanErrorSurvivesRowsCloseFailure(t *testing.T) {
	db, err := sql.Open("wa-auth-rows-close-fail", "")
	if err != nil {
		t.Fatalf("open driver falso: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})

	rec, err := auth.LookupUser(db, "qualquer")
	if err == nil {
		t.Fatal("linha inescaneavel nao reportou erro")
	}
	if rec != nil {
		t.Fatal("erro devolveu um UserRecord")
	}
	if !strings.Contains(err.Error(), "db scan") {
		t.Fatalf("o erro de Close do driver mascarou a causa real: %v", err)
	}
}

func TestLookupUser_HasHmacReflectsStoredKey(t *testing.T) {
	db := newAuthTestDB(t)
	insertUser(t, db, "u1", "alice", "alice-token", false)

	rec, err := auth.LookupUser(db, "alice-token")
	if err != nil {
		t.Fatalf("LookupUser: %v", err)
	}
	if rec.HasHmac {
		t.Fatal("HasHmac true sem hmac_key gravado")
	}

	if _, err := db.Exec(`UPDATE users SET hmac_key = $1 WHERE id = 'u1'`, []byte("k")); err != nil {
		t.Fatalf("update hmac_key: %v", err)
	}
	rec, err = auth.LookupUser(db, "alice-token")
	if err != nil {
		t.Fatalf("LookupUser: %v", err)
	}
	if !rec.HasHmac {
		t.Fatal("HasHmac false com hmac_key nao vazio gravado")
	}
}

func TestLookupUser_HistoryDefaultsToZeroWhenNull(t *testing.T) {
	db := newAuthTestDB(t)
	insertUser(t, db, "u1", "alice", "alice-token", false)

	rec, err := auth.LookupUser(db, "alice-token")
	if err != nil {
		t.Fatalf("LookupUser: %v", err)
	}
	if rec.History != "0" {
		t.Fatalf("History com coluna NULL: got %q, want %q", rec.History, "0")
	}

	if _, err := db.Exec(`UPDATE users SET history = 42 WHERE id = 'u1'`); err != nil {
		t.Fatalf("update history: %v", err)
	}
	rec, err = auth.LookupUser(db, "alice-token")
	if err != nil {
		t.Fatalf("LookupUser: %v", err)
	}
	if rec.History != "42" {
		t.Fatalf("History: got %q, want %q", rec.History, "42")
	}
}

func TestExtractToken_PrefersHeaderOverQueryString(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?token=da-query", nil)
	r.Header.Set("token", "do-header")

	if got := auth.ExtractToken(r); got != "do-header" {
		t.Fatalf("got %q, want %q — a query string sobrescreveu o header", got, "do-header")
	}
}

func TestExtractToken_FallsBackToQueryString(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?token=da-query", nil)

	if got := auth.ExtractToken(r); got != "da-query" {
		t.Fatalf("got %q, want %q", got, "da-query")
	}
}

func TestExtractToken_ReturnsEmptyWhenAbsent(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)

	if got := auth.ExtractToken(r); got != "" {
		t.Fatalf("got %q, want vazio", got)
	}
}

// TestExtractToken_EmptyHeaderDoesNotShadowQuery: header presente porém vazio
// não pode impedir o fallback, senão `token: ""` vira um jeito de forçar
// autenticação anônima.
func TestExtractToken_EmptyHeaderDoesNotShadowQuery(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?token=da-query", nil)
	r.Header.Set("token", "")

	if got := auth.ExtractToken(r); got != "da-query" {
		t.Fatalf("got %q, want %q", got, "da-query")
	}
}

func TestGetOrSetCache_HitDoesNotCallFactory(t *testing.T) {
	c := cache.New(5*time.Minute, 10*time.Minute)
	c.Set("t", "cacheado", cache.DefaultExpiration)

	val, found := auth.GetOrSetCache(c, "t", func() interface{} {
		t.Fatal("factory chamada em cache hit")
		return nil
	})
	if !found {
		t.Fatal("found=false em cache hit")
	}
	if val != "cacheado" {
		t.Fatalf("got %v, want %q", val, "cacheado")
	}
}

func TestGetOrSetCache_MissCallsFactory(t *testing.T) {
	c := cache.New(5*time.Minute, 10*time.Minute)
	calls := 0

	val, found := auth.GetOrSetCache(c, "t", func() interface{} {
		calls++
		return "fresco"
	})
	if found {
		t.Fatal("found=true em cache miss")
	}
	if calls != 1 {
		t.Fatalf("factory chamada %d vezes, queria 1", calls)
	}
	if val != "fresco" {
		t.Fatalf("got %v, want %q", val, "fresco")
	}
}

// TestGetOrSetCache_ExpiredEntryIsAMiss trava o motivo pelo qual o cache tem
// TTL (sec/F11): revogar um usuário precisa ter efeito antes do fim do processo.
func TestGetOrSetCache_ExpiredEntryIsAMiss(t *testing.T) {
	c := cache.New(time.Millisecond, time.Millisecond)
	c.Set("t", "antigo", time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	val, found := auth.GetOrSetCache(c, "t", func() interface{} { return "novo" })
	if found {
		t.Fatal("entrada expirada foi servida como cache hit — revogacao nao teria efeito")
	}
	if val != "novo" {
		t.Fatalf("got %v, want %q", val, "novo")
	}
}

// TestWithUserInfo_UsesTypedKey é a regressão do bug corrigido em b513bbe /
// 8130898: com chave string crua, dois pacotes colidem no mesmo contexto. O
// valor precisa sair pela chave TIPADA e NÃO sair pela string equivalente.
func TestWithUserInfo_UsesTypedKey(t *testing.T) {
	ctx := auth.WithUserInfo(context.Background(), "payload")

	if got := ctx.Value(auth.UserInfoKey); got != "payload" {
		t.Fatalf("valor pela chave tipada: got %v, want %q", got, "payload")
	}
	//nolint:staticcheck // a string crua é exatamente o que NÃO pode funcionar
	if got := ctx.Value("userinfo"); got != nil {
		t.Fatalf("a string crua \"userinfo\" recuperou o valor (%v) — a chave voltou a ser untyped", got)
	}
}
