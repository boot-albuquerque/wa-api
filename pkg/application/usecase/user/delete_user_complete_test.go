package user_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"

	dbpkg "wa-api/pkg/infra/db"

	// DeleteUserCompleteUseCase ainda fala com *sql.DB direto (não passou pela
	// F6), então o único jeito de testá-lo é contra um banco de verdade. O
	// driver é o mesmo que o resto do repositório usa em teste.
	_ "modernc.org/sqlite"
)

// openTestDB abre um sqlite descartável e roda as instruções de ddl nele.
func openTestDB(t *testing.T, ddl ...string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "delete.db")+dbpkg.SQLitePragmas)
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("fechar db: %v", err)
		}
	})
	for _, stmt := range ddl {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("ddl %q: %v", stmt, err)
		}
	}
	return db
}

// fullSchema é a tabela com todas as colunas que o use case lê.
const fullSchema = `CREATE TABLE users (
	id TEXT PRIMARY KEY,
	name TEXT,
	jid TEXT,
	token TEXT,
	s3_enabled BOOLEAN DEFAULT 0
)`

func insertUser(t *testing.T, db *sql.DB, stmt string, args ...any) {
	t.Helper()
	if _, err := db.Exec(stmt, args...); err != nil {
		t.Fatalf("inserir usuário: %v", err)
	}
}

func TestDeleteUserCompleteUseCase_Execute_MissingID(t *testing.T) {
	t.Parallel()

	uc := user.NewDeleteUserCompleteUseCase(nil, &contractsfake.SessionController{}, &contractsfake.UserInfoRepublisher{}, &contractsfake.Logger{}, t.TempDir())
	if _, err := uc.Execute(context.Background(), ""); err == nil {
		t.Fatal("esperava erro para ID vazio")
	}
}

func TestDeleteUserCompleteUseCase_Execute_ExistenceQueryFails(t *testing.T) {
	t.Parallel()

	// Sem a tabela users, a primeira consulta já falha.
	db := openTestDB(t)
	logger := &contractsfake.Logger{}
	uc := user.NewDeleteUserCompleteUseCase(db, &contractsfake.SessionController{}, &contractsfake.UserInfoRepublisher{}, logger, t.TempDir())

	if _, err := uc.Execute(context.Background(), "u1"); err == nil {
		t.Fatal("esperava erro de banco")
	}
	if !logger.Logged("database error checking user existence") {
		t.Errorf("log ausente; houve %v", logger.Messages())
	}
}

func TestDeleteUserCompleteUseCase_Execute_UserNotFound(t *testing.T) {
	t.Parallel()

	db := openTestDB(t, fullSchema)
	uc := user.NewDeleteUserCompleteUseCase(db, &contractsfake.SessionController{}, &contractsfake.UserInfoRepublisher{}, &contractsfake.Logger{}, t.TempDir())

	if _, err := uc.Execute(context.Background(), "ausente"); err == nil {
		t.Fatal("esperava erro de usuário inexistente")
	}
}

