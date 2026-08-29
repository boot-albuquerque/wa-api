package user

import (
	"context"
	"testing"

	"wa-api/internal/noise/persistence/store"
	"wa-api/internal/noise/protocol/types"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/infra/noise/client"
	"wa-api/pkg/infra/noise/client/testkit"
)

// ContactNames devolve o roster TIPADO. Existe ao lado de GetAllContacts, que
// devolve `any`, porque quem precisa CASAR nomes por JID não pode receber o
// tipo do SDK — isso arrastaria o vendor para dentro da camada de aplicação.

func TestUserAdapter_ContactNames_NoSession(t *testing.T) {
	a := NewUserAdapter(testkit.GetterWith(nil))

	_, err := a.ContactNames(context.Background(), "u1")

	assertAppErr(t, err, codeUserSessionUnavailable, apperr.CategoryValidation)
}

// TestUserAdapter_ContactNames_ConverteOsQuatroNomes: os quatro campos têm de
// atravessar a fronteira. Perder um silenciosamente faria a lista de conversas
// exibir um nome pior sem nenhum sinal de que algo se perdeu.
func TestUserAdapter_ContactNames_ConverteOsQuatroNomes(t *testing.T) {
	jid := types.NewJID("5511999", types.DefaultUserServer)
	cs := &testkit.ContactStore{Contacts: map[types.JID]types.ContactInfo{
		jid: {Found: true, FullName: "Alice Agenda", FirstName: "Alice", PushName: "Ali", BusinessName: "Loja"},
	}}
	fake := &testkit.Fake{StoreFn: func() *store.Device { return storeWith(nil, cs) }}
	a := NewUserAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))

	got, err := a.ContactNames(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ContactNames devolveu erro: %v", err)
	}

	c, ok := got[domain.JID(jid.String())]
	if !ok {
		t.Fatalf("contato ausente no resultado: %+v", got)
	}
	if c.FullName != "Alice Agenda" || c.FirstName != "Alice" || c.PushName != "Ali" || c.BusinessName != "Loja" {
		t.Errorf("nomes perdidos na conversao: %+v", c)
	}
	if c.Melhor() != "Alice Agenda" {
		t.Errorf("Melhor() = %q, quero a agenda", c.Melhor())
	}
}
