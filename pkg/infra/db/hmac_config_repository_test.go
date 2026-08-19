package db

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
)

// O adapter de users.hmac_key contra o schema de producao.
//
// As tres instrucoes sao constantes do proprio arquivo de producao, entao o
// que este teste mede e' se elas CASAM com o schema real — e' o buraco da F71,
// em que uma query pedia uma coluna inexistente e nenhum teste a executava.

const hmacRepoUserID = "user-1"

func newHmacRepoDB(t *testing.T) *sqlx.DB {
	t.Helper()
	database := openTestDB(t)
	if err := InitializeSchema(database); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	if _, err := database.Exec(database.Rebind(
		`INSERT INTO users (id, name, token, token_hash) VALUES (?, ?, ?, ?)`),
		hmacRepoUserID, "tenant", "tok", "hash"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return database
}

// Linha ausente devolve (nil, nil): o handler historico respondia
// sql.ErrNoRows com 200 e chave vazia (41bc8e2^:handlers.go:6832). Se o
// adapter propagasse o erro, a rota viraria 500 para um usuario inexistente.
func TestHmacConfigRepository_LoadSemLinha_NaoEhErro(t *testing.T) {
	repo := NewHmacConfigRepository(newHmacRepoDB(t))

	chave, err := repo.LoadHmacKey(context.Background(), "quem-nao-existe")
	if err != nil {
		t.Fatalf("linha ausente devolveu erro: %v", err)
	}
	if chave != nil {
		t.Errorf("chave = %q, quero nil", chave)
	}
}

// Coluna NULL (o estado de quem nunca configurou) chega como nil, e nao como
// erro de scan.
func TestHmacConfigRepository_LoadColunaNula(t *testing.T) {
	repo := NewHmacConfigRepository(newHmacRepoDB(t))

	chave, err := repo.LoadHmacKey(context.Background(), hmacRepoUserID)
	if err != nil {
		t.Fatalf("coluna NULL devolveu erro: %v", err)
	}
	if len(chave) != 0 {
		t.Errorf("chave = %q, quero vazio", chave)
	}
}

// O ciclo completo: gravar, reler byte a byte, substituir e apagar.
func TestHmacConfigRepository_SaveLoadDelete(t *testing.T) {
	database := newHmacRepoDB(t)
	repo := NewHmacConfigRepository(database)
	ctx := context.Background()

	// Bytes NAO textuais de proposito: a coluna e' BYTEA/BLOB e guarda
	// ciphertext AES-GCM, que tem byte zero e byte alto. Um teste com ASCII
	// passaria mesmo se a coluna fosse TEXT.
	primeira := []byte{0x00, 0x01, 0xff, 0xfe, 'k', 'e', 'y'}
	if err := repo.SaveHmacKey(ctx, hmacRepoUserID, primeira); err != nil {
		t.Fatalf("SaveHmacKey: %v", err)
	}
	lida, err := repo.LoadHmacKey(ctx, hmacRepoUserID)
	if err != nil {
		t.Fatalf("LoadHmacKey: %v", err)
	}
	if string(lida) != string(primeira) {
		t.Fatalf("releitura = %v, quero %v", lida, primeira)
	}

	segunda := []byte{0x10, 0x20}
	if err := repo.SaveHmacKey(ctx, hmacRepoUserID, segunda); err != nil {
		t.Fatalf("SaveHmacKey (substituicao): %v", err)
	}
	if lida, err = repo.LoadHmacKey(ctx, hmacRepoUserID); err != nil || string(lida) != string(segunda) {
		t.Fatalf("substituicao = %v, %v; quero %v", lida, err, segunda)
	}

	if err := repo.DeleteHmacKey(ctx, hmacRepoUserID); err != nil {
		t.Fatalf("DeleteHmacKey: %v", err)
	}
	if lida, err = repo.LoadHmacKey(ctx, hmacRepoUserID); err != nil || len(lida) != 0 {
		t.Fatalf("apos apagar = %v, %v; quero vazio", lida, err)
	}

	// Revogacao e' idempotente: apagar de novo nao e' erro.
	if err := repo.DeleteHmacKey(ctx, hmacRepoUserID); err != nil {
		t.Fatalf("DeleteHmacKey repetido: %v", err)
	}
}

// Escrita e revogacao sao ESCOPADAS por usuario: mexer num nao pode tocar no
// outro.
func TestHmacConfigRepository_EscopoPorUsuario(t *testing.T) {
	database := newHmacRepoDB(t)
	repo := NewHmacConfigRepository(database)
	ctx := context.Background()

	const outro = "user-2"
	if _, err := database.Exec(database.Rebind(
		`INSERT INTO users (id, name, token, token_hash) VALUES (?, ?, ?, ?)`),
		outro, "tenant 2", "tok2", "hash2"); err != nil {
		t.Fatalf("seed user 2: %v", err)
	}

	chaveDoOutro := []byte{0xaa, 0xbb}
	if err := repo.SaveHmacKey(ctx, outro, chaveDoOutro); err != nil {
		t.Fatalf("SaveHmacKey user-2: %v", err)
	}
	if err := repo.SaveHmacKey(ctx, hmacRepoUserID, []byte{0x01}); err != nil {
		t.Fatalf("SaveHmacKey user-1: %v", err)
	}
	if err := repo.DeleteHmacKey(ctx, hmacRepoUserID); err != nil {
		t.Fatalf("DeleteHmacKey user-1: %v", err)
	}

	lida, err := repo.LoadHmacKey(ctx, outro)
	if err != nil {
		t.Fatalf("LoadHmacKey user-2: %v", err)
	}
	if string(lida) != string(chaveDoOutro) {
		t.Fatalf("a chave de user-2 virou %v apos operacoes em user-1; quero %v", lida, chaveDoOutro)
	}
}
