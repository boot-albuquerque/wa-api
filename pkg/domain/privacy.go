package domain

import "fmt"

// WhatsAppCheck é o resultado da verificação de um telefone contra a base do
// WhatsApp.
type WhatsAppCheck struct {
	Query        string
	IsIn         bool
	JID          string
	VerifiedName string
}

// AvatarInfo é a foto de perfil de um contato.
type AvatarInfo struct {
	ID  string
	URL string
}

// Blocklist é a lista de bloqueados da sessão.
type Blocklist struct {
	// JIDs são os contatos bloqueados, em forma canônica.
	JIDs []string
	// DHash é o hash que o WhatsApp usa para versionar a lista.
	DHash string
}

// BlocklistUpdate é o resultado de bloquear ou desbloquear um contato.
type BlocklistUpdate struct {
	// ResolvedJID é o JID que a lista de bloqueio de fato usou.
	ResolvedJID JID
	// RequestedJID é o JID pedido, já normalizado. Difere de ResolvedJID
	// quando o pedido veio como LID e precisou ser traduzido.
	RequestedJID JID
	// Entries é a lista de bloqueados resultante.
	Entries []string
	// DHash é o hash da lista resultante.
	DHash string
}

// privacySettingValues é a tabela do que o WhatsApp aceita em cada
// configuração de privacidade.
//
// Vivia em set_privacy_setting.go, escrita em types.PrivacySettingType e
// types.PrivacySetting do SDK. É conhecimento de domínio — quais valores são
// válidos — que estava expresso em tipos de infraestrutura, e por isso
// obrigava o use case a importar o SDK só para validar uma string contra uma
// lista de strings (ADR-001).
var privacySettingValues = map[string][]string{
	"groupadd":     {"all", "contacts", "contact_blacklist", "none"},
	"last":         {"all", "contacts", "contact_blacklist", "none"},
	"status":       {"all", "contacts", "contact_blacklist", "none"},
	"profile":      {"all", "contacts", "contact_blacklist", "none"},
	"readreceipts": {"all", "none"},
	"online":       {"all", "match_last_seen"},
	"calladd":      {"all", "known"},
}

// ValidatePrivacySetting reports whether name is a supported privacy setting
// and value is one of the values WhatsApp accepts for it.
func ValidatePrivacySetting(name, value string) error {
	allowed, ok := privacySettingValues[name]
	if !ok {
		return fmt.Errorf("invalid privacy setting name %q", name)
	}
	for _, v := range allowed {
		if value == v {
			return nil
		}
	}
	return fmt.Errorf("invalid value %q for privacy setting %q", value, name)
}
