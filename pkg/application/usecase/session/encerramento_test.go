package session_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/domain"
)

// Testes da F79: /session/disconnect e /session/logout devolviam 200 sem
// encerrar coisa alguma. Os dois use cases consumiam SessionGuard — uma porta
// que só responde "existe sessão?" — chamavam EnsureSession, logavam
// "disconnect validated" / "logout validated" e retornavam.
//
// Medido em produção antes da correção: duas chamadas a /session/disconnect
// responderam 200, e 29 segundos depois a sessão ainda recebia Message e
// ReadReceipt, com /session/status reportando connected=true, loggedIn=true.
//
// A suíte que já existia (TestUseCases_SemSessao_PropagamACausa) não pegava
// isso: ela só exercita a RECUSA da guarda. Um use case que valida e não age
// passa nela com folga. Estes testes cobrem o caminho de sucesso, que era
// justamente o que ninguém olhava.

var errPorta = errors.New("porta: encerramento falhou")

func TestDisconnectUseCase_DerrubaOTransporte(t *testing.T) {
	sc := &contractsfake.SessionController{}
	log := &contractsfake.Logger{}

	if _, err := session.NewDisconnectUseCase(sc, log).
		Execute(context.Background(), txtID, domain.DisconnectRequest{}); err != nil {
		t.Fatalf("Execute devolveu erro no caminho feliz: %v", err)
	}

	if len(sc.DisconnectCalls) != 1 {
		t.Fatalf("Disconnect chamado %d vezes, quero 1 — o use case voltou a só validar?", len(sc.DisconnectCalls))
	}
	if got := sc.DisconnectCalls[0].TxtID; got != txtID {
		t.Errorf("Disconnect recebeu txtID %q, quero %q", got, txtID)
	}
}

func TestLogoutUseCase_DesvinculaOAparelho(t *testing.T) {
	sc := &contractsfake.SessionController{}
	det := &contractsfake.SessionDetacher{}
	log := &contractsfake.Logger{}

	if _, err := session.NewLogoutUseCase(sc, det, log).
		Execute(context.Background(), txtID, domain.LogoutRequest{}); err != nil {
		t.Fatalf("Execute devolveu erro no caminho feliz: %v", err)
	}

	if len(sc.LogoutCalls) != 1 {
		t.Fatalf("Logout chamado %d vezes, quero 1 — o use case voltou a só validar?", len(sc.LogoutCalls))
	}
	if got := sc.LogoutCalls[0].TxtID; got != txtID {
		t.Errorf("Logout recebeu txtID %q, quero %q", got, txtID)
	}
}

// A falha da porta não pode virar 200. Antes da F79 a questão nem se punha —
// não havia chamada que pudesse falhar.
func TestDisconnectUseCase_FalhaDaPortaPropaga(t *testing.T) {
	sc := &contractsfake.SessionController{
		DisconnectFunc: func(context.Context, string) error { return errPorta },
	}
	log := &contractsfake.Logger{}

	_, err := session.NewDisconnectUseCase(sc, log).
		Execute(context.Background(), txtID, domain.DisconnectRequest{})

	if !errors.Is(err, errPorta) {
		t.Fatalf("a causa da porta se perdeu: %v", err)
	}
	if _, found := log.FindLevel(contractsfake.LevelWarn, "disconnect failed"); !found {
		t.Errorf("falha de desconexão não foi logada em warn: %v", log.Messages())
	}
}

func TestLogoutUseCase_FalhaDaPortaPropaga(t *testing.T) {
	sc := &contractsfake.SessionController{
		LogoutFunc: func(context.Context, string) error { return errPorta },
	}
	det := &contractsfake.SessionDetacher{}
	log := &contractsfake.Logger{}

	_, err := session.NewLogoutUseCase(sc, det, log).
		Execute(context.Background(), txtID, domain.LogoutRequest{})

	if !errors.Is(err, errPorta) {
		t.Fatalf("a causa da porta se perdeu: %v", err)
	}
	if _, found := log.FindLevel(contractsfake.LevelWarn, "logout failed"); !found {
		t.Errorf("falha de logout não foi logada em warn: %v", log.Messages())
	}
}

