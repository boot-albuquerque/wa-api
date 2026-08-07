package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// legacySchemaSQL e' o recorte do schema antigo suficiente para provar a
// renomeacao: a tabela de versao (que o dbutil le antes de qualquer migracao),
// uma tabela com dados, uma tabela que a referencia por chave estrangeira, e um
// indice. Nao e' o schema inteiro de proposito — o que esta' sob teste e'
// renameLegacyTables, nao o conteudo de upgrades/.
const legacySchemaSQL = `
CREATE TABLE whatsmeow_version (version INTEGER, compat INTEGER);

CREATE TABLE whatsmeow_device (
	jid TEXT PRIMARY KEY,
	push_name TEXT NOT NULL DEFAULT ''
);

CREATE TABLE whatsmeow_privacy_tokens (
	our_jid TEXT,
	their_jid TEXT,
	token bytea NOT NULL,
	timestamp BIGINT NOT NULL,
	PRIMARY KEY (our_jid, their_jid),
	FOREIGN KEY (our_jid) REFERENCES whatsmeow_device(jid) ON DELETE CASCADE ON UPDATE CASCADE
);

CREATE INDEX idx_whatsmeow_privacy_tokens_our_jid_timestamp
ON whatsmeow_privacy_tokens (our_jid, timestamp);
`

func openLegacyDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=foreign_keys(1)", path))
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(legacySchemaSQL); err != nil {
		t.Fatalf("criar schema legado: %v", err)
	}
	return db
}

