package whatsmeow

import (
	"context"
	wasession "wa-api/pkg/infra/wa-noise/session"
	"wa-api/pkg/infra/wa-noise/waclient"

	appport "wa-api/pkg/application/contracts"
)

// GroupAdapter implementa as quatro portas de grupo sobre o clientManager.
//
// Um tipo só implementando GroupDirectory, GroupLifecycle, GroupSettings e
// GroupRequests: a segmentação existe para que cada use case dependa apenas
// da capacidade que exerce, não para multiplicar adapters — todos falam com
// o mesmo cliente.
type GroupAdapter struct {
	*wasession.SessionGuardAdapter
}

// NewGroupAdapter cria o adapter com a função de lookup.
func NewGroupAdapter(getClient waclient.Getter) *GroupAdapter {
	return &GroupAdapter{SessionGuardAdapter: wasession.NewSessionGuardAdapter(getClient)}
}

// bgCtx é context.Background(), nomeado para deixar explícito que não é
// esquecimento: GroupManagementUseCase chamava estas operações com
// context.Background(), ignorando o cancelamento da requisição. O
// comportamento foi preservado literalmente — trocá-lo por ctx é correção de
// lógica, não movimento, e passaria a abortar operações de escrita em grupo
// quando o cliente HTTP desiste. Fica como follow-up nomeado.
var bgCtx = context.Background()

// Verificações em tempo de compilação de que o adapter implementa as portas.
var (
	_ appport.GroupDirectory = (*GroupAdapter)(nil)
	_ appport.GroupLifecycle = (*GroupAdapter)(nil)
	_ appport.GroupSettings  = (*GroupAdapter)(nil)
	_ appport.GroupRequests  = (*GroupAdapter)(nil)
)