// A ordem importa: sem sessão, não se age sobre ela. Se alguém inverter as
// duas chamadas, os testes de caminho feliz e de propagação continuam
// passando — este é o único que acusa.
func TestEncerramento_SemSessao_NaoAgeSobreAPorta(t *testing.T) {
	for _, tc := range []struct {
		nome  string
		run   func(*contractsfake.SessionController, *contractsfake.Logger) error
		vezes func(*contractsfake.SessionController) int
	}{
		{
			"Disconnect",
			func(sc *contractsfake.SessionController, l *contractsfake.Logger) error {
				_, err := session.NewDisconnectUseCase(sc, l).Execute(context.Background(), txtID, domain.DisconnectRequest{})
				return err
			},
			func(sc *contractsfake.SessionController) int { return len(sc.DisconnectCalls) },
		},
		{
			"Logout",
			func(sc *contractsfake.SessionController, l *contractsfake.Logger) error {
				_, err := session.NewLogoutUseCase(sc, &contractsfake.SessionDetacher{}, l).Execute(context.Background(), txtID, domain.LogoutRequest{})
				return err
			},
			func(sc *contractsfake.SessionController) int { return len(sc.LogoutCalls) },
		},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			sc := &contractsfake.SessionController{SessionGuard: contractsfake.FailSession(errNoSession)}
			log := &contractsfake.Logger{}

			if err := tc.run(sc, log); !errors.Is(err, errNoSession) {
				t.Fatalf("a recusa da guarda se perdeu: %v", err)
			}
			if n := tc.vezes(sc); n != 0 {
				t.Errorf("agiu sobre a porta %d vezes apesar de não haver sessão", n)
			}
		})
	}
}

// --- F80 ---------------------------------------------------------------

// TestLogoutUseCase_SoltaASessao trava a F80.
//
// O logout iniciado pelo TELEFONE emite *events.LoggedOut, que aciona o
// kill-channel e tira o cliente dos registries. O iniciado pela API não
// emite evento nenhum: o store era apagado, mas o cliente continuava
// registrado com estado obsoleto, e /session/status respondia
// loggedIn=true para uma sessão que já não existia.
//
// Medido antes da correção: POST /session/logout devolveu 200 e logou
// "logged out" para duas sessões; /session/status e /admin/users seguiram
// reportando loggedIn=true nas duas.
func TestLogoutUseCase_SoltaASessao(t *testing.T) {
	sc := &contractsfake.SessionController{}
	det := &contractsfake.SessionDetacher{}
	log := &contractsfake.Logger{}

	if _, err := session.NewLogoutUseCase(sc, det, log).
		Execute(context.Background(), txtID, domain.LogoutRequest{}); err != nil {
		t.Fatalf("Execute devolveu erro no caminho feliz: %v", err)
	}

	if len(det.DetachCalls) != 1 {
		t.Fatalf("Detach chamado %d vezes, quero 1 — a sessao ficou registrada apos o logout", len(det.DetachCalls))
	}
	if got := det.DetachCalls[0].UserID; got != txtID {
		t.Errorf("Detach recebeu userID %q, quero %q", got, txtID)
	}
}

// TestLogoutUseCase_NaoSoltaSeOLogoutFalhou: soltar uma sessao que continua
// autenticada no WhatsApp seria pior que o defeito original — o aparelho
// segue pareado e a API perde o cliente que o representa. Foi exatamente o
// que aconteceria no 500 de "websocket not connected" medido em campo.
func TestLogoutUseCase_NaoSoltaSeOLogoutFalhou(t *testing.T) {
	for _, tc := range []struct {
		nome string
		sc   *contractsfake.SessionController
		erro error
	}{
		{
			"porta recusa",
			&contractsfake.SessionController{
				LogoutFunc: func(context.Context, string) error { return errPorta },
			},
			errPorta,
		},
		{
			"sem sessao",
			&contractsfake.SessionController{SessionGuard: contractsfake.FailSession(errNoSession)},
			errNoSession,
		},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			det := &contractsfake.SessionDetacher{}

			_, err := session.NewLogoutUseCase(tc.sc, det, &contractsfake.Logger{}).
				Execute(context.Background(), txtID, domain.LogoutRequest{})

			if !errors.Is(err, tc.erro) {
				t.Fatalf("erro esperado se perdeu: %v", err)
			}
			if n := len(det.DetachCalls); n != 0 {
				t.Errorf("soltou a sessao %d vezes apesar de o logout ter falhado", n)
			}
		})
	}
}

// TestDisconnectUseCase_NaoSoltaASessao: Desconectar MANTEM o pareamento, e
// portanto a sessao registrada — e' o que permite reconectar sem QR novo.
// Se alguem replicar o Detach da F80 aqui por simetria, /session/status
// passa a responder "no session" onde deveria dizer connected=false,
// loggedIn=true, e o proprio contraste entre as duas acoes se perde.
func TestDisconnectUseCase_NaoSoltaASessao(t *testing.T) {
	sc := &contractsfake.SessionController{}

	if _, err := session.NewDisconnectUseCase(sc, &contractsfake.Logger{}).
		Execute(context.Background(), txtID, domain.DisconnectRequest{}); err != nil {
		t.Fatalf("Execute devolveu erro: %v", err)
	}
	if len(sc.DisconnectCalls) != 1 {
		t.Fatalf("Disconnect chamado %d vezes, quero 1", len(sc.DisconnectCalls))
	}
}
