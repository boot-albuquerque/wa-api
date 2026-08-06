package port

import "context"

// SessionProvider cria sessões WhatsApp sem que a camada de aplicação
// conheça o SDK que as implementa. É a fronteira que o ADR-0001 precisa
// atravessar: a implementação nativa futura substitui SessionProvider/Session
// inteiros, sem tocar em SessionOrchestrator, use case ou handler.
type SessionProvider interface {
	// NewSession materializa a sessão de spec.UserID, reaproveitando o
	// device persistido se houver (hoje container.GetDevice, lifecycle.go:
	// 187-202) ou criando um novo. Não conecta nem pareia: quem decide isso
	// é o orchestrator, consultando HasCredentials.
	NewSession(ctx context.Context, spec SessionSpec) (Session, error)
}

// SessionSpec é o que o orchestrator resolve do banco antes de pedir a
// sessão. O port não faz I/O de banco: tudo que a implementação precisa
// saber chega por aqui.
//
// Não há JID de entrada por decisão de design: o JID só existe depois do
// pareamento, e "tem device persistido" vs "precisa parear" é justamente o
// que HasCredentials/Pair expõem — passá-lo na entrada duplicaria essa
// resolução do lado de quem chama.
type SessionSpec struct {
	// UserID é a chave de sessão do wa-api (users.id), usada por toda a
	// orquestração e pelos registries.
	UserID string

	// Token é o token de API do usuário. Acompanha a sessão porque os
	// efeitos disparados a partir dela (webhook, cache de UserInfo)
	// correlacionam por token, não por UserID.
	Token string
}

// Session é uma sessão WhatsApp viva. Cobre exatamente as três superfícies
// que uma implementação nativa substitui — pareamento, transporte e eventos
// de sessão — e nada de orquestração (banco, webhook, S3, retry continuam
// no SessionOrchestrator, agnósticos de provider).
type Session interface {
	// HasCredentials informa se já existe device pareado para esta sessão
	// (hoje client.Store.ID != nil, lifecycle.go:296). Falso significa que
	// Pair precisa rodar antes de haver conexão utilizável.
	HasCredentials() bool

	// JID devolve o identificador do device pareado. ok é falso enquanto a
	// sessão não tiver sido pareada.
	JID() (jid string, ok bool)

	// Pair inicia o fluxo de QR e devolve o canal de eventos de pareamento.
	// O canal é fechado pela implementação quando o fluxo termina (sucesso
	// ou expiração final). Erro se a sessão já tiver credenciais.
	Pair(ctx context.Context) (<-chan PairingEvent, error)

	// Connect estabelece o transporte. Só é válido quando HasCredentials —
	// no caminho de pareamento, a conexão faz parte de Pair.
	Connect(ctx context.Context) error

	// Disconnect derruba o transporte sem desfazer o pareamento. Idempotente.
	Disconnect()

	// IsConnected informa se o transporte está de pé. Alimenta status/health.
	IsConnected() bool

	// Logout encerra a autenticação no WhatsApp, descartando as credenciais
	// do device. Após Logout, HasCredentials volta a ser falso.
	Logout(ctx context.Context) error

	// IsLoggedIn informa se a sessão está autenticada (distinto de
	// IsConnected: pode haver credenciais válidas com transporte caído).
	IsLoggedIn() bool

	// SetProxy configura o proxy do transporte. Invariante: deve ser chamado
	// antes de Connect/Pair — implementações devolvem erro se chamado com o
	// transporte já de pé, em vez de aplicar silenciosamente na próxima
	// reconexão.
	SetProxy(cfg ProxyConfig) error

	// Subscribe registra um observador dos eventos de sessão/transporte
	// (os 6 tipos de SessionEvent). Não cobre eventos de domínio — mensagem,
	// presença, grupo e histórico seguem no handler registrado via
	// SessionAttachHook. A função devolvida remove a inscrição e é segura
	// para chamar mais de uma vez.
	Subscribe(fn func(SessionEvent)) (unsubscribe func(), err error)
}

// ProxyMode distingue os dois caminhos de proxy hoje tratados em
// lifecycle.go:267-281, que exigem chamadas diferentes no transporte
// (dialer SOCKS vs endereço HTTP). O modo é explícito no tipo em vez de
// inferido do esquema da URL: quem resolve a configuração no banco já sabe
// qual é, e inferir de novo lá embaixo duplicaria a decisão.
type ProxyMode string

const (
	// ProxyModeSOCKS5 cobre socks5/socks5h — a implementação constrói o
	// dialer a partir de URL.
	ProxyModeSOCKS5 ProxyMode = "socks5"
	// ProxyModeHTTP cobre http/https, aplicados como endereço de proxy.
	ProxyModeHTTP ProxyMode = "http"
)

// ProxyConfig descreve o proxy do transporte da sessão.
type ProxyConfig struct {
	Mode ProxyMode
	// URL é o endereço completo do proxy, incluindo esquema e credenciais
	// quando houver.
	URL string
}