func tableNames(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type IN ('table','index')")
	if err != nil {
		t.Fatalf("listar objetos: %v", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[n] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterar: %v", err)
	}
	return out
}

// TestRenameLegacyTablesMigraBancoAntigoPreservandoDados e' o teste que
// justifica a existencia de renameLegacyTables. Um banco criado sob os nomes
// antigos precisa sair da migracao com os nomes novos, os MESMOS dados, e as
// chaves estrangeiras ainda apontando para o lugar certo.
func TestRenameLegacyTablesMigraBancoAntigoPreservandoDados(t *testing.T) {
	db := openLegacyDB(t)
	ctx := context.Background()

	if _, err := db.Exec(`INSERT INTO whatsmeow_device (jid, push_name) VALUES ('5511@s.whatsapp.net', 'Fulano')`); err != nil {
		t.Fatalf("semear device: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO whatsmeow_privacy_tokens (our_jid, their_jid, token, timestamp) VALUES ('5511@s.whatsapp.net', '5522@s.whatsapp.net', X'DEADBEEF', 42)`); err != nil {
		t.Fatalf("semear token: %v", err)
	}

	container := NewWithDB(db, "sqlite", nil)
	if err := renameLegacyTables(ctx, container.db); err != nil {
		t.Fatalf("renameLegacyTables: %v", err)
	}

	objs := tableNames(t, db)
	for _, want := range []string{
		"wanoise_version", "wanoise_device", "wanoise_privacy_tokens",
		"idx_wanoise_privacy_tokens_our_jid_timestamp",
	} {
		if !objs[want] {
			t.Errorf("esperava %s apos a migracao, nao encontrei", want)
		}
	}
	for _, unwanted := range []string{
		"whatsmeow_version", "whatsmeow_device", "whatsmeow_privacy_tokens",
		"idx_whatsmeow_privacy_tokens_our_jid_timestamp",
	} {
		if objs[unwanted] {
			t.Errorf("%s ainda existe apos a migracao", unwanted)
		}
	}

	var pushName string
	if err := db.QueryRow(`SELECT push_name FROM wanoise_device WHERE jid='5511@s.whatsapp.net'`).Scan(&pushName); err != nil {
		t.Fatalf("ler device migrado: %v", err)
	}
	if pushName != "Fulano" {
		t.Errorf("push_name = %q, esperava %q", pushName, "Fulano")
	}

	var ts int64
	if err := db.QueryRow(`SELECT timestamp FROM wanoise_privacy_tokens WHERE their_jid='5522@s.whatsapp.net'`).Scan(&ts); err != nil {
		t.Fatalf("ler token migrado: %v", err)
	}
	if ts != 42 {
		t.Errorf("timestamp = %d, esperava 42", ts)
	}

	// A FK precisa ter seguido o rename: o DELETE em cascata so' dispara se a
	// referencia aponta para wanoise_device, nao para um nome que sumiu.
	if _, err := db.Exec(`DELETE FROM wanoise_device WHERE jid='5511@s.whatsapp.net'`); err != nil {
		t.Fatalf("deletar device: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM wanoise_privacy_tokens`).Scan(&n); err != nil {
		t.Fatalf("contar tokens: %v", err)
	}
	if n != 0 {
		t.Errorf("esperava cascata apagar o token, sobraram %d", n)
	}
}

// TestRenameLegacyTablesEIdempotente trava a propriedade que permite chamar a
// rotina em todo start do processo: rodar de novo nao pode falhar nem mexer em
// nada.
func TestRenameLegacyTablesEIdempotente(t *testing.T) {
	db := openLegacyDB(t)
	ctx := context.Background()
	container := NewWithDB(db, "sqlite", nil)

	if err := renameLegacyTables(ctx, container.db); err != nil {
		t.Fatalf("primeira passada: %v", err)
	}
	antes := tableNames(t, db)
	if err := renameLegacyTables(ctx, container.db); err != nil {
		t.Fatalf("segunda passada: %v", err)
	}
	depois := tableNames(t, db)

	if len(antes) != len(depois) {
		t.Fatalf("a segunda passada mudou o schema: %v -> %v", antes, depois)
	}
	for name := range antes {
		if !depois[name] {
			t.Errorf("%s sumiu na segunda passada", name)
		}
	}
}

// TestRenameLegacyTablesEmBancoNovoNaoFazNada garante que instalacao nova nao
// paga nada e, principalmente, nao quebra: nenhuma tabela antiga existe, entao
// a rotina precisa sair limpa sem tentar renomear nada.
func TestRenameLegacyTablesEmBancoNovoNaoFazNada(t *testing.T) {
	path := filepath.Join(t.TempDir(), "novo.db")
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=foreign_keys(1)", path))
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	container := NewWithDB(db, "sqlite", nil)
	if err := renameLegacyTables(context.Background(), container.db); err != nil {
		t.Fatalf("renameLegacyTables em banco vazio: %v", err)
	}
	if objs := tableNames(t, db); len(objs) != 0 {
		t.Errorf("banco vazio ganhou objetos: %v", objs)
	}
}

// TestRenameLegacyTablesNaoMexeQuandoAsDuasExistem documenta a escolha feita
// no caso ambiguo: com a tabela antiga E a nova presentes (so' acontece por
// intervencao manual), a rotina nao adivinha qual tem os dados bons — ela sai
// sem erro e sem tocar em nenhuma das duas.
func TestRenameLegacyTablesNaoMexeQuandoAsDuasExistem(t *testing.T) {
	db := openLegacyDB(t)
	if _, err := db.Exec(`CREATE TABLE wanoise_device (jid TEXT PRIMARY KEY, push_name TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("criar tabela nova conflitante: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO whatsmeow_device (jid) VALUES ('antiga@s.whatsapp.net')`); err != nil {
		t.Fatalf("semear antiga: %v", err)
	}

	container := NewWithDB(db, "sqlite", nil)
	if err := renameLegacyTables(context.Background(), container.db); err != nil {
		t.Fatalf("esperava saida limpa, veio erro: %v", err)
	}

	objs := tableNames(t, db)
	if !objs["whatsmeow_device"] || !objs["wanoise_device"] {
		t.Errorf("as duas tabelas deveriam continuar existindo, vi %v", objs)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM whatsmeow_device`).Scan(&n); err != nil {
		t.Fatalf("contar na antiga: %v", err)
	}
	if n != 1 {
		t.Errorf("a linha da tabela antiga deveria estar intacta, vi %d", n)
	}
}
