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

// --- F219: o `history` da resposta era um LITERAL FIXO -----------------------
//
// `get_status.go:81` devolvia `History: "0"` enquanto todos os campos vizinhos
// vinham do `entry`. Medido em campo a 2026-08-22: com 9999 no banco, o
// `/session/status` respondia `0`. A coluna JÁ era lida pelo `ListUsers` e
// escaneada para `row.History`; nunca chegava ao `domain.UserListEntry`.
//
// O dublê aqui devolve o valor no `UserListEntry` porque é ISSO que o
// repositório real faz depois da correção (`user_repository.go`, campo
// `History: int(row.History.Int64)`). Um dublê que devolvesse sempre zero
// abençoaria o defeito.

func registoComHistorico(id string, history int) *contractsfake.UserRepository {
	return &contractsfake.UserRepository{
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{{
				ID: id, Name: "sessao", JID: "5511@s.whatsapp.net", History: history,
			}}, nil
		},
	}
}

func statusCom(t *testing.T, history int) *domain.GetStatusResult {
	t.Helper()
	ligada := &contractsfake.SessionStatusReader{
		SessionStatusFunc: func(context.Context, string) (bool, bool) { return true, true },
	}
	res, err := session.NewGetStatusUseCase(ligada, registoComHistorico("u1", history), &contractsfake.Logger{}).
		Execute(context.Background(), "u1")
	if err != nil {
		t.Fatalf("Execute devolveu erro: %v", err)
	}
	return res
}

// TestGetStatus_HistoryVemDoRegistoENaoDeUmLiteral é o teste do DEFEITO, com o
// valor exato medido em campo.
func TestGetStatus_HistoryVemDoRegistoENaoDeUmLiteral(t *testing.T) {
	if got := statusCom(t, 9999).History; got != "9999" {
		t.Errorf("History = %q, quero \"9999\" — o valor do registo não chegou à resposta (F219)", got)
	}
}

// TestGetStatus_HistoryZeroContinuaAResponderZero: o zero LEGÍTIMO (limite
// desligado) tem de continuar a sair como "0".
//
// Sem este, trocar o literal por um valor lido passaria a ser indistinguível
// do defeito antigo sempre que o limite estivesse desligado — que é o caso mais
// comum em produção, e portanto o que menos denunciaria a regressão.
func TestGetStatus_HistoryZeroContinuaAResponderZero(t *testing.T) {
	if got := statusCom(t, 0).History; got != "0" {
		t.Errorf("History = %q, quero \"0\" para limite desligado", got)
	}
}

// TestGetStatus_HistoryDistingueValoresDiferentes trava a propriedade que o
// literal violava: valores diferentes no registo produzem respostas
// diferentes. Um literal passa nos dois testes acima se for por acaso igual ao
// esperado; não passa neste.
func TestGetStatus_HistoryDistingueValoresDiferentes(t *testing.T) {
	a, b := statusCom(t, 7).History, statusCom(t, 300).History
	if a == b {
		t.Errorf("History deu %q para 7 e %q para 300 — a resposta não depende do registo", a, b)
	}
	if a != "7" || b != "300" {
		t.Errorf("History = %q e %q, quero \"7\" e \"300\"", a, b)
	}
}
