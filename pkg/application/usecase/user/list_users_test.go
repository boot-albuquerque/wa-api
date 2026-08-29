package user_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

func TestListUsersUseCase_Execute_RepositoryError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	repo := &contractsfake.UserRepository{
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) { return nil, boom },
	}
	logger := &contractsfake.Logger{}
	uc := user.NewListUsersUseCase(repo, logger, &contractsfake.SessionStatusReader{})

	users, err := uc.Execute(context.Background(), domain.ListUsersInput{})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, queria embrulhar boom", err)
	}
	if users != nil {
		t.Errorf("users = %v, queria nil", users)
	}
	if !logger.Logged("Failed to list users") {
		t.Error("esperava log de erro da listagem")
	}
}

func TestListUsersUseCase_Execute(t *testing.T) {
	t.Parallel()

	entries := []domain.UserListEntry{
		{
			ID: "u1", Name: "alice", Webhook: "http://w", JID: "5511@s.whatsapp.net",
			QRCode: "qr", Expiration: 42, ProxyURL: "http://proxy", HasProxyURL: true,
			WebhookUseProxy: true, Events: "Message",
			S3:     domain.S3Config{Enabled: true, Bucket: "b", AccessKey: "segredo", RetentionDays: 3},
			Engine: domain.EngineWaHeadless,
		},
		{ID: "u2", Name: "bob"},
	}

	tests := []struct {
		name          string
		entries       []domain.UserListEntry
		statusFunc    func(ctx context.Context, userID string) (bool, bool)
		wantLen       int
		wantConnected bool
	}{
		{
			name:    "lista vazia devolve fatia vazia, não nil",
			entries: nil,
			wantLen: 0,
		},
		{
			name:          "duas entradas com status da sessão",
			entries:       entries,
			statusFunc:    func(context.Context, string) (bool, bool) { return true, true },
			wantLen:       2,
			wantConnected: true,
		},
		{
			name:    "sessão desconectada",
			entries: entries,
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{
				ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
					return tt.entries, nil
				},
			}
			sessions := &contractsfake.SessionStatusReader{SessionStatusFunc: tt.statusFunc}
			uc := user.NewListUsersUseCase(repo, &contractsfake.Logger{}, sessions)

			users, err := uc.Execute(context.Background(), domain.ListUsersInput{UserID: "u1"})
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(users) != tt.wantLen {
				t.Fatalf("len(users) = %d, queria %d", len(users), tt.wantLen)
			}
			if len(repo.ListUsersCalls) != 1 || repo.ListUsersCalls[0].ID != "u1" {
				t.Errorf("ListUsers chamado com %+v, queria filtro u1", repo.ListUsersCalls)
			}
			if tt.wantLen == 0 {
				// Fatia vazia e NUNCA nil: o apresentador serializa `[]`, e
				// `null` é outro valor para qualquer cliente. Antes o use
				// case fazia append num slice nil e devolvia nil.
				if users == nil {
					t.Error("users = nil para a listagem vazia; queria fatia vazia")
				}
				return
			}
			first := users[0]
			// sec/F20: a listagem nunca devolve o token.
			if first.Token != "" {
				t.Errorf("Token = %q, queria vazio na listagem", first.Token)
			}
			// A chave de acesso não sai do use case em forma nenhuma: o
			// resultado não tem campo para ela, mascarada ou não.
			if first.S3.Bucket != "b" {
				t.Errorf("S3.Bucket = %q, queria \"b\"", first.S3.Bucket)
			}
			if first.Connected != tt.wantConnected || first.LoggedIn != tt.wantConnected {
				t.Errorf("Connected/LoggedIn = %v/%v, queria %v", first.Connected, first.LoggedIn, tt.wantConnected)
			}
			if !first.Proxy.Enabled {
				t.Error("Proxy.Enabled = false, queria true")
			}
			// item 9: leitura administrativa devolve o engine.
			if first.Engine != domain.EngineWaHeadless.String() {
				t.Errorf("Engine = %q, queria %q", first.Engine, domain.EngineWaHeadless.String())
			}
			if len(sessions.SessionStatusCalls) != tt.wantLen {
				t.Errorf("SessionStatus chamado %d vezes, queria %d", len(sessions.SessionStatusCalls), tt.wantLen)
			}
		})
	}
}
