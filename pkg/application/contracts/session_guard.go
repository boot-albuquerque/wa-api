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

// SessionController encerra uma sessão WhatsApp.
//
// É a porta que a ADR-001 nomeia para as operações de ciclo de vida que
// sobravam em ClientProvider — as últimas que ainda pediam o cliente concreto
// só para chamar IsConnected/Logout/Disconnect nele.
type SessionController interface {
	SessionGuard
	SessionStatusReader

	// Logout encerra a autenticação da sessão no WhatsApp.
	Logout(ctx context.Context, txtID string) error

	// Disconnect derruba o transporte da sessão.
	Disconnect(ctx context.Context, txtID string) error
}
