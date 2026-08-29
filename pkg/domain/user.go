// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
package domain

import (
	"time"
)

// EngineValido aceita o vazio como o padrão dado e recusa qualquer valor que
// não seja um engine conhecido — mesma regra de Engine.IsValidForCreate, mas
// devolvendo string simples (não o tipo Engine) para os call sites que ainda
// trabalham com string crua nesta camada.
func EngineValido(bruto, padrao string) (string, bool) {
	if bruto == "" {
		return padrao, true
	}
	if !Engine(bruto).IsValidForCreate() {
		return "", false
	}
	return bruto, true
}

// ListUsersInput is the use case input for listing users. Empty UserID means
// "every user"; a non-empty one narrows the listing to a single user.
//
// Input and not Request throughout this family: `…Request` is the name the
// convention reserves for the WIRE type, and two types with the same name —
// one of them carrying `json` tags — is the collision a review does not catch
// (docs/HTTP-DTO-CONVENTIONS.md §4).
//
// This one has no wire type at all: the id it carries comes from the PATH of
// GET /admin/users/{id}, and there is no body to decode.
type ListUsersInput struct {
	UserID string
}

// AddUserInput is the use case input for provisioning a new user.
//
// No `json` tags: this is no longer the wire format. The body of
// POST /admin/users is dtoadmin.AddUserRequest, and it is the DTO that
// validates, normalizes and builds this value.
type AddUserInput struct {
	Name        string
	Token       string
	Webhook     string
	Expiration  int
	Events      string
	ProxyConfig *ProxyConfig
	S3Config    *S3Config
	HmacKey     string
	History     int

	// Engine is the transport the caller chose for this session
	// (EngineWaNoise or EngineWaHeadless). It is REQUIRED (items 4-5 of the
	// architectural prompt, F281): the DTO passes it through unmodified,
	// and AddUserUseCase.Execute rejects an absent, null, empty or unknown
	// value with invalid_engine — there is no silent default.
	Engine string
}

// EditUserInput is the use case input for a partial user update.
//
// An empty string means "field not informed" for every string field — the
// semantics the use case already had. History is the exception and stays a
// POINTER (F218): nil is "not mentioned", and a pointer to 0 is "disable the
// limit", two states a plain int cannot tell apart.
type EditUserInput struct {
	UserID      string
	Name        string
	Token       string
	Webhook     string
	Expiration  int
	Events      string
	ProxyConfig *ProxyConfig
	S3Config    *S3Config
	History     *int

	// Engine, quando presente, é comparado ao valor persistido — o motor é
	// IMUTÁVEL depois da criação (F279). nil = campo não mencionado (edição
	// normal, não mexe no motor); ponteiro para string (mesmo vazia) =
	// tentativa de mudar, que só é aceita se for IGUAL ao valor já gravado.
	Engine *string
}

// DeleteUserInput is the use case input for removing a user.
type DeleteUserInput struct {
	UserID string
}

// CheckUserRequest é a entrada dos casos de uso que consultam telefones.
//
// Sem etiquetas `json`: deixou de ser o corpo de POST /user/check e de
// POST /user/info na migração da família de utilizadores. Quem descodifica é
// pkg/presentation/http/dto/user.CheckUserRequest.
type CheckUserRequest struct {
	Phone []string
}

// GetUserLIDRequest é o request para obter o LID de um usuário
type GetUserLIDRequest struct {
	JID string // from URL
}

// BlockUserRequest é a entrada do caso de uso de bloqueio.
//
// Sem etiquetas `json` e sem ChatTarget: deixou de ser o corpo de
// POST /user/block. Quem descodifica, resolve o alias `chat` e valida é
// pkg/presentation/http/dto/user.BlockUserRequest.
type BlockUserRequest struct {
	Phone string
	JID   string
}

// UnblockUserRequest é a entrada do caso de uso de desbloqueio.
type UnblockUserRequest struct {
	Phone string
	JID   string
}

// ProxyConfig representa a configuração de proxy
type ProxyConfig struct {
	Enabled         bool   `json:"enabled"`
	ProxyURL        string `json:"proxyUrl"`
	WebhookUseProxy *bool  `json:"webhookUseProxy,omitempty"`
}