func TestDeleteUserCompleteUseCase_Execute_PartialSchemaKeepsGoing(t *testing.T) {
	t.Parallel()

	// Só a coluna id: as consultas de dados do usuário e de configuração S3
	// falham, e o contrato é seguir adiante — a deleção ainda acontece.
	db := openTestDB(t, `CREATE TABLE users (id TEXT PRIMARY KEY)`)
	insertUser(t, db, `INSERT INTO users (id) VALUES ('u1')`)
	logger := &contractsfake.Logger{}
	uc := user.NewDeleteUserCompleteUseCase(db, &contractsfake.SessionController{}, &contractsfake.UserInfoRepublisher{}, logger, t.TempDir())

	res, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	// Success/Code saíram do resultado: eram uma segunda cópia do envelope
	// HTTP dentro do domínio. O sucesso é `err == nil`, e o corpo devolvido
	// é o utilizador removido.
	if res.User.ID != "u1" {
		t.Errorf("resultado = %+v", res)
	}
	for _, msg := range []string{"problem retrieving user information", "problem retrieving user s3 configuration"} {
		if !logger.Logged(msg) {
			t.Errorf("log %q ausente; houve %v", msg, logger.Messages())
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = 'u1'`).Scan(&count); err != nil {
		t.Fatalf("contar usuários: %v", err)
	}
	if count != 0 {
		t.Errorf("usuário ainda presente após a deleção (count=%d)", count)
	}
}

func TestDeleteUserCompleteUseCase_Execute_DeleteFails(t *testing.T) {
	t.Parallel()

	db := openTestDB(t, fullSchema,
		`CREATE TRIGGER no_delete BEFORE DELETE ON users BEGIN SELECT RAISE(ABORT, 'proibido'); END`)
	insertUser(t, db, `INSERT INTO users (id, name, jid, token, s3_enabled) VALUES ('u1','alice','5511@s.whatsapp.net','tok',0)`)
	logger := &contractsfake.Logger{}
	rep := &contractsfake.UserInfoRepublisher{}
	uc := user.NewDeleteUserCompleteUseCase(db, &contractsfake.SessionController{}, rep, logger, t.TempDir())

	if _, err := uc.Execute(context.Background(), "u1"); err == nil {
		t.Fatal("esperava erro de banco na deleção")
	}
	if !logger.Logged("database error deleting user") {
		t.Errorf("log ausente; houve %v", logger.Messages())
	}
	// F273 companion assertion: a deleção falhou (o trigger a barrou), então
	// a cache do usuário — que ainda existe na tabela — não pode ser
	// invalidada. Se isso acontecesse, uma leitura concorrente repovoaria a
	// cache a partir da MESMA linha, mas o token ficaria descoberto por uma
	// janela sem necessidade.
	if len(rep.RepublishCalls) != 0 {
		t.Errorf("RepublishUser chamado %d vez(es), queria 0 — a deleção falhou", len(rep.RepublishCalls))
	}
}

// TestDeleteUserCompleteUseCase_Execute_InvalidaCacheAposDeletar é o teste
// do defeito F273 para este segundo caminho de remoção
// (`DELETE /admin/users/{id}/full`): a entrada mede que `GET /webhook`
// respondia 200 com o token da sessão `descartavel-4` mesmo depois do
// `/full`. Trava também a ORDEM — apagar a linha primeiro, invalidar
// depois — com o mesmo dublê contador que delete_user_test.go usa.
func TestDeleteUserCompleteUseCase_Execute_InvalidaCacheAposDeletar(t *testing.T) {
	t.Parallel()

	db := openTestDB(t, fullSchema)
	insertUser(t, db, `INSERT INTO users (id, name, jid, token, s3_enabled) VALUES ('descartavel-4','alice','5511@s.whatsapp.net','tok-desc-4',0)`)
	logger := &contractsfake.Logger{}
	rep := &contractsfake.UserInfoRepublisher{
		ContadorDeEscritas: func() int {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = 'descartavel-4'`).Scan(&count); err != nil {
				t.Fatalf("contar usuários: %v", err)
			}
			// 0 restante == já apagado.
			if count == 0 {
				return 1
			}
			return 0
		},
	}
	uc := user.NewDeleteUserCompleteUseCase(db, &contractsfake.SessionController{}, rep, logger, t.TempDir())

	if _, err := uc.Execute(context.Background(), "descartavel-4"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if len(rep.RepublishCalls) != 1 {
		t.Fatalf("RepublishUser chamado %d vez(es), queria 1", len(rep.RepublishCalls))
	}
	if rep.RepublishCalls[0].UserID != "descartavel-4" {
		t.Errorf("RepublishUser userID = %q, queria %q", rep.RepublishCalls[0].UserID, "descartavel-4")
	}
	if len(rep.EscritasAoSerChamado) != 1 || rep.EscritasAoSerChamado[0] != 1 {
		t.Fatalf("invalidou ANTES de a linha sumir da tabela: EscritasAoSerChamado=%v", rep.EscritasAoSerChamado)
	}
}

