package db_test

// Testes de integração dos use cases de user/ contra um banco real.
//
// Vieram de pkg/application/usecase/user/add_user_token_test.go. Mudaram de
// lugar, não de conteúdo: a partir da Fase 6 os use cases não recebem mais
// *sqlx.DB — recebem appport.UserRepository — e um teste que abre um sqlite
// de verdade passa a ser teste do adapter de persistência. Manter o arquivo
// na camada de aplicação exigiria abrir uma exceção de depguard para
// jmoiron/sqlx exatamente no diretório que a fase acabou de fechar.
//
// Todas as asserções são as de antes, inclusive o controle de TOCTOU.

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
	dbpkg "wa-api/pkg/infra/db"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type discardLogger struct{}

func (discardLogger) Info(context.Context, string, ...any)  {}
func (discardLogger) Warn(context.Context, string, ...any)  {}
func (discardLogger) Error(context.Context, string, ...any) {}

func newUserTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	if err := dbpkg.InitializeSchema(db); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	return db
}

func TestAddUserRejectsDuplicateToken(t *testing.T) {
	db := newUserTestDB(t)
	uc := user.NewAddUserUseCase(dbpkg.NewUserRepository(db), discardLogger{})
	ctx := context.Background()

	if _, err := uc.Execute(ctx, domain.AddUserRequest{Name: "alice", Token: "shared"}); err != nil {
		t.Fatalf("first add: %v", err)
	}

	_, err := uc.Execute(ctx, domain.AddUserRequest{Name: "mallory", Token: "shared"})
	if !errors.Is(err, user.ErrDuplicateToken) {
		t.Fatalf("second add error = %v, want user.ErrDuplicateToken", err)
	}

	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM users WHERE token_hash = ?", domain.HashToken("shared")); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Errorf("rows with the shared token = %d, want 1", count)
	}
}

// TestAddUserConcurrentSameTokenCreatesOneRow é o controle do TOCTOU: o par
// SELECT COUNT(*)/INSERT anterior rodava fora de transação, então duas
// requisições simultâneas com o mesmo token passavam ambas pela checagem.
func TestAddUserConcurrentSameTokenCreatesOneRow(t *testing.T) {
	db := newUserTestDB(t)
	uc := user.NewAddUserUseCase(dbpkg.NewUserRepository(db), discardLogger{})

	const attempts = 8
	var wg sync.WaitGroup
	successes := make([]bool, attempts)
	for i := range attempts {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := uc.Execute(context.Background(),
				domain.AddUserRequest{Name: "racer", Token: "contended"})
			successes[idx] = err == nil
		}(i)
	}
	wg.Wait()

	won := 0
	for _, ok := range successes {
		if ok {
			won++
		}
	}
	if won != 1 {
		t.Errorf("concurrent adds that succeeded = %d, want exactly 1", won)
	}

	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM users WHERE token_hash = ?", domain.HashToken("contended")); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Errorf("rows persisted = %d, want 1", count)
	}
}

func TestAddUserPersistsTokenHash(t *testing.T) {
	db := newUserTestDB(t)
	uc := user.NewAddUserUseCase(dbpkg.NewUserRepository(db), discardLogger{})

	resp, err := uc.Execute(context.Background(), domain.AddUserRequest{Name: "alice", Token: "tok"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	var got string
	if err := db.Get(&got, "SELECT token_hash FROM users WHERE id = ?", resp.ID); err != nil {
		t.Fatalf("read token_hash: %v", err)
	}
	if want := domain.HashToken("tok"); got != want {
		t.Errorf("token_hash = %q, want %q", got, want)
	}
}

func TestEditUserRejectsTokenBelongingToAnotherUser(t *testing.T) {
	db := newUserTestDB(t)
	ctx := context.Background()
	add := user.NewAddUserUseCase(dbpkg.NewUserRepository(db), discardLogger{})

	if _, err := add.Execute(ctx, domain.AddUserRequest{Name: "alice", Token: "alice-token"}); err != nil {
		t.Fatalf("add alice: %v", err)
	}
	bob, err := add.Execute(ctx, domain.AddUserRequest{Name: "bob", Token: "bob-token"})
	if err != nil {
		t.Fatalf("add bob: %v", err)
	}

	edit := user.NewEditUserUseCase(dbpkg.NewUserRepository(db), discardLogger{})
	err = edit.Execute(ctx, domain.EditUserRequest{UserID: bob.ID, Token: "alice-token"})
	if !errors.Is(err, user.ErrDuplicateToken) {
		t.Fatalf("edit error = %v, want user.ErrDuplicateToken", err)
	}

	var stillBob string
	if err := db.Get(&stillBob, "SELECT token FROM users WHERE id = ?", bob.ID); err != nil {
		t.Fatalf("read bob token: %v", err)
	}
	if stillBob != "bob-token" {
		t.Errorf("bob token = %q, want it unchanged", stillBob)
	}
}

func TestEditUserUpdatesTokenHashAlongsideToken(t *testing.T) {
	db := newUserTestDB(t)
	ctx := context.Background()

	created, err := user.NewAddUserUseCase(dbpkg.NewUserRepository(db), discardLogger{}).
		Execute(ctx, domain.AddUserRequest{Name: "alice", Token: "old-token"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	if err := user.NewEditUserUseCase(dbpkg.NewUserRepository(db), discardLogger{}).
		Execute(ctx, domain.EditUserRequest{UserID: created.ID, Token: "new-token"}); err != nil {
		t.Fatalf("edit: %v", err)
	}

	var got string
	if err := db.Get(&got, "SELECT token_hash FROM users WHERE id = ?", created.ID); err != nil {
		t.Fatalf("read token_hash: %v", err)
	}
	if want := domain.HashToken("new-token"); got != want {
		t.Errorf("token_hash = %q, want %q (hash left stale after token change)", got, want)
	}
}

func TestListUsersDoesNotReturnPlaintextToken(t *testing.T) {
	db := newUserTestDB(t)
	ctx := context.Background()

	if _, err := user.NewAddUserUseCase(dbpkg.NewUserRepository(db), discardLogger{}).
		Execute(ctx, domain.AddUserRequest{Name: "alice", Token: "secret-token"}); err != nil {
		t.Fatalf("add: %v", err)
	}

	users, err := user.NewListUsersUseCase(dbpkg.NewUserRepository(db), discardLogger{}, stubSessionStatus{}).
		Execute(ctx, domain.ListUsersRequest{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("users = %d, want 1", len(users))
	}
	if users[0].Token != "" {
		t.Errorf("Token = %q, want empty; GET /admin/users must not leak credentials", users[0].Token)
	}
}

// stubSessionStatus reporta sempre "sem sessão". Antes da ADR-001 a fake
// equivalente precisava de três métodos, um deles devolvendo interface{}.
type stubSessionStatus struct{}

func (stubSessionStatus) SessionStatus(context.Context, string) (bool, bool) { return false, false }
