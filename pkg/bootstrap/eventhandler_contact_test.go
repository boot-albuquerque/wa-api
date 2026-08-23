package bootstrap

import (
	"slices"
	"testing"

	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
	"wa-api/pkg/domain"
)

// --- F73: PushName e BusinessName passam a gerar webhook --------------------
//
// Estes dois anunciam que um CONTACTO mudou de nome — caso distinto do
// PushNameSetting, que é o nome do PRÓPRIO utilizador e já era assinável.
// Até 2026-08-21 caíam num `case` que descartava em silêncio.
//
// O que se trava aqui é o par (webhook ligado, payload com o nome ANTIGO e o
// NOVO). O nome antigo é o que permite a quem mantém cache local saber qual
// entrada substituir; sem ele, o webhook diz que algo mudou e não diz o quê.

func TestHandlePushName_GeraWebhookComOsDoisNomes(t *testing.T) {
	evh := &UserEventHandler{UserID: "u1"}
	st := &eventState{postmap: map[string]any{}}

	evh.handlePushName(&events.PushName{
		JID:         types.NewJID("5511999999999", types.DefaultUserServer),
		OldPushName: "Antigo",
		NewPushName: "Novo",
	}, st)

	if st.dowebhook != 1 {
		t.Fatalf("dowebhook = %d, quero 1 — o evento não gera notificação (F73)", st.dowebhook)
	}
	for chave, quero := range map[string]any{
		"type":          "PushName",
		"jid":           "5511999999999@s.whatsapp.net",
		"old_push_name": "Antigo",
		"new_push_name": "Novo",
	} {
		if got := st.postmap[chave]; got != quero {
			t.Errorf("postmap[%q] = %v, quero %v", chave, got, quero)
		}
	}
}

func TestHandleBusinessName_GeraWebhookComOsDoisNomes(t *testing.T) {
	evh := &UserEventHandler{UserID: "u1"}
	st := &eventState{postmap: map[string]any{}}

	evh.handleBusinessName(&events.BusinessName{
		JID:             types.NewJID("5511888888888", types.DefaultUserServer),
		OldBusinessName: "Loja A",
		NewBusinessName: "Loja B",
	}, st)

	if st.dowebhook != 1 {
		t.Fatalf("dowebhook = %d, quero 1 (F73)", st.dowebhook)
	}
	if got := st.postmap["old_business_name"]; got != "Loja A" {
		t.Errorf("old_business_name = %v, quero \"Loja A\"", got)
	}
	if got := st.postmap["new_business_name"]; got != "Loja B" {
		t.Errorf("new_business_name = %v, quero \"Loja B\"", got)
	}
}

// TestF73_EventosSaoAssinaveis: sem isto, o handler despacharia um evento que
// nenhum utilizador pode subscrever — webhook que nunca sai.
func TestF73_EventosSaoAssinaveis(t *testing.T) {
	for _, nome := range []string{"PushName", "BusinessName"} {
		if !slices.Contains(domain.SupportedEventTypes, nome) {
			t.Errorf("%q não está em SupportedEventTypes: o handler despacha e ninguém pode subscrever", nome)
		}
	}
}
