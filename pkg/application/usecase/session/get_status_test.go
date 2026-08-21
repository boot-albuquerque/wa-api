package session_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/domain"
)

// F196. GET /session/status devolvia 400 "no session" para uma sessão CONHECIDA
// mas desconectada, enquanto GET /session/connect com o mesmo token devolvia
// 200. A guarda exigia um cliente vivo no registry; connect não passa por ela
// porque é quem cria o cliente.
//
// Era a resposta mais inútil possível no único momento em que alguém pergunta o
// estado: quando a sessão NÃO está de pé.

func registoDe(id string) *contractsfake.UserRepository {
	return &contractsfake.UserRepository{
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{{ID: id, Name: "sessao", JID: "5511@s.whatsapp.net"}}, nil
		},
	}
}

// TestGetStatus_DesconectadaDevolveEstado é a correção: sessão que existe no
// banco e não tem cliente vivo responde com o estado, não com uma recusa.
func TestGetStatus_DesconectadaDevolveEstado(t *testing.T) {
	semCliente := &contractsfake.SessionStatusReader{
		SessionStatusFunc: func(context.Context, string) (bool, bool) { return false, false },
	}

	res, err := session.NewGetStatusUseCase(semCliente, registoDe("u1"), &contractsfake.Logger{}).
		Execute(context.Background(), "u1")
	if err != nil {
		t.Fatalf("sessão desconectada foi RECUSADA em vez de reportada (F196): %v", err)
	}
	if res.Connected || res.LoggedIn {
		t.Fatalf("connected=%v loggedIn=%v, quero ambos false", res.Connected, res.LoggedIn)
	}
	// O registo persistido tem de vir junto: é o que torna a resposta útil.
	// Devolver connected:false e mais nada seria trocar uma recusa por um
	// silêncio.
	if res.ID != "u1" || res.Jid == "" {
		t.Fatalf("resultado sem o registo da sessão: %+v", res)
	}
}

// TestGetStatus_ConectadaContinuaAReportar é o outro lado: a correção não pode
// ter tornado o estado sempre false.
func TestGetStatus_ConectadaContinuaAReportar(t *testing.T) {
	comCliente := &contractsfake.SessionStatusReader{
		SessionStatusFunc: func(context.Context, string) (bool, bool) { return true, true },
	}

	res, err := session.NewGetStatusUseCase(comCliente, registoDe("u1"), &contractsfake.Logger{}).
		Execute(context.Background(), "u1")
	if err != nil {
		t.Fatalf("Execute = %v", err)
	}
	if !res.Connected || !res.LoggedIn {
		t.Fatalf("connected=%v loggedIn=%v, quero ambos true", res.Connected, res.LoggedIn)
	}
}

// TestGetStatus_SessaoInexistenteContinuaARecusar trava o limite da correção:
// "desconectada" passa a ser reportada, "não existe" continua a ser recusada.
// Sem isto, a correção teria transformado qualquer token num 200.
func TestGetStatus_SessaoInexistenteContinuaARecusar(t *testing.T) {
	vazio := &contractsfake.UserRepository{
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return nil, nil
		},
	}

	if _, err := session.NewGetStatusUseCase(&contractsfake.SessionStatusReader{}, vazio, &contractsfake.Logger{}).
		Execute(context.Background(), "desconhecido"); err == nil {
		t.Fatal("sessão inexistente devia ser recusada")
	}
}

// TestGetStatus_FalhaDeBancoNaoViraEstadoVazio: se a leitura do registo falhar,
// a resposta não pode ser um 200 com tudo a false — isso diria ao cliente que a
// sessão existe e está desligada, quando não se sabe nada.
func TestGetStatus_FalhaDeBancoNaoViraEstadoVazio(t *testing.T) {
	boom := errors.New("banco fora")
	quebrado := &contractsfake.UserRepository{
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return nil, boom
		},
	}

	res, err := session.NewGetStatusUseCase(&contractsfake.SessionStatusReader{}, quebrado, &contractsfake.Logger{}).
		Execute(context.Background(), "u1")
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, quero envolver %v", err, boom)
	}
	if res != nil {
		t.Fatalf("resultado = %+v, quero nil", res)
	}
}