// S3Config representa a configuração S3
type S3Config struct {
	Enabled       bool   `json:"enabled"`
	Endpoint      string `json:"endpoint"`
	Region        string `json:"region"`
	Bucket        string `json:"bucket"`
	AccessKey     string `json:"accessKey"`
	SecretKey     string `json:"secretKey"`
	PathStyle     bool   `json:"pathStyle"`
	PublicURL     string `json:"publicUrl"`
	MediaDelivery string `json:"mediaDelivery"`
	RetentionDays int    `json:"retentionDays"`

	// AccessKeyConfigured (F308) is derived by the repository's SELECT
	// (COALESCE(s3_access_key,'') <> '') for the read path only — it is
	// never set by a write path, and AccessKey above still carries the
	// real secret there. It exists so a caller that only ever reads this
	// struct (the admin listing) can report whether a key is present
	// without the key itself ever leaving the database.
	AccessKeyConfigured bool `json:"accessKeyConfigured"`
}

// UserAccount is the use case RESULT describing one provisioned API user:
// who it is, how it is configured and whether its WhatsApp session is up.
//
// It carries no `json` tags and no map[string]any. The wire shape is
// dtoadmin.UserResponse, built by a hand-written presenter — before this,
// the two configuration blocks were maps assembled inside the use case, so
// the KEY NAMES of a public payload were decided by the application layer
// and no type in the program declared them.
//
// The S3 access key is deliberately absent: the listing used to serve a
// hardcoded "***" for it, which is the same three characters whether a key
// was configured or not — and the repository does not even read the column
// (pkg/infra/db/user_repository.go:277). It carried no information.
type UserAccount struct {
	ID      string
	Name    string
	Token   string
	Webhook string
	JID     string
	QRCode  string

	// Connected and LoggedIn are distinct states: there can be a valid
	// credential with the transport down.
	Connected bool
	LoggedIn  bool

	Expiration int64
	Events     string

	// HmacConfigured reports that a per-user webhook signing key exists. The
	// key itself never leaves the database in cleartext (F158).
	HmacConfigured bool

	// Engine is the transport this session was created with (EngineWaNoise
	// or EngineWaHeadless).
	Engine string

	Proxy UserProxySettings
	S3    UserS3Settings
}

// UserProxySettings is the outbound proxy configuration of one user.
type UserProxySettings struct {
	Enabled bool
	URL     string

	// WebhookUseProxy reports whether webhook deliveries also go through the
	// proxy, as opposed to only the WhatsApp transport.
	WebhookUseProxy bool
}

// UserS3Settings is the media-storage configuration of one user, minus every
// secret: neither the access key nor the secret key belongs in a response.
type UserS3Settings struct {
	Enabled       bool
	Endpoint      string
	Region        string
	Bucket        string
	PathStyle     bool
	PublicURL     string
	MediaDelivery string
	RetentionDays int

	// AccessKeyConfigured (F308) reports whether an access key is present
	// for this session, without exposing the key itself. Before this
	// field, `enabled: true` and an empty key were indistinguishable from
	// the API — an operator had no way to tell a live S3 config from a
	// half-set-up one.
	AccessKeyConfigured bool
}

// SessionDeviceInfo carrega os dados de identidade e estado do aparelho
// pareado que já vivem no store local — nenhum deles custa chamada de rede.
//
// Estão num struct só, e não em seis métodos da porta, porque são lidos de
// uma vez e sempre juntos: quebrar em métodos separados multiplicaria a
// superfície da porta sem dar a ninguém a chance de pedir só um.
type SessionDeviceInfo struct {
	// LID é a identidade do próprio aparelho no espaço @lid, distinta do
	// JID de telefone. Ver a nota sobre identidade dupla em GetLIDForPN.
	LID string `json:"lid"`

	// Platform é o que o WhatsApp reporta do aparelho pareado ("iphone",
	// "android", ...).
	Platform string `json:"platform"`

	// RegistrationID identifica esta instalação no protocolo Signal.
	RegistrationID uint32 `json:"registration_id"`

	// LIDMigrationTimestamp marca quando a conta migrou para o espaço @lid.
	// Zero significa que não houve migração registrada.
	LIDMigrationTimestamp int64 `json:"lid_migration_timestamp"`

	// Initialized indica se o store completou o handshake inicial.
	Initialized bool `json:"initialized"`

	// Connected e LoggedIn são estados distintos, e a diferença importa:
	// pode haver credencial válida com o transporte caído (Desconectar), e
	// nesse caso Connected=false com LoggedIn=true.
	Connected bool `json:"connected"`
	LoggedIn  bool `json:"logged_in"`
}

