package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/proto/waAdv"
	"wa-api/internal/wa-noise/protocol/types"
)

// testAccount monta o ADVSignedDeviceIdentity minimo que PutDevice exige — os
// quatro campos sao desreferenciados sem checagem de nil.
func testAccount() *waAdv.ADVSignedDeviceIdentity {
	return &waAdv.ADVSignedDeviceIdentity{
		Details:             []byte("details"),
		AccountSignature:    make([]byte, signedPreKeySignatureLength),
		AccountSignatureKey: make([]byte, curve25519KeyLength),
		DeviceSignature:     make([]byte, signedPreKeySignatureLength),
	}
}

// Este arquivo cobre a lacuna da F23: os tres pontos que ramificam por dialeto
// (GetManySessions, DeleteAppStateMutationMACs e GetManyLIDsForPNs) usam
// `= ANY($2)` com PostgresArrayWrapper no Postgres e placeholders `$N`
// expandidos no resto. A suite roda sobre SQLite, entao so' o ramo generico era
// executado — e producao usa Postgres. O caminho testado e o caminho de
// producao eram ramos diferentes do mesmo `if`.
//
// A entrada sugeria subir um Postgres efemero no `make check` via
// testcontainers. Nao e' o caminho aqui: o repositorio inteiro e'
// deliberadamente Docker-free nos testes, e trocar isso por causa de tres
// ramos e' caro demais.
//
// A forma escolhida: os testes rodam contra um Postgres de verdade quando
// WA_TEST_POSTGRES_DSN estiver no ambiente, e dao `t.Skip` quando nao estiver.
// Isso deixa `make check` local exatamente como esta' (Docker-free, sem
// dependencia de servico) e ao mesmo tempo torna o ramo executavel em CI, ou na
// maquina de quem estiver mexendo nessas queries, sem escrever nada novo:
//
//	WA_TEST_POSTGRES_DSN='postgres://user:pass@localhost:5432/wa_test?sslmode=disable' \
//	  go test ./internal/wa-noise/persistence/store/sqlstore/ -run Postgres -v
//
// O banco apontado pelo DSN e' MODIFICADO (as tabelas sao dropadas e recriadas
// a cada run), entao aponte para um banco descartavel.
const postgresDSNEnv = "WA_TEST_POSTGRES_DSN"

