package media

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"wa-api/pkg/infra/storage"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// openUsersDB abre um SQLite em arquivo (nao :memory:, porque o pool do
// database/sql pode abrir mais de uma conexao e cada uma veria um banco
// diferente) com apenas as colunas que ProcessOutgoingMedia consulta.
func openUsersDB(t *testing.T, createTable bool) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("fechar db: %v", err)
		}
	})
	if createTable {
		if _, err := db.Exec(`CREATE TABLE users (id TEXT PRIMARY KEY, s3_enabled BOOLEAN, media_delivery TEXT)`); err != nil {
			t.Fatalf("criar tabela: %v", err)
		}
	}
	return db
}

func seedUser(t *testing.T, db *sqlx.DB, id string, enabled bool, delivery string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO users (id, s3_enabled, media_delivery) VALUES ($1, $2, $3)`, id, enabled, delivery); err != nil {
		t.Fatalf("semear usuario: %v", err)
	}
}

func TestProcessOutgoingMediaConsultaFalha(t *testing.T) {
	// Sem a tabela users a consulta falha; a funcao degrada para base64
	// desligado e devolve nil sem erro.
	db := openUsersDB(t, false)

	got, err := ProcessOutgoingMedia("u1", "chat@s.whatsapp.net", "msg-1", []byte("dados"), "image/png", "f.png", db)

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != nil {
		t.Fatalf("quero nil, tenho %v", got)
	}
}

func TestProcessOutgoingMediaS3Desligado(t *testing.T) {
	db := openUsersDB(t, true)
	seedUser(t, db, "u1", false, "s3")

	got, err := ProcessOutgoingMedia("u1", "chat@s.whatsapp.net", "msg-1", []byte("dados"), "image/png", "f.png", db)

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != nil {
		t.Fatalf("quero nil, tenho %v", got)
	}
}

func TestProcessOutgoingMediaEntregaBase64(t *testing.T) {
	db := openUsersDB(t, true)
	seedUser(t, db, "u1", true, "base64")

	got, err := ProcessOutgoingMedia("u1", "chat@s.whatsapp.net", "msg-1", []byte("dados"), "image/png", "f.png", db)

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != nil {
		t.Fatalf("entrega base64 nao passa pelo S3; quero nil, tenho %v", got)
	}
}

func TestProcessOutgoingMediaUploadBemSucedido(t *testing.T) {
	// Um endpoint S3 de mentira que aceita o PutObject. Com PublicURL
	// configurado, a montagem da URL e' pura, entao esta e' a unica chamada de
	// rede do caminho feliz.
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	const userID = "wa-api-test-upload-ok"
	manager := storage.GetS3Manager()
	if err := manager.InitializeS3Client(userID, &storage.S3Config{
		Enabled:   true,
		Endpoint:  srv.URL,
		Region:    "us-east-1",
		Bucket:    "midias",
		AccessKey: "chave",
		SecretKey: "segredo",
		PathStyle: true,
		PublicURL: "https://cdn.exemplo",
	}); err != nil {
		t.Fatalf("inicializar cliente S3: %v", err)
	}
	t.Cleanup(func() { manager.RemoveClient(userID) })

	db := openUsersDB(t, true)
	seedUser(t, db, userID, true, "s3")

	got, err := ProcessOutgoingMedia(userID, "chat@s.whatsapp.net", "msg-ok", []byte("dados"), "image/png", "f.png", db)

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got == nil {
		t.Fatal("quero os metadados do upload, tenho nil")
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("metodo = %q, quero PUT", gotMethod)
	}
	if !strings.HasPrefix(gotPath, "/midias/") {
		t.Fatalf("caminho = %q, quero prefixo /midias/ (path style)", gotPath)
	}
	for _, want := range []struct {
		key string
		val interface{}
	}{
		{"bucket", "midias"},
		{"size", 5},
		{"fileName", "f.png"},
		{"mimeType", "image/png"},
	} {
		if got[want.key] != want.val {
			t.Errorf("%s = %v, quero %v", want.key, got[want.key], want.val)
		}
	}
	if url, _ := got["url"].(string); !strings.HasPrefix(url, "https://cdn.exemplo/midias/") {
		t.Errorf("url = %v", got["url"])
	}
}

func TestProcessOutgoingMediaUploadFalhaSemCliente(t *testing.T) {
	tests := []string{"s3", "both"}
	for _, delivery := range tests {
		t.Run(delivery, func(t *testing.T) {
			db := openUsersDB(t, true)
			seedUser(t, db, "u1", true, delivery)

			// O S3Manager global nao tem cliente para "u1", entao o upload
			// falha: a funcao loga e devolve nil sem propagar o erro.
			got, err := ProcessOutgoingMedia("u1", "chat@s.whatsapp.net", "msg-1", []byte("dados"), "image/png", "f.png", db)

			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if got != nil {
				t.Fatalf("quero nil apos a falha de upload, tenho %v", got)
			}
		})
	}
}
