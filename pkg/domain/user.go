// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
package domain

import (
	"encoding/json"
	"time"
)

// ListUsersRequest é o request para listar usuários
type ListUsersRequest struct {
	UserID string // Optional: if provided, lists a single user
}

// AddUserRequest é o request para adicionar um novo usuário
type AddUserRequest struct {
	Name        string       `json:"name"`
	Token       string       `json:"token"`
	Webhook     string       `json:"webhook,omitempty"`
	Expiration  int          `json:"expiration,omitempty"`
	Events      string       `json:"events,omitempty"`
	ProxyConfig *ProxyConfig `json:"proxyConfig,omitempty"`
	S3Config    *S3Config    `json:"s3Config,omitempty"`
	HmacKey     string       `json:"hmacKey,omitempty"`
	History     int          `json:"history,omitempty"`
}

// EditUserRequest é o request para editar um usuário existente
type EditUserRequest struct {
	UserID      string       `json:"-"` // from URL
	Name        string       `json:"name,omitempty"`
	Token       string       `json:"token,omitempty"`
	Webhook     string       `json:"webhook,omitempty"`
	Expiration  int          `json:"expiration,omitempty"`
	Events      string       `json:"events,omitempty"`
	ProxyConfig *ProxyConfig `json:"proxyConfig,omitempty"`
	S3Config    *S3Config    `json:"s3Config,omitempty"`
	// POINTER, not int, to separate "not mentioned" from "explicitly zero" (F218).
	//
	// With plain int + omitempty, zero was the zero value — indistinguishable
	// from absent. The API accepted setting history to 3, 30, or 1000, but
	// could NEVER set it back to 0 (disable). Same pattern as
	// SendLocationRequest.Latitude (F121): nil = not mentioned, 0 = valid value.
	History *int `json:"history,omitempty"`
}

// --- Aliases snake_case na LEITURA (F210, decisão 49=a do canal) ------------
//
// A API lia estes dois campos em camelCase (`s3Config`, `proxyConfig`) e
// devolvia-os em snake_case (`s3_config`, `proxy_config`, ver
// session.go:66-67). O ciclo mais natural que existe — ler o utilizador, mudar
// um campo, reenviar — chegava com o nome da RESPOSTA, o binding não o
// reconhecia, e o pedido era ignorado em SILÊNCIO com 200.
//
// Medido em campo a 2026-08-22:
//
//	PUT {"name":"lucas","s3_config":{"bucket":"snake-case"}} -> 200, bucket=""
//	PUT {"name":"lucas","s3Config":{"bucket":"camel-case"}}  -> 200, bucket="camel-case"
//
// A escolha foi aceitar OS DOIS na leitura e manter snake_case na resposta.
// Alinhar tudo em snake_case seria mais limpo, mas o README documenta
// camelCase como formato de pedido (README.md:289-311) e não documenta a forma
// da resposta — alinhar em snake partiria o contrato escrito.
//
// O camelCase VENCE quando ambos vêm no mesmo corpo: é o documentado, e quem
// envia os dois de propósito está a pedir ambiguidade, não a exprimir intenção.

// aliasesDeConfig são os nomes alternativos aceites na desserialização.
type aliasesDeConfig struct {
	ProxyConfigSnake *ProxyConfig `json:"proxy_config,omitempty"`
	S3ConfigSnake    *S3Config    `json:"s3_config,omitempty"`
}

// UnmarshalJSON aceita `s3_config`/`proxy_config` além de `s3Config`/`proxyConfig`.
func (r *EditUserRequest) UnmarshalJSON(data []byte) error {
	type semMetodo EditUserRequest // evita recursão infinita
	aux := struct {
		*semMetodo
		aliasesDeConfig
	}{semMetodo: (*semMetodo)(r)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if r.ProxyConfig == nil {
		r.ProxyConfig = aux.ProxyConfigSnake
	}
	if r.S3Config == nil {
		r.S3Config = aux.S3ConfigSnake
	}
	return nil
}

// UnmarshalJSON: o mesmo para a criação. Aplicar só à edição criaria uma
// assimetria nova — PUT a aceitar dois nomes e POST a aceitar um.
func (r *AddUserRequest) UnmarshalJSON(data []byte) error {
	type semMetodo AddUserRequest
	aux := struct {
		*semMetodo
		aliasesDeConfig
	}{semMetodo: (*semMetodo)(r)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if r.ProxyConfig == nil {
		r.ProxyConfig = aux.ProxyConfigSnake
	}
	if r.S3Config == nil {
		r.S3Config = aux.S3ConfigSnake
	}
	return nil
}

// DeleteUserRequest é o request para deletar um usuário
type DeleteUserRequest struct {
	UserID string
}

// CheckUserRequest é o request para verificar se um usuário está no WhatsApp
type CheckUserRequest struct {
	Phone []string `json:"phone"`
}

// GetUserLIDRequest é o request para obter o LID de um usuário
type GetUserLIDRequest struct {
	JID string // from URL
}

// BlockUserRequest é o request para bloquear um usuário
type BlockUserRequest struct {
	ChatTarget
	Phone string `json:"Phone,omitempty"`
	JID   string `json:"JID,omitempty"`
}

func (r *BlockUserRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// UnblockUserRequest é o request para desbloquear um usuário
type UnblockUserRequest struct {
	ChatTarget
	Phone string `json:"Phone,omitempty"`
	JID   string `json:"JID,omitempty"`
}

func (r *UnblockUserRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

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
}

// UserResponse representa um usuário na resposta
type UserResponse struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Token          string                 `json:"token"`
	Webhook        string                 `json:"webhook"`
	JID            string                 `json:"jid,omitempty"`
	QRCode         string                 `json:"qrcode,omitempty"`
	Connected      bool                   `json:"connected"`
	LoggedIn       bool                   `json:"loggedIn,omitempty"`
	Expiration     int64                  `json:"expiration,omitempty"`
	ProxyConfig    map[string]interface{} `json:"proxy_config,omitempty"`
	S3Config       map[string]interface{} `json:"s3_config,omitempty"`
	Events         string                 `json:"events,omitempty"`
	HmacConfigured bool                   `json:"hmac_configured,omitempty"`
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
