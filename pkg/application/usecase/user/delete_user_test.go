package user_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

func TestDeleteUserUseCase_Execute(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	tests := []struct {
		name       string
		req        domain.DeleteUserRequest
		deleteFunc func(ctx context.Context, id string) (bool, error)
		wantErr    bool
		wantIs     error
		wantCalls  int
		wantLog    string
	}{
		{
			name:    "id vazio nem chega ao repositório",
			req:     domain.DeleteUserRequest{},
			wantErr: true,
		},
		{
			name:       "erro do repositório é embrulhado e logado",
			req:        domain.DeleteUserRequest{UserID: "u1"},
			deleteFunc: func(context.Context, string) (bool, error) { return false, boom },
			wantErr:    true,
			wantIs:     boom,
			wantCalls:  1,
			wantLog:    "Failed to delete user",
		},
		{
			name:       "nenhuma linha removida vira not found",
			req:        domain.DeleteUserRequest{UserID: "u1"},
			deleteFunc: func(context.Context, string) (bool, error) { return false, nil },
			wantErr:    true,
			wantCalls:  1,
		},
		{
			name:      "remoção bem-sucedida",
			req:       domain.DeleteUserRequest{UserID: "u1"},
			wantCalls: 1,
			wantLog:   "User deleted successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{DeleteUserFunc: tt.deleteFunc}
			logger := &contractsfake.Logger{}
			uc := user.NewDeleteUserUseCase(repo, logger)

			err := uc.Execute(context.Background(), tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("err = %v, queria embrulhar %v", err, tt.wantIs)
			}
			if len(repo.DeleteUserCalls) != tt.wantCalls {
				t.Errorf("DeleteUser chamado %d vezes, queria %d", len(repo.DeleteUserCalls), tt.wantCalls)
			}
			if tt.wantLog != "" {
				rec, ok := logger.Find(tt.wantLog)
				if !ok {
					t.Fatalf("log %q ausente; houve %v", tt.wantLog, logger.Messages())
				}
				if v, ok := rec.Keyval("user_id"); !ok || v != tt.req.UserID {
					t.Errorf("keyval user_id = %v, queria %q", v, tt.req.UserID)
				}
			}
		})
	}
}
