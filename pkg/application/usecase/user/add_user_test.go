package user_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

// hmacKey32 tem exatamente o comprimento mínimo aceito por AddUser.
const hmacKey32 = "0123456789abcdef0123456789abcdef"

func TestAddUserUseCase_Execute_Rejections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  domain.AddUserRequest
		want error // identidade esperada, quando há uma
	}{
		{
			name: "nome vazio",
			req:  domain.AddUserRequest{Token: "tok"},
		},
		{
			name: "token vazio",
			req:  domain.AddUserRequest{Name: "alice"},
		},
		{
			name: "hmac curto demais",
			req:  domain.AddUserRequest{Name: "alice", Token: "tok", HmacKey: "curto"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{}
			uc := user.NewAddUserUseCase(repo, &contractsfake.Logger{})

			resp, err := uc.Execute(context.Background(), tt.req)
			if err == nil {
				t.Fatal("esperava erro de validação")
			}
			if resp != nil {
				t.Errorf("resposta = %+v, queria nil", resp)
			}
			if len(repo.CreateUserCalls) != 0 {
				t.Errorf("CreateUser chamado %d vezes, queria 0", len(repo.CreateUserCalls))
			}
		})
	}
}

func TestAddUserUseCase_Execute_DuplicateToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fn   func(ctx context.Context, rec domain.UserRecord) (bool, error)
	}{
		{
			name: "adapter devolve ErrDuplicateToken",
			fn: func(context.Context, domain.UserRecord) (bool, error) {
				return false, user.ErrDuplicateToken
			},
		},
		{
			name: "adapter recusa sem erro",
			fn: func(context.Context, domain.UserRecord) (bool, error) {
				return false, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{CreateUserFunc: tt.fn}
			uc := user.NewAddUserUseCase(repo, &contractsfake.Logger{})

			_, err := uc.Execute(context.Background(), domain.AddUserRequest{Name: "alice", Token: "tok"})
			if !errors.Is(err, user.ErrDuplicateToken) {
				t.Fatalf("err = %v, queria user.ErrDuplicateToken", err)
			}
		})
	}
}

func TestAddUserUseCase_Execute_RepositoryError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	repo := &contractsfake.UserRepository{
		CreateUserFunc: func(context.Context, domain.UserRecord) (bool, error) { return false, boom },
	}
	logger := &contractsfake.Logger{}
	uc := user.NewAddUserUseCase(repo, logger)

	_, err := uc.Execute(context.Background(), domain.AddUserRequest{Name: "alice", Token: "tok"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, queria embrulhar boom", err)
	}
	rec, ok := logger.FindLevel(contractsfake.LevelError, "Failed to insert user")
	if !ok {
		t.Fatal("esperava log de erro da inserção")
	}
	if !rec.IsStructured() {
		t.Errorf("keyvals = %v, queria pares estruturados", rec.Keyvals)
	}
}

func TestAddUserUseCase_Execute_Success(t *testing.T) {
	t.Parallel()

	useProxy := false
	tests := []struct {
		name           string
		req            domain.AddUserRequest
		wantProxy      bool
		wantS3Enabled  bool
		wantHmacConfig bool
	}{
		{
			name: "mínimo, com defaults",
			req:  domain.AddUserRequest{Name: "alice", Token: "tok"},
			// sem ProxyConfig, webhookUseProxy vira true por default
			wantProxy: true,
		},
		{
			name: "webhookUseProxy explícito em false",
			req: domain.AddUserRequest{
				Name:        "bob",
				Token:       "tok2",
				ProxyConfig: &domain.ProxyConfig{ProxyURL: "http://proxy:8080", WebhookUseProxy: &useProxy},
			},
			wantProxy: false,
		},
		{
			name: "eventos com entradas vazias são ignoradas",
			req:  domain.AddUserRequest{Name: "carol", Token: "tok3", Events: "Message,, ,ReadReceipt"},
			// a entrada vazia entra no `continue`, não vira erro
			wantProxy: true,
		},
		{
			name:           "hmac no comprimento mínimo",
			req:            domain.AddUserRequest{Name: "dave", Token: "tok4", HmacKey: hmacKey32},
			wantProxy:      true,
			wantHmacConfig: true,
		},
		{
			name: "s3 habilitado inicializa o cliente",
			req: domain.AddUserRequest{
				Name:  "erin",
				Token: "tok5",
				S3Config: &domain.S3Config{
					Enabled: true, Endpoint: "http://s3:9000", Region: "us-east-1",
					Bucket: "b", AccessKey: "ak", SecretKey: "sk", PathStyle: true,
					PublicURL: "http://cdn", MediaDelivery: "base64", RetentionDays: 7,
				},
			},
			wantProxy:     true,
			wantS3Enabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{}
			uc := user.NewAddUserUseCase(repo, &contractsfake.Logger{})

			resp, err := uc.Execute(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(repo.CreateUserCalls) != 1 {
				t.Fatalf("CreateUser chamado %d vezes, queria 1", len(repo.CreateUserCalls))
			}
			rec := repo.CreateUserCalls[0].Rec
			if rec.ID == "" {
				t.Error("ID gerado vazio")
			}
			if rec.Name != tt.req.Name || rec.Token != tt.req.Token {
				t.Errorf("registro gravado = %+v, queria nome/token do request", rec)
			}
			if rec.WebhookUseProxy != tt.wantProxy {
				t.Errorf("WebhookUseProxy = %v, queria %v", rec.WebhookUseProxy, tt.wantProxy)
			}
			if resp.ID != rec.ID {
				t.Errorf("resp.ID = %q, queria %q", resp.ID, rec.ID)
			}
			if resp.HmacConfigured != tt.wantHmacConfig {
				t.Errorf("HmacConfigured = %v, queria %v", resp.HmacConfigured, tt.wantHmacConfig)
			}
			if got := resp.S3Config["enabled"]; got != tt.wantS3Enabled {
				t.Errorf("s3 enabled = %v, queria %v", got, tt.wantS3Enabled)
			}
			// A chave de acesso nunca volta em claro na resposta.
			if got := resp.S3Config["access_key"]; got != "***" {
				t.Errorf("access_key = %v, queria mascarada", got)
			}
			if got := resp.ProxyConfig["webhookUseProxy"]; got != tt.wantProxy {
				t.Errorf("proxy webhookUseProxy = %v, queria %v", got, tt.wantProxy)
			}
			if tt.wantHmacConfig && len(rec.HmacKey) == 0 {
				t.Error("HmacKey gravada vazia")
			}
		})
	}
}
