package db_test

// Testes da coluna `users.engine` (migração 19) e do backfill determinístico
// que a preenche no arranque.
//
// O banco é um SQLite real, criado por dbpkg.InitializeSchema — o mesmo caminho
// que a produção percorre. Um dublê de banco aqui não mediria nada: o que está
// sob teste é DDL e SQL, e um mapa em memória concordaria com qualquer regra.

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/domain"
	dbpkg "wa-api/pkg/infra/db"

	"github.com/jmoiron/sqlx"
)

// engineOf lê a coluna crua, sem passar pelo repositório: o teste do backfill
// tem de ver o que está GRAVADO, não o que a camada de leitura decide mostrar.
func engineOf(t *testing.T, db *sqlx.DB, id string) string {
	t.Helper()
	var got string
	if err := db.Get(&got, `SELECT engine FROM users WHERE id = $1`, id); err != nil {
		t.Fatalf("read engine of %q: %v", id, err)
	}
	return got
}

// insertLegacyUser cria uma linha no ESTADO ANTIGO: com a coluna existindo mas
// ainda a dizer legacy_unknown, que é exatamente o que o ALTER TABLE deixa para
// trás numa base que já tinha sessões.
func insertLegacyUser(t *testing.T, db *sqlx.DB, id string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO users (id, name, token, token_hash, webhook, expiration, events, jid, qrcode, engine)
		 VALUES ($1, $2, '', $3, '', 0, '', '', '', $4)`,
		id, "user-"+id, "hash-"+id, string(domain.EngineLegacyUnknown))
	if err != nil {
		t.Fatalf("insert legacy user %q: %v", id, err)
	}
}

// TestMigrationAddsEngineColumnWithLegacyDefault trava o DEFAULT do ALTER.
//
// O default é legacy_unknown e NÃO wa_noise de propósito: a migração não pode
// afirmar um transporte que não mediu. Se alguém trocar o default por wa_noise
// "para simplificar", o backfill passa a ser um no-op e a distinção entre
// "sabemos" e "assumimos" desaparece sem que nada falhe.
func TestMigrationAddsEngineColumnWithLegacyDefault(t *testing.T) {
	db := newUserTestDB(t)

	// INSERT sem citar a coluna: quem escolhe o valor é o DEFAULT da coluna.
	if _, err := db.Exec(
		`INSERT INTO users (id, name, token, token_hash, webhook, expiration, events, jid, qrcode)
		 VALUES ('legacy-1', 'legacy', '', 'h1', '', 0, '', '', '')`); err != nil {
		t.Fatalf("insert without engine: %v", err)
	}

	if got := engineOf(t, db, "legacy-1"); got != string(domain.EngineLegacyUnknown) {
		t.Fatalf("default engine = %q, want %q", got, domain.EngineLegacyUnknown)
	}
}

// TestBackfillReproducesHeadlessSessionList é o teste do defeito: a
// distribuição tem de bater EXATAMENTE com WA_API_ENGINE_HEADLESS_SESSIONS,
// que é a regra que já roda em produção hoje.
func TestBackfillReproducesHeadlessSessionList(t *testing.T) {
	db := newUserTestDB(t)
	ctx := context.Background()

	for _, id := range []string{"alpha", "bravo", "charlie", "delta"} {
		insertLegacyUser(t, db, id)
	}

	headless := []string{"bravo", "delta"}
	report, err := dbpkg.BackfillUserEngines(ctx, db, headless, domain.EngineWaNoise)
	if err != nil {
		t.Fatalf("BackfillUserEngines: %v", err)
	}

	// Enumeração nome por nome, não contagem agregada: um total certo com os
	// membros trocados passaria numa asserção de contagem.
	want := map[string]string{
		"alpha":   string(domain.EngineWaNoise),
		"bravo":   string(domain.EngineWaHeadless),
		"charlie": string(domain.EngineWaNoise),
		"delta":   string(domain.EngineWaHeadless),
	}
	for id, expected := range want {
		if got := engineOf(t, db, id); got != expected {
			t.Errorf("engine of %q = %q, want %q", id, got, expected)
		}
	}

	counts := []struct {
		name string
		got  int
		want int
	}{
		{"TotalUsers", report.TotalUsers, 4},
		{"PendingBefore", report.PendingBefore, 4},
		{"ToWaHeadless", report.ToWaHeadless, 2},
		{"ToWaNoise", report.ToWaNoise, 2},
		{"RemainingLegacyUnknown", report.RemainingLegacyUnknown, 0},
		{"len(ListedButAbsent)", len(report.ListedButAbsent), 0},
	}
	for _, c := range counts {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

// TestBackfillIsIdempotent: correr de novo não muda nada, porque o backfill
// corre em TODO arranque e não uma vez só.
//
// A segunda passagem também não pode reescrever um engine já decidido — é isso
// que impede um reinício de desfazer uma escolha feita pela API mais tarde.
func TestBackfillIsIdempotent(t *testing.T) {
	db := newUserTestDB(t)
	ctx := context.Background()

	for _, id := range []string{"alpha", "bravo"} {
		insertLegacyUser(t, db, id)
	}
	headless := []string{"bravo"}

	if _, err := dbpkg.BackfillUserEngines(ctx, db, headless, domain.EngineWaNoise); err != nil {
		t.Fatalf("first backfill: %v", err)
	}

	// Uma escolha posterior, do tipo que a API vai poder fazer: alpha passa a
	// headless SEM estar na lista de ambiente.
	if _, err := db.Exec(`UPDATE users SET engine = $1 WHERE id = 'alpha'`,
		string(domain.EngineWaHeadless)); err != nil {
		t.Fatalf("post-backfill engine change: %v", err)
	}

	second, err := dbpkg.BackfillUserEngines(ctx, db, headless, domain.EngineWaNoise)
	if err != nil {
		t.Fatalf("second backfill: %v", err)
	}

	if second.PendingBefore != 0 {
		t.Errorf("second run PendingBefore = %d, want 0", second.PendingBefore)
	}
	if second.ToWaNoise != 0 || second.ToWaHeadless != 0 {
		t.Errorf("second run changed rows: to_wa_noise=%d to_wa_headless=%d, want 0/0",
			second.ToWaNoise, second.ToWaHeadless)
	}
	if got := engineOf(t, db, "alpha"); got != string(domain.EngineWaHeadless) {
		t.Errorf("second run overwrote a deliberate choice: alpha = %q, want %q",
			got, domain.EngineWaHeadless)
	}
	if got := engineOf(t, db, "bravo"); got != string(domain.EngineWaHeadless) {
		t.Errorf("bravo = %q, want %q", got, domain.EngineWaHeadless)
	}
}

// TestBackfillOrderPutsHeadlessFirst trava a ORDEM das duas instruções.
//
// A segunda reivindica "tudo que ainda está desconhecido". Invertê-las daria
// TODAS as sessões ao engine padrão e deixaria a primeira sem nada a casar —
// e essa inversão passaria em qualquer teste que só contasse o total. Aqui a
// lista de headless tem de sobreviver ao UPDATE do padrão.
func TestBackfillOrderPutsHeadlessFirst(t *testing.T) {
	db := newUserTestDB(t)
	ctx := context.Background()

	for _, id := range []string{"a", "b", "c"} {
		insertLegacyUser(t, db, id)
	}
	if _, err := dbpkg.BackfillUserEngines(ctx, db, []string{"a", "b", "c"}, domain.EngineWaNoise); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	for _, id := range []string{"a", "b", "c"} {
		if got := engineOf(t, db, id); got != string(domain.EngineWaHeadless) {
			t.Fatalf("engine of %q = %q, want %q (default UPDATE ran before the headless one)",
				id, got, domain.EngineWaHeadless)
		}
	}
}

// TestBackfillReportsListedButAbsent: id no ambiente sem linha no banco é
// informação para o operador, não falha de arranque.
func TestBackfillReportsListedButAbsent(t *testing.T) {
	db := newUserTestDB(t)
	insertLegacyUser(t, db, "present")

	report, err := dbpkg.BackfillUserEngines(context.Background(), db,
		[]string{"present", "ghost"}, domain.EngineWaNoise)
	if err != nil {
		t.Fatalf("BackfillUserEngines: %v", err)
	}
	if len(report.ListedButAbsent) != 1 || report.ListedButAbsent[0] != "ghost" {
		t.Fatalf("ListedButAbsent = %v, want [ghost]", report.ListedButAbsent)
	}
}

// TestBackfillRejectsInvalidDefaultEngine: um padrão de legacy_unknown deixaria
// a tabela no estado exato que este código existe para remover.
func TestBackfillRejectsInvalidDefaultEngine(t *testing.T) {
	db := newUserTestDB(t)
	insertLegacyUser(t, db, "alpha")

	for _, bad := range []domain.Engine{domain.EngineLegacyUnknown, "", "wanoise"} {
		_, err := dbpkg.BackfillUserEngines(context.Background(), db, nil, bad)
		if !errors.Is(err, domain.ErrInvalidEngine) {
			t.Errorf("default %q: err = %v, want ErrInvalidEngine", bad, err)
		}
		if got := engineOf(t, db, "alpha"); got != string(domain.EngineLegacyUnknown) {
			t.Errorf("default %q: row was modified (%q) despite the rejection", bad, got)
		}
	}
}

// TestBackfillOnEmptyDatabase: zero sessões é resultado válido e reportado.
func TestBackfillOnEmptyDatabase(t *testing.T) {
	db := newUserTestDB(t)
	report, err := dbpkg.BackfillUserEngines(context.Background(), db, nil, domain.EngineWaNoise)
	if err != nil {
		t.Fatalf("BackfillUserEngines: %v", err)
	}
	if report.TotalUsers != 0 || report.ToWaNoise != 0 || report.ToWaHeadless != 0 {
		t.Fatalf("report on empty db = %+v, want all zeros", report)
	}
}

// --- Repositório: gravar e ler o engine -------------------------------------

// TestCreateUserDefaultsEngineToWaNoise trava o valor ZERO documentado de
// domain.UserRecord.Engine.
//
// Nenhuma rota HTTP sabe pedir engine ainda, então todo criador de hoje deixa o
// campo vazio. Gravar legacy_unknown numa linha que está a nascer AGORA seria
// mentira — a linha é nova e a configuração corrente diz wa_noise.
func TestCreateUserDefaultsEngineToWaNoise(t *testing.T) {
	db := newUserTestDB(t)
	repo := dbpkg.NewUserRepository(db)

	created, err := repo.CreateUser(context.Background(), domain.UserRecord{
		ID: "new-1", Name: "new", Token: "tok-new-1",
	})
	if err != nil || !created {
		t.Fatalf("CreateUser: created=%v err=%v", created, err)
	}
	if got := engineOf(t, db, "new-1"); got != string(domain.EngineWaNoise) {
		t.Fatalf("engine of a freshly created user = %q, want %q", got, domain.EngineWaNoise)
	}
}

// TestCreateUserPersistsExplicitEngine e a leitura de volta pela MESMA porta que
// a aplicação usa (ListUsers), não por um SELECT do teste: é a leitura real que
// tem de carregar o campo.
func TestCreateUserPersistsExplicitEngine(t *testing.T) {
	db := newUserTestDB(t)
	repo := dbpkg.NewUserRepository(db)
	ctx := context.Background()

	for _, e := range []domain.Engine{domain.EngineWaNoise, domain.EngineWaHeadless} {
		id := "user-" + e.String()
		created, err := repo.CreateUser(ctx, domain.UserRecord{
			ID: id, Name: id, Token: "tok-" + id, Engine: e,
		})
		if err != nil || !created {
			t.Fatalf("CreateUser(%q): created=%v err=%v", e, created, err)
		}

		entries, err := repo.ListUsers(ctx, id)
		if err != nil {
			t.Fatalf("ListUsers(%q): %v", id, err)
		}
		if len(entries) != 1 {
			t.Fatalf("ListUsers(%q) returned %d entries, want 1", id, len(entries))
		}
		if entries[0].Engine != e {
			t.Errorf("read back engine = %q, want %q", entries[0].Engine, e)
		}
	}
}

// TestCreateUserRejectsInvalidEngine: valor errado é ERRO, não correção
// silenciosa. E a linha não pode ficar meio criada.
func TestCreateUserRejectsInvalidEngine(t *testing.T) {
	db := newUserTestDB(t)
	repo := dbpkg.NewUserRepository(db)
	ctx := context.Background()

	for _, bad := range []domain.Engine{domain.EngineLegacyUnknown, "wanoise", "headless", "foobar"} {
		created, err := repo.CreateUser(ctx, domain.UserRecord{
			ID: "bad-" + bad.String(), Name: "bad", Token: "tok-bad-" + bad.String(), Engine: bad,
		})
		if !errors.Is(err, domain.ErrInvalidEngine) {
			t.Errorf("CreateUser(engine=%q) err = %v, want ErrInvalidEngine", bad, err)
		}
		if created {
			t.Errorf("CreateUser(engine=%q) reported created", bad)
		}
		var count int
		if err := db.Get(&count, `SELECT COUNT(*) FROM users WHERE id = $1`, "bad-"+bad.String()); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("CreateUser(engine=%q) wrote a row anyway", bad)
		}
	}
}

// TestUpdateUserEngine_IdempotentResendSucceeds cobre o caminho de SUCESSO
// da edição de engine que sobrevive à F279: reenviar o MESMO valor já
// gravado é um no-op tolerado (o mesmo comportamento que
// EditUserUseCase.Execute já documentava na fronteira HTTP - ver
// edit_user.go:66-73 - agora garantido pelo próprio repositório).
func TestUpdateUserEngine_IdempotentResendSucceeds(t *testing.T) {
	db := newUserTestDB(t)
	repo := dbpkg.NewUserRepository(db)
	ctx := context.Background()

	if _, err := repo.CreateUser(ctx, domain.UserRecord{ID: "u1", Name: "u1", Token: "tok-u1"}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	noise := domain.EngineWaNoise
	if err := repo.UpdateUser(ctx, "u1", domain.UserUpdate{Engine: &noise}); err != nil {
		t.Fatalf("UpdateUser with the SAME engine as already persisted: %v", err)
	}
	if got := engineOf(t, db, "u1"); got != string(domain.EngineWaNoise) {
		t.Fatalf("engine after idempotent update = %q, want %q", got, domain.EngineWaNoise)
	}

	entries, err := repo.ListUsers(ctx, "u1")
	if err != nil || len(entries) != 1 {
		t.Fatalf("ListUsers: %v (%d entries)", err, len(entries))
	}
	if entries[0].Engine != domain.EngineWaNoise {
		t.Errorf("ListUsers engine = %q, want %q", entries[0].Engine, domain.EngineWaNoise)
	}
}

// TestUpdateUserEngine_DivergentValueRejected is the F279 regression test at
// this layer: changing a session's engine to a DIFFERENT (but otherwise
// valid) value must now fail with domain.ErrEngineImmutable, and must not
// touch the row. This use to be the success path this same test file
// documented (TestUpdateUserEngine, before F279) - that was exactly the bug:
// the repository accepted a divergent engine with no error, and only
// EditUserUseCase.Execute, one layer above, ever refused it.
func TestUpdateUserEngine_DivergentValueRejected(t *testing.T) {
	db := newUserTestDB(t)
	repo := dbpkg.NewUserRepository(db)
	ctx := context.Background()

	if _, err := repo.CreateUser(ctx, domain.UserRecord{ID: "u1", Name: "u1", Token: "tok-u1"}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	headless := domain.EngineWaHeadless
	err := repo.UpdateUser(ctx, "u1", domain.UserUpdate{Engine: &headless})
	if !errors.Is(err, domain.ErrEngineImmutable) {
		t.Fatalf("UpdateUser(engine=%q) err = %v, want errors.Is(err, domain.ErrEngineImmutable)", headless, err)
	}
	if got := engineOf(t, db, "u1"); got != string(domain.EngineWaNoise) {
		t.Fatalf("engine changed despite the rejected update: got %q, want %q", got, domain.EngineWaNoise)
	}
}

// TestUpdateUserRejectsInvalidEngine: recusa E preserva o valor anterior.
// Um valor que não é um engine válido de jeito nenhum é recusado com
// ErrInvalidEngine, não ErrEngineImmutable - validade é checada ANTES de
// imutabilidade (ver o comentário em UpdateUser).
func TestUpdateUserRejectsInvalidEngine(t *testing.T) {
	db := newUserTestDB(t)
	repo := dbpkg.NewUserRepository(db)
	ctx := context.Background()

	if _, err := repo.CreateUser(ctx, domain.UserRecord{ID: "u1", Name: "u1", Token: "tok-u1"}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	for _, bad := range []domain.Engine{domain.EngineLegacyUnknown, "", "headless"} {
		e := bad
		if err := repo.UpdateUser(ctx, "u1", domain.UserUpdate{Engine: &e}); !errors.Is(err, domain.ErrInvalidEngine) {
			t.Errorf("UpdateUser(engine=%q) err = %v, want ErrInvalidEngine", bad, err)
		}
		if got := engineOf(t, db, "u1"); got != string(domain.EngineWaNoise) {
			t.Errorf("UpdateUser(engine=%q) changed the row to %q", bad, got)
		}
	}
}
