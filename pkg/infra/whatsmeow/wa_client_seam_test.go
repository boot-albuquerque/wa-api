package whatsmeow

import (
	"context"
	"errors"
	"testing"

	whatsmeow "wa-api/internal/waclient"
	"wa-api/internal/waclient/proto/waE2E"
	"wa-api/internal/waclient/store"
	"wa-api/internal/waclient/types"
)

// exatamente o comportamento de ClientManager.GetWhatsmeowClient).
func getterWith(clients map[string]waClient) waClientGetter {
	return func(txtID string) waClient {
		return clients[txtID]
	}
}

// errClient é um waClient mínimo que devolve err para uma operação
// específica — usado nos testes de propagação de erro do SDK.
type errClient struct {
	fakeWAClient
	errOp string
	err   error
}

func (e *errClient) SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	if e.errOp == "SendMessage" {
		return whatsmeow.SendResponse{}, e.err
	}
	return e.fakeWAClient.SendMessage(ctx, to, message, extra...)
}

// errStore simula um *store.Device com Contacts/LIDs para UserAdapter.
type errStore struct{ *store.Device }

// helper: cria um erro para usar nos testes de propagação.
var testErr = errors.New("synthetic SDK error")

// TestFakeWAClient_SatisfiesInterface é o guarda-compilação: o fake tem
// a forma exata de waClient e waClientGetter; se um dia a interface
// ganhar um método novo, este teste quebra antes de qualquer outro.
func TestFakeWAClient_SatisfiesInterface(t *testing.T) {
	var _ waClient = (*fakeWAClient)(nil)
	var _ waClientGetter = getterWith(nil)
}