// ContactName são os nomes que o roster local conhece de um contato.
//
// Tipado, e não o `any` que GetAllContacts devolve: a lista de conversas
// precisa CASAR nomes por JID, e fazer isso com o tipo do SDK arrastaria o
// vendor para dentro da camada de aplicação.
type ContactName struct {
	FullName     string
	FirstName    string
	PushName     string
	BusinessName string
}

// Melhor devolve o nome mais apresentável que se conhece do contato.
//
// A ordem não é arbitrária: FullName vem da agenda de quem consulta e é o
// nome que a pessoa escolheu para aquele contato; PushName é o que o contato
// escolheu para si; BusinessName é o registro comercial. FirstName fica por
// último por ser o mais incompleto. Sem uma ordem declarada, cada chamador
// inventaria a sua e a lista mudaria de nome conforme quem a monta.
func (c ContactName) Melhor() string {
	for _, n := range []string{c.FullName, c.PushName, c.BusinessName, c.FirstName} {
		if n != "" {
			return n
		}
	}
	return ""
}

// ChatSummary é uma conversa na lista: com quem se fala e quando foi a
// última interação. NÃO carrega mensagens — a lista existe para ordenar
// conversas, e trazer conteúdo a tornaria cara sem tornar-se mais útil.
type ChatSummary struct {
	// JID é a identidade do chat como ela vive no histórico local. Para
	// contatos pode ser `@lid` ou `@s.whatsapp.net` (ver a normalização em
	// GetManyLIDsForPNs); para grupos é sempre `@g.us`.
	JID string `json:"jid"`

	Name    string `json:"name"`
	IsGroup bool   `json:"is_group"`

	// LastActivity é o timestamp da mensagem mais recente já persistida
	// nesta conversa. É a chave de ordenação da lista.
	LastActivity time.Time `json:"last_activity"`

	// Os nomes crus ficam disponíveis para quem quiser aplicar outra regra
	// de preferência que não a de ContactName.Melhor.
	PushName     string `json:"push_name,omitempty"`
	FullName     string `json:"full_name,omitempty"`
	BusinessName string `json:"business_name,omitempty"`

	// Phone é o número de telefone da conversa, quando o mapeamento local o
	// resolve. Acréscimo da F181.
	//
	// Existe porque NOME e IDENTIFICADOR LEGÍVEL são problemas diferentes, e
	// só o primeiro é insolúvel. Medido nas contas reais: das 48 conversas
	// sem nome, 44 pessoas não estão no roster — para elas não há nome a
	// obter, e o Baileys confirma que pode nunca haver. Mas as 48 resolvem
	// para telefone, 100%.
	//
	// `182699419517150@lid` não diz nada a ninguém; `556799881100` é
	// reconhecível, pesquisável e colável. É o que o próprio WhatsApp mostra
	// quando não tem nome.
	//
	// É campo PRÓPRIO e NÃO substitui JID de propósito: o JID é o que o
	// cliente usa nas outras rotas, e trocá-lo repetiria o erro que a F183
	// acabou de evitar. Também não vira Name: um cliente que ordene ou
	// pesquise por nome passaria a misturar nomes com números.
	//
	// Vazio para grupos, para newsletters e para quem não tem mapeamento —
	// ausência é resposta, não falha.
	Phone string `json:"phone,omitempty"`
}

// ChatListPage é uma fatia da lista de conversas, com o total para que o
// cliente saiba quanto falta sem precisar paginar até o fim.
type ChatListPage struct {
	Chats  []ChatSummary `json:"chats"`
	Total  int           `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}
