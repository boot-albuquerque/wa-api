package db

import (
	"context"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	appport "wa-api/pkg/application/contracts"
)

// O adapter das dez colunas s3_* contra o schema de producao.
//
// As quatro instrucoes sao constantes do proprio arquivo de producao, entao o
// que este teste mede e' se elas CASAM com o schema real — e' o buraco da F71,
// em que uma query pedia uma coluna inexistente e nenhum teste a executava.

const s3RepoUserID = "user-s3-repo"

func newS3RepoDB(t *testing.T) *sqlx.DB {
	t.Helper()
	database := openTestDB(t)
	if err := InitializeSchema(database); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	if _, err := database.Exec(database.Rebind(
		`INSERT INTO users (id, name, token, token_hash) VALUES (?, ?, ?, ?)`),
		s3RepoUserID, "tenant", "tok-s3", "hash-s3"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return database
}

func sampleS3Config() appport.S3ConfigRecord {
	return appport.S3ConfigRecord{
		Enabled:       true,
		Endpoint:      "https://203.0.113.10",
		Region:        "us-east-1",
		Bucket:        "mybucket",
		AccessKey:     "AKIAEXAMPLE",
		SecretKey:     "enc:v1:Zm9vYmFy",
		PathStyle:     true,
		PublicURL:     "https://cdn.example.com",
		MediaDelivery: "both",
		RetentionDays: 7,
	}
}

// As dez colunas fazem ida e volta. Gravar nove e esquecer uma nao produziria
// erro nenhum — so' um campo silenciosamente zerado na proxima leitura.
func TestS3ConfigRepository_SaveELoadPreservamAsDezColunas(t *testing.T) {
	repo := NewS3ConfigRepository(newS3RepoDB(t))
	quero := sampleS3Config()

	if err := repo.SaveS3Config(context.Background(), s3RepoUserID, quero); err != nil {
		t.Fatalf("SaveS3Config: %v", err)
	}
	tenho, err := repo.LoadS3Config(context.Background(), s3RepoUserID)
	if err != nil {
		t.Fatalf("LoadS3Config: %v", err)
	}
	if tenho == nil {
		t.Fatal("LoadS3Config devolveu nil para uma linha que existe")
	}
	if *tenho != quero {
		t.Fatalf("ida e volta perdeu campo:\n tenho %+v\n quero %+v", *tenho, quero)
	}
}

// A leitura da rota GET nao pode trazer o segredo. A assercao e' dupla: sobre
// o valor devolvido E sobre a INSTRUCAO, porque um blank no lado do caller
// deixaria o segredo cruzar a fronteira do banco antes de ser apagado.
func TestS3ConfigRepository_LoadSemSegredo_NaoLeAColuna(t *testing.T) {
	repo := NewS3ConfigRepository(newS3RepoDB(t))
	if err := repo.SaveS3Config(context.Background(), s3RepoUserID, sampleS3Config()); err != nil {
		t.Fatalf("SaveS3Config: %v", err)
	}

	tenho, err := repo.LoadS3ConfigWithoutSecret(context.Background(), s3RepoUserID)
	if err != nil {
		t.Fatalf("LoadS3ConfigWithoutSecret: %v", err)
	}
	if tenho.SecretKey != "" {
		t.Fatalf("SecretKey = %q, quero vazia", tenho.SecretKey)
	}
	// O resto TEM de vir, ou o vazio acima poderia ser "a leitura nao leu".
	if tenho.Bucket != "mybucket" || !tenho.Enabled || tenho.RetentionDays != 7 {
		t.Fatalf("a leitura sem segredo perdeu o resto da configuracao: %+v", *tenho)
	}
	if strings.Contains(s3ConfigSelectWithoutSecretQuery, "s3_secret_key") {
		t.Fatal("a instrucao da leitura publica nomeia s3_secret_key: o segredo cruza a fronteira do banco")
	}
}

// O DELETE restaura o estado limpo historico, INCLUINDO os tres defaults
// (`41bc8e2^:handlers.go:6465`). Zerar tudo para o zero-value deixaria
// media_delivery = "" — valor que nenhum leitor de midia reconhece
// (pkg/bootstrap/eventhandler_message.go:47).
func TestS3ConfigRepository_DeleteRestauraOEstadoLimpo(t *testing.T) {
	repo := NewS3ConfigRepository(newS3RepoDB(t))
	if err := repo.SaveS3Config(context.Background(), s3RepoUserID, sampleS3Config()); err != nil {
		t.Fatalf("SaveS3Config: %v", err)
	}

	if err := repo.DeleteS3Config(context.Background(), s3RepoUserID); err != nil {
		t.Fatalf("DeleteS3Config: %v", err)
	}
	tenho, err := repo.LoadS3Config(context.Background(), s3RepoUserID)
	if err != nil {
		t.Fatalf("LoadS3Config: %v", err)
	}
	quero := appport.S3ConfigRecord{
		PathStyle:     s3DefaultPathStyle,
		MediaDelivery: s3DefaultMediaDelivery,
		RetentionDays: s3DefaultRetentionDays,
	}
	if *tenho != quero {
		t.Fatalf("estado limpo:\n tenho %+v\n quero %+v", *tenho, quero)
	}
}

// Apagar uma configuracao que nunca existiu nao e' erro: revogacao e'
// idempotente por natureza.
func TestS3ConfigRepository_DeleteEhIdempotente(t *testing.T) {
	repo := NewS3ConfigRepository(newS3RepoDB(t))
	ctx := context.Background()
	if err := repo.DeleteS3Config(ctx, s3RepoUserID); err != nil {
		t.Fatalf("primeiro delete: %v", err)
	}
	if err := repo.DeleteS3Config(ctx, s3RepoUserID); err != nil {
		t.Fatalf("segundo delete: %v", err)
	}
}

// Linha ausente devolve (nil, nil): quem nao existe nao tem S3 habilitado, que
// e' o que os dois callers passam a decidir. Propagar erro viraria 500 numa
// pergunta cuja resposta e' "nao".
func TestS3ConfigRepository_LoadSemLinha_NaoEhErro(t *testing.T) {
	repo := NewS3ConfigRepository(newS3RepoDB(t))

	for _, tc := range []struct {
		name string
		load func(context.Context, string) (*appport.S3ConfigRecord, error)
	}{
		{"com segredo", repo.LoadS3Config},
		{"sem segredo", repo.LoadS3ConfigWithoutSecret},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := tc.load(context.Background(), "quem-nao-existe")
			if err != nil {
				t.Fatalf("linha ausente devolveu erro: %v", err)
			}
			if cfg != nil {
				t.Errorf("cfg = %+v, quero nil", cfg)
			}
		})
	}
}

