package port

import "context"

// MessageComposer é a capacidade que os use cases de envio exercem antes de
// delegar o envio propriamente dito ao wrapper: garantir que há sessão e
// obter um identificador de mensagem quando o cliente não forneceu um.
//
// Os 12 use cases send_* usavam o *whatsmeow.Client para exatamente essas
// duas coisas — comparar com nil e chamar GenerateMessageID(). Duas operações
// de uma superfície de ~200 métodos, e o preço era a camada de aplicação
// inteira conhecer o SDK. Ver ADR-001.
type MessageComposer interface {
	// SessionGuard é embutido porque todo caminho de envio começa
	// verificando a sessão, inclusive quando o ID já veio na requisição.
	SessionGuard

	// NewMessageID gera um identificador de mensagem para a sessão txtID.
	NewMessageID(ctx context.Context, txtID string) (string, error)
}
