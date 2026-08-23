package session_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/domain"
)

// F198. A rota /status/set/text era um stub: validava, logava
// "set status message validated" e devolvia 200 sem tocar no SDK. O estado do
// utilizador nunca mudava, e o cliente não tinha como distinguir isso de
// sucesso.
//
// É a TERCEIRA ocorrência deste padrão no repositório — a F79 corrigiu
// Disconnect e Logout pela mesma razão. Por isso o que se trava aqui não é
// "responde 200": é que a PORTA FOI CHAMADA, com o texto que o cliente pediu.

func TestSetStatusMessage_ChamaOSDKComOTexto(t *testing.T) {
	st := &contractsfake.StatusMessageSetter{}

	_, err := session.NewSetStatusMessageUseCase(st, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.SetStatusMessageRequest{Body: "disponivel"})
	if err != nil {
		t.Fatalf("Execute = %v", err)
	}

	if len(st.SetStatusMessageCalls) != 1 {
		t.Fatalf("SetStatusMessage chamado %d vez(es), quero 1 — a rota responde 200 sem fazer nada (F198)",
			len(st.SetStatusMessageCalls))
	}
	got := st.SetStatusMessageCalls[0]
	if got.TxtID != "u1" {
		t.Errorf("txtID = %q, quero \"u1\"", got.TxtID)
	}
	// O TEXTO importa: chamar o SDK com a mensagem errada responde 200 e muda
	// o estado para outra coisa, que é pior que não mudar nada.
	if got.Msg != "disponivel" {
		t.Errorf("msg = %q, quero \"disponivel\"", got.Msg)
	}
}

// TestSetStatusMessage_FalhaDoSDKNaoViraSucesso trava o outro lado: se o SDK
// recusar, a rota NÃO pode responder 200 — seria a F198 com mais passos.
func TestSetStatusMessage_FalhaDoSDKNaoViraSucesso(t *testing.T) {
	boom := errors.New("sdk recusou")
	st := &contractsfake.StatusMessageSetter{
		SetStatusMessageFunc: func(context.Context, string, string) error { return boom },
	}

	res, err := session.NewSetStatusMessageUseCase(st, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.SetStatusMessageRequest{Body: "x"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, quero envolver %v", err, boom)
	}
	if res != nil {
		t.Fatalf("resultado = %+v, quero nil quando o SDK falha", res)
	}
}

// TestSetStatusMessage_NaoChamaOSDKSemCorpo confirma que a validação continua
// ANTES da chamada: publicar um estado vazio apagaria o do utilizador.
func TestSetStatusMessage_NaoChamaOSDKSemCorpo(t *testing.T) {
	st := &contractsfake.StatusMessageSetter{}

	if _, err := session.NewSetStatusMessageUseCase(st, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.SetStatusMessageRequest{}); err == nil {
		t.Fatal("corpo vazio devia ser recusado")
	}
	if len(st.SetStatusMessageCalls) != 0 {
		t.Fatalf("SDK chamado %d vez(es) com corpo vazio — apagaria o estado do utilizador",
			len(st.SetStatusMessageCalls))
	}
}
