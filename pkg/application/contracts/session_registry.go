package port

// SessionRegistry guarda os handles vivos de cada sessão. Existe por uma
// razão estrutural, não numérica: o SessionOrchestrator vive em
// pkg/application/session e precisa registrar/recuperar esses handles sem
// importar pkg/infra/whatsmeow, o que inverteria a direção de dependência
// application → infra. Implementado por ClientManager
// (pkg/infra/whatsmeow/client_manager.go), que continua com a mesma
// responsabilidade que já tem hoje.
//
// Escopo deliberadamente estreito: só CRUD de handle. Nada de Start/Stop/
// Reconnect — ciclo de vida é do orchestrator, não do registry.
//
// O handle de MyClient não aparece aqui: quem o monta e registra é o
// adapter de SessionAttachHook, em pkg/bootstrap, porque MyClient carrega
// estado de bootstrap que a camada de aplicação não pode enxergar.
type SessionRegistry interface {
	// Register associa a sessão viva a userID, substituindo qualquer
	// entrada anterior.
	Register(userID string, session Session)

	// Get devolve a sessão de userID; ok é falso se não houver nenhuma.
	Get(userID string) (session Session, ok bool)

	// Unregister remove os handles de userID. Idempotente.
	Unregister(userID string)

	// ProvisionWebhookClient prepara o cliente HTTP de entrega de webhook
	// desta sessão (hoje montado inline em lifecycle.go:234-291). O
	// orchestrator resolve do banco apenas se há proxy a usar — TLS,
	// timeout e redirect policy são detalhe da implementação, não do
	// contrato. proxyURL vazio significa entregar webhook sem proxy.
	ProvisionWebhookClient(userID string, proxyURL string) error
}
