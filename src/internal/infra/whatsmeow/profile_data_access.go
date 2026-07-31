package whatsmeow

import (
	"context"

	"wa-api/internal/domain"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// ProfileDataAccess adapta *whatsmeow.Client para a interface
// usecase.ProfileDataAccess, permitindo mock em testes unitários.
type ProfileDataAccess struct {
	client *whatsmeow.Client
}

// NewProfileDataAccess cria o adapter.
func NewProfileDataAccess(client *whatsmeow.Client) *ProfileDataAccess {
	return &ProfileDataAccess{client: client}
}

// PushName retorna o nome público do WhatsApp.
func (d *ProfileDataAccess) PushName() string {
	if d.client.Store != nil {
		return d.client.Store.PushName
	}
	return ""
}

// OwnJID retorna o JID do próprio dispositivo como domain.JID.
func (d *ProfileDataAccess) OwnJID() (domain.JID, bool) {
	if d.client.Store != nil && d.client.Store.ID != nil {
		return domain.JID(d.client.Store.ID.ToNonAD().String()), true
	}
	return "", false
}

// toTypesJID converte domain.JID para types.JID (whatsmeow).
func toTypesJID(jid domain.JID) (types.JID, error) {
	return types.ParseJID(string(jid))
}

// ProfilePictureURL retorna URL e ID da foto de perfil.
func (d *ProfileDataAccess) ProfilePictureURL(ctx context.Context, jid domain.JID) (string, string, error) {
	tj, err := toTypesJID(jid)
	if err != nil {
		return "", "", err
	}
	pic, err := d.client.GetProfilePictureInfo(ctx, tj, &whatsmeow.GetProfilePictureParams{Preview: false})
	if err != nil || pic == nil {
		return "", "", err
	}
	return pic.URL, pic.ID, nil
}

// ContactInfo retorna nome completo e nome comercial do contato.
func (d *ProfileDataAccess) ContactInfo(ctx context.Context, jid domain.JID) (string, string, error) {
	if d.client.Store == nil || d.client.Store.Contacts == nil {
		return "", "", nil
	}
	tj, err := toTypesJID(jid)
	if err != nil {
		return "", "", err
	}
	contacts, err := d.client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		return "", "", err
	}
	if info, ok := contacts[tj]; ok {
		return info.FullName, info.BusinessName, nil
	}
	return "", "", nil
}
