package whatsmeow

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"wa-api/internal/waclient/types"
)

// toJIDs converte uma lista de domain.JID para o tipo do SDK.
func toJIDs(in []domain.JID) ([]types.JID, error) {
	out := make([]types.JID, len(in))
	for i, j := range in {
		parsed, err := toJID(j)
		if err != nil {
			return nil, err
		}
		out[i] = parsed
	}
	return out, nil
}

// GroupAdapter implementa as quatro portas de grupo sobre o clientManager.
//
// Um tipo só implementando GroupDirectory, GroupLifecycle, GroupSettings e
// GroupRequests: a segmentação existe para que cada use case dependa apenas
// da capacidade que exerce, não para multiplicar adapters — todos falam com
// o mesmo cliente.
type GroupAdapter struct {
	*SessionGuardAdapter
}

// NewGroupAdapter cria o adapter com a função de lookup.
func NewGroupAdapter(getClient waClientGetter) *GroupAdapter {
	return &GroupAdapter{SessionGuardAdapter: NewSessionGuardAdapter(getClient)}
}

// bgCtx é context.Background(), nomeado para deixar explícito que não é
// esquecimento: GroupManagementUseCase chamava estas operações com
// context.Background(), ignorando o cancelamento da requisição. O
// comportamento foi preservado literalmente — trocá-lo por ctx é correção de
// lógica, não movimento, e passaria a abortar operações de escrita em grupo
// quando o cliente HTTP desiste. Fica como follow-up nomeado.
var bgCtx = context.Background()

// client devolve o cliente da sessão ou o erro tipado de sessão ausente.
func (a *GroupAdapter) client(txtID string) (waClient, error) {
	client := a.getClient(txtID)
	if client == nil {
		return nil, ErrNoSession(txtID, nil)
	}
	return client, nil
}

// Verificações em tempo de compilação de que o adapter implementa as portas.
var (
	_ appport.GroupDirectory = (*GroupAdapter)(nil)
	_ appport.GroupLifecycle = (*GroupAdapter)(nil)
	_ appport.GroupSettings  = (*GroupAdapter)(nil)
	_ appport.GroupRequests  = (*GroupAdapter)(nil)
)
