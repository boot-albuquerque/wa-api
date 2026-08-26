package db_test

// Testes da coluna `users.account_type` (migração 21). O banco é um SQLite
// real, criado por dbpkg.InitializeSchema — o mesmo caminho que a produção
// percorre, seguindo o precedente de user_engine_test.go.

import (
	"context"
	"testing"

	"wa-api/pkg/domain"
	dbpkg "wa-api/pkg/infra/db"
)

// accountTypeOf lê a coluna crua, sem passar por GetUserAccountType: o teste
// do DEFAULT tem de ver o que o ALTER TABLE gravou, não o que a leitura decide
// devolver.
func accountTypeOf(t *testing.T, db interface {
	Get(dest any, query string, args ...any) error
}, id string) string {
	t.Helper()
	var got string
	if err := db.Get(&got, `SELECT account_type FROM users WHERE id = $1`, id); err != nil {
		t.Fatalf("read account_type of %q: %v", id, err)
	}
	return got
}

// TestMigrationAddsAccountTypeColumnWithUnknownDefault trava o DEFAULT do
// ALTER: uma linha inserida sem citar a coluna tem de cair em "unknown", nunca
// em "personal" — a migração não mede nada, então não pode afirmar nada.
func TestMigrationAddsAccountTypeColumnWithUnknownDefault(t *testing.T) {
	db := newUserTestDB(t)

	if _, err := db.Exec(
		`INSERT INTO users (id, name, token, token_hash, webhook, expiration, events, jid, qrcode)
		 VALUES ('legacy-1', 'legacy', '', 'h1', '', 0, '', '', '')`); err != nil {
		t.Fatalf("insert without account_type: %v", err)
	}

	if got := accountTypeOf(t, db, "legacy-1"); got != string(domain.AccountTypeUnknown) {
		t.Errorf("account_type default = %q, want %q", got, domain.AccountTypeUnknown)
	}
}

// TestSetAndGetUserAccountType_RoundTrip trava a persistencia: o que foi
// escrito e' o que volta a ser lido.
func TestSetAndGetUserAccountType_RoundTrip(t *testing.T) {
	db := newUserTestDB(t)
	if _, err := db.Exec(
		`INSERT INTO users (id, name, token, token_hash, webhook, expiration, events, jid, qrcode)
		 VALUES ('u-1', 'user', '', 'h1', '', 0, '', '', '')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := dbpkg.SetUserAccountType(context.Background(), db, "u-1", domain.AccountTypeBusiness); err != nil {
		t.Fatalf("SetUserAccountType: %v", err)
	}

	got, err := dbpkg.GetUserAccountType(context.Background(), db, "u-1")
	if err != nil {
		t.Fatalf("GetUserAccountType: %v", err)
	}
	if got != domain.AccountTypeBusiness {
		t.Errorf("GetUserAccountType = %q, want %q", got, domain.AccountTypeBusiness)
	}

	// Sobrescrever para unknown tambem tem de ser honrado: revalidacao (item
	// 34) pode rebaixar uma classificacao anterior se o sinal deixar de estar
	// disponivel, e a persistencia nao pode recusar isso.
	if err := dbpkg.SetUserAccountType(context.Background(), db, "u-1", domain.AccountTypeUnknown); err != nil {
		t.Fatalf("SetUserAccountType(unknown): %v", err)
	}
	got, err = dbpkg.GetUserAccountType(context.Background(), db, "u-1")
	if err != nil {
		t.Fatalf("GetUserAccountType: %v", err)
	}
	if got != domain.AccountTypeUnknown {
		t.Errorf("GetUserAccountType after downgrade = %q, want %q", got, domain.AccountTypeUnknown)
	}
}

// TestGetUserAccountType_NoSuchUser: linha ausente devolve erro, nunca
// "unknown" silencioso que se confundiria com uma leitura bem-sucedida.
func TestGetUserAccountType_NoSuchUser(t *testing.T) {
	db := newUserTestDB(t)
	_, err := dbpkg.GetUserAccountType(context.Background(), db, "nao-existe")
	if err == nil {
		t.Fatal("esperava erro para usuario inexistente")
	}
}
