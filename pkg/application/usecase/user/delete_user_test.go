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
		req        domain.DeleteUserInput
		deleteFunc func(ctx context.Context, id string) (bool, error)
		wantErr    bool
		wantIs     error
		wantCalls  int
		wantLog    string
	}{
		{
			name:    "id vazio nem chega ao repositório",
			req:     domain.DeleteUserInput{},
			wantErr: true,
		},
		{
			name:       "erro do repositório é embrulhado e logado",
			req:        domain.DeleteUserInput{UserID: "u1"},
			deleteFunc: func(context.Context, string) (bool, error) { return false, boom },
			wantErr:    true,
			wantIs:     boom,
			wantCalls:  1,
			wantLog:    "Failed to delete user",
		},
		{
			name:       "nenhuma linha removida vira not found",
			req:        domain.DeleteUserInput{UserID: "u1"},
			deleteFunc: func(context.Context, string) (bool, error) { return false, nil },
			wantErr:    true,
			wantCalls:  1,
		},
		{
			name:      "remoção bem-sucedida",
			req:       domain.DeleteUserInput{UserID: "u1"},
			wantCalls: 1,
			wantLog:   "User deleted successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{DeleteUserFunc: tt.deleteFunc}
			logger := &contractsfake.Logger{}
			rep := &contractsfake.UserInfoRepublisher{}
			uc := user.NewDeleteUserUseCase(repo, rep, logger)

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

// TestDeleteUserUseCase_RepublicaCacheSoAposSucesso é o teste do defeito
// F273: um token de uma sessão APAGADA continuava a autenticar porque nem
// DeleteUserUseCase nem DeleteUserCompleteUseCase invalidavam a entrada de
// cache do token ao apagar a linha. Aqui a asserção é dupla: (1) a
// invalidação acontece, e (2) só quando a deleção teve sucesso — uma falha
// de banco não pode disparar a limpeza de uma cache que ainda reflete um
// usuário que continua na tabela.
func TestDeleteUserUseCase_RepublicaCacheSoAposSucesso(t *testing.T) {
	t.Parallel()

	t.Run("sucesso invalida a cache do usuário apagado", func(t *testing.T) {
		t.Parallel()
		repo := &contractsfake.UserRepository{
			DeleteUserFunc: func(context.Context, string) (bool, error) { return true, nil },
		}
		rep := &contractsfake.UserInfoRepublisher{}
		uc := user.NewDeleteUserUseCase(repo, rep, &contractsfake.Logger{})

		if err := uc.Execute(context.Background(), domain.DeleteUserInput{UserID: "descartavel-2"}); err != nil {
			t.Fatalf("Execute = %v", err)
		}
		if len(rep.RepublishCalls) != 1 {
			t.Fatalf("RepublishUser chamado %d vez(es), queria 1", len(rep.RepublishCalls))
		}
		if rep.RepublishCalls[0].UserID != "descartavel-2" {
			t.Errorf("RepublishUser userID = %q, queria %q", rep.RepublishCalls[0].UserID, "descartavel-2")
		}
	})

	t.Run("erro de banco NÃO invalida a cache", func(t *testing.T) {
		t.Parallel()
		repo := &contractsfake.UserRepository{
			DeleteUserFunc: func(context.Context, string) (bool, error) { return false, errors.New("boom") },
		}
		rep := &contractsfake.UserInfoRepublisher{}
		uc := user.NewDeleteUserUseCase(repo, rep, &contractsfake.Logger{})

		if err := uc.Execute(context.Background(), domain.DeleteUserInput{UserID: "u1"}); err == nil {
			t.Fatal("esperava erro")
		}
		if len(rep.RepublishCalls) != 0 {
			t.Fatalf("RepublishUser chamado %d vez(es), queria 0 — a deleção falhou", len(rep.RepublishCalls))
		}
	})
}

// TestDeleteUserUseCase_InvalidaDEPOISDeApagarENaoAntes trava a ORDEM
// exigida pela política anti-regressão do CLAUDE.md para este achado:
// apagar a linha primeiro, invalidar a cache depois. Invalidar antes
// deixaria uma janela em que uma leitura concorrente repovoa a cache a
// partir da linha que ainda existe (ver a correção sugerida na F273). O
// dublê conta quantas chamadas a DeleteUser já aconteceram no instante em
// que RepublishUser é chamado — inverter a ordem no código faz esse número
// cair para 0.
func TestDeleteUserUseCase_InvalidaDEPOISDeApagarENaoAntes(t *testing.T) {
	t.Parallel()

	repo := &contractsfake.UserRepository{
		DeleteUserFunc: func(context.Context, string) (bool, error) { return true, nil },
	}
	rep := &contractsfake.UserInfoRepublisher{
		ContadorDeEscritas: func() int { return len(repo.DeleteUserCalls) },
	}
	uc := user.NewDeleteUserUseCase(repo, rep, &contractsfake.Logger{})

	if err := uc.Execute(context.Background(), domain.DeleteUserInput{UserID: "descartavel-2"}); err != nil {
		t.Fatalf("Execute = %v", err)
	}

	if len(rep.EscritasAoSerChamado) != 1 {
		t.Fatalf("republicador chamado %d vez(es)", len(rep.EscritasAoSerChamado))
	}
	if rep.EscritasAoSerChamado[0] != 1 {
		t.Fatalf("invalidou com %d deleções feitas, quero 1 — a invalidação está ANTES da deleção",
			rep.EscritasAoSerChamado[0])
	}
}