func TestDeleteUserCompleteUseCase_Execute_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		s3Enabled    bool
		connected    bool
		sessionErr   error
		createDir    bool
		wantLogs     []string
		wantLogout   int
		wantDiscon   int
		wantMissLogs []string
	}{
		{
			name:         "sessão ausente não tenta logout",
			sessionErr:   errors.New("sem sessão"),
			wantMissLogs: []string{"Logging out user", "Disconnecting from WhatsApp"},
		},
		{
			name:         "sessão presente mas desconectada só desconecta",
			wantLogs:     []string{"Disconnecting from WhatsApp"},
			wantMissLogs: []string{"Logging out user"},
			wantDiscon:   1,
		},
		{
			name:       "sessão conectada faz logout e desconecta",
			connected:  true,
			wantLogs:   []string{"Logging out user", "Disconnecting from WhatsApp"},
			wantLogout: 1,
			wantDiscon: 1,
		},
		{
			name:       "diretório de mídia é removido",
			createDir:  true,
			wantLogs:   []string{"deleting media and history files from disk"},
			wantDiscon: 1,
		},
		{
			name:       "s3 habilitado sinaliza a limpeza pendente",
			s3Enabled:  true,
			wantLogs:   []string{"S3 deletion needed - to be handled by handler"},
			wantDiscon: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t, fullSchema)
			insertUser(t, db,
				`INSERT INTO users (id, name, jid, token, s3_enabled) VALUES (?,?,?,?,?)`,
				"u1", "alice", "5511@s.whatsapp.net", "tok", tt.s3Enabled)

			exPath := t.TempDir()
			if tt.createDir {
				dir := filepath.Join(exPath, "files", "u1")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("criar diretório de mídia: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dir, "a.bin"), []byte("x"), 0o600); err != nil {
					t.Fatalf("criar arquivo de mídia: %v", err)
				}
			}

			sc := &contractsfake.SessionController{}
			if tt.sessionErr != nil {
				sc.SessionGuard = contractsfake.FailSession(tt.sessionErr)
			}
			sc.SessionStatusFunc = func(context.Context, string) (bool, bool) { return tt.connected, tt.connected }
			logger := &contractsfake.Logger{}
			uc := user.NewDeleteUserCompleteUseCase(db, sc, &contractsfake.UserInfoRepublisher{}, logger, exPath)

			res, err := uc.Execute(context.Background(), "u1")
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if res.User.ID != "u1" || res.User.Name != "alice" {
				t.Errorf("resultado = %+v", res)
			}
			if res.User.JID != "5511@s.whatsapp.net" {
				t.Errorf("JID = %q", res.User.JID)
			}
			if res.Details == "" {
				t.Error("Details vazio: era calculado e descartado pelo handler, e agora é servido")
			}
			for _, msg := range tt.wantLogs {
				if !logger.Logged(msg) {
					t.Errorf("log %q ausente; houve %v", msg, logger.Messages())
				}
			}
			for _, msg := range tt.wantMissLogs {
				if logger.Logged(msg) {
					t.Errorf("log %q presente e não deveria", msg)
				}
			}
			if len(sc.LogoutCalls) != tt.wantLogout {
				t.Errorf("Logout chamado %d vezes, queria %d", len(sc.LogoutCalls), tt.wantLogout)
			}
			if len(sc.DisconnectCalls) != tt.wantDiscon {
				t.Errorf("Disconnect chamado %d vezes, queria %d", len(sc.DisconnectCalls), tt.wantDiscon)
			}
			if tt.createDir {
				if _, err := os.Stat(filepath.Join(exPath, "files", "u1")); !os.IsNotExist(err) {
					t.Errorf("diretório de mídia sobreviveu: %v", err)
				}
			}
			// O usuário sai do banco em todos os caminhos de sucesso.
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = 'u1'`).Scan(&count); err != nil {
				t.Fatalf("contar usuários: %v", err)
			}
			if count != 0 {
				t.Errorf("usuário ainda presente (count=%d)", count)
			}
		})
	}
}
