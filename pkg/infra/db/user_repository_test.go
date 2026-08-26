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

	"wa-api/pkg/application/contracts/contractsfake"
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
	db, err := sqlx.Open("sqlite",
		filepath.Join(t.TempDir(), "user.db")+dbpkg.SQLitePragmas)
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
	uc := user.NewAddUserUseCase(dbpkg.NewUserRepository(db), &contractsfake.HmacKeyEncryptor{}, &contractsfake.S3SecretCipher{}, discardLogger{})
	ctx := context.Background()

	if _, err := uc.Execute(ctx, domain.AddUserRequest{Name: "alice", Token: "shared", Engine: "wa_noise"}); err != nil {
		t.Fatalf("first add: %v", err)
	}

	_, err := uc.Execute(ctx, domain.AddUserRequest{Name: "mallory", Token: "shared", Engine: "wa_noise"})
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
	uc := user.NewAddUserUseCase(dbpkg.NewUserRepository(db), &contractsfake.HmacKeyEncryptor{}, &contractsfake.S3SecretCipher{}, discardLogger{})

	const attempts = 8
	var wg sync.WaitGroup
	successes := make([]bool, attempts)
	for i := range attempts {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := uc.Execute(context.Background(),
				domain.AddUserRequest{Name: "racer", Token: "contended", Engine: "wa_noise"})
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
	uc := user.NewAddUserUseCase(dbpkg.NewUserRepository(db), &contractsfake.HmacKeyEncryptor{}, &contractsfake.S3SecretCipher{}, discardLogger{})

	resp, err := uc.Execute(context.Background(), domain.AddUserRequest{Name: "alice", Token: "tok", Engine: "wa_noise"})
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
	add := user.NewAddUserUseCase(dbpkg.NewUserRepository(db), &contractsfake.HmacKeyEncryptor{}, &contractsfake.S3SecretCipher{}, discardLogger{})

	if _, err := add.Execute(ctx, domain.AddUserRequest{Name: "alice", Token: "alice-token", Engine: "wa_noise"}); err != nil {
		t.Fatalf("add alice: %v", err)
	}
	bob, err := add.Execute(ctx, domain.AddUserRequest{Name: "bob", Token: "bob-token", Engine: "wa_noise"})
	if err != nil {
		t.Fatalf("add bob: %v", err)
	}

	edit := user.NewEditUserUseCase(dbpkg.NewUserRepository(db), &contractsfake.S3SecretCipher{}, &contractsfake.UserInfoRepublisher{}, discardLogger{})
	err = edit.Execute(ctx, domain.EditUserRequest{UserID: bob.ID, Token: "alice-token"})
	if !errors.Is(err, user.ErrDuplicateToken) {
		t.Fatalf("edit error = %v, want user.ErrDuplicateToken", err)
	}

	// Confere pelo HASH, e nao pela coluna em texto claro: desde a F97 etapa 1
	// ela e sempre vazia, entao compara-la nao distinguiria "a edicao foi
	// recusada" de "a edicao passou e apagou o token" — os dois dariam "".
	var stillBob string
	if err := db.Get(&stillBob, "SELECT token_hash FROM users WHERE id = ?", bob.ID); err != nil {
		t.Fatalf("read bob token_hash: %v", err)
	}
	if want := domain.HashToken("bob-token"); stillBob != want {
		t.Errorf("bob token_hash = %q, want %q (a edicao recusada nao pode ter mexido nele)", stillBob, want)
	}
}

func TestEditUserUpdatesTokenHashAlongsideToken(t *testing.T) {
	db := newUserTestDB(t)
	ctx := context.Background()

	created, err := user.NewAddUserUseCase(dbpkg.NewUserRepository(db), &contractsfake.HmacKeyEncryptor{}, &contractsfake.S3SecretCipher{}, discardLogger{}).
		Execute(ctx, domain.AddUserRequest{Name: "alice", Token: "old-token", Engine: "wa_noise"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	if err := user.NewEditUserUseCase(dbpkg.NewUserRepository(db), &contractsfake.S3SecretCipher{}, &contractsfake.UserInfoRepublisher{}, discardLogger{}).
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

	if _, err := user.NewAddUserUseCase(dbpkg.NewUserRepository(db), &contractsfake.HmacKeyEncryptor{}, &contractsfake.S3SecretCipher{}, discardLogger{}).
		Execute(ctx, domain.AddUserRequest{Name: "alice", Token: "secret-token", Engine: "wa_noise"}); err != nil {
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

// TestCreateUser_NaoGravaOTextoClaro fixa a F97 etapa 1 pelas DUAS metades,
// porque uma sem a outra é inútil ou perigosa.
//
// Metade 1: o texto claro não é gravado. Quem obtiver leitura do banco —
// backup, réplica, dump de suporte — não obtém credencial utilizável.
//
// Metade 2: o hash É gravado. Sem ele o usuário nasceria sem NENHUMA forma de
// autenticar, e a "melhoria de segurança" seria uma conta inutilizável.
func TestCreateUser_NaoGravaOTextoClaro(t *testing.T) {
	db := newUserTestDB(t)
	repo := dbpkg.NewUserRepository(db)
	const token = "tok-em-claro"
	if _, err := repo.CreateUser(context.Background(), domain.UserRecord{
		ID: "u1", Name: "alice", Token: token,
	}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	var linha struct {
		Token     string `db:"token"`
		TokenHash string `db:"token_hash"`
	}
	if err := db.Get(&linha, "SELECT token, token_hash FROM users WHERE id = ?", "u1"); err != nil {
		t.Fatalf("ler usuario: %v", err)
	}

	if linha.Token != "" {
		t.Errorf("token = %q, want vazio: o texto claro continua no banco e a F97 nao valeu de nada", linha.Token)
	}
	if want := domain.HashToken(token); linha.TokenHash != want {
		t.Fatalf("token_hash = %q, want %q: o usuario nasceu sem forma de autenticar", linha.TokenHash, want)
	}
}