// Colunas nulas (media_delivery e s3_retention_days nascem NULL) chegam com o
// COALESCE aplicado, e nao como erro de scan.
func TestS3ConfigRepository_ColunasNulasCaemNoDefault(t *testing.T) {
	database := newS3RepoDB(t)
	if _, err := database.Exec(database.Rebind(
		`UPDATE users SET media_delivery = NULL, s3_retention_days = NULL WHERE id = ?`),
		s3RepoUserID); err != nil {
		t.Fatalf("anular colunas: %v", err)
	}
	repo := NewS3ConfigRepository(database)

	cfg, err := repo.LoadS3Config(context.Background(), s3RepoUserID)
	if err != nil {
		t.Fatalf("LoadS3Config: %v", err)
	}
	if cfg.MediaDelivery != s3DefaultMediaDelivery {
		t.Errorf("media_delivery = %q, quero %q", cfg.MediaDelivery, s3DefaultMediaDelivery)
	}
	if cfg.RetentionDays != s3DefaultRetentionDays {
		t.Errorf("s3_retention_days = %d, quero %d", cfg.RetentionDays, s3DefaultRetentionDays)
	}
}

// Banco fechado: as quatro instrucoes reportam a falha em vez de a engolir.
func TestS3ConfigRepository_BancoFechado_PropagaOErro(t *testing.T) {
	database := newS3RepoDB(t)
	if err := database.Close(); err != nil {
		t.Fatalf("fechar o banco: %v", err)
	}
	repo := NewS3ConfigRepository(database)
	ctx := context.Background()

	if err := repo.SaveS3Config(ctx, s3RepoUserID, sampleS3Config()); err == nil {
		t.Error("SaveS3Config nao reportou a falha do banco")
	}
	if _, err := repo.LoadS3Config(ctx, s3RepoUserID); err == nil {
		t.Error("LoadS3Config nao reportou a falha do banco")
	}
	if _, err := repo.LoadS3ConfigWithoutSecret(ctx, s3RepoUserID); err == nil {
		t.Error("LoadS3ConfigWithoutSecret nao reportou a falha do banco")
	}
	if err := repo.DeleteS3Config(ctx, s3RepoUserID); err == nil {
		t.Error("DeleteS3Config nao reportou a falha do banco")
	}
}
