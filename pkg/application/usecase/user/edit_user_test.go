package user_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

func TestEditUserUseCase_Execute_Rejections(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	tests := []struct {
		name       string
		req        domain.EditUserRequest
		existsFunc func(ctx context.Context, id string) (bool, error)
		wantIs     error
		wantUpdate bool
	}{
		{
			name: "id vazio",
			req:  domain.EditUserRequest{},
		},
		{
			name:       "erro ao consultar existência",
			req:        domain.EditUserRequest{UserID: "u1"},
			existsFunc: func(context.Context, string) (bool, error) { return false, boom },
			wantIs:     boom,
		},
		{
			name:       "usuário inexistente",
			req:        domain.EditUserRequest{UserID: "u1"},
			existsFunc: func(context.Context, string) (bool, error) { return false, nil },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{UserExistsFunc: tt.existsFunc}
			uc := user.NewEditUserUseCase(repo, &contractsfake.Logger{})

			err := uc.Execute(context.Background(), tt.req)
			if err == nil {
				t.Fatal("esperava erro")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("err = %v, queria embrulhar %v", err, tt.wantIs)
			}
			if len(repo.UpdateUserCalls) != 0 {
				t.Errorf("UpdateUser chamado %d vezes, queria 0", len(repo.UpdateUserCalls))
			}
		})
	}
}

func TestEditUserUseCase_Execute_UpdateErrors(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	tests := []struct {
		name      string
		updateErr error
		wantIs    error
		wantLog   bool
	}{
		{
			name:      "token duplicado sobe com a identidade original",
			updateErr: user.ErrDuplicateToken,
			wantIs:    user.ErrDuplicateToken,
		},
		{
			name:      "nada a atualizar sobe sem embrulho",
			updateErr: domain.ErrNoFieldsToUpdate,
			wantIs:    domain.ErrNoFieldsToUpdate,
		},
		{
			name:      "erro genérico vira erro de banco logado",
			updateErr: boom,
			wantIs:    boom,
			wantLog:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{
				UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
				UpdateUserFunc: func(context.Context, string, domain.UserUpdate) error { return tt.updateErr },
			}
			logger := &contractsfake.Logger{}
			uc := user.NewEditUserUseCase(repo, logger)

			err := uc.Execute(context.Background(), domain.EditUserRequest{UserID: "u1", Name: "novo"})
			if !errors.Is(err, tt.wantIs) {
				t.Fatalf("err = %v, queria %v", err, tt.wantIs)
			}
			if got := logger.Logged("Failed to update user"); got != tt.wantLog {
				t.Errorf("log de falha = %v, queria %v", got, tt.wantLog)
			}
		})
	}
}

func TestEditUserUseCase_Execute_InvalidEventEntriesAreSkipped(t *testing.T) {
	t.Parallel()

	repo := &contractsfake.UserRepository{
		UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
	}
	uc := user.NewEditUserUseCase(repo, &contractsfake.Logger{})

	if err := uc.Execute(context.Background(), domain.EditUserRequest{UserID: "u1", Events: "Message,, ,ReadReceipt"}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(repo.UpdateUserCalls) != 1 {
		t.Fatalf("UpdateUser chamado %d vezes, queria 1", len(repo.UpdateUserCalls))
	}
}

func TestEditUserUseCase_Execute_BuildsPartialUpdate(t *testing.T) {
	t.Parallel()

	useProxy := true
	tests := []struct {
		name   string
		req    domain.EditUserRequest
		assert func(t *testing.T, upd domain.UserUpdate)
	}{
		{
			name: "todos os campos escalares",
			req: domain.EditUserRequest{
				UserID: "u1", Name: "n", Token: "t", Webhook: "http://w",
				Expiration: 10, Events: "Message", History: 5,
			},
			assert: func(t *testing.T, upd domain.UserUpdate) {
				t.Helper()
				for name, p := range map[string]bool{
					"Name": upd.Name == nil, "Token": upd.Token == nil,
					"Webhook": upd.Webhook == nil, "Expiration": upd.Expiration == nil,
					"Events": upd.Events == nil, "History": upd.History == nil,
				} {
					if p {
						t.Errorf("%s = nil, queria informado", name)
					}
				}
				if upd.ProxyURL != nil || upd.S3 != nil {
					t.Error("ProxyURL/S3 informados sem estarem no request")
				}
			},
		},
		{
			name: "proxy habilitado propaga a URL",
			req: domain.EditUserRequest{
				UserID:      "u1",
				ProxyConfig: &domain.ProxyConfig{Enabled: true, ProxyURL: "http://proxy:8080", WebhookUseProxy: &useProxy},
			},
			assert: func(t *testing.T, upd domain.UserUpdate) {
				t.Helper()
				if upd.ProxyURL == nil || *upd.ProxyURL != "http://proxy:8080" {
					t.Errorf("ProxyURL = %v, queria a URL do request", upd.ProxyURL)
				}
				if upd.WebhookUseProxy == nil || !*upd.WebhookUseProxy {
					t.Error("WebhookUseProxy não propagado")
				}
			},
		},
		{
			name: "proxy desabilitado zera a URL",
			req: domain.EditUserRequest{
				UserID:      "u1",
				ProxyConfig: &domain.ProxyConfig{Enabled: false, ProxyURL: "http://proxy:8080"},
			},
			assert: func(t *testing.T, upd domain.UserUpdate) {
				t.Helper()
				if upd.ProxyURL == nil || *upd.ProxyURL != "" {
					t.Errorf("ProxyURL = %v, queria string vazia", upd.ProxyURL)
				}
			},
		},
		{
			name: "s3 habilitado",
			req: domain.EditUserRequest{
				UserID:   "u1",
				S3Config: &domain.S3Config{Enabled: true, Bucket: "b", Region: "r"},
			},
			assert: func(t *testing.T, upd domain.UserUpdate) {
				t.Helper()
				if upd.S3 == nil || !upd.S3.Enabled {
					t.Errorf("S3 = %v, queria habilitado", upd.S3)
				}
			},
		},
		{
			name: "s3 desabilitado remove o cliente",
			req: domain.EditUserRequest{
				UserID:   "u1",
				S3Config: &domain.S3Config{Enabled: false},
			},
			assert: func(t *testing.T, upd domain.UserUpdate) {
				t.Helper()
				if upd.S3 == nil || upd.S3.Enabled {
					t.Errorf("S3 = %v, queria desabilitado", upd.S3)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{
				UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
			}
			uc := user.NewEditUserUseCase(repo, &contractsfake.Logger{})

			if err := uc.Execute(context.Background(), tt.req); err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(repo.UpdateUserCalls) != 1 {
				t.Fatalf("UpdateUser chamado %d vezes, queria 1", len(repo.UpdateUserCalls))
			}
			call := repo.UpdateUserCalls[0]
			if call.ID != tt.req.UserID {
				t.Errorf("id = %q, queria %q", call.ID, tt.req.UserID)
			}
			tt.assert(t, call.Update)
		})
	}
}
