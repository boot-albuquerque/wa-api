package whatsmeow

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// UserAdapter implementa ContactDirectory, BlocklistManager e PrivacyManager
// sobre o clientManager.
type UserAdapter struct {
	*SessionGuardAdapter
}

// NewUserAdapter cria o adapter com a função de lookup.
func NewUserAdapter(getClient waClientGetter) *UserAdapter {
	return &UserAdapter{SessionGuardAdapter: NewSessionGuardAdapter(getClient)}
}

func (a *UserAdapter) client(txtID string) (waClient, error) {
	client := a.getClient(txtID)
	if client == nil {
		return nil, ErrNoSession(txtID, nil)
	}
	return client, nil
}

// IsOnWhatsApp verifica quais dos telefones informados têm conta.
func (a *UserAdapter) IsOnWhatsApp(ctx context.Context, txtID string, phones []string) ([]domain.WhatsAppCheck, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}
	resp, err := client.IsOnWhatsApp(ctx, phones)
	if err != nil {
		return nil, err
	}

	var out []domain.WhatsAppCheck
	for _, item := range resp {
		verifiedName := ""
		if item.VerifiedName != nil {
			verifiedName = item.VerifiedName.Details.GetVerifiedName()
		}
		out = append(out, domain.WhatsAppCheck{
			Query:        item.Query,
			IsIn:         item.IsIn,
			JID:          item.JID.String(),
			VerifiedName: verifiedName,
		})
	}
	return out, nil
}

// GetUserInfo devolve os metadados dos JIDs informados.
func (a *UserAdapter) GetUserInfo(ctx context.Context, txtID string, jids []domain.JID) (any, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}
	parsed, err := toJIDs(jids)
	if err != nil {
		return nil, err
	}
	return client.GetUserInfo(ctx, parsed)
}

// GetAllContacts devolve a agenda da sessão e a contagem.
func (a *UserAdapter) GetAllContacts(ctx context.Context, txtID string) (any, int, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, 0, err
	}
	contacts, err := client.Store().Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, 0, err
	}
	return contacts, len(contacts), nil
}

// GetLIDForPN resolve o LID correspondente a um número de telefone.
func (a *UserAdapter) GetLIDForPN(ctx context.Context, txtID string, jid domain.JID) (domain.JID, error) {
	client, err := a.client(txtID)
	if err != nil {
		return "", err
	}
	parsed, err := toJID(jid)
	if err != nil {
		return "", err
	}
	lid, err := client.Store().LIDs.GetLIDForPN(ctx, parsed)
	if err != nil {
		return "", err
	}
	if lid.IsEmpty() {
		return "", nil
	}
	return domain.JID(lid.String()), nil
}

// Verificações em tempo de compilação de que o adapter implementa as portas.
var (
	_ appport.ContactDirectory = (*UserAdapter)(nil)
	_ appport.BlocklistManager = (*UserAdapter)(nil)
	_ appport.PrivacyManager   = (*UserAdapter)(nil)
)
