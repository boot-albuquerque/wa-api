// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
// Entities são imutáveis e não dependem de frameworks ou bibliotecas externas.
package domain

// Profile representa o perfil público de uma conta WhatsApp conectada.
// Todos os campos são strings (podem ser vazias se o whatsmeow não retornar o campo).
type Profile struct {
	Pushname     string `json:"pushname"`
	AvatarURL    string `json:"avatar_url"`
	AvatarID     string `json:"avatar_id"`
	JID          string `json:"jid"`
	FullName     string `json:"full_name"`
	BusinessName string `json:"business_name"`
}
