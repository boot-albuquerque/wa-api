package domain

import "errors"

// GetAvatarRequest é a entrada do caso de uso do avatar.
//
// Sem etiquetas `json` e sem ChatTarget: deixou de ser o corpo de
// POST /user/avatar na migração da família de utilizadores. Quem descodifica
// o corpo, resolve o alias `chat` e valida é
// pkg/presentation/http/dto/user.GetAvatarRequest.
type GetAvatarRequest struct {
	Phone   string
	Preview bool
}

// ErrAvatarNotFound é devolvido quando o contato simplesmente não tem foto de
// perfil pública — o caso comum (maioria dos contatos não tem foto pública),
// não um erro de servidor. Antes ficava indistinguível de falha real porque o
// use case devolvia um error genérico e o handler respondia 500 para
// qualquer erro; callers (ex.: wa-worker) tratavam TODO 500 como "sem avatar"
// às cegas, mascarando falhas reais (sessão caída, JID inválido etc.).
var ErrAvatarNotFound = errors.New("no avatar found")

// ErrAvatarUnauthorized é devolvido quando o contato existe e TEM foto, mas
// escondeu-a via configuração de privacidade do WhatsApp — distinto de
// ErrAvatarNotFound (o contato não tem foto): aqui a foto existe, só não é
// visível pra este usuário. Mapeado pelo handler para 403, não 404, para que
// o caller consiga diferenciar "nunca vai ter foto" de "pode voltar a ter se
// a privacidade mudar".
var ErrAvatarUnauthorized = errors.New("avatar hidden by privacy settings")

// GetContactsRequest para POST /user/contacts (no fields needed)
type GetContactsRequest struct{}

// GetBlocklistRequest para GET /user/blocklist (no fields needed)
type GetBlocklistRequest struct{}

// GetPrivacySettingsRequest para GET /user/privacy (no fields needed)
type GetPrivacySettingsRequest struct{}

// SetPrivacySettingRequest é a entrada do caso de uso de privacidade.
type SetPrivacySettingRequest struct {
	PrivacySetting string
	Value          string
}