// newPostgresContainer devolve um Container ligado ao Postgres do ambiente, com
// o schema recriado do zero. Pula o teste se o DSN nao estiver configurado.
func newPostgresContainer(t *testing.T) *Container {
	t.Helper()
	dsn := os.Getenv(postgresDSNEnv)
	if dsn == "" {
		t.Skipf("defina %s para exercitar o ramo Postgres (F23 em HOUSEKEEP.md)", postgresDSNEnv)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("abrir postgres: %v", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("conectar em %s: %v", postgresDSNEnv, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	dropAllTables(t, db)

	container := NewWithDB(db, "postgres", nil)
	if err := container.Upgrade(context.Background()); err != nil {
		t.Fatalf("migrar postgres: %v", err)
	}
	return container
}

// dropAllTables limpa o schema antes de cada run. Sem isso, o segundo run
// encontraria a tabela de versao ja' na ultima versao e pularia as migracoes,
// escondendo qualquer quebra nelas.
func dropAllTables(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(
		"SELECT tablename FROM pg_tables WHERE schemaname = current_schema()")
	if err != nil {
		t.Fatalf("listar tabelas: %v", err)
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterar tabelas: %v", err)
	}
	_ = rows.Close()

	for _, n := range names {
		if _, err := db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %q CASCADE", n)); err != nil {
			t.Fatalf("dropar %s: %v", n, err)
		}
	}
}

// mac32 monta um index MAC do tamanho que o CHECK do schema exige.
func mac32(fill byte) []byte {
	m := make([]byte, 32)
	for i := range m {
		m[i] = fill
	}
	return m
}

// newPostgresDevice espelha newTestDevice, mas contra Postgres.
func newPostgresDevice(t *testing.T) (*Container, *SQLStore) {
	t.Helper()
	container := newPostgresContainer(t)
	device := container.NewDevice()
	jid := types.JID{User: "5511999999999", Device: 0, Server: types.DefaultUserServer}
	device.ID = &jid
	device.LID = types.JID{User: "111111111111111", Device: 0, Server: types.HiddenUserServer}
	device.Account = testAccount()
	if err := container.PutDevice(context.Background(), device); err != nil {
		t.Fatalf("PutDevice: %v", err)
	}
	return container, NewSQLStore(container, jid)
}

// TestPostgresGetManySessions exercita o ramo `= ANY($2)` de GetManySessions —
// o que roda em producao e que a suite SQLite nunca alcanca.
func TestPostgresGetManySessions(t *testing.T) {
	_, s := newPostgresDevice(t)
	ctx := context.Background()

	quero := map[string][]byte{
		"alice:0": []byte("sessao-alice"),
		"bob:0":   []byte("sessao-bob"),
	}
	for addr, sess := range quero {
		if err := s.PutSession(ctx, addr, sess); err != nil {
			t.Fatalf("PutSession(%s): %v", addr, err)
		}
	}
	// Uma sessao que NAO deve voltar, para provar que o ANY filtra.
	if err := s.PutSession(ctx, "carol:0", []byte("sessao-carol")); err != nil {
		t.Fatalf("PutSession(carol): %v", err)
	}

	got, err := s.GetManySessions(ctx, []string{"alice:0", "bob:0", "ausente:0"})
	if err != nil {
		t.Fatalf("GetManySessions: %v", err)
	}
	// O mapa vem PRE-POPULADO com nil para cada endereco pedido — e' contrato
	// da funcao, nao efeito do dialeto. Entao "ausente:0" aparece, com valor
	// nil, e o que se confere e' o VALOR, nao a presenca da chave.
	if len(got) != 3 {
		t.Fatalf("= %d entradas, esperado 3 (uma por endereco pedido): %v", len(got), got)
	}
	for addr, want := range quero {
		if string(got[addr]) != string(want) {
			t.Errorf("%s = %q, esperado %q", addr, got[addr], want)
		}
	}
	if got["ausente:0"] != nil {
		t.Errorf("ausente:0 = %q, esperado nil", got["ausente:0"])
	}
	if _, ok := got["carol:0"]; ok {
		t.Error("carol:0 nao foi pedida e nao pode aparecer — o ANY nao filtrou")
	}
}

// TestPostgresDeleteAppStateMutationMACs exercita o mesmo ramo de dialeto na
// remocao em lote de MACs.
func TestPostgresDeleteAppStateMutationMACs(t *testing.T) {
	_, s := newPostgresDevice(t)
	ctx := context.Background()
	const name = testCollection
	// A tabela de MACs tem FK para wanoise_app_state_version(jid, name): sem a
	// linha de versao, o insert e' rejeitado.
	seedAppStateVersion(t, s, name)

	// index_mac tem CHECK (length(index_mac) = 32) no schema — MACs mais curtos
	// sao rejeitados pelo banco.
	macs := [][]byte{mac32('A'), mac32('B'), mac32('C')}
	for i, indexMAC := range macs {
		if err := s.PutAppStateMutationMACs(ctx, name, uint64(i+1), []store.AppStateMutationMAC{{
			IndexMAC: indexMAC,
			ValueMAC: mac32(byte('v' + i)),
		}}); err != nil {
			t.Fatalf("PutAppStateMutationMACs: %v", err)
		}
	}

	if err := s.DeleteAppStateMutationMACs(ctx, name, macs[:2]); err != nil {
		t.Fatalf("DeleteAppStateMutationMACs: %v", err)
	}

	for _, apagado := range macs[:2] {
		got, err := s.GetAppStateMutationMAC(ctx, name, apagado)
		if err != nil {
			t.Fatalf("GetAppStateMutationMAC: %v", err)
		}
		if got != nil {
			t.Errorf("%x deveria ter sido apagado, veio %q", apagado, got)
		}
	}
	sobrevivente, err := s.GetAppStateMutationMAC(ctx, name, macs[2])
	if err != nil {
		t.Fatalf("GetAppStateMutationMAC: %v", err)
	}
	if sobrevivente == nil {
		t.Error("indexC nao estava na lista e nao deveria ter sido apagado")
	}
}

// TestPostgresGetManyLIDsForPNs exercita o ramo de dialeto do mapeamento
// PN -> LID em lote.
func TestPostgresGetManyLIDsForPNs(t *testing.T) {
	container, _ := newPostgresDevice(t)
	ctx := context.Background()

	pnA := types.NewJID("5511111111111", types.DefaultUserServer)
	pnB := types.NewJID("5522222222222", types.DefaultUserServer)
	pnAusente := types.NewJID("5533333333333", types.DefaultUserServer)
	lidA := types.NewJID("111111111111111", types.HiddenUserServer)
	lidB := types.NewJID("222222222222222", types.HiddenUserServer)

	if err := container.LIDMap.PutManyLIDMappings(ctx, []store.LIDMapping{
		{LID: lidA, PN: pnA},
		{LID: lidB, PN: pnB},
	}); err != nil {
		t.Fatalf("PutManyLIDMappings: %v", err)
	}

	got, err := container.LIDMap.GetManyLIDsForPNs(ctx, []types.JID{pnA, pnB, pnAusente})
	if err != nil {
		t.Fatalf("GetManyLIDsForPNs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("= %d mapeamentos, esperado 2: %v", len(got), got)
	}
	if got[pnA] != lidA {
		t.Errorf("pnA -> %v, esperado %v", got[pnA], lidA)
	}
	if got[pnB] != lidB {
		t.Errorf("pnB -> %v, esperado %v", got[pnB], lidB)
	}
	if _, ok := got[pnAusente]; ok {
		t.Error("um PN sem mapeamento nao pode aparecer no resultado")
	}
}

// TestPostgresMigracoesAplicamDoZero prova que o schema inteiro sobe no
// dialeto de producao — inclusive a renomeacao de prefixo de renameLegacyTables,
// que so' era exercitada contra SQLite.
func TestPostgresMigracoesAplicamDoZero(t *testing.T) {
	container := newPostgresContainer(t)
	ctx := context.Background()

	// Se as migracoes rodaram, o device round-trippa.
	device := container.NewDevice()
	jid := types.JID{User: "5599999999999", Device: 0, Server: types.DefaultUserServer}
	device.ID = &jid
	device.Account = testAccount()
	if err := container.PutDevice(ctx, device); err != nil {
		t.Fatalf("PutDevice: %v", err)
	}
	got, err := container.GetDevice(ctx, jid)
	if err != nil {
		t.Fatalf("GetDevice: %v", err)
	}
	if got == nil || got.ID.User != jid.User {
		t.Fatalf("device = %+v, esperado o que foi gravado", got)
	}
	if got.SignedPreKey == nil || got.NoiseKey == nil {
		t.Error("as chaves nao sobreviveram ao round trip")
	}
}
