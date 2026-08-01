package port

import "context"

// SessionGuard responde à única pergunta que a maioria esmagadora dos use
// cases fazia ao ClientProvider: "existe sessão WhatsApp para este txtID?".
//
// Antes, perguntar isso custava um GetWhatsmeowClient que devolvia
// *whatsmeow.Client — o use case recebia o cliente inteiro do SDK, com suas
// ~200 operações, para em seguida compará-lo com nil e descartá-lo. O tipo
// concreto vazava para a camada de aplicação sem que nada além da existência
// da sessão fosse de fato usado, e testar esses use cases exigia uma sessão
// WhatsApp real. Ver ADR-001.
type SessionGuard interface {
	// EnsureSession devolve nil se há sessão utilizável para txtID, e erro
	// caso contrário. Não devolve o cliente: quem precisa operar sobre a
	// sessão consome uma porta de capacidade (MessageSender, GroupManager,
	// …), não esta.
	EnsureSession(ctx context.Context, txtID string) error
}
