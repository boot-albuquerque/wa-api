package whatsmeow

import (
	"context"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/capabilities/prekeys"
	"wa-api/internal/wa-noise/persistence/store"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// preKeyTransport adapta *Client a prekeys.Transport. Existe para que o pacote
// internal/wa-noise/prekeys possa operar sobre uma interface estreita sem
// importar o pacote raiz (o que fecharia um ciclo) e sem que *Client precise
// ganhar metodos exportados novos so' para satisfazer a interface.
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 4".
type preKeyTransport struct {
	cli *Client
}

var _ prekeys.Transport = preKeyTransport{}

// preKeyT devolve o adaptador de prekeys deste cliente.
func (cli *Client) preKeyT() prekeys.Transport {
	return preKeyTransport{cli}
}

func (t preKeyTransport) Store() *store.Device {
	return t.cli.Store
}

// State devolve o ponteiro para o estado de prekeys do cliente. O ponteiro
// precisa ser estavel — preKeyState e' campo de *Client e preKeyTransport
// embrulha o ponteiro do cliente, entao o mutex de dentro nunca e' copiado por
// valor.
func (t preKeyTransport) State() *prekeys.State {
	return &t.cli.preKeyState
}

func (t preKeyTransport) Log() waLog.Logger {
	return t.cli.Log
}

// SendIQ traduz prekeys.IQ para o infoQuery da raiz. Os campos que este
// dominio nunca preenche (Target, ID, SMaxID, Timeout, NoRetry) ficam no zero,
// exatamente como ficavam quando os nos eram montados na raiz.
func (t preKeyTransport) SendIQ(ctx context.Context, query prekeys.IQ) (*waBinary.Node, error) {
	return t.cli.sendIQ(ctx, infoQuery{
		Namespace: query.Namespace,
		Type:      infoQueryType(query.Type),
		To:        query.To,
		Content:   query.Content,
	})
}
