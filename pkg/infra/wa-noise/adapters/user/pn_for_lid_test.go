package user

import (
	"context"
	"errors"
	"testing"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"
)

// GetPNForLID é a direção INVERSA de GetLIDForPN, e existe porque a
// identidade é dupla: quem recebe um @lid de um evento precisa poder chegar ao
// telefone sem saber de antemão qual dos dois tinha em mãos.
//
// O contrato de ausência é o mesmo do sentido direto: mapeamento desconhecido
// devolve JID vazia SEM erro. Confundir os dois faria "não conheço este LID"
// virar falha da API.

func adapterComLIDs(t *testing.T, lids *fakeLIDStore) *UserAdapter {
	t.Helper()
	dev := storeWith(lids, nil)
	fake := &testkit.Fake{StoreFn: func() *store.Device { return dev }}
	return NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
}

func TestUserAdapter_GetPNForLID_Resolve(t *testing.T) {
	lid := types.NewJID("90937", types.HiddenUserServer)
	pn := types.NewJID("5511999", types.DefaultUserServer)
	a := adapterComLIDs(t, &fakeLIDStore{mapping: map[types.JID]types.JID{lid: pn}})

	got, err := a.GetPNForLID(context.Background(), "u1", domain.JID(lid.String()))
	if err != nil {
		t.Fatalf("GetPNForLID devolveu erro: %v", err)
	}
	if string(got) != pn.String() {
		t.Errorf("GetPNForLID = %q, quero %q", got, pn.String())
	}
}

// TestUserAdapter_GetPNForLID_SemMapeamentoNaoEhErro trava o contrato que
// diferencia "não conheço" de "falhei". Devolver erro aqui faria o use case
// registrar uma falha inexistente em Unavailable.
func TestUserAdapter_GetPNForLID_SemMapeamentoNaoEhErro(t *testing.T) {
	a := adapterComLIDs(t, &fakeLIDStore{})

	got, err := a.GetPNForLID(context.Background(), "u1", "90937@lid")
	if err != nil {
		t.Fatalf("ausencia de mapeamento virou erro: %v", err)
	}
	if got != "" {
		t.Errorf("GetPNForLID = %q sem mapeamento, quero vazio", got)
	}
}

func TestUserAdapter_GetPNForLID_PropagaErro(t *testing.T) {
	sdkErr := errors.New("store fora do ar")
	a := adapterComLIDs(t, &fakeLIDStore{errOnGet: sdkErr})

	if _, err := a.GetPNForLID(context.Background(), "u1", "90937@lid"); err == nil {
		t.Fatal("GetPNForLID nao propagou o erro do store")
	}
}

func TestUserAdapter_GetPNForLID_NoSession(t *testing.T) {
	a := NewUserAdapter(testkit.GetterWith(nil))

	_, err := a.GetPNForLID(context.Background(), "u1", "90937@lid")

	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("erro = %v, quero no_session (%v)", err, apperr.CategoryValidation)
	}
}
