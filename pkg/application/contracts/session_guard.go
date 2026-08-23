package port

import "context"

// SessionGuard responde à única pergunta que a maioria esmagadora dos use
// cases fazia ao ClientProvider: "existe sessão WhatsApp para este txtID?".
//
// Antes, perguntar isso custava um GetWaNoiseClient que devolvia
// *wanoise.Client — o use case recebia o cliente inteiro do SDK, com suas
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

// Encerrar uma sessão são DUAS capacidades, e separá-las é uma decisão de
// segurança e não de estilo (2026-08-22).
//
// Desconectar derruba o transporte: a sessão pode voltar sozinha. SAIR
// desautentica — e no transporte de página isso DESEMPAREIA a conta, exigindo
// um humano com o telefone para restaurar. A H122 mediu que a operação EXISTE e
// funciona neste build; o bloqueio é de política.
//
// Com uma interface única, um adaptador que só pode desconectar teria de
// implementar o sair para compilar, e a implementação mais provável seria
// chamar a operação que funciona — apagando um pareamento que ninguém pediu
// para apagar. A separação torna isso impossível de acontecer por acidente.

// SessionDisconnector derruba o transporte de uma sessão, sem desautenticar.
type SessionDisconnector interface {
	SessionGuard
	SessionStatusReader

	// Disconnect derruba o transporte da sessão.
	Disconnect(ctx context.Context, txtID string) error
}

// SessionLogouter desautentica a sessão no WhatsApp.
//
// Num transporte de página isto desempareia a conta. Um adaptador que NÃO possa
// desfazer o efeito com segurança deve recusar-se a satisfazer esta porta, em
// vez de a implementar e confiar em quem chama.
type SessionLogouter interface {
	SessionGuard
	SessionStatusReader

	// Logout encerra a autenticação da sessão no WhatsApp.
	Logout(ctx context.Context, txtID string) error
}

// SessionController é a composição das duas, para o adaptador que satisfaz
// ambas declarar isso numa linha.
type SessionController interface {
	SessionDisconnector
	SessionLogouter
}
